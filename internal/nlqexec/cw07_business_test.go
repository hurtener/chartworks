package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type cw07ReplayRouter struct {
	constraints []exec.BusinessConstraint
	binding     string
	err         error
}

func (*cw07ReplayRouter) Route(context.Context, identity.Envelope, nlqroute.RouteRequest) (nlqroute.RouteResult, error) {
	return nlqroute.RouteResult{}, errors.New("unexpected route")
}

func (r *cw07ReplayRouter) ReplayClarifications(context.Context, identity.Envelope, nlqroute.RouteResult) ([]exec.BusinessConstraint, string, error) {
	return append([]exec.BusinessConstraint(nil), r.constraints...), r.binding, r.err
}

type cw07SourceReader struct{ binding exec.Binding }

func (r cw07SourceReader) Binding(context.Context, identity.Envelope, string, string) (exec.Binding, error) {
	return r.binding.Clone(), nil
}

func (r *unitRepository) ReadSavedQuery(_ context.Context, _ identity.Envelope, id string, operation bool) (QueryRecord, error) {
	if !operation {
		query, ok := r.queries[id]
		if !ok {
			return QueryRecord{}, store.ErrNotFound
		}
		return query, nil
	}
	for _, query := range r.queries {
		if query.Operation == id {
			return query, nil
		}
	}
	return QueryRecord{}, store.ErrNotFound
}

func cw07ExecBinding(revision int64) exec.Binding {
	return exec.Binding{Tenant: "tenant", Source: "source", Context: "context", Revision: revision, Dialect: "postgres", Contract: "contract", Fingerprint: strings.Repeat("a", 64), Relations: []exec.Relation{{ID: "dataset", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "id", NativeType: "integer", Category: "integer", Safe: true}, {Name: "region_code", NativeType: "text", Category: "text", Safe: true}}}}}
}

func cw07InterpretationQuery(t *testing.T, value string) (QueryRecord, exec.Binding, []exec.BusinessConstraint) {
	t.Helper()
	e := unitEnvelope(t)
	binding := cw07ExecBinding(1)
	resolution := exec.Hash([]string{"reviewed", value})
	constraints := []exec.BusinessConstraint{{Resolution: resolution, Dataset: "dataset", Column: "region_code", SourceRevision: 1, Kind: "text", Operator: "eq", Nulls: "exclude", Value: value}}
	baseSQL := "SELECT id FROM analytics.sales"
	bound, err := exec.BindBusinessConstraints(context.Background(), binding, baseSQL, nil, constraints)
	if err != nil {
		t.Fatal(err)
	}
	validation := exec.Receipt{Validated: true, Source: "source", Context: "context", Dialect: "postgres", Contract: "contract", Manifest: exec.Hash("validated")}
	bound.Receipt.Validation = &validation
	q := unitQuery(e, "query-interpretation", "topic", "v1", "context", false)
	q.SQL, q.Parameters = bound.SQL, bound.Parameters
	q.Route.SourceBindingDigest = exec.Hash(binding)
	q.Route.Interpretation = &nlqroute.Interpretation{Version: "semantic-interpretation-v1", Parser: "deterministic-span-v1", Values: []nlqroute.ValueInterpretation{{ID: resolution, Topic: "topic", Dataset: "dataset", Column: "region_code", GovernedValue: "reviewed", CanonicalValue: value, Operator: "eq"}}}
	q.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: baseSQL, Binding: bound.Receipt}
	return q, binding, constraints
}

