package postgres

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

type frozenHead struct {
	view                                            reporting.RunView
	actor, session, requestHash, taskHash, reuseKey string
	manifestExpires                                 time.Time
	reserved, maximum                               int64
	published                                       bool
	operationState                                  string
}

const frozenHeadColumns = `h.operation_id,h.block_id,h.revision,h.revision_digest,h.manifest_digest,h.state,h.code,h.private,h.source_id,h.context_id,h.partition_digest,h.locale,h.timezone,h.created_at,h.payload_expires_at,h.observed_at,h.finished_at,h.reused_from,h.retained_bytes,h.reserved_calls,h.reserved_tokens,h.frozen_version,h.actor_id,h.session_id,h.request_hash,h.task_hash,h.reuse_key,h.reserved_bytes,h.max_artifact_bytes,o.status,o.attempt_count,COALESCE(NOT b.archived AND b.published_revision IS NOT NULL,false),h.expires_at`

// All predicates are bound metadata and signed input. No payload is fetched and
// then filtered, and a context name never narrows a broader retained partition.
const frozenEligibility = `h.tenant_id=$1
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='execution_context' AND g->>'permission'='use' AND g->>'id' IN(h.context_id,'*'))
 AND (NOT h.private OR (h.actor_id=$4 AND h.session_id=$5 AND $7 AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='block' AND g->>'permission'='preview' AND g->>'id' IN(h.block_id,'*'))))
 AND (
  ($6 AND h.actor_id=$4 AND h.session_id=$5
   AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='block' AND g->>'permission'='execute' AND g->>'id' IN(h.block_id,'*'))
   AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='source' AND g->>'permission'='query' AND g->>'id' IN(h.source_id,'*'))
   AND EXISTS(SELECT 1 FROM chartworks.block_revision_references rr WHERE (rr.tenant_id,rr.block_id,rr.revision)=(h.tenant_id,h.block_id,h.revision))
   AND NOT EXISTS(SELECT 1 FROM chartworks.block_revision_references rr WHERE (rr.tenant_id,rr.block_id,rr.revision)=(h.tenant_id,h.block_id,h.revision)
       AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'=rr.kind AND g->>'permission'=rr.permission AND g->>'id' IN(rr.resource_id,'*'))))
  OR (NOT $6 AND (
   EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='run' AND g->>'permission'='read' AND g->>'id' IN(h.operation_id,'*'))
   OR (NOT h.private AND NOT b.archived AND b.published_revision IS NOT NULL AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='block' AND g->>'permission'='read' AND g->>'id' IN(h.block_id,'*'))))))`

const frozenFrom = ` FROM chartworks.frozen_runs h JOIN chartworks.operations o ON(o.tenant_id,o.operation_id)=(h.tenant_id,h.operation_id) LEFT JOIN chartworks.block_heads b ON(b.tenant_id,b.block_id)=(h.tenant_id,h.block_id) WHERE `

func frozenArgs(e identity.Envelope, id string, execution bool) ([]any, error) {
	if !e.Valid() {
		return nil, access.ErrUnauthenticated
	}
	if !identity.Identifier(id) {
		return nil, store.ErrInvalid
	}
	action := "reporting.read"
	if execution {
		action = "reporting.execute"
	}
	if !e.Has(action) {
		return nil, access.ErrForbidden
	}
	grants, err := blockGrants(e)
	if err != nil {
		return nil, err
	}
	return []any{e.Tenant(), id, grants, e.User(), e.Session(), execution, e.Has("reporting.preview")}, nil
}

func scanFrozenHead(row pgx.Row) (h frozenHead, err error) {
	var reused *string
	v := &h.view
	err = row.Scan(&v.ID, &v.Block, &v.Revision, &v.RevisionDigest, &v.ManifestDigest, &v.State, &v.Code, &v.Private, &v.Source, &v.Context, &v.PartitionDigest, &v.Locale, &v.Timezone, &v.Created, &v.Expires, &v.Observed, &v.Finished, &reused, &v.RetainedBytes, &v.ReservedCalls, &v.ReservedTokens, &v.FrozenVersion, &h.actor, &h.session, &h.requestHash, &h.taskHash, &h.reuseKey, &h.reserved, &h.maximum, &h.operationState, &v.Attempts, &h.published, &h.manifestExpires)
	if reused != nil {
		v.ReusedFrom = *reused
	}
	v.Parameters = []reporting.BoundValue{}
	v.Outputs = []reporting.OutputSummary{}
	v.QueryAttempts = []readexec.Attempt{}
	return h, err
}

func frozenReadHeadTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, execution bool, lock bool) (frozenHead, error) {
	args, err := frozenArgs(e, id, execution)
	if err != nil {
		return frozenHead{}, err
	}
	query := `SELECT ` + frozenHeadColumns + frozenFrom + frozenEligibility + ` AND h.operation_id=$2`
	if lock {
		query += ` FOR UPDATE OF h`
	}
	return scanFrozenHead(tx.QueryRow(ctx, query, args...))
}

func frozenAttemptMatches(h frozenHead, a readexec.Attempt) bool {
	return a.Manifest.Valid() && a.Number >= 1 && a.Number <= 3 && a.Manifest.Operation == h.view.ID && a.Manifest.Session == h.session &&
		a.Manifest.Receipt.Source == h.view.Source && a.Manifest.Receipt.Context == h.view.Context && a.Manifest.Preview == h.view.Private
}

func frozenAttemptsTx(ctx context.Context, tx pgx.Tx, tenant string, h frozenHead) ([]readexec.Attempt, error) {
	byNumber := map[int]readexec.Attempt{}
	rows, err := tx.Query(ctx, `SELECT receipt FROM chartworks.frozen_run_attempts WHERE tenant_id=$1 AND operation_id=$2 ORDER BY attempt_number`, tenant, h.view.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var raw []byte
		var a readexec.Attempt
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &a) != nil || !frozenAttemptMatches(h, a) {
			rows.Close()
			return nil, store.ErrInvalid
		}
		byNumber[a.Number] = a
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 ORDER BY attempt_number`, tenant, h.actor, h.view.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		a, scanErr := scanRead(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if !frozenAttemptMatches(h, a) {
			return nil, store.ErrInvalid
		}
		if old, found := byNumber[a.Number]; found && (old.ID != a.ID || readexec.Hash(old.Manifest) != readexec.Hash(a.Manifest)) {
			return nil, store.ErrInvalid
		}
		byNumber[a.Number] = a
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]readexec.Attempt, 0, len(byNumber))
	for _, a := range byNumber {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func frozenValuesTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, h frozenHead, out *reporting.RunRecord) error {
	var manifest, result []byte
	var resultHash *string
	if err := tx.QueryRow(ctx, `SELECT manifest,result,result_digest FROM chartworks.frozen_run_payloads WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), h.view.ID).Scan(&manifest, &result, &resultHash); err != nil {
		return err
	}
	var m reporting.RunManifest
	if json.Unmarshal(manifest, &m) != nil || m.Digest() != h.view.ManifestDigest || m.Tenant != e.Tenant() || m.ID != h.view.ID || m.Actor != h.actor || m.Session != h.session ||
		m.Block != h.view.Block || m.Revision.Number != h.view.Revision || m.Revision.Digest != h.view.RevisionDigest || m.RequestHash != h.requestHash || m.TaskHash != h.taskHash ||
		m.Binding.Source != h.view.Source || m.Binding.Context != h.view.Context || readexec.Hash(m.Binding) != h.view.PartitionDigest || m.Private != h.view.Private || m.ReuseKey != h.reuseKey ||
		!m.Created.Equal(h.view.Created) || !m.Expires.Equal(h.manifestExpires) || m.Limits.Validate() != nil {
		return store.ErrInvalid
	}
	out.Manifest = &m
	out.View.Parameters = append([]reporting.BoundValue{}, m.Resolved.Values...)
	out.View.Trust = m.Trust
	if result != nil {
		var r readexec.Result
		if json.Unmarshal(result, &r) != nil || resultHash == nil || readexec.Hash(r) != *resultHash || len(r.Rows) > m.Limits.MaxRows || len(result) > m.Limits.MaxResultBytes {
			return store.ErrInvalid
		}
		out.Result = &r
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM chartworks.frozen_run_outputs WHERE tenant_id=$1 AND operation_id=$2 ORDER BY ordinal`, e.Tenant(), h.view.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var output reporting.RetainedOutput
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &output) != nil || reporting.CheckFrozenOutput(m, output, output.State == "indeterminate") != nil {
			return store.ErrInvalid
		}
		out.Outputs = append(out.Outputs, output)
		out.View.Outputs = append(out.View.Outputs, reporting.OutputSummary{ID: output.ID, Kind: output.Kind, State: output.State, Code: output.Code, Digest: output.Digest})
	}
	return rows.Err()
}

func frozenReadTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, execution, values bool) (reporting.RunRecord, error) {
	h, err := frozenReadHeadTx(ctx, tx, e, id, execution, false)
	if err != nil {
		return reporting.RunRecord{}, err
	}
	out := reporting.RunRecord{View: h.view, Outputs: []reporting.RetainedOutput{}, Reach: access.Artifact{Tenant: e.Tenant(), RunID: h.view.ID, ParentKind: "block", ParentID: h.view.Block, Published: h.published, Private: h.view.Private, Contexts: []access.Resource{{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: h.view.Context}}}}
	if !execution {
		if err = access.RequireArtifact(e, out.Reach); err != nil {
			return reporting.RunRecord{}, err
		}
	}
	out.View.QueryAttempts, err = frozenAttemptsTx(ctx, tx, e.Tenant(), h)
	if err != nil {
		return reporting.RunRecord{}, err
	}
	if !time.Now().Before(out.View.Expires) || h.view.State == "expired" {
		out.View.State, out.View.Code = "expired", "retention_expired"
		return out, ctx.Err()
	}
	if h.view.State == "sealed" || h.view.State == "normalized" {
		out.View.State = h.operationState
	}
	if values {
		if err = frozenValuesTx(ctx, tx, e, h, &out); err != nil {
			return reporting.RunRecord{}, err
		}
		if execution {
			if err = reporting.RequireRunManifest(e, *out.Manifest); err != nil {
				return reporting.RunRecord{}, err
			}
		}
	}
	if !e.Valid() {
		return reporting.RunRecord{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

// ReadFrozenRun applies current target/context/private reach before fetching
// definition or value bytes. An expired payload is a tombstone, never a rerun.
func (d *DB) ReadFrozenRun(ctx context.Context, e identity.Envelope, id string, execution bool) (out reporting.RunRecord, err error) {
	if _, err = frozenArgs(e, id, execution); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var readErr error
		out, readErr = frozenReadTx(ctx, tx, e, id, execution, true)
		return readErr
	})
	if err != nil {
		return reporting.RunRecord{}, err
	}
	return out, nil
}

// ListFrozenArtifacts pages already eligible metadata without loading result,
// SQL or narrative payloads. Each response is bounded independently of storage size.
func (d *DB) ListFrozenArtifacts(ctx context.Context, e identity.Envelope, after string, limit int) (out reporting.ArtifactList, err error) {
	out.Items = []reporting.RunView{}
	if limit < 1 || limit > 100 || after != "" && !identity.Identifier(after) {
		return out, store.ErrInvalid
	}
	args, err := frozenArgs(e, "list", false)
	if err != nil {
		return out, err
	}
	args[1] = after
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, queryErr := tx.Query(ctx, `SELECT h.operation_id`+frozenFrom+frozenEligibility+` AND h.operation_id>$2 ORDER BY h.operation_id LIMIT $8`, append(args, limit+1)...)
		if queryErr != nil {
			return queryErr
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if queryErr = rows.Scan(&id); queryErr != nil {
				rows.Close()
				return queryErr
			}
			ids = append(ids, id)
		}
		queryErr = rows.Err()
		rows.Close()
		if queryErr != nil {
			return queryErr
		}
		if len(ids) > limit {
			ids = ids[:limit]
			out.Next = ids[len(ids)-1]
		}
		for _, id := range ids {
			r, readErr := frozenReadTx(ctx, tx, e, id, false, false)
			if readErr != nil {
				return readErr
			}
			out.Items = append(out.Items, r.View)
		}
		return nil
	})
	if err != nil {
		return reporting.ArtifactList{}, err
	}
	return out, nil
}
