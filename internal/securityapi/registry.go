package securityapi

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/sdk/chartworks"
)

const securityRequestMaxBytes = 4096

// RetentionPolicyRequest is the closed body accepted by the retention policy
// update operation.
type RetentionPolicyRequest struct {
	Expected       int64 `json:"expected_revision"`
	AuditDays      int   `json:"audit_days"`
	OperationHours int   `json:"operation_hours"`
}

// EmptyRequest is the explicit empty JSON object used by maintenance actions.
type EmptyRequest struct{}

// DiagnosticCheck is the safe permission result returned by the diagnostics
// operation; it contains no bearer, identity or private resource data.
type DiagnosticCheck struct {
	Operation Operation `json:"operation"`
	Allowed   bool      `json:"allowed"`
}

func securityErrors() []api.ErrorResponse {
	return []api.ErrorResponse{
		{Status: 401, Code: "unauthorized"},
		{Status: 401, Code: "unauthenticated"},
		{Status: 403, Code: "forbidden"},
		{Status: 404, Code: "not_found"},
		{Status: 409, Code: "conflict"},
		{Status: 410, Code: "expired"},
		{Status: 422, Code: "invalid_request"},
		{Status: 500, Code: "internal_error"},
		{Status: 503, Code: "unavailable"},
	}
}

// APIRegistry describes the exact operational routes served by Handler.
// Metrics is omitted when telemetry export is disabled, matching Handler.
func APIRegistry(metrics bool) (*api.Registry, error) {
	policyResponse, err := api.SchemaFor("retentionPolicyResponse", reflect.TypeFor[chartworks.Policy](), true)
	if err != nil {
		return nil, err
	}
	auditResponse, err := api.SchemaFor("auditEventsResponse", reflect.TypeFor[[]chartworks.Audit](), true)
	if err != nil {
		return nil, err
	}
	operationResponse, err := api.SchemaFor("retentionSweepResponse", reflect.TypeFor[chartworks.Operation](), true)
	if err != nil {
		return nil, err
	}
	diagnosticsResponse, err := api.SchemaFor("accessDiagnosticsResponse", reflect.TypeFor[[]DiagnosticCheck](), true)
	if err != nil {
		return nil, err
	}
	metricsResponse, err := api.SchemaFor("metricsResponse", reflect.TypeFor[string](), true)
	if err != nil {
		return nil, err
	}
	request, err := api.SchemaFor("retentionPolicyRequest", reflect.TypeFor[RetentionPolicyRequest](), false)
	if err != nil {
		return nil, err
	}
	empty, err := api.SchemaFor("emptyRequest", reflect.TypeFor[EmptyRequest](), false)
	if err != nil {
		return nil, err
	}
	errors := securityErrors()
	definitions := []api.Definition{
		{Operation: api.Operation{Method: "GET", Path: "/v1/retention-policy", Action: "ops.read", Effect: "read"}, ID: "getRetentionPolicy", Summary: "Read the current retention policy", ResourceLoader: "securityapi.Service.Policy", Audit: "read_only_no_domain_audit", Response: policyResponse, Errors: errors},
		{Operation: api.Operation{Method: "PUT", Path: "/v1/retention-policy", Action: "ops.write", Effect: "write"}, ID: "setRetentionPolicy", Summary: "Update the retention policy with revision CAS", ResourceLoader: "securityapi.Service.Configure", Audit: "retention.policy_updated", Request: request, RequestContentType: "application/json", MaxBodyBytes: securityRequestMaxBytes, Response: policyResponse, Errors: errors},
		{Operation: api.Operation{Method: "GET", Path: "/v1/audit-events", Action: "ops.audit", Effect: "read"}, ID: "listAuditEvents", Summary: "Read bounded tenant audit metadata", ResourceLoader: "securityapi.Service.Audits", Audit: "read_only_no_domain_audit", Query: []api.Parameter{{Name: "limit", In: "query", Description: "Maximum number of audit records", Type: "integer", Required: false, Min: 1, Max: 1000}}, Response: auditResponse, Errors: errors},
		{Operation: api.Operation{Method: "POST", Path: "/v1/retention-sweeps", Action: "ops.maintain", Effect: "erase"}, ID: "sweepRetention", Summary: "Admit one bounded retention sweep", ResourceLoader: "securityapi.Service.Sweep", Audit: "retention.sweep_requested", Headers: []api.Parameter{{Name: "Idempotency-Key", In: "header", Description: "Stable key for the logical sweep request", Type: "string", Required: true, Max: 128, Pattern: "^[A-Za-z0-9_.:-]+$"}}, Request: empty, RequestContentType: "application/json", MaxBodyBytes: securityRequestMaxBytes, Response: operationResponse, Errors: errors},
		{Operation: api.Operation{Method: "GET", Path: "/v1/access/diagnostics", Action: "ops.inspect", Effect: "read"}, ID: "accessDiagnostics", Summary: "Read current operation permission diagnostics", ResourceLoader: "access.Require", Audit: "read_only_no_domain_audit", Response: diagnosticsResponse, Errors: errors},
	}
	if metrics {
		definitions = append(definitions, api.Definition{Operation: api.Operation{Method: "GET", Path: "/metrics", Action: "ops.metrics", Effect: "read"}, ID: "metrics", Summary: "Read operator-scoped Prometheus metrics", ResourceLoader: "telemetry.Reporter.Handler", Audit: "read_only_no_domain_audit", ResponseContentType: "text/plain; version=0.0.4", Response: metricsResponse, Errors: errors})
	}
	return api.New(definitions)
}
