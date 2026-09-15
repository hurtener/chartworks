// Package reportingapi registers thin consumers of governed block authoring.
package reportingapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

// MaxBodyBytes bounds every reporting JSON request body before decoding.
const MaxBodyBytes = 2 << 20

// Registry advertises only handlers backed by the supplied common services.
func Registry(validation, capture, observe bool) (*api.Registry, error) {
	entries := []struct {
		method, path string
		action       reporting.Access
		id, summary  string
		in, out      reflect.Type
		feature      string
	}{
		{"POST", "/v1/blocks/{id}/parameters/assist", reporting.Write, "parameterizeBlock", "Append an AST-verified typed period amendment without publication", reflect.TypeFor[reporting.ParameterizeRequest](), reflect.TypeFor[reporting.View](), "observe"},
		{"POST", "/v1/blocks/{id}/impact", reporting.Read, "recheckBlockImpact", "Explicitly observe dependency impact without altering definitions", reflect.TypeFor[reporting.ImpactRequest](), reflect.TypeFor[reporting.Impact](), "observe"},
		{"POST", "/v1/blocks/{id}/impact/apply", reporting.Write, "applyBlockImpact", "Create a private draft for an exact current dependency proposal", reflect.TypeFor[reporting.ApplyImpactRequest](), reflect.TypeFor[reporting.View](), "observe"},
		{"POST", "/v1/blocks", reporting.Write, "createBlock", "Create an unvalidated private block draft", reflect.TypeFor[reporting.CreateRequest](), reflect.TypeFor[reporting.View](), ""},
		{"GET", "/v1/blocks", reporting.Read, "listBlocks", "List only currently authorized block definitions", nil, reflect.TypeFor[reporting.Page](), ""},
		{"POST", "/v1/blocks/capture", reporting.Write, "captureBlock", "Capture a completed authorized query as an unvalidated draft", reflect.TypeFor[reporting.CaptureRequest](), reflect.TypeFor[reporting.View](), "capture"},
		{"POST", "/v1/blocks/questions/assess", reporting.Read, "assessBlockQuestions", "Assess authorized localized questions with bounded lexical matching", reflect.TypeFor[reporting.QuestionRequest](), reflect.TypeFor[reporting.Assessment](), ""},
		{"GET", "/v1/blocks/{id}", reporting.Read, "readBlock", "Read a SQL-private published or authorized exact revision", nil, reflect.TypeFor[reporting.View](), ""},
		{"GET", "/v1/blocks/{id}/sql", reporting.SQLRead, "readBlockSQL", "Read SQL through separately scoped inspection authority", nil, reflect.TypeFor[reporting.SQLView](), ""},
		{"GET", "/v1/blocks/{id}/history", reporting.Read, "blockHistory", "Read permission-filtered immutable lifecycle history", nil, reflect.TypeFor[reporting.History](), ""},
		{"PUT", "/v1/blocks/{id}", reporting.Write, "editBlock", "Append a CAS-fenced private amendment without publication", reflect.TypeFor[reporting.EditRequest](), reflect.TypeFor[reporting.View](), ""},
		{"POST", "/v1/blocks/{id}/validate", reporting.Validate, "validateBlock", "Validate an exact revision through the existing bounded read core", reflect.TypeFor[reporting.ValidateRequest](), reflect.TypeFor[reporting.ValidationResult](), "validate"},
		{"POST", "/v1/blocks/{id}/preview", reporting.Preview, "previewBlock", "Preview exact outputs privately without retaining artifacts", reflect.TypeFor[reporting.PreviewRequest](), reflect.TypeFor[reporting.PreviewResult](), "validate"},
		{"POST", "/v1/blocks/{id}/publish", reporting.Publish, "publishBlock", "Publish one exact revision against fresh validation evidence", reflect.TypeFor[reporting.PublishRequest](), reflect.TypeFor[reporting.State](), ""},
		{"POST", "/v1/blocks/{id}/certify", reporting.Certify, "certifyBlock", "Attest to one exact published revision and evidence receipt", reflect.TypeFor[reporting.CertifyRequest](), reflect.TypeFor[reporting.Attestation](), ""},
		{"POST", "/v1/blocks/{id}/withdraw", reporting.Certify, "withdrawBlockCertification", "Withdraw an attestation while preserving its history", reflect.TypeFor[reporting.WithdrawRequest](), reflect.TypeFor[reporting.Withdrawal](), ""},
		{"POST", "/v1/blocks/{id}/reject", reporting.Write, "rejectBlock", "Reject the current private draft without removing history", reflect.TypeFor[reporting.TransitionRequest](), reflect.TypeFor[reporting.State](), ""},
		{"POST", "/v1/blocks/{id}/restore", reporting.Write, "restoreBlock", "Copy an authorized revision into a new unvalidated draft", reflect.TypeFor[reporting.RestoreRequest](), reflect.TypeFor[reporting.View](), ""},
		{"POST", "/v1/blocks/{id}/archive", reporting.Write, "archiveBlock", "Archive the default publication while retaining exact revisions", reflect.TypeFor[reporting.TransitionRequest](), reflect.TypeFor[reporting.State](), ""},
		{"POST", "/v1/blocks/{id}/parameters/resolve", reporting.Read, "resolveBlockParameters", "Resolve typed defaults and logical period windows without execution", reflect.TypeFor[reporting.ResolveRequest](), reflect.TypeFor[reporting.ResolutionResult](), ""},
	}
	definitions := make([]api.Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.feature == "validate" && !validation || entry.feature == "capture" && !capture || entry.feature == "observe" && !observe {
			continue
		}
		response, err := api.SchemaFor(entry.id+"Response", entry.out, true)
		if err != nil {
			return nil, err
		}
		d := api.Definition{Operation: api.Operation{Method: entry.method, Path: entry.path, Action: entry.action.Action(), Effect: "governed_block_metadata"}, ID: entry.id, Summary: entry.summary, ResourceLoader: "reporting.Service and PostgreSQL tenant/revision/parent/private eligibility", Audit: "block lifecycle and common read-attempt journal; no SQL in audit", Response: response, Replay: "never", Errors: []api.ErrorResponse{
			{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 401, Code: "unauthorized"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"}, {Status: 409, Code: "stale_validation"}, {Status: 413, Code: "limit_exceeded"}, {Status: 422, Code: "invalid_query"}, {Status: 429, Code: "busy"}, {Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"}}}
		if entry.in != nil {
			d.MaxBodyBytes = MaxBodyBytes
			d.Request, err = api.SchemaFor(entry.id+"Request", entry.in, false, api.NullableCollections)
			if err != nil {
				return nil, err
			}
		}
		if entry.method == "GET" {
			d.Replay = "read"
			d.Effect = "authorized_metadata_read"
			d.Audit = "read_only_no_domain_audit"
		}
		if entry.id == "listBlocks" {
			d.Query = []api.Parameter{{Name: "after", In: "query", Type: "string", Max: 128}, {Name: "limit", In: "query", Type: "integer", Min: 1, Max: 100}, {Name: "include_drafts", In: "query", Type: "boolean"}}
		}
		if entry.id == "readBlock" || entry.id == "readBlockSQL" {
			d.Query = []api.Parameter{{Name: "revision", In: "query", Type: "integer", Min: 1, Max: 256}, {Name: "draft", In: "query", Type: "boolean"}}
		}
		if entry.id == "previewBlock" || entry.id == "validateBlock" {
			d.Effect = "explicit_bounded_source_read_private_evidence"
		}
		if entry.id == "resolveBlockParameters" || entry.id == "assessBlockQuestions" {
			d.Effect = "authorized_metadata_read"
			d.Audit = "read_only_no_domain_audit"
		}
		definitions = append(definitions, d)
	}
	return api.New(definitions)
}

