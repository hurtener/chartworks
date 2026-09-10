package reporting

import (
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
)

// PreparedRun is issued only by the frozen service after resolving an admitted
// request. Its detached encoding cannot be altered through caller slice aliases.
type PreparedRun struct {
	encoded   []byte
	authority string
	deadline  time.Time
}

func prepareRun(e identity.Envelope, m RunManifest) (PreparedRun, error) {
	if err := RequireRunManifest(e, m); err != nil {
		return PreparedRun{}, err
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) > 4<<20 {
		return PreparedRun{}, ErrInvalid
	}
	p := PreparedRun{encoded: b, authority: authority(e), deadline: e.Deadline()}
	_, err = p.Checked(e)
	return p, err
}

// Checked revalidates the original immutable admission at the store boundary.
func (p PreparedRun) Checked(e identity.Envelope) (RunManifest, error) {
	var m RunManifest
	if len(p.encoded) == 0 || len(p.encoded) > 4<<20 || !e.Valid() || p.authority != authority(e) || !time.Now().Before(p.deadline) {
		return m, access.ErrUnauthenticated
	}
	if json.Unmarshal(p.encoded, &m) != nil || !identity.Identifier(m.ID) || !hashValid(m.RequestHash) || !hashValid(m.TaskHash) ||
		!hashValid(m.ReuseKey) || m.Revision.Number < 1 || m.Revision.Digest != DefinitionDigest(m.Revision.Definition) ||
		m.Revision.ExecutionDigest != ExecutionDigest(m.Revision.Definition) || m.Limits.Validate() != nil ||
		!m.Binding.Valid() || m.Binding.Tenant != m.Tenant || m.Binding.Source != m.Revision.Definition.Source || m.Binding.Context != m.Revision.Definition.Context ||
		m.Created.IsZero() || !m.Expires.After(m.Created) || len(m.Outputs) == 0 || len(m.Outputs) > 32 ||
		m.PartialPolicy != "fail" && m.PartialPolicy != "allow_partial" || !locale(m.Locale) {
		return RunManifest{}, ErrInvalid
	}
	if err := RequireRunManifest(e, m); err != nil {
		return RunManifest{}, err
	}
	return m, nil
}

// RunWrite is a closed internal checkpoint union. Only a service-issued proof
// and an owned live invocation can persist values or publish completion.
type RunWrite struct {
	Kind           string          `json:"kind"`
	Manifest       RunManifest     `json:"manifest"`
	Result         *exec.Result    `json:"result,omitempty"`
	Attempt        *exec.Attempt   `json:"attempt,omitempty"`
	Output         *RetainedOutput `json:"output,omitempty"`
	Outcome        string          `json:"outcome,omitempty"`
	Code           string          `json:"code,omitempty"`
}

// PreparedRunWrite prevents transports and unrelated jobs from manufacturing
// publication evidence. The invocation is checked again inside the transaction.
type PreparedRunWrite struct {
	encoded   []byte
	authority string
	deadline  time.Time
}

func prepareRunWrite(e identity.Envelope, w RunWrite) (PreparedRunWrite, error) {
	if err := RequireRunManifest(e, w.Manifest); err != nil {
		return PreparedRunWrite{}, err
	}
	b, err := json.Marshal(w)
	if err != nil || len(b) > w.Manifest.Limits.MaxArtifactBytes+4<<20 {
		return PreparedRunWrite{}, ErrBudget
	}
	return PreparedRunWrite{encoded: b, authority: authority(e), deadline: e.Deadline()}, nil
}

// Checked detaches and verifies checkpoint identity, size and tagged-union shape.
func (p PreparedRunWrite) Checked(inv jobs.Invocation) (RunWrite, error) {
	var w RunWrite
	if !inv.Valid() || len(p.encoded) == 0 || len(p.encoded) > 68<<20 || !time.Now().Before(p.deadline) || json.Unmarshal(p.encoded, &w) != nil {
		return w, ErrInvalid
	}
	e, err := inv.Current("reporting.run", w.Manifest.Block, w.Manifest.RequestHash)
	if err != nil {
		return RunWrite{}, err
	}
	if authority(e) != p.authority || inv.Lease().Task.ID != w.Manifest.ID || inv.Lease().Task.ManifestHash != w.Manifest.TaskHash {
		return RunWrite{}, access.ErrNotFound
	}
	if err = RequireRunManifest(e, w.Manifest); err != nil {
		return RunWrite{}, err
	}
	switch w.Kind {
	case "result":
		if w.Result == nil || w.Attempt == nil || w.Output != nil || w.Outcome != "" || w.Attempt.Manifest.Operation != w.Manifest.ID ||
			w.Attempt.Manifest.Number != inv.Lease().Attempt || w.Attempt.Finished == nil || w.Attempt.RemoteState != "stopped" || !successful(w.Attempt.Status) ||
			exec.ValidateResult(*w.Result, w.Manifest.Limits.MaxRows, w.Manifest.Limits.MaxResultBytes) != nil {
			return RunWrite{}, ErrInvalid
		}
	case "output_start", "output":
		if w.Result != nil || w.Attempt != nil || w.Output == nil || w.Outcome != "" || !identity.Identifier(w.Output.ID) {
			return RunWrite{}, ErrInvalid
		}
		found := false
		for _, output := range w.Manifest.Outputs {
			found = found || output.ID == w.Output.ID && output.Kind == w.Output.Kind
		}
		if !found || w.Kind == "output_start" && (w.Output.Kind != "narrative" || w.Output.State != "indeterminate" || w.Output.ReservedCalls < 1 || w.Output.ReservedTokens < 1) {
			return RunWrite{}, ErrInvalid
		}
		if w.Kind == "output" && w.Output.State != "succeeded" && w.Output.State != "failed" {
			return RunWrite{}, ErrInvalid
		}
	case "complete":
		if w.Result != nil || w.Attempt != nil || w.Output != nil || w.Outcome != "succeeded" && w.Outcome != "partial" && w.Outcome != "failed" {
			return RunWrite{}, ErrInvalid
		}
	default:
		return RunWrite{}, ErrInvalid
	}
	return w, nil
}
