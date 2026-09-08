package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// TestPhase16PostgresRuleStoreBoundaries exercises the persisted rule-evidence
// boundaries directly against the disposable PostgreSQL store. The normal
// phase acceptance proves the happy lifecycle; these cases keep stale pins,
// dependency reach, and immutable comparison identity fail-closed at the
// repository seam as well.
func TestPhase16PostgresRuleStoreBoundaries(t *testing.T) {
	fixture := newPhase16AcceptanceFixture(t)
	ctx := context.Background()
	topic := fixture.pack.Topic
	dependency := fixture.pack.Datasets[0].Source
	dataset := fixture.pack.Datasets[0].ID

	t.Run("invalidation reauthorizes every persisted dependency", func(t *testing.T) {
		initial, err := fixture.f.db.ReadInvalidations(ctx, fixture.e, topic, 0, 8)
		if err != nil || len(initial) != 1 {
			t.Fatalf("initial invalidation read: %#v %v", initial, err)
		}
		for _, tc := range []struct {
			name   string
			source string
			data   string
			ctx    string
		}{
			{name: "source", source: "unreachable-source", data: dataset, ctx: dependency.Context},
			{name: "dataset", source: dependency.Source, data: "unreachable-dataset", ctx: dependency.Context},
			{name: "context", source: dependency.Source, data: dataset, ctx: "unreachable-context"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				narrow := fixture.f.token.envelope(t, fixture.e.Tenant(), fixture.e.User(),
					"topics.read",
					"cw.topic.read:"+topic,
					"cw.source.read:"+tc.source,
					"cw.dataset.query:"+tc.data,
					"cw.execution_context.use:"+tc.ctx,
				)
				_, err := fixture.f.db.ReadInvalidations(ctx, narrow, topic, 0, 8)
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("invalidation returned data without persisted dependency reach: %v", err)
				}
			})
		}
		foreign := fixture.f.token.envelope(t, "foreign-tenant", fixture.e.User(),
			"topics.read", "cw.topic.read:"+topic, "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*")
		got, err := fixture.f.db.ReadInvalidations(ctx, foreign, topic, 0, 8)
		if err != nil || len(got) != 0 {
			t.Fatalf("foreign tenant crossed the evidence partition: %#v %v", got, err)
		}
	})

	t.Run("stale retained pins and missing current versions conflict", func(t *testing.T) {
		stale := fixture.published
		stale.Digest = strings.Repeat("f", 64)
		if _, err := fixture.f.db.PublishRules(ctx, fixture.e, stale, "stale-review", 1); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("stale current topic pin reached publication: %v", err)
		}
		if _, err := fixture.f.db.RetireRules(ctx, fixture.e, stale, "stale-retirement", 1); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("stale retained topic pin reached retirement: %v", err)
		}
		if _, err := fixture.f.db.ReadPublishedRules(ctx, fixture.e, topic, "missing-rules", drafts.Read, true); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("missing current rule version was not a conflict: %v", err)
		}
	})

	t.Run("comparison validation and duplicate identity", func(t *testing.T) {
		comparison := rulesets.Comparison{
			ID:         strings.Repeat("a", 32),
			Mode:       "replay",
			Topic:      topic,
			References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}},
			Baseline: rulesets.Evaluation{
				Topic:        topic,
				TopicVersion: fixture.pack.Version,
				RuleVersion:  "rules-v1",
				PackDigest:   fixture.published.Digest,
				Result:       semantics.ConstraintEvaluation{Allowed: true},
			},
		}
		saved, err := fixture.f.db.RecordComparison(ctx, fixture.e, comparison)
		if err != nil || saved.ID != comparison.ID || saved.CreatedAt.IsZero() {
			t.Fatalf("valid replay evidence was not persisted: %#v %v", saved, err)
		}
		if _, err = fixture.f.db.RecordComparison(ctx, fixture.e, comparison); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate comparison identity was not idempotently fenced: %v", err)
		}
		invalidReplay := comparison
		invalidReplay.ID = strings.Repeat("b", 32)
		invalidReplay.Candidate = &rulesets.Evaluation{RuleVersion: "rules-v2"}
		if _, err = fixture.f.db.RecordComparison(ctx, fixture.e, invalidReplay); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("replay accepted candidate evidence: %v", err)
		}
		invalidShadow := comparison
		invalidShadow.ID = strings.Repeat("c", 32)
		invalidShadow.Mode = "shadow"
		if _, err = fixture.f.db.RecordComparison(ctx, fixture.e, invalidShadow); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("shadow accepted missing candidate evidence: %v", err)
		}
	})
}

// TestPhase16PostgresRejectsCorruptRetainedRuleDefinition verifies that a
// persisted object which has bypassed the immutable-row guard is still rejected
// by the read decoder. The trigger is disabled only for this disposable test
// row and is restored, together with the exact original bytes, before return.
func TestPhase16PostgresRejectsCorruptRetainedRuleDefinition(t *testing.T) {
	fixture := newPhase16AcceptanceFixture(t)
	ctx := context.Background()
	raw := support.Raw(t, fixture.f.dsn)
	topic := fixture.pack.Topic
	const trigger = "topic_rule_published_version_immutable"
	const table = "chartworks.topic_rule_published_versions"
	var original []byte
	if err := raw.QueryRow(ctx, `SELECT definition FROM `+table+` WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, fixture.e.Tenant(), topic, "rules-v1").Scan(&original); err != nil {
		t.Fatal("snapshot published rule definition", err)
	}
	restore := func() error {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := raw.Exec(restoreCtx, `ALTER TABLE `+table+` DISABLE TRIGGER `+trigger); err != nil {
			return err
		}
		if _, err := raw.Exec(restoreCtx, `UPDATE `+table+` SET definition=$1::jsonb WHERE tenant_id=$2 AND topic_id=$3 AND version_id=$4`, original, fixture.e.Tenant(), topic, "rules-v1"); err != nil {
			return err
		}
		_, err := raw.Exec(restoreCtx, `ALTER TABLE `+table+` ENABLE TRIGGER `+trigger)
		return err
	}
	t.Cleanup(func() {
		if err := restore(); err != nil {
			t.Errorf("restore immutable rule fixture: %v", err)
		}
	})
	if _, err := raw.Exec(ctx, `ALTER TABLE `+table+` DISABLE TRIGGER `+trigger); err != nil {
		t.Fatal("prepare corrupt retained rule fixture", err)
	}
	if _, err := raw.Exec(ctx, `UPDATE `+table+` SET definition='{"topic":"tampered-topic"}'::jsonb WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, fixture.e.Tenant(), topic, "rules-v1"); err != nil {
		t.Fatal("persist corrupt retained rule fixture", err)
	}
	if _, err := raw.Exec(ctx, `ALTER TABLE `+table+` ENABLE TRIGGER `+trigger); err != nil {
		t.Fatal("restore immutable rule guard", err)
	}
	if _, err := fixture.f.db.ReadPublishedRules(ctx, fixture.e, topic, "rules-v1", drafts.Read, false); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("corrupt retained rule definition was returned: %v", err)
	}
	if err := restore(); err != nil {
		t.Fatal("restore retained rule definition", err)
	}
	if _, err := fixture.f.db.ReadPublishedRules(ctx, fixture.e, topic, "rules-v1", drafts.Read, false); err != nil {
		t.Fatalf("restored retained rule definition was not readable: %v", err)
	}
}
