package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func TestPhase08(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		bare := f.token.envelope(t, "source-a", "reader")
		before := f.lookups.Load()
		if _, err := f.s.Create(ctx, bare, sources.CreateRequest{ID: "sales", Name: "Sales", Connection: "warehouse"}); err == nil {
			t.Fatal("unsigned creation allowed")
		}
		if f.lookups.Load() != before {
			t.Fatal("denied creation read credentials")
		}
		source := f.create(t, "sales")
		f.create(t, "other")
		if source.Revision != 1 || source.ContextID != "sales:v1" {
			t.Fatal("unversioned source")
		}
		limited := f.token.envelope(t, "source-a", "reader", "sources.read", "cw.source.read:sales", "cw.execution_context.use:sales:v1")
		list, err := f.s.List(ctx, limited, 1)
		if err != nil || len(list) != 1 || list[0].ID != "sales" {
			t.Fatal("signed list selection", err, list)
		}
		read, err := f.s.Get(ctx, limited, source.ID)
		if err != nil || read != source {
			t.Fatal("source read", err)
		}
		status, err := f.s.Test(ctx, limited, source.ID)
		if err != nil || !status.Available || status.ContextID != source.ContextID {
			t.Fatal("source test", err)
		}
		for _, e := range []identity.Envelope{bare, f.actor(t, "source-b", "reader")} {
			if _, err := f.s.Get(ctx, e, source.ID); err == nil {
				t.Fatal("foreign/unsigned read")
			}
			if _, err := f.s.Rotate(ctx, e, source.ID, 1); err == nil {
				t.Fatal("foreign/unsigned rotate")
			}
		}
		for _, value := range []any{source, list, status} {
			wire, _ := json.Marshal(value)
			for _, secret := range []string{sourcePassword, "read_dsn", "write_dsn", "connection_alias", "CHARTWORKS_SOURCE_READ", f.role, "PRIVATE_COLUMN_CANARY"} {
				if strings.Contains(string(wire), secret) {
					t.Fatal("secret present in public model", secret)
				}
			}
		}
		before = f.lookups.Load()
		if _, err := f.s.Create(ctx, f.e, sources.CreateRequest{ID: "unapproved", Name: "Unapproved", Connection: "CHARTWORKS_STORE_URL"}); err == nil || f.lookups.Load() != before {
			t.Fatal("arbitrary secret reference resolved")
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		source := f.create(t, "sales")
		oldPlan := f.plan(t, source, `SELECT id,amount FROM analytics.sales ORDER BY id`)
		if _, err := f.s.Read(ctx, f.e, oldPlan); err != nil {
			t.Fatal(err)
		}
		newPassword := "ROTATED_SYNTHETIC_PASSWORD"
		if _, err := f.admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{f.role}.Sanitize()+" PASSWORD '"+newPassword+"'"); err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(f.readDSN())
		if err != nil {
			t.Fatal(err)
		}
		u.User = url.UserPassword(f.role, newPassword)
		f.setReadDSN(u.String())
		rotated, err := f.s.Rotate(ctx, f.e, source.ID, 1)
		if err != nil || rotated.Revision != 2 || rotated.ContextID == source.ContextID {
			t.Fatal("rotation", err)
		}
		if _, err := f.s.Read(ctx, f.e, oldPlan); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("old plan used new context", err)
		}
		if _, err := f.s.Rotate(ctx, f.e, source.ID, 1); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale rotation accepted", err)
		}
		fresh := f.plan(t, rotated, `SELECT id,amount FROM analytics.sales ORDER BY id`)
		if _, err := f.s.Read(ctx, f.e, fresh); err != nil {
			t.Fatal("refreshed pool", err)
		}
		if f.writeLookups.Load() != 0 {
			t.Fatal("reader resolved writer credential")
		}
		raw := support.Raw(t, f.dsn)
		rows, err := raw.Query(ctx, `SELECT row_to_json(r)::text FROM chartworks.source_revisions r`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil)
		for rows.Next() {
			var stored string
			if err = rows.Scan(&stored); err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{sourcePassword, newPassword, token, "NEVER_RESOLVE_WRITE_CANARY"} {
				if strings.Contains(stored, secret) {
					t.Fatal("credential persisted")
				}
			}
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("AC03", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		source := f.create(t, "sales")
		discovery, err := f.s.Discover(context.Background(), f.e, source.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"id": "numeric", "amount": "numeric", "cash": "numeric", "created_at": "temporal", "active": "boolean", "name": "text", "document": "structured", "payload": "binary", "domain_value": "numeric", "mood": "unknown"}
		seen := 0
		for _, relation := range discovery.Relations {
			if relation.Name != "sales" {
				continue
			}
			for _, column := range relation.Columns {
				if want[column.Name] != column.Category {
					t.Fatal("wrong native category", column)
				}
				if (column.Name == "domain_value" || column.Name == "mood") && column.Safe {
					t.Fatal("custom type treated as safe")
				}
				seen++
			}
		}
		if seen != len(want) {
			t.Fatal("missing categories", seen)
		}
		plan := f.plan(t, source, `SELECT id,amount,cash,created_at,active,name,document,payload FROM analytics.sales ORDER BY id`)
		rows, err := f.s.Read(context.Background(), f.e, plan)
		if err != nil || len(rows.Values) != 2 || rows.Values[0][1] == nil || *rows.Values[0][1] != "9007199254740993.125" || rows.Values[1][7] != nil {
			t.Fatal("native values lost exactness/NULL", err, rows)
		}
		if !strings.Contains(*rows.Values[0][7], "cafe") {
			t.Fatal("binary text transport changed")
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		source := f.create(t, "sales")
		before := f.lookups.Load()
		if _, err := f.s.Read(ctx, f.e, readexec.Plan{}); !errors.Is(err, readexec.ErrBinding) || f.lookups.Load() != before {
			t.Fatal("zero plan reached warehouse")
		}
		var forged readexec.Plan
		if err := json.Unmarshal([]byte(`{"SQL":"DELETE FROM analytics.sales","nativeChecked":true}`), &forged); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Read(ctx, f.e, forged); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("deserialized plan authorized")
		}
		for _, sql := range []string{`DELETE FROM analytics.sales`, `SELECT nextval('stolen')`, `SELECT secret FROM analytics.sales`, `SELECT id FROM analytics.sales; DELETE FROM analytics.sales`} {
			if _, err := f.validator.Validate(ctx, f.e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: sql}); err == nil {
				t.Fatal("unsafe candidate accepted")
			}
		}
		if f.lookups.Load() != before {
			t.Fatal("rejected SQL reached native planning")
		}
		plan := f.plan(t, source, `SELECT id FROM analytics.sales WHERE id=$1`, readexec.Parameter{Kind: "integer", Value: "1"})
		if rows, err := f.s.Read(ctx, f.e, plan); err != nil || len(rows.Values) != 1 {
			t.Fatal("valid plan failed", err)
		}
	})
	t.Run("AC05", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		source := f.create(t, "sales")
		plan := f.plan(t, source, `SELECT id FROM analytics.sales`)
		label := source.ContextID + ":narrow"
		before := f.lookups.Load()
		if _, err := f.validator.Validate(ctx, f.e, readexec.Request{Source: source.ID, Context: label, SQL: `SELECT id FROM analytics.sales`}); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("client label narrowed broad credentials", err)
		}
		if f.lookups.Load() != before {
			t.Fatal("invented context reached warehouse")
		}
		// A role or schema restriction change is detected even when no API client asks for rotation.
		if _, err := f.admin.Exec(ctx, `ALTER TABLE analytics.sales ENABLE ROW LEVEL SECURITY`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Read(ctx, f.e, plan); !errors.Is(err, readexec.ErrUnsupported) {
			t.Fatal("unproven RLS context executed", err)
		}
		if _, err := f.admin.Exec(ctx, `ALTER TABLE analytics.sales DISABLE ROW LEVEL SECURITY; ALTER TABLE analytics.sales ALTER COLUMN amount TYPE numeric(32,3)`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Read(ctx, f.e, plan); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("schema revision silently reused", err)
		}
		rotated, err := f.s.Rotate(ctx, f.e, source.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Read(ctx, f.e, f.plan(t, rotated, `SELECT id FROM analytics.sales`)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("AC06", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		source := f.create(t, "sales")
		plan := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
		var wg sync.WaitGroup
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rows, err := f.s.Read(ctx, f.e, plan)
				if err != nil || len(rows.Values) != 2 {
					t.Error("concurrent source read", err)
				}
			}()
		}
		wg.Wait()
		// Distinct sources do not turn a tenant label into authority to use another context.
		other := f.create(t, "other")
		narrow := f.token.envelope(t, "source-a", "operator", "sources.query", "cw.source.query:sales", "cw.execution_context.use:"+source.ContextID, "cw.dataset.query:*")
		if _, err := f.validator.Validate(ctx, narrow, readexec.Request{Source: other.ID, Context: other.ContextID, SQL: `SELECT id FROM analytics.sales`}); err == nil {
			t.Fatal("source reach widened")
		}
		u, err := url.Parse(f.readDSN())
		if err != nil {
			t.Fatal(err)
		}
		u.User = url.UserPassword(f.role, "WRONG_CREDENTIAL_CANARY")
		f.setReadDSN(u.String())
		if _, err := f.s.Test(ctx, f.e, source.ID); err == nil || strings.Contains(err.Error(), "CANARY") || strings.Contains(err.Error(), f.role) {
			t.Fatal("credential failure hidden or leaked", err)
		}
		before := f.lookups.Load()
		if got, err := f.s.Get(ctx, f.e, source.ID); err != nil || got != source || f.lookups.Load() != before {
			t.Fatal("retained metadata depends on warehouse")
		}
		f.s.Close()
		f.s.Close()
		if _, err := f.s.Read(ctx, f.e, plan); !errors.Is(err, store.ErrUnavailable) {
			t.Fatal("closed pool reused", err)
		}
	})
}

