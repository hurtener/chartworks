package postgres

import (
	"context"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.EffectDependencyRepository = (*DB)(nil)

// Discovery reads only native metadata indexes and selected definition identity
// fields. No SQL, values, manifest payload, model or warehouse call is involved.
func (d *DB) DiscoverAuthoringEffectDependencies(ctx context.Context, e identity.Envelope, in reporting.EffectDependencyRequest) (out reporting.EffectDependencyManifest, err error) {
	if err = reporting.RequireEffectDependencyDiscovery(e, in); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	out = reporting.EffectDependencyManifest{Version: "report-effect-dependencies-v1", Operation: in.Operation, Kind: "report", ID: in.ID, Revision: in.Revision, Actions: []string{"reporting.read"}, References: []reporting.ResourceReference{}}
	if in.Operation == "block_validate" || in.Operation == "block_run" || in.Operation == "block_view" {
		out.Kind = "block"
	}
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		if in.Operation == "block_view" {
			return blockRunDependenciesTx(ctx, tx, e, in.ID, &out)
		}
		if in.Operation == "execute" || in.Operation == "view" {
			out.Run = in.ID
			// Execution/private artifacts belong to the original canonical login.
			// Public retained reads need independent parent policy in Pengui.
			err := tx.QueryRow(ctx, `SELECT c.document_id,c.revision,c.definition_digest,c.private
 FROM chartworks.composition_runs c WHERE c.tenant_id=$1 AND c.operation_id=$2 AND c.kind='report'
 AND (NOT c.private OR (c.actor_id=$3 AND c.session_id=$4))
 AND ($5='view' OR (c.private AND c.actor_id=$3 AND c.session_id=$4))
 AND NOT EXISTS(SELECT 1 FROM chartworks.composition_run_groups g WHERE g.tenant_id=c.tenant_id AND g.operation_id=c.operation_id AND g.kind<>'block')`, e.Tenant(), in.ID, e.User(), e.Session(), in.Operation).Scan(&out.ID, &out.Revision, &out.Digest, &out.Private)
			if err != nil {
				return err
			}
			rows, err := tx.Query(ctx, `SELECT DISTINCT action,kind,permission,resource_id FROM chartworks.composition_run_references WHERE tenant_id=$1 AND operation_id=$2 ORDER BY action,kind,permission,resource_id LIMIT 257`, e.Tenant(), in.ID)
			if err != nil {
				return err
			}
			native, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (r reporting.CompositionReference, err error) {
				err = row.Scan(&r.Action, &r.Kind, &r.Permission, &r.ID)
				return
			})
			if err != nil {
				return err
			}
			if len(native) == 0 || len(native) > 256 {
				return store.ErrInvalid
			}
			for _, r := range native {
				if !slices.Contains([]string{"reporting.read", "reporting.execute", "reporting.preview", "sources.query"}, r.Action) {
					return store.ErrInvalid
				}
				ref := reporting.ResourceReference{Kind: r.Kind, Permission: r.Permission, ID: r.ID}
				if in.Operation == "view" {
					// Retained consumption has no execute/source-query authority.
					if ref.Permission == "execute" || ref.Kind == "source" && ref.Permission == "query" {
						ref.Permission = "read"
					}
				} else {
					out.Actions = append(out.Actions, r.Action)
				}
				out.References = append(out.References, ref)
			}
			out.References = append(out.References, reporting.ResourceReference{Kind: "report", Permission: "read", ID: out.ID})
			if out.Private {
				out.Actions = append(out.Actions, "reporting.preview")
				out.References = append(out.References, reporting.ResourceReference{Kind: "report", Permission: "preview", ID: out.ID})
			}
			if in.Operation == "view" {
				return nil
			}
		}
		base, err := reportDependenciesTx(ctx, tx, e, reporting.DependencyRequest{Kind: out.Kind, ID: out.ID, Revision: out.Revision}, out.Run != "")
		if err != nil {
			return err
		}
		if out.Run != "" && base.Digest != out.Digest {
			return store.ErrInvalid
		}
		if out.Run == "" {
			out.Digest, out.Private = base.Digest, base.Private
		}
		publicRun := in.Operation == "block_run" || in.Operation == "report_run"
		if publicRun && base.Private {
			return access.ErrNotFound
		}
		for _, b := range base.Blocks {
			if publicRun && b.Private {
				return access.ErrNotFound
			}
		}
		out.References = append(out.References, base.References...)
		out.References = append(out.References, reporting.ResourceReference{Kind: out.Kind, Permission: "read", ID: out.ID})
		if !publicRun {
			out.References = append(out.References, reporting.ResourceReference{Kind: out.Kind, Permission: "write", ID: out.ID})
		}
		if out.Private {
			out.Actions = append(out.Actions, "reporting.preview")
			out.References = append(out.References, reporting.ResourceReference{Kind: out.Kind, Permission: "preview", ID: out.ID})
		}
		if publicRun {
			out.Actions = append(out.Actions, "reporting.execute")
			out.References = append(out.References, reporting.ResourceReference{Kind: out.Kind, Permission: "execute", ID: out.ID})
		} else if in.Operation == "block_validate" {
			out.Actions = append(out.Actions, "reporting.validate")
		} else {
			out.Actions = append(out.Actions, "reporting.write", "reporting.execute", "reporting.preview")
			out.References = append(out.References, reporting.ResourceReference{Kind: "report", Permission: "execute", ID: out.ID}, reporting.ResourceReference{Kind: "report", Permission: "preview", ID: out.ID})
		}
		for _, b := range base.Blocks {
			var source, partition string
			var narrative bool
			if err := tx.QueryRow(ctx, `SELECT definition->>'source',definition->>'context',jsonb_path_exists(definition,'$.outputs[*] ? (@.kind == "narrative")') FROM chartworks.block_revisions WHERE tenant_id=$1 AND block_id=$2 AND revision=$3 AND digest=$4`, e.Tenant(), b.ID, b.Revision, b.Digest).Scan(&source, &partition, &narrative); err != nil {
				return err
			}
			if !identity.Identifier(source) || !identity.Identifier(partition) {
				return store.ErrInvalid
			}
			if !slices.Contains(base.References, reporting.ResourceReference{Kind: "source", Permission: "read", ID: source}) || !slices.Contains(base.References, reporting.ResourceReference{Kind: "execution_context", Permission: "use", ID: partition}) {
				return store.ErrInvalid
			}
			// This manual lane never infers model permission from report access.
			if narrative && in.Operation != "block_validate" {
				return reporting.ErrInvalid
			}
			out.Actions = append(out.Actions, "sources.read", "topics.read", "sources.query")
			out.References = append(out.References, reporting.ResourceReference{Kind: "source", Permission: "query", ID: source})
			if in.Operation != "block_validate" {
				out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "execute", ID: b.ID})
				// A private composition's child runs retain preview privacy even
				// when their block definition has already been published.
				if !publicRun {
					out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "preview", ID: b.ID})
				}
			}
		}
		return nil
	})
	if err != nil {
		return reporting.EffectDependencyManifest{}, err
	}
	slices.Sort(out.Actions)
	out.Actions = slices.Compact(out.Actions)
	slices.SortFunc(out.References, func(a, b reporting.ResourceReference) int {
		return slices.Compare([]string{a.Kind, a.Permission, a.ID}, []string{b.Kind, b.Permission, b.ID})
	})
	out.References = slices.Compact(out.References)
	if len(out.References) > 128 {
		return reporting.EffectDependencyManifest{}, store.ErrInvalid
	}
	if !e.Valid() {
		return reporting.EffectDependencyManifest{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

// A frozen block run retains its exact original partition and immutable block
// revision requirements. No current publication or payload is used for discovery.
func blockRunDependenciesTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, run string, out *reporting.EffectDependencyManifest) error {
	var source, partition string
	err := tx.QueryRow(ctx, `SELECT block_id,revision,revision_digest,private,source_id,context_id FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2 AND (NOT private OR (actor_id=$3 AND session_id=$4))`, e.Tenant(), run, e.User(), e.Session()).Scan(&out.ID, &out.Revision, &out.Digest, &out.Private, &source, &partition)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT kind,permission,resource_id FROM chartworks.block_revision_references WHERE tenant_id=$1 AND block_id=$2 AND revision=$3 ORDER BY kind,permission,resource_id LIMIT 129`, e.Tenant(), out.ID, out.Revision)
	if err != nil {
		return err
	}
	refs, err := dependencyReferences(rows)
	if err != nil {
		return err
	}
	if len(refs) == 0 || !identity.Identifier(source) || !identity.Identifier(partition) {
		return store.ErrInvalid
	}
	out.Run = run
	out.References = append(refs, reporting.ResourceReference{Kind: "block", Permission: "read", ID: out.ID}, reporting.ResourceReference{Kind: "source", Permission: "read", ID: source}, reporting.ResourceReference{Kind: "execution_context", Permission: "use", ID: partition})
	if out.Private {
		out.Actions = append(out.Actions, "reporting.preview")
		out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "preview", ID: out.ID})
	}
	return nil
}
