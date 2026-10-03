package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

// This uses generated, independently reviewed immutable semantics and the real
// source, metadata, HTTP and SDK seams. Recorded model output only fixes the SQL
// candidate; it supplies neither the retained proof nor the independent oracle.
func TestScalarEntailmentLifecycleAcceptance(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t, true)
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	question := ""
	for _, c := range cases {
		if c.ID == "cohort-known-net" {
			question = c.Question
		}
	}
	paid, cancelled := scalarEntailmentSelections(t, current.Pack, datasets["orders"])
	request := nlqexec.PlanRequest{Operation: "scalar-entailment-original-plan", QuestionRequest: nlqexec.QuestionRequest{
		Topic: current.Pack.Topic, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish,
		Question: question, MetricIDs: []string{"known_cohort_net", "unknown_order_amounts", "unknown_cohort_refund_amounts"},
		Kinds: []string{"kpi", "measure", "dimension"}, LimitPerKind: 8,
		InterpretationSelections: []nlqroute.InterpretationSelection{paid},
	}}
	if request.Question == "" {
		t.Fatal("missing held-out cohort question")
	}
	model.mode.Store(phase18RawResponse(t, scopedNetSQL))
	client := scalarEntailmentClient(t, h, h.query, h.queryActor, phase18Scopes(h.queryActor.Tenant(), true))
	plan, err := client.PlanNLQ(t.Context(), request)
	if err != nil {
		t.Fatal("SDK scalar entailment Plan", err)
	}
	scope, _ := store.NewScope(h.queryActor.Tenant(), h.queryActor.User())
	read := func(id string) nlqexec.QueryRecord {
		t.Helper()
		q, err := h.f.db.ReadQuery(t.Context(), scope, id)
		if err != nil {
			t.Fatal("stored query", err)
		}
		return q
	}
	stored := read(plan.QueryID)
	if plan.Analytical == nil || plan.Analytical.Version != exec.AnalyticalScalarEntailmentVersion || plan.Bindings == nil || plan.Bindings.SchemaVersion != 6 || stored.AnalyticalVersion != 13 || stored.Clarification == nil || stored.Clarification.BaseSQL != scopedNetSQL || len(stored.Clarification.BaseParameters) != 0 || len(stored.Parameters) != 4 || stored.PlanOperation != request.Operation {
		t.Fatal("v13 protected base, durable original operation or schema6 custody missing")
	}
	if stored.Analytical.ScalarEntailment != exec.AnalyticalScalarEntailmentPolicy || stored.Clarification.Binding.PopulationPolicy != exec.AnalyticalScalarEntailmentPolicy || len(stored.Clarification.Binding.Entailments) != 1 {
		t.Fatal("missing closed entailment policy/effect")
	}
	effect := stored.Clarification.Binding.Entailments[0]
	if effect.Kind != "entailed_scalar_predicate" || effect.Occurrences != 6 || effect.Parameters == nil || len(effect.Parameters) != 0 || len(effect.Coverage) != 64 || len(effect.Resolution) != 64 {
		t.Fatal("six distinct selected aggregate occurrences lack zero-parameter effect")
	}
	wantOutputs := map[string]int{}
	for i, id := range request.MetricIDs {
		wantOutputs[current.Pack.Topic+":kpi:"+id] = i
	}
	if len(stored.Analytical.Outputs) != len(wantOutputs) {
		t.Fatal("missing exact output ordinal evidence")
	}
	for _, output := range stored.Analytical.Outputs {
		if ordinal, ok := wantOutputs[output.Metric]; !ok || ordinal != output.Column {
			t.Fatal("metric bound to wrong result ordinal", output)
		}
		delete(wantOutputs, output.Metric)
	}
	public, err := json.Marshal([]any{plan.Analytical, plan.Bindings})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"canonical_value"`, `"value":"P"`, `"values":["P"]`, `"base_sql"`, scopedNetSQL} {
		if strings.Contains(string(public), forbidden) {
			t.Fatal("protected predicate or base SQL entered public proof")
		}
	}

	ordinaryRequest := request
	ordinaryRequest.Operation = "scalar-entailment-v9-control"
	ordinaryRequest.InterpretationSelections = nil
	ordinary, err := client.PlanNLQ(t.Context(), ordinaryRequest)
	if err != nil {
		t.Fatal("SDK ordinary v9 control", err)
	}
	old := read(ordinary.QueryID)
	if old.AnalyticalVersion != 9 || old.Analytical == nil || old.Analytical.Version != exec.AnalyticalScopedPopulationsVersion || old.Clarification == nil || old.Clarification.Binding.SchemaVersion != 2 || old.Clarification.Binding.Entailments != nil || old.SQL != stored.SQL || !reflect.DeepEqual(old.Parameters, stored.Parameters) {
		t.Fatal("entailed request changed ordinary v9 SQL/period parameter vector or relabeled old policy")
	}
	if !reflect.DeepEqual(old.Clarification.Binding.Bindings, stored.Clarification.Binding.Bindings) {
		t.Fatal("entailed request changed ordinary v9 fact-owned period positions")
	}
	metadata := support.Raw(t, h.f.dsn)
	reads := func() int64 { return count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) }
	noWork := func(t *testing.T, label string, calls int64, attempts int64) {
		t.Helper()
		if got := model.requests.Load(); got != calls || reads() != attempts {
			t.Fatalf("%s performed gateway/result work: calls %d -> %d, reads %d -> %d", label, calls, got, attempts, reads())
		}
	}
	run := nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "scalar-entailment-terminal-run"}
	result, err := client.RunNLQ(t.Context(), run)
	if err != nil {
		t.Fatal("SDK scalar entailment Run", err)
	}
	scalarEntailmentAssertRows(t, result, scalarPredicateOracle(t, warehouse, false, true))
	ordinaryRun := nlqexec.RunRequest{QueryID: ordinary.QueryID, Operation: "scalar-entailment-v9-run"}
	oldResult, err := client.RunNLQ(t.Context(), ordinaryRun)
	if err != nil {
		t.Fatal("ordinary v9 Run", err)
	}
	scalarEntailmentAssertRows(t, oldResult, scalarPredicateOracle(t, warehouse, false, false))

	// Recreate the router and service and reach them through a new HTTP server.
	// The original Plan key remains distinct from the later terminal Run key.
	restarted := scalarEntailmentService(t, h, model, h.f.db)
	restartedClient := scalarEntailmentClient(t, h, restarted, h.queryActor, phase18Scopes(h.queryActor.Tenant(), true))
	calls, attempts := model.requests.Load(), reads()
	replay, err := restartedClient.RunNLQ(t.Context(), run)
	if err != nil {
		t.Fatal("SDK terminal replay after recreation", err)
	}
	scalarEntailmentAssertRows(t, replay, scalarPredicateOracle(t, warehouse, false, true))
	repeated, err := restartedClient.PlanNLQ(t.Context(), request)
	if err != nil || repeated.QueryID != plan.QueryID || repeated.SessionID != plan.SessionID || repeated.Analytical != nil || repeated.Bindings != nil || repeated.SQL != "" {
		t.Fatal("original-operation SDK Plan pointer replay changed identity or exposed proof", err)
	}
	oldReplay, err := restartedClient.RunNLQ(t.Context(), ordinaryRun)
	if err != nil {
		t.Fatal("v9 terminal replay after recreation", err)
	}
	scalarEntailmentAssertRows(t, oldReplay, scalarPredicateOracle(t, warehouse, false, false))
	oldRepeated, err := restartedClient.PlanNLQ(t.Context(), ordinaryRequest)
	if err != nil || oldRepeated.QueryID != ordinary.QueryID || oldRepeated.SessionID != ordinary.SessionID || oldRepeated.Analytical != nil || oldRepeated.Bindings != nil || oldRepeated.SQL != "" {
		t.Fatal("v9 original-operation pointer replay changed identity or exposed proof", err)
	}
	changedSubmission := request
	changedSubmission.Question += " altered request"
	if _, err := restartedClient.PlanNLQ(t.Context(), changedSubmission); err == nil {
		t.Fatal("same original Plan operation accepted a different request")
	}
	noWork(t, "v13 and old v9 replay", calls, attempts)

	for _, fault := range []string{
		"schema", "binding_policy", "proof_policy", "proof_scope", "proof_coverage", "effect_missing", "effect_duplicate", "effect_extra", "effect_kind", "effect_nil_parameters", "effect_parameters", "effect_coverage", "effect_resolution", "effect_occurrences", "period_omission", "period_parameters", "period_population", "base_sql", "base_parameters", "final_sql", "parameters", "query_digest", "contract_digest", "constraint_digest", "source_digest", "validation_source", "validation_context", "context", "version", "output_swap", "route_value", "route_publication",
	} {
		t.Run("recomputed_tamper_"+fault, func(t *testing.T) {
			bad := scalarEntailmentService(t, h, model, scalarEntailmentFault{Repository: h.f.db, fault: fault})
			calls, attempts := model.requests.Load(), reads()
			if _, err := bad.Run(t.Context(), h.queryActor, run); err == nil {
				t.Fatal("tampered terminal result replay accepted")
			}
			noWork(t, "tampered terminal Run "+fault, calls, attempts)
			if _, err := bad.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "scalar-entailment-tampered-" + fault}); err == nil {
				t.Fatal("tampered record authorized a fresh result attempt")
			}
			noWork(t, "tampered fresh Run "+fault, calls, attempts)
			if _, err := bad.Refine(t.Context(), h.queryActor, nlqexec.RefineRequest{QueryID: plan.QueryID}); err == nil {
				t.Fatal("tampered protected base authorized refinement")
			}
			noWork(t, "tampered Refine "+fault, calls, attempts)
		})
	}

	t.Run("current_signed_authority", func(t *testing.T) {
		for _, denial := range []string{"tenant", "actor", "session", "execute", "source", "dataset", "topic", "context"} {
			t.Run(denial, func(t *testing.T) {
				tenant, user := h.queryActor.Tenant(), h.queryActor.User()
				if denial == "tenant" {
					tenant = "other-tenant"
				}
				if denial == "actor" {
					user = "other-user"
				}
				scopes := phase18Scopes(tenant, true)
				for i, scope := range scopes {
					if denial == "execute" && scope == "query.execute" {
						scopes[i] = "query.ungranted"
					}
					if prefix := map[string]string{"source": "cw.source.query:", "dataset": "cw.dataset.query:", "topic": "cw.topic.read:", "context": "cw.execution_context.use:"}[denial]; prefix != "" && strings.HasPrefix(scope, prefix) {
						scopes[i] = prefix + "unrelated"
					}
				}
				claims := h.f.token.claims(tenant, user, scopes)
				claims["session"] = h.queryActor.Session()
				if denial == "session" {
					claims["session"] = "other-session"
				}
				bearer := h.f.token.sign(t, claims, nil)
				server := httptest.NewServer(nlqapi.ExecutionHandler(h.f.token.verifier, restarted, http.NotFoundHandler()))
				defer server.Close()
				denied, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
				if err != nil {
					t.Fatal(err)
				}
				calls, attempts := model.requests.Load(), reads()
				if _, err := denied.RunNLQ(t.Context(), run); err == nil {
					t.Fatal("current bearer lacked required reach but obtained retained result")
				}
				noWork(t, "retained Run authority "+denial, calls, attempts)
				if denial == "tenant" || denial == "session" {
					if _, err := denied.PlanNLQ(t.Context(), request); err == nil {
						t.Fatal("foreign original-operation pointer lookup accepted")
					}
					noWork(t, "original Plan authority "+denial, calls, attempts)
				}
				if denial == "actor" {
					// Operations are actor-namespaced. This bearer can submit a
					// fresh authorized request, but cannot recover the other actor's ID.
					own, err := denied.PlanNLQ(t.Context(), request)
					if err != nil || own.QueryID == "" || own.QueryID == plan.QueryID || own.Analytical == nil || own.Analytical.Version != exec.AnalyticalScalarEntailmentVersion {
						t.Fatal("actor-namespaced original operation did not create a distinct authorized plan", err)
					}
					otherScope, err := store.NewScope(tenant, user)
					if err != nil {
						t.Fatal(err)
					}
					owned, err := h.f.db.ReadQuery(t.Context(), otherScope, own.QueryID)
					if err != nil || owned.ID != own.QueryID || owned.PlanOperation != request.Operation || owned.Session != h.queryActor.Session() {
						t.Fatal("fresh plan lost its actor/session operation ownership", err)
					}
					if _, err := h.f.db.ReadQuery(t.Context(), scope, own.QueryID); !errors.Is(err, store.ErrNotFound) {
						t.Fatal("other actor's fresh plan leaked back to original owner", err)
					}
					if model.requests.Load() <= calls || reads() != attempts {
						t.Fatal("actor-namespaced fresh Plan did not generate independently or performed result work")
					}
				}
			})
		}
	})

	t.Run("original_plan_pointer_requires_plan_action", func(t *testing.T) {
		scopes := phase18Scopes(h.queryActor.Tenant(), true)
		for i, scope := range scopes {
			if scope == "query.plan" {
				scopes[i] = "query.ungranted"
			}
		}
		denied := scalarEntailmentClient(t, h, restarted, h.queryActor, scopes)
		calls, attempts := model.requests.Load(), reads()
		if _, err := denied.PlanNLQ(t.Context(), request); err == nil {
			t.Fatal("original-operation pointer lookup bypassed current query.plan authority")
		}
		noWork(t, "original Plan action denial", calls, attempts)
	})

	t.Run("refinement_current_predicates", func(t *testing.T) {
		remove := nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{InterpretationEdits: []nlqroute.InterpretationEdit{{Target: paid.Topic + ":" + paid.Dimension + ":" + paid.Value, Action: "remove"}}}}
		removed, err := restartedClient.RefineNLQ(t.Context(), remove)
		if err != nil {
			t.Fatal("remove governed selection", err)
		}
		q := read(removed.QueryID)
		if removed.Analytical == nil || removed.Analytical.Version != exec.AnalyticalScopedPopulationsVersion || q.AnalyticalVersion != 9 || q.Clarification == nil || q.Clarification.Binding.SchemaVersion != 2 || q.Clarification.Binding.Entailments != nil || q.Clarification.BaseSQL != scopedNetSQL || q.SQL != old.SQL || !reflect.DeepEqual(q.Parameters, old.Parameters) || q.Parent != plan.QueryID {
			t.Fatal("removed selection inherited v13 discharge or lost protected base")
		}
		removedRun := nlqexec.RunRequest{QueryID: removed.QueryID, Operation: "scalar-entailment-refined-v9-run"}
		beforeCalls, beforeReads := model.requests.Load(), reads()
		removedResult, err := restartedClient.RunNLQ(t.Context(), removedRun)
		if err != nil {
			t.Fatal("removed-selection v9 child Run", err)
		}
		scalarEntailmentAssertRows(t, removedResult, scalarPredicateOracle(t, warehouse, false, false))
		if model.requests.Load() != beforeCalls || reads() != beforeReads+1 {
			t.Fatal("removed-selection child did not perform exactly one source-only result attempt")
		}
		changed := nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{InterpretationSelections: []nlqroute.InterpretationSelection{cancelled}}}
		chats, attempts := scalarEntailmentChatCalls(model), reads()
		if _, err := restartedClient.RefineNLQ(t.Context(), changed); err == nil {
			t.Fatal("nonentailed reviewed value inherited old discharge")
		}
		if scalarEntailmentChatCalls(model) != chats || reads() != attempts {
			t.Fatal("nonentailed refinement reached SQL generation or result execution")
		}
		changedPeriod := nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: strings.ReplaceAll(question, "2026", "2025")}}
		child, err := restartedClient.RefineNLQ(t.Context(), changedPeriod)
		if err != nil {
			t.Fatal("same selection fresh period", err)
		}
		q = read(child.QueryID)
		if child.Analytical == nil || child.Analytical.Version != exec.AnalyticalScalarEntailmentVersion || q.Clarification == nil || q.Clarification.BaseSQL != scopedNetSQL || q.Clarification.Binding.SchemaVersion != 6 || len(q.Clarification.Binding.Entailments) != 1 || q.Clarification.Binding.Entailments[0].Coverage == effect.Coverage || reflect.DeepEqual(q.Parameters, stored.Parameters) || len(q.Parameters) != 4 || q.Parent != plan.QueryID {
			t.Fatal("period replacement reused historical entailment coverage/bounds")
		}
		for _, p := range q.Parameters {
			if strings.HasPrefix(p.Value, "2027-") {
				t.Fatal("historical period upper bound survived refinement")
			}
		}
		if q.Route.Interpretation == nil || len(q.Route.Interpretation.Values) != 1 || q.Route.Interpretation.Values[0].GovernedValue != paid.Value {
			t.Fatal("fresh period lost explicit paid selection")
		}
		periodRun := nlqexec.RunRequest{QueryID: child.QueryID, Operation: "scalar-entailment-refined-2025-run"}
		beforeCalls, beforeReads = model.requests.Load(), reads()
		periodResult, err := restartedClient.RunNLQ(t.Context(), periodRun)
		if err != nil {
			t.Fatal("fresh-period v13 child Run", err)
		}
		want2025 := scalarPredicateYearOracle(t, warehouse, 2025, false, true)
		scalarEntailmentAssertRows(t, periodResult, want2025)
		if model.requests.Load() != beforeCalls || reads() != beforeReads+1 {
			t.Fatal("new-period child did not perform exactly one source-only result attempt")
		}
		beforeCalls, beforeReads = model.requests.Load(), reads()
		periodReplay, err := restartedClient.RunNLQ(t.Context(), periodRun)
		if err != nil {
			t.Fatal("fresh-period child terminal replay", err)
		}
		scalarEntailmentAssertRows(t, periodReplay, want2025)
		noWork(t, "fresh-period child replay", beforeCalls, beforeReads)
		t.Logf("refined 2025 cohort independent source oracle=%v", want2025)
	})

	t.Run("learning_producer_and_owned_consumer_closed", func(t *testing.T) {
		if err := restarted.Feedback(t.Context(), h.queryActor, nlqexec.FeedbackRequest{QueryID: plan.QueryID, Verdict: "positive"}); err != nil {
			t.Fatal("v13 feedback must remain recordable", err)
		}
		examples, err := restarted.Examples(t.Context(), h.queryActor, current.Pack.Topic, 8)
		if err != nil || len(examples) != 0 {
			t.Fatal("schema6 silently entered automatic learning", err)
		}
		learned := assertScopedLearningBase(t, restarted, h.queryActor, current.Pack.Topic, ordinary.QueryID, scopedNetSQL, nlqexec.ScopedScalarExamplePolicy)
		active := activateScopedLearningBase(t, restarted, h.queryActor, learned)
		fresh := request
		fresh.Operation = "scalar-entailment-rejects-v9-owned-example"
		planned, err := restartedClient.PlanNLQ(t.Context(), fresh)
		if err != nil {
			t.Fatal("v13 owned-example consumption control", err)
		}
		q := read(planned.QueryID)
		if q.ExampleSelection.Eligibility == nil || q.ExampleSelection.Eligibility.CurrentOwnedPredicates || q.ExampleSelection.Eligibility.CurrentScopedPolicy != "" {
			t.Fatal("schema6 borrowed older owned-example eligibility")
		}
		for _, candidate := range q.ExampleSelection.Selected {
			if candidate.ExampleID == active.ID {
				t.Fatal("v13 selected an active v9 owned example")
			}
		}
		if q.ExampleSelection.Usage != nil {
			for _, used := range q.ExampleSelection.Usage.Used {
				if used.ExampleID == active.ID {
					t.Fatal("v13 used an active v9 owned example")
				}
			}
		}
	})

	for name, sql := range map[string]string{
		"missing_count_companion":     strings.Replace(scopedNetSQL, ",g.events-g.known AS unknown_orders", "", 1),
		"missing_refund_count_filter": scalarEntailmentMissingCountFilterSQL(),
		"wrong_refund_count_filter":   strings.Replace(scopedNetSQL, "count(r.refund_id) AS events", "count(r.refund_id) FILTER (WHERE r.status_code='V') AS events", 1),
		"missing_parent_filter":       strings.Replace(scopedNetSQL, " AND o.status_code='P'", "", 1),
		"swapped_complete_key_pairs":  strings.Replace(scopedNetSQL, "r.division_id=o.division_id AND r.order_id=o.order_id", "r.division_id=o.order_id AND r.order_id=o.division_id", 1),
		"wrong_complete_key_member":   strings.Replace(scopedNetSQL, "r.division_id=o.division_id AND r.order_id=o.order_id", "r.division_id=o.division_id AND r.refund_id=o.order_id", 1),
		"incomplete_composite_key":    strings.Replace(scopedNetSQL, "r.division_id=o.division_id AND ", "", 1),
	} {
		t.Run("native_"+name, func(t *testing.T) {
			if sql == scopedNetSQL {
				t.Fatal("native negative did not modify the SQL candidate")
			}
			model.mode.Store(phase18RawResponse(t, sql))
			defer model.mode.Store(phase18RawResponse(t, scopedNetSQL))
			fresh := request
			fresh.Operation = "scalar-entailment-native-" + name
			chats, attempts := scalarEntailmentChatCalls(model), reads()
			if _, err := restartedClient.PlanNLQ(t.Context(), fresh); err == nil {
				t.Fatal("malformed aggregate population accepted")
			}
			if scalarEntailmentChatCalls(model) <= chats || reads() != attempts {
				t.Fatal("native negative did not actually generate the bad candidate or executed result work")
			}
		})
	}

	t.Run("current_physical_uniqueness", func(t *testing.T) {
		if _, err := h.f.admin.Exec(t.Context(), `ALTER TABLE analytics.adv_orders DROP CONSTRAINT adv_orders_pkey CASCADE`); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := h.f.admin.Exec(t.Context(), `ALTER TABLE analytics.adv_orders ADD PRIMARY KEY(division_id,order_id);
