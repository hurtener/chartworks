package reportingapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// RunDispatch explicitly executes or resumes one bounded, fenced attempt.
type RunDispatch struct {
	Resume bool `json:"resume"`
}

// RetentionRequest bounds one explicit artifact erasure pass.
type RetentionRequest struct {
	Limit int `json:"limit"`
}

// RetentionResult counts artifacts whose values were erased.
type RetentionResult struct {
	Removed int64 `json:"removed"`
}
type runtimeEndpoint struct {
	definition api.Definition
	schemaErr  error
	call       func(context.Context, identity.Envelope, string, url.Values, []byte) (any, error)
}

func runtimeEntry[I, O any](method, path, action, id, summary string, call func(context.Context, identity.Envelope, string, url.Values, I) (O, error)) runtimeEndpoint {
	d := api.Definition{Operation: api.Operation{Method: method, Path: path, Action: action, Effect: "explicit_fenced_domain_work"}, ID: id, Summary: summary, ResourceLoader: "common service verifies exact retained target, source partition and private provenance", Audit: "domain operation and effect journal; no SQL, rows or credentials", Replay: "never", Errors: runtimeErrors()}
	var schemaErr error
	d.Response, schemaErr = api.SchemaFor(id+"Response", reflect.TypeFor[O](), true)
	if method == "GET" {
		d.Replay = "read"
		d.Effect = "authorized_retained_read"
		d.Audit = "read_only_no_domain_audit"
	} else {
		d.MaxBodyBytes = MaxBodyBytes
		var requestErr error
		d.Request, requestErr = api.SchemaFor(id+"Request", reflect.TypeFor[I](), false, api.NullableCollections)
		if requestErr != nil {
			schemaErr = requestErr
		}
	}
	return runtimeEndpoint{d, schemaErr, func(ctx context.Context, e identity.Envelope, id string, q url.Values, raw []byte) (any, error) {
		var in I
		if method != "GET" {
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, reporting.ErrInvalid
			}
		}
		return call(ctx, e, id, q, in)
	}}
}
func runtimeErrors() []api.ErrorResponse {
	return []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthorized"}, {Status: 401, Code: "unauthenticated"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"}, {Status: 409, Code: "stale_validation"}, {Status: 409, Code: "incomplete"}, {Status: 410, Code: "expired"}, {Status: 413, Code: "limit_exceeded"}, {Status: 422, Code: "invalid_query"}, {Status: 429, Code: "busy"}, {Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"}}
}
func runtimeEntries(runs *reporting.Runs, proposals *engineering.Autopilot, execution, planning bool) []runtimeEndpoint {
	entries := []runtimeEndpoint{
		runtimeEntry("GET", "/v1/reporting-runs/{id}", "reporting.read", "readReportingRun", "Read a retained artifact summary", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (reporting.RunView, error) {
			return runs.Get(ctx, e, id)
		}),
		runtimeEntry("GET", "/v1/reporting-runs/{id}/receipt", "reporting.execute", "inspectReportingRun", "Inspect an actor/session private run receipt", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (reporting.RunView, error) {
			return runs.Inspect(ctx, e, id)
		}),
		runtimeEntry("GET", "/v1/reporting-runs", "reporting.read", "listReportingRuns", "List currently authorized retained artifacts", func(ctx context.Context, e identity.Envelope, _ string, q url.Values, _ struct{}) (reporting.ArtifactList, error) {
			limit, err := runtimeInt(q, "limit", 20, 1, 100)
			if err != nil {
				return reporting.ArtifactList{}, err
			}
			return runs.List(ctx, e, q.Get("after"), limit)
		}),
		runtimeEntry("GET", "/v1/reporting-runs/{id}/rows", "reporting.read", "reportingRunRows", "Page exact retained result values", func(ctx context.Context, e identity.Envelope, id string, q url.Values, _ struct{}) (reporting.ResultPage, error) {
			offset, err := runtimeInt(q, "offset", 0, 0, 10000)
			if err != nil {
				return reporting.ResultPage{}, err
			}
			limit, err := runtimeInt(q, "limit", 100, 1, 1000)
			if err != nil {
				return reporting.ResultPage{}, err
			}
			return runs.Rows(ctx, e, id, offset, limit)
		}),
		runtimeEntry("GET", "/v1/reporting-runs/{id}/output", "reporting.read", "reportingRunOutput", "Read an exact retained output", func(ctx context.Context, e identity.Envelope, id string, q url.Values, _ struct{}) (reporting.RetainedOutput, error) {
			return runs.Output(ctx, e, id, q.Get("output"))
		}),
		runtimeEntry("POST", "/v1/reporting-runs/{id}/cancel", "jobs.cancel", "cancelReportingRun", "Persist explicit run cancellation intent", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (reporting.RunView, error) {
			return runs.Cancel(ctx, e, id)
		}),
		runtimeEntry("POST", "/v1/reporting-retention", "reporting.retention", "expireReportingArtifacts", "Erase expired artifact values in a bounded pass", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in RetentionRequest) (RetentionResult, error) {
			n, err := runs.Expire(ctx, e, in.Limit)
			return RetentionResult{n}, err
		}),
		runtimeEntry("GET", "/v1/engineering-proposals/{id}", "engineering.autopilot.read", "readEngineeringProposal", "Read exact reviewed proposal material and effect evidence", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (engineering.AutopilotProposal, error) {
			return proposals.Get(ctx, e, id)
		}),
	}
	if execution {
		entries = append(entries,
			runtimeEntry("POST", "/v1/blocks/{id}/runs", "reporting.execute", "admitReportingRun", "Seal an idempotent frozen run manifest", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in reporting.RunRequest) (reporting.RunView, error) {
				return runs.Admit(ctx, e, id, in)
			}),
			runtimeEntry("POST", "/v1/reporting-runs/{id}/execute", "reporting.execute", "executeReportingRun", "Execute or explicitly resume one fenced frozen attempt", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in RunDispatch) (reporting.RunView, error) {
				return runs.Run(ctx, e, id, in.Resume)
			}))
	}
	if planning {
		entries = append(entries,
			runtimeEntry("POST", "/v1/engineering-proposals", "engineering.autopilot.propose", "proposeEngineering", "Plan and verify a bounded managed-data proposal", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in engineering.AutopilotGoal) (engineering.AutopilotProposal, error) {
				return proposals.Propose(ctx, e, in)
			}),
			runtimeEntry("PUT", "/v1/engineering-proposals/{id}", "engineering.autopilot.propose", "editEngineeringProposal", "Append a reviewed-material amendment and invalidate approval", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in engineering.AutopilotEditRequest) (engineering.AutopilotProposal, error) {
				return proposals.Edit(ctx, e, id, in)
			}),
			runtimeEntry("POST", "/v1/engineering-proposals/{id}/amend", "engineering.autopilot.propose", "amendEngineeringProposal", "Create independent review material from exact drift evidence", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in engineering.AutopilotAmendRequest) (engineering.AutopilotProposal, error) {
				return proposals.Amend(ctx, e, id, in)
			}),
			runtimeEntry("POST", "/v1/engineering-proposals/{id}/review", "engineering.autopilot.review", "reviewEngineeringProposal", "Independently approve or reject exact proposal material", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in engineering.AutopilotReviewRequest) (engineering.AutopilotProposal, error) {
				return proposals.Review(ctx, e, id, in)
			}),
			runtimeEntry("POST", "/v1/engineering-proposals/{id}/apply", "engineering.autopilot.apply", "applyEngineeringProposal", "Apply reviewed material through ordinary managed-write gates", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in engineering.AutopilotApplyRequest) (engineering.AutopilotProposal, error) {
				return proposals.Apply(ctx, e, id, in)
			}),
			runtimeEntry("POST", "/v1/engineering-proposals/{id}/compensate", "engineering.autopilot.compensate", "compensateEngineeringProposal", "Quarantine only owned unreferenced managed effects", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, in engineering.AutopilotApplyRequest) (engineering.AutopilotProposal, error) {
				return proposals.Compensate(ctx, e, id, in)
			}),
			runtimeEntry("POST", "/v1/engineering-proposals/{id}/drift", "engineering.autopilot.drift", "detectEngineeringDrift", "Retain deduplicated drift amendment evidence", func(ctx context.Context, e identity.Envelope, id string, _ url.Values, _ struct{}) (engineering.AutopilotDrift, error) {
				return proposals.DetectDrift(ctx, e, id)
			}))
	}
	for i := range entries {
		d := &entries[i].definition
		switch d.ID {
		case "admitReportingRun":
			d.Effect = "frozen_manifest_reservation"
		case "executeReportingRun":
			d.Effect = "bounded_source_read_optional_model_retained_artifact"
		case "cancelReportingRun":
			d.Effect = "durable_cancellation_intent"
		case "expireReportingArtifacts":
			d.Effect = "expired_artifact_erasure"
		case "proposeEngineering":
			d.Effect = "bounded_model_review_material"
		case "editEngineeringProposal":
			d.Effect = "review_material_amendment"
		case "reviewEngineeringProposal":
			d.Effect = "independent_business_review"
		case "applyEngineeringProposal":
			d.Effect = "managed_warehouse_write"
		case "compensateEngineeringProposal":
			d.Effect = "managed_visibility_quarantine"
		case "detectEngineeringDrift":
			d.Effect = "bounded_source_observation_amendment_evidence"
		case "amendEngineeringProposal":
			d.Effect = "bounded_model_review_material"
		}
		switch d.ID {
		case "listReportingRuns":
			d.Query = []api.Parameter{{Name: "after", In: "query", Type: "string", Max: 128}, {Name: "limit", In: "query", Type: "integer", Min: 1, Max: 100}}
		case "reportingRunRows":
			d.Query = []api.Parameter{{Name: "offset", In: "query", Type: "integer", Min: 0, Max: 10000}, {Name: "limit", In: "query", Type: "integer", Min: 1, Max: 1000}}
		case "reportingRunOutput":
			d.Query = []api.Parameter{{Name: "output", In: "query", Type: "string", Max: 128}}
		}
	}
	return entries
}
func runtimeInt(q url.Values, key string, fallback, min, max int) (int, error) {
	v, ok := q[key]
	if !ok {
		return fallback, nil
	}
	if len(v) != 1 {
		return 0, reporting.ErrInvalid
	}
	n, err := strconv.Atoi(v[0])
	if err != nil || n < min || n > max || strconv.Itoa(n) != v[0] {
		return 0, reporting.ErrInvalid
	}
	return n, nil
}

