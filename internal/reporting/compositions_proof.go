package reporting

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// RequireComposition applies current authority independently of authored labels.
func RequireComposition(e identity.Envelope, m CompositionManifest) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	if m.Tenant != e.Tenant() || m.Actor != e.User() || m.Session != e.Session() {
		return access.ErrNotFound
	}
	if err := RequireDocument(e, m.Kind, m.Document, Execute); err != nil {
		return err
	}
	if m.Private {
		if err := RequireDocument(e, m.Kind, m.Document, Preview); err != nil {
			return err
		}
	}
	for _, page := range m.Pages {
		if err := RequireDocument(e, "report", page.Report, Execute); err != nil {
			return err
		}
		if page.Private {
			if err := RequireDocument(e, "report", page.Report, Preview); err != nil {
				return err
			}
		}
	}
	for _, group := range m.Groups {
		if err := requireCompositionGroup(e, group); err != nil {
			return err
		}
	}
	return nil
}

func requireCompositionGroup(e identity.Envelope, g CompositionGroup) error {
	if g.Kind == "block" {
		if err := Require(e, g.Block, Execute); err != nil {
			return err
		}
		if g.Private {
			if err := Require(e, g.Block, Preview); err != nil {
				return err
			}
		}
	} else if g.Kind != "query" || !e.Has("query.execute") {
		return access.ErrForbidden
	}
	datasets := []string{}
	for _, ref := range g.References {
		if ref.Kind == "dataset" && ref.Permission == "query" {
			datasets = append(datasets, ref.ID)
		}
	}
	if err := exec.Require(e, g.Binding, datasets); err != nil {
		return err
	}
	return RequireReferences(e, Execute, g.References)
}

func validComposition(m CompositionManifest) bool {
	if m.Version != CompositionVersion || !identity.Identifier(m.ID) || !identity.Identifier(m.Tenant) || !identity.Identifier(m.Actor) || !identity.Identifier(m.Session) || !documentKind(m.Kind) || !identity.Identifier(m.Document) || m.Revision < 1 || m.Revision > 256 || !hashValid(m.Digest) || !hashValid(m.RequestHash) || !hashValid(m.TaskHash) || m.Created.IsZero() || !m.Expires.After(m.Created) || m.Limits.Validate() != nil || m.ArtifactLimits.Validate() != nil || !slices.Contains([]string{"fail_closed", "allow_partial"}, m.Policy) || len(m.Pages) > m.Limits.MaxPages || len(m.Pages) == 0 && !m.Redacted || len(m.Groups) > m.Limits.MaxQueries || m.Redacted && m.Kind != "dashboard" {
		return false
	}
	retention := time.Duration(m.ArtifactLimits.Retention)
	if m.Private {
		retention = time.Duration(m.ArtifactLimits.PreviewRetention)
	}
	if !m.Expires.Equal(m.Created.Add(retention)) {
		return false
	}
	groups := map[string]CompositionGroup{}
	for _, g := range m.Groups {
		if !identity.Identifier(g.ID) || groups[g.ID].ID != "" || !identity.Identifier(g.Binding.Source) || !identity.Identifier(g.Binding.Context) || g.Private != m.Private || len(g.References) == 0 || len(g.References) > 128 || !locale(g.Locale) {
			return false
		}
		switch g.Kind {
		case "block":
			if !identity.Identifier(g.Block) || g.Revision < 1 || g.Revision > 256 || !hashValid(g.Definition) || !hashValid(g.Execution) || len(g.Outputs) < 1 || len(g.Outputs) > 64 || g.Query != nil || g.Origin != nil || g.Trust == nil || !slices.Contains([]string{"published", "certified_only", "explicit_stale"}, g.Policy) || g.Resolution.At.IsZero() || g.Resolved.At.IsZero() {
				return false
			}
		case "query":
			if g.Query == nil || g.Origin == nil || !validQueryOrigin(*g.Origin, *g.Query) || g.Block != "" || g.Revision != 0 || g.Trust != nil || len(g.Outputs) != 0 || len(g.Arguments) != 0 || g.Narrative || !m.Limits.LiveQueries || g.Query.Durability == "session_bound" && (!m.Limits.SessionBound || !m.Private || g.Origin.Actor != m.Actor || g.Origin.Session != m.Session) {
				return false
			}
		default:
			return false
		}
		outputs := map[string]bool{}
		for _, id := range g.Outputs {
			if !identity.Identifier(id) || outputs[id] {
				return false
			}
			outputs[id] = true
		}
		groups[g.ID] = g
	}
	pages, used := map[string]bool{}, map[string]bool{}
	count := 0
	for _, p := range m.Pages {
		if !identity.Identifier(p.ID) || pages[p.ID] || !identity.Identifier(p.Report) || p.Revision < 1 || p.Revision > 256 || !hashValid(p.Digest) || !text(p.Title, 256) || !locale(p.Locale) || p.Private && !m.Private {
			return false
		}
		if _, err := namedZone(p.Timezone); err != nil {
			return false
		}
		pages[p.ID] = true
		widgets := map[string]bool{}
		for _, w := range p.Widgets {
			count++
			if count > m.Limits.MaxWidgets || !validWidget(w.Definition, m.Limits) || widgets[w.Definition.ID] || w.Group != "" && w.Code != "" {
				return false
			}
			widgets[w.Definition.ID] = true
			if w.Definition.Kind == "text" {
				if w.Group != "" || w.Code != "" {
					return false
				}
				continue
			}
			if w.Code != "" {
				if !compositionCodeValid(w.Code) {
					return false
				}
				continue
			}
			g, exists := groups[w.Group]
			if !exists || g.Kind != w.Definition.Kind {
				return false
			}
			used[g.ID] = true
			if g.Kind == "block" {
				if w.Definition.Block.Block != g.Block || w.Definition.Block.Revision != g.Revision || len(w.Definition.Block.Outputs) == 0 {
					return false
				}
				for _, id := range w.Definition.Block.Outputs {
					if !slices.Contains(g.Outputs, id) {
						return false
					}
				}
			} else if digest(w.Definition.Query) != digest(g.Query) {
				return false
			}
		}
	}
	return len(used) == len(groups)
}

