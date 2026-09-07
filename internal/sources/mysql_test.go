package sources

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestMySQLSourceProbeLocal(t *testing.T) {
	dsn := os.Getenv("CHARTWORKS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CHARTWORKS_TEST_MYSQL_DSN is not set")
	}
	settings := config.DefaultSources()
	service := &Service{settings: settings, lookup: func(name string) (string, bool) { return dsn, name == "MYSQL_READ_DSN" }, pools: map[string]poolEntry{}, mysqlPools: map[string]mysqlPoolEntry{}, retiring: map[string]bool{}}
	t.Cleanup(service.Close)
	connection := config.SourceConnection{Dialect: "mysql", Tenant: "tenant", ID: "mysql", Version: "v1", ReadDSN: "env:MYSQL_READ_DSN", Relations: []config.SourceRelation{{Schema: "cw_bruin_probe", Name: "cw_chartworks_fixture", Columns: []string{"id", "amount", "payload"}}}}
	binding, err := service.probe(t.Context(), connection, "source", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Dialect != "mysql" || len(binding.Relations) != 1 || len(binding.Relations[0].Columns) != 3 || binding.Relations[0].Columns[1].Category != "decimal" {
		t.Fatalf("unexpected MySQL binding: %#v", binding)
	}
}

func TestMySQLResultTypesStayExact(t *testing.T) {
	decimal, err := mysqlResultField("amount", "DECIMAL")
	if err != nil {
		t.Fatal(err)
	}
	collector, err := readexec.NewCollector([]readexec.Field{decimal}, 2, 1024)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := mysqlResultValue(decimal, []byte("9007199254740993.125"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = collector.Add([][]byte{raw}); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(collector.Result().Rows[0][0])
	if string(got) != `"9007199254740993.125"` {
		t.Fatalf("decimal changed: %s", got)
	}
	binary, err := mysqlResultField("payload", "VARBINARY")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := mysqlResultValue(binary, []byte{0, 255})
	if err != nil || string(encoded) != `\x00ff` {
		t.Fatalf("binary changed: %q %v", encoded, err)
	}
}
