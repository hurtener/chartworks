package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Exercise the repository directly: service validation must not be the only
// protection against malformed references or missing authority.
func TestPhase27StoreReadBoundaries(t *testing.T) {
	f := newPhase18Fixture(t)
	_, topics := newPhase18Service(t, f)
	service, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	e := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	ctx := context.Background()
	definition := phase27Definition(t, f, e, "SELECT id, amount FROM analytics.sales ORDER BY id")
	for _, id := range []string{"store-a", "store-b", "store-c"} {
		if _, err := service.Create(ctx, e, reporting.CreateRequest{ID: id, Definition: definition}); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("reference bounds", func(t *testing.T) {
		for _, ref := range []reporting.Reference{{Revision: -1}, {Revision: 257}, {Draft: true, Revision: 1}} {
			if _, err := f.f.db.ReadBlock(ctx, e, "store-a", ref, reporting.Read); !errors.Is(err, store.ErrInvalid) {
				t.Fatalf("reference %+v: %v", ref, err)
			}
		}
	})
	t.Run("list bounds", func(t *testing.T) {
		for _, req := range []reporting.ListRequest{{Limit: 0}, {Limit: 101}, {Limit: 1, After: "invalid/cursor"}} {
			if _, err := f.f.db.ListBlocks(ctx, e, req); !errors.Is(err, store.ErrInvalid) {
				t.Fatalf("request %+v: %v", req, err)
			}
		}
	})
	t.Run("missing authority", func(t *testing.T) {
		if _, err := f.f.db.ReadBlock(ctx, identity.Envelope{}, "store-a", reporting.Reference{Draft: true}, reporting.Read); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal(err)
		}
		if _, err := f.f.db.ListBlocks(ctx, identity.Envelope{}, reporting.ListRequest{Limit: 1}); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal(err)
		}
		if _, err := f.f.db.BlockHistory(ctx, identity.Envelope{}, "store-a"); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal(err)
		}
		if _, err := f.f.db.CommitBlock(ctx, e, reporting.Prepared{}); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("SQL requires ordinary read authority too", func(t *testing.T) {
		sqlOnly := phase27Actor(t, f, e.User(), []string{"reporting.sql.read", "cw.block.read:*"})
		if _, err := f.f.db.ReadBlock(ctx, sqlOnly, "store-a", reporting.Reference{Draft: true}, reporting.SQLRead); !errors.Is(err, access.ErrForbidden) {
			t.Fatal(err)
		}
	})
	t.Run("pagination", func(t *testing.T) {
		var ids []string
		after := ""
		for i := 0; i < 3; i++ {
			page, err := f.f.db.ListBlocks(ctx, e, reporting.ListRequest{Limit: 1, After: after, IncludeDrafts: true})
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("page %d: %+v %v", i, page, err)
			}
			item := page.Items[0]
			if !item.Private || item.Revision != 1 {
				t.Fatalf("draft metadata: %+v", item)
			}
			ids = append(ids, item.State.ID)
			if i < 2 && page.Next != item.State.ID || i == 2 && page.Next != "" {
				t.Fatalf("cursor: %+v", page)
			}
			after = page.Next
		}
		if !reflect.DeepEqual(ids, []string{"store-a", "store-b", "store-c"}) {
			t.Fatal(ids)
		}
	})
	t.Run("corrupt retained health", func(t *testing.T) {
		raw := support.Raw(t, f.f.dsn)
		if _, err := raw.Exec(ctx, `INSERT INTO chartworks.block_health(tenant_id,block_id,revision,observation) VALUES($1,'store-a',1,'{"status":42}')`, e.Tenant()); err != nil {
			t.Fatal(err)
		}
		snapshot, err := f.f.db.ReadBlock(ctx, e, "store-a", reporting.Reference{Draft: true}, reporting.Read)
		if !errors.Is(err, store.ErrInvalid) || !reflect.DeepEqual(snapshot, reporting.Snapshot{}) {
			t.Fatalf("corrupt health returned data: %+v %v", snapshot, err)
		}
	})
	t.Run("retained validation health", func(t *testing.T) {
		raw := support.Raw(t, f.f.dsn)
		for _, tc := range []struct {
			id, status, reason string
			expired, mismatch  bool
		}{
			{id: "health-fresh", status: "healthy", reason: "validated_observation"},
			{id: "health-expired", status: "stale", reason: "validation_expired", expired: true},
			{id: "health-mismatch", mismatch: true},
		} {
			t.Run(tc.id, func(t *testing.T) {
				created, err := service.Create(ctx, e, reporting.CreateRequest{ID: tc.id, Definition: definition})
				if err != nil {
					t.Fatal(err)
				}
				now := time.Now().UTC()
				evidence := reporting.ValidationRecord{}
				evidence.Evidence.ID = "11111111111111111111111111111111"
				evidence.Evidence.Revision = 1
				evidence.Evidence.RevisionID = created.RevisionID
				evidence.Evidence.DefinitionDigest = created.Digest
				evidence.Evidence.CreatedAt = now.Add(-2 * time.Hour)
				evidence.Evidence.ExpiresAt = now.Add(time.Hour)
				if tc.expired {
					evidence.Evidence.ExpiresAt = now.Add(-time.Hour)
				}
				if tc.mismatch {
					evidence.Evidence.Revision = 2
				}
				body, err := json.Marshal(evidence)
				if err != nil {
					t.Fatal(err)
				}
				// Inject a retained record through the fixture administrator to exercise
				// read-time validation independently of service-issued write proofs.
				if _, err := raw.Exec(ctx, `INSERT INTO chartworks.block_validations(tenant_id,block_id,revision,evidence_id,actor_id,attempt_id,record,created_at,expires_at) VALUES($1,$2,1,$3,$4,$3,$5,$6,$7)`, e.Tenant(), tc.id, evidence.Evidence.ID, e.User(), body, evidence.Evidence.CreatedAt, evidence.Evidence.ExpiresAt); err != nil {
					t.Fatal(err)
				}
				snapshot, err := f.f.db.ReadBlock(ctx, e, tc.id, reporting.Reference{Draft: true}, reporting.Read)
				if tc.mismatch {
					if !errors.Is(err, store.ErrInvalid) || !reflect.DeepEqual(snapshot, reporting.Snapshot{}) {
						t.Fatalf("mismatched evidence returned data: %+v %v", snapshot, err)
					}
					return
				}
				if err != nil || snapshot.Health.Status != tc.status || snapshot.Health.Reason != tc.reason {
					t.Fatalf("health: %+v %v", snapshot.Health, err)
				}
				if !tc.expired && (snapshot.Health.ObservedAt == nil || !snapshot.Health.ObservedAt.Equal(evidence.Evidence.CreatedAt)) {
					t.Fatalf("observation time lost: %+v", snapshot.Health)
				}
			})
		}
	})
	t.Run("closed repository", func(t *testing.T) {
		f.f.db.Close()
		snapshot, err := f.f.db.ReadBlock(ctx, e, "store-b", reporting.Reference{Draft: true}, reporting.Read)
		if !errors.Is(err, store.ErrUnavailable) || !reflect.DeepEqual(snapshot, reporting.Snapshot{}) {
			t.Fatalf("read: %+v %v", snapshot, err)
		}
		page, err := f.f.db.ListBlocks(ctx, e, reporting.ListRequest{Limit: 1, IncludeDrafts: true})
		if !errors.Is(err, store.ErrUnavailable) || !reflect.DeepEqual(page, reporting.Page{}) {
			t.Fatalf("list: %+v %v", page, err)
		}
		if _, err := f.f.db.BlockHistory(ctx, e, "store-b"); !errors.Is(err, store.ErrUnavailable) {
			t.Fatal(err)
		}
	})
}
