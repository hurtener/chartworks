//nolint:revive // Public registration names mirror stable HTTP operation identifiers.
package migrationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/store"
)

const MaxBodyBytes = 10 << 20

func Registry() (*api.Registry, error) {
	type row struct {
		path, action, effect, id, summary string
		in, out                           reflect.Type
	}
	rows := []row{
		{"/v1/migrations/dry-runs", "migration.read", "migration_dry_run", "migrationDryRun", "Validate a neutral cohort manifest and produce an exhaustive loss and dependency plan", reflect.TypeFor[migration.DryRunRequest](), reflect.TypeFor[migration.Plan]()},
		{"/v1/migrations/imports", "migration.write", "migration_import_commit", "migrationImport", "Import or idempotently resume a neutral cohort through public domain seams", reflect.TypeFor[migration.ImportRequest](), reflect.TypeFor[migration.Batch]()},
		{"/v1/migrations/resume", "migration.write", "migration_import_commit", "migrationResume", "Resume a fenced migration batch from its durable checkpoint", reflect.TypeFor[migration.ResumeRequest](), reflect.TypeFor[migration.Batch]()},
		{"/v1/migrations/exports", "migration.read", "migration_export_read", "migrationExport", "Export a bounded neutral page without credentials or current authority claims", reflect.TypeFor[migration.ExportRequest](), reflect.TypeFor[migration.Export]()},
		{"/v1/migrations/cutovers", "migration.cutover", "migration_cutover_commit", "migrationCutover", "Activate one cohort route at an exact schedule occurrence boundary", reflect.TypeFor[migration.CutoverRequest](), reflect.TypeFor[migration.Cutover]()},
		{"/v1/migrations/rollbacks", "migration.cutover", "migration_cutover_commit", "migrationRollback", "Restore the prior cohort route and record irreversible external effects", reflect.TypeFor[migration.RollbackRequest](), reflect.TypeFor[migration.Cutover]()},
		{"/v1/migrations/erasures", "migration.erase", "migration_erase_commit", "migrationErase", "Erase bounded online migration records under current signed authority", reflect.TypeFor[migration.EraseRequest](), reflect.TypeFor[migration.EraseResult]()},
	}
	defs := make([]api.Definition, 0, len(rows))
	for _, r := range rows {
		req, err := api.SchemaFor(r.id+"Request", r.in, false, api.NullableCollections)
		if err != nil {
			return nil, fmt.Errorf("migrationapi: request schema %s: %w", r.id, err)
		}
		resp, err := api.SchemaFor(r.id+"Response", r.out, true)
		if err != nil {
			return nil, fmt.Errorf("migrationapi: response schema %s: %w", r.id, err)
		}
		defs = append(defs, api.Definition{Operation: api.Operation{Method: http.MethodPost, Path: r.path, Action: r.action, Effect: r.effect}, ID: r.id, Summary: r.summary, ResourceLoader: "migration.Service: verified tenant plus exact batch or cohort", Audit: "migration lifecycle and cutover event", MaxBodyBytes: MaxBodyBytes, Request: req, Response: resp, Errors: []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"}, {Status: 413, Code: "limit_exceeded"}, {Status: 422, Code: "unsupported_or_not_ready"}, {Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"}}})
	}
	return api.New(defs)
}

func Handler(verifier *auth.Verifier, service *migration.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry, err := Registry()
	if err != nil {
		return next
	}
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, _, ok := registry.Match(r.Method, r.URL.Path)
		if !ok {
			failure(w, migration.ErrInvalid)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if err = access.Require(e, d.Action, access.Resource{Tenant: e.Tenant(), Kind: "tenant", Permission: permission(d.Action), ID: e.Tenant()}); err != nil {
			failure(w, err)
			return
		}
		var out any
		switch d.ID {
		case "migrationDryRun":
			var in migration.DryRunRequest
			err = decode(w, r, d, &in)
			if err == nil {
				out, err = service.DryRun(r.Context(), e, in)
			}
		case "migrationImport":
			var in migration.ImportRequest
			err = decode(w, r, d, &in)
			if err == nil {
				out, err = service.Import(r.Context(), e, in)
			}
		case "migrationResume":
			var in migration.ResumeRequest
			err = decode(w, r, d, &in)
			if err == nil {
				out, err = service.Resume(r.Context(), e, in)
			}
		case "migrationExport":
			var in migration.ExportRequest
			err = decode(w, r, d, &in)
			if err == nil {
				out, err = service.Export(r.Context(), e, in)
			}
		case "migrationCutover":
			var in migration.CutoverRequest
			err = decode(w, r, d, &in)
			if err == nil {
				out, err = service.Cutover(r.Context(), e, in)
			}
		case "migrationRollback":
			var in migration.RollbackRequest
			err = decode(w, r, d, &in)
			if err == nil {
				out, err = service.Rollback(r.Context(), e, in)
			}
		case "migrationErase":
			var in migration.EraseRequest
			err = decode(w, r, d, &in)
			if err == nil {
				out, err = service.Erase(r.Context(), e, in)
			}
		default:
			err = migration.ErrInvalid
		}
		if err != nil {
			failure(w, err)
			return
		}
		headers(w)
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := registry.Match(r.Method, r.URL.Path); !ok {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func permission(action string) string {
	switch action {
	case "migration.write":
		return "write"
	case "migration.cutover":
		return "write"
	case "migration.erase":
		return "erase"
	default:
		return "read"
	}
}
func decode(w http.ResponseWriter, r *http.Request, d api.Definition, out any) error {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) != 0 {
		return migration.ErrInvalid
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.MaxBodyBytes)))
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return migration.ErrLimit
		}
		return migration.ErrInvalid
	}
	if d.Request.Validate(raw, d.MaxBodyBytes) != nil || json.Unmarshal(raw, out) != nil {
		return migration.ErrInvalid
	}
	return nil
}
func headers(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func failure(w http.ResponseWriter, err error) {
	status, code := classify(err)
	headers(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
func classify(err error) (int, string) {
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		return 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		return 403, "forbidden"
	case errors.Is(err, migration.ErrNotFound), errors.Is(err, store.ErrNotFound), errors.Is(err, access.ErrNotFound):
		return 404, "not_found"
	case errors.Is(err, migration.ErrConflict), errors.Is(err, store.ErrConflict):
		return 409, "conflict"
	case errors.Is(err, migration.ErrLimit):
		return 413, "limit_exceeded"
	case errors.Is(err, migration.ErrUnsupported), errors.Is(err, migration.ErrNotReady):
		return 422, "unsupported_or_not_ready"
	case errors.Is(err, migration.ErrInvalid), errors.Is(err, store.ErrInvalid):
		return 400, "invalid_request"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 504, "cancelled_or_timed_out"
	default:
		return 503, "unavailable"
	}
}
