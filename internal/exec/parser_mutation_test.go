package exec

import (
	"encoding/json"
	"fmt"
	"testing"

	pgquery "github.com/wasilibs/go-pgquery"
)

// Parser upgrades must not silently turn new AST fields into authorizing syntax.
// Mutations start from real libpg_query output and change one recognized node at
// a time; this is a defensive contract test, not an alternative fake parser.
func TestSQLRejectsUnknownASTFields(t *testing.T) {
	corpus := []string{
		`WITH q(x) AS (SELECT id FROM analytics.sales) SELECT q.x FROM q ORDER BY x`,
		`SELECT s.id FROM analytics.sales AS s JOIN analytics.items AS i ON s.id=i.sale_id`,
		`SELECT x.id FROM (SELECT id FROM analytics.sales) AS x`,
		`SELECT CASE WHEN amount IS NULL THEN 0 ELSE amount END AS n FROM analytics.sales`,
		`SELECT CASE id WHEN 1 THEN amount ELSE 0 END FROM analytics.sales`,
		`SELECT coalesce(amount,0), greatest(amount,1), amount::numeric(18,2) FROM analytics.sales`,
		`SELECT sum(amount) FILTER (WHERE amount>0) OVER (PARTITION BY active ORDER BY id ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) FROM analytics.sales`,
		`SELECT id FROM analytics.sales WHERE active IS TRUE AND id IN (SELECT sale_id FROM analytics.items)`,
		`SELECT ARRAY[id,2], ROW(id,amount), current_timestamp FROM analytics.sales`,
		`SELECT id,sum(amount) FROM analytics.sales GROUP BY GROUPING SETS ((id),())`,
		`SELECT id FROM analytics.sales UNION ALL SELECT sale_id FROM analytics.items ORDER BY id LIMIT $1`,
		`VALUES (1,'a'),(2,'b')`,
	}
	var total int
	for _, sql := range corpus {
		raw, err := pgquery.ParseToJSON(sql)
		if err != nil {
			t.Fatal(err)
		}
		var original map[string]any
		if err := json.Unmarshal([]byte(raw), &original); err != nil {
			t.Fatal(err)
		}
		var nodes []map[string]any
		var walk func(any)
		walk = func(v any) {
			switch v := v.(type) {
			case map[string]any:
				for key, body := range v {
					if len(key) > 0 && key[0] >= 'A' && key[0] <= 'Z' {
						if m, ok := body.(map[string]any); ok {
							nodes = append(nodes, m)
						}
					}
					// These typed pointers are not wrapped in a Node union.
					if key == "withClause" || key == "alias" || key == "typeName" || key == "over" {
						if m, ok := body.(map[string]any); ok {
							nodes = append(nodes, m)
						}
					}
					walk(body)
				}
			case []any:
				for _, value := range v {
					walk(value)
				}
			}
		}
		walk(original)
		for _, node := range nodes {
			node["unproven_extension"] = true
			r := sqlResolver{binding: parserBinding(), dependencies: map[string]bool{}, parameters: map[int]bool{}, parameterCount: 1}
			_, err := r.selectStatement(object(array(original["stmts"])[0])["stmt"], nil)
			delete(node, "unproven_extension")
			if err == nil {
				t.Fatalf("unknown AST field accepted in %s: %v", sql, node)
			}
			total++
		}
	}
	if total < 100 {
		t.Fatal("mutation inventory unexpectedly incomplete", total)
	}
	t.Logf("rejected %d independent parser-node extension mutations", total)
}

func TestSQLResolverNestedFailureBoundaries(t *testing.T) {
	for i, sql := range []string{
		`WITH q AS (SELECT id FROM analytics.sales),q AS (SELECT id FROM analytics.sales) SELECT id FROM q`,
		`WITH q(a,b) AS (SELECT id FROM analytics.sales) SELECT a FROM q`,
		`WITH q("Unqualified") AS (SELECT id FROM analytics.sales) SELECT 1 FROM q`,
		`SELECT secret FROM analytics.sales UNION SELECT sale_id FROM analytics.items`,
		`SELECT id FROM analytics.sales UNION SELECT secret FROM analytics.items`,
		`SELECT id,amount FROM analytics.sales UNION SELECT sale_id FROM analytics.items`,
		`SELECT id FROM analytics.sales UNION SELECT sale_id FROM analytics.items ORDER BY secret`,
		`VALUES (1),(1,2)`, `VALUES (pg_read_file('unsafe'))`,
		`SELECT id AS "Unsupported" FROM analytics.sales`,
		`SELECT id FROM analytics.sales LIMIT pg_sleep(1)`,
		`SELECT x.id FROM (SELECT secret FROM analytics.sales) AS x`,
		`SELECT x.id FROM analytics.sales AS x JOIN analytics.items AS x ON true`,
		`SELECT id FROM analytics.missing JOIN analytics.items ON true`,
		`SELECT id FROM analytics.sales JOIN analytics.missing ON true`,
		`SELECT row_number() OVER missing FROM analytics.sales`,
		`SELECT row_number() OVER (ORDER BY secret) FROM analytics.sales`,
		`SELECT sum(amount) OVER (ROWS BETWEEN pg_sleep(1) PRECEDING AND CURRENT ROW) FROM analytics.sales`,
		`SELECT id FROM analytics.sales WHERE id>0 AND secret='x'`,
		`SELECT secret IS NULL FROM analytics.sales`,
		`SELECT sum(secret) FROM analytics.sales`,
		`SELECT coalesce(secret,'x'),greatest(secret,'x') FROM analytics.sales`,
		`SELECT CASE secret WHEN 'a' THEN 1 ELSE 0 END FROM analytics.sales`,
		`SELECT CASE WHEN secret='a' THEN 1 ELSE 0 END FROM analytics.sales`,
		`SELECT CASE WHEN id=1 THEN secret ELSE 'x' END FROM analytics.sales`,
		`SELECT CASE WHEN id=1 THEN 'x' ELSE secret END FROM analytics.sales`,
		`SELECT secret::text FROM analytics.sales`,
		`SELECT ARRAY[secret], ROW(secret) FROM analytics.sales`,
		`SELECT id FROM analytics.sales WHERE secret IN (SELECT name FROM analytics.sales)`,
		`SELECT current_user`,
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, _, err := resolveFixture(sql); err == nil {
				t.Fatal("unsafe nested dependency accepted", sql)
			}
		})
	}
}
