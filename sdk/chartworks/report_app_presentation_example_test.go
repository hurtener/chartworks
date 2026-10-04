package chartworks_test

import (
	"encoding/json"
	"fmt"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

func ExampleReportAppPresentationPatch() {
	// Use a column ID and fields advertised by ReadManualChart's selected output.
	// Explicit zero is a set; reset restores the reviewed display-label default.
	zero := 0
	patch := chartworks.ReportAppPresentationPatch{
		Version: chartworks.ReportAppPresentationVersion,
		Edits: []chartworks.ReportAppColumnPresentationEdit{{
			Column: "amount",
			Set:    &chartworks.ReportAppColumnPresentationSet{FractionDigits: &zero},
			Reset:  []chartworks.ReportAppPresentationField{chartworks.ReportAppPresentationDisplayLabel},
		}},
	}
	// Set Presentation on ReportAppBlockPresentationRequest and pass the exact
	// block/revision/digest/output/version to AmendManualChartPresentation.
	wire, _ := json.Marshal(patch)
	fmt.Println(string(wire))
	// Output: {"version":1,"edits":[{"column":"amount","set":{"fraction_digits":0},"reset":["display_label"]}]}
}
