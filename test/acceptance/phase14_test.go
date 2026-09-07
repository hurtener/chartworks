package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Cloud protocol and recorded Service lifecycle suites run separately in CI.
// These named criteria exercise real self-hosted engines, never cloud cutover.
func TestPhase14(t *testing.T) {
	for _, criterion := range []string{"AC01", "AC02", "AC03", "AC04", "AC05", "AC06"} {
		t.Run(criterion, func(t *testing.T) {
			for _, dialect := range []string{"postgres", "mysql", "sqlserver"} {
				t.Run(dialect, func(t *testing.T) {
					f, relation := newWarehouseFixture(t, dialect)
					src := f.create(t, "warehouse")
					ctx := context.Background()
					switch criterion {
					case "AC01":
						discovery, err := f.s.Discover(ctx, f.e, src.ID)
						if err != nil || len(discovery.Relations) == 0 || discovery.ContextID != src.ContextID {
							t.Fatal("native discovery", err)
						}
						p := f.plan(t, src, "SELECT id,amount,payload FROM "+relation+" ORDER BY id")
						out := warehouseExecute(t, f, p, warehouseLimits())
						if len(out.Result.Rows) != 2 || len(out.Result.Schema) != 3 {
							t.Fatal("schema/row shape")
						}
						wire, _ := json.Marshal(src)
						if strings.Contains(string(wire), "PASSWORD") || strings.Contains(string(wire), "dsn") {
							t.Fatal("secret in public projection")
						}
						if dialect != "postgres" {
							row := out.Result.Rows[1]
							if string(row[0]) != `"9007199254740993"` || string(row[1]) != `"12345678901234567890.123456789"` || string(row[2]) != `"00ff"` {
								t.Fatalf("native precision changed: %s", row)
							}
							if string(out.Result.Rows[0][1]) != "null" {
								t.Fatal("null became a value")
							}
						}
					case "AC02":
						for _, statement := range []string{"SELECT id FROM " + relation + "; SELECT id FROM " + relation, "SELECT id FROM " + relation + "_missing", "DELETE FROM " + relation} {
							if _, err := f.validator.Validate(ctx, f.e, readexec.Request{Source: src.ID, Context: src.ContextID, SQL: statement}); err == nil {
								t.Fatal("unsupported statement admitted")
							}
						}
						p := f.plan(t, src, "SELECT id FROM "+relation+" ORDER BY id")
						if len(warehouseExecute(t, f, p, warehouseLimits()).Result.Rows) != 2 {
							t.Fatal("valid native plan failed")
						}
					case "AC03":
						other := f.actor(t, "source-b", "reader")
						if _, err := f.s.Get(ctx, other, src.ID); err == nil {
							t.Fatal("foreign source read")
						}
						restricted := f.token.envelope(t, "source-a", "reader", "sources.read", "cw.source.read:"+src.ID, "cw.execution_context.use:other:v1")
						if _, err := f.s.Discover(ctx, restricted, src.ID); err == nil {
							t.Fatal("wrong context discovery")
						}
						if dialect != "postgres" {
							warehouseReadOnly(t, f, relation, dialect)
						} else {
							reader := f.readDSN()
							conn := support.Raw(t, reader)
							if _, err := conn.Exec(ctx, "UPDATE "+relation+" SET id=id"); err == nil {
								t.Fatal("reader write succeeded")
							}
						}
					case "AC04":
						marker := "$1"
						if dialect == "mysql" {
							marker = "?"
						}
						if dialect == "sqlserver" {
							marker = "@p1"
						}
						p := f.plan(t, src, "SELECT id FROM "+relation+" WHERE id > "+marker+" ORDER BY id", readexec.Parameter{Kind: "integer", Value: "0"})
						limits := warehouseLimits()
						limits.Rows = 1
						out := warehouseExecute(t, f, p, limits)
						if len(out.Result.Rows) != 1 {
							t.Fatal("row ceiling not enforced")
						}
						if out.Result.Truncation != "rows" {
							t.Fatal("row cap lost truncation evidence")
						}
						bytePlan := f.plan(t, src, "SELECT id AS bounded_identifier_for_serialized_schema_accounting FROM "+relation+" ORDER BY id")
						full := warehouseExecute(t, f, bytePlan, warehouseLimits())
						limits.Rows = 100
						limits.Bytes = full.Result.Bytes - 1
						if limits.Bytes < 128 {
							t.Fatal("byte fixture too small")
						}
						if full.Result.Bytes > limits.Bytes {
							bounded := warehouseExecute(t, f, bytePlan, limits)
							if bounded.Result.Bytes > limits.Bytes || bounded.Result.Truncation != "bytes" {
								t.Fatal("serialized byte ceiling not enforced")
							}
						}
						limits = warehouseLimits()
						active, stop := context.WithCancel(ctx)
						observer := &warehouseCancelObserver{cancel: stop}
						cancelledOut, cancelErr := f.s.ExecuteRead(active, f.e, p, limits, "20112233445566778899aabbccddeeff", observer)
						stop()
						if cancelErr == nil || !observer.acknowledged || cancelledOut.RemoteState != "stopped" {
							t.Fatal("acknowledged cancellation cleanup", cancelErr, cancelledOut.RemoteState)
						}

						cancelled, cancel := context.WithCancel(ctx)
						cancel()
						if _, err := f.s.ExecuteRead(cancelled, f.e, p, limits, "00112233445566778899aabbccddeeff", nil); err == nil {
							t.Fatal("cancelled request executed")
						}
						rotated, err := f.s.Rotate(ctx, f.e, src.ID, src.Revision)
						if err != nil || rotated.ContextID == src.ContextID {
							t.Fatal("rotation", err)
						}
						if _, err := f.s.ExecuteRead(ctx, f.e, p, limits, "10112233445566778899aabbccddeeff", nil); !errors.Is(err, readexec.ErrBinding) {
							t.Fatal("old context executed after rotation", err)
						}
					case "AC05":
						// Independent creation against the same real namespace must be repeatable;
						// immutable registry identities remain separate from physical tables.
						second := f.create(t, "warehouse-second")
						if second.ID == src.ID || second.ContextID == src.ContextID {
							t.Fatal("source identity collision")
						}
						a := warehouseExecute(t, f, f.plan(t, src, "SELECT id FROM "+relation+" ORDER BY id"), warehouseLimits())
						b := warehouseExecute(t, f, f.plan(t, second, "SELECT id FROM "+relation+" ORDER BY id"), warehouseLimits())
						x, _ := json.Marshal(a.Result.Rows)
						y, _ := json.Marshal(b.Result.Rows)
						if string(x) != string(y) {
							t.Fatal("repeatable seed/read changed")
						}
					case "AC06":
						bad := f.cfg
						bad.Connections = append([]config.SourceConnection(nil), bad.Connections...)
						bad.Connections[0].Dialect = "unsupported"
						if config.ValidateSources(bad) == nil {
							t.Fatal("unknown engine accepted")
						}
						f.s.Close()
						if _, err := f.s.Test(ctx, f.e, src.ID); !errors.Is(err, store.ErrUnavailable) {
							t.Fatal("closed source silently reopened", err)
						}
						metadataConfig := f.cfg
						metadataConfig.Enabled = false
						metadata, err := sources.New(f.db, metadataConfig, func(string) (string, bool) { return "", false })
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(metadata.Close)
						if _, err := metadata.Get(ctx, f.e, src.ID); err != nil {
							t.Fatal("retained metadata requires driver", err)
						}
					}
				})
			}
		})
	}
}

type warehouseCancelObserver struct {
	cancel       context.CancelFunc
	acknowledged bool
}

func (o *warehouseCancelObserver) Check(context.Context) error { return nil }
func (o *warehouseCancelObserver) Dispatch(_ context.Context, id readexec.RemoteQuery, ack bool) error {
	if !id.Valid() {
		return readexec.ErrBinding
	}
	if ack {
		o.acknowledged = true
		o.cancel()
	}
	return nil
}
