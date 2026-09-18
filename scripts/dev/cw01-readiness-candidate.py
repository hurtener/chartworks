#!/usr/bin/env python3
"""Prepare inspected source changes; never change a branch or run tests."""
from pathlib import Path
import subprocess

EXPECTED = "f0fa46c6364751e33f6d79b1795ba21c13fbf04c"
assert subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip() == EXPECTED


def replace(path: str, before: str, after: str) -> None:
    target = Path(path)
    source = target.read_text()
    if source.count(before) != 1:
        raise SystemExit(f"Expected exactly one inspected replacement in {path}")
    target.write_text(source.replace(before, after))


replace("internal/exec/business_model.go", '''\tcase "time_window":
\t\t// MySQL TIMESTAMP is session-zone-sensitive, unlike DATETIME.''', '''\tcase "time_window":
\t\t// SQL Server TIMESTAMP is a binary rowversion, not calendar time.
\t\t// Native identity wins even when a source supplies a temporal category.
\t\tif dialect == "sqlserver" && (native == "timestamp" || native == "rowversion") {
\t\t\treturn false
\t\t}
\t\t// MySQL TIMESTAMP is session-zone-sensitive, unlike DATETIME.''')

replace("test/acceptance/topic_publications_test.go", '''\tif _, err := f.db.ReadPublishedTopic(ctx, e, pack.Topic, "", drafts.Export); !errors.Is(err, store.ErrInvalid) {
\t\tt.Fatal("invalid publication access accepted", err)
\t}''', '''\tfor _, invalid := range []drafts.Access{0, 255} {
\t\tif _, err := f.db.ReadPublishedTopic(ctx, e, pack.Topic, "", invalid); !errors.Is(err, store.ErrInvalid) {
\t\t\tt.Fatal("invalid publication access accepted", err)
\t\t}
\t}
\t// Export is now a supported protected operation, not an invalid access mode.
\tif _, err := f.db.ReadPublishedTopic(ctx, unauthorized, pack.Topic, "", drafts.Export); !errors.Is(err, access.ErrForbidden) {
\t\tt.Fatal("publication export omitted its own action and resource permission", err)
\t}
\tif _, err := f.db.ReadPublishedTopic(ctx, e, pack.Topic, "", drafts.Export); !errors.Is(err, store.ErrNotFound) {
\t\tt.Fatal("authorized export without a publication did not preserve missing state", err)
\t}''')

replace("test/acceptance/topic_publications_test.go", '''\texact, err := client.PublishedTopicVersion(ctx, pack.Topic, pack.Version)
\tif err != nil || !reflect.DeepEqual(exact, published) {
\t\tt.Fatal("exact retained read", err)
\t}''', '''\texact, err := client.PublishedTopicVersion(ctx, pack.Topic, pack.Version)
\tif err != nil || !reflect.DeepEqual(exact, published) {
\t\tt.Fatal("exact retained read", err)
\t}
\texported, err := f.db.ReadPublishedTopic(ctx, e, pack.Topic, pack.Version, drafts.Export)
\tif err != nil || !reflect.DeepEqual(exported, published) {
\t\tt.Fatal("authorized publication export did not retain the exact version", err)
\t}
\tfor _, exportScopes := range [][]string{
\t\t{"topics.export", "sources.read", "cw.topic.read:" + pack.Topic, "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"},
\t\t{"topics.read", "sources.read", "cw.topic.export:" + pack.Topic, "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"},
\t} {
\t\texportDenied := f.token.envelope(t, f.e.Tenant(), f.e.User(), exportScopes...)
\t\tif _, err := f.db.ReadPublishedTopic(ctx, exportDenied, pack.Topic, pack.Version, drafts.Export); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
\t\t\tt.Fatal("publication export accepted incomplete action or resource authority", err)
\t\t}
\t}''')

replace("scripts/coverage_gate.py", '''# This is an aggregate build + all-packages budget. Go's -timeout=20m still
# bounds each package separately; a cold race/coverpkg build is outside it.''', '''# The cumulative acceptance package now includes CW-01 as well as every existing
# phase and standalone regression. Its race/coverpkg run exceeded 20 minutes
# while starting another test, not while stuck in that test. Keep a bounded
# 30-minute per-package budget and the unchanged aggregate 60-minute deadline.
# A cold race/coverpkg build is outside Go's per-package test timeout.''')
replace("scripts/coverage_gate.py", '"-timeout=20m", "-covermode=atomic"', '"-timeout=30m", "-covermode=atomic"')

new_test = Path("internal/exec/business_sqlserver_time_test.go")
if new_test.exists():
    raise SystemExit("Refusing to overwrite an existing SQL Server test")
