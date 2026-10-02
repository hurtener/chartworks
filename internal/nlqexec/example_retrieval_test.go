package nlqexec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
)

func TestLearningGenerationRetrievalQueryBounds(t *testing.T) {
	a := grainAdmission("Revenue")
	for _, input := range []string{"Revenue / INGRESOS !", "收入 month", strings.Repeat("oversized", 20)} {
		q := generationExampleQuery(a, input)
		if q.Limit != maxLearningCandidates || q.SourceBindingDigest != exec.Hash(a.binding) || q.AllowOwned {
			t.Fatal("retrieval query invented authority")
		}
		if strings.Contains(fmt.Sprintf("%#v", q), "Revenue") {
			t.Fatal("query diagnostic exposed text")
		}
	}
	var words []string
	for i := 0; i < 129; i++ {
		words = append(words, fmt.Sprintf("term%d", i))
	}
	if q := generationExampleQuery(a, strings.Join(words, " ")); q.SearchText != "" {
		t.Fatal("unbounded expansion")
	}
	if q := generationExampleQuery(a, "Revenue revenue ingresos"); q.SearchText != `"ingresos" OR "revenue"` {
		t.Fatal("unstable normalized query", q.SearchText)
	}
}
