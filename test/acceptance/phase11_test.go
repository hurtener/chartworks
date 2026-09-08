package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func TestPhase11(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		raw, columns := engineeringCSV()
		loaded := f.load(t, engineeringSpec("typed-csv", "csv", raw, columns), raw)
		result := f.readUpload(t, *loaded.Upload.Source).Result
		if len(result.Schema) != 7 || len(result.Rows) != 2 || string(result.Rows[1][0]) != `"9007199254740993"` || string(result.Rows[1][1]) != `"9007199254740993.125"` || string(result.Rows[1][2]) != "true" || string(result.Rows[1][3]) != `""` || string(result.Rows[0][3]) != "null" || string(result.Rows[0][5]) != "null" || string(result.Rows[0][6]) != "null" {
			t.Fatal("declared CSV types or NULL/empty distinction changed", result)
		}
		if !strings.Contains(string(result.Rows[1][4]), "2026-01-02") || !strings.Contains(string(result.Rows[1][5]), "9007199254740993") || result.Schema[1].Encoding != "string" {
			t.Fatal("temporal/JSON/exact numeric transport changed", result)
		}
		pair := []engineering.UploadColumn{{Name: "id", Type: "integer"}, {Name: "note", Type: "text", Nullable: true}}
		for _, format := range []string{"xlsx", "parquet"} {
			var data []byte
			if format == "xlsx" {
				data = engineeringXLSX(t, nil)
			} else {
				data = engineeringParquet(t)
			}
			spec := engineeringSpec("typed-"+format, format, data, pair)
			load := f.load(t, spec, data)
			read := f.readUpload(t, *load.Upload.Source).Result
			if len(read.Rows) != 2 || string(read.Rows[1][0]) != `"9007199254740993"` || string(read.Rows[0][1]) != "null" || string(read.Rows[1][1]) != `""` {
				t.Fatal("format lost exact types or NULL", format, read)
			}
			if format == "xlsx" {
				spec.ID, spec.Sheet = "other-sheet", "Other"
				other := f.load(t, spec, data)
				read = f.readUpload(t, *other.Upload.Source).Result
				if len(read.Rows) != 1 || string(read.Rows[0][0]) != `"7"` || string(read.Rows[0][1]) != `"other-sheet"` {
					t.Fatal("selected worksheet was ignored", read)
				}
			}
		}
		for _, tc := range []struct{ header, want string }{{"  Año de Venta ", "ano_de_venta"}, {"\ufeffID", "id"}, {"Nombre-Apellido", "nombre_apellido"}, {"123", "c_123"}} {
			header, want := tc.header, tc.want
			if got, err := engineering.NormalizeHeader(header); err != nil || got != want {
				t.Fatal("unstable header normalization", got, err)
			}
		}
	})
	t.Run("AC02", func(t *testing.T) {
		column := []engineering.UploadColumn{{Name: "id", Type: "integer"}}
		for _, raw := range [][]byte{[]byte("id\n1\nPRIVATE_VALUE\n"), []byte("id,id\n1,2\n"), []byte("id\n1\x00\n"), []byte("id\n9223372036854775808\n"), []byte("id\n\"unterminated\n"), {0xff, 0xfe}} {
			spec := engineeringSpec("invalid", "csv", raw, column)
			r, err := engineering.Parse(context.Background(), raw, spec, config.DefaultUploads(), func([]engineering.Cell) error { return nil })
			if !errors.Is(err, engineering.ErrFormat) || r.Rows != 0 || strings.Contains(err.Error(), "PRIVATE_VALUE") {
				t.Fatal("unsafe CSV accepted or error exposed content", r, err)
			}
		}
		pair := []engineering.UploadColumn{{Name: "id", Type: "integer"}, {Name: "note", Type: "text", Nullable: true}}
		for name, edit := range map[string]func(map[string]string){
			"traversal": func(parts map[string]string) { parts["../escape.xml"] = "do not extract" },
			"macro":     func(parts map[string]string) { parts["xl/vbaProject.bin"] = "executable" },
			"formula": func(parts map[string]string) {
				parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], "<v>9007199254740993</v>", "<f>HYPERLINK(SECRET)</f><v>1</v>", 1)
			},
			"external": func(parts map[string]string) {
				parts["xl/_rels/workbook.xml.rels"] = strings.Replace(parts["xl/_rels/workbook.xml.rels"], `Target="worksheets/sheet1.xml"`, `Target="https://invalid.example/SECRET" TargetMode="External"`, 1)
			},
		} {
			raw := engineeringXLSX(t, edit)
			spec := engineeringSpec(name, "xlsx", raw, pair)
			if _, err := engineering.Parse(context.Background(), raw, spec, config.DefaultUploads(), func([]engineering.Cell) error { return nil }); err == nil {
				t.Fatal("unsafe archive accepted", name)
			}
		}
		for name, change := range map[string]func(*config.Uploads){
			"rows":       func(l *config.Uploads) { l.MaxRows = 1 },
			"cells":      func(l *config.Uploads) { l.MaxCells = 1 },
			"cell-bytes": func(l *config.Uploads) { l.MaxCellBytes = 1 },
		} {
			raw := []byte("id\n12\n13\n")
			limits := config.DefaultUploads()
			change(&limits)
			if _, err := engineering.Parse(context.Background(), raw, engineeringSpec(name, "csv", raw, column), limits, func([]engineering.Cell) error { return nil }); !errors.Is(err, engineering.ErrLimit) {
				t.Fatal("unbounded parser", name, err)
			}
		}
		xlsx := engineeringXLSX(t, nil)
		for _, change := range []func(*config.Uploads){func(l *config.Uploads) { l.MaxArchiveEntries = 1 }, func(l *config.Uploads) { l.MaxSheets = 1 }, func(l *config.Uploads) { l.MaxExpansionRatio = 1 }} {
			limits := config.DefaultUploads()
			change(&limits)
			if _, err := engineering.Parse(context.Background(), xlsx, engineeringSpec("bounded", "xlsx", xlsx, pair), limits, func([]engineering.Cell) error { return nil }); err == nil {
				t.Fatal("archive allocation ceiling ignored")
			}
		}
		ctx, stop := context.WithCancel(context.Background())
		stop()
		if _, err := engineering.Parse(ctx, xlsx, engineeringSpec("cancel", "xlsx", xlsx, pair), config.DefaultUploads(), func([]engineering.Cell) error { t.Fatal("cancelled parse emitted data"); return nil }); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled parser continued", err)
		}
	})
	t.Run("AC03", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		raw, columns := engineeringCSV()
		spec := engineeringSpec("private", "csv", raw, columns)
		f.stage(t, spec, raw)
		claims := f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes())
		claims["session"] = "other-session"
		otherSession, err := f.token.verifier.Verify(context.Background(), f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range []identity.Envelope{{}, f.actor(t, "source-b", "operator"), f.actor(t, "source-a", "other-user"), otherSession, f.token.envelope(t, "source-a", "operator", "sources.upload")} {
			before := f.lookups.Load()
			body := &unreadUploadBody{}
			if _, err := f.service.StageUpload(context.Background(), e, spec.ID, body); err == nil || body.reads.Load() != 0 || f.lookups.Load() != before {
				t.Fatal("denied stage touched body or credentials", err)
			}
			if _, err := f.service.InspectUpload(context.Background(), e, spec.ID); err == nil {
				t.Fatal("private upload metadata leaked")
			}
			if _, err := f.service.LoadUpload(context.Background(), e, spec.ID, "forbidden", false); err == nil {
				t.Fatal("foreign upload activation allowed")
			}
		}
		loaded, err := f.service.LoadUpload(context.Background(), f.e, spec.ID, "private-load", false)
		if err != nil || loaded.Upload.Source == nil {
			t.Fatal(err, loaded)
		}
		wrong := f.token.envelope(t, "source-a", "operator", "sources.query", "cw.source.query:private", "cw.dataset.query:*", "cw.execution_context.use:unrelated:v1")
		binding, err := f.s.Binding(context.Background(), f.e, spec.ID, loaded.Upload.Source.ContextID)
		if err != nil {
			t.Fatal(err)
		}
		before := f.lookups.Load()
		r := binding.Relations[0]
		if _, err = f.validator.Validate(context.Background(), wrong, readexec.Request{Source: spec.ID, Context: loaded.Upload.Source.ContextID, SQL: "SELECT id FROM " + pgx.Identifier{r.Schema, r.Name}.Sanitize()}); err == nil || f.lookups.Load() != before {
			t.Fatal("wrong execution context reached workspace", err)
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		raw, columns := engineeringCSV()
		spec := engineeringSpec("interrupted", "csv", raw, columns)
		f.stage(t, spec, raw)
		repo := &interruptedActivation{DB: f.db}
		repo.interrupt.Store(true)
		service, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
		if err != nil {
			t.Fatal(err)
		}
		run, err := service.LoadUpload(context.Background(), f.e, spec.ID, "stable-load-key", false)
		service.Close()
		if err != nil || run.Upload.State == "active" || run.Operation.State != "retry" {
			t.Fatal("interruption incorrectly published", err, run)
		}
		if _, err = f.s.Get(context.Background(), f.e, spec.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("unpublished source became visible", err)
		}
		schema, table, err := sources.ManagedLocation(f.cfg.Connections[1], spec.ID)
		if err != nil {
			t.Fatal(err)
		}
		var before int64
		if err = f.admin.QueryRow(context.Background(), "SELECT table_oid FROM "+pgx.Identifier{schema, "_uploads"}.Sanitize()+" WHERE upload_id=$1 AND state='loaded' AND raw IS NULL", spec.ID).Scan(&before); err != nil {
			t.Fatal("external commit evidence missing", err)
		}
		resumed, err := f.service.LoadUpload(context.Background(), f.e, spec.ID, "stable-load-key", true)
		if err != nil || resumed.Upload.State != "active" || resumed.Operation.ID != run.Operation.ID || resumed.Operation.Attempts != 2 {
			t.Fatal("restart did not reconcile the original operation", err, resumed)
		}
		var after int64
		var count int
		if err = f.admin.QueryRow(context.Background(), "SELECT table_oid FROM "+pgx.Identifier{schema, "_uploads"}.Sanitize()+" WHERE upload_id=$1", spec.ID).Scan(&after); err != nil || before != after {
			t.Fatal("retry replaced committed table", before, after, err)
		}
		if err = f.admin.QueryRow(context.Background(), "SELECT count(*) FROM "+pgx.Identifier{schema, table}.Sanitize()).Scan(&count); err != nil || count != 2 {
			t.Fatal("retry duplicated rows", count, err)
		}
		var wait sync.WaitGroup
		for range 4 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				out, err := f.service.LoadUpload(context.Background(), f.e, spec.ID, "stable-load-key", true)
				if err != nil || out.Upload.State != "active" {
					t.Error("concurrent replay", err, out)
				}
			}()
		}
		wait.Wait()
		bad := []byte("id\n1\nnot-an-integer\n")
		badSpec := engineeringSpec("rollback", "csv", bad, []engineering.UploadColumn{{Name: "id", Type: "integer"}})
		f.stage(t, badSpec, bad)
		failed, err := f.service.LoadUpload(context.Background(), f.e, badSpec.ID, "rollback-key", false)
		if err != nil || failed.Upload.State == "active" {
			t.Fatal("partial malformed file published", err, failed)
		}
		_, badTable, _ := sources.ManagedLocation(f.cfg.Connections[1], badSpec.ID)
		var exists bool
		if err = f.admin.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, pgx.Identifier{schema, badTable}.Sanitize()).Scan(&exists); err != nil || exists {
			t.Fatal("failed load left an orphan table", err)
		}
	})
	t.Run("AC05", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		client := f.binaryClient(t)
		raw, columns := engineeringCSV()
		internal := engineeringSpec("sdk-upload", "csv", raw, columns)
		wire, err := json.Marshal(internal)
		if err != nil {
			t.Fatal(err)
		}
		var spec sdk.UploadSpec
		if err = json.Unmarshal(wire, &spec); err != nil {
			t.Fatal(err)
		}
		reserved, err := client.ReserveUpload(context.Background(), spec)
		if err != nil || reserved.Created.IsZero() || reserved.Expires.IsZero() {
			t.Fatal("SDK reservation contract", err, reserved)
		}
		if _, err = client.StageUpload(context.Background(), spec.ID, bytes.NewReader(raw), int64(len(raw))); err != nil {
			t.Fatal("SDK binary transfer", err)
		}
		loaded, err := client.LoadUpload(context.Background(), spec.ID, "sdk-load", false)
		if err != nil || loaded.Upload.Source == nil {
			t.Fatal("SDK activation", err, loaded)
		}
		source := loaded.Upload.Source
		discovery, err := client.DiscoverSource(context.Background(), source.ID)
		if err != nil || len(discovery.Relations) != 1 {
			t.Fatal("ordinary source discovery", err, discovery)
		}
		relation := discovery.Relations[0]
		result, err := client.ExecuteRead(context.Background(), source.ID, sdk.ReadExecutionRequest{Context: source.ContextID, SQL: "SELECT id,amount FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize() + " ORDER BY id", Execution: sdk.ReadExecutionOptions{Operation: "sdk-common-read", Number: 1}})
		if err != nil || result.Result == nil || len(result.Result.Rows) != 2 || string(result.Result.Rows[1][1]) != `"9007199254740993.125"` || result.Result.Cost.ScannedBytes != nil {
			t.Fatal("upload bypassed or diverged from the common executor", err, result)
		}
		receipt, err := client.ReadExecution(context.Background(), result.Attempt.ID)
		if err != nil || receipt.Manifest.Receipt.Context != source.ContextID || receipt.Status != "succeeded" {
			t.Fatal("ordinary retained execution receipt", err, receipt)
		}
		erased, err := client.EraseUpload(context.Background(), spec.ID, "sdk-erase", false)
		if err != nil || erased.Upload.State != "erased" {
			t.Fatal("SDK erasure receipt", err, erased)
		}
		swept, err := client.SweepUploads(context.Background(), 2)
		if err != nil || len(swept) != 0 {
			t.Fatal("SDK empty staging sweep", err, swept)
		}
	})
	t.Run("AC06", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		ctx := context.Background()
		raw, columns := engineeringCSV()
		keep := f.load(t, engineeringSpec("keep", "csv", raw, columns), raw)
		remove := f.load(t, engineeringSpec("remove", "csv", raw, columns), raw)
		profile := f.profile(t, f.profileSpec(t, *remove.Upload.Source, "remove-profile", nil, ""))
		erased, err := f.service.EraseUpload(ctx, f.e, "remove", "remove-key", false)
		if err != nil || erased.Upload.State != "erased" {
			t.Fatal("owned erasure", err, erased)
		}
		if _, err = f.s.Get(ctx, f.e, "remove"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("erased source still visible", err)
		}
		if _, err = f.service.Evidence(ctx, f.e, profile.Profile.Version); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("erased source profile still exposed", err)
		}
		f.readUpload(t, *keep.Upload.Source)
		var unrelated int
		if err = f.admin.QueryRow(ctx, `SELECT count(*) FROM analytics.sales`).Scan(&unrelated); err != nil || unrelated != 2 {
			t.Fatal("erasure affected baseline data", err)
		}
		seedExpiredStaging(t, f, raw, columns)
		swept, err := f.service.SweepUploads(ctx, f.e, 2)
		if err != nil || len(swept) != 1 || swept[0].Upload.ID != "expired-staging" || swept[0].Upload.State != "erased" {
			t.Fatal("expiry sweep crossed its staging boundary", err, swept)
		}
		schema, _, _ := sources.ManagedLocation(f.cfg.Connections[1], "expired-staging")
		var retained bool
		if err = f.admin.QueryRow(ctx, "SELECT raw IS NOT NULL FROM "+pgx.Identifier{schema, "_uploads"}.Sanitize()+" WHERE upload_id='expired-staging'").Scan(&retained); err != nil || retained {
			t.Fatal("expired staged bytes retained", err)
		}
		f.readUpload(t, *keep.Upload.Source)
		// An object with the same name and even the same owner is not the
		// registered object. Erasure must not adopt its replacement OID.
		victim := f.load(t, engineeringSpec("replacement", "csv", raw, columns), raw)
		_ = victim
		schema, table, _ := sources.ManagedLocation(f.cfg.Connections[1], "replacement")
		qualified := pgx.Identifier{schema, table}.Sanitize()
		if _, err = f.admin.Exec(ctx, "ALTER TABLE "+qualified+" RENAME TO protected_original; CREATE TABLE "+qualified+"(id bigint); ALTER TABLE "+qualified+" OWNER TO "+pgx.Identifier{f.writer}.Sanitize()+"; INSERT INTO "+qualified+" VALUES(77)"); err != nil {
			t.Fatal(err)
		}
		refused, err := f.service.EraseUpload(ctx, f.e, "replacement", "replacement-erase", false)
		if err != nil || refused.Upload.State == "erased" || refused.Code != "workspace_ownership_unproven" {
			t.Fatal("unproven object was erased", err, refused)
		}
		var value int
		if err = f.admin.QueryRow(ctx, "SELECT id FROM "+qualified).Scan(&value); err != nil || value != 77 {
			t.Fatal("replacement object changed", err)
		}
	})
}

