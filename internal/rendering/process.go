package rendering

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime/debug"
	"strconv"
	"time"
)

// Process supervises a fixed local executable with a sealed JSON stdin/stdout
// protocol. It never invokes a shell, forwards environment variables, or accepts
// a caller-selected executable/argument/URL.
type Process struct {
	path    string
	options Options
	slots   chan struct{}
}

func NewProcess(path string, options Options) (*Process, error) {
	if path == "" || path[0] != '/' || !options.valid() {
		return nil, ErrInvalid
	}
	return &Process{path: path, options: options, slots: make(chan struct{}, options.MaxConcurrent)}, nil
}

type cappedBuffer struct {
	bytes.Buffer
	max      int
	exceeded bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		room := b.max - b.Len()
		if room > 0 {
			_, _ = b.Buffer.Write(p[:room])
		}
		b.exceeded = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func (p *Process) Process(ctx context.Context, work SealedWork) (Rendition, error) {
	if p == nil || ctx == nil || work.Version != WorkerProtocolVersion {
		return Rendition{}, ErrInvalid
	}
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
	cmd := exec.CommandContext(bounded, p.path, "--sealed-render-worker")
	cmd.Env = []string{"GOMEMLIMIT=" + strconv.FormatInt(p.options.MaxMemoryBytes, 10) + "B"}
	cmd.Env = append(cmd.Env, "GOMEMLIMIT_BYTES="+strconv.FormatInt(p.options.MaxMemoryBytes, 10))
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
	var rendition Rendition
	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	dec.DisallowUnknownFields()
	if dec.Decode(&rendition) != nil || dec.Decode(new(any)) != io.EOF || rendition.State != "succeeded" || rendition.Bytes != len(rendition.Content) {
		return Rendition{}, ErrWorker
	}
	return rendition, nil
}

// WorkerMain is the complete credential-free worker entrypoint. It consumes one
// request, emits one bounded response, then exits so crashes cannot corrupt the service.
func WorkerMain(stdin io.Reader, stdout io.Writer, maxInput, maxOutput int, memory int64) error {
	if stdin == nil || stdout == nil || maxInput < 1024 || maxOutput < 1024 || memory < 32<<20 {
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
	if dec.Decode(&work) != nil || dec.Decode(new(any)) != io.EOF || work.Version != WorkerProtocolVersion {
		return ErrInvalid
	}
	result, err := renderSealed(work.Request, work.View, maxOutput)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > maxOutput {
		return ErrOutputLimit
	}
	_, err = stdout.Write(encoded)
	return err
}

// LocalProcessor exercises the same sealed contract without a process and is
// useful only for deterministic unit tests; production construction uses Process.
type LocalProcessor struct{ MaxBytes int }

func (l LocalProcessor) Process(_ context.Context, w SealedWork) (Rendition, error) {
	return renderSealed(w.Request, w.View, l.MaxBytes)
}

var _ = time.Second
