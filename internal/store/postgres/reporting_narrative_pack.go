package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.NarrativePackSelector = (*DB)(nil)

// SelectedNarrativePack reads the current production selection and its exact
// accepted runtime material in one transaction after signed run reach is checked.
func (d *DB) SelectedNarrativePack(ctx context.Context, e identity.Envelope, m reporting.RunManifest) (out reporting.NarrativeRuntime, err error) {
	if err = reporting.Require(e, m.Block, reporting.Execute); err != nil {
		return out, err
	}
	if err = reporting.RequireRunManifest(e, m); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var readErr error
		out, readErr = selectedNarrativePackTx(ctx, tx, e.Tenant())
		return readErr
	})
	return out, err
}

func selectedNarrativePackTx(ctx context.Context, tx pgx.Tx, tenant string) (reporting.NarrativeRuntime, error) {
	var raw, reviewRaw, proposalRaw, proposalReviewRaw []byte
	var state, runtimeDigest, configDigest, packDigest, proposalID, proposalDigest, proposalState, proposalAuthor, selectionActor string
	var selectedAt time.Time
	err := tx.QueryRow(ctx, `SELECT p.material,p.review,p.state,p.runtime_digest,p.configuration_digest,s.pack_digest,
 s.proposal_id,s.actor_id,s.selected_at,q.proposal,q.proposal_digest,q.state,q.review,q.author_id
 FROM chartworks.evaluation_pack_selection s
 JOIN chartworks.evaluation_runtime_packs p ON (p.tenant_id,p.pack_digest)=(s.tenant_id,s.pack_digest)
 JOIN chartworks.evaluation_proposals q ON (q.tenant_id,q.proposal_id)=(s.tenant_id,s.proposal_id)
 WHERE s.tenant_id=$1 FOR SHARE OF s,p,q`, tenant).
		Scan(&raw, &reviewRaw, &state, &runtimeDigest, &configDigest, &packDigest,
			&proposalID, &selectionActor, &selectedAt, &proposalRaw, &proposalDigest, &proposalState, &proposalReviewRaw, &proposalAuthor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reporting.NarrativeRuntime{}, store.ErrNotFound
		}
		return reporting.NarrativeRuntime{}, err
	}
	if state != string(evaluation.Accepted) {
		return reporting.NarrativeRuntime{}, store.ErrConflict
	}
	var proposal evaluation.OptimizationProposal
	var proposalReview evaluation.ReviewReceipt
	if json.Unmarshal(proposalRaw, &proposal) != nil || json.Unmarshal(proposalReviewRaw, &proposalReview) != nil {
		return reporting.NarrativeRuntime{}, store.ErrMigration
	}
	computedProposalDigest, digestErr := digestJSON(proposal)
	if digestErr != nil || proposal.Validate() != nil || proposal.Mode != evaluation.Live ||
		proposal.ID != proposalID || computedProposalDigest != proposalDigest ||
		proposal.Candidate.PackDigest != packDigest || proposalState != "approve" ||
		proposalReview.ProposalID != proposalID || proposalReview.ProposalDigest != proposalDigest ||
		proposalReview.Decision != "approve" || proposalReview.Reviewer == "" ||
		proposalReview.Reviewer == proposalAuthor || proposalReview.ReviewedAt.IsZero() ||
		selectionActor == "" || selectedAt.IsZero() {
		return reporting.NarrativeRuntime{}, store.ErrConflict
	}
	var record evaluation.RuntimePackRecord
	if json.Unmarshal(raw, &record) != nil || json.Unmarshal(reviewRaw, &record.Review) != nil {
		return reporting.NarrativeRuntime{}, store.ErrMigration
	}
	record.State = evaluation.Lifecycle(state)
	if record.Validate() != nil || record.State != evaluation.Accepted || record.Review == nil ||
		record.Review.Reviewer == record.Author || record.Digest != runtimeDigest ||
		record.Config.Digest != configDigest || record.Pack.Digest != packDigest {
		return reporting.NarrativeRuntime{}, store.ErrConflict
	}
	model := ""
	for _, binding := range record.Config.Models {
		if binding.Role == "narrative" {
			model = binding.Model
			break
		}
	}
	out := reporting.NarrativeRuntime{Pin: reporting.NarrativePackPin{
		PackDigest: record.Pack.Digest, RuntimeDigest: record.Digest,
		ConfigurationDigest: record.Config.Digest, Model: model}, Config: record.Config}
	if !out.Valid() {
		return reporting.NarrativeRuntime{}, store.ErrConflict
	}
	return out, nil
}
