# Apply only to the recovered feature source. All transformations are exact and
# fail on concurrent edits rather than replacing them. This stays on the dev branch.
from pathlib import Path
import json

def replace(path, old, new):
    p=Path(path);s=p.read_text()
    assert s.count(old)==1,(path,old[:80],s.count(old))
    p.write_text(s.replace(old,new))

def append(path, text):
    p=Path(path);p.write_text(p.read_text()+text)

replace('internal/reportingapi/http.go', 'func failure(w http.ResponseWriter, err error) {\n\tstatus, code', 'func classify(err error) (int, string) {\n\tstatus, code')
replace('internal/reportingapi/http.go', '\theaders(w)\n\tw.WriteHeader(status)\n\t_ = json.NewEncoder(w).Encode(struct {', '\treturn status, code\n}\n\nfunc failure(w http.ResponseWriter, err error) {\n\tstatus, code := classify(err)\n\theaders(w)\n\tw.WriteHeader(status)\n\t_ = json.NewEncoder(w).Encode(struct {')
replace('internal/reporting/runs_service.go', '"slices"', '"slices"\n"sync"')
replace('internal/reporting/runs_service.go', 'type Runs struct {', '''type Runs struct {
 life context.Context
 cancel context.CancelFunc
 mu sync.RWMutex
 closed bool''')
replace('internal/reporting/runs_service.go','return &Runs{blocks: blocks,','life, cancel := context.WithCancel(context.Background())\n\treturn &Runs{life: life, cancel: cancel, blocks: blocks,')
replace('internal/reporting/runs_service.go','in, err := normalizedRunRequest(input)','ctx, stop, err := s.beginWork(ctx)\n\tif err != nil { return RunView{}, err }\n\tdefer stop()\n\tin, err := normalizedRunRequest(input)')
replace('internal/reporting/runs_execution.go','\tr, err := s.repo.ReadFrozenRun(ctx, e, id, true)','\tctx, stop, err := s.beginWork(ctx)\n\tif err != nil { return RunView{}, err }\n\tdefer stop()\n\tr, err := s.repo.ReadFrozenRun(ctx, e, id, true)')
append('internal/reporting/runs_service.go','''
// CanExecute reports source execution availability independently of optional models.
func (s *Runs) CanExecute() bool { return s != nil && s.blocks.CanValidate() }

// PageRowLimit is the configured bound shared by transport and retained paging.
func (s *Runs) PageRowLimit() int { return s.limits.PageRows }

// ResponseByteLimit permits configured artifact payloads plus bounded metadata.
func (s *Runs) ResponseByteLimit() int { return s.limits.MaxArtifactBytes + (4 << 20) }

// Close cancels and joins admission/execution before shared dependencies close.
// It creates no additional worker, scheduler or model client.
func (s *Runs) Close() {
 s.cancel()
 s.mu.Lock()
 defer s.mu.Unlock()
 s.closed = true
}

func (s *Runs) beginWork(ctx context.Context) (context.Context, func(), error) {
 s.mu.RLock()
 if s.closed { s.mu.RUnlock(); return nil, nil, ErrUnavailable }
 work, cancel := context.WithCancel(ctx)
 joined := context.AfterFunc(s.life, cancel)
 return work, func() { joined(); cancel(); s.mu.RUnlock() }, nil
}
''')
append('internal/engineering/autopilot_planning.go','''
// Enabled reports configured reviewed L2 availability, never signed authority.
func (s *Autopilot) Enabled() bool { return s != nil && s.limits.Enabled }
''')
# Omitted defaults are valid intent; supplied values still obey the closed enums.
replace('internal/reporting/runs_model.go','`json:"policy" jsonschema:', '`json:"policy,omitempty" jsonschema:')
replace('internal/reporting/runs_model.go','`json:"partial_policy" jsonschema:', '`json:"partial_policy,omitempty" jsonschema:')
replace('test/acceptance/phase28_test.go','\treturn runs\n}', '\tt.Cleanup(runs.Close)\n\treturn runs\n}')

replace('internal/foundation/work.go','\tpipelines     *engineering.PipelineService','\tpipelines     *engineering.PipelineService\n\tautopilot *engineering.Autopilot\n\truns *reporting.Runs')
replace('internal/foundation/work.go','\tw.handler = sourceapi.PipelineHandler(verifier, w.pipelines, w.handler)','''	w.handler = sourceapi.PipelineHandler(verifier, w.pipelines, w.handler)
	w.autopilot, err = engineering.NewAutopilot(db, w.pipelines, v.Autopilot)
	if err != nil { w.close(); return nil, err }
	autopilotRegistry, err := sourceapi.AutopilotAPIRegistry(w.autopilot.Enabled())
	if err != nil { w.close(); return nil, err }
	w.handler = sourceapi.AutopilotHandler(verifier, w.autopilot, w.handler)''')
