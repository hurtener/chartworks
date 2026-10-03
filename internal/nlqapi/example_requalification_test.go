package nlqapi

import (
	"net/http"
	"testing"
)

func TestLearningRequalificationPublicSchema(t *testing.T) {
	r, err := ExecutionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	d, _, ok := r.Match(http.MethodPost, "/v1/nlq/examples/requalify")
	if !ok {
		t.Fatal("missing requalification operation")
	}
	if err = d.Request.Validate([]byte(`{"example_id":"retained","expected_version":2,"anchor":{"topic":"topic","context":"context","locale":"en","question":"Revenue"}}`), MaxBodyBytes); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"example_id":"retained","expected_version":2,"anchor":{},"activate":true}`, `{"example_id":"retained","expected_version":2,"anchor":{"grant":"all"}}`, `{"example_id":"retained","expected_version":2,"anchor":{},"sql":"SELECT 1"}`} {
		if d.Request.Validate([]byte(input), MaxBodyBytes) == nil {
			t.Fatal("open qualification schema")
		}
	}
}
