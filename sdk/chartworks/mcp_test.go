package chartworks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMCPWireAndFreshCallerAuthority(t *testing.T) {
	var provided, received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := received.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/mcp" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != fmt.Sprintf("Bearer synthetic-%d", n) || r.Header.Get("Accept") != "application/json, text/event-stream" || r.Header.Get("Mcp-Protocol-Version") != "2025-11-25" || r.Header.Get("Mcp-Session-Id") != "" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("MCP transport contract or cached token", r.URL.Path)
		}
		if n == 2 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"structuredContent":{"error":{"code":"forbidden","outcome":"not_started"}}}}`)
	}))
	defer server.Close()
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return fmt.Sprintf("synthetic-%d", provided.Add(1)), nil })
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.MCP(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"chart_catalog","arguments":{}}}`))
	if err != nil || !strings.Contains(string(raw), `"isError":true`) {
		t.Fatal("tool errors must remain typed results", string(raw), err)
	}
	raw, err = client.MCP(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil || len(raw) != 0 || provided.Load() != 2 || received.Load() != 2 {
		t.Fatal("notification/fresh authority", string(raw), err)
	}
	for _, body := range []string{"", `[]`, `{}`, `{"method":3}`, `{"method":""}`, `{"method":"ping"`, `{"method":"ping","payload":"` + strings.Repeat("x", 10<<20) + `"}`} {
		if _, err = client.MCP(context.Background(), json.RawMessage(body)); err == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
	if provided.Load() != 2 || received.Load() != 2 {
		t.Fatal("invalid input contacted provider or server")
	}
}
