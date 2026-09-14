package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	"strings"
	"testing"
)

func testPhase31CatalogBounds(t *testing.T) {
	for _, kind := range []string{"block", "report"} {
		t.Run(kind, func(t *testing.T) {
			f := newPhase31Fixture(t, false)
			id := "p31-catalog-" + strings.Repeat("x", 108)
			if kind == "block" {
				f.domain.block(t, id, f.domain.base)
			} else {
				f.domain.report(t, id, phase29Text("Bounded retained catalog"), true)
			}
			for n := 0; n < 64; n++ {
				f.run(t, kind, id, fmt.Sprintf("catalog-%02d", n))
			}
			limits := config.DefaultReportingViewer()
			limits.MaxMessageBytes = 16384
			bounded, err := reporting.NewDelivery(f.domain.blocks, f.domain.runs, f.domain.documents, f.domain.compositions, f.domain.f.f.db, limits)
			if err != nil {
				t.Fatal(err)
			}
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			request := reporting.ReportingRunsRequest{Kind: kind, Resource: id, Limit: 64}
			baseline, err := f.service.Runs(t.Context(), f.domain.execute, request)
			wire, encodeErr := json.Marshal(baseline)
			if err != nil || encodeErr != nil || len(baseline.Items) != 64 || len(wire) <= limits.MaxMessageBytes {
				t.Fatal("fixture did not exceed the actual serialized bound", len(wire), len(baseline.Items), err, encodeErr)
			}
			refused, err := bounded.Runs(t.Context(), f.domain.execute, request)
			if !errors.Is(err, reporting.ErrBudget) || len(refused.Items) != 0 || refused.Next != "" {
				t.Fatal("catalog byte limit was advisory or leaked partial metadata", refused, err)
			}
			request.Limit = 1
			page, err := bounded.Runs(t.Context(), f.domain.execute, request)
			if err != nil || len(page.Items) != 1 || page.Next == "" {
				t.Fatal("bounded continuation failed", page, err)
			}
			if beforeQueries != f.domain.attemptCount(t) || beforeModels != f.domain.f.model.requests.Load() {
				t.Fatal("catalog paging executed source/model")
			}
		})
	}
}
