package reporting

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// CheckFrozenEligibility reuses the definition lifecycle gate at the transaction
// that accepts or publishes retained work. Approval never replaces current reach.
func CheckFrozenEligibility(e identity.Envelope, m RunManifest, snapshot Snapshot, now time.Time) error {
	if err := RequireRunManifest(e, m); err != nil { return err }
	if !now.Before(m.Expires) { return ErrExpired }
	if snapshot.State.ID != m.Block || snapshot.Revision.Number != m.Revision.Number || snapshot.Revision.Digest != m.Revision.Digest ||
		snapshot.Revision.ID != m.Revision.ID || snapshot.Validation == nil || snapshot.Validation.BindingDigest != exec.Hash(m.Binding) ||
		DependencyDigest(snapshot.Validation.Dependencies, m.Revision.Definition.Topics) != DependencyDigest(m.Dependencies, m.Revision.Definition.Topics) {
		return ErrStale
	}
	return runEligibility(e, snapshot, m.Policy, now)
}

// CheckFrozenResult validates actual ordered, typed values against the frozen
// contract. Queue attempts are deliberately not physical query-attempt numbers.
func CheckFrozenResult(ctx context.Context, m RunManifest, result exec.Result, a exec.Attempt) error {
	if ctx == nil || !a.Manifest.Valid() || a.ID == "" || a.Number < 1 || a.Number > 3 || a.Finished == nil || !successful(a.Status) || a.RemoteState != "stopped" ||
		a.Manifest.Operation != m.ID || a.Manifest.Session != m.Session || a.Manifest.Preview != m.Private ||
		a.Manifest.Receipt.Source != m.Binding.Source || a.Manifest.Receipt.Context != m.Binding.Context || a.Manifest.Receipt.Contract != m.Binding.Contract ||
		a.Manifest.Receipt.Dialect != m.Binding.Dialect || a.Rows != len(result.Rows) || a.Bytes != result.Bytes || result.Outcome != a.Status ||
		len(result.Rows) > m.Limits.MaxRows || result.Bytes > m.Limits.MaxResultBytes || result.Bytes < 0 {
		return ErrInvalid
	}
	want := make([]string, 0, len(m.Dependencies))
	for _, dependency := range m.Dependencies {
		if dependency.Source != m.Binding.Source || dependency.Context != m.Binding.Context { return ErrInvalid }
		want = append(want, dependency.Dataset)
	}
	got := append([]string(nil), a.Manifest.Receipt.Dependencies...)
	slices.Sort(want); slices.Sort(got)
	if len(want) == 0 || !slices.Equal(want, got) { return ErrInvalid }
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > m.Limits.MaxResultBytes { return ErrBudget }
	limits := config.DefaultReporting()
	limits.PreviewRows, limits.PreviewBytes = m.Limits.MaxRows, m.Limits.MaxResultBytes
	definition := m.Revision.Definition
	definition.Outputs = m.Outputs
	return checkResult(ctx, definition, result, limits)
}

// ContentDigest excludes only the digest itself. It binds the full selected
// output, provenance, failure receipt and reserved usage, not renderer geometry alone.
func (o RetainedOutput) ContentDigest() string { o.Digest = ""; return digest(o) }

// CheckFrozenOutput validates the persisted tagged union, exact saved mapping,
// narrative reservation and content digest. It does not perform inference.
func CheckFrozenOutput(m RunManifest, o RetainedOutput, starting bool) error {
	var saved *Output
	for index := range m.Outputs { if m.Outputs[index].ID == o.ID { saved = &m.Outputs[index]; break } }
	if saved == nil || saved.Kind != o.Kind { return ErrInvalid }
	if starting {
		if saved.Narrative == nil || o.Kind != "narrative" || o.State != "indeterminate" || o.Code != "narrative_indeterminate" ||
			o.ReservedCalls != saved.Narrative.MaxCalls || o.ReservedTokens != saved.Narrative.MaxTokens || o.Chart != nil || o.Narrative != nil || o.Digest != "" { return ErrInvalid }
		return nil
	}
	if o.State != "succeeded" && o.State != "failed" || o.Digest != o.ContentDigest() { return ErrInvalid }
	if o.State == "failed" {
		if o.Chart != nil || !slices.Contains([]string{"narrative_unavailable", "narrative_failed", "narrative_indeterminate", "output_failed"}, o.Code) { return ErrInvalid }
		if o.Narrative != nil && (o.Narrative.Text != "" || len(o.Narrative.Claims) != 0 || len(o.Narrative.Evidence) != 0) { return ErrInvalid }
		return nil
	}
	if o.Code != "" { return ErrInvalid }
	if o.Kind != "narrative" {
		if saved.Mapping == nil || o.Chart == nil || o.Narrative != nil || o.ReservedCalls != 0 || o.ReservedTokens != 0 ||
			digest(o.Chart.Mapping) != digest(*saved.Mapping) { return ErrInvalid }
		return nil
	}
	if saved.Narrative == nil || o.Chart != nil || o.Narrative == nil || o.ReservedCalls != saved.Narrative.MaxCalls || o.ReservedTokens != saved.Narrative.MaxTokens { return ErrInvalid }
	n, spec := o.Narrative, saved.Narrative
	if n.PromptVersion != spec.PromptVersion || n.ModelVersion != spec.ModelVersion || n.SchemaVersion != spec.SchemaVersion || n.Locale != spec.Locale || n.Tone != spec.Tone ||
		n.EvidenceHash != digest(n.Evidence) || n.OutputHash != digest([]any{n.Text, n.Claims}) { return ErrInvalid }
	text, err := groundedText(NarrativeAnswer{Claims: n.Claims}, n.Evidence, *spec)
	if err != nil || text != n.Text { return ErrInvalid }
	return nil
}

// FrozenCompletion derives the terminal outcome from the complete ordered
// selected-output set. A caller cannot declare success over missing/failed work.
func FrozenCompletion(m RunManifest, outputs []RetainedOutput) (string, string, error) {
	if len(outputs) != len(m.Outputs) || len(outputs) == 0 { return "", "", ErrIncomplete }
	for index, output := range outputs {
		if output.ID != m.Outputs[index].ID { return "", "", ErrInvalid }
		if err := CheckFrozenOutput(m, output, false); err != nil { return "", "", err }
	}
	state, code := frozenOutcome(outputs, m.PartialPolicy)
	return state, code, nil
}
