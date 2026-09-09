package chartworks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHeaderAloneCannotEnableReplay(t *testing.T) {
	r := unitOperationRegistry(t)
	data, err := r.OpenAPI("synthetic replay", "1")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		t.Fatal("fixture")
	}
	op := doc["paths"].(map[string]any)["/v1/erase"].(map[string]any)["post"].(map[string]any)
	delete(op, "x-chartworks-replay")
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ParseOperations(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == "eraseFixture" && row.Replay != "never" {
			t.Fatal("header-only mutation became replayable")
		}
	}
	op["x-chartworks-replay"] = "read"
	data, _ = json.Marshal(doc)
	if _, err = ParseOperations(data); err == nil {
		t.Fatal("mutation claimed read replay")
	}
}

type requestObserver func(*http.Request) (*http.Response, error)

func (o requestObserver) RoundTrip(r *http.Request) (*http.Response, error) { return o(r) }
func TestMutationCannotBeRewoundByHTTPTransport(t *testing.T) {
	transport := requestObserver(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.GetBody != nil || r.Header.Get("Idempotency-Key") != "stable" {
			t.Error("mutation escaped explicit retry loop")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"receipt"}`)), Request: r}, nil
	})
	c, err := New("https://backend.example", &http.Client{Transport: transport}, func(context.Context) (string, error) { return "synthetic", nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Sweep(t.Context(), "stable"); err != nil {
		t.Fatal(err)
	}
}

func TestTypedRoutesCannotEscapePinnedMount(t *testing.T) {
	calls := 0
	c, err := NewInProcess(http.NotFoundHandler(), func(context.Context) (string, error) { calls++; return "synthetic", nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{".", ".."} {
		if _, err := c.Source(t.Context(), id); err == nil {
			t.Fatal("typed path traversal accepted")
		}
	}
	for _, path := range []string{"//evil.example/path", "https://evil.example/path", "/v1/%2fother", "/v1/{id}", "/v1/./sources"} {
		var out string
		if err := c.call(t.Context(), "GET", path, "", nil, &out); err == nil {
			t.Fatal("noncanonical route accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid path obtained authority")
	}
}
