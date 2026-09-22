package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

// This exercises two different operation IDs over the actual PostgreSQL
// frozen-run store. The physical-attempt receipt must remain on the origin.
func TestFrozenReuseIdentityPostgres(t *testing.T) {
	f := newReportingStoreFixture(t)
	ctx := context.Background()
	block := f.create(t, "reuse-identity-block", true).State.ID
	first := f.admit(t, f.runs, block, "reuse-identity-first", 60)
	first, err := f.runs.Run(ctx, f.execute, first.ID, false)
	if err != nil || first.State != "succeeded" || len(first.QueryAttempts) != 1 || first.ReusedFrom != "" {
		t.Fatal("origin must execute exactly one source attempt", err, first.State, first.QueryAttempts)
	}
	origin, err := f.f.f.db.ReadFrozenRun(ctx, f.execute, first.ID, true)
	if err != nil || origin.Manifest == nil || origin.Manifest.ReuseKey != reporting.ReuseIdentity(*origin.Manifest) {
		t.Fatal("persisted origin did not seal its complete reuse identity", err)
	}

	const peers = 2
	views := make([]reporting.RunView, peers)
	for i, key := range []string{"reuse-identity-second", "reuse-identity-third"} {
		views[i] = f.admit(t, f.runs, block, key, 60)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errorsByPeer := make([]error, peers)
	for i := range views {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			views[i], errorsByPeer[i] = f.runs.Run(ctx, f.execute, views[i].ID, false)
		}()
	}
	close(start)
	wg.Wait()
	previousIDs := map[string]bool{first.ID: true}
	for _, v := range views {
		previousIDs[v.ID] = true
	}
	directOrigin := false
	for i, v := range views {
		directOrigin = directOrigin || v.ReusedFrom == first.ID
		if errorsByPeer[i] != nil || v.State != "succeeded" || v.ReusedFrom == v.ID || !previousIDs[v.ReusedFrom] || len(v.QueryAttempts) != 0 || v.Observed == nil || first.Observed == nil || !v.Observed.Equal(*first.Observed) {
			t.Fatal("concurrent distinct runs repeated source work or lost origin observation", i, errorsByPeer[i], v.ID, v.ReusedFrom, len(v.QueryAttempts))
		}
		stored, readErr := f.f.f.db.ReadFrozenRun(ctx, f.execute, v.ID, true)
		if readErr != nil || stored.Manifest == nil || stored.Manifest.ReuseKey != origin.Manifest.ReuseKey || stored.Manifest.ReuseKey != reporting.ReuseIdentity(*stored.Manifest) {
			t.Fatal("peer reused with noncanonical persisted identity", i, readErr)
		}
	}
	if !directOrigin {
		t.Fatal("concurrent reuse lost its executed origin")
	}

	// A disposable fixture simulates a stale stored key while preserving the
	// row/payload digest and every other eligibility predicate. Immutable
	// production triggers are restored in the same transaction. Without the
	// canonical candidate check, this target would copy the changed candidate.
	spoofBlock := f.create(t, "reuse-identity-stale-key-block", true).State.ID
	spoofOrigin := f.admit(t, f.runs, spoofBlock, "reuse-identity-stale-key-origin", 60)
	spoofOrigin, err = f.runs.Run(ctx, f.execute, spoofOrigin.ID, false)
	if err != nil || len(spoofOrigin.QueryAttempts) != 1 {
		t.Fatal("stale-key fixture origin did not execute", err)
	}
	spoofTarget := f.admit(t, f.runs, spoofBlock, "reuse-identity-stale-key-target", 60)
	spoofRecord, err := f.f.f.db.ReadFrozenRun(ctx, f.execute, spoofOrigin.ID, true)
	if err != nil || spoofRecord.Manifest == nil {
		t.Fatal("stale-key fixture origin manifest unavailable", err)
	}
	spoofRecord.Manifest.Model += "-changed"
	if spoofRecord.Manifest.ReuseKey == reporting.ReuseIdentity(*spoofRecord.Manifest) {
		t.Fatal("stale-key substitution did not change the canonical identity")
	}
	spoofBody, err := json.Marshal(spoofRecord.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.raw.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, statement := range []string{
		"ALTER TABLE chartworks.frozen_runs DISABLE TRIGGER frozen_run_immutable",
		"ALTER TABLE chartworks.frozen_run_payloads DISABLE TRIGGER frozen_payload_immutable",
	} {
		if _, err = tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE chartworks.frozen_runs SET manifest_digest=$3 WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), spoofOrigin.ID, spoofRecord.Manifest.Digest()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE chartworks.frozen_run_payloads SET manifest=$3 WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), spoofOrigin.ID, spoofBody); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"ALTER TABLE chartworks.frozen_run_payloads ENABLE TRIGGER frozen_payload_immutable",
		"ALTER TABLE chartworks.frozen_runs ENABLE TRIGGER frozen_run_immutable",
	} {
		if _, err = tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	spoofTarget, err = f.runs.Run(ctx, f.execute, spoofTarget.ID, false)
	if err != nil || spoofTarget.State != "succeeded" || spoofTarget.ReusedFrom != "" || len(spoofTarget.QueryAttempts) != 1 {
		t.Fatal("stale-key candidate substituted for a new physical result", err, spoofTarget.State, spoofTarget.ReusedFrom, len(spoofTarget.QueryAttempts))
	}

	stale := f.admit(t, f.runs, block, "reuse-identity-stale", 60)
	before := f.f.f.lookups.Load()
	foreign := f.f.f.token.envelope(t, "other-tenant", f.execute.User(), phase28Scopes("other-tenant")...)
	if _, err := f.runs.Run(ctx, foreign, stale.ID, false); !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("cross-tenant run reached a reusable artifact", err)
	}
	wrongContext := phase27Actor(t, f.f, f.execute.User(), []string{"reporting.execute", "cw.block.execute:*", "cw.source.query:*", "cw.dataset.query:*", "cw.execution_context.use:other-context"})
	if _, err := f.runs.Run(ctx, wrongContext, stale.ID, false); !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("different signed context reached a reusable artifact", err)
	}
	withoutExecute := slices.DeleteFunc(phase28Scopes(f.execute.Tenant()), func(s string) bool { return s == "reporting.execute" })
	denied := phase27Actor(t, f.f, f.execute.User(), withoutExecute)
	if _, err := f.runs.Run(ctx, denied, stale.ID, false); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("removed signed action reached reuse", err)
	}
	if f.f.f.lookups.Load() != before {
		t.Fatal("denied requests touched source credentials")
	}

	// The new source head makes the sealed block stale. Its matching old key
	// cannot fetch the successful origin after a real source revision change.
	if _, err := f.f.f.s.Rotate(ctx, f.f.f.e, origin.Manifest.Binding.Source, origin.Manifest.Binding.Revision); err != nil {
		t.Fatal("rotate real source revision", err)
	}
	out, err := f.runs.Run(ctx, f.execute, stale.ID, false)
	if !errors.Is(err, reporting.ErrStale) || out.ReusedFrom != "" || len(out.QueryAttempts) != 0 {
		t.Fatal("stale source revision reused old result", err, out)
	}
}