ALTER TABLE analytics.adv_refunds ADD CONSTRAINT adv_refunds_division_id_order_id_fkey FOREIGN KEY(division_id,order_id) REFERENCES analytics.adv_orders(division_id,order_id);
ALTER TABLE analytics.adv_order_lines ADD CONSTRAINT adv_order_lines_division_id_order_id_fkey FOREIGN KEY(division_id,order_id) REFERENCES analytics.adv_orders(division_id,order_id)`); err != nil {
				t.Fatal(err)
			}
		}()
		calls, attempts := model.requests.Load(), reads()
		if _, err := restartedClient.RunNLQ(t.Context(), run); err == nil {
			t.Fatal("terminal replay borrowed removed physical uniqueness")
		}
		if _, err := restartedClient.RefineNLQ(t.Context(), nlqexec.RefineRequest{QueryID: plan.QueryID}); err == nil {
			t.Fatal("refinement borrowed removed physical uniqueness")
		}
		fresh := request
		fresh.Operation = "scalar-entailment-without-current-unique-key"
		if _, err := restartedClient.PlanNLQ(t.Context(), fresh); err == nil {
			t.Fatal("fresh proof borrowed stale reviewed uniqueness")
		}
		noWork(t, "current uniqueness denial", calls, attempts)
	})

	t.Run("restored_uniqueness_control", func(t *testing.T) {
		calls, attempts := model.requests.Load(), reads()
		replay, err := restartedClient.RunNLQ(t.Context(), run)
		if err != nil {
			t.Fatal("restored physical keys must restore replay before publication mutation", err)
		}
		scalarEntailmentAssertRows(t, replay, scalarPredicateOracle(t, warehouse, false, true))
		noWork(t, "restored uniqueness replay", calls, attempts)
	})

	t.Run("current_publication", func(t *testing.T) {
		pub, err := h.topics.Read(t.Context(), h.author, current.Pack.Topic, "")
		if err != nil {
			t.Fatal(err)
		}
		archived, err := h.topics.Archive(t.Context(), h.author, current.Pack.Topic, pub.State.Revision, "Qualify current entailment publication custody")
		if err != nil {
			t.Fatal(err)
		}
		calls, attempts := model.requests.Load(), reads()
		if _, err := restartedClient.RunNLQ(t.Context(), run); err == nil {
			t.Fatal("retired current publication authorized entailment replay")
		}
		if _, err := restartedClient.RefineNLQ(t.Context(), nlqexec.RefineRequest{QueryID: plan.QueryID}); err == nil {
			t.Fatal("retired publication authorized refinement")
		}
		noWork(t, "current publication denial", calls, attempts)
		if _, err := h.topics.Rollback(t.Context(), h.author, current.Pack.Topic, topics.TransitionRequest{Version: pub.State.Version, Expected: archived.Revision, Note: "Restore the same reviewed test publication"}); err != nil {
			t.Fatal("restore current publication", err)
		}
	})

	var currentPublicationRun nlqexec.RunRequest
	t.Run("restored_publication_requires_fresh_admission", func(t *testing.T) {
		calls, attempts := model.requests.Load(), reads()
		// Archive and rollback advance the authenticated AnswerContext's
		// publication revision. The historical binding stays historical.
		if _, err := restartedClient.RunNLQ(t.Context(), run); err == nil {
			t.Fatal("publication rollback silently restored historical predicate authority")
		}
		if _, err := restartedClient.RefineNLQ(t.Context(), nlqexec.RefineRequest{QueryID: plan.QueryID}); err == nil {
			t.Fatal("publication rollback authorized a stale refinement parent")
		}
		noWork(t, "stale restored-publication denial", calls, attempts)
		fresh := request
		fresh.Operation = "scalar-entailment-restored-publication-plan"
		currentPlan, err := restartedClient.PlanNLQ(t.Context(), fresh)
		if err != nil || currentPlan.QueryID == plan.QueryID || currentPlan.Analytical == nil || currentPlan.Analytical.Version != exec.AnalyticalScalarEntailmentVersion {
			t.Fatal("restored publication lacks a freshly authorized v13 plan", err)
		}
		currentRecord := read(currentPlan.QueryID)
		if currentRecord.Route.AnswerContext == stored.Route.AnswerContext {
			t.Fatal("fresh publication admission borrowed historical answer context")
		}
		currentPublicationRun = nlqexec.RunRequest{QueryID: currentPlan.QueryID, Operation: "scalar-entailment-restored-publication-run"}
		calls, attempts = model.requests.Load(), reads()
		result, err := restartedClient.RunNLQ(t.Context(), currentPublicationRun)
		if err != nil {
			t.Fatal("fresh restored-publication Run", err)
		}
		scalarEntailmentAssertRows(t, result, scalarPredicateOracle(t, warehouse, false, true))
		if model.requests.Load() != calls || reads() != attempts+1 {
			t.Fatal("fresh restored-publication Run did not perform exactly one source-only attempt")
		}
		calls, attempts = model.requests.Load(), reads()
		replay, err := restartedClient.RunNLQ(t.Context(), currentPublicationRun)
		if err != nil {
			t.Fatal("fresh restored-publication replay control", err)
		}
		scalarEntailmentAssertRows(t, replay, scalarPredicateOracle(t, warehouse, false, true))
		noWork(t, "fresh restored-publication terminal replay", calls, attempts)
	})

	t.Run("current_source_revision", func(t *testing.T) {
		if currentPublicationRun.QueryID == "" {
			t.Fatal("source revision negative lacks its independently valid terminal control")
		}
		origin := current.Pack.Datasets[0].Source
		source, err := h.f.s.Get(t.Context(), h.f.e, origin.Source)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.f.s.Rotate(t.Context(), h.f.e, origin.Source, source.Revision); err != nil {
			t.Fatal("advance actual source context revision", err)
		}
		calls, attempts := model.requests.Load(), reads()
		if _, err := restartedClient.RunNLQ(t.Context(), currentPublicationRun); err == nil {
			t.Fatal("terminal replay accepted changed actual source context")
		}
		if _, err := restartedClient.RefineNLQ(t.Context(), nlqexec.RefineRequest{QueryID: currentPublicationRun.QueryID}); err == nil {
			t.Fatal("refinement accepted changed actual source context")
		}
		noWork(t, "actual source revision denial", calls, attempts)
	})
}

func scalarEntailmentSelections(t *testing.T, pack semantics.TopicPack, orders string) (nlqroute.InterpretationSelection, nlqroute.InterpretationSelection) {
	t.Helper()
	var paid, cancelled nlqroute.InterpretationSelection
	for _, d := range pack.Dimensions {
		if d.Field.Dataset != orders || d.Field.ID != "status_code" {
			continue
		}
		for _, v := range d.Values {
			s := nlqroute.InterpretationSelection{Topic: pack.Topic, Dimension: d.ID, Value: v.ID, Operator: "eq"}
			if v.ID == "orders_paid" && v.Value == "P" {
				paid = s
			}
			if v.ID == "orders_cancelled" && v.Value == "C" {
				cancelled = s
			}
		}
	}
	if paid.Value == "" || cancelled.Value == "" {
		t.Fatal("missing exact reviewed paid/cancelled selection controls")
	}
	return paid, cancelled
}

func scalarEntailmentService(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, repo nlqexec.Repository) *nlqexec.Service {
	t.Helper()
	index, err := vindex.New(h.f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(h.f.db, h.f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(h.topics, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(router, h.topics, h.f.s, h.f.validator, h.f.executor, model.engine, repo)
	if err != nil {
		t.Fatal(err)
	}
	return query
}

func scalarEntailmentClient(t *testing.T, h *generatedTopicHarness, query *nlqexec.Service, actor identity.Envelope, scopes []string) *sdk.Client {
	t.Helper()
	server := httptest.NewServer(nlqapi.ExecutionHandler(h.f.token.verifier, query, http.NotFoundHandler()))
	t.Cleanup(server.Close)
	claims := h.f.token.claims(actor.Tenant(), actor.User(), scopes)
	claims["session"] = actor.Session()
	bearer := h.f.token.sign(t, claims, nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func scalarEntailmentAssertRows(t *testing.T, result nlqexec.RunResult, want []string) {
	t.Helper()
	if result.Execution.Result == nil || len(result.Execution.Result.Rows) != 1 || len(result.Execution.Result.Rows[0]) != len(want) {
		t.Fatal("missing exact scalar result shape")
	}
	for i, cell := range result.Execution.Result.Rows[0] {
		got := "NULL"
		if string(cell) != "null" && json.Unmarshal(cell, &got) != nil {
			got = string(cell)
		}
		if got != want[i] && (got == "NULL" || want[i] == "NULL" || !liveNumberEquals(got, want[i])) {
			t.Fatalf("row oracle output %d: got %s want %s", i, got, want[i])
		}
	}
}

// Counting recorded HTTP chat dispatches proves SQL generation did not occur;
// a fresh Route is allowed to make independently budgeted embedding requests.
func scalarEntailmentChatCalls(model *gatewayFixture) int {
	model.mu.Lock()
	defer model.mu.Unlock()
	n := 0
	for _, path := range model.paths {
		if !strings.Contains(path, "embedding") && !strings.Contains(path, "rerank") {
			n++
		}
	}
	return n
}

// Faults enter only through the protected storage seam. Plan-operation lookup
// still returns only the historical opaque ID, never an executable proof.
// Public integrity hashes
// are refreshed after modifications; admission must reconstruct the actual
// semantic/binding authority rather than accept a self-consistent JSON receipt.
type scalarEntailmentFault struct {
	nlqexec.Repository
	fault string
}

func (r scalarEntailmentFault) ReadQuery(ctx context.Context, scope store.Scope, id string) (nlqexec.QueryRecord, error) {
	q, err := r.Repository.ReadQuery(ctx, scope, id)
	if err == nil {
		q = r.mutate(q)
	}
	return q, err
}

func (r scalarEntailmentFault) ReadPlanOperation(ctx context.Context, scope store.Scope, operation string) (nlqexec.QueryRecord, error) {
	reader, ok := r.Repository.(nlqexec.PlanOperationReader)
	if !ok {
		return nlqexec.QueryRecord{}, fmt.Errorf("test repository lacks original Plan operation seam")
	}
	q, err := reader.ReadPlanOperation(ctx, scope, operation)
	if err == nil {
		q = r.mutate(q)
	}
	return q, err
}

func (r scalarEntailmentFault) mutate(q nlqexec.QueryRecord) nlqexec.QueryRecord {
	if q.AnalyticalVersion != 13 || q.Analytical == nil || q.Clarification == nil {
		return q
	}
	b, p := &q.Clarification.Binding, q.Analytical
	switch r.fault {
	case "schema":
		b.SchemaVersion = 2
	case "binding_policy":
		b.PopulationPolicy = exec.AnalyticalScalarPopulationPolicy
	case "proof_policy":
		p.ScalarEntailment = "unreviewed-scalar-entailment"
	case "proof_scope":
		p.Scope = strings.ReplaceAll(p.Scope, "independent_entailed_scoped_singleton_populations", "independent_scoped_singleton_populations")
	case "proof_coverage":
		p.ScalarEntailmentCoverage = exec.Hash("forged complete coverage")
	case "effect_missing":
		b.Entailments = nil
	case "effect_duplicate":
		b.Entailments = append(b.Entailments, b.Entailments[0])
	case "effect_extra":
		extra := b.Entailments[0]
		extra.Resolution = exec.Hash("extra resolution")
		b.Entailments = append(b.Entailments, extra)
	case "effect_kind":
		b.Entailments[0].Kind = "ordinary_predicate"
	case "effect_nil_parameters":
		b.Entailments[0].Parameters = nil
	case "effect_parameters":
		b.Entailments[0].Parameters = []int{1}
	case "effect_coverage":
		b.Entailments[0].Coverage = exec.Hash("self-consistent invented leaf coverage")
	case "effect_resolution":
		b.Entailments[0].Resolution = exec.Hash("another predicate resolution")
	case "effect_occurrences":
		b.Entailments[0].Occurrences--
	case "period_omission":
		b.Bindings = b.Bindings[:1]
	case "period_parameters":
		b.Bindings[0].Parameters = []int{3, 4}
	case "period_population":
		b.Bindings[0].Population = b.Bindings[1].Population
	case "base_sql":
		q.Clarification.BaseSQL = strings.Replace(q.Clarification.BaseSQL, " AND o.status_code='P'", "", 1)
	case "base_parameters":
		q.Clarification.BaseParameters = []exec.Parameter{{Kind: "text", Value: "P"}}
	case "final_sql":
		q.SQL = strings.Replace(q.SQL, "g.total - r.total", "r.total - g.total", 1)
		q.SQL += " WHERE FALSE"
	case "parameters":
		q.Parameters[0].Value = "2025-01-01T05:00:00Z"
	case "contract_digest":
		p.Contract = exec.Hash([]any{p.Contract, "invented reviewed contract"})
	case "constraint_digest":
		b.Constraints = exec.Hash([]any{b.Bindings, b.Entailments})
	case "source_digest":
		q.Route.SourceBindingDigest = exec.Hash("changed current source")
		b.SourceBinding = q.Route.SourceBindingDigest
	case "validation_source":
		b.Validation.Source = "other-source"
	case "validation_context":
		b.Validation.Context = "other-context"
	case "context":
		q.Context = "other-context"
		q.Route.Request.Context = q.Context
	case "version":
		q.AnalyticalVersion = 9
		p.Version = exec.AnalyticalScopedPopulationsVersion
	case "output_swap":
		p.Outputs[0].Column, p.Outputs[1].Column = p.Outputs[1].Column, p.Outputs[0].Column
	case "route_value":
		q.Route.Interpretation.Values[0].CanonicalValue = "C"
		q.Route.Interpretation.Values[0].GovernedValue = "orders_cancelled"
		copy := *q.Route.Interpretation
		copy.Digest = ""
		q.Route.Interpretation.Digest = exec.Hash(copy)
	case "route_publication":
		q.Route.Interpretation.Pins[0].Version = "invented-version"
		copy := *q.Route.Interpretation
		copy.Digest = ""
		q.Route.Interpretation.Digest = exec.Hash(copy)
	}
	p.Query = exec.AnalyticalQueryDigest(q.SQL, q.Parameters)
	b.Statement = exec.Hash([]any{q.SQL, q.Parameters})
	if r.fault == "query_digest" {
		p.Query = exec.Hash([]any{q.SQL, q.Parameters, "forged query"})
	}
	return q
}

// Keep both money aggregates filtered while one COUNT occurrence loses the
// mandatory posted/paid population. A filter on a sibling aggregate is no proof.
func scalarEntailmentMissingCountFilterSQL() string {
	old := "sum(r.amount_usd) AS total,count(r.refund_id) AS events,count(r.amount_usd) AS known"
	filtered := "sum(r.amount_usd) FILTER(WHERE r.status_code='P' AND o.status_code='P') AS total,count(r.refund_id) AS events,count(r.amount_usd) FILTER(WHERE r.status_code='P' AND o.status_code='P') AS known"
	query := strings.Replace(scopedNetSQL, old, filtered, 1)
	return strings.Replace(query, " WHERE r.status_code='P' AND o.status_code='P') SELECT", ") SELECT", 1)
}
