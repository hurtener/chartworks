// Package topicapi is the thin shared-registry HTTP consumer for topic authoring
// and publication lifecycle operations.
package topicapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// MaxBodyBytes bounds authoring and lifecycle request bodies.
const MaxBodyBytes = 2 << 20

// RevisionRequest selects one exact private draft revision.
type RevisionRequest struct {
	Revision int64 `json:"revision"`
}

// HistoryRequest selects a bounded descending draft history page.
type HistoryRequest struct {
	Before int64 `json:"before"`
	Limit  int   `json:"limit"`
}

// DiffRequest selects two exact revisions for comparison.
type DiffRequest struct {
	Before int64 `json:"before"`
	After  int64 `json:"after"`
}

// ExportRequest supplies exact logical slots for neutral export.
type ExportRequest struct {
	Revision int64                          `json:"revision"`
	Mapping  []semantics.ExportDatasetSlots `json:"mapping"`
}

// PublishedVersionRequest selects one retained published version.
type PublishedVersionRequest struct {
	Version string `json:"version"`
}

// ArchiveRequest CAS-fences a topic archive transition.
type ArchiveRequest struct {
	Expected int64  `json:"expected_revision"`
	Note     string `json:"note"`
}

// RuleVersionRequest selects one retained published rules version.
type RuleVersionRequest struct {
	Version string `json:"version"`
}

