package rendering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/reporting"
)

// Process supervises a fixed executable through a sealed stdin/stdout protocol.
type Process struct {
	path    string
	options Options
	slots   chan struct{}
}

func NewProcess(path string, options Options) (*Process, error) {
	if path == "" || path[0] != '/' || !options.valid() {
		return nil, ErrInvalid
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalid
	}
	if err := sandboxSupported(options.Isolation); err != nil {
		return nil, err
	}
	return &Process{path: path, options: options, slots: make(chan struct{}, options.MaxConcurrent)}, nil
}

type workerResponse struct {
	Version       string    `json:"version"`
	RequestDigest string    `json:"request_digest"`
	Rendition     Rendition `json:"rendition"`
}
type cappedBuffer struct {
	bytes.Buffer
	max      int
	exceeded bool
}

func (b *cappedBuffer) Write(v []byte) (int, error) {
	if b.Len()+len(v) > b.max {
		room := b.max - b.Len()
		if room > 0 {
			_, _ = b.Buffer.Write(v[:room])
		}
		b.exceeded = true
		return len(v), nil
	}
	return b.Buffer.Write(v)
}

func sealedDigest(work SealedWork) string {
	wire, _ := json.Marshal(struct {
		Version     string                       `json:"version"`
		Request     Request                      `json:"request"`
		View        reporting.DeliveryViewResult `json:"view"`
		Composition *SealedComposition           `json:"composition,omitempty"`
	}{work.Version, work.Request, work.View, work.Composition})
	s := sha256.Sum256(wire)
	return hex.EncodeToString(s[:])
}

// Process executes one sealed render in a fresh sandboxed worker process.
func (p *Process) Process(ctx context.Context, work SealedWork) (Rendition, error) {
	if p == nil || ctx == nil || work.Version != WorkerProtocolVersion || !validWorkerRequest(work) {
		return Rendition{}, ErrInvalid
	}
	wantDigest := sealedDigest(work)
	if work.Digest != "" && work.Digest != wantDigest {
		return Rendition{}, ErrInvalid
	}
	work.Digest = wantDigest
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	default:
		return Rendition{}, ErrBusy
	}
	wire, err := json.Marshal(work)
	if err != nil || len(wire) > p.options.MaxInputBytes {
		return Rendition{}, ErrInvalid
	}
	bounded, cancel := context.WithTimeout(ctx, p.options.MaxTime)
	defer cancel()
	cmd, cleanup, err := sandboxCommand(bounded, p.path, p.options.Isolation)
	if err != nil {
		return Rendition{}, err
	}
	defer cleanup()
	cmd.Env = []string{"GOMEMLIMIT=" + strconv.FormatInt(p.options.MaxMemoryBytes, 10) + "B", "GOMEMLIMIT_BYTES=" + strconv.FormatInt(p.options.MaxMemoryBytes, 10), "RENDER_MAX_INPUT_BYTES=" + strconv.Itoa(p.options.MaxInputBytes), "RENDER_MAX_OUTPUT_BYTES=" + strconv.Itoa(p.options.MaxOutputBytes)}
	cmd.Stdin = bytes.NewReader(wire)
	out := &cappedBuffer{max: p.options.MaxOutputBytes}
	diagnostics := &cappedBuffer{max: 4096}
	cmd.Stdout, cmd.Stderr = out, diagnostics
	err = cmd.Run()
	if bounded.Err() != nil {
		if errors.Is(bounded.Err(), context.DeadlineExceeded) {
			return Rendition{}, ErrTimeout
		}
		return Rendition{}, bounded.Err()
	}
	if out.exceeded || out.Len() >= out.max {
		return Rendition{}, ErrOutputLimit
	}
	if err != nil {
		return Rendition{}, fmt.Errorf("%w: process", ErrWorker)
	}
	var response workerResponse
	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	dec.DisallowUnknownFields()
	if dec.Decode(&response) != nil || dec.Decode(new(any)) != io.EOF || response.Version != WorkerProtocolVersion || response.RequestDigest != wantDigest {
		return Rendition{}, ErrWorker
	}
	if err = validateWorkerRendition(work, response.Rendition); err != nil {
		return Rendition{}, err
	}
	return response.Rendition, nil
}

func validWorkerRequest(work SealedWork) bool {
	r := work.Request
	if !oneOf(r.Format, "html", "svg") || !oneOf(r.Theme, "light", "dark") || r.Width < 320 || r.Width > 4096 {
		return false
	}
	if work.Composition != nil {
		return r.Full && r.Height >= 1 && r.Height <= 1_000_000 && work.Composition.Content != "" && work.Composition.SourceDigest != ""
	}
	return !r.Full && r.Height >= 200 && r.Height <= 4096 && work.View.Output != nil && work.View.Output.State == "succeeded" && work.View.Output.RetainedDigest != ""
}

