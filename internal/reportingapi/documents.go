package reportingapi

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// DocumentCreate names a stable report/dashboard identity and a private revision.
type DocumentCreate struct {
	ID         string                       `json:"id"`
	Definition reporting.DocumentDefinition `json:"definition"`
}

// DocumentEdit creates an immutable amendment without changing a pending review.
type DocumentEdit struct {
	ExpectedVersion int64                        `json:"expected_version"`
	From            reporting.DocumentReference  `json:"from"`
	Definition      reporting.DocumentDefinition `json:"definition"`
}

// DocumentTransition identifies an exact lifecycle revision and CAS version.
type DocumentTransition struct {
	ExpectedVersion int64  `json:"expected_version"`
	Revision        int64  `json:"revision"`
	Note            string `json:"note"`
}

// DocumentImport carries original JSON as bounded text rather than an open
// transport object. Unsupported versions remain private quarantine records.
type DocumentImport struct {
	ID             string                      `json:"id"`
	DefinitionJSON string                      `json:"definition_json"`
	External       reporting.ExternalReference `json:"external"`
}

func documentReference(q url.Values) (reporting.DocumentReference, error) {
	revision, err := runtimeInt(q, "revision", 0, 1, 256)
	if err != nil {
		return reporting.DocumentReference{}, err
	}
	return reporting.DocumentReference{Revision: int64(revision), Stage: q.Get("stage")}, nil
}

