package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// NLQFeedbackSource is the production adapter over reviewed learning records.
// It preserves their existing state and exact origin; it never promotes a split.
type NLQFeedbackSource struct{ Service *nlqexec.Service }

// ReviewedFeedback returns only active independently reviewed examples.
func (a NLQFeedbackSource) ReviewedFeedback(ctx context.Context, e identity.Envelope, topic string, limit int) ([]FeedbackEvidence, error) {
	if a.Service == nil {
		return nil, ErrInvalid
	}
	rows, err := a.Service.Examples(ctx, e, topic, limit)
	if err != nil {
		return nil, err
	}
	out := make([]FeedbackEvidence, 0, len(rows))
	for _, r := range rows {
		if r.State != "active" || r.ReviewedAt == nil {
			continue
		}
		sum := sha256.Sum256([]byte(r.Question))
		out = append(out, FeedbackEvidence{ID: r.ID, Locale: string(r.Origin.Locale), InputDigest: hex.EncodeToString(sum[:]), ExpectedDigest: r.Digest, Decision: "generated_sql", SourceBindingDigest: r.Origin.SourceBindingDigest, CreatedAt: r.Created})
	}
	return out, nil
}
