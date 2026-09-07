package sources

import (
	"context"
	"crypto/rand"
	dbsql "database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestMySQLSourceProbeLocal(t *testing.T) {
	adminDSN := os.Getenv("CHARTWORKS_TEST_MYSQL_DSN")
	if adminDSN == "" {
		t.Skip("CHARTWORKS_TEST_MYSQL_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "cw_probe_" + hex.EncodeToString(nonce[:])
	password := "SYNTHETIC_probe_Password9!"
	admin, err := dbsql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, cleanupErr := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name); cleanupErr != nil {
			t.Errorf("drop synthetic MySQL database: %v", cleanupErr)
		}
		if _, cleanupErr := admin.ExecContext(ctx, "DROP USER IF EXISTS '"+name+"'@'%'"); cleanupErr != nil {
			t.Errorf("drop synthetic MySQL reader: %v", cleanupErr)
		}
		if cleanupErr := admin.Close(); cleanupErr != nil {
			t.Errorf("close synthetic MySQL administrator: %v", cleanupErr)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	exec := func(statement string) {
		t.Helper()
		if _, execErr := admin.ExecContext(ctx, statement); execErr != nil {
			t.Fatal("synthetic MySQL probe setup", execErr)
		}
	}
	exec("CREATE DATABASE " + name)
	exec("CREATE TABLE " + name + ".sales(id BIGINT PRIMARY KEY,amount DECIMAL(30,9),payload VARBINARY(32))")
	exec("INSERT INTO " + name + ".sales VALUES(1,9007199254740993.125,X'00ff')")
	exec("CREATE USER '" + name + "'@'%' IDENTIFIED BY '" + password + "'")
	exec("GRANT SELECT ON " + name + ".* TO '" + name + "'@'%'")
	cfg.User, cfg.Passwd, cfg.DBName = name, password, name
	readerDSN := cfg.FormatDSN()
	settings := config.DefaultSources()
	service := &Service{settings: settings, lookup: func(name string) (string, bool) { return readerDSN, name == "MYSQL_READ_DSN" }, pools: map[string]poolEntry{}, mysqlPools: map[string]mysqlPoolEntry{}, retiring: map[string]bool{}}
	t.Cleanup(service.Close)
	connection := config.SourceConnection{Dialect: "mysql", AllowInsecureLocal: true, Tenant: "tenant", ID: "mysql", Version: "v1", ReadDSN: "env:MYSQL_READ_DSN", Relations: []config.SourceRelation{{Schema: name, Name: "sales", Columns: []string{"id", "amount", "payload"}}}}
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

func TestMySQLSourceRejectsPlaintextRemoteDSN(t *testing.T) {
	service := &Service{settings: config.DefaultSources(), lookup: func(string) (string, bool) {
		return "reader:secret@tcp(db.example:3306)/analytics", true
	}, mysqlPools: map[string]mysqlPoolEntry{}}
	_, _, err := service.mysqlClient(t.Context(), config.SourceConnection{Dialect: "mysql", Tenant: "tenant", ID: "mysql", Version: "v1", ReadDSN: "env:MYSQL_READ_DSN"})
	if err != readexec.ErrUnsafe {
		t.Fatalf("plaintext remote DSN was not denied: %v", err)
	}
}

type mysqlCleanupReceipt struct {
	cause   error
	stopped bool
}

func (e *mysqlCleanupReceipt) Error() string     { return e.cause.Error() }
func (e *mysqlCleanupReceipt) Unwrap() error     { return e.cause }
func (e *mysqlCleanupReceipt) ReadStopped() bool { return e.stopped }

func TestMySQLReadCleanupStateRequiresReceipt(t *testing.T) {
	t.Parallel()
	if state, ok := mysqlReadCleanupState(nil); !ok || state != "stopped" {
		t.Fatalf("successful cleanup state=%q known=%t", state, ok)
	}
	if state, ok := mysqlReadCleanupState(context.Canceled); ok || state != "" {
		t.Fatalf("request cancellation alone claimed cleanup state=%q known=%t", state, ok)
	}
	for _, stopped := range []bool{false, true} {
		receipt := &mysqlCleanupReceipt{cause: context.Canceled, stopped: stopped}
		state, ok := mysqlReadCleanupState(receipt)
		want := "unknown"
		if stopped {
			want = "stopped"
		}
		if !ok || state != want {
			t.Fatalf("cleanup receipt stopped=%t, state=%q known=%t", stopped, state, ok)
		}
		if !errors.Is(receipt, context.Canceled) {
			t.Fatal("cleanup receipt lost the request failure")
		}
	}
}
