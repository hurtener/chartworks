package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

func chartReadFixture(t *testing.T) ReadResult {
	t.Helper()
	var out ReadResult
	if err := json.Unmarshal([]byte(`{"schema":[{"name":"i","type":"integer","encoding":"string"},{"name":"d","type":"decimal","encoding":"string"},{"name":"n","type":"number","encoding":"number"},{"name":"b","type":"boolean","encoding":"boolean"},{"name":"t","type":"text","encoding":"string"},{"name":"at","type":"temporal","encoding":"string"},{"name":"blob","type":"binary","encoding":"string"},{"name":"json","type":"structured","encoding":"string"}],"rows":[["9007199254740993","3.125",9007199254740993.125,true,"<literal>","2026-09-08","cafe","{\"a\":1}"]],"outcome":"succeeded"}`), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestChartReadAdapterUsesExactTypedWire(t *testing.T) {
	in := chartReadFixture(t)
	out, err := ChartDataFromReadResult(context.Background(), in, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if out.Rows[0][0].Value != "9007199254740993" || out.Rows[0][2].Value != "9007199254740993.125" || out.Rows[0][3].Value != "true" || out.Columns[4].Role != "dimension" || out.Columns[5].Role != "time" {
		t.Fatalf("adapter lost qualified values: %+v", out)
	}
	for _, c := range out.Columns {
		if c.Aggregation != "" || c.Provenance.Source != "" || c.Format.Currency != "" {
			t.Fatal("adapter invented reviewed metadata")
		}
	}
	in.Rows[0][0] = json.RawMessage(`null`)
	out, err = ChartDataFromReadResult(context.Background(), in, charts.Defaults())
	if err != nil || !out.Rows[0][0].Null {
		t.Fatal("explicit null", err)
	}
	in.Rows = nil
	in.Outcome = "empty"
	if _, err = ChartDataFromReadResult(context.Background(), in, charts.Defaults()); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*ReadResult){
		func(v *ReadResult) { v.Outcome = "unknown" }, func(v *ReadResult) { v.Outcome = "empty" }, func(v *ReadResult) { v.Rows = nil }, func(v *ReadResult) { v.Schema = nil }, func(v *ReadResult) { v.Schema[0].Type = "unknown" }, func(v *ReadResult) { v.Schema[0].Encoding = "number" }, func(v *ReadResult) { v.Rows[0] = v.Rows[0][:1] }, func(v *ReadResult) { v.Rows[0][0] = json.RawMessage(`1`) }, func(v *ReadResult) { v.Rows[0][2] = json.RawMessage(`"3.125"`) }, func(v *ReadResult) { v.Rows[0][2] = json.RawMessage(`true`) }, func(v *ReadResult) { v.Rows[0][2] = json.RawMessage(`1 2`) }, func(v *ReadResult) { v.Rows[0][3] = json.RawMessage(`"true"`) }, func(v *ReadResult) { v.Truncation = "rows" }, func(v *ReadResult) { v.Outcome = "truncated"; v.Truncation = "full_source" },
	} {
		bad := chartReadFixture(t)
		edit(&bad)
		if _, err := ChartDataFromReadResult(context.Background(), bad, charts.Defaults()); err == nil {
			t.Fatalf("bad typed result accepted: %+v", bad)
		}
	}
	for _, edit := range []func(*ChartLimits){func(l *ChartLimits) { l.MaxRows = 0 }, func(l *ChartLimits) { l.MaxColumns = 1 }, func(l *ChartLimits) { l.MaxCellBytes = 16 }, func(l *ChartLimits) { l.MaxBytes = 1024; l.MaxCellBytes = 1024 }} {
		l := charts.Defaults()
		edit(&l)
		if _, err := ChartDataFromReadResult(context.Background(), chartReadFixture(t), l); err == nil {
			t.Fatal("adapter limit bypass")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ChartDataFromReadResult(ctx, chartReadFixture(t), charts.Defaults()); !errors.Is(err, context.Canceled) {
		t.Fatal("adapter cancellation ignored", err)
	}
	//nolint:staticcheck // Intentional nil-context rejection contract.
	if _, err := ChartDataFromReadResult(nil, chartReadFixture(t), charts.Defaults()); err == nil {
		t.Fatal("nil context accepted")
	}
	if DefaultChartOptions() != charts.DefaultOptions() {
		t.Fatal("public defaults diverged")
	}
}
func TestChartSelectionFailureReceiptIsBoundedAndNoRawError(t *testing.T) {
	for _, body := range []string{`{"error":"SECRET","receipt":{"calls":[{"role":"visual_rank","provider":"fixture","requested_model":"m","attempts":1,"duration_ms":1,"cached":false}]}}`, `{"error":"SECRET"}`, `bad-json`, strings.Repeat("x", (64<<10)+1), `{"receipt":{"calls":[{"attempts":-1}]}}`, `{"receipt":{"calls":[],"warning":"` + strings.Repeat("w", 257) + `"}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(504); _, _ = io.WriteString(w, body) }))
		client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic", nil })
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.SelectChart(context.Background(), ChartSelectRequest{})
		var status *StatusError
		if !errors.As(err, &status) || status.Status != 504 || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("safe status lost", err)
		}
		expected := strings.Contains(body, `"visual_rank"`)
		if (status.Receipt != nil) != expected {
			t.Fatalf("unbounded/untyped receipt %s", body[:min(100, len(body))])
		}
		server.Close()
	}
	if readFailureReceipt(failedChartReader{}) != nil {
		t.Fatal("failed read accepted")
	}
}

type failedChartReader struct{}

func (failedChartReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
