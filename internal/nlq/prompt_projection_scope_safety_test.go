package nlq

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSQLRecoveryScopedProjectionFilterOnlyAndBounds(t *testing.T) {
	a, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"clarification", "interpreted_value", "interpreted_time", "required_rule", "future_reason"} {
		t.Run(reason, func(t *testing.T) {
			in := scopedProjectionFixture()
			in.Constraints.Required[0].Text = strings.ReplaceAll(in.Constraints.Required[0].Text, "catalog_term", reason)
			got, err := a.Assemble(context.Background(), in, TierHigh)
			if err != nil || strings.Contains(got.Prompt, "physical_projection:") || !strings.Contains(got.Prompt, "unused") || !strings.Contains(got.Prompt, "analytics.other") || !reflect.DeepEqual(got.Relations, in.Relations) {
				t.Fatal("filter-only or unknown selection invented a complete output closure", err)
			}
		})
	}
	for _, malformed := range []string{"not json", "null", "[]", "{}"} {
		in := scopedProjectionFixture()
		in.Constraints.Required[0].Text = malformed
		if _, err := a.Assemble(context.Background(), in, TierHigh); !errors.Is(err, ErrInvalid) {
			t.Fatal("malformed scoped selection accepted", err)
		}
	}
	in := scopedMultiProjectionFixture()
	in.Metrics[0].Dependencies = make([]MetricDependency, 4096)
	for i := range in.Metrics[0].Dependencies {
		in.Metrics[0].Dependencies[i] = scopedProjectionColumn("south_sales", "amount")
	}
	// Direct owner call exercises the aggregate bound, independently from the
	// assembler's stricter per-metric limits. The dimension adds the 4,097th.
	if _, _, err := scopedPromptRelations(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("combined dependency budget was not enforced", err)
	}
	roots := make([]map[string]any, 129)
	for i := range roots {
		roots[i] = map[string]any{"reference": map[string]string{"kind": "dimension"}, "reason": "catalog_term"}
	}
	raw, _ := json.Marshal(roots)
	if _, err := independentlySelectedProjection(string(raw)); !errors.Is(err, ErrInvalid) {
		t.Fatal("selection count bound was not enforced", err)
	}
}
