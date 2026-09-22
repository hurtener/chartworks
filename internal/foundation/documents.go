package foundation

import (
	"net/http"
	"time"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/store/postgres"
)

// mountDocuments shares the existing query, block, source, model and operation
// services. The metadata/retained-read surface remains available without them.
func mountDocuments(limits config.Reporting, renderConfig config.Rendering, db *postgres.DB, verifier *auth.Verifier, blocks *reporting.Service, runs *reporting.Runs, query *nlqexec.Service, runner *jobs.RequestRunner, next http.Handler) (*api.Registry, *reporting.Delivery, *rendering.Service, http.Handler, error) {
	queries := reporting.DocumentsFromQueries(query)
	documents, err := reporting.NewDocuments(db, blocks, queries, limits)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	compositions, err := reporting.NewCompositions(documents, db, runs, queries, runner)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	registry, err := reportingapi.DocumentsRegistry(documents.CanFilterOptions())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	delivery, err := reporting.NewDelivery(blocks, runs, documents, compositions, db, limits.Viewer)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var renderer *rendering.Service
	if renderConfig.Enabled {
		options := rendering.Options{WorkerVersion: renderConfig.WorkerVersion, ThemeVersion: renderConfig.ThemeVersion, MaxTime: time.Duration(renderConfig.MaxTime), MaxMemoryBytes: renderConfig.MaxMemoryBytes, MaxInputBytes: renderConfig.MaxInputBytes, MaxOutputBytes: renderConfig.MaxOutputBytes, MaxConcurrent: renderConfig.MaxConcurrent, MaxWidgets: renderConfig.MaxWidgets, Retention: time.Duration(renderConfig.Retention), Isolation: renderConfig.Isolation}
		worker, workerErr := rendering.NewProcess(renderConfig.WorkerPath, options)
		if workerErr != nil {
			return nil, nil, nil, nil, workerErr
		}
		renderer, err = rendering.NewManaged(delivery, db, worker, renderConfig.MaxOutputBytes, options)
	}
	if err != nil {
		return nil, nil, nil, nil, err
	}
	deliveryRegistry, err := reportingapi.DeliveryRegistry(delivery.CanExecute(), renderer != nil, renderer != nil && renderer.Durable())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	registry, err = api.Compose(registry, deliveryRegistry)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	handler := reportingapi.DocumentsHandler(verifier, documents, compositions, next)
	return registry, delivery, renderer, reportingapi.DeliveryHandler(verifier, delivery, delivery.CanExecute(), handler, renderer), nil
}