// Handler verifies action authority before request bodies and delegates every
// addressed-resource and mutation check to the common service, not a router role.
func Handler(verifier *auth.Verifier, service *reporting.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry, err := Registry(service.CanValidate(), service.CanCapture(), service.CanObserve())
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
	}
	requests := make(chan struct{}, service.Limits().MaxConcurrent*4)
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers(w)
		selected, id, _ := registry.Match(r.Method, r.URL.Path)
		if selected.Path == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if !e.Has(selected.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		if id != "" {
			action := reporting.Access(strings.TrimPrefix(selected.Action, "reporting."))
			if err = reporting.Require(e, id, action); err != nil {
				failure(w, err)
				return
			}
		}
		select {
		case requests <- struct{}{}:
			defer func() { <-requests }()
		default:
			failure(w, reporting.ErrBusy)
			return
		}
		if r.URL.RawPath != "" || r.Header.Get("Content-Encoding") != "" || r.Header.Get("Idempotency-Key") != "" {
			failure(w, reporting.ErrInvalid)
			return
		}
		parsedQuery, queryErr := url.ParseQuery(r.URL.RawQuery)
		if queryErr != nil || !validQuery(parsedQuery, selected.Query) {
			failure(w, reporting.ErrInvalid)
			return
		}
		if r.Method == "GET" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
			if err != nil || len(body) > 0 {
				failure(w, reporting.ErrInvalid)
				return
			}
		}
		var out any
		switch selected.ID {
		case "parameterizeBlock":
			var in reporting.ParameterizeRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Parameterize(r.Context(), e, id, in)
			}
		case "recheckBlockImpact":
			var in reporting.ImpactRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.RecheckImpact(r.Context(), e, id, in)
			}
		case "applyBlockImpact":
			var in reporting.ApplyImpactRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.ApplyImpact(r.Context(), e, id, in)
			}
		case "createBlock":
			var in reporting.CreateRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Create(r.Context(), e, in)
			}
		case "listBlocks":
			var in reporting.ListRequest
			in, err = listRequest(r.URL.Query())
			if err == nil {
				out, err = service.List(r.Context(), e, in)
			}
		case "captureBlock":
			var in reporting.CaptureRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.CaptureQuery(r.Context(), e, in)
			}
		case "assessBlockQuestions":
			var in reporting.QuestionRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.AssessQuestions(r.Context(), e, in)
			}
		case "readBlock":
			var in reporting.Reference
			in, err = reference(r.URL.Query())
			if err == nil {
				out, err = service.Read(r.Context(), e, id, in)
			}
		case "readBlockSQL":
			var in reporting.Reference
			in, err = reference(r.URL.Query())
			if err == nil {
				out, err = service.SQL(r.Context(), e, id, in)
			}
		case "blockHistory":
			out, err = service.History(r.Context(), e, id)
		case "editBlock":
			var in reporting.EditRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Edit(r.Context(), e, id, in)
			}
		case "validateBlock":
			var in reporting.ValidateRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Validate(r.Context(), e, id, in)
			}
		case "previewBlock":
			var in reporting.PreviewRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Preview(r.Context(), e, id, in)
			}
		case "publishBlock":
			var in reporting.PublishRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Publish(r.Context(), e, id, in)
			}
		case "certifyBlock":
			var in reporting.CertifyRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Certify(r.Context(), e, id, in)
			}
		case "withdrawBlockCertification":
			var in reporting.WithdrawRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Withdraw(r.Context(), e, id, in)
			}
		case "rejectBlock":
			var in reporting.TransitionRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Reject(r.Context(), e, id, in)
			}
		case "restoreBlock":
			var in reporting.RestoreRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Restore(r.Context(), e, id, in)
			}
		case "archiveBlock":
			var in reporting.TransitionRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Archive(r.Context(), e, id, in)
			}
		case "resolveBlockParameters":
			var in reporting.ResolveRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Resolve(r.Context(), e, id, in)
			}

		default:
			err = reporting.ErrInvalid
		}
		if err != nil {
			failure(w, err)
			return
		}
		if err = r.Context().Err(); err != nil {
			failure(w, err)
			return
		}
		raw, err := json.Marshal(out)
		if err != nil {
			failure(w, err)
			return
		}
		if len(raw) > 16<<20 {
			failure(w, exec.ErrLimit)
			return
		}
		_, _ = w.Write(append(raw, '\n'))
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, found := registry.Match(r.Method, r.URL.Path)
		if !found {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
func validQuery(q url.Values, declared []api.Parameter) bool {
	if len(q) > len(declared) {
		return false
	}
	for key, values := range q {
		if len(values) != 1 {
			return false
		}
		found := false
		for _, p := range declared {
			if key == p.Name {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func reference(q url.Values) (reporting.Reference, error) {
	var out reporting.Reference
	if v, ok := q["revision"]; ok {
		n, err := strconv.ParseInt(v[0], 10, 64)
		if err != nil || n < 1 || n > 256 || strconv.FormatInt(n, 10) != v[0] {
			return out, reporting.ErrInvalid
		}
		out.Revision = n
	}
	if v, ok := q["draft"]; ok {
		if v[0] != "true" && v[0] != "false" {
			return out, reporting.ErrInvalid
		}
		out.Draft = v[0] == "true"
	}
	if out.Draft && out.Revision != 0 {
		return out, reporting.ErrInvalid
	}
	return out, nil
}
func listRequest(q url.Values) (reporting.ListRequest, error) {
	out := reporting.ListRequest{After: q.Get("after"), Limit: 20}
	if v, ok := q["limit"]; ok {
		n, err := strconv.Atoi(v[0])
		if err != nil || n < 1 || n > 100 || strconv.Itoa(n) != v[0] {
			return out, reporting.ErrInvalid
		}
		out.Limit = n
	}
	if v, ok := q["include_drafts"]; ok {
		if v[0] != "true" && v[0] != "false" {
			return out, reporting.ErrInvalid
		}
		out.IncludeDrafts = v[0] == "true"
	}
	return out, nil
}
func decode(w http.ResponseWriter, r *http.Request, d api.Definition, out any) error {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) != 0 {
		return reporting.ErrInvalid
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.MaxBodyBytes)))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return exec.ErrLimit
		}
		return reporting.ErrInvalid
	}
	if d.Request == nil || d.Request.Validate(raw, d.MaxBodyBytes) != nil || json.Unmarshal(raw, out) != nil {
		return reporting.ErrInvalid
	}
	return nil
}
func headers(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
func httpFault(err error) (int, string) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status, code = 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden), errors.Is(err, nlqexec.ErrInspectionRequired):
		status, code = 403, "forbidden"
	case errors.Is(err, access.ErrNotFound), errors.Is(err, store.ErrNotFound), errors.Is(err, nlqexec.ErrForeignSession):
		status, code = 404, "not_found"
	case errors.Is(err, semantics.ErrInvalid), errors.Is(err, engineering.ErrInvalid), errors.Is(err, jobs.ErrInvalid), errors.Is(err, reporting.ErrInvalid), errors.Is(err, store.ErrInvalid), errors.Is(err, nlqexec.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, engineering.ErrProposalReview), errors.Is(err, engineering.ErrProposalConflict), errors.Is(err, engineering.ErrCompensationBlocked), errors.Is(err, engineering.ErrState), errors.Is(err, store.ErrConflict), errors.Is(err, nlqexec.ErrNoPlan):
		status, code = 409, "conflict"
	case errors.Is(err, engineering.ErrProposalDrift), errors.Is(err, reporting.ErrStale), errors.Is(err, exec.ErrBinding):
		status, code = 409, "stale_validation"
	case errors.Is(err, reporting.ErrExpired):
		status, code = 410, "expired"
	case errors.Is(err, reporting.ErrIncomplete), errors.Is(err, exec.ErrUncertain):
		status, code = 409, "incomplete"
	case errors.Is(err, reporting.ErrBudget), errors.Is(err, engineering.ErrLimit), errors.Is(err, exec.ErrLimit):
		status, code = 413, "limit_exceeded"
	case errors.Is(err, exec.ErrQuery):
		status, code = 422, "invalid_query"
	case errors.Is(err, reporting.ErrBusy):
		status, code = 429, "busy"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code = 504, "cancelled_or_timed_out"
	}
	return status, code
}

func failure(w http.ResponseWriter, err error) {
	status, code := httpFault(err)
	headers(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