func compositionCodeValid(code string) bool {
	return slices.Contains([]string{"dependency_unavailable", "dependency_denied", "dependency_stale", "live_queries_disabled", "session_bound_disabled", "session_unavailable", "binding_invalid", "budget_exhausted", "query_failed", "query_truncated", "output_failed", "output_incomplete", "strict_omission", "deadline_exceeded", "query_indeterminate", "partial_report"}, code)
}

// PreparedComposition is an opaque in-process proof, not a persisted credential.
type PreparedComposition struct {
	encoded   []byte
	authority string
	deadline  time.Time
}

func prepareComposition(e identity.Envelope, m CompositionManifest) (PreparedComposition, error) {
	if !validComposition(m) {
		return PreparedComposition{}, ErrInvalid
	}
	body, err := json.Marshal(m)
	if err != nil || len(body) > m.Limits.MaxRetainedBytes {
		return PreparedComposition{}, ErrBudget
	}
	proof := PreparedComposition{encoded: body, authority: authority(e), deadline: e.Deadline()}
	_, err = proof.Checked(e)
	return proof, err
}

// Checked is repeated inside the accepting metadata transaction.
func (p PreparedComposition) Checked(e identity.Envelope) (CompositionManifest, error) {
	var m CompositionManifest
	if !e.Valid() || p.authority != authority(e) || !time.Now().Before(p.deadline) {
		return m, access.ErrUnauthenticated
	}
	if len(p.encoded) == 0 || len(p.encoded) > 16<<20 || json.Unmarshal(p.encoded, &m) != nil || !validComposition(m) || len(p.encoded) > m.Limits.MaxRetainedBytes {
		return CompositionManifest{}, ErrInvalid
	}
	if err := RequireComposition(e, m); err != nil {
		return CompositionManifest{}, err
	}
	return m, nil
}

// CompositionWrite is a pre-model marker, plan, result or terminal transition.
type CompositionWrite struct {
	Manifest CompositionManifest
	Kind     string
	Group    string
	Plan     *nlqexec.SavedPlan
	Result   *GroupResult
	Outcome  string
	Code     string
}

// PreparedCompositionWrite is bound to the current common operation lease.
type PreparedCompositionWrite struct {
	encoded []byte
	lease   jobs.RequestLease
}

func prepareCompositionWrite(inv jobs.Invocation, w CompositionWrite) (PreparedCompositionWrite, error) {
	body, err := json.Marshal(w)
	if err != nil || len(body) > 32<<20 {
		return PreparedCompositionWrite{}, ErrBudget
	}
	proof := PreparedCompositionWrite{encoded: body, lease: inv.Lease()}
	_, err = proof.Checked(inv)
	return proof, err
}

// Checked rejects fabricated, expired, unrelated and previous-owner writes.
func (p PreparedCompositionWrite) Checked(inv jobs.Invocation) (CompositionWrite, error) {
	var w CompositionWrite
	if len(p.encoded) == 0 || len(p.encoded) > 32<<20 || json.Unmarshal(p.encoded, &w) != nil || !validComposition(w.Manifest) || digest(p.lease) != digest(inv.Lease()) {
		return w, ErrInvalid
	}
	m := w.Manifest
	if inv.Lease().Task.ID != m.ID || inv.Lease().Task.ManifestHash != m.TaskHash {
		return CompositionWrite{}, ErrInvalid
	}
	e, err := inv.Current(m.Kind+".run", m.Document, m.RequestHash)
	if err != nil {
		return CompositionWrite{}, err
	}
	if err := RequireComposition(e, m); err != nil {
		return CompositionWrite{}, err
	}
	if w.Kind == "complete" {
		if w.Group != "" || w.Plan != nil || w.Result != nil || !slices.Contains([]string{"completed", "partial", "failed"}, w.Outcome) || w.Code != "" && w.Code != "partial_report" {
			return CompositionWrite{}, ErrInvalid
		}
		return w, nil
	}
	g, ok := compositionGroup(m, w.Group)
	if !ok || w.Outcome != "" || w.Code != "" {
		return CompositionWrite{}, ErrInvalid
	}
	switch w.Kind {
	case "query_start":
		if g.Kind != "query" || w.Plan != nil || w.Result != nil {
			return CompositionWrite{}, ErrInvalid
		}
	case "plan":
		if g.Kind != "query" || w.Plan == nil || w.Result != nil || !identity.Identifier(w.Plan.Query) || w.Plan.Operation != "composition:"+m.ID+":"+g.ID || !hashValid(w.Plan.InputDigest) || !hashValid(w.Plan.QueryDigest) || w.Plan.BindingDigest != exec.Hash(g.Binding) {
			return CompositionWrite{}, ErrInvalid
		}
	case "group":
		if w.Plan != nil || w.Result == nil || CheckCompositionResult(m, g, *w.Result) != nil {
			return CompositionWrite{}, ErrInvalid
		}
	default:
		return CompositionWrite{}, ErrInvalid
	}
	return w, nil
}

