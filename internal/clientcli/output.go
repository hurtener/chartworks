package clientcli

import (
	"encoding/json"
	"io"

	"github.com/hurtener/chartworks/internal/gateway"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

// writeOutput rejects a short write even when an injected writer incorrectly
// returns a nil error. An incomplete machine-readable result is never success.
func writeOutput(writer io.Writer, data []byte) error {
	n, err := writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

// A successful HTTP exchange is not necessarily a successful MCP operation.
// Protocol errors lose arbitrary message/data text; valid tool results retain
// their typed owner outcome/receipt for reconciliation, with a nonzero exit code.
func writeMCPBody(stdout, stderr io.Writer, body []byte) int {
	if len(body) == 0 {
		return 0 // A successful initialized notification has no response body.
	}
	if _, err := gateway.DecodeJSON(body, 32<<20); err != nil {
		return fail(stderr, cw.ErrInvalidCatalog)
	}
	var response struct {
		Version string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *struct {
			Code int `json:"code"`
		} `json:"error,omitempty"`
	}
	if json.Unmarshal(body, &response) != nil || response.Version != "2.0" || (response.Error == nil) == (response.Result == nil) {
		return fail(stderr, cw.ErrInvalidCatalog)
	}
	if response.Error != nil {
		if writeJSON(stdout, response) != 0 {
			return 1
		}
		return fail(stderr, cw.ErrInvalidCatalog)
	}
	var result struct {
		IsError    bool `json:"isError"`
		Structured struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"structuredContent"`
	}
	if json.Unmarshal(response.Result, &result) != nil {
		return fail(stderr, cw.ErrInvalidCatalog)
	}
	if writeBody(stdout, body) != 0 {
		return 1
	}
	if !result.IsError {
		return 0
	}
	code := 1
	switch result.Structured.Error.Code {
	case "forbidden", "unauthenticated", "unauthorized":
		code = 3
	case "expired", "conflict":
		code = 4
	case "invalid_request", "limit_exceeded":
		code = 2
	}
	if writeOutput(stderr, []byte("chartworks: MCP operation rejected; inspect its outcome and receipt before retrying\n")) != nil {
		return 1
	}
	return code
}
