package nlqexec

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// SavedDerivationValid distinguishes a saved copy from a direct continuation or
// reviewed replacement. The existing immutable parent pin carries its evidence;
// a copy never claims that the parent's direct submission happened again.
func SavedDerivationValid(q QueryRecord) bool {
	if q.SavedCopyParent == "" {
		return true
	}
	digest, err := hex.DecodeString(q.ParentDigest)
	return identity.Identifier(q.SavedCopyParent) && q.SavedCopyParent == q.Parent && q.Parent != q.ID &&
		q.ParentRevision > 0 && err == nil && len(digest) == 32 && strings.ToLower(q.ParentDigest) == q.ParentDigest &&
		q.PlanOperation == "" && q.PlanRequestDigest == "" && q.IntentReview == nil && q.GenerationResolution == nil &&
		q.GenerationPending == nil && strings.TrimSpace(q.SQL) != ""
}

// SavedDerivationMeaningEqual compares the immutable semantic partition, not
// mutable execution diagnostics or a mechanically equivalent SQL correction.
// It is deliberately stronger than the public saved query definition digest.
func SavedDerivationMeaningEqual(child, parent QueryRecord) bool {
	meaning := func(q QueryRecord) string {
		analytical := q.Analytical
		if analytical != nil {
			copy := *analytical
			copy.Query = ""
			analytical = &copy
		}
		route := q.Route
		if route.Applicability != nil {
			proof := *route.Applicability
			proof.Query, proof.PriorQuery, proof.PriorDigest = "", "", ""
			route.Applicability = &proof
		}
		return exec.Hash([]any{"saved-derivation-meaning-v1", q.Session, q.Context, q.Topic, q.Topics, q.TopicVersions,
			q.RuleVersions, q.Templates, q.ExampleSelection, q.Locale, q.Question, route, q.RelationScope,
			q.AnalyticalVersion, analytical, q.Clarification})
	}
	return meaning(child) == meaning(parent)
}

// SavedDerivationCreationValid is checked while the exact parent is locked at
// persistence. The only differences allowed are the child's new operation and
// lifecycle identity, its explicit derivation pin, and absent direct seals.
func SavedDerivationCreationValid(child, parent QueryRecord) bool {
	if child.Route.Applicability != nil || parent.Route.Applicability != nil {
		c, p := child.Route.Applicability, parent.Route.Applicability
		if c == nil || p == nil || c.Query != child.ID || p.Query != parent.ID || c.PriorQuery != parent.ID || c.PriorDigest != exec.Hash(p) {
			return false
		}
	}
	if child.SavedCopyParent == "" || !identity.Identifier(parent.ID) || !SavedDerivationValid(child) || child.SavedCopyParent != parent.ID || child.ParentRevision != parent.Revision ||
		child.ParentDigest != QueryLineageDigest(parent) || child.EvidenceStale || parent.EvidenceStale ||
		parent.GenerationPending != nil || !GenerationResolutionValid(parent) || !IntentReviewValid(parent) ||
		!SavedDerivationValid(parent) || child.Status != "planned" || child.Result != nil || child.Revision != 1 ||
		child.ExecutionFixes != 0 || !savedDerivationParentStatus(parent.Status) || !SavedDerivationMeaningEqual(child, parent) {
		return false
	}
	payload := func(q QueryRecord) string {
		return exec.Hash([]any{q.SQL, q.Parameters, q.Generation, q.Receipt, q.Analytical, q.Assumptions, q.Ambiguities, q.Errors, q.ValidationFixes})
	}
	return payload(child) == payload(parent)
}

func savedDerivationParentStatus(status string) bool {
	return status == "planned" || status == "succeeded" || status == "empty" || status == "truncated"
}

type savedDerivationPair struct{ child, parent QueryRecord }

