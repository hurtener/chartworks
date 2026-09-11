package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// RecordAutopilotSchedule verifies the actual ordinary schedule and effect key
// before declaring the multi-object proposal applied.
func (d *DB) RecordAutopilotSchedule(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, result jobs.Schedule) (out engineering.AutopilotProposal, err error) {
	v, err := proof.Checked(e, "engineering.autopilot.apply")
	if err != nil {
		return out, err
	}
	g := v.Material.Request.Schedule
	if g == nil || !identity.Identifier(result.ID) {
		return out, store.ErrInvalid
	}
	refs := []access.Resource{access.Tenant(e, "write"), {Tenant: e.Tenant(), Kind: "execution_binding", Permission: "use", ID: g.BindingID}}
	if g.ID != "" {
		refs = append(refs, access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "write", ID: g.ID})
	}
	if err = access.Require(e, "scheduling.write", refs...); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := proposalApplyTx(ctx, tx, e, v)
		if err != nil {
			return err
		}
		if p.State != "applying" && p.State != "applied" || p.ApplyActor != e.User() || p.ApplySession != e.Session() {
			return store.ErrConflict
		}
		run, err := readPipelineExecutionTx(ctx, tx, e, p.Operation)
		if err != nil {
			return err
		}
		if run.State != "published" || run.Operation.State != "succeeded" {
			return store.ErrConflict
		}
		if p.Material.Topic != nil {
			found := false
			for _, effect := range p.Effects {
				if effect.Kind == "topic_draft" && effect.State == "committed" {
					found = true
				}
			}
			if !found {
				return store.ErrConflict
			}
		}
		actual, err := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR SHARE`, e.Tenant(), result.ID))
		if err != nil {
			return err
		}
		want := engineering.ProposalScheduleRequest(p)
		if actual.Revision != g.ExpectedRevision+1 || digestValue(actual.Request) != digestValue(want) || result.Revision != actual.Revision || digestValue(result.Request) != digestValue(want) {
			return store.ErrConflict
		}
		var same bool
		key := engineering.ProposalScheduleKey(p)
		if g.ID == "" {
			err = tx.QueryRow(ctx, `SELECT creator_id=$3 AND creator_session=$4 AND client_key=$5 FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2`, e.Tenant(), actual.ID, e.User(), e.Session(), key).Scan(&same)
		} else {
			if actual.ID != g.ID {
				return store.ErrConflict
			}
			err = tx.QueryRow(ctx, `SELECT change_actor=$3 AND change_session=$4 AND change_key=$5 AND change_revision=revision FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2`, e.Tenant(), actual.ID, e.User(), e.Session(), key).Scan(&same)
		}
		if err != nil {
			return err
		}
		if !same {
			return store.ErrConflict
		}
		changed, err := proposalEffectTx(ctx, tx, e, p, engineering.ProposalEffect{Kind: "schedule", Target: actual.ID, State: "committed", Version: actual.Revision, Digest: digestValue(want)})
		if err != nil {
			return err
		}
		if changed || p.State != "applied" {
			if _, err = tx.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET version=version+1,state='applied',applied_at=COALESCE(applied_at,clock_timestamp()),updated_at=clock_timestamp() WHERE tenant_id=$1 AND proposal_id=$2`, e.Tenant(), p.ID); err != nil {
				return err
			}
			if err = proposalEventTx(ctx, tx, e, p.ID, p.Version+1, p.Revision, "engineering.proposal_applied", map[string]any{"schedule": actual.ID, "schedule_revision": actual.Revision}); err != nil {
				return err
			}
		}
		out, err = proposalTx(ctx, tx, e, p.ID, "engineering.autopilot.apply", false)
		return err
	})
	if err != nil {
		return engineering.AutopilotProposal{}, err
	}
	return out, nil
}
