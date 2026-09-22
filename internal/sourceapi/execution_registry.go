package sourceapi

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

// ExecutionAPIRegistry supplies the existing handler, manifest and OpenAPI from one inventory.
func ExecutionAPIRegistry() (*api.Registry, error) {
	definitions := []registration{
		{operation("GET", "/v1/read-operations/{id}", "sources.query", "attempt_metadata_read"), "readOperation", "Read retained operation attempt", "exec.Executor.ByOperation", "read_only_no_domain_audit", nil, reflect.TypeFor[readexec.Attempt]()},
		{operation("POST", "/v1/sources/{id}/execute", "sources.query", "bounded_warehouse_read"), "executeRead", "Validate and execute one bounded read", "exec.Validator.Validate; exec.Executor.Execute", "read.accepted; read outcome journal", reflect.TypeFor[ExecutionRequest](), reflect.TypeFor[readexec.ExecutionReport]()},
		{operation("GET", "/v1/read-executions/{id}", "sources.query", "attempt_metadata_read"), "readExecution", "Read retained execution attempt", "exec.Executor.Inspect", "read_only_no_domain_audit", nil, reflect.TypeFor[readexec.Attempt]()},
		{operation("POST", "/v1/read-executions/{id}/cancel", "sources.query", "cancel_intent_and_backend_signal"), "cancelRead", "Record cancellation and signal owned read", "exec.Executor.Control", "read.cancel_requested; read outcome journal", reflect.TypeFor[struct{}](), reflect.TypeFor[readexec.ControlReceipt]()},
		{operation("POST", "/v1/read-executions/{id}/reconcile", "sources.query", "remote_observation_and_attempt_reconciliation"), "reconcileRead", "Observe and reconcile an uncertain read", "exec.Executor.Control", "read outcome journal", reflect.TypeFor[struct{}](), reflect.TypeFor[readexec.ControlReceipt]()},
	}
	registry, err := compileRegistrations(definitions, sourceErrors(), sourceRequestMaxBytes, false, 0)
	if err != nil {
		return nil, err
	}
	compiled := registry.Definitions()
	for i := range compiled {
		switch compiled[i].ID {
		case "readExecution":
			compiled[i].Interaction = "query_result"
		case "cancelRead":
			compiled[i].Interaction = "query_cancel"
		}
	}
	return api.New(compiled)
}
