package charts_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

// The hashes were captured before adding the optional presentation extension.
func TestPresentationLegacyBytes(t *testing.T) {
	d, scalar := bind(t, charts.Table)
	rich := richFixture(t, "line_two_units")
	rm, _ := richBuild(t, rich)
	dd := displayData()
	tm, err := charts.BindDisplay(t.Context(), dd, charts.Table, charts.Bindings{Columns: []string{"period", "actual", "baseline"}}, []charts.Order{}, charts.DefaultOptions(), nil, &charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "period", Visible: true}, {Column: "actual", Visible: true}, {Column: "baseline", Visible: false}}, PageSize: 25, ShowTotals: true}, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	km, err := charts.BindDisplay(t.Context(), dd, charts.KPI, charts.Bindings{Value: "actual", Comparison: "baseline", Target: "target"}, []charts.Order{}, charts.DefaultOptions(), &charts.KPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowDelta: true, ShowPercentDelta: true, ShowTargetDifference: true, Thresholds: []charts.KPIThreshold{}}, nil, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                        string
		d                           charts.Data
		m                           charts.Mapping
		mapping, output, sourceRows string
	}{
		{"scalar", d, scalar, "515f30a5ce0b269e96428d598d7129d003c93e0e0694facd460eb19f67f66091", "d2550102f1b3db3426fd09ee5fc892a822e2b65bce8a388bcd6d0ba2a88e8c1d", "eee30db61f1eb92cc5b5067b3fa9a3dda1bb469701561dae30f2751cb66c13eb"},
		{"rich", rich.Data, rm, "fb8fe73d45ec847d9810a42989b68544342bc6515a39f7aa8af839993dd04171", "fe695aac7abf69ef9ee1526f839796facd41f3c295d7e84a95d6edee3f01e572", "fe695aac7abf69ef9ee1526f839796facd41f3c295d7e84a95d6edee3f01e572"},
		{"table", dd, tm, "27fe1fe380f8bac5c5b4407e7c8ce3f0f1d0e6906dd5f95ac01b199a0817c041", "864af07bc01df9929e6bd700c2fbaba4d6018fa10a54a20df230e74c56d9f42d", "cac5b9a09a6ae6de5e4dbcf628f49acbdfc539e6bf0c633a2449a033b889550e"},
		{"kpi", dd, km, "4110fd1fca855d39a869bdd861e19d9698b60822c5a886526e3685e139745d7f", "76d37b9c90cb9bdfe1352b1ea55cc775315ebe874fafb7f1e25bd45049372c1b", "76d37b9c90cb9bdfe1352b1ea55cc775315ebe874fafb7f1e25bd45049372c1b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := charts.Build(t.Context(), tc.d, tc.m, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			rows, err := charts.BuildWithSourceRows(t.Context(), tc.d, tc.m, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range []struct {
				name string
				v    any
				want string
			}{{"mapping", tc.m, tc.mapping}, {"output", out, tc.output}, {"sourceRows", rows, tc.sourceRows}} {
				wire, err := json.Marshal(item.v)
				if err != nil {
					t.Fatal(err)
				}
				got := fmt.Sprintf("%x", sha256.Sum256(wire))
				if got != item.want {
					t.Fatalf("legacy %s bytes changed: %s", item.name, got)
				}
			}
		})
	}
}
