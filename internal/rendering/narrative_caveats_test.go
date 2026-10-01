package rendering

import (
	"context"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
)

func TestNarrativeHTMLPreservesInertCaveats(t *testing.T) {
	for _, policy := range []string{"", reporting.NarrativePolicyVersion, "bounded-narrative-v3"} {
		t.Run(policy, func(t *testing.T) {
			out := &reporting.ViewerOutput{ID: "narrative", Kind: "narrative", State: "succeeded", RetainedDigest: strings.Repeat("a", 64), Narrative: &reporting.NarrativeResult{PolicyVersion: policy, Text: "Known retained observations", Caveats: []string{"retained_observation_not_live_source", "source_result_truncated", "<script>private label</script>"}}}
			f := &fixtureViewer{value: reporting.DeliveryViewResult{Output: out}}
			service, err := newTestService(f, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			got, err := service.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "narrative"}, Format: "html", Theme: "light", Width: 800, Height: 420})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{`data-narrative-caveats="true"`, "retained_observation_not_live_source", "source_result_truncated", "&lt;script&gt;private label&lt;/script&gt;"} {
				if !strings.Contains(got.Content, want) {
					t.Fatal("caveat omitted", want)
				}
			}
			if strings.Contains(got.Content, "<script>") {
				t.Fatal("active caveat markup")
			}
		})
	}
}
