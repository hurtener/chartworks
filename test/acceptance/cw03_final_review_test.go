package acceptance

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

// Output translations are independent of block metadata. Exercise publication,
// delivery, exact accepted locale, retained output reads, and floating references.
func TestCW03OutputLocaleAndEmptyDefaultDelivery(t *testing.T) {
	f := newPhase29Execution(t, false)
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, f.documents, f.compositions, f.f.f.db, config.DefaultReportingViewer())
	if err != nil {
		t.Fatal(err)
	}
	d := cw03Definition(t, f.base)
	d.Metadata = d.Metadata[:1] // Only English at block level; outputs also have Spanish.
	f.block(t, "cw03-locale-independent", d)
	beforeQueries, beforeModels := f.f.f.lookups.Load(), f.f.model.requests.Load()
	for _, tc := range []struct{ requested, title, locale string }{
		{"es-AR", "Salida second", "es-AR"},
		{"es-ES", "Salida second", "es-AR"},
		{"en-US", "Output second", "en"},
		{"fr-CA", "Output second", "en"},
		{"", "Output second", "en"},
	} {
		view, err := delivery.Describe(t.Context(), f.execute, reporting.DeliveryDescribeRequest{Target: reporting.DeliveryTarget{Kind: "block", ID: "cw03-locale-independent"}, Locale: tc.requested})
		if err != nil || len(view.Outputs) != 4 || view.Outputs[0].Title != tc.title || view.Outputs[0].Locale != tc.locale || view.Resource.Locale != d.Metadata[0].Locale {
			t.Fatalf("independent output translation %q: %+v, %v", tc.requested, view, err)
		}
	}
	if f.f.f.lookups.Load() != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("describing output translations performed warehouse/model work")
	}
	accepted, err := f.runs.Admit(t.Context(), f.execute, "cw03-locale-independent", reporting.RunRequest{Key: "cw03-locale-retained", Locale: "es-ES"})
	if err != nil {
		t.Fatal(err)
	}
	done, err := f.runs.Run(t.Context(), f.execute, accepted.ID, false)
	if err != nil || done.Locale != "es-ES" || len(done.QueryAttempts) != 1 {
		t.Fatal("accepted locale/fan-out changed", done, err)
	}
	beforeQueries = f.f.f.lookups.Load()
	reader := phase28Reader(t, f.f, "cw03-locale-reader", done.Block, done.Context)
	view, err := delivery.View(t.Context(), reader, reporting.DeliveryViewRequest{Kind: "block", Run: done.ID, Limit: 1})
	if err != nil || len(view.Outputs) != 4 || view.Outputs[0].Title != "Salida second" || view.Locale != "es-ES" || !reflect.DeepEqual(view.AcceptedSelection, accepted.Selection) {
		t.Fatal("retained locale/selection changed", view, err)
	}
	if f.f.f.lookups.Load() != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("retained localized read performed warehouse/model work")
	}

	// A report legitimately published against enabled defaults may later see a
	// floating block revision with no defaults. Explain the omission without
	// guessing a replacement output or executing the old/default selection.
	report := phase29Text("Synthetic floating defaults")
	report.Widgets = []reporting.Widget{phase29BlockWidget("data", "cw03-locale-independent", 0)}
	state := f.report(t, "cw03-no-default-report", report, true)
	current, err := f.blocks.Read(t.Context(), f.blockAuthor, "cw03-locale-independent", reporting.Reference{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range d.Outputs {
		d.Outputs[i].Intent.DefaultSelected = false
	}
	draft, err := f.blocks.Edit(t.Context(), f.blockAuthor, current.State.ID, reporting.EditRequest{ExpectedVersion: current.State.Version, Definition: d})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, f.blocks, f.blockAuthor, draft)
	beforeQueries = f.f.f.lookups.Load()
	description, err := delivery.Describe(t.Context(), f.execute, reporting.DeliveryDescribeRequest{Target: reporting.DeliveryTarget{Kind: "report", ID: state.ID}, Locale: "es-AR"})
	if err != nil || len(description.Pages) != 1 || len(description.Pages[0].Widgets) != 1 {
		t.Fatal(description, err)
	}
	widget := description.Pages[0].Widgets[0]
	if widget.Code != "output_selection_empty" || widget.Selection == nil || widget.Selection.Mode != "defaults" || len(widget.Selection.Selected) != 0 || len(widget.Selection.Choices) != 4 || widget.Selection.Choices[1].State != "disabled" {
		t.Fatal("empty defaults lost authorized disabled/omitted choices", widget)
	}
	if _, err = f.runs.Admit(t.Context(), f.execute, current.State.ID, reporting.RunRequest{Key: "cw03-no-default-run"}); reporting.SelectionErrorCode(err) != "output_selection_empty" {
		t.Fatal("empty defaults became an implicit execution", err)
	}
	if f.f.f.lookups.Load() != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("omission description/rejection performed warehouse/model work")
	}
}

// The existing domain accepts canonical locale tags up to 64 bytes. Persistence
// must accept the same tags through ordinary native migration and publication.
func TestCW03OutputLocalePersistenceBounds(t *testing.T) {
	f := newPhase29Execution(t, false)
	legacy := phase27Copy(t, f.base)
	const extended = "en-x-aaaaaaa-bbbbbbb-ccccccc-ddddddd-eeeeeee-fffffff"
	legacy.Metadata = append(legacy.Metadata, reporting.Localized{Locale: extended, Title: "Extended locale", Description: "Synthetic locale fixture", Question: "Which observations were retained?"})
	definition, err := reporting.MigrateDefinition(legacy)
	if err != nil {
		t.Fatal(err)
	}
	f.block(t, "cw03-extended-locale", definition)
	exported, err := f.blocks.SQL(t.Context(), f.blockAuthor, "cw03-extended-locale", reporting.Reference{Revision: 1})
	if err != nil || exported.Definition == nil || !reflect.DeepEqual(*exported.Definition, definition) {
		t.Fatal("native locale migration/publication/export lost authored intent", exported, err)
	}
	invalid := phase27Copy(t, definition)
	invalid.Outputs[0].Intent.Metadata[0].Locale = "en-x-" + strings.Repeat("aaaaaaa-", 8) + "a"
	if len(invalid.Outputs[0].Intent.Metadata[0].Locale) <= 64 {
		t.Fatal("oversize fixture must cross the domain ceiling")
	}
	if _, err = f.blocks.Create(t.Context(), f.blockAuthor, reporting.CreateRequest{ID: "cw03-overlong-locale", Definition: invalid}); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("domain accepted an overlong locale", err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	var valid bool
	body, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err = raw.QueryRow(t.Context(), `SELECT chartworks.reporting_output_intents_valid($1::jsonb)`, body).Scan(&valid); err != nil || valid {
		t.Fatal("storage locale ceiling diverged", valid, err)
	}
}
