package acceptance

import (
	"context"
	"crypto/rand"
	dbsql "database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	bruinmssql "github.com/bruin-data/bruin/pkg/mssql"
	bruinquery "github.com/bruin-data/bruin/pkg/query"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/sources"
	_ "github.com/microsoft/go-mssqldb"
)

// Each fixture owns a random namespace and a SELECT-only synthetic account.
// The supplied DSN is the disposable container administrator, never a live account.
func newWarehouseFixture(t *testing.T, dialect string) (*sourceFixture, string) {
	t.Helper()
	f := newSourceFixture(t, nil)
	if dialect == "postgres" {
		return f, "analytics.sales"
	}
	env := "CHARTWORKS_TEST_MYSQL_DSN"
	if dialect == "sqlserver" {
		env = "CHARTWORKS_TEST_SQLSERVER_DSN"
	}
	raw := os.Getenv(env)
	if raw == "" {
		t.Fatalf("%s is required for real phase14 acceptance", env)
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "cw_p14_" + hex.EncodeToString(nonce[:])
	password := "SYNTHETIC_p14_Password9!"
	admin, err := dbsql.Open(dialect, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exec := func(statement string) {
		t.Helper()
		if _, e := admin.ExecContext(ctx, statement); e != nil {
			t.Fatal("synthetic warehouse seed", e)
		}
	}
	var reader string
	if dialect == "mysql" {
		cfg, e := mysql.ParseDSN(raw)
		if e != nil {
			t.Fatal(e)
		}
		exec("CREATE DATABASE " + name)
		exec("CREATE TABLE " + name + ".sales(id BIGINT PRIMARY KEY,amount DECIMAL(30,9),payload VARBINARY(32))")
		exec("INSERT INTO " + name + ".sales VALUES(9007199254740993,12345678901234567890.123456789,X'00ff'),(2,NULL,NULL)")
		exec("CREATE USER '" + name + "'@'%' IDENTIFIED BY '" + password + "'")
		exec("GRANT SELECT ON " + name + ".* TO '" + name + "'@'%'")
		cfg.User, cfg.Passwd, cfg.DBName = name, password, name
		reader = cfg.FormatDSN()
		t.Cleanup(func() {
			_, _ = admin.ExecContext(context.Background(), "DROP DATABASE "+name)
			_, _ = admin.ExecContext(context.Background(), "DROP USER '"+name+"'@'%'")
		})
	} else {
		exec("CREATE SCHEMA [" + name + "]")
		exec("CREATE TABLE [" + name + "].sales(id BIGINT PRIMARY KEY,amount DECIMAL(30,9),payload VARBINARY(32))")
		exec("INSERT INTO [" + name + "].sales VALUES(9007199254740993,12345678901234567890.123456789,0x00ff),(2,NULL,NULL)")
		exec("CREATE LOGIN [" + name + "] WITH PASSWORD='" + password + "', CHECK_POLICY=OFF")
		exec("CREATE USER [" + name + "] FOR LOGIN [" + name + "]")
		exec("GRANT SELECT, VIEW DEFINITION ON SCHEMA::[" + name + "] TO [" + name + "]")
		exec("GRANT SHOWPLAN TO [" + name + "]")
		u, e := url.Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		u.User = url.UserPassword(name, password)
		reader = u.String()
		t.Cleanup(func() {
			for _, q := range []string{"DROP TABLE [" + name + "].sales", "DROP SCHEMA [" + name + "]", "DROP USER [" + name + "]", "DROP LOGIN [" + name + "]"} {
				_, _ = admin.ExecContext(context.Background(), q)
			}
		})
	}
	f.s.Close()
	f.setReadDSN(reader)
	f.cfg.Connections[0].Dialect = dialect
	f.cfg.Connections[0].AllowInsecureLocal = true
	f.cfg.Connections[0].Relations = []config.SourceRelation{{Schema: name, Name: "sales", Columns: []string{"id", "amount", "payload"}}}
	f.s, err = sources.New(f.db, f.cfg, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.s.Close)
	f.validator, err = readexec.NewValidator(f.s, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	return f, name + ".sales"
}

func warehouseLimits() readexec.Limits {
	return readexec.Limits{Rows: 100, Bytes: 8192, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e9}
}
func warehouseExecute(t *testing.T, f *sourceFixture, p readexec.Plan, limits readexec.Limits) readexec.NativeResult {
	t.Helper()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.ExecuteRead(context.Background(), f.e, p, limits, hex.EncodeToString(nonce[:]), nil)
	if err != nil {
		t.Fatal("native source execution", err)
	}
	if out.RemoteState != "stopped" {
		t.Fatalf("native cleanup not confirmed: %s", out.RemoteState)
	}
	return out
}
func warehouseReadOnly(t *testing.T, f *sourceFixture, relation, dialect string) {
	t.Helper()
	db, err := dbsql.Open(dialect, f.readDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// #nosec G202 -- relation is the fixture-owned schema/table with a fixed prefix and random hex suffix.
	if _, err = db.ExecContext(ctx, "UPDATE "+relation+" SET id=id"); err == nil {
		t.Fatal("synthetic reader has write permission")
	}
	if strings.Contains(err.Error(), sourcePassword) {
		t.Fatal("driver exposed synthetic password")
	}
}

// This controlled native leaf fixture exercises cancellation while the server is
// still waiting; public SQL admission is tested separately by TestPhase14.
func warehouseSQLServerActiveCancel(t *testing.T, f *sourceFixture) {
	t.Helper()
	u, err := url.Parse(f.readDSN())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	client, err := bruinmssql.NewDB(&bruinmssql.Config{Host: u.Hostname(), Port: port, Username: u.User.Username(), Password: password, Database: u.Query().Get("database"), Query: u.RawQuery})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	observer := &warehouseSQLServerObserver{}
	started := time.Now()
	rows, _, err := client.OpenRead(context.Background(), &bruinquery.Query{Query: "WAITFOR DELAY '00:00:05'; SELECT 1 AS completed"}, "fixture-"+hex.EncodeToString(nonce[:]), observer, bruinmssql.ReadOptions{Timeout: 100 * time.Millisecond, CancelTimeout: time.Second})
	if rows != nil {
		_ = rows.Close()
	}
	var failure *bruinmssql.ReadFailure
	if !observer.dispatched || !errors.As(err, &failure) || !failure.Stopped || time.Since(started) > 4*time.Second {
		t.Fatal("native wait cancellation/cleanup was not confirmed", err)
	}
	admin, err := dbsql.Open("sqlserver", os.Getenv("CHARTWORKS_TEST_SQLSERVER_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		if err = admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE session_id=@p1 AND context_info=@p2", observer.identity.SessionID, []byte(observer.identity.AttemptTag)).Scan(&count); err != nil {
			t.Fatal("native cancellation reconciliation", err)
		}
		if count == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("cancelled native session remained active")
		case <-ticker.C:
		}
	}
}

type warehouseSQLServerObserver struct {
	identity   bruinmssql.ReadIdentity
	dispatched bool
}

func (o *warehouseSQLServerObserver) OnDispatch(_ context.Context, id bruinmssql.ReadIdentity) error {
	o.identity = id
	o.dispatched = true
	return nil
}
func (o *warehouseSQLServerObserver) OnAcknowledged(context.Context, bruinmssql.ReadIdentity) error {
	return nil
}
