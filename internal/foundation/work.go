package foundation

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/gateway/bifrost"
	"github.com/hurtener/chartworks/internal/jobs"
	broker "github.com/hurtener/chartworks/internal/jobs/pengui"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/internal/workapi"
)

type workAssembly struct {
	handler http.Handler
	engine gateway.Engine
	queue *jobs.Service
	broker *broker.Provider
	sourceService *sources.Service
	cancel context.CancelFunc
	wait sync.WaitGroup
	once sync.Once
	logger *slog.Logger
}

func setupWork(parent context.Context, v config.Values, db *postgres.DB, verifier *auth.Verifier, lookup config.LookupEnv, logs *Loggers, next http.Handler) (*workAssembly, error) {
	if db == nil || verifier == nil || lookup == nil || logs == nil || next == nil {
		return nil, errors.New("work: incomplete composition dependencies")
	}
	w := &workAssembly{logger: logs.App}
	var err error
	if v.Features.Gateway {
		w.engine, err = bifrost.New(parent, v.Gateway, lookup)
		if err != nil { return nil, err }
	}
	limits, err := jobs.FromConfig(v.Jobs)
	if err != nil { w.close(); return nil, err }
	w.queue, err = jobs.NewMetadata(db, limits)
	if err != nil { w.close(); return nil, err }
	if v.Jobs.Enabled {
		if err := db.ConfigureQueue(parent, limits); err != nil { w.close(); return nil, err }
		w.broker, err = broker.New(v.Jobs.Authority, v.Auth, lookup)
		if err != nil { w.close(); return nil, err }
		w.queue, err = jobs.New(db, w.broker, limits, func(stage string) { logs.App.Error("background work failure", "stage", stage) })
		if err != nil { w.close(); return nil, err }
		ctx, cancel := context.WithCancel(parent)
		w.cancel = cancel
		w.wait.Add(1)
		go func() {
			defer w.wait.Done()
			if err := w.queue.Run(ctx); err != nil { logs.App.Error("background work lifecycle failed", "stage", "worker_run") }
		}()
	}
	w.sourceService, err = sources.New(db, v.Sources, lookup)
	if err != nil { w.close(); return nil, err }
	var validator *readexec.Validator
	if v.Sources.Enabled {
		validator, err = readexec.NewValidator(w.sourceService, v.Exec)
		if err != nil { w.close(); return nil, err }
	}
	w.handler = sourceapi.Handler(verifier, w.sourceService, validator, workapi.Handler(verifier, w.engine, w.queue, next))
	return w, nil
}

func (w *workAssembly) close() {
	w.once.Do(func() {
		if w.cancel != nil { w.cancel() }
		w.wait.Wait()
		if w.sourceService != nil { w.sourceService.Close() }
		if w.engine != nil { w.engine.Close() }
		if w.broker != nil { w.broker.Close() }
	})
}
