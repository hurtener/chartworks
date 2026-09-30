//go:build cgo && (linux || darwin)

package sources

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bruinmysql "github.com/bruin-data/bruin/pkg/mysql"
	"github.com/bruin-data/bruin/pkg/query"
	mysqldriver "github.com/go-sql-driver/mysql"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestSQLRecoveryMySQLUniqueKeyShapesLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	for _, sql := range []string{
		"ALTER TABLE " + f.schema + ".items DROP PRIMARY KEY, ADD UNIQUE KEY full_key(id,quantity)",
		"CREATE UNIQUE INDEX expression_key ON " + f.schema + ".sales ((id+0))",
		"UPDATE " + f.schema + ".sales SET category=CONCAT(CHAR(65+id),id)",
		"CREATE UNIQUE INDEX prefix_key ON " + f.schema + ".sales(category(1))",
		"CREATE UNIQUE INDEX collation_key ON " + f.schema + ".sales(category)",
	} {
		if _, err := f.admin.ExecContext(t.Context(), sql); err != nil {
			t.Fatal(err)
		}
	}
	actual, err := f.service.probe(t.Context(), f.service.settings.Connections[0], f.binding.Source, f.binding.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range actual.Relations {
		switch r.Name {
		case "items":
			if r.HasUniqueKey([]string{"id"}) || !r.HasUniqueKey([]string{"quantity", "id"}) {
				t.Fatal("partial composite key admitted", r.UniqueKeys)
			}
		case "sales":
			if len(r.UniqueKeys) != 1 || !r.HasUniqueKey([]string{"id"}) || r.HasUniqueKey([]string{"category"}) {
				t.Fatal("prefix/expression/collation-dependent key borrowed", r.UniqueKeys)
			}
		}
	}
	legacy, err := f.service.probe(withStoredKeyPolicy(t.Context(), readexec.Binding{}), f.service.settings.Connections[0], f.binding.Source, f.binding.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range legacy.Relations {
		if len(r.UniqueKeys) > 0 {
			t.Fatal("legacy source gained uniqueness authority")
		}
	}
	if actual.Fingerprint == legacy.Fingerprint {
		t.Fatal("new keys absent from source fingerprint")
	}
}

func TestSQLRecoveryMySQLUniqueKeyTransactionFenceLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	connection := f.service.settings.Connections[0]
	client, location, err := f.service.mysqlClient(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	ddl := make(chan error, 1)
	go func() {
		select {
		case <-locked:
		case <-ctx.Done():
			ddl <- ctx.Err()
			return
		}
		conn, err := f.admin.Conn(ctx)
		if err != nil {
			ddl <- err
			return
		}
		defer conn.Close()
		if _, err = conn.ExecContext(ctx, "SET SESSION lock_wait_timeout=1"); err == nil {
			_, err = conn.ExecContext(ctx, "ALTER TABLE "+f.schema+".sales DROP PRIMARY KEY")
		}
		ddl <- err
	}()
	verify := func(ctx context.Context, session bruinmysql.ReadSession) error {
		actual, err := inspectMySQLContext(ctx, session, connection, f.binding.Source, f.binding.Revision, location)
		if err != nil {
			return err
		}
		if readexec.Hash(actual) != readexec.Hash(f.binding) {
			return readexec.ErrBinding
		}
		close(locked)
		select {
		case err := <-ddl:
			var native *mysqldriver.MySQLError
			if !errors.As(err, &native) || native.Number != 1205 {
				t.Errorf("index DDL was not blocked by verified read transaction: %v", err)
				return readexec.ErrBinding
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	stream, _, err := client.OpenReadVerified(ctx, &query.Query{Query: "SELECT sum(amount) FROM " + f.schema + ".sales"}, "key-fence-"+readexec.Hash(f.schema)[:20], discardMySQLObserver{}, bruinmysql.ReadOptions{RequireTLS: false}, verify)
	if err != nil {
		t.Fatal("verified read", err)
	}
	for stream.Next() {
		if _, err = stream.Values(); err != nil {
			_ = stream.Close()
			t.Fatal(err)
		}
	}
	if err = stream.Err(); err != nil {
		_ = stream.Close()
		t.Fatal(err)
	}
	if err = stream.Close(); err != nil {
		t.Fatal(err)
	}
	// Closing the original owned transaction releases MDL. Its previously
	// discovered key must then fail revalidation once DDL removes that evidence.
	if _, err = f.admin.ExecContext(ctx, "ALTER TABLE "+f.schema+".sales DROP PRIMARY KEY"); err != nil {
		t.Fatal("released DDL", err)
	}
	observed, probeErr := f.service.probe(ctx, connection, f.binding.Source, f.binding.Revision, nil)
	if probeErr != nil || readexec.Hash(observed) == readexec.Hash(f.binding) {
		t.Fatal("removed key not reflected in physical binding", probeErr)
	}
	if _, err = f.validator.Validate(ctx, f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: "SELECT sum(amount) FROM " + f.schema + ".sales"}); !errors.Is(err, readexec.ErrBinding) && !errors.Is(err, readexec.ErrUnsafe) {
		t.Fatal("stale unique key not denied by native safety boundary", err)
	}
}

func TestSQLRecoveryMySQLPhysicalJoinLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	if _, err := f.admin.ExecContext(t.Context(), "INSERT INTO "+f.schema+".items VALUES(5,9)"); err != nil {
		t.Fatal(err)
	}
	var items, sales readexec.Relation
	for _, r := range f.binding.Relations {
		switch r.Name {
		case "items":
			items = r
		case "sales":
			sales = r
		}
	}
	for _, join := range []string{"inner", "left"} {
		statement := "SELECT s.category,sum(i.quantity) AS quantity FROM " + f.schema + ".items i " + join + " JOIN " + f.schema + ".sales s ON i.id=s.id GROUP BY s.category"
		plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: statement})
		if err != nil {
			t.Fatal("native join", err)
		}
		contract := readexec.AnalyticalContract{Version: readexec.AnalyticalGroupedPopulationsVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("reviewed-quantity-by-category"), Dataset: items.ID, Metrics: []readexec.AnalyticalMetric{{ID: "quantity", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "quantity"}}}, Grain: &readexec.AnalyticalGrain{Policy: readexec.AnalyticalGroupingPolicy, Columns: []string{sales.ID + "/category"}, Dimensions: []string{"category"}}, Joins: []readexec.AnalyticalJoin{{Left: items.ID, Right: sales.ID, Type: join, LeftColumns: []string{"id"}, RightColumns: []string{"id"}}}, Intent: &readexec.AnalyticalIntent{Policy: readexec.AnalyticalIntentPolicy}, QueryPopulation: &readexec.AnalyticalQueryPopulation{Policy: readexec.AnalyticalQueryPopulationPolicy}}
		if proof, err := readexec.CheckAnalyticalPlan(t.Context(), plan, contract); err != nil || proof == nil {
			t.Fatal("physical join proof", join, err)
		}
		out, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 5 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, readexec.Hash(join)[:32], &cloudObserverCapture{})
		if err != nil {
			t.Fatal("physical join read", err)
		}
		got := map[string]string{}
		for _, row := range out.Result.Rows {
			label := "NULL"
			if string(row[0]) != "null" {
				if json.Unmarshal(row[0], &label) != nil {
					t.Fatal("typed dimension")
				}
			}
			var amount string
			if json.Unmarshal(row[1], &amount) != nil {
				t.Fatal("exact decimal")
			}
			got[label] = amount
		}
		if got["A"] != "7.00" || join == "inner" && len(got) != 1 || join == "left" && (len(got) != 2 || got["NULL"] != "9.00") {
			t.Fatal("join population result", join, got)
		}
		wrong := contract
		wrong.Joins = append([]readexec.AnalyticalJoin(nil), contract.Joins...)
		if join == "left" {
			wrong.Joins[0].Type = "inner"
		} else {
			wrong.Joins[0].Type = "left"
		}
		if proof, err := readexec.CheckAnalyticalPlan(t.Context(), plan, wrong); err == nil || proof != nil {
			t.Fatal("changed reviewed join acquired proof")
		}
	}
}
