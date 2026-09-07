package acceptance

import (
	"context"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestSQLServerReadParameterBindings(t *testing.T) {
	f, relation := newWarehouseFixture(t, "sqlserver")
	source := f.create(t, "warehouse")
	integer := func(value string) readexec.Parameter {
		return readexec.Parameter{Kind: "integer", Value: value}
	}
	for _, test := range []struct {
		name, predicate string
		parameters      []readexec.Parameter
		rows            int
	}{
		{"single", "id > @p1", []readexec.Parameter{integer("0")}, 2},
		{"reused", "id > @p1 AND id <> @p1", []readexec.Parameter{integer("0")}, 2},
		{"reordered", "id > @p2 AND id < @p1", []readexec.Parameter{integer("100"), integer("0")}, 1},
		{"text", "id > 0 AND @p1 = 'bound'", []readexec.Parameter{{Kind: "text", Value: "bound"}}, 2},
		{"boolean", "id > 0 AND @p1 = CAST(1 AS bit)", []readexec.Parameter{{Kind: "boolean", Value: "true"}}, 2},
		{"null", "id > 0 AND @p1 IS NULL", []readexec.Parameter{{Kind: "null"}}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := f.plan(t, source, "SELECT id FROM "+relation+" WHERE "+test.predicate+" ORDER BY id", test.parameters...)
			out := warehouseExecute(t, f, plan, warehouseLimits())
			if len(out.Result.Rows) != test.rows || string(out.Result.Rows[0][0]) != `"2"` {
				t.Fatal("native parameter values or ordering changed")
			}
		})
	}
	for _, test := range []struct {
		name, predicate string
		parameters      []readexec.Parameter
	}{
		{"missing", "id > @p1", nil},
		{"extra", "id > @p1", []readexec.Parameter{integer("0"), integer("1")}},
		{"unbound_index", "id > @p2", []readexec.Parameter{integer("0")}},
		{"unbound_name", "id > @other", []readexec.Parameter{integer("0")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.validator.Validate(context.Background(), f.e, readexec.Request{
				Source: source.ID, Context: source.ContextID,
				SQL: "SELECT id FROM " + relation + " WHERE " + test.predicate, Parameters: test.parameters,
			})
			if err == nil {
				t.Fatal("invalid parameter binding admitted")
			}
		})
	}
}
