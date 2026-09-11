package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestStoreRuleLifecycleAtomicity(t *testing.T) {
	// An absent caller context is invalid input, not a background authority fallback.
	var missingContext context.Context
	f := newPhase16AcceptanceFixture(t)
	db, ctx := f.f.db, context.Background()
	raw := support.Raw(t, f.f.dsn)
	snapshot := func() string {
		return storeTableSnapshot(t, raw, "topic_rule_draft_heads", "topic_rule_draft_versions", "topic_rule_reviews", "topic_rule_publication_heads", "topic_rule_published_versions", "topic_rule_publication_events", "topic_rule_evidence_invalidations", "audit_events")
	}
	def := phase16RuleDefinition(f.published)
	def.Version = "store-rule-v2"
	subject, err := semantics.NewRuleSubject(phase16PublicPack(f.published), f.published.Digest)
	if err != nil {
		t.Fatal(err)
	}
	model, err := semantics.CompilePublishedRules(subject, def)
	if err != nil {
		t.Fatal(err)
	}
	// These are service-compiled rules, tested directly at the persistence boundary
	// so its own topic pin and review fences cannot be hidden by transport checks.
	for _, tc := range []struct{ table, op string }{{"topic_rule_draft_heads", "UPDATE"}, {"topic_rule_draft_versions", "INSERT"}, {"audit_events", "INSERT"}} {
		t.Run("draft/"+tc.table, func(t *testing.T) {
			before := snapshot()
			remove := storeWriteFault(t, raw, tc.table, tc.op, "")
			_, err := db.SaveRuleDraft(ctx, f.e, f.published, model, 1, "Synthetic change")
			remove()
			if !errors.Is(err, store.ErrUnavailable) || snapshot() != before {
				t.Fatal("failed draft changed rule state", err)
			}
		})
	}
	draft, err := db.SaveRuleDraft(ctx, f.e, f.published, model, 1, "Synthetic change")
	if err != nil {
		t.Fatal(err)
	}
	reviewRequest := rulesets.ReviewRequest{DraftRevision: draft.Revision, Digest: draft.Digest, Decision: "approve", Note: "Synthetic reviewed change"}
	for _, table := range []string{"topic_rule_reviews", "audit_events"} {
		t.Run("review/"+table, func(t *testing.T) {
			before := snapshot()
			remove := storeWriteFault(t, raw, table, "INSERT", "")
			_, err := db.ReviewRuleDraft(ctx, f.e, f.published, f.pack.Topic, reviewRequest)
			remove()
			if !errors.Is(err, store.ErrUnavailable) || snapshot() != before {
				t.Fatal("failed review persisted approval", err)
			}
		})
	}
	review, err := db.ReviewRuleDraft(ctx, f.e, f.published, f.pack.Topic, reviewRequest)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ table, op string }{{"topic_rule_published_versions", "INSERT"}, {"topic_rule_publication_heads", "UPDATE"}, {"topic_rule_publication_events", "INSERT"}, {"topic_rule_evidence_invalidations", "INSERT"}, {"audit_events", "INSERT"}} {
		t.Run("publish/"+tc.table, func(t *testing.T) {
			before := snapshot()
			remove := storeWriteFault(t, raw, tc.table, tc.op, "")
			_, err := db.PublishRules(ctx, f.e, f.published, review.ID, 1)
			remove()
			if !errors.Is(err, store.ErrUnavailable) || snapshot() != before {
				t.Fatal("failed publication changed active rules or invalidated evidence", err)
			}
		})
	}
	// Missing read relations must fail before the reviewed version becomes active.
	for _, table := range []string{"topic_publication_heads", "topic_rule_reviews", "topic_rule_publication_heads"} {
		t.Run("publish/read_failure/"+table, func(t *testing.T) {
			before := snapshot()
			restore := storeHideTable(t, raw, table)
			_, err := db.PublishRules(ctx, f.e, f.published, review.ID, 1)
			restore()
			if !errors.Is(err, store.ErrUnavailable) || snapshot() != before {
				t.Fatal("publication ignored unavailable prerequisite", err)
			}
		})
	}
	published, err := db.PublishRules(ctx, f.e, f.published, review.ID, 1)
	if err != nil || published.State.Version != def.Version {
		t.Fatal("publication after rollback", err)
	}
	for _, tc := range []struct{ table, op string }{{"topic_rule_publication_heads", "UPDATE"}, {"topic_rule_publication_events", "INSERT"}, {"topic_rule_evidence_invalidations", "INSERT"}, {"audit_events", "INSERT"}} {
		t.Run("retire/"+tc.table, func(t *testing.T) {
			before := snapshot()
			remove := storeWriteFault(t, raw, tc.table, tc.op, "")
			_, err := db.RetireRules(ctx, f.e, f.published, "Synthetic retirement", published.State.Revision)
			remove()
			if !errors.Is(err, store.ErrUnavailable) || snapshot() != before {
				t.Fatal("failed retirement removed active rules or added invalidation", err)
			}
		})
	}
	// A stale retained topic is not enough to approve, publish, or retire a rule.
	stale := f.published
	stale.Digest = strings.Repeat("b", 64)
	for _, tc := range []struct {
		name string
		call func() error
		want error
	}{
		{"review/stale_topic", func() error { _, err := db.ReviewRuleDraft(ctx, f.e, stale, f.pack.Topic, reviewRequest); return err }, store.ErrConflict},
		{"retire/stale_topic", func() error { _, err := db.RetireRules(ctx, f.e, stale, "Synthetic retirement", 2); return err }, store.ErrConflict},
		{"review/missing_draft", func() error {
			r := reviewRequest
			r.DraftRevision = 999
			_, err := db.ReviewRuleDraft(ctx, f.e, f.published, f.pack.Topic, r)
			return err
		}, store.ErrNotFound},
		{"review/wrong_digest", func() error {
			r := reviewRequest
			r.Digest = strings.Repeat("b", 64)
			_, err := db.ReviewRuleDraft(ctx, f.e, f.published, f.pack.Topic, r)
			return err
		}, store.ErrConflict},
		{"save/context", func() error {
			_, err := db.SaveRuleDraft(missingContext, f.e, f.published, model, 2, "Synthetic change")
			return err
		}, access.ErrUnauthenticated},
		{"review/context", func() error {
			_, err := db.ReviewRuleDraft(missingContext, f.e, f.published, f.pack.Topic, reviewRequest)
			return err
		}, access.ErrUnauthenticated},
		{"publish/context", func() error { _, err := db.PublishRules(missingContext, f.e, f.published, review.ID, 2); return err }, access.ErrUnauthenticated},
		{"retire/context", func() error {
			_, err := db.RetireRules(missingContext, f.e, f.published, "Synthetic retirement", 2)
			return err
		}, access.ErrUnauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := snapshot()
			if err := tc.call(); !errors.Is(err, tc.want) || snapshot() != before {
				t.Fatal("rejected lifecycle request changed rules", err)
			}
		})
	}
	// Metadata and invalidation readers must not return partial policy state if
	// their protected backing relation is missing.
	for _, tc := range []struct {
		name, table string
		call        func() error
	}{
		{"current_pin", "topic_rule_publication_heads", func() error { _, err := db.RuleVersionPin(ctx, f.e, f.pack.Topic, "", drafts.Read); return err }},
		{"retained_pin", "topic_rule_published_versions", func() error {
			_, err := db.RuleVersionPin(ctx, f.e, f.pack.Topic, def.Version, drafts.Read)
			return err
		}},
		{"invalidations", "topic_rule_evidence_invalidations", func() error { _, err := db.ReadInvalidations(ctx, f.e, f.pack.Topic, 0, 128); return err }},
		{"retained_rules", "topic_rule_published_versions", func() error {
			_, err := db.ReadPublishedRules(ctx, f.e, f.pack.Topic, def.Version, drafts.Read, false)
			return err
		}},
	} {
		t.Run("read_failure/"+tc.name, func(t *testing.T) {
			restore := storeHideTable(t, raw, tc.table)
			err := tc.call()
			restore()
			if !errors.Is(err, store.ErrUnavailable) {
				t.Fatal("missing backing relation was silently accepted", err)
			}
		})
	}
	retired, err := db.RetireRules(ctx, f.e, f.published, "Synthetic retirement", 2)
	if err != nil || !retired.Retired {
		t.Fatal(err)
	}
	if _, err := db.RetireRules(ctx, f.e, f.published, "Duplicate retirement", 3); !errors.Is(err, store.ErrConflict) {
		t.Fatal("retired pointer accepted a second retirement", err)
	}
}
