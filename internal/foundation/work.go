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

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/gateway/bifrost"
	"github.com/hurtener/chartworks/internal/jobs"
	broker "github.com/hurtener/chartworks/internal/jobs/pengui"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/internal/workapi"
)

// work owns all enabled SDK clients and durable-worker goroutines. Its close method is
// joined before the store or shared JWT verifier are released by the composition root.
type work struct {
	ctx     context.Context
	handler http.Handler
	engine  gateway.Engine
	queue   *jobs.Service
	broker  *broker.Provider
	cancel  context.CancelFunc
	wait    sync.WaitGroup
	once    sync.Once
	logger  *slog.Logger
}

func setupWork(ctx context.Context, v config.Values, db *postgres.DB, verifier *auth.Verifier, next http.Handler, lookup func(string) (string, bool), log io.Writer) (*work, error) {
	ctx, cancel := context.WithCancel(ctx)
	w := &work{ctx: ctx, cancel: cancel, handler: next, logger: slog.New(slog.NewJSONHandler(log, nil))}
	if v.Features.Gateway {
		engine, err := bifrost.New(ctx, v.Gateway, lookup, bifrost.TransportOptions{})
		if err != nil {
			w.close()
			return nil, err
		}
		w.engine = engine
	}
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
		queue, err := jobs.New(db, provider, limits)
		if err != nil {
			w.close()
			return nil, err
		}
		w.queue = queue
	}
	w.handler = workapi.Handler(verifier, w.engine, w.queue, next)
	return w, nil
}
func jobLimits(j config.Jobs) jobs.Limits {
	return jobs.Limits{Workers: j.Workers, GlobalConcurrency: j.GlobalConcurrency, TenantConcurrency: j.TenantConcurrency, MaxPending: j.MaxPending, MaxPendingPerTenant: j.MaxPendingPerTenant, MaxAttempts: j.MaxAttempts, Batch: j.Batch, Lease: time.Duration(j.Lease), Heartbeat: time.Duration(j.Heartbeat), Poll: time.Duration(j.Poll), AttemptTimeout: time.Duration(j.AttemptTimeout), Backoff: time.Duration(j.Backoff)}
}
func (w *work) run() {
	ctx := w.ctx
	if w.queue == nil {
		return
	}
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
		if w.engine != nil {
			w.engine.Close()
		}
		if w.broker != nil {
			w.broker.Close()
		}
	})
}