// Registry returns the concrete topic operation definitions.
func Registry() (*api.Registry, error) {
	type route struct {
		method, path, id, action, effect, owner, audit string
		request, response                              reflect.Type
	}
	routes := []route{
		{"POST", "/v1/topic-drafts", "saveTopicDraft", "topics.write", "source_catalog_read_and_draft_commit", "drafts.Service.Save", "topic.drafted", reflect.TypeFor[drafts.SaveRequest](), reflect.TypeFor[drafts.Version]()},
		{"POST", "/v1/topic-draft-imports", "importTopicDraft", "topics.write", "source_catalog_read_and_draft_commit", "drafts.Service.Import", "topic.drafted", reflect.TypeFor[drafts.ImportRequest](), reflect.TypeFor[drafts.Version]()},
		{"POST", "/v1/topic-onboarding", "onboardTopicProfile", "topics.write", "profile_catalog_read_and_draft_commit", "drafts.Service.OnboardProfile", "topic.drafted", reflect.TypeFor[drafts.OnboardRequest](), reflect.TypeFor[drafts.Version]()},
		{"GET", "/v1/topics/{id}/draft", "getTopicDraft", "topics.read", "metadata_read", "drafts.Service.Read", "read_only_no_domain_audit", nil, reflect.TypeFor[drafts.Version]()},
		{"POST", "/v1/topics/{id}/draft-entities", "mutateTopicEntities", "topics.write", "source_catalog_read_and_draft_commit", "drafts.Service.MutateEntities", "topic.drafted", reflect.TypeFor[drafts.EntityMutationRequest](), reflect.TypeFor[drafts.Version]()},
		{"POST", "/v1/topics/{id}/draft-rebind", "rebindTopicDataset", "topics.write", "profile_catalog_read_and_draft_commit", "drafts.Service.RebindDataset", "topic.drafted", reflect.TypeFor[drafts.RebindRequest](), reflect.TypeFor[drafts.Version]()},
		{"POST", "/v1/topics/{id}/draft-enhancements", "enhanceTopicDraft", "topics.write", "bounded_gateway_and_draft_checkpoint", "drafts.Service.Enhance", "topic.drafted", reflect.TypeFor[drafts.EnhanceRequest](), reflect.TypeFor[drafts.EnhanceResult]()},
		{"POST", "/v1/topics/{id}/draft-versions/read", "getTopicDraftVersion", "topics.read", "metadata_read", "drafts.Service.Read", "read_only_no_domain_audit", reflect.TypeFor[RevisionRequest](), reflect.TypeFor[drafts.Version]()},
		{"POST", "/v1/topics/{id}/draft-history", "getTopicDraftHistory", "topics.read", "metadata_read", "drafts.Service.History", "read_only_no_domain_audit", reflect.TypeFor[HistoryRequest](), reflect.TypeFor[[]drafts.Revision]()},
		{"POST", "/v1/topics/{id}/draft-diff", "diffTopicDraft", "topics.read", "metadata_read", "drafts.Service.Diff", "read_only_no_domain_audit", reflect.TypeFor[DiffRequest](), reflect.TypeFor[semantics.VersionDiff]()},
		{"POST", "/v1/topics/{id}/draft-export", "exportTopicDraft", "topics.export", "metadata_export", "drafts.Service.Export", "read_only_no_domain_audit", reflect.TypeFor[ExportRequest](), reflect.TypeFor[semantics.PortablePack]()},
		{"POST", "/v1/topics/{id}/reviews", "reviewTopic", "topics.review", "review_receipt_commit", "topics.Service.Review", "topic.reviewed", reflect.TypeFor[topics.ReviewRequest](), reflect.TypeFor[topics.Review]()},
		{"POST", "/v1/topics/{id}/publications", "publishTopic", "topics.publish", "gateway_and_atomic_publication", "topics.Service.Publish", "topic.published", reflect.TypeFor[topics.PublishRequest](), reflect.TypeFor[topics.Published]()},
		{"GET", "/v1/topics/{id}/published", "getPublishedTopic", "topics.read", "retained_metadata_read", "topics.Service.Read", "read_only_no_domain_audit", nil, reflect.TypeFor[topics.Published]()},
		{"POST", "/v1/topics/{id}/published-versions/read", "getPublishedTopicVersion", "topics.read", "retained_metadata_read", "topics.Service.Read", "read_only_no_domain_audit", reflect.TypeFor[PublishedVersionRequest](), reflect.TypeFor[topics.Published]()},
		{"GET", "/v1/topics/{id}/contract", "getTopicContract", "topics.read", "source_catalog_read", "topics.Service.Contract", "read_only_no_domain_audit", nil, reflect.TypeFor[topics.Contract]()},
		{"GET", "/v1/topics/{id}/health", "getTopicHealth", "topics.read", "retained_metadata_read", "topics.Service.Health", "read_only_no_domain_audit", nil, reflect.TypeFor[topics.Health]()},
		{"POST", "/v1/topics/{id}/recheck", "recheckTopicHealth", "topics.read", "source_catalog_read_and_health_commit", "topics.Service.Recheck", "topic.health_rechecked", reflect.TypeFor[struct{}](), reflect.TypeFor[topics.Health]()},
		{"POST", "/v1/topics/{id}/rollbacks", "rollbackTopic", "topics.publish", "atomic_publication_rollback", "topics.Service.Rollback", "topic.rolled_back", reflect.TypeFor[topics.TransitionRequest](), reflect.TypeFor[topics.Published]()},
		{"POST", "/v1/topics/{id}/archive", "archiveTopic", "topics.publish", "atomic_publication_archive", "topics.Service.Archive", "topic.archived", reflect.TypeFor[ArchiveRequest](), reflect.TypeFor[topics.State]()},
		{"POST", "/v1/topics/{id}/rule-drafts", "saveRuleDraft", "topics.write", "rule_draft_commit", "rulesets.Service.Save", "rules.drafted", reflect.TypeFor[rulesets.SaveRequest](), reflect.TypeFor[rulesets.Draft]()},
		{"POST", "/v1/topics/{id}/rule-reviews", "reviewRules", "topics.review", "rule_review_receipt_commit", "rulesets.Service.Review", "rules.reviewed", reflect.TypeFor[rulesets.ReviewRequest](), reflect.TypeFor[rulesets.Review]()},
		{"POST", "/v1/topics/{id}/rule-publications", "publishRules", "topics.publish", "atomic_rule_publication", "rulesets.Service.Publish", "rules.published", reflect.TypeFor[rulesets.PublishRequest](), reflect.TypeFor[rulesets.Published]()},
		{"GET", "/v1/topics/{id}/rules", "getPublishedRules", "topics.read", "retained_metadata_read", "rulesets.Service.Read", "read_only_no_domain_audit", nil, reflect.TypeFor[rulesets.Published]()},
		{"POST", "/v1/topics/{id}/rule-versions/read", "getPublishedRuleVersion", "topics.read", "retained_metadata_read", "rulesets.Service.Read", "read_only_no_domain_audit", reflect.TypeFor[RuleVersionRequest](), reflect.TypeFor[rulesets.Published]()},
		{"POST", "/v1/topics/{id}/rules/evaluate", "evaluateRules", "topics.read", "deterministic_constraint_read", "rulesets.Service.Evaluate", "read_only_no_domain_audit", reflect.TypeFor[rulesets.EvaluateRequest](), reflect.TypeFor[rulesets.Evaluation]()},
		{"POST", "/v1/topics/{id}/rules/retire", "retireRules", "topics.publish", "atomic_rule_retirement", "rulesets.Service.Retire", "rules.retired", reflect.TypeFor[rulesets.RetireRequest](), reflect.TypeFor[rulesets.State]()},
	}
	defs := make([]api.Definition, 0, len(routes))
	for _, r := range routes {
		response, err := api.SchemaFor(r.id+"Response", r.response, true)
		if err != nil {
			return nil, err
		}
		d := api.Definition{
			Operation: api.Operation{Method: r.method, Path: r.path, Action: r.action, Effect: r.effect}, ID: r.id,
			Summary: map[string]string{
				"saveTopicDraft":           "Create or edit an immutable private topic draft",
				"importTopicDraft":         "Map and admit a portable topic draft",
				"onboardTopicProfile":      "Create an unresolved topic draft from active profile evidence",
				"getTopicDraft":            "Read the current private draft",
				"mutateTopicEntities":      "Apply atomic entity CRUD to a new private draft revision",
				"rebindTopicDataset":       "Move a dataset to active profile evidence and rewrite references",
				"enhanceTopicDraft":        "Advance one bounded resumable semantic generation step",
				"getTopicDraftVersion":     "Read an exact private draft revision",
				"getTopicDraftHistory":     "List scoped private draft revision metadata",
				"diffTopicDraft":           "Compare two exact private draft revisions",
				"exportTopicDraft":         "Export a scoped draft through logical binding slots",
				"reviewTopic":              "Record an immutable review of an exact draft digest",
				"publishTopic":             "Publish a reviewed topic with all matching facet generations",
				"getPublishedTopic":        "Read the retained active published topic",
				"getPublishedTopicVersion": "Read an exact retained published topic version",
				"getTopicContract":         "Read a published topic after current source validation",
				"getTopicHealth":           "Read the retained current-source health observation",
				"recheckTopicHealth":       "Recheck public source continuity and commit a complete observation",
				"rollbackTopic":            "Restore an exact retained topic version and facet set",
				"archiveTopic":             "Archive the active topic and every matching facet head",
				"saveRuleDraft":            "Create or edit an immutable proposed ruleset",
				"reviewRules":              "Record an immutable review of an exact ruleset draft",
				"publishRules":             "Activate an approved ruleset for the current topic version",
				"getPublishedRules":        "Read the active published ruleset",
				"getPublishedRuleVersion":  "Read an exact retained ruleset version",
				"evaluateRules":            "Evaluate active hard constraints over explicit semantic references",
				"retireRules":              "Retire the active ruleset with revision CAS",
			}[r.id],
			ResourceLoader: r.owner, Audit: r.audit, Response: response,
			Errors: []api.ErrorResponse{
				{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"},
				{Status: 401, Code: "unauthorized"}, {Status: 403, Code: "forbidden"},
				{Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"},
				{Status: 409, Code: "context_changed"}, {Status: 413, Code: "limit_exceeded"},
				{Status: 422, Code: "unsupported"}, {Status: 503, Code: "unavailable"},
				{Status: 504, Code: "cancelled_or_timed_out"},
			},
		}
		if r.request != nil {
			d.Request, err = api.SchemaFor(r.id+"Request", r.request, false, api.OptionalJSONFields)
			if err != nil {
				return nil, err
			}
			d.MaxBodyBytes = MaxBodyBytes
		}
		defs = append(defs, d)
	}
	return api.New(defs)
}

// Handler serves the registered topic routes through shared verification.
func Handler(verifier *auth.Verifier, service *drafts.Service, published *topics.Service, rules *rulesets.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil && published == nil && rules == nil {
		return next
	}
	registry, err := Registry()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { failure(w, err) })
	}
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		selected, id, _ := registry.Match(r.Method, r.URL.Path)
		if selected.Path == "" {
			w.WriteHeader(405)
			return
		}
		if !e.Has(selected.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" || r.Method == "GET" && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
			failure(w, store.ErrInvalid)
			return
		}
		if service == nil {
			switch selected.ID {
			case "saveTopicDraft", "importTopicDraft", "onboardTopicProfile", "getTopicDraft", "getTopicDraftVersion", "getTopicDraftHistory", "diffTopicDraft", "exportTopicDraft", "mutateTopicEntities", "rebindTopicDataset", "enhanceTopicDraft":
				failure(w, store.ErrNotFound)
				return
			}
		}
		if rules == nil {
			switch selected.ID {
			case "saveRuleDraft", "reviewRules", "publishRules", "getPublishedRules", "getPublishedRuleVersion", "evaluateRules", "retireRules":
				failure(w, store.ErrNotFound)
				return
			}
		}
		decode := func(out any) error { return body(w, r, selected.Request, out) }
		var out any
		switch selected.ID {
		case "saveTopicDraft":
			var in drafts.SaveRequest
			if err = decode(&in); err == nil {
				out, err = service.Save(r.Context(), e, in)
			}
		case "importTopicDraft":
			var in drafts.ImportRequest
			if err = decode(&in); err == nil {
				out, err = service.Import(r.Context(), e, in)
			}
		case "onboardTopicProfile":
			var in drafts.OnboardRequest
			if err = decode(&in); err == nil {
				out, err = service.OnboardProfile(r.Context(), e, in)
			}
		case "getTopicDraft":
			out, err = service.Read(r.Context(), e, id, 0)
		case "mutateTopicEntities":
			var in drafts.EntityMutationRequest
			if err = decode(&in); err == nil {
				out, err = service.MutateEntities(r.Context(), e, id, in)
			}
		case "rebindTopicDataset":
			var in drafts.RebindRequest
			if err = decode(&in); err == nil {
				out, err = service.RebindDataset(r.Context(), e, id, in)
			}
		case "enhanceTopicDraft":
			var in drafts.EnhanceRequest
			if err = decode(&in); err == nil {
				out, err = service.Enhance(r.Context(), e, id, in)
			}
		case "getTopicDraftVersion":
			var in RevisionRequest
			if err = decode(&in); err == nil {
				if in.Revision < 1 {
					err = store.ErrInvalid
				} else {
					out, err = service.Read(r.Context(), e, id, in.Revision)
				}
			}
		case "getTopicDraftHistory":
			var in HistoryRequest
			if err = decode(&in); err == nil {
				out, err = service.History(r.Context(), e, id, in.Before, in.Limit)
			}
		case "diffTopicDraft":
			var in DiffRequest
			if err = decode(&in); err == nil {
				out, err = service.Diff(r.Context(), e, id, in.Before, in.After)
			}
		case "exportTopicDraft":
			var in ExportRequest
			if err = decode(&in); err == nil {
				out, err = service.Export(r.Context(), e, id, in.Revision, in.Mapping)
			}
		case "reviewTopic":
			if published == nil {
				err = store.ErrNotFound
			} else {
				var in topics.ReviewRequest
				if err = decode(&in); err == nil {
					out, err = published.Review(r.Context(), e, id, in)
				}
			}
		case "publishTopic":
			if published == nil {
				err = store.ErrNotFound
			} else {
				var in topics.PublishRequest
				if err = decode(&in); err == nil {
					out, err = published.Publish(r.Context(), e, id, in)
				}
			}
		case "getPublishedTopic":
			if published == nil {
				err = store.ErrNotFound
			} else {
				out, err = published.Read(r.Context(), e, id, "")
			}
		case "getPublishedTopicVersion":
			if published == nil {
				err = store.ErrNotFound
			} else {
				var in PublishedVersionRequest
				if err = decode(&in); err == nil {
					out, err = published.Read(r.Context(), e, id, in.Version)
				}
			}
		case "getTopicContract":
			if published == nil {
				err = store.ErrNotFound
			} else {
				out, err = published.Contract(r.Context(), e, id)
			}
		case "getTopicHealth":
			if published == nil {
				err = store.ErrNotFound
			} else {
				out, err = published.Health(r.Context(), e, id)
			}
		case "recheckTopicHealth":
			if published == nil {
				err = store.ErrNotFound
			} else {
				var in struct{}
				if err = decode(&in); err == nil {
					out, err = published.Recheck(r.Context(), e, id)
				}
			}
		case "rollbackTopic":
			if published == nil {
				err = store.ErrNotFound
			} else {
				var in topics.TransitionRequest
				if err = decode(&in); err == nil {
					out, err = published.Rollback(r.Context(), e, id, in)
				}
			}
		case "archiveTopic":
			if published == nil {
				err = store.ErrNotFound
			} else {
				var in ArchiveRequest
				if err = decode(&in); err == nil {
					out, err = published.Archive(r.Context(), e, id, in.Expected, in.Note)
				}
			}
		case "saveRuleDraft":
			var in rulesets.SaveRequest
			if err = decode(&in); err == nil {
				if in.Definition.Topic != id {
					err = store.ErrInvalid
				} else {
					out, err = rules.Save(r.Context(), e, in)
				}
			}
		case "reviewRules":
			var in rulesets.ReviewRequest
			if err = decode(&in); err == nil {
				out, err = rules.Review(r.Context(), e, id, in)
			}
		case "publishRules":
			var in rulesets.PublishRequest
			if err = decode(&in); err == nil {
				out, err = rules.Publish(r.Context(), e, id, in)
			}
		case "getPublishedRules":
			out, err = rules.Read(r.Context(), e, id, "")
		case "getPublishedRuleVersion":
			var in RuleVersionRequest
			if err = decode(&in); err == nil {
				out, err = rules.Read(r.Context(), e, id, in.Version)
			}
		case "evaluateRules":
			var in rulesets.EvaluateRequest
			if err = decode(&in); err == nil {
				out, err = rules.Evaluate(r.Context(), e, id, in)
			}
		case "retireRules":
			var in rulesets.RetireRequest
			if err = decode(&in); err == nil {
				out, err = rules.Retire(r.Context(), e, id, in)
			}
		}
		if err != nil {
			failure(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, known := registry.Match(r.Method, r.URL.Path); known {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			protected.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func body(w http.ResponseWriter, r *http.Request, schema *gateway.Schema, out any) error {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) != 0 {
		return store.ErrInvalid
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return readexec.ErrLimit
		}
		return store.ErrInvalid
	}
	if schema.Validate(b, MaxBodyBytes) != nil || json.Unmarshal(b, out) != nil {
		return store.ErrInvalid
	}
	return nil
}
func failure(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status, code = 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, access.ErrNotFound), errors.Is(err, store.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, store.ErrInvalid), errors.Is(err, semantics.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, store.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(err, readexec.ErrBinding):
		status, code = 409, "context_changed"
	case errors.Is(err, readexec.ErrLimit):
		status, code = 413, "limit_exceeded"
	case errors.Is(err, readexec.ErrUnsupported):
		status, code = 422, "unsupported"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code = 504, "cancelled_or_timed_out"
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