func TestCW07InterpretationBusinessEvidencePlanRunAndDrift(t *testing.T) {
	e := unitEnvelope(t)
	q, binding, constraints := cw07InterpretationQuery(t, "NORTH")
	contract := unitContract("topic", "v1", "source", "context", "dataset", true, false)
	newService := func(query QueryRecord, current topics.Contract, source exec.Binding, replayer *cw07ReplayRouter) (*Service, *unitExecutor, *unitRepository) {
		repo := newUnitRepository()
		repo.queries[query.ID] = query
		executor := &unitExecutor{reports: []exec.ExecutionReport{unitResult("succeeded")}}
		return &Service{topics: &unitTopicReader{current: map[string]topics.Contract{"topic": current}, retained: map[string]topics.Contract{"topic/v1": contract}}, sources: cw07SourceReader{source}, router: replayer, validator: &unitValidator{}, executor: executor, repo: repo}, executor, repo
	}
	replayer := &cw07ReplayRouter{constraints: constraints, binding: exec.Hash(binding)}
	service, executor, repo := newService(q, contract, binding, replayer)
	result, err := service.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "operation-interpretation"})
	if err != nil || result.Status != "succeeded" || executor.calls != 1 {
		t.Fatalf("interpretation-only plan did not run: result=%#v err=%v calls=%d", result, err, executor.calls)
	}
	terminal := repo.queries[q.ID]
	repo.operations["operation-interpretation"] = terminal
	if replayed, err := service.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "operation-interpretation"}); err != nil || replayed.Status != "succeeded" || executor.calls != 1 {
		t.Fatalf("terminal interpretation evidence did not replay: result=%#v err=%v calls=%d", replayed, err, executor.calls)
	}
	replayer.err = exec.ErrBinding
	if _, err := service.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "operation-interpretation"}); !errors.Is(err, exec.ErrBinding) || executor.calls != 1 {
		t.Fatalf("terminal semantic drift exposed retained result: err=%v calls=%d", err, executor.calls)
	}
	replayer.err = nil

	for _, tc := range []struct {
		name   string
		query  QueryRecord
		state  topics.Contract
		source exec.Binding
		replay *cw07ReplayRouter
		mutate func(*QueryRecord)
	}{
		{name: "source-binding", query: q, state: contract, source: cw07ExecBinding(2), replay: replayer},
		{name: "publication", query: q, state: unitContract("topic", "v2", "source", "context", "dataset", true, false), source: binding, replay: replayer},
		{name: "governed-vocabulary", query: q, state: contract, source: binding, replay: &cw07ReplayRouter{err: exec.ErrBinding}},
		{name: "parser", query: q, state: contract, source: binding, replay: &cw07ReplayRouter{err: exec.ErrBinding}},
		{name: "receipt", query: q, state: contract, source: binding, replay: replayer, mutate: func(query *QueryRecord) { query.Clarification.Binding.Statement = exec.Hash("tampered") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := tc.query
			if tc.mutate != nil {
				evidence := *query.Clarification
				query.Clarification = &evidence
				tc.mutate(&query)
			}
			service, executor, _ := newService(query, tc.state, tc.source, tc.replay)
			_, err := service.Run(context.Background(), e, RunRequest{QueryID: query.ID, Operation: "operation-" + tc.name})
			if !errors.Is(err, exec.ErrBinding) || executor.calls != 0 {
				t.Fatalf("drift reached execution: err=%v calls=%d", err, executor.calls)
			}
		})
	}

	replaced, replacedBinding, replacedConstraints := cw07InterpretationQuery(t, "SOUTH")
	service, executor, _ = newService(replaced, contract, replacedBinding, &cw07ReplayRouter{constraints: replacedConstraints, binding: exec.Hash(replacedBinding)})
	if _, err := service.Run(context.Background(), e, RunRequest{QueryID: replaced.ID, Operation: "operation-replaced"}); err != nil || executor.calls != 1 {
		t.Fatalf("reviewed correction did not rebind: err=%v calls=%d", err, executor.calls)
	}
	removed := unitQuery(e, "query-removed", "topic", "v1", "context", false)
	removed.Route.Interpretation = &nlqroute.Interpretation{Version: "semantic-interpretation-v1", Parser: "deterministic-span-v1"}
	service, executor, _ = newService(removed, contract, binding, &cw07ReplayRouter{err: errors.New("inactive evidence replayed")})
	if _, err := service.Run(context.Background(), e, RunRequest{QueryID: removed.ID, Operation: "operation-removed"}); err != nil || executor.calls != 1 {
		t.Fatalf("removed interpretation remained executable evidence: err=%v calls=%d", err, executor.calls)
	}

	savedQuery, savedBinding, savedConstraints := cw07InterpretationQuery(t, "NORTH")
	savedContract := contract
	savedContract.Publication.Digest = strings.Repeat("d", 64)
	service, executor, _ = newService(savedQuery, savedContract, savedBinding, &cw07ReplayRouter{constraints: savedConstraints, binding: exec.Hash(savedBinding)})
	service.topics.(*unitTopicReader).retained["topic/v1"] = savedContract
	saved := SavedQuestion{Durability: "session_bound", Context: "context", Topics: []SavedTopic{{Topic: "topic", Version: "v1", Digest: savedContract.Publication.Digest}}, Query: savedQuery.ID}
	evidence, err := service.InspectSaved(context.Background(), e, saved)
	if err != nil {
		t.Fatal("saved interpretation evidence unavailable", err)
	}
	plan, err := service.PrepareSaved(context.Background(), e, saved, evidence, "operation-saved-interpretation", "en")
	if err != nil {
		t.Fatal("saved interpretation plan unavailable", err)
	}
	if _, err := service.Run(context.Background(), e, RunRequest{QueryID: plan.Query, Operation: plan.Operation}); err != nil || executor.calls != 1 {
		t.Fatalf("saved interpretation clone bypassed or rejected business evidence: err=%v calls=%d", err, executor.calls)
	}
}
