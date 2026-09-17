"""Apply this reviewed CW-01 edit set on the isolated feature branch only.

Every replacement and new-file precondition is checked before writing. The
branch-scoped workflow formats and commits these exact edits; final validation
runs separately against committed source without executing this helper.
"""
from pathlib import Path

replacements = [
    (
        "internal/exec/business_sql.go",
        "\tif err := ValidateBusinessConstraints(binding, constraints); err != nil {",
        "\tif err := ctx.Err(); err != nil {\n\t\treturn BusinessBoundQuery{}, err\n\t}\n\tif err := ValidateBusinessConstraints(binding, constraints); err != nil {",
    ),
    (
        "internal/exec/business_sql.go",
        "\tif len(constraints) == 0 {\n\t\treturn BusinessBoundQuery{SQL: statement, Parameters: append([]Parameter(nil), parameters...)}, nil\n\t}",
        "\tif len(constraints) == 0 {\n\t\tif err := ctx.Err(); err != nil {\n\t\t\treturn BusinessBoundQuery{}, err\n\t\t}\n\t\treturn BusinessBoundQuery{SQL: statement, Parameters: append([]Parameter(nil), parameters...)}, nil\n\t}",
    ),
    (
        "internal/exec/business_sql.go",
        "\treturn BusinessBoundQuery{SQL: bound, Parameters: outParams, Receipt: BusinessBindingReceipt{SchemaVersion: 1, SourceBinding: Hash(binding), Constraints: Hash(constraints), Statement: Hash([]any{bound, outParams}), Bindings: bindings}}, ctx.Err()",
        "\tif err := ctx.Err(); err != nil {\n\t\treturn BusinessBoundQuery{}, err\n\t}\n\treturn BusinessBoundQuery{SQL: bound, Parameters: outParams, Receipt: BusinessBindingReceipt{SchemaVersion: 1, SourceBinding: Hash(binding), Constraints: Hash(constraints), Statement: Hash([]any{bound, outParams}), Bindings: bindings}}, nil",
    ),
    (
        "test/acceptance/cw01_test.go",
        '\tt.Run("AC10", func(t *testing.T) { cw01AuthoringAcceptance(t); cw01ConsumerAcceptance(t) })',
        '\tt.Run("AC10", func(t *testing.T) {\n\t\tcw01MigrationAcceptance(t)\n\t\tcw01AuthoringAcceptance(t)\n\t\tcw01ConsumerAcceptance(t)\n\t})',
    ),
]