func compositionGroup(m CompositionManifest, id string) (CompositionGroup, bool) {
	for _, g := range m.Groups {
		if g.ID == id {
			return g, true
		}
	}
	return CompositionGroup{}, false
}

// GroupResultDigest omits its own hash field and preserves exact result types.
func GroupResultDigest(r GroupResult) string {
	r.Digest = ""
	return digest(r)
}

// CheckCompositionResult prevents trust, partition and output substitution.
func CheckCompositionResult(m CompositionManifest, g CompositionGroup, r GroupResult) error {
	if r.Group != g.ID || r.Kind != g.Kind || r.Digest != GroupResultDigest(r) || !slices.Contains([]string{"completed", "partial", "failed"}, r.State) || r.Code != "" && !compositionCodeValid(r.Code) || r.State == "completed" && r.Code != "" || r.State != "completed" && r.Code == "" {
		return ErrInvalid
	}
	if r.State == "failed" {
		if r.Query != nil || r.Block != nil || len(r.Outputs) != 0 || r.Observed != nil || r.ChildRun != "" || r.QueryPlan != nil {
			return ErrInvalid
		}
		return nil
	}
	if r.Observed == nil || r.Observed.Before(m.Created) || r.Observed.After(m.Expires) {
		return ErrInvalid
	}
	if g.Kind == "block" {
		b := r.Block
		if b == nil || r.Query != nil || r.QueryPlan != nil || b.ID != r.ChildRun || b.Block != g.Block || b.Revision != g.Revision || b.RevisionDigest != g.Definition || b.PartitionDigest != exec.Hash(g.Binding) || b.Private != g.Private || len(r.Outputs) != len(g.Outputs) || b.Observed == nil || !b.Observed.Equal(*r.Observed) {
			return ErrInvalid
		}
		for i, output := range r.Outputs {
			if output.ID != g.Outputs[i] || output.Digest == "" || r.State == "completed" && output.State != "succeeded" {
				return ErrInvalid
			}
		}
	} else {
		q := r.Query
		if q == nil || r.Block != nil || len(r.Outputs) != 0 || r.ChildRun != "" || r.QueryPlan == nil || q.Query != r.QueryPlan.Query || q.Partition != exec.Hash(g.Binding) || q.SemanticDigest != g.Origin.SemanticDigest || q.Execution.Result == nil || !successful(q.Execution.Attempt.Status) || q.Execution.Attempt.RemoteState != "stopped" || q.Execution.Attempt.Finished == nil || !q.Execution.Attempt.Finished.Equal(*r.Observed) || q.Execution.Attempt.Manifest.Operation != r.QueryPlan.Operation || q.Execution.Attempt.Manifest.Session != m.Session || q.Execution.Attempt.Manifest.Preview != m.Private || r.State == "completed" && (q.EvidenceStale || q.Execution.Result.Outcome == "truncated") {
			return ErrInvalid
		}
	}
	return nil
}

// CompositionCompletion derives state from all selected widgets. Omissions,
// truncation, redaction and output failures never become a complete report.
func CompositionCompletion(m CompositionManifest, results []GroupResult) (string, string, bool, error) {
	byID := map[string]GroupResult{}
	for _, r := range results {
		g, exists := compositionGroup(m, r.Group)
		if !exists || byID[r.Group].Group != "" || CheckCompositionResult(m, g, r) != nil {
			return "", "", false, ErrInvalid
		}
		byID[r.Group] = r
	}
	incomplete := m.Redacted
	var first *time.Time
	mixed := false
	for _, page := range m.Pages {
		for _, w := range page.Widgets {
			if w.Definition.Kind == "text" {
				continue
			}
			if w.Code != "" {
				incomplete = true
				continue
			}
			r, exists := byID[w.Group]
			if !exists {
				return "", "", false, ErrIncomplete
			}
			incomplete = incomplete || r.State != "completed"
			if r.Observed != nil {
				if first == nil {
					first = r.Observed
				} else if !first.Equal(*r.Observed) {
					mixed = true
				}
			}
		}
	}
	if incomplete {
		if m.Policy == "fail_closed" {
			return "failed", "partial_report", mixed, nil
		}
		return "partial", "partial_report", mixed, nil
	}
	return "completed", "", mixed, nil
}
