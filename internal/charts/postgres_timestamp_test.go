package charts_test

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func TestPostgresTimestampCharts(t *testing.T) {
	for _, kind := range []charts.Kind{charts.Line, charts.Area} {
		for _, stamp := range []string{"2026-03-08 05:00:00+00", "2026-03-08 02:00:00-03", "2026-03-08 10:30:00.123456+05:30"} {
			t.Run(string(kind)+"/"+stamp, func(t *testing.T) {
				data, bindings, order := chartfixtures.Fixture(kind, "binding")
				data.Rows = data.Rows[:1]
				data.Rows[0][0].Value = stamp
				mapping, err := charts.Bind(context.Background(), data, kind, bindings, order, charts.DefaultOptions(), charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				out, err := charts.Build(context.Background(), data, mapping, charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				if out.State != "ready" || len(out.Points) != 1 || out.Points[0].Category.Value != stamp {
					t.Fatalf("timestamp did not retain its original value: %+v", out)
				}
			})
		}
	}
}