func documentEntries(documents *reporting.Documents, runs *reporting.Compositions) []runtimeEndpoint {
	entries := []runtimeEndpoint{}
	for _, kind := range []string{"report", "dashboard"} {
		path := "/v1/" + kind + "s"
		entries = append(entries,
			runtimeEntry("GET", path, "reporting.read", "list_"+kind+"s", "List authorized published "+kind+" metadata", func(ctx context.Context, e identity.Envelope, _ string, q url.Values, _ struct{}) (reporting.DocumentList, error) {
				limit, err := runtimeInt(q, "limit", 20, 1, 100)
				if err != nil {
					return reporting.DocumentList{}, err
				}
				return documents.List(ctx, e, kind, q.Get("after"), limit)
			}),
			runtimeEntry("POST", path, "reporting.write", "create_"+kind, "Create a private "+kind+" draft", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in DocumentCreate) (reporting.DocumentState, error) {
				return documents.Create(ctx, e, kind, in.ID, in.Definition)
			}),
			runtimeEntry("POST", path+"/import", "reporting.write", "import_"+kind, "Import an exact external version or quarantine unsupported content", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in DocumentImport) (reporting.DocumentImportResult, error) {
				return documents.Import(ctx, e, kind, in.ID, json.RawMessage(in.DefinitionJSON), in.External)
			}),
			runtimeEntry("GET", path+"/{id}", "reporting.read", "read_"+kind, "Read an authorized exact "+kind+" revision without execution", func(ctx context.Context, e identity.Envelope, id string, q url.Values, _ struct{}) (reporting.DocumentView, error) {
				ref, err := documentReference(q)
				if err != nil {
					return reporting.DocumentView{}, err
				}
				return documents.Read(ctx, e, kind, id, ref)
			}),
			runtimeEntry("PUT", path+"/{id}", "reporting.write", "edit_"+kind, "Append an immutable "+kind+" amendment under CAS", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in DocumentEdit) (reporting.DocumentState, error) {
				return documents.Edit(ctx, e, kind, id, in.ExpectedVersion, in.From, in.Definition)
			}),
			runtimeEntry("POST", path+"/{id}/runs", "reporting.execute", "admit_"+kind+"_run", "Reserve a key and seal exact "+kind+" composition inputs", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in reporting.CompositionRequest) (reporting.CompositionView, error) {
				return runs.Admit(ctx, e, kind, id, in)
			}))
		for _, transition := range []string{"review", "publish", "reject", "archive"} {
			action := "reporting.write"
			if transition == "publish" || transition == "reject" {
				action = "reporting.publish"
			}
			entries = append(entries, runtimeEntry("POST", path+"/{id}/"+transition, action, transition+"_"+kind, "Apply an exact version-fenced "+kind+" lifecycle transition", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in DocumentTransition) (reporting.DocumentState, error) {
				return documents.Transition(ctx, e, kind, id, in.ExpectedVersion, in.Revision, transition, in.Note)
			}))
		}
	}
	entries = append(entries,
		runtimeEntry("GET", "/v1/composition-runs/{id}", "reporting.read", "read_composition_run", "Read retained composition metadata with current page redaction", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (reporting.CompositionView, error) {
			return runs.Get(ctx, e, id)
		}),
		runtimeEntry("GET", "/v1/composition-runs/{id}/receipt", "reporting.execute", "inspect_composition_run", "Inspect an actor/session-private composition execution receipt", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (reporting.CompositionView, error) {
			return runs.Inspect(ctx, e, id)
		}),
		runtimeEntry("POST", "/v1/composition-runs/{id}/execute", "reporting.execute", "execute_composition_run", "Execute or explicitly resume one bounded composition attempt", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in RunDispatch) (reporting.CompositionView, error) {
			return runs.Run(ctx, e, id, in.Resume)
		}),
		runtimeEntry("GET", "/v1/composition-runs/{id}/widget", "reporting.read", "read_composition_widget", "Read only the addressed visible widget's exact retained output subset", func(ctx context.Context, e identity.Envelope, id string, q url.Values, _ struct{}) (reporting.CompositionPayload, error) {
			return runs.Widget(ctx, e, id, q.Get("page"), q.Get("widget"))
		}),
		runtimeEntry("POST", "/v1/composition-runs/{id}/cancel", "jobs.cancel", "cancel_composition_run", "Record durable composition cancellation intent without claiming native termination", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (reporting.CompositionView, error) {
			return runs.Cancel(ctx, e, id)
		}),
		runtimeEntry("POST", "/v1/composition-retention", "reporting.retention", "expire_composition_artifacts", "Erase a bounded page of expired composition-owned values", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in RetentionRequest) (RetentionResult, error) {
			n, err := runs.Expire(ctx, e, in.Limit)
			return RetentionResult{Removed: n}, err
		}))
	for i := range entries {
		d := &entries[i].definition
		d.ResourceLoader = "current signed target/page/context reach before protected metadata or retained values; creator and audience grant nothing"
		switch d.ID {
		case "list_reports", "list_dashboards":
			d.Query = []api.Parameter{{Name: "after", In: "query", Type: "string", Max: 128}, {Name: "limit", In: "query", Type: "integer", Min: 1, Max: 100}}
		case "read_report", "read_dashboard":
			d.Query = []api.Parameter{{Name: "revision", In: "query", Type: "integer", Min: 1, Max: 256}, {Name: "stage", In: "query", Type: "string", Max: 16}}
		case "read_composition_widget":
			d.Query = []api.Parameter{{Name: "page", In: "query", Type: "string", Max: 128}, {Name: "widget", In: "query", Type: "string", Max: 128}}
		case "admit_report_run", "admit_dashboard_run":
			d.Effect = "immutable_composition_manifest_reservation"
		case "execute_composition_run":
			d.Effect = "bounded_source_read_optional_model_retained_composition"
		case "cancel_composition_run":
			d.Effect = "durable_cancellation_intent"
		case "expire_composition_artifacts":
			d.Effect = "expired_artifact_erasure"
		}
	}
	return entries
}

// DocumentsRegistry describes the same closed DTOs consumed by the live handler.
// Text-only authoring, composition and retained reads need no model provider.
func DocumentsRegistry() (*api.Registry, error) {
	entries := documentEntries(nil, nil)
	definitions := make([]api.Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.schemaErr != nil {
			return nil, entry.schemaErr
		}
		definitions = append(definitions, entry.definition)
	}
	return api.New(definitions)
}
