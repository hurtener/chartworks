#!/usr/bin/env python3
"""Apply inspected incremental fixes; separate read-only CI verifies the commit."""
from pathlib import Path
import subprocess


def replace(path: str, before: str, after: str) -> None:
    file = Path(path)
    text = file.read_text()
    if text.count(before) != 1:
        raise RuntimeError(f"{path}: expected exactly one reviewed anchor: {before!r}")
    file.write_text(text.replace(before, after))


# pgx.CopyFromFunc uses nil,nil for completion. io.EOF is an abort, not a row terminator.
replace("internal/engineering/workspace.go", '\t"io"\n', '')
replace("internal/engineering/workspace.go", '\t\t\t\treturn nil, io.EOF\n', '\t\t\t\treturn nil, nil\n')

# Discovery preserves format_type, including typmods, whereas wire results use OID names.
replace("internal/engineering/profiles.go", 'c.Name == spec.TimeColumn && c.NativeType != "date" && c.NativeType != "timestamp" && c.NativeType != "timestamptz"', 'c.Name == spec.TimeColumn && !profileTimeType(c.NativeType)')
replace("internal/engineering/profile_stats.go", 'func relationFor(r ProfileRecord)', '''// This is a type-family check after the source adapter's positive OID proof.
// Keep display precision in the profile; never infer a timezone from this name.
var profileTimeTypePattern = regexp.MustCompile(`^(date|timestamp(\\([0-6]\\))?( (with|without) time zone)?|timestamptz(\\([0-6]\\))?)$`)

func profileTimeType(native string) bool { return profileTimeTypePattern.MatchString(native) }

func relationFor(r ProfileRecord)''')
replace("test/acceptance/phase12_test.go", '"id":         {"int4", "numeric", 0, 2}', '"id":         {"integer", "numeric", 0, 2}')
replace("test/acceptance/phase12_test.go", '"amount":     {"numeric", "numeric", 0, 2}', '"amount":     {"numeric(30,3)", "numeric", 0, 2}')
replace("test/acceptance/phase12_test.go", '"active":     {"bool", "boolean", 0, 2}', '"active":     {"boolean", "boolean", 0, 2}')
replace("test/acceptance/phase12_test.go", '"created_at": {"timestamptz", "temporal", 0, 2}', '"created_at": {"timestamp with time zone", "temporal", 0, 2}')

# A signed wildcard user may list their own empty private history. No foreign row
# may be returned; missing action/context authority must still be rejected.
replace("test/acceptance/phase12_test.go", '''			if _, err = f.service.History(ctx, e, input.Source, input.Context, input.Dataset, 1); err == nil {
				t.Fatal("history crossed signed/private reach")
			}''', '''			rows, historyErr := f.service.History(ctx, e, input.Source, input.Context, input.Dataset, 1)
			if len(rows) != 0 {
				t.Fatal("history exposed another identity's private evidence")
			}
			if (!e.Valid() || !e.Has("cw.execution_context.use:*")) && historyErr == nil {
				t.Fatal("history accepted missing signed context authority")
			}''')

# The restore gate compares the actual migrated inventory, not a stale six-row literal.
replace("test/acceptance/phase02_test.go", 'count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations`) != 6', 'count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations`) != count(t, support.Raw(t, dsn), `SELECT count(*) FROM chartworks.schema_migrations`)')