// savedDerivationParents uses the actor-scoped repository and checks session
// before consuming a parent. A missing or erased parent is never reconstructed.
func (s *Service) savedDerivationParents(ctx context.Context, e identity.Envelope, q QueryRecord) ([]savedDerivationPair, QueryRecord, error) {
	var pairs []savedDerivationPair
	seen := map[string]bool{}
	for q.SavedCopyParent != "" {
		if len(pairs) >= MaxRefinementDepth || seen[q.ID] {
			return nil, QueryRecord{}, ErrRefinementLimit
		}
		seen[q.ID] = true
		if !SavedDerivationValid(q) || q.Session != e.Session() {
			return nil, QueryRecord{}, exec.ErrBinding
		}
		parent, err := s.repo.ReadQuery(ctx, mustScope(e), q.SavedCopyParent)
		if err != nil {
			return nil, QueryRecord{}, err
		}
		if seen[parent.ID] || parent.ID != q.SavedCopyParent || parent.Session != e.Session() ||
			parent.Context != q.Context || parent.Revision != q.ParentRevision || QueryLineageDigest(parent) != q.ParentDigest ||
			!savedDerivationParentStatus(parent.Status) || !SavedDerivationMeaningEqual(q, parent) || !sameParameters(q.Parameters, parent.Parameters) ||
			(q.SQL != parent.SQL && !terminalQueryStatus(q.Status)) {
			return nil, QueryRecord{}, exec.ErrBinding
		}
		if q.Route.Applicability != nil || parent.Route.Applicability != nil {
			c, p := q.Route.Applicability, parent.Route.Applicability
			if c == nil || p == nil || c.Query != q.ID || p.Query != parent.ID || c.PriorQuery != parent.ID || c.PriorDigest != exec.Hash(p) {
				return nil, QueryRecord{}, exec.ErrBinding
			}
		}
		pairs = append(pairs, savedDerivationPair{q, parent})
		q = parent
	}
	return pairs, q, nil
}

// verifySavedDerivation is the metadata admission hook. Direct seals
// remain on the original record and are checked there, never reinterpreted as
// submissions made by a saved operation. The caller still resolves current
// signed resource reach through its ordinary current/retained admission path.
func (s *Service) verifySavedDerivation(ctx context.Context, e identity.Envelope, q QueryRecord) error {
	if q.SavedCopyParent == "" {
		return nil
	}
	_, origin, err := s.savedDerivationParents(ctx, e, q)
	if err != nil {
		return err
	}
	if !GenerationResolutionValid(origin) || !IntentReviewValid(origin) {
		return exec.ErrBinding
	}
	if err := s.verifyIntentReview(ctx, e, origin); err != nil {
		return err
	}
	if origin.GenerationResolution != nil {
		if err := s.verifyGenerationAncestry(ctx, e, origin); err != nil {
			return err
		}
		pending, err := s.repo.ReadQuery(ctx, mustScope(e), origin.GenerationResolution.QueryID)
		if err != nil {
			return err
		}
		if pending.GenerationPending == nil || !GenerationPendingValid(pending) ||
			exec.Hash(pending.IntentReview) != exec.Hash(origin.IntentReview) {
			return exec.ErrBinding
		}
		// A resolved submission retains its accepted origin after the original
		// answer form expires. checkGenerationPending would incorrectly require
		// an already accepted answer to be submitted again before that expiry.
		if err := s.verifyIntentReview(ctx, e, pending); err != nil {
			return err
		}
	}
	return nil
}

// verifySavedDerivationExecution runs only after gatewayRequirement has authorized the
// consumer. Native validation may inspect the source; this is not a metadata
// admission hook and never invokes a model or executes warehouse result rows.
func (s *Service) verifySavedDerivationExecution(ctx context.Context, e identity.Envelope, q QueryRecord, a admission) error {
	if q.SavedCopyParent == "" {
		return nil
	}
	pairs, _, err := s.savedDerivationParents(ctx, e, q)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if pair.child.SQL == pair.parent.SQL {
			continue
		}
		oldPlan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: pair.parent.SQL, Parameters: pair.parent.Parameters}, a.relationScope)
		if err != nil {
			return err
		}
		childPlan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: pair.child.SQL, Parameters: pair.child.Parameters}, a.relationScope)
		if err != nil {
			return err
		}
		if !oldPlan.Receipt().Validated || !childPlan.Receipt().Validated || !correctionEquivalent(pair.parent.SQL, pair.parent.Parameters, generatedCandidate{SQL: pair.child.SQL, Parameters: pair.child.Parameters}, a.binding, oldPlan, childPlan) {
			return exec.ErrBinding
		}
	}
	return nil
}
