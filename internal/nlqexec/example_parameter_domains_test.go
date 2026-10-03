package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
)

func TestSQLRecoveryLearningDomainIdentityAndNoRetainedAuthority(t *testing.T) {
	legacy, _ := exampleparams.New([]string{"text"})
	schema, err := legacy.WithDomains([]string{"date"})
	if err != nil {
		t.Fatal(err)
	}
	x := ExampleRecord{Topic: "topic", Question: "Records with an abstract date", SQL: `SELECT id FROM analytics.sales WHERE created_at>$1::date`, ParameterSchema: schema}
	x.Digest = parameterExampleDigest(x.Topic, x.Question, x.SQL, x.ParameterSchema)
	if !ExampleParametersValid(x) || portableExampleVersion(x.ParameterSchema) != 4 {
		t.Fatal("domain identity")
	}
	if x.Digest == exec.Hash([]any{"parameterized-example-v1", x.Topic, x.Question, x.SQL, x.ParameterSchema}) {
		t.Fatal("v2 reused legacy digest domain")
	}
	if text := learnedExampleText(x); !strings.Contains(text, `"domain":"date"`) || strings.Contains(text, "2000-01-02") {
		t.Fatal("domain demonstration disclosed probe")
	}
	for _, policy := range []string{"", OwnedExamplePolicy} {
		row := PortableExample{SchemaVersion: 4, ParameterSchema: schema, Origin: ExampleOrigin{BindingPolicy: policy}}
		if !portableExampleValid(row) {
			t.Fatal("portable domain/policy", policy)
		}
		row.SchemaVersion = 3
		if portableExampleValid(row) {
			t.Fatal("domain portable downgrade")
		}
	}
	e := unitEnvelope(t)
	binding, _ := retainedSourceReader{}.Binding(context.Background(), e, "source", "context")
	if err := verifyExampleParameterDomains(context.Background(), e, schema, exec.Plan{}, binding); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("schema borrowed unsealed/retained plan", err)
	}
	if err := verifyExampleParameterDomains(context.Background(), e, legacy, exec.Plan{}, binding); err != nil {
		t.Fatal("legacy reinterpreted", err)
	}
	q := QueryRecord{Question: "Records using 2026-01-02", SQL: x.SQL, Parameters: []exec.Parameter{{Kind: "text", Value: "2026-01-02"}}}
	question, withoutProof, err := learnedParameterSchemaFromPlan(context.Background(), e, q, exec.Plan{}, binding, "")
	if err != nil || withoutProof.Version != exampleparams.Version || withoutProof.Slots[0].Domain != "" || strings.Contains(question, "2026-01-02") {
		t.Fatal("historical value guessed a domain", err)
	}
	if got, _ := legacy.ProbeValues(); got[0] != "example" {
		t.Fatal("legacy probe silently upgraded")
	}
}