func validateWorkerRendition(work SealedWork, r Rendition) error {
	media := map[string]string{"html": "text/html; charset=utf-8", "svg": "image/svg+xml"}[work.Request.Format]
	tz := work.View.Timezone
	if tz == "" {
		tz = "UTC"
	}
	projection, expectedSource := Projection{}, ""
	if work.Composition != nil {
		expectedSource = work.Composition.SourceDigest
	} else {
		var err error
		projection, err = projectionFor(work.View, tz)
		if err != nil || work.View.Output == nil {
			return ErrWorker
		}
		expectedSource = work.View.Output.RetainedDigest
	}
	d := sha256.Sum256([]byte(r.Content))
	if r.State != "succeeded" || r.Version != Version || r.Format != work.Request.Format || r.MediaType != media || r.Theme != work.Request.Theme || r.Width != work.Request.Width || r.Height != work.Request.Height || r.SourceDigest != expectedSource || !reflect.DeepEqual(r.Projection, projection) || r.Bytes != len(r.Content) || r.Digest != hex.EncodeToString(d[:]) || !safeStatic(r.Format, r.Content) {
		return ErrWorker
	}
	return nil
}
func safeStatic(format, content string) bool {
	lower := strings.ToLower(content)
	for _, bad := range []string{"<script", "<foreignobject", "<iframe", "<object", "<embed", "<link", "javascript:", "data:text/html", "href=\"http", "href='http", "src=\"http", "src='http", "url(http", "@import"} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	for rest := lower; ; {
		i := strings.Index(rest, " on")
		if i < 0 {
			break
		}
		rest = rest[i+3:]
		j := 0
		for j < len(rest) && rest[j] >= 'a' && rest[j] <= 'z' {
			j++
		}
		if j > 0 && j < len(rest) && rest[j] == '=' {
			return false
		}
	}
	if format == "html" {
		return strings.HasPrefix(lower, "<!doctype html>") && strings.Contains(lower, "<html") && strings.Contains(lower, "</html>")
	}
	if format == "svg" {
		if !strings.HasPrefix(lower, "<svg ") || !strings.HasSuffix(lower, "</svg>") {
			return false
		}
		d := xml.NewDecoder(strings.NewReader(content))
		for {
			_, err := d.Token()
			if err == io.EOF {
				return true
			}
			if err != nil {
				return false
			}
		}
	}
	return false
}

func WorkerMain(stdin io.Reader, stdout io.Writer, maxInput, maxOutput int, memory int64) error {
	if stdin == nil || stdout == nil || maxInput < 1024 || maxInput > 64<<20 || maxOutput < 1024 || maxOutput > 64<<20 || memory < 32<<20 {
		return ErrInvalid
	}
	debug.SetMemoryLimit(memory)
	if err := applyMemoryLimit(memory); err != nil {
		return ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, int64(maxInput)+1))
	if err != nil || len(raw) > maxInput {
		return ErrInvalid
	}
	var work SealedWork
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&work) != nil || dec.Decode(new(any)) != io.EOF || work.Version != WorkerProtocolVersion || work.Digest == "" || work.Digest != sealedDigest(work) || !validWorkerRequest(work) {
		return ErrInvalid
	}
	var result Rendition
	if work.Composition != nil {
		if len(work.Composition.Content) > maxOutput || work.Composition.SourceDigest == "" || !safeStatic(work.Request.Format, work.Composition.Content) {
			return ErrInvalid
		}
		digest := sha256.Sum256([]byte(work.Composition.Content))
		result = Rendition{State: "succeeded", Version: Version, Format: work.Request.Format, MediaType: map[string]string{"html": "text/html; charset=utf-8", "svg": "image/svg+xml"}[work.Request.Format], Theme: work.Request.Theme, Width: work.Request.Width, Height: work.Request.Height, SourceDigest: work.Composition.SourceDigest, Digest: hex.EncodeToString(digest[:]), Bytes: len(work.Composition.Content), Content: work.Composition.Content}
	} else {
		result, err = renderSealed(work.Request, work.View, maxOutput)
	}
	if err != nil {
		return err
	}
	response := workerResponse{Version: WorkerProtocolVersion, RequestDigest: work.Digest, Rendition: result}
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > maxOutput {
		return ErrOutputLimit
	}
	_, err = stdout.Write(encoded)
	return err
}

type LocalProcessor struct{ MaxBytes int }

// Process exists for deterministic tests. Production foundation construction
// accepts only the configured sandboxed Process.
func (l LocalProcessor) Process(_ context.Context, w SealedWork) (Rendition, error) {
	if w.Composition != nil {
		if !safeStatic(w.Request.Format, w.Composition.Content) {
			return Rendition{}, ErrInvalid
		}
		digest := sha256.Sum256([]byte(w.Composition.Content))
		return Rendition{State: "succeeded", Version: Version, Format: w.Request.Format, MediaType: map[string]string{"html": "text/html; charset=utf-8", "svg": "image/svg+xml"}[w.Request.Format], Theme: w.Request.Theme, Width: w.Request.Width, Height: w.Request.Height, SourceDigest: w.Composition.SourceDigest, Digest: hex.EncodeToString(digest[:]), Bytes: len(w.Composition.Content), Content: w.Composition.Content}, nil
	}
	return renderSealed(w.Request, w.View, l.MaxBytes)
}
