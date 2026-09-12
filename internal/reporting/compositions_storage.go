package reporting

import (
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// CompositionReference is a required signed action/resource pair, never a grant.
// Page binds artifact context checks to the page they protect before payload I/O.
type CompositionReference struct {
	Page       string
	Action     string
	Kind       string
	Permission string
	ID         string
}

// CompositionReferences builds immutable pre-query requirements from a checked
// manifest. Descriptive audience, creator and origin labels grant no access.
func CompositionReferences(m CompositionManifest) []CompositionReference {
	out := []CompositionReference{{Action: "reporting.execute", Kind: m.Kind, Permission: "execute", ID: m.Document}}
	if m.Private {
		out = append(out, CompositionReference{Action: "reporting.preview", Kind: m.Kind, Permission: "preview", ID: m.Document})
	}
	groups := map[string]CompositionGroup{}
	for _, group := range m.Groups {
		groups[group.ID] = group
	}
	for _, page := range m.Pages {
		out = append(out, CompositionReference{Page: page.ID, Action: "reporting.execute", Kind: "report", Permission: "execute", ID: page.Report})
		if page.Private {
			out = append(out, CompositionReference{Page: page.ID, Action: "reporting.preview", Kind: "report", Permission: "preview", ID: page.Report})
		}
		seen := map[string]bool{}
		for _, widget := range page.Widgets {
			g, exists := groups[widget.Group]
			if !exists || seen[g.ID] {
				continue
			}
			seen[g.ID] = true
			out = append(out, CompositionReference{Page: page.ID, Action: "reporting.read", Kind: "execution_context", Permission: "use", ID: g.Binding.Context})
			out = append(out, CompositionReference{Page: page.ID, Action: "sources.query", Kind: "source", Permission: "query", ID: g.Binding.Source},
				CompositionReference{Page: page.ID, Action: "sources.query", Kind: "execution_context", Permission: "use", ID: g.Binding.Context})
			if g.Kind == "block" {
				out = append(out, CompositionReference{Page: page.ID, Action: "reporting.execute", Kind: "block", Permission: "execute", ID: g.Block})
				if g.Private {
					out = append(out, CompositionReference{Page: page.ID, Action: "reporting.preview", Kind: "block", Permission: "preview", ID: g.Block})
				}
			} else {
				out = append(out, CompositionReference{Page: page.ID, Action: "query.execute", Kind: "execution_context", Permission: "use", ID: g.Binding.Context})
			}
			for _, ref := range g.References {
				out = append(out, CompositionReference{Page: page.ID, Action: "reporting.execute", Kind: ref.Kind, Permission: ref.Permission, ID: ref.ID})
				if ref.Kind == "dataset" && ref.Permission == "query" {
					out = append(out, CompositionReference{Page: page.ID, Action: "sources.query", Kind: ref.Kind, Permission: ref.Permission, ID: ref.ID})
				}
			}
		}
	}
	return out
}

// CheckCompositionBlock repeats the exact definition and current eligibility
// check inside metadata admission. It neither validates nor executes new SQL.
func CheckCompositionBlock(e identity.Envelope, g CompositionGroup, snapshot Snapshot) error {
	if g.Kind != "block" || snapshot.State.ID != g.Block || snapshot.Revision.Number != g.Revision || snapshot.Revision.Digest != g.Definition || snapshot.Revision.ExecutionDigest != g.Execution || snapshot.Validation == nil || snapshot.Validation.BindingDigest != exec.Hash(g.Binding) {
		return ErrStale
	}
	if err := runEligibility(e, snapshot, g.Policy, time.Now()); err != nil {
		return err
	}
	return requireCompositionGroup(e, g)
}

// RequireCompositionArtifact separates retained read from execution authority.
// Private previews remain actor/session-bound and need explicit run and preview
// reach even after the same authored revision is published later.
func RequireCompositionArtifact(e identity.Envelope, kind, id, run, actor, session string, private bool) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	exact := access.Require(e, "reporting.read", access.Resource{Tenant: e.Tenant(), Kind: "run", Permission: "read", ID: run})
	if private {
		if exact != nil || actor != e.User() || session != e.Session() {
			return access.ErrNotFound
		}
		return RequireDocument(e, kind, id, Preview)
	}
	if exact == nil {
		return nil
	}
	return RequireDocument(e, kind, id, Read)
}

// CompositionStaticPayload returns inert text or an explicit omitted-widget
// receipt. A dynamic widget without a checkpoint has no invented payload.
func CompositionStaticPayload(page string, w CompositionWidget) *CompositionPayload {
	out := &CompositionPayload{Page: page, Widget: w.Definition.ID, Outputs: []RetainedOutput{}}
	if w.Definition.Kind == "text" {
		out.State, out.Text = "completed", clone(w.Definition.Text)
		return out
	}
	if w.Code != "" {
		out.State, out.Code = "failed", w.Code
		return out
	}
	return nil
}

// An output-local failure affects only widgets that selected that output.
// Query truncation, stale dependencies and failed execution remain group-wide.
func compositionSelectionState(result GroupResult, selected []string) (string, string) {
	if result.Kind != "block" || result.Code != "output_failed" {
		return result.State, result.Code
	}
	byID := map[string]RetainedOutput{}
	for _, output := range result.Outputs {
		byID[output.ID] = output
	}
	for _, id := range selected {
		output, exists := byID[id]
		if !exists {
			return "partial", "output_incomplete"
		}
		if output.State != "succeeded" {
			return "partial", "output_failed"
		}
	}
	return "completed", ""
}

// CompositionPayloadFromResult enforces the widget's saved output subset even
// when its group retained the union requested by several widgets.
func CompositionPayloadFromResult(page, widget string, outputs []string, result GroupResult) (CompositionPayload, error) {
	out := CompositionPayload{Page: page, Widget: widget, Outputs: []RetainedOutput{}}
	out.State, out.Code = compositionSelectionState(result, outputs)
	if result.Digest != GroupResultDigest(result) {
		return CompositionPayload{}, ErrInvalid
	}
	if result.State == "failed" {
		return out, nil
	}
	if result.Kind == "query" {
		if len(outputs) != 0 || result.Query == nil {
			return CompositionPayload{}, ErrInvalid
		}
		out.Query = clone(result.Query)
		return out, nil
	}
	byID := map[string]RetainedOutput{}
	for _, output := range result.Outputs {
		byID[output.ID] = output
	}
	for _, id := range outputs {
		output, ok := byID[id]
		if !ok {
			return CompositionPayload{}, ErrIncomplete
		}
		out.Outputs = append(out.Outputs, clone(output))
	}
	return out, nil
}

// CompositionRetainedBytes counts immutable execution payloads. Metadata
// indexes have separate hard schema bounds and never contain normalized rows.
func CompositionRetainedBytes(record CompositionRecord) int64 {
	body, _ := json.Marshal(record.Manifest)
	n := int64(len(body))
	for _, page := range record.Manifest.Pages {
		for _, widget := range page.Widgets {
			if value := CompositionStaticPayload(page.ID, widget); value != nil {
				body, _ = json.Marshal(value)
				n += int64(len(body))
			}
		}
	}
	for _, result := range record.Results {
		body, _ = json.Marshal(result)
		n += int64(len(body))
	}
	for _, plan := range record.Plans {
		body, _ = json.Marshal(plan)
		n += int64(len(body))
	}
	return n
}