// RuntimeRegistry includes retained reads even while execution providers are disabled.
func RuntimeRegistry(execution, planning bool) (*api.Registry, error) {
	entries := runtimeEntries(nil, nil, execution, planning)
	defs := make([]api.Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.schemaErr != nil {
			return nil, entry.schemaErr
		}
		defs = append(defs, entry.definition)
	}
	return api.New(defs)
}

// RuntimeHandler uses the same closed schemas as discovery and delegates authority to the domain services.
func RuntimeHandler(verifier *auth.Verifier, runs *reporting.Runs, proposals *engineering.Autopilot, execution, planning bool, next http.Handler) http.Handler {
	if verifier == nil || runs == nil || proposals == nil || next == nil {
		return http.NotFoundHandler()
	}
	registry, err := RuntimeRegistry(execution, planning)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
	}
	entries := runtimeEntries(runs, proposals, execution, planning)
	calls := map[string]runtimeEndpoint{}
	for _, entry := range entries {
		calls[entry.definition.ID] = entry
	}
	slots := make(chan struct{}, 16)
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers(w)
		d, id, _ := registry.Match(r.Method, r.URL.Path)
		if d.Path == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if !e.Has(d.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			failure(w, reporting.ErrBusy)
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || !validQuery(q, d.Query) || r.URL.RawPath != "" || r.Header.Get("Content-Encoding") != "" || r.Header.Get("Idempotency-Key") != "" {
			failure(w, reporting.ErrInvalid)
			return
		}
		var raw json.RawMessage
		if r.Method == "GET" {
			body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
			if readErr != nil || len(body) > 0 {
				failure(w, reporting.ErrInvalid)
				return
			}
		} else {
			if err = decode(w, r, d, &raw); err != nil {
				failure(w, err)
				return
			}
		}
		out, err := calls[d.ID].call(r.Context(), e, id, q, raw)
		if err != nil {
			failure(w, err)
			return
		}
		if err = r.Context().Err(); err != nil {
			failure(w, err)
			return
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			failure(w, err)
			return
		}
		if len(encoded) > 64<<20 {
			failure(w, reporting.ErrBudget)
			return
		}
		_, _ = w.Write(append(encoded, '\n'))
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, found := registry.Match(r.Method, r.URL.Path); !found {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
