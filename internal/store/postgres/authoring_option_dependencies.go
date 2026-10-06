package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.OptionDependencyRepository = (*DB)(nil)

func (d *DB) DiscoverAuthoringOptionDependencies(ctx context.Context, e identity.Envelope, in reporting.OptionDependencyRequest) (out reporting.OptionDependencyManifest, err error) {
	if err = reporting.RequireOptionDependencyDiscovery(e, in); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	out = reporting.OptionDependencyManifest{Version: "report-option-dependencies-v1", Target: in.Target, Operation: in.Operation, Actions: []string{"reporting.read", "sources.read", "sources.query", "topics.read"}, References: []reporting.ResourceReference{}}
	// Query only the coordinates of the original attempt. Tenant, actor, canonical
	// login, operation and the complete target digest are predicates before read.
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		var refs, blocks []byte
		var source, partition, dataset string
		err := tx.QueryRow(ctx, `SELECT record->'references',record->'blocks',source_id,record->>'context',record->>'dataset'
 FROM chartworks.authoring_option_operations WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND operation_id=$4 AND target_digest=$5`, e.Tenant(), e.User(), e.Session(), in.Operation, readexec.Hash(in.Target)).Scan(&refs, &blocks, &source, &partition, &dataset)
		if err != nil {
			return err
		}
		var originalBlocks []reporting.AuthoringOptionBlock
		if json.Unmarshal(refs, &out.References) != nil || json.Unmarshal(blocks, &originalBlocks) != nil || len(out.References) < 1 || len(out.References) > 128 || len(originalBlocks) > 100 || !identity.Identifier(source) || !identity.Identifier(partition) || !identity.Identifier(dataset) {
			return store.ErrInvalid
		}
		out.References = append(out.References, dataQueryReferences(source, partition, dataset)...)
		for _, b := range originalBlocks {
			if !identity.Identifier(b.Block) || !slices.Contains([]string{"published", "private_preview"}, b.Policy) {
				return store.ErrInvalid
			}
			out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "read", ID: b.Block}, reporting.ResourceReference{Kind: "block", Permission: "execute", ID: b.Block})
			if b.Policy == "private_preview" {
				out.Actions = append(out.Actions, "reporting.preview")
				out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "preview", ID: b.Block})
			}
		}
		out.Original = true
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, store.ErrNotFound) {
		if in.Mode != "search" {
			return reporting.OptionDependencyManifest{}, store.ErrNotFound
		}
		// Existing discovery cores supply the complete immutable publication/report
		// closure. They read metadata only; the real option service still validates
		// the chosen dimension/page/filter and current execution fences.
		if target := in.Target.Dataset; target != nil {
			m, e2 := d.DiscoverAuthoringDataDependencies(ctx, e, reporting.DataDependencyRequest{Topic: target.Topic, Dataset: target.Dataset})
			if e2 != nil {
				return reporting.OptionDependencyManifest{}, e2
			}
			out.References = append(out.References, m.References...)
			out.References = append(out.References, m.QueryReferences...)
		} else {
			r := in.Target.Report
			operation := "report_run"
			if r.Policy == "private_preview" {
				operation = "preview"
			}
			m, e2 := d.DiscoverAuthoringEffectDependencies(ctx, e, reporting.EffectDependencyRequest{Operation: operation, ID: r.Report, Revision: r.Revision})
			if e2 != nil {
				return reporting.OptionDependencyManifest{}, e2
			}
			if m.Digest != r.Digest {
				return reporting.OptionDependencyManifest{}, reporting.ErrStale
			}
			for _, ref := range m.References {
				if ref.Permission != "write" {
					out.References = append(out.References, ref)
				}
			}
		}
		err = nil
	}
	if err != nil {
		return reporting.OptionDependencyManifest{}, err
	}
	if target := in.Target.Dataset; target != nil {
		out.Actions = append(out.Actions, "charts.bind", "reporting.write", "reporting.preview", "reporting.validate")
		out.References = append(out.References, reporting.ResourceReference{Kind: "topic", Permission: "write", ID: target.Topic.Topic})
		for _, p := range []string{"read", "write", "preview"} {
			out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: p, ID: target.NewBlock})
		}
	} else {
		r := in.Target.Report
		out.Actions = append(out.Actions, "reporting.execute")
		out.References = append(out.References, reporting.ResourceReference{Kind: "report", Permission: "read", ID: r.Report}, reporting.ResourceReference{Kind: "report", Permission: "execute", ID: r.Report})
		if r.Policy == "private_preview" {
			out.Actions = append(out.Actions, "reporting.preview")
			out.References = append(out.References, reporting.ResourceReference{Kind: "report", Permission: "preview", ID: r.Report})
		}
	}
	slices.Sort(out.Actions)
	out.Actions = slices.Compact(out.Actions)
	slices.SortFunc(out.References, func(a, b reporting.ResourceReference) int {
		return slices.Compare([]string{a.Kind, a.Permission, a.ID}, []string{b.Kind, b.Permission, b.ID})
	})
	out.References = slices.Compact(out.References)
	if len(out.References) > 128 || len(out.Actions) > 8 {
		return reporting.OptionDependencyManifest{}, store.ErrInvalid
	}
	if !e.Valid() {
		return reporting.OptionDependencyManifest{}, access.ErrUnauthenticated
	}
	if err = ctx.Err(); err != nil {
		return reporting.OptionDependencyManifest{}, err
	}
	return out, nil
}
