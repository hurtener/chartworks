package chartworks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBYOSDKFreshAuthorityAndReferences(t *testing.T) {
	var tokens, calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.Header.Get("Authorization") != fmt.Sprintf("Bearer token-%d", calls.Load()) {
			http.Error(w, "authority not refreshed", 400)
			return
		}
		if r.URL.Path == "/v1/nlq/sql" {
			var in SQLSubmission
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Parameters == nil || in.Operation != "one" {
				http.Error(w, "invalid submit", 400)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	c, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return fmt.Sprintf("token-%d", tokens.Add(1)), nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ref := QueryContextReference{SchemaVersion: 1, ID: strings.Repeat("a", 64), Context: "context"}
	if _, err = c.GetQueryContext(ctx, QueryContextRequest{SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.ReadQueryContext(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SubmitSQL(ctx, SQLSubmission{Reference: ref, Operation: "one", SQL: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	if tokens.Load() != 3 || calls.Load() != 3 {
		t.Fatal("token reused")
	}
	if _, err = c.ReadQueryContext(ctx, QueryContextReference{}); err == nil {
		t.Fatal("invalid reference")
	}
	if _, err = c.SubmitSQL(ctx, SQLSubmission{Reference: ref, Operation: "invalid/operation"}); err == nil {
		t.Fatal("invalid operation")
	}
	if calls.Load() != 3 {
		t.Fatal("invalid reference performed I/O")
	}
}
