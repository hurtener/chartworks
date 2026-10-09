package acceptance

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestReportAppPreparationConcurrentQuotaReservesLifecycle(t *testing.T) {
	f, _, e, _, base, raw, scopes := preparationFixtureBase(t, "quota-preparation")
	old := time.Now().Add(-25 * time.Hour).UTC()
	for n := 900; n < 906; n++ {
		r := preparationCopy(t, base, n)
		r.Status = "uncertain"
		r.CreatedAt = old
		r.Deadline = old.Add(30 * time.Second)
		r.ExpiresAt = old.Add(15 * time.Minute)
		seedPreparation(t, raw, r, true, nil, nil, nil)
	}
	scope, _ := store.NewScope(e.Tenant(), e.User())
	original, err := f.f.f.db.GetReadOperation(t.Context(), scope, base.SourceOperation)
	if err != nil {
		t.Fatal(err)
	}
	var candidates [2]reporting.AuthoringPreparationRecord
	var owners [2]identity.Envelope
	for i, target := range []string{"quota-chart-a", "quota-chart-b"} {
		owners[i] = phase27Actor(t, f.f, e.User(), append(slices.Clone(scopes), "cw.block.read:"+target, "cw.block.write:"+target, "cw.block.preview:"+target))
		candidates[i] = preparationCopy(t, base, 910+i)
		r := &candidates[i]
		r.Target = target
		r.Request.NewBlock = target
		r.InputDigest = readexec.Hash(r.Request)
		r.SourceOperation = "chart-prepare:" + readexec.Hash([]string{e.Tenant(), e.User(), e.Session(), target, r.Operation})
	}
	var failures [2]error
	var fresh [2]bool
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range 2 {
		go func() {
			defer wg.Done()
			_, fresh[i], failures[i] = f.f.f.db.ReserveAuthoringPreparation(t.Context(), owners[i], candidates[i])
		}()
	}
	wg.Wait()
	admitted := 0
	for i := range 2 {
		if fresh[i] {
			admitted++
			if failures[i] != nil {
				t.Fatal(failures[i])
			}
			if err := f.f.f.db.SealAuthoringPreparation(t.Context(), owners[i], candidates[i], original.Manifest.Receipt); err != nil {
				t.Fatal(err)
			}
			a := preparationNativeAttempt(t, original, candidates[i], 920+i)
			if err := f.f.f.db.BeginRead(t.Context(), scope, a, 1); err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(failures[i], readexec.ErrLimit) {
			t.Fatal("unexpected admission result", failures[i])
		}
	}
	if admitted != 1 {
		t.Fatal("concurrent reservations ignored lifecycle growth", admitted)
	}
	var charged int64
	err = raw.QueryRow(t.Context(), `SELECT sum(CASE WHEN settlement IS NULL OR status IN('accepted','uncertain') THEN GREATEST(2232320,octet_length(record::text)+COALESCE(octet_length(read_receipt::text),0)+COALESCE(octet_length(admitted_manifest::text),0)+COALESCE(octet_length(settlement::text),0)) ELSE octet_length(record::text)+COALESCE(octet_length(read_receipt::text),0)+COALESCE(octet_length(admitted_manifest::text),0)+COALESCE(octet_length(settlement::text),0) END) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND actor_id=$2`, e.Tenant(), e.User()).Scan(&charged)
	if err != nil || charged > 16<<20 {
		t.Fatal("hard byte cap exceeded after seal and BeginRead", charged, err)
	}
}
