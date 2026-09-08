package foundation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/gateway/bifrost"
	"github.com/hurtener/chartworks/internal/jobs"
	broker "github.com/hurtener/chartworks/internal/jobs/pengui"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	semantictopics "github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/internal/topicapi"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/internal/workapi"
)

// work owns all enabled SDK clients and durable-worker goroutines. Its close method is
// joined before the store or shared JWT verifier are released by the composition root.
type work struct {
	sourceService *sources.Service
	engineering   *engineering.Service
	pipelines     *engineering.PipelineService
	handler       http.Handler
	engine        gateway.Engine
	queue         *jobs.Service
	broker        *broker.Provider
	registry      *api.Registry
	cancel        context.CancelFunc
	wait          sync.WaitGroup
	once          sync.Once
	logger        *slog.Logger
}

func setupWork(ctx context.Context, v config.Values, db *postgres.DB, verifier *auth.Verifier, next http.Handler, lookup func(string) (string, bool), log io.Writer) (*work, error) {
	if ctx == nil || db == nil || verifier == nil || next == nil || lookup == nil || log == nil {
		return nil, errors.New("work: complete service dependencies required")
	}
	ctx, cancel := context.WithCancel(ctx)
	w := &work{cancel: cancel, handler: next, logger: slog.New(slog.NewJSONHandler(log, nil))}
	if v.Features.Gateway {
		engine, err := bifrost.New(ctx, v.Gateway, lookup, bifrost.TransportOptions{})
		if err != nil {
			w.close()
			return nil, err
		}
		w.engine = engine
	}
	metadata, err := jobs.NewMetadata(db, jobLimits(v.Jobs))
	if err != nil {
		w.close()
		return nil, err
	}
	w.queue = metadata
	if v.Jobs.Enabled {
		credentials := map[string]broker.Credential{}
		for _, ref := range v.Jobs.Credentials {
			id, ok := lookup(strings.TrimPrefix(ref.ClientID, "env:"))
			secret, has := lookup(strings.TrimPrefix(ref.ClientSecret, "env:"))
			if !ok || !has {
				w.close()
				return nil, errors.New("work: broker credential unavailable")
			}
			credentials[ref.Tenant] = broker.Credential{ClientID: id, Secret: secret}
		}
		provider, err := broker.New(v.Jobs.BrokerURL, credentials, verifier, nil)
		if err != nil {
			w.close()
			return nil, err
		}
		w.broker = provider
		limits := jobLimits(v.Jobs)
		if err := db.ConfigureQueue(ctx, limits); err != nil {
			w.close()
			return nil, err
		}
		queue, err := jobs.New(db, provider, limits, func(stage string) {
			w.logger.Error("durable work failed; inspect current job receipt and dependency status", "stage", stage)
		})
		if err != nil {
			w.close()
			return nil, err
		}
		w.queue = queue
	}
	w.sourceService, err = sources.New(db, v.Sources, lookup)
	if err != nil {
		w.close()
		return nil, err
	}
	var executor *readexec.Executor
	var validator *readexec.Validator
	if v.Sources.Enabled {
		validator, err = readexec.NewValidator(w.sourceService, v.Exec)
		if err != nil {
			w.close()
			return nil, err
		}
	}
	if validator != nil {
		executor, err = readexec.NewExecutor(w.sourceService, db, v.Exec)
		if err != nil {
			w.close()
			return nil, err
		}
	}
	w.engineering, err = engineering.New(db, w.sourceService, validator, executor, w.engine, v, lookup)
	if err != nil {
		w.close()
		return nil, err
	}
	w.pipelines, err = engineering.NewPipelineService(db, w.sourceService, validator, w.engine, v, lookup)
	if err != nil {
		w.close()
		return nil, err
	}
	w.handler = sourceapi.Handler(verifier, w.sourceService, validator, workapi.Handler(verifier, w.engine, w.queue, next))
	w.handler = sourceapi.ExecutionHandler(verifier, validator, executor, w.handler)
	w.handler = sourceapi.EngineeringHandler(verifier, w.engineering, w.handler)
	w.handler = sourceapi.PipelineHandler(verifier, w.pipelines, w.handler)
	topics, err := drafts.New(db, w.sourceService, w.engineering)
	if err != nil {
		w.close()
		return nil, err
	}
	index, err := vindex.New(db)
	if err != nil {
		w.close()
		return nil, err
	}
	published, err := semantictopics.New(db, w.sourceService, index, w.engine)
	if err != nil {
		w.close()
		return nil, err
	}
	rules, err := rulesets.New(db, db)
	if err != nil {
		w.close()
		return nil, err
	}
	w.handler = topicapi.Handler(verifier, topics, published, rules, w.handler)
	publicRegistry, err := PublicRegistry()
	if err != nil {
		w.close()
		return nil, err
	}
	securityRegistry, err := securityapi.APIRegistry(v.Telemetry.Metrics)
	if err != nil {
		w.close()
		return nil, err
	}
	workRegistry, err := workapi.APIRegistry(w.engine, w.queue)
	if err != nil {
		w.close()
		return nil, err
	}
	sourceRegistry, err := sourceapi.SourceRegistry(w.sourceService.Enabled(), validator != nil)
	if err != nil {
		w.close()
		return nil, err
	}
	engineeringRegistry, err := sourceapi.EngineeringAPIRegistry(w.engineering.UploadsEnabled(), w.engineering.ProfilingEnabled(), w.engineering.UploadByteLimit())
	if err != nil {
		w.close()
		return nil, err
	}
	var executionRegistry *api.Registry
	if validator != nil && executor != nil {
		executionRegistry, err = sourceapi.ExecutionAPIRegistry()
		if err != nil {
			w.close()
			return nil, err
		}
	}
	pipelineRegistry, err := sourceapi.PipelineAPIRegistry(w.pipelines.Enabled())
	if err != nil {
		w.close()
		return nil, err
	}
	topicRegistry, err := topicapi.Registry()
	if err != nil {
		w.close()
		return nil, err
	}
	w.registry, err = api.Compose(publicRegistry, securityRegistry, workRegistry, sourceRegistry, engineeringRegistry, executionRegistry, pipelineRegistry, topicRegistry)
	if err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}
