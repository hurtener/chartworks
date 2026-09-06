from pathlib import Path

def replace(path,old,new):
 p=Path(path);s=p.read_text();assert old in s,(path,old[:80]);p.write_text(s.replace(old,new,1))

replace('internal/exec/execution.go','type ExecutionAdapter interface {','type ExecutionAdapter interface {\n ReadCapabilities() Capabilities')
replace('internal/exec/execution.go','type ExecutionReport struct {','type ExecutionReport struct {\n Capabilities Capabilities `json:"capabilities"`')
replace('internal/exec/execution.go','report := ExecutionReport{Attempt: current}','report := ExecutionReport{Attempt: current,Capabilities:x.adapter.ReadCapabilities()}')
replace('internal/exec/execution.go','if err != nil {\n\t\t\tstate = "unknown"\n\t\t}', 'if err != nil {\n state="unknown"\n if errors.Is(err,ErrUnsupported) { state="unsupported" }\n }')
p=Path('internal/sources/read.go');s=p.read_text();s+='''
// ReadCapabilities reports the qualified PostgreSQL implementation only.
func (s *Service) ReadCapabilities() readexec.Capabilities {
 return readexec.Capabilities{Cancellation:"owner_cancel_request",Reconciliation:"tagged_backend_observation",ServerDeadline:true,PlannerCost:"optimizer_estimate",ScanByteCeiling:false,ResultRetention:false}
}
''';p.write_text(s)
p=Path('sdk/chartworks/execution.go');s=p.read_text();s+='''
// ReadCapabilities separates implemented cancellation and cost gates from unknown guarantees.
type ReadCapabilities struct {
 Cancellation string `json:"cancellation"`
 Reconciliation string `json:"reconciliation"`
 ServerDeadline bool `json:"server_deadline"`
 PlannerCost string `json:"planner_cost"`
 ScanByteCeiling bool `json:"scan_byte_ceiling"`
 ResultRetention bool `json:"result_retention"`
}
''';s=s.replace('type ReadExecutionReport struct {','type ReadExecutionReport struct {\n Capabilities ReadCapabilities `json:"capabilities"`',1);p.write_text(s)