type unreadUploadBody struct{ reads atomic.Int64 }

func (b *unreadUploadBody) Read([]byte) (int, error) { b.reads.Add(1); return 0, io.ErrUnexpectedEOF }

// This fault occurs after the real workspace commit and before the real metadata
// publication. Recreating the service proves reconciliation from durable state.
type interruptedActivation struct {
	*postgres.DB
	interrupt atomic.Bool
}

func (r *interruptedActivation) ActivateUpload(ctx context.Context, inv jobs.Invocation, upload engineering.UploadRecord, receipt engineering.WorkspaceReceipt, source sources.Record) error {
	if r.interrupt.Swap(false) {
		return store.ErrUnavailable
	}
	return r.DB.ActivateUpload(ctx, inv, upload, receipt, source)
}

// Seed a synthetic already-expired upload with a committed external staging
// record. This exercises clock-based recovery without disabling immutability
// triggers, changing production limits or sleeping for the minimum one-minute TTL.
func seedExpiredStaging(t *testing.T, f *engineeringFixture, raw []byte, columns []engineering.UploadColumn) {
	t.Helper()
	spec := engineeringSpec("expired-staging", "csv", raw, columns)
	body, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	schema, table, err := sources.ManagedLocation(f.cfg.Connections[1], spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	metadata := support.Raw(t, f.dsn)
	if _, err = metadata.Exec(context.Background(), `INSERT INTO chartworks.uploads(tenant_id,source_id,actor_id,session_id,spec,spec_hash,state,created_at,expires_at,accounted_bytes) VALUES($1,$2,$3,$4,$5,$6,'staged',$7,$8,$9)`, f.e.Tenant(), spec.ID, f.e.User(), f.e.Session(), body, readexec.Hash(spec), now.Add(-48*time.Hour), now.Add(-24*time.Hour), spec.Bytes); err != nil {
		t.Fatal("expired metadata fixture", err)
	}
	if _, err = f.admin.Exec(context.Background(), fmt.Sprintf("INSERT INTO %s(upload_id,spec_hash,checksum,table_name,state,raw) VALUES($1,$2,$3,$4,'staged',$5)", pgx.Identifier{schema, "_uploads"}.Sanitize()), spec.ID, readexec.Hash(spec), spec.SHA256, table, raw); err != nil {
		t.Fatal("expired workspace fixture", err)
	}
}