func jobLimits(j config.Jobs) jobs.Limits {
	return jobs.Limits{Workers: j.Workers, GlobalConcurrency: j.GlobalConcurrency, TenantConcurrency: j.TenantConcurrency, MaxPending: j.MaxPending, MaxPendingPerTenant: j.MaxPendingPerTenant, MaxAttempts: j.MaxAttempts, Batch: j.Batch, Lease: time.Duration(j.Lease), Heartbeat: time.Duration(j.Heartbeat), Poll: time.Duration(j.Poll), AttemptTimeout: time.Duration(j.AttemptTimeout), Backoff: time.Duration(j.Backoff)}
}
func (w *work) run(ctx context.Context) {
	if w.queue == nil || !w.queue.DispatchEnabled() {
		return
	}
	ctx, stop := context.WithCancel(ctx)
	cancel := w.cancel
	w.cancel = func() { stop(); cancel() }
	w.wait.Add(1)
	go func() {
		defer w.wait.Done()
		if err := w.queue.Run(ctx); err != nil && ctx.Err() == nil {
			w.logger.Error("durable worker stopped; check configured queue limits and metadata availability")
		}
	}()
}
func (w *work) close() {
	w.once.Do(func() {
		w.cancel()
		w.wait.Wait()
		if w.engineering != nil {
			w.engineering.Close()
		}
		if w.pipelines != nil {
			w.pipelines.Close()
		}
		if w.sourceService != nil {
			w.sourceService.Close()
		}
		if w.engine != nil {
			w.engine.Close()
		}
		if w.broker != nil {
			w.broker.Close()
		}
	})
}
