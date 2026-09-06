package exec

// Capabilities describes actual adapter guarantees, not requested permission.
// PlannerCost is an estimate gate; ScanByteCeiling=false forbids promising a
// warehouse scan-byte budget based on client row/response-byte caps.
type Capabilities struct {
	Cancellation    string `json:"cancellation"`
	Reconciliation  string `json:"reconciliation"`
	ServerDeadline  bool   `json:"server_deadline"`
	PlannerCost     string `json:"planner_cost"`
	ScanByteCeiling bool   `json:"scan_byte_ceiling"`
	ResultRetention bool   `json:"result_retention"`
}
