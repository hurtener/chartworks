//go:build cgo && (linux || darwin)

package sources

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
)

// Real MySQL catalog, native EXPLAIN and public-probe execution. Durable example
// lifecycle is separately exercised through PostgreSQL metadata acceptance.
func TestSQLRecoveryMySQLLearningDomainsLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t, "+00:00")
	admin, err := f.admin.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(t.Context(), "SET SESSION time_zone='+00:00'"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(t.Context(), "UPDATE "+f.schema+".sales SET created_at='2000-01-02 03:04:05',created_timestamp='2000-01-02 03:04:05'"); err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct{ name, sql, domain string }{
		{"date_cast", "SELECT id FROM " + f.schema + ".sales WHERE CAST(created_at AS DATE)=CAST(? AS DATE)", "date"},
		{"datetime_cast", "SELECT id FROM " + f.schema + ".sales WHERE created_at=CAST(? AS DATETIME)", "timestamp"},
		{"datetime_column", "SELECT id FROM " + f.schema + ".sales WHERE created_at=?", "timestamp"},
		{"timestamp_column", "SELECT id FROM " + f.schema + ".sales WHERE created_timestamp=?", "timestamp"},
		{"time_cast", "SELECT id FROM " + f.schema + ".sales WHERE CAST(? AS TIME) IS NOT NULL", "time"},
		{"json_cast", "SELECT id FROM " + f.schema + ".sales WHERE CAST(? AS JSON) IS NOT NULL", "json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy, _ := exampleparams.New([]string{"text"})
			schema, err := legacy.WithDomains([]string{tc.domain})
			if err != nil {
				t.Fatal(err)
			}
			probes, err := schema.ProbeValues()
			if err != nil {
				t.Fatal(err)
			}
			request := readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: tc.sql, Parameters: []readexec.Parameter{{Kind: "text", Value: probes[0]}}}
			plan, err := f.validator.Validate(t.Context(), f.envelope, request)
			if err != nil {
				t.Fatal("fixed public-probe native validation", err)
			}
			domains, err := plan.LearningParameterDomains(t.Context(), f.envelope, f.binding)
			if err != nil || !reflect.DeepEqual(domains, []string{tc.domain}) {
				t.Fatal("catalog/cast domain custody", domains, err)
			}
			result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", 701+i), &cloudObserverCapture{})
			if err != nil || len(result.Result.Rows) != 4 {
				t.Fatal("public probe actual result", len(result.Result.Rows), err)
			}
			wrong := f.binding.Clone()
			wrong.Revision++
			if _, err := plan.LearningParameterDomains(t.Context(), f.envelope, wrong); err == nil {
				t.Fatal("retained wrong source domain proof")
			}
		})
	}
}
