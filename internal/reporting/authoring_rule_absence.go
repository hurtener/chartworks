package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
)

// AuthoringRuleAbsence returns a detached server-owned origin fence. A prepared
// definition cannot silently rebase to a different topic or adopt rule policies
// that its finite compiler did not implement. Legacy revisions stay unchanged.
func AuthoringRuleAbsence(r Revision) ([]TopicPin, error) {
	pins := r.Provenance.RuleAbsence
	if len(pins) == 0 {
		return nil, nil
	}
	if len(pins) != 1 || len(r.Definition.Topics) != 1 || pins[0] != r.Definition.Topics[0] || len(r.Definition.Rules) != 0 || !identity.Identifier(pins[0].Topic) || !identity.Identifier(pins[0].Version) || !hashValid(pins[0].Digest) {
		return nil, ErrStale
	}
	return clone(pins), nil
}

// RetainAuthoringRuleAbsence prevents internal derivative writes from stripping
// the origin fence. Copy and restore clone the selected revision's provenance;
// unrelated legacy blocks do not acquire an absence policy.
func RetainAuthoringRuleAbsence(before, after Revision) error {
	a, err := AuthoringRuleAbsence(before)
	if err != nil {
		return err
	}
	b, err := AuthoringRuleAbsence(after)
	if err != nil || digest(a) != digest(b) {
		return ErrStale
	}
	return nil
}

func (s *Service) checkAuthoringRuleAbsence(ctx context.Context, e identity.Envelope, r Revision) error {
	pins, err := AuthoringRuleAbsence(r)
	if err != nil || len(pins) == 0 {
		return err
	}
	for _, pin := range pins {
		reason, err := s.authoringRulesDisposition(ctx, e, pin)
		if err != nil {
			return err
		}
		if reason != "" {
			return ErrStale
		}
	}
	return nil
}