new_files = {
    "internal/exec/business_cancel_test.go": r'''package exec

import (
 "context"
 "errors"
 "reflect"
 "strconv"
 "sync/atomic"
 "testing"
)

// A deterministic, monotonic cancellation source exercises every observed Err
// checkpoint without sleeps, goroutine races, or dependence on machine speed.
type businessCancelCheckpoint struct {
 context.Context
 cancel context.CancelFunc
 remaining atomic.Int64
}

func (c *businessCancelCheckpoint) Err() error {
 if c.remaining.Add(-1) <= 0 { c.cancel() }
 return c.Context.Err()
}

func TestBusinessBindingCancellationIsAtomic(t *testing.T) {
 for _, constrained := range []bool{false, true} {
  t.Run(strconv.FormatBool(constrained), func(t *testing.T) {
   var constraints []BusinessConstraint
   if constrained { constraints = []BusinessConstraint{businessFixtureConstraint()} }
   canceled, completed := 0, 0
   for checkpoint := int64(1); checkpoint <= 32; checkpoint++ {
    parent, cancel := context.WithCancel(context.Background())
    ctx := &businessCancelCheckpoint{Context:parent, cancel:cancel}
    ctx.remaining.Store(checkpoint)
    out, err := BindBusinessConstraints(ctx, parserBinding(), "SELECT id FROM analytics.sales ORDER BY id", nil, constraints)
    cancel()
    if err != nil {
     canceled++
     if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, BusinessBoundQuery{}) {
      t.Fatalf("checkpoint %d exposed partial SQL, parameters, or a receipt on cancellation: %v", checkpoint, err)
     }
    } else {
     completed++
     if out.SQL == "" || constrained && (len(out.Parameters) != 1 || len(out.Receipt.Bindings) != 1) {
      t.Fatalf("checkpoint %d returned incomplete successful binding", checkpoint)
     }
    }
   }
   if canceled < 2 || completed == 0 { t.Fatal("did not exercise both cancellation and completed binding") }
  })
 }
}
''',
    "test/acceptance/cw01_migration_test.go": r'''package acceptance

import (
 "context"
 "testing"

 "github.com/hurtener/chartworks/internal/store/postgres"
 "github.com/hurtener/chartworks/test/support"
)

// Apply the already shipped migration prefix first, then upgrade through the
// normal runner. A fresh database alone cannot prove retained-prefix fidelity.
func cw01MigrationAcceptance(t *testing.T) {
 t.Helper()
 ctx := context.Background()
 dsn := support.Database(t)
 connection := upgradeFixture(t, dsn)
 manifest, err := postgres.Migrations()
 if err != nil { t.Fatal(err) }
 if len(manifest) != 37 || manifest[34].Name != "migrations/035_reporting_output_intent.sql" || manifest[35].Name != "migrations/036_nlq_clarification.sql" || manifest[36].Name != "migrations/037_clarification_comparison.sql" {
  t.Fatal("CW-01 did not append to the shipped schema")
 }
 for _, migration := range manifest[1:35] {
  sql(t, connection, migration.SQL)
  sql(t, connection, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
 }
 sql(t, connection, `INSERT INTO chartworks.policy_revisions(tenant_id,revision,audit_days,operation_hours,created_by) VALUES('cw01-upgrade',1,12,48,'actor'); INSERT INTO chartworks.policies VALUES('cw01-upgrade',1)`)
 database := support.Open(t, dsn)
 if err := database.Check(ctx); err != nil { t.Fatal(err) }
 if count(t, connection, `SELECT count(*) FROM chartworks.schema_migrations`) != 37 {
  t.Fatal("upgrade did not apply exactly two new migrations")
 }
 for _, migration := range manifest[:35] {
  var name, checksum string
  if err := connection.QueryRow(ctx, `SELECT name,checksum FROM chartworks.schema_migrations WHERE version=$1`, migration.Version).Scan(&name, &checksum); err != nil || name != migration.Name || checksum != migration.Checksum {
   t.Fatal("upgrade changed a shipped migration", err)
  }
 }
 policy, err := database.Policy(ctx, support.Scope(t, "cw01-upgrade", "actor"))
 if err != nil || policy.AuditDays != 12 || policy.OperationHours != 48 {
  t.Fatal("upgrade lost retained tenant data", err)
 }
 for _, column := range [][2]string{{"nlq_queries", "clarification"}, {"topic_rule_comparison_evidence", "baseline_clarification_result"}, {"topic_rule_comparison_evidence", "candidate_clarification_result"}} {
  if count(t, connection, `SELECT count(*) FROM information_schema.columns WHERE table_schema='chartworks' AND table_name=$1 AND column_name=$2`, column[0], column[1]) != 1 {
   t.Fatal("upgrade omitted typed clarification persistence")
  }
 }
 options := postgres.Defaults()
 options.MigrationPolicy = "check"
 checked, err := postgres.Open(ctx, dsn, options)
 if err != nil { t.Fatal(err) }
 checked.Close()
}
''',
}

# Build all intended contents before mutating any file.
contents = {}
for path, before, after in replacements:
    text = contents.get(path)
    if text is None:
        text = Path(path).read_text()
    if text.count(before) != 1:
        raise SystemExit(f"{path}: expected one reviewed replacement anchor")
    contents[path] = text.replace(before, after)
for path, text in new_files.items():
    if Path(path).exists():
        raise SystemExit(f"{path}: refusing to replace an existing file")
    contents[path] = text
for path, text in contents.items():
    Path(path).write_text(text)
