package nlqexec

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/store"
)

func savedDerivationCopy(parent QueryRecord, id string) QueryRecord {
	child := parent
	child.ID, child.Parent, child.SavedCopyParent, child.Operation = id, parent.ID, parent.ID, id+"-operation"
	child.PlanOperation, child.PlanRequestDigest = "", ""
	child.IntentReview, child.GenerationResolution = nil, nil
	child.Status, child.Result, child.Revision, child.ExecutionFixes = "planned", nil, 1, 0
	child.Created, child.Updated = time.Now().UTC(), time.Now().UTC()
	bindParentLineage(&child, &parent)
	return child
}

func TestSavedDerivationCreationPreservesDirectEvidence(t *testing.T) {
	for _, kind := range []string{"ordinary", "resolution", "review", "reviewed-resolution"} {
		t.Run(kind, func(t *testing.T) {
			parent := unitQuery(unitEnvelope(t), "parent", "topic", "v1", "context", false)
			parent.Parent, parent.ParentRevision, parent.ParentDigest = "pending", 1, exec.Hash("pending")
			parent.PlanOperation, parent.PlanRequestDigest = "original-plan", exec.Hash("original-request")
			parent.Status, parent.Result, parent.ExecutionFixes = "succeeded", unitResult("succeeded").Result, 1
			if kind == "resolution" || kind == "reviewed-resolution" {
				parent.GenerationResolution = &GenerationResolution{QueryID: "pending", AnswerDigest: exec.Hash("answer")}
			}
			if kind == "review" || kind == "reviewed-resolution" {
				parent.AnalyticalVersion = 7
				parent.IntentReview = &IntentReviewEvidence{
					Legacy:       IntentReviewOrigin{QueryID: "legacy", Revision: 1, Digest: exec.Hash("legacy")},
					Preflight:    IntentReviewOrigin{QueryID: "preflight", Revision: 1, Digest: exec.Hash("preflight")},
					AnswerDigest: exec.Hash("review"),
				}
			}
			before := QueryLineageDigest(parent)
			child := savedDerivationCopy(parent, "copy")
			if !SavedDerivationCreationValid(child, parent) || before != QueryLineageDigest(parent) {
				t.Fatal("explicit copy changed or failed to bind direct parent evidence")
			}
			child.GenerationResolution, child.IntentReview = parent.GenerationResolution, parent.IntentReview
			if kind != "ordinary" && SavedDerivationCreationValid(child, parent) {
				t.Fatal("copy claimed the parent's direct submission")
			}
		})
	}
}

func TestSavedDerivationCreationRejectsAlteredPayload(t *testing.T) {
	parent := unitQuery(unitEnvelope(t), "parent", "topic", "v1", "context", false)
	parent.Parameters = []exec.Parameter{{Kind: "text", Value: "original"}}
	parent.Analytical = &exec.AnalyticalReceipt{Query: exec.Hash("original-query"), Contract: exec.Hash("original-contract")}
	for _, tc := range []struct {
		name string
		edit func(*QueryRecord)
	}{
		{"missing marker", func(q *QueryRecord) { q.SavedCopyParent = "" }},
		{"foreign marker", func(q *QueryRecord) { q.SavedCopyParent = "other" }},
		{"self", func(q *QueryRecord) { q.Parent, q.SavedCopyParent = q.ID, q.ID }},
		{"parent revision", func(q *QueryRecord) { q.ParentRevision++ }},
		{"parent digest", func(q *QueryRecord) { q.ParentDigest = exec.Hash("changed") }},
		{"plan reservation", func(q *QueryRecord) { q.PlanOperation = "stolen" }},
		{"pending", func(q *QueryRecord) { q.GenerationPending = &GenerationPending{} }},
		{"sql", func(q *QueryRecord) { q.SQL += " LIMIT 1" }},
		{"parameters", func(q *QueryRecord) {
			q.Parameters = []exec.Parameter{{Kind: "text", Value: "other"}}
		}},
		{"rule pins", func(q *QueryRecord) { q.RuleVersions = []string{"other"} }},
		{"question", func(q *QueryRecord) { q.Question = "Different question" }},
		{"route", func(q *QueryRecord) { q.Route.AnswerContext = "other" }},
		{"clarification", func(q *QueryRecord) { q.Clarification = &ClarificationEvidence{SchemaVersion: 1} }},
		{"scope", func(q *QueryRecord) { q.RelationScope = []exec.RelationScope{{Dataset: "other"}} }},
		{"generation", func(q *QueryRecord) { q.Generation.Prompt = "other" }},
		{"receipt", func(q *QueryRecord) { q.Receipt.Warning = "other" }},
		{"analytical query", func(q *QueryRecord) { copy := *q.Analytical; copy.Query = exec.Hash("other"); q.Analytical = &copy }},
		{"analytical contract", func(q *QueryRecord) { copy := *q.Analytical; copy.Contract = exec.Hash("other"); q.Analytical = &copy }},
		{"stale", func(q *QueryRecord) { q.EvidenceStale = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := savedDerivationCopy(parent, "copy")
			tc.edit(&child)
			if SavedDerivationCreationValid(child, parent) {
				t.Fatal("altered copy admitted")
			}
		})
	}
}

