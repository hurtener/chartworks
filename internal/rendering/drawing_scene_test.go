package rendering

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

var sceneCatalog = []charts.Kind{charts.Area, charts.Bar, charts.ColumnChart, charts.Donut, charts.GroupedBar, charts.Heatmap, charts.KPI, charts.Line, charts.Pie, charts.Scatter, charts.StackedBar, charts.StackedColumn, charts.Treemap}

func TestDrawingScenePreservesExistingSVG(t *testing.T) {
	golden := map[string]string{
		"area/light":           "21d48cead34e39b147685271be39eb6ef30beeb2d48b4ac847f0becbb75bd89a",
		"area/dark":            "dcc6a9bb665321f78a83508e6fe77934bdb05d4468f86e13fd9639895c9c7b3b",
		"bar/light":            "d4c69f6791edd87699f8974d328a129f991545eacb0f5e88c8fcb60dcb854d46",
		"bar/dark":             "cfd9cee63a6890c567c0980edfb604d4bfe2c2772ff7b551aab797cd9289ff45",
		"column/light":         "247adff37632616b5f1d56bf7c5e4ff293e9b4bf5dd09ccd5800aa615ea7a6a5",
		"column/dark":          "eba7d18c9fb13cceea06b08ef690bb83d3b0a727a443fa738251c85b5294aa96",
		"donut/light":          "2503b616fde3746072a09f0b12b159d450c36cc3aad4b763aba49b0376f18930",
		"donut/dark":           "b9cf87c61fb6594c10312619a4c90523083ba9142c3cae8614ab4cfc8affa07d",
		"grouped_bar/light":    "ec376bb4aa45dad4e96d6a11d660142814f870b3ec8694d766f0108dc15fdd36",
		"grouped_bar/dark":     "bdf36090278c39e2ec01d5c9de36f32afa74a4324d8c844aa5ed465280e2f80a",
		"heatmap/light":        "f092b9b015e32804c6bfba68e8947cf47520bc1647545a98286ccfb88450771a",
		"heatmap/dark":         "1b71bfdcbe98a5a543c52eada263d62d4cf7b6f9459b20f28022691b82df7d31",
		"kpi/light":            "7506f36ab68d5ee37fe03a45f0b927273ecafb989f87ae10683f1846a6216b70",
		"kpi/dark":             "606bbd04653d1c025fd31ce24bb67ebe8cff245c36274d17c5b2bd961654d392",
		"line/light":           "d90f6317eafd8010ed705aca488061b6682aff624d12fedf1f59dae68c718994",
		"line/dark":            "584f25fdce7449a44cdc3768b3a830194ae9d52877aef7809950eec5e195158f",
		"pie/light":            "01ae5e782b265f64d8b7fc7bc58a6c4ffca2e64a1464c29a1bd04021f3352c20",
		"pie/dark":             "152f9f27b261f5a3b5c678d58bb76f5e623a15ecd7c4e9d6dce47c92f1180e69",
		"scatter/light":        "56865c91a1896c7050c7325042720fd53639fd36cddf61bc46c24ce7743b7835",
		"scatter/dark":         "ce4f3bf773ce8d2cec4435370119124c3cd058178a234dfd9e13790c14a88839",
		"stacked_bar/light":    "5a1321124cfe79530bb8fcddab0a479fa54491180f5d8e379613c005e48b8581",
		"stacked_bar/dark":     "9aa7b18de1b07fb30f6921f9a66dfdb45fa3e9d111f186e883a6189fa9971d60",
		"stacked_column/light": "7b2c489dfdc48a8152c66f8e0074e887ebc95424b80348c70390055d75383923",
		"stacked_column/dark":  "43ecfecc7ac0ffda93edcb73ed123668a53efb81009c47100b088ede778d06cc",
		"treemap/light":        "e0d2d1516bbf11e3677c88c8d522397283d6ca581c0f2c9cbc61e68f1d3033b3",
		"treemap/dark":         "14caf49d89c19bdbeb559624f355fc29781371fd05f9f79b3683a067bc61b8e3",
	}
	for _, kind := range sceneCatalog {
		for _, theme := range []string{"light", "dark"} {
			t.Run(string(kind)+"/"+theme, func(t *testing.T) {
				out := chartfixtures.Produce(kind, "binding")
				s, err := renderChartSVG(out.Output, theme, 800, 420, "UTC")
				if err != nil || fmt.Sprintf("%x", sha256.Sum256([]byte(s))) != golden[string(kind)+"/"+theme] {
					t.Fatal("existing SVG bytes changed", err)
				}
			})
		}
	}
}
func TestDrawingSceneRejectsOpenOrOversizedPrimitives(t *testing.T) {
	for _, s := range []*drawingScene{{nodes: []drawingPrimitive{{kind: "image", text: "https://example.invalid/a"}}}, {nodes: []drawingPrimitive{{kind: "slice"}}}, {nodes: []drawingPrimitive{{kind: "text", text: strings.Repeat("x", (1<<20)+1)}}}} {
		if s.valid() {
			t.Fatal("unbounded or unknown scene accepted")
		}
	}
	s := &drawingScene{}
	for i := 0; i < 16385; i++ {
		s.add(drawingPrimitive{kind: "group_end"})
	}
	if s.valid() || len(s.nodes) > 16384 {
		t.Fatal("node budget applied after allocation")
	}
}
