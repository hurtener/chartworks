package acceptance

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/jackc/pgx/v5"
	"github.com/parquet-go/parquet-go"
	pqsnap "github.com/parquet-go/parquet-go/compress/snappy"
)

// Both databases and both workspace roles are real PostgreSQL objects. The
// metadata service account is never accepted as an engineering writer.
type engineeringFixture struct {
	*sourceFixture
	service  *engineering.Service
	executor *readexec.Executor
	values   config.Values
	writer   string
	reads    atomic.Int64
}

func newEngineeringFixture(t *testing.T, change func(*config.Values), model gateway.Engine) *engineeringFixture {
	t.Helper()
	base := newSourceFixture(t, func(c *config.Sources) {
		c.Connections = append(c.Connections, config.SourceConnection{
			Tenant: "source-a", ID: "workspace", Version: "operator-v1",
			ReadDSN: "env:CHARTWORKS_SOURCE_READ", WriteDSN: "env:CHARTWORKS_SOURCE_WRITE",
			ManagedSchema: "cw_test", Relations: []config.SourceRelation{},
		})
	})
	f := &engineeringFixture{sourceFixture: base, values: config.Defaults(), writer: base.role + "_writer"}
	f.e = f.actor(t, "source-a", "operator")
	f.values.Sources = f.cfg.Clone()
	f.values.Uploads.Enabled = true
	f.values.Profiling.Enabled = true
	f.values.Uploads.MaxBytes = 1 << 20
	f.values.Uploads.MaxExpandedBytes = 4 << 20
	f.values.Uploads.MaxPageBytes = 1 << 20
	f.values.Uploads.MaxRowGroupBytes = 4 << 20
	if change != nil {
		change(&f.values)
	}
	ctx := context.Background()
	u, err := url.Parse(f.warehouse)
	if err != nil {
		t.Fatal(err)
	}
	quoted := pgx.Identifier{f.writer}.Sanitize()
	if _, err = f.admin.Exec(ctx, "CREATE ROLE "+quoted+" LOGIN PASSWORD 'SYNTHETIC_WRITER_PASSWORD' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS; GRANT CREATE ON DATABASE "+pgx.Identifier{strings.TrimPrefix(u.Path, "/")}.Sanitize()+" TO "+quoted); err != nil {
		t.Fatal("isolated writer", err)
	}
	t.Cleanup(func() {
		_, _ = f.admin.Exec(context.Background(), "DROP OWNED BY "+quoted)
		_, _ = f.admin.Exec(context.Background(), "DROP ROLE "+quoted)
	})
	u.User = url.UserPassword(f.writer, "SYNTHETIC_WRITER_PASSWORD")
	f.mu.Lock()
	f.sourceFixture.values["CHARTWORKS_SOURCE_WRITE"] = u.String()
	f.mu.Unlock()
	// Operational retention must be configured explicitly, just as for the
	// existing shared operation ledger. This is not an identity or role store.
	scope, err := store.NewScope(f.e.Tenant(), f.e.User())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.SetPolicy(ctx, scope, 0, store.Policy{AuditDays: 30, OperationHours: 24}); err != nil {
		t.Fatal("operation retention fixture", err)
	}
	f.executor, err = readexec.NewExecutor(f.s, f.db, f.values.Exec)
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = engineering.New(f.db, f.s, f.validator, f.executor, model, f.values, f.lookup)
	if err != nil {
		t.Fatal("engineering service", err)
	}
	t.Cleanup(f.service.Close)
	return f
}

func (f *engineeringFixture) actor(t *testing.T, tenant, user string) identity.Envelope {
	t.Helper()
	return f.token.envelope(t, tenant, user,
		"sources.write", "sources.read", "sources.rotate", "sources.query", "sources.upload", "sources.erase",
		"engineering.profile", "engineering.read", "jobs.read", "jobs.cancel",
		"engineering.pipeline.write", "engineering.pipeline.publish", "engineering.pipeline.run", "engineering.pipeline.read",
		"cw.tenant.write:"+tenant, "cw.tenant.erase:"+tenant,
		"cw.source.read:*", "cw.source.write:*", "cw.source.query:*", "cw.source.erase:*",
		"cw.execution_context.use:*", "cw.dataset.query:*",
		"cw.topic.read:*", "cw.topic.write:*", "cw.block.read:*", "cw.block.write:*",
		"cw.report.read:*", "cw.report.write:*", "cw.dashboard.read:*", "cw.dashboard.write:*")
}

func engineeringSpec(id, format string, raw []byte, columns []engineering.UploadColumn) engineering.UploadSpec {
	hash := sha256.Sum256(raw)
	spec := engineering.UploadSpec{ID: id, Name: "Synthetic upload", Connection: "workspace", Format: format, Columns: columns, Bytes: int64(len(raw)), SHA256: hex.EncodeToString(hash[:])}
	if format == "xlsx" {
		spec.Sheet = "Data"
	}
	return spec
}

func (f *engineeringFixture) stage(t *testing.T, spec engineering.UploadSpec, raw []byte) engineering.UploadStatus {
	t.Helper()
	ctx := context.Background()
	reserved, err := f.service.ReserveUpload(ctx, f.e, spec)
	if err != nil || reserved.State != "awaiting_data" {
		t.Fatal("reserve", err, reserved)
	}
	staged, err := f.service.StageUpload(ctx, f.e, spec.ID, bytes.NewReader(raw))
	if err != nil || staged.State != "staged" || staged.Source != nil {
		t.Fatal("stage", err, staged)
	}
	return staged
}