func TestSourceRotationFence(t *testing.T) {
	f := newSourceFixture(t, nil)
	ctx := context.Background()
	source := f.create(t, "sales")
	entered, release := make(chan struct{}), make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		readDone <- f.db.WithSource(ctx, support.Scope(t, f.e.Tenant(), f.e.User()), source.ID, func(ctx context.Context, record sources.Record) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-entered
	rotated := make(chan error, 1)
	go func() { _, err := f.s.Rotate(ctx, f.e, source.ID, 1); rotated <- err }()
	select {
	case err := <-rotated:
		close(release)
		<-readDone
		t.Fatal("rotation passed active metadata fence", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-rotated; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.Rotate(ctx, f.e, source.ID, 2); results <- err }()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Fatal("rotation error", err)
		}
	}
	if wins != 1 {
		t.Fatal("rotation CAS admitted competing contexts", wins)
	}
}

func TestSourceMetadataModeAndBounds(t *testing.T) {
	f := newSourceFixture(t, nil)
	ctx := context.Background()
	source := f.create(t, "sales")
	cfg := f.cfg
	cfg.Enabled = false
	cfg.Connections = nil
	metadata, err := sources.New(f.db, cfg, func(string) (string, bool) { t.Error("metadata resolved source secret"); return "", false })
	if err != nil {
		t.Fatal(err)
	}
	defer metadata.Close()
	if metadata.Enabled() {
		t.Fatal("disabled source advertises execution")
	}
	if got, err := metadata.Get(ctx, f.e, source.ID); err != nil || got != source {
		t.Fatal(err)
	}
	if _, err := metadata.Test(ctx, f.e, source.ID); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal("metadata executed source test", err)
	}
	for _, request := range []sources.CreateRequest{{ID: "bad/", Name: "x", Connection: "warehouse"}, {ID: "x", Name: "", Connection: "warehouse"}, {ID: "x", Name: "x", Connection: "bad/"}} {
		if _, err := f.s.Create(ctx, f.e, request); err == nil {
			t.Fatal("bad request admitted")
		}
	}
	if _, err := f.s.List(ctx, f.e, 0); err == nil {
		t.Fatal("unbounded list")
	}
	if _, err := f.s.Rotate(ctx, f.e, source.ID, 0); err == nil {
		t.Fatal("unversioned rotation")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.s.Get(ctx, f.e, source.ID); err == nil {
		t.Fatal("cancelled read succeeded")
	}
	for _, value := range []string{"", "postgres://localhost/db", "postgres://u:p@remote.example/db?sslmode=disable", "postgres://u:p@localhost/db?sslmode=disable&options=leak", "postgres://u:p@localhost/db?sslmode=disable&sslmode=disable"} {
		f.setReadDSN(value)
		if _, err := f.s.Test(context.Background(), f.e, source.ID); err == nil {
			t.Fatal("invalid connection admitted", fmt.Sprint(len(value)))
		}
	}
	var typedNil *sources.Service
	if _, err := readexec.NewValidator(typedNil, config.DefaultReadValidation()); err == nil {
		t.Error("typed-nil validator adapter")
	}
}
