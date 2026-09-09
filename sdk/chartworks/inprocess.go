package chartworks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
)

// NewInProcess runs the ordinary SDK through the application's authenticated
// HTTP handler without opening a listener. Supply the same composed handler
// used by the server, not a domain implementation or an unprotected test double.
// Every request still carries current Pengui authority and passes the handler's
// verifier, resource loaders, schemas and service checks. There is deliberately
// no envelope, tenant, user, role or source-credential constructor here.
//
// The handler must honor request cancellation, as the production handlers do.
// Work executes synchronously: cancellation never leaves a detached goroutine
// running a mutation after this client returns. Streaming transports are not
// emulated; Chartworks' bounded JSON and polling operations are supported.
func NewInProcess(handler http.Handler, token TokenProvider) (*Client, error) {
	if handler == nil {
		return nil, errors.New("chartworks: authenticated handler required")
	}
	return New("http://127.0.0.1", &http.Client{Transport: inProcessTransport{handler: handler}}, token)
}

type inProcessTransport struct{ handler http.Handler }

func (t inProcessTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "http" || request.URL.Host != "127.0.0.1" {
		return nil, errors.New("chartworks: invalid in-process request")
	}
	if request.Body != nil {
		defer func() { _ = request.Body.Close() }()
	}
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	// Give the handler a server-side request copy. Do not share mutable request
	// headers with callers, and do not create authority from context values.
	local := request.Clone(request.Context())
	local.RequestURI = local.URL.RequestURI()
	local.RemoteAddr = "127.0.0.1:0"
	writer := &boundedResponse{header: make(http.Header)}
	t.handler.ServeHTTP(writer, local)
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	if writer.err != nil {
		return nil, writer.err
	}
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	return &http.Response{
		StatusCode: writer.status,
		Header: writer.header.Clone(),
		Body: io.NopCloser(bytes.NewReader(writer.body.Bytes())),
		ContentLength: int64(writer.body.Len()),
		Request: request,
		Proto: "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}, nil
}

// The largest currently registered SDK response is bounded at 32 MiB. This
// limit applies while the handler writes, not after an unbounded buffer grows.
const inProcessResponseLimit = 32 << 20

type boundedResponse struct {
	header http.Header
	body bytes.Buffer
	status int
	err error
}

func (w *boundedResponse) Header() http.Header { return w.header }
func (w *boundedResponse) WriteHeader(status int) {
	if w.status != 0 || w.err != nil {
		return
	}
	if status < 100 || status > 599 || status == http.StatusSwitchingProtocols {
		w.err = errors.New("chartworks: invalid in-process response")
		return
	}
	if status < 200 {
		return
	}
	w.status = status
}
func (w *boundedResponse) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if len(data) > inProcessResponseLimit-w.body.Len() {
		w.err = errors.New("chartworks: in-process response exceeds limit")
		return 0, w.err
	}
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}

// Flush does not create a stream or a background task. It only commits headers
// for handlers that use the standard Flusher interface for a bounded response.
func (w *boundedResponse) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
}

// clientContext bounds token-supplier work as well as the HTTP exchange. The
// caller's earlier deadline always wins. No credential is retained between calls.
func (c *Client) clientContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if c == nil || c.http == nil || c.token == nil || ctx == nil {
		return nil, nil, errors.New("chartworks: invalid request")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, c.http.Timeout)
	return bounded, cancel, nil
}
