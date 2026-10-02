package nlqexec

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
)

func TestLearningRequalificationOriginAndPortableVersion(t *testing.T) {
	old := ExampleRecord{ID: strings.Repeat("1", 32), Topic: "topic", Question: "Revenue", SQL: "SELECT sum(amount) FROM analytics.sales", Origin: ExampleOrigin{SchemaVersion: 1, TopicVersion: "v1"}}
	old.Digest = originExampleDigest(old.Topic, old.Question, old.SQL, nil, old.Origin)
	current := old
	current.Origin.TopicVersion = "v2"
	current.Origin.Requalification = &ExampleRequalification{Policy: ExampleRequalificationPolicy, ExampleID: old.ID, Version: 1, Digest: old.Digest, OriginDigest: exec.Hash(old.Origin), ContractDigest: exec.Hash("verified-current-contract")}
	current.Digest = originExampleDigest(current.Topic, current.Question, current.SQL, nil, current.Origin)
	if current.Digest == old.Digest || !ExampleParametersValid(current) || portableExampleOriginVersion(nil, current.Origin) != 5 {
		t.Fatal("qualification origin missing from identity")
	}
	altered := current
	altered.Origin.Requalification = nil
	if altered.Digest == originExampleDigest(altered.Topic, altered.Question, altered.SQL, nil, altered.Origin) {
		t.Fatal("downgraded qualification kept identity")
	}
	bad := *current.Origin.Requalification
	bad.ContractDigest = "missing"
	altered = current
	altered.Origin.Requalification = &bad
	if ExampleParametersValid(altered) || bad.Valid() {
		t.Fatal("invalid proof metadata admitted")
	}
	schema, err := exampleparams.New([]string{"number"})
	if err != nil {
		t.Fatal(err)
	}
	typed := current
	typed.ParameterSchema = schema
	typed.Digest = originExampleDigest(typed.Topic, typed.Question, typed.SQL, schema, typed.Origin)
	if !ExampleParametersValid(typed) {
		t.Fatal("typed qualified identity rejected")
	}
	typed.ParameterSchema = nil
	if ExampleParametersValid(typed) {
		t.Fatal("typed qualified schema stripped")
	}
	row := PortableExample{SchemaVersion: 1, Origin: current.Origin}
	if portableExampleValid(row) {
		t.Fatal("new provenance imported as legacy")
	}
	row.SchemaVersion = 5
	if !portableExampleValid(row) {
		t.Fatal("versioned provenance rejected")
	}
}
