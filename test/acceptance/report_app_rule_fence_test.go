package acceptance

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/test/support"
)

// The rule publisher holds its native topic SHARE lock before the block
// transition starts. Its first active rule remains invisible at the transition's
// metadata read, so only the later exclusive topic-head fence closes this race.
func TestReportAppPreparedRulePublicationFence(t *testing.T) {
	for _, transition := range []string{"publish", "certify"} {
		t.Run(transition, func(t *testing.T) {
			const target = "dataset-rule-publication-race"
			f, s, author, request, publication, _, scopes := reportDatasetFixture(t, target)
			ctx := t.Context()
			prepared, err := s.PrepareDatasetChart(ctx, author, request)
			if err != nil || prepared.Status != "prepared" {
				t.Fatal(prepared, err)
			}
			created, err := s.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: target, Preparation: prepared.Preparation, Digest: prepared.Digest})
			if err != nil {
				t.Fatal(err)
			}
			validation, err := f.blocks.Validate(ctx, author, target, reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision})
			if err != nil {
				t.Fatal(err)
			}
			actor := phase27Actor(t, f.f, author.User(), append(slices.Clone(scopes), "reporting.publish", "reporting.certify", "cw.block.publish:"+target, "cw.block.certify:"+target))
			beforeState := validation.State
			if transition == "certify" {
				beforeState, err = f.blocks.Publish(ctx, actor, target, reporting.PublishRequest{ExpectedVersion: validation.State.Version, Evidence: validation.Evidence.ID})
				if err != nil {
					t.Fatal(err)
				}
			}
			rules, err := rulesets.New(f.f.f.db, f.f.f.db, f.f.f.db)
			if err != nil {
				t.Fatal(err)
			}
			definition := semantics.RuleSetDefinition{SchemaVersion: semantics.SchemaVersion, ID: "prepared-origin-rules", Version: "rules-v1", Topic: publication.State.Topic, TopicVersion: publication.State.Version, PackDigest: publication.Digest,
				Rules: []semantics.RuleDefinition{{ID: "require-reviewed-dataset", Version: "v1", Category: semantics.RuleStructural, Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 100, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "Synthetic concurrent review"}, Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: semantics.Reference{Kind: semantics.KindDataset, ID: request.Intent.Dataset}}}}}
			draft, err := rules.Save(ctx, f.blockAuthor, rulesets.SaveRequest{Definition: definition, Change: "Review first rule activation"})
			if err != nil {
				t.Fatal(err)
			}
			review, err := rules.Review(ctx, f.blockAuthor, publication.State.Topic, rulesets.ReviewRequest{DraftRevision: draft.Revision, Digest: draft.Digest, Decision: "approve", Note: "Approve synthetic constraint"})
			if err != nil {
				t.Fatal(err)
			}
			raw := support.Raw(t, f.f.f.dsn)
			if _, err := raw.Exec(ctx, `CREATE FUNCTION chartworks.test_prepared_rule_barrier() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(738294); RETURN NEW; END $$;
CREATE TRIGGER test_prepared_rule_barrier BEFORE UPDATE ON chartworks.topic_rule_publication_heads FOR EACH ROW EXECUTE FUNCTION chartworks.test_prepared_rule_barrier(); SELECT pg_advisory_lock(738294)`); err != nil {
				t.Fatal(err)
			}
			rulesFinished, blockFinished := make(chan struct{}), make(chan struct{})
			rulesResult, blockResult := make(chan error, 1), make(chan error, 1)
			blockStarted := false
			defer func() {
				_, _ = raw.Exec(context.Background(), `SELECT pg_advisory_unlock(738294)`)
				<-rulesFinished
				if blockStarted {
					<-blockFinished
				}
				_, _ = raw.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_prepared_rule_barrier ON chartworks.topic_rule_publication_heads; DROP FUNCTION IF EXISTS chartworks.test_prepared_rule_barrier()`)
			}()
			go func() {
				defer close(rulesFinished)
				_, err := rules.Publish(ctx, f.blockAuthor, publication.State.Topic, rulesets.PublishRequest{Review: review.ID})
				rulesResult <- err
			}()
			waitLocked := func(pattern string, early <-chan error) {
				t.Helper()
				until := time.Now().Add(2 * time.Second)
				for time.Now().Before(until) {
					select {
					case err := <-early:
						t.Fatal("transition bypassed its expected fence", err)
					default:
					}
					var waiting bool
					if err := raw.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)`, pattern).Scan(&waiting); err != nil {
						t.Fatal(err)
					}
					if waiting {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Fatal("native transaction did not reach expected lock")
			}
			waitLocked("UPDATE chartworks.topic_rule_publication_heads%", rulesResult)
			beforeReads := f.attemptCount(t)
			blockStarted = true
			go func() {
				defer close(blockFinished)
				var err error
				if transition == "publish" {
					_, err = f.blocks.Publish(ctx, actor, target, reporting.PublishRequest{ExpectedVersion: beforeState.Version, Evidence: validation.Evidence.ID})
				} else {
					_, err = f.blocks.Certify(ctx, actor, target, reporting.CertifyRequest{ExpectedVersion: beforeState.Version, Revision: created.Block.Revision, Evidence: validation.Evidence.ID, Note: "Explicit synthetic certification"})
				}
				blockResult <- err
			}()
			waitLocked("%topic_publication_heads%FOR UPDATE OF h%", blockResult)
			if _, err := raw.Exec(ctx, `SELECT pg_advisory_unlock(738294)`); err != nil {
				t.Fatal(err)
			}
			if err := <-rulesResult; err != nil {
				t.Fatal("first rule publication failed", err)
			}
			if err := <-blockResult; !errors.Is(err, reporting.ErrStale) {
				t.Fatal("rule activation bypassed native block fence", err)
			}
			current, err := f.f.f.db.ReadBlock(ctx, actor, target, reporting.Reference{Revision: created.Block.Revision}, reporting.Read)
			if err != nil || current.State.Version != beforeState.Version || current.Attestation != nil || transition == "publish" && current.PublishedAt != nil || f.attemptCount(t) != beforeReads {
				t.Fatal("stale lifecycle transition committed or queried", err)
			}
		})
	}
}
