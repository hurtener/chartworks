package foundation

import (
	"net/http"

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
func mountDocuments(limits config.Reporting, db *postgres.DB, verifier *auth.Verifier, blocks *reporting.Service, runs *reporting.Runs, query *nlqexec.Service, runner *jobs.RequestRunner, next http.Handler) (*api.Registry, *reporting.Delivery, *rendering.Service, http.Handler, error) {
	queries := reporting.DocumentsFromQueries(query)
	documents, err := reporting.NewDocuments(db, blocks, queries, limits)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	compositions, err := reporting.NewCompositions(documents, db, runs, queries, runner)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	registry, err := reportingapi.DocumentsRegistry()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	delivery, err := reporting.NewDelivery(blocks, runs, documents, compositions, db, limits.Viewer)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	renderer, err := rendering.New(delivery, 16<<20)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	deliveryRegistry, err := reportingapi.DeliveryRegistry(delivery.CanExecute(), true)
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