func (f *engineeringFixture) load(t *testing.T, spec engineering.UploadSpec, raw []byte) engineering.UploadRun {
	t.Helper()
	f.stage(t, spec, raw)
	loaded, err := f.service.LoadUpload(context.Background(), f.e, spec.ID, spec.ID+"-load", false)
	if err != nil || loaded.Upload.State != "active" || loaded.Upload.Source == nil {
		t.Fatal("activate", err, loaded)
	}
	return loaded
}

func (f *engineeringFixture) readUpload(t *testing.T, source sources.Source) readexec.ExecutionReport {
	t.Helper()
	ctx := context.Background()
	binding, err := f.s.Binding(ctx, f.e, source.ID, source.ContextID)
	if err != nil || len(binding.Relations) != 1 {
		t.Fatal("managed source binding", err, binding)
	}
	relation := binding.Relations[0]
	columns := make([]string, len(relation.Columns))
	for i, column := range relation.Columns {
		columns[i] = pgx.Identifier{column.Name}.Sanitize()
	}
	plan, err := f.validator.Validate(ctx, f.e, readexec.Request{
		Source: source.ID, Context: source.ContextID,
		SQL: "SELECT " + strings.Join(columns, ",") + " FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize() + " ORDER BY " + columns[0],
	})
	if err != nil {
		t.Fatal("common validator", err)
	}
	report, err := f.executor.Execute(ctx, f.e, plan, readexec.Options{Operation: "upload-read-" + strconv.FormatInt(f.reads.Add(1), 10), Number: 1})
	if err != nil || report.Result == nil || report.Attempt.Status != "succeeded" {
		t.Fatal("common executor", err, report)
	}
	return report
}

func (f *engineeringFixture) profileSpec(t *testing.T, source sources.Source, version string, columns []string, timeColumn string) engineering.ProfileSpec {
	t.Helper()
	binding, err := f.s.Binding(context.Background(), f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range binding.Relations {
		if len(binding.Relations) == 1 || relation.Name == "sales" {
			return engineering.ProfileSpec{ID: version, Source: source.ID, Context: source.ContextID, Dataset: relation.ID, Columns: columns, TimeColumn: timeColumn, SkipLLM: true}
		}
	}
	t.Fatal("fixture dataset missing")
	return engineering.ProfileSpec{}
}

func (f *engineeringFixture) profile(t *testing.T, spec engineering.ProfileSpec) engineering.ProfileRun {
	t.Helper()
	run, err := f.service.Build(context.Background(), f.e, spec, spec.ID+"-build", false)
	if err != nil || run.Profile.State != "complete" || run.Profile.Profile == nil {
		t.Fatal("profile publication", err, run)
	}
	return run
}

func (f *engineeringFixture) client(t *testing.T) *sdk.Client {
	t.Helper()
	h := sourceapi.Handler(f.token.verifier, f.s, f.validator, http.NotFoundHandler())
	h = sourceapi.ExecutionHandler(f.token.verifier, f.validator, f.executor, h)
	h = sourceapi.EngineeringHandler(f.token.verifier, f.service, h)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		return f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func engineeringCSV() ([]byte, []engineering.UploadColumn) {
	raw := []byte("ID,Amount,Active,Note,Event Time,Document,Payload\n9007199254740993,9007199254740993.125,true,,2026-01-02T06:04:05+03:00,\"{\"\"n\"\":9007199254740993}\",cafe\n2,5.500,false,\\N,2026-01-03T03:04:05Z,\\N,\\N\n")
	columns := []engineering.UploadColumn{{Name: "id", Type: "integer"}, {Name: "amount", Type: "decimal"}, {Name: "active", Type: "boolean"}, {Name: "note", Type: "text", Nullable: true}, {Name: "event_time", Type: "timestamp"}, {Name: "document", Type: "json", Nullable: true}, {Name: "payload", Type: "binary", Nullable: true}}
	return raw, columns
}

// The package is deliberately synthetic and has two real sheet relationships.
// Deterministic ZIP ordering makes checksum/idempotency tests reproducible.
func engineeringXLSX(t *testing.T, edit func(map[string]string)) []byte {
	t.Helper()
	parts := map[string]string{
		"[Content_Types].xml":        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="book" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Data" sheetId="1" r:id="first"/><sheet name="Other" sheetId="2" r:id="second"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="first" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="second" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>ID</t></is></c><c r="B1" t="inlineStr"><is><t>Note</t></is></c></row><row r="2"><c r="A2"><v>9007199254740993</v></c><c r="B2" t="inlineStr"><is><t></t></is></c></row><row r="3"><c r="A3"><v>2</v></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml":   `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>ID</t></is></c><c r="B1" t="inlineStr"><is><t>Note</t></is></c></row><row r="2"><c r="A2"><v>7</v></c><c r="B2" t="inlineStr"><is><t>other-sheet</t></is></c></row></sheetData></worksheet>`,
	}
	if edit != nil {
		edit(parts)
	}
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for _, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0600)
		writer, err := archive.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.Write([]byte(parts[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func engineeringParquet(t *testing.T) []byte {
	t.Helper()
	type row struct {
		ID   int64   `parquet:"id"`
		Note *string `parquet:"note,optional"`
	}
	var out bytes.Buffer
	writer := parquet.NewGenericWriter[row](&out, parquet.Compression(&pqsnap.Codec{}))
	empty := ""
	if _, err := writer.Write([]row{{ID: 9007199254740993, Note: &empty}, {ID: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
