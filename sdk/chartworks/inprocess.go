package chartworks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// InProcessOptions selects the same configured mount and request timeout as a
// network client. It carries no tenant, identity, envelope or source credentials.
// Empty BasePath means the root mount; zero Timeout selects the bounded default.
type InProcessOptions struct {
	BasePath string
	Timeout  time.Duration
}

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
// emulated; Chartworks's bounded JSON and polling operations are supported.
func NewInProcess(handler http.Handler, token TokenProvider) (*Client, error) {
	return NewInProcessWithOptions(handler, token, InProcessOptions{})
}

// NewInProcessWithOptions supports a configured HTTP base path without creating
// a different router or bypassing the application's authentication middleware.
func NewInProcessWithOptions(handler http.Handler, token TokenProvider, options InProcessOptions) (*Client, error) {
	if handler == nil || !clientBasePath(options.BasePath) {
		return nil, errors.New("chartworks: authenticated handler and valid mount required")
	}
	return New("http://127.0.0.1"+options.BasePath, &http.Client{
		Transport: inProcessTransport{handler: handler},
		Timeout:   options.Timeout,
	}, token)
}

type inProcessTransport struct{ handler http.Handler }

// wireContext preserves cancellation/deadlines but discards ambient values, just
// as crossing an HTTP connection does. In particular, a verified envelope or a
// middleware marker from an earlier caller cannot be injected into this request.
// The token provider has already received the original per-call context.
type wireContext struct{ context.Context }

func (wireContext) Value(any) any { return nil }

func (t inProcessTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request != nil && request.Body != nil {
		defer func() { _ = request.Body.Close() }()
	}
	if request == nil || request.URL == nil || request.URL.Scheme != "http" || request.URL.Host != "127.0.0.1" || t.handler == nil {
		return nil, errors.New("chartworks: invalid in-process request")
	}
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	local := request.Clone(wireContext{request.Context()})
	local.RequestURI = local.URL.RequestURI()
	local.RemoteAddr = "127.0.0.1:0"
	writer := &boundedResponse{header: make(http.Header), head: request.Method == http.MethodHead}
	t.handler.ServeHTTP(writer, local)
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	if writer.err != nil {
		return nil, writer.err
	}
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return &http.Response{
		StatusCode:    writer.status,
		Header:        writer.committed,
		Body:          io.NopCloser(bytes.NewReader(writer.body.Bytes())),
		ContentLength: int64(writer.body.Len()),
		Request:       request,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
	}, nil
}

// The largest currently registered SDK response is bounded at 32 MiB. This
// limit applies while the handler writes, not after an unbounded buffer grows.
const inProcessResponseLimit = 32 << 20

type boundedResponse struct {
	header    http.Header
	committed http.Header
	body      bytes.Buffer
	status    int
	head      bool
	err       error
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
	w.committed = w.header.Clone()
}
func (w *boundedResponse) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.status == 0 {
		if w.header.Get("Content-Type") == "" && len(data) > 0 {
			w.header.Set("Content-Type", http.DetectContentType(data))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.status == http.StatusNoContent || w.status == http.StatusNotModified {
		return 0, http.ErrBodyNotAllowed
	}
	if w.head {
		return len(data), nil
	}
	if len(data) > inProcessResponseLimit-w.body.Len() {
		w.err = errors.New("chartworks: in-process response exceeds limit")
		return 0, w.err
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