replace('internal/foundation/work.go','\tw.handler = reportingapi.Handler(verifier, blockService, w.handler)','''	w.handler = reportingapi.Handler(verifier, blockService, w.handler)
	runner, err := jobs.NewRequestRunner(db, jobLimits(v.Jobs))
	if err != nil { w.close(); return nil, err }
	role := v.Gateway.Roles["narrative"]
	modelVersion := role.ModelRevision
	if modelVersion == "" { modelVersion = role.Model }
	w.runs, err = reporting.NewRuns(blockService, db, runner, w.engine, modelVersion, v.Reporting.Execution)
	if err != nil { w.close(); return nil, err }
	runsRegistry, err := reportingapi.RunsRegistry(w.runs.CanExecute())
	if err != nil { w.close(); return nil, err }
	w.handler = reportingapi.RunsHandler(verifier, w.runs, w.handler)''')
replace('internal/foundation/work.go','byoRegistry, chartRegistry, blockRegistry)','byoRegistry, chartRegistry, blockRegistry, autopilotRegistry, runsRegistry)')
replace('internal/foundation/work.go','byo, chartService, w.registry, w.handler)','byo, chartService, w.registry, w.handler, w.runs)')
replace('internal/foundation/work.go','\t\tif w.engineering != nil {','\t\tif w.runs != nil { w.runs.Close() }\n\t\tif w.autopilot != nil { w.autopilot.Close() }\n\t\tif w.engineering != nil {')
replace('internal/foundation/mcp.go','"github.com/hurtener/chartworks/internal/nlqexec"','"github.com/hurtener/chartworks/internal/nlqexec"\n"github.com/hurtener/chartworks/internal/reporting"\n"github.com/hurtener/chartworks/internal/reportingapi"')
replace('internal/foundation/mcp.go','next http.Handler) (*api.Registry','next http.Handler, frozen ...*reporting.Runs) (*api.Registry')
replace('internal/foundation/mcp.go','\tselected, err := mcpserver.SelectGroups(bindings, v.MCP.Groups)','''	for _, runs := range frozen {
		group, err := reportingapi.RunMCPBindings(runs)
		if err != nil { return nil, nil, err }
		bindings = append(bindings, group...)
	}
	selected, err := mcpserver.SelectGroups(bindings, v.MCP.Groups)''')
replace('internal/mcpserver/registry.go','case "metadata_read", "retained_metadata_read", "byo_context_read":','case "metadata_read", "retained_metadata_read", "byo_context_read", "retained_artifact_read":')
replace('internal/mcpserver/registry.go','case "caller_data_transform_no_persistence":','case "reporting_run_admission":\n\t\treturn effects{persists:true, openWorld:true}, true\n\tcase "reporting_run_execution":\n\t\treturn effects{persists:true, openWorld:true, paid:true}, true\n\tcase "caller_data_transform_no_persistence":')
replace('internal/mcpserver/registry.go','s == "charts" }','s == "charts" || s == "reporting" }')
replace('internal/config/mcp.go','[]string{"discovery", "query", "byo", "charts"}','[]string{"discovery", "query", "byo", "charts", "reporting"}')
replace('internal/config/mcp.go','len(m.Groups) > 4','len(m.Groups) > 5')
replace('internal/config/mcp.go','g != "charts")','g != "charts" && g != "reporting")')

# These are implementation-in-progress labels, not a completion declaration.
p=Path('docs/plans/phase-registry.json');registry=json.loads(p.read_text())
for phase in ['26','28']:
    assert registry['phases'][phase]['status']=='planned'
    registry['phases'][phase]['status']='in_progress'
p.write_text(json.dumps(registry,indent=2)+'\n')
for name in ['phase-26-engineering-autopilot.md','phase-28-reporting-execution-artifacts.md']:
    replace('docs/plans/'+name,'Status: planned.','Status: in_progress.')
append('docs/plans/phase-21-http-api.md','''
## Phases 26/28 cumulative extension (implementation in progress)

The running composition registers reviewed L2 proposal methods and frozen-run
admission/execution, cancellation, retained summaries, result paging, outputs and
bounded retention. Each operation uses the existing verifier and a closed schema;
resource loading remains in the owning domain/store. The SDK uses the shared
caller-token transport and the CLI discovers these actual OpenAPI registrations.
Reporting MCP read/admission/execution bindings reuse these registered contracts.
This extension is not a claim that either new phase or final release is complete.
''')
