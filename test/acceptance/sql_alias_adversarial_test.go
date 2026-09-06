package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/jackc/pgx/v5"
)

// Physical input positions and PostgreSQL name resolution are not the positions
// or visibility of a filtered semantic projection. Use the real database as the
// resolution oracle, then assert denial before native planning/credential access.
func TestSQLAliasCannotExposeHiddenInputs(t *testing.T) {
	t.Run("physical_column_alias", func(t *testing.T) {
		f := newSourceFixture(t, func(c *config.Sources) {
			c.Connections[0].Relations = []config.SourceRelation{{Schema: "analytics", Name: "leaky", Columns: []string{"id"}}}
		})
		ctx := context.Background()
		if _, err := f.admin.Exec(ctx, `CREATE TABLE analytics.leaky(secret text,id integer); INSERT INTO analytics.leaky VALUES('HIDDEN_ALIAS_CANARY',1); GRANT SELECT ON analytics.leaky TO `+pgx.Identifier{f.role}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		source := f.create(t, "leaky")
		statement := `SELECT exposed FROM analytics.leaky AS t(exposed)`
		var actual string
		if err := f.admin.QueryRow(ctx, statement).Scan(&actual); err != nil || actual != "HIDDEN_ALIAS_CANARY" {
			t.Fatal("physical alias oracle did not select excluded first column", err)
		}
		before := f.lookups.Load()
		if plan, err := f.validator.Validate(ctx, f.e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: statement}); !errors.Is(err, readexec.ErrUnsafe) || plan.Receipt().Validated || f.lookups.Load() != before {
			t.Fatal("base-table positional alias bypassed semantic column boundary", err)
		}
		// Named table aliases and fully resolved derived-column aliases still work.
		for _, sql := range []string{`SELECT t.id FROM analytics.leaky AS t`, `SELECT q.exposed FROM (SELECT id FROM analytics.leaky) AS q(exposed)`, `WITH q AS (SELECT id FROM analytics.leaky) SELECT exposed FROM q AS x(exposed)`} {
			plan := f.plan(t, source, sql)
			rows, err := f.s.Read(ctx, f.e, plan)
			if err != nil || len(rows.Values) != 1 || *rows.Values[0][0] != "1" {
				t.Fatal("safe scoped alias regressed", err)
			}
		}
	})
	t.Run("input_vs_output_name_resolution", func(t *testing.T) {
		f := newSourceFixture(t, nil)
		ctx := context.Background()
		if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET secret=CASE id WHEN 1 THEN 'z' ELSE 'a' END`); err != nil {
			t.Fatal(err)
		}
		source := f.create(t, "sales")
		group := `SELECT 1 AS secret, count(id) FROM analytics.sales GROUP BY secret`
		rows, err := f.admin.Query(ctx, group)
		if err != nil {
			t.Fatal(err)
		}
		groups := 0
		for rows.Next() {
			groups++
		}
		err = rows.Err()
		rows.Close()
		if err != nil || groups != 2 {
			t.Fatal("GROUP BY oracle did not resolve hidden input before constant alias", groups, err)
		}
		order := `SELECT id AS secret FROM analytics.sales ORDER BY lower(secret)`
		var first int
		if err := f.admin.QueryRow(ctx, order).Scan(&first); err != nil || first != 2 {
			t.Fatal("nested ORDER BY oracle did not resolve hidden input", first, err)
		}
		before := f.lookups.Load()
		for _, sql := range []string{group, order, `SELECT id AS secret FROM analytics.sales ORDER BY secret || ''`, `SELECT id AS secret FROM analytics.sales ORDER BY secret::text`, `SELECT 1 AS secret, count(id) FROM analytics.sales GROUP BY GROUPING SETS ((secret),())`, `SELECT DISTINCT ON (secret) id AS secret FROM analytics.sales`} {
			if plan, err := f.validator.Validate(ctx, f.e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: sql}); !errors.Is(err, readexec.ErrUnsafe) || plan.Receipt().Validated || f.lookups.Load() != before {
				t.Fatal("hidden input name was promoted to visible output alias", err)
			}
		}
		plan := f.plan(t, source, `SELECT id AS secret FROM analytics.sales ORDER BY secret`)
		result, err := f.s.Read(ctx, f.e, plan)
		if err != nil || len(result.Values) != 2 || *result.Values[0][0] != "1" {
			t.Fatal("bare ORDER BY output alias regressed", err)
		}
	})
}
