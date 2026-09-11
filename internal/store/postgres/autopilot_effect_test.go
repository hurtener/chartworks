package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestProposalEffectReplayRequiresReadableEvidence(t *testing.T) {
	effect := engineering.ProposalEffect{Kind: "pipeline_draft", Target: "pipeline", State: "committed", Version: 1, Digest: "synthetic-digest", Observed: time.Now()}
	prior := effect
	prior.Observed = prior.Observed.Add(-time.Hour)
	payload, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	marker := errors.New("synthetic effect receipt failure")
	for _, tc := range []struct {
		name string
		row  blockValidationRow
		kind string
		want error
	}{
		{"storage failure", blockValidationRow{err: marker}, effect.Kind, marker},
		{"malformed receipt", blockValidationRow{body: []byte(`{`)}, effect.Kind, store.ErrInvalid},
		{"same effect new observation", blockValidationRow{body: payload}, effect.Kind, nil},
		{"unknown effect kind", blockValidationRow{err: pgx.ErrNoRows}, "unregistered-effect", store.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := effect
			candidate.Kind = tc.kind
			// Any unexpected write panics through the embedded absent transaction.
			changed, err := proposalEffectTx(context.Background(), blockValidationTx{row: tc.row}, identity.Envelope{}, engineering.AutopilotProposal{ID: "proposal", Revision: 1}, candidate)
			if !errors.Is(err, tc.want) || changed {
				t.Fatal("effect receipt replay", changed, err)
			}
		})
	}
}

type proposalEffectFailureTx struct {
	blockValidationTx
	failure error
	writes  int
}

func (tx *proposalEffectFailureTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.writes++
	return pgconn.CommandTag{}, tx.failure
}
func TestProposalEffectWriteFailureDoesNotClaimCommit(t *testing.T) {
	marker := errors.New("synthetic effect write failure")
	prior := engineering.ProposalEffect{Kind: "pipeline_run", Target: "pipeline", State: "running", Version: 1}
	raw, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		row  blockValidationRow
	}{
		{"new receipt", blockValidationRow{err: pgx.ErrNoRows}},
		{"updated receipt", blockValidationRow{body: raw}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &proposalEffectFailureTx{blockValidationTx: blockValidationTx{row: tc.row}, failure: marker}
			effect := prior
			effect.State = "succeeded"
			changed, err := proposalEffectTx(context.Background(), tx, identity.Envelope{}, engineering.AutopilotProposal{ID: "proposal", Revision: 1}, effect)
			if changed || !errors.Is(err, marker) || tx.writes != 1 {
				t.Fatal("failed effect write claimed commit", changed, err, tx.writes)
			}
		})
	}
}