func TestSavedDerivationMeaningPreservesExecutionRepair(t *testing.T) {
	parent := unitQuery(unitEnvelope(t), "parent", "topic", "v1", "context", false)
	parent.Analytical = &exec.AnalyticalReceipt{Contract: exec.Hash("contract"), Query: exec.Hash("query")}
	child := savedDerivationCopy(parent, "copy")
	child.SQL += ";"
	child.Status, child.Result, child.Revision, child.ExecutionFixes = "succeeded", unitResult("succeeded").Result, 2, 1
	child.Generation.Prompt = "correction"
	child.Receipt = gateway.Receipt{Warning: "additional correction receipt"}
	child.Assumptions, child.Ambiguities, child.Errors = []string{"corrected"}, []string{"corrected"}, []string{"diagnostic"}
	proof := *child.Analytical
	proof.Query = exec.Hash("corrected-query")
	child.Analytical = &proof
	if !SavedDerivationMeaningEqual(child, parent) || SavedDerivationCreationValid(child, parent) {
		t.Fatal("repair needs immutable meaning equality, not fresh-copy admission")
	}
	if parent.Analytical.Query != exec.Hash("query") {
		t.Fatal("comparison mutated parent receipt")
	}
}

func TestSavedDerivationTraversalIsBoundedAndMetadataOnly(t *testing.T) {
	e := unitEnvelope(t)
	parent := unitQuery(e, "parent", "topic", "v1", "context", false)
	repo := newUnitRepository()
	repo.queries[parent.ID] = parent
	s := &Service{repo: repo}
	child := savedDerivationCopy(parent, "copy")
	if err := s.verifySavedDerivation(context.Background(), e, child); err != nil {
		t.Fatal("exact metadata copy required source/model work", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*QueryRecord)
	}{
		{"foreign session", func(q *QueryRecord) { q.Session = "other" }},
		{"new revision", func(q *QueryRecord) { q.Revision++ }},
		{"new operation", func(q *QueryRecord) { q.Operation = "another-run" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := parent
			tc.edit(&changed)
			repo.queries[parent.ID] = changed
			if err := s.verifySavedDerivation(context.Background(), e, child); err == nil {
				t.Fatal("changed parent accepted")
			}
		})
	}
	delete(repo.queries, parent.ID)
	if err := s.verifySavedDerivation(context.Background(), e, child); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("erased parent was reconstructed", err)
	}
	repo.queries[parent.ID] = parent
	last := parent
	for i := 0; i <= MaxRefinementDepth; i++ {
		last = savedDerivationCopy(last, fmt.Sprintf("copy-%d", i))
		repo.queries[last.ID] = last
	}
	if err := s.verifySavedDerivation(context.Background(), e, last); !errors.Is(err, ErrRefinementLimit) {
		t.Fatal("unbounded copy ancestry", err)
	}
}

func TestSavedDerivationResolvedOriginDoesNotRenewExpiredAnswerForm(t *testing.T) {
	s, pending, _ := pendingFixture(t)
	e := unitEnvelope(t)
	pending.GenerationPending.Problem.ExpiresAt = time.Now().Add(-time.Hour)
	pending.GenerationPending.Problem.AnswerContext = generationPendingDigest(pending.GenerationPending)
	s.repo.(*unitRepository).queries[pending.ID] = pending
	resolved := unitQuery(e, "resolved", "topic", "v1", "context", false)
	resolved.Parent = pending.ID
	bindParentLineage(&resolved, &pending)
	resolved.GenerationResolution = &GenerationResolution{QueryID: pending.ID, AnswerDigest: exec.Hash("accepted-answer")}
	s.repo.(*unitRepository).queries[resolved.ID] = resolved
	child := savedDerivationCopy(resolved, "copy")
	if err := s.verifySavedDerivation(context.Background(), e, child); err != nil {
		t.Fatal("saved copy required a second answer after valid acceptance", err)
	}
	delete(s.repo.(*unitRepository).queries, pending.ID)
	if err := s.verifySavedDerivation(context.Background(), e, child); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("missing direct generation origin accepted", err)
	}
}

func TestSavedDerivationCorrectionRequiresFreshNativeProof(t *testing.T) {
	e := unitEnvelope(t)
	parent := unitQuery(e, "parent", "topic", "v1", "context", false)
	repo := newUnitRepository()
	repo.queries[parent.ID] = parent
	validator := &sequenceValidator{errors: []error{exec.ErrUnsafe}}
	s := &Service{repo: repo, validator: validator}
	child := savedDerivationCopy(parent, "copy")
	child.SQL += ";"
	if err := s.verifySavedDerivation(context.Background(), e, child); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("unexecuted changed SQL accepted", err)
	}
	child.Status = "succeeded"
	if err := s.verifySavedDerivation(context.Background(), e, child); err != nil || validator.calls != 0 {
		t.Fatal("metadata verification performed native work", err, validator.calls)
	}
	a := admission{source: "source", context: "context", binding: exec.Binding{Dialect: "postgres"}, relationScope: parent.RelationScope}
	if err := s.verifySavedDerivationExecution(context.Background(), e, child, a); !errors.Is(err, exec.ErrUnsafe) || validator.calls != 1 {
		t.Fatal("correction skipped native proof", err, validator.calls)
	}
	if err := s.verifySavedDerivationExecution(context.Background(), e, child, a); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("zero validator plans accepted", err)
	}
}