new_test.write_text('''package exec

import (
    "context"
    "errors"
    "reflect"
    "strings"
    "testing"
)

func TestBusinessSQLServerRowversionRejectedBeforeBinding(t *testing.T) {
    for _, native := range []string{"timestamp", "TIMESTAMP", "rowversion", "ROWVERSION"} {
        for _, temporal := range []string{"date", "timestamp", "timestamptz"} {
            t.Run(native+"/"+temporal, func(t *testing.T) {
                binding := parserBinding()
                binding.Dialect = "sqlserver"
                binding.Relations[0].Columns[1].NativeType = native
                binding.Relations[0].Columns[1].Category = "timestamp"
                c := sqlServerTimeConstraint(temporal)
                var failure *BusinessConstraintError
                err := ValidateBusinessConstraints(binding, []BusinessConstraint{c})
                if !errors.As(err, &failure) || failure.Code != "unsupported_constraint_type" || failure.Field != "target" || failure.Resolution != c.Resolution || !errors.Is(err, ErrUnsupported) {
                    t.Fatal("binary rowversion passed calendar admission", err)
                }
                statement := "SELECT [id] FROM [analytics].[sales]"
                out, err := BindBusinessConstraints(context.Background(), binding, statement, nil, []BusinessConstraint{c})
                if !errors.Is(err, ErrUnsupported) || !reflect.DeepEqual(out, BusinessBoundQuery{}) {
                    t.Fatal("rejected rowversion returned partial SQL, parameters or receipt", err)
                }
                unconstrained, err := BindBusinessConstraints(context.Background(), binding, statement, nil, nil)
                if err != nil || !reflect.DeepEqual(unconstrained, BusinessBoundQuery{SQL: statement}) {
                    t.Fatal("calendar rejection changed unconstrained SQL", err)
                }
            })
        }
    }
}

func TestBusinessSQLServerCalendarTypesPreserved(t *testing.T) {
    cases := []struct {
        native string
        temporal string
        cast string
    }{
        {"date", "date", "DATE"},
        {"datetime", "timestamp", "DATETIME2"},
        {"datetime2", "timestamp", "DATETIME2"},
        {"DATETIME2", "timestamp", "DATETIME2"},
        {"datetime2(7)", "timestamp", "DATETIME2"},
        {"datetimeoffset", "timestamptz", "DATETIMEOFFSET"},
        {"datetimeoffset(7)", "timestamptz", "DATETIMEOFFSET"},
    }
    for _, tc := range cases {
        t.Run(tc.native, func(t *testing.T) {
            binding := parserBinding()
            binding.Dialect = "sqlserver"
            binding.Relations[0].Columns[1].NativeType = tc.native
            c := sqlServerTimeConstraint(tc.temporal)
            out, err := BindBusinessConstraints(context.Background(), binding, "SELECT [id] FROM [analytics].[sales]", nil, []BusinessConstraint{c})
            if err != nil || strings.Count(out.SQL, " AS "+tc.cast+")") != 2 || len(out.Parameters) != 2 {
                t.Fatal("supported SQL Server calendar type changed", err)
            }
            if out.Parameters[0].Value != c.Value || out.Parameters[1].Value != c.Upper || !strings.Contains(out.SQL, "@p1") || !strings.Contains(out.SQL, "@p2") {
                t.Fatal("calendar binding changed exact bounds or parameter positions")
            }
            if len(out.Receipt.Bindings) != 1 || !reflect.DeepEqual(out.Receipt.Bindings[0].Parameters, []int{1, 2}) || out.Receipt.Validation != nil {
                t.Fatal("calendar binding lost provenance or fabricated a read proof")
            }
        })
    }
}

func TestBusinessPostgresNativeTimestampStillSupported(t *testing.T) {
    binding := parserBinding()
    binding.Dialect = "postgres"
    binding.Relations[0].Columns[1].NativeType = "timestamp"
    c := sqlServerTimeConstraint("timestamp")
    out, err := BindBusinessConstraints(context.Background(), binding, `SELECT "id" FROM "analytics"."sales"`, nil, []BusinessConstraint{c})
    if err != nil || strings.Count(out.SQL, " AS TIMESTAMP)") != 2 || len(out.Parameters) != 2 {
        t.Fatal("SQL Server native-type rejection leaked into PostgreSQL", err)
    }
}

func sqlServerTimeConstraint(temporal string) BusinessConstraint {
    c := BusinessConstraint{}
    c.Resolution = Hash("reviewed-sqlserver-calendar-window")
    c.Dataset, c.Column, c.SourceRevision = "sales", "amount", 1
    c.Kind, c.Operator, c.Nulls = "time_window", "range", "exclude"
    c.TemporalType, c.Calendar, c.TimeZone = temporal, "gregorian", "America/Argentina/Buenos_Aires"
    c.Grain, c.Bounds = "day", "[)"
    c.Value, c.Upper = "2026-01-02", "2026-01-03"
    if temporal == "timestamptz" {
        c.Value, c.Upper = "2026-01-02T03:00:00Z", "2026-01-03T03:00:00Z"
    }
    return c
}
''')
subprocess.run(["gofmt", "-w", "internal/exec/business_model.go", str(new_test), "test/acceptance/topic_publications_test.go"], check=True)
subprocess.run(["git", "add", "internal/exec/business_model.go", str(new_test), "test/acceptance/topic_publications_test.go", "scripts/coverage_gate.py"], check=True)
subprocess.run(["git", "diff", "--cached", "--check"], check=True)
subprocess.run(["git", "diff", "--cached", "--stat"], check=True)
subprocess.run(["git", "diff", "--cached"], check=True)