# Configuration snapshots are serializable. Empty arrays must not become forbidden
# JSON null when a managed alias has no pre-existing tables or a policy list is empty.
replace("internal/config/sources.go", 'append([]SourceRelation(nil), s.Connections[i].Relations...)', 'append([]SourceRelation{}, s.Connections[i].Relations...)')
replace("internal/config/sources.go", 'append([]string(nil), s.Connections[i].Relations[j].Columns...)', 'append([]string{}, s.Connections[i].Relations[j].Columns...)')
replace("internal/config/engineering.go", 'append([]ProfilePolicy(nil), p.Policies...)', 'append([]ProfilePolicy{}, p.Policies...)')
replace("internal/config/engineering.go", 'append([]string(nil), p.Policies[i].RangeColumns...)', 'append([]string{}, p.Policies[i].RangeColumns...)')
replace("internal/config/config.go", 'append([]BrokerCredential(nil), v.Jobs.Credentials...)', 'append([]BrokerCredential{}, v.Jobs.Credentials...)')
replace("internal/config/config.go", 'append([]Provider(nil), v.Gateway.Bifrost.Providers...)', 'append([]Provider{}, v.Gateway.Bifrost.Providers...)')
replace("internal/config/config.go", 'func Defaults() Values {\n\treturn Values{', 'func Defaults() Values {\n\tv := Values{')
replace("internal/config/config.go", '''	}
}

// Overrides are explicit CLI overrides''', '''	}
	v.Gateway.Bifrost.Providers = []Provider{}
	v.Jobs.Credentials = []BrokerCredential{}
	return v
}

// Overrides are explicit CLI overrides''')

files = {
"internal/config/engineering_serialization_test.go": r'''package config

import (
 "bytes"
 "encoding/json"
 "testing"
)

func TestEngineeringSnapshotPreservesEmptyArrays(t *testing.T) {
 v := Defaults()
 v.Sources.Connections = []SourceConnection{{Tenant:"tenant", ID:"workspace", ManagedSchema:"cw_test", Relations:[]SourceRelation{}}}
 c := Config{values:v}
 for _, snapshot := range []Values{v, c.Values(), {Sources:v.Sources.Clone(), Profiling:v.Profiling.Clone()}} {
  // Test the concrete new fields separately from unrelated zero-value settings.
  for _, value := range []any{snapshot.Sources, snapshot.Profiling} {
   raw, err := json.Marshal(value)
   if err != nil { t.Fatal(err) }
   if err = checkJSON(json.NewDecoder(bytes.NewReader(raw)),0); err != nil { t.Fatalf("snapshot cannot pass the closed configuration decoder: %s: %v",raw,err) }
  }
 }
 raw, err := json.Marshal(c.Values())
 if err != nil { t.Fatal(err) }
 if err = checkJSON(json.NewDecoder(bytes.NewReader(raw)),0); err != nil { t.Fatalf("complete snapshot contains invalid JSON: %s: %v",raw,err) }
 original := DefaultProfiling()
 original.Policies = []ProfilePolicy{{ID:"p",Tenant:"t",Source:"s",RangeColumns:[]string{}}}
 copied := original.Clone()
 copied.Policies[0].RangeColumns = append(copied.Policies[0].RangeColumns,"amount")
 if len(original.Policies[0].RangeColumns)!=0 {t.Fatal("snapshot shares a policy column list")}
}
''',
"internal/engineering/profile_native_type_test.go": r'''package engineering

import "testing"

func TestProfileNativeTemporalTypes(t *testing.T) {
 for _, native := range []string{"date","timestamp","timestamptz","timestamp with time zone","timestamp without time zone","timestamp(0) with time zone","timestamp(6) without time zone","timestamptz(3)"} {
  if !profileTimeType(native) {t.Errorf("qualified native temporal type rejected: %q",native)}
 }
 for _, native := range []string{"time","timetz","interval","text","numeric(30,3)","analytics.timestamp","timestamp(9) with time zone","timestamp with time zone; SELECT 1"," timestamp","timestamp\n"} {
  if profileTimeType(native) {t.Errorf("unsupported type accepted: %q",native)}
 }
}
'''
}
for name, content in files.items():
    path = Path(name)
    if path.exists():
        raise RuntimeError(f"refusing to overwrite {name}")
    path.write_text(content)
subprocess.run(["git", "add", "--", *files], check=True)
print("Applied COPY end-of-data, native temporal display, complete migration restore, private-list isolation and config serialization fixes.")
