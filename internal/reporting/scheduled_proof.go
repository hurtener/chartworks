package reporting

import (
	"time"

	"github.com/hurtener/chartworks/internal/jobs"
)

// CheckScheduledFrozen prevents the scheduled seal from replacing an accepted
// block, output order, locale, partition policy or logical resolution instant.
func CheckScheduledFrozen(j jobs.Job, m RunManifest) error {
	if !j.Valid() || j.Kind != jobs.ReportingKind || j.Reporting == nil || j.Reporting.Blocked != "" {
		return jobs.ErrAuthority
	}
	d := j.Reporting
	t := d.Target
	if t.ResourceKind() != "block" || m.ID != j.ID || m.Tenant != j.Tenant || m.Actor != j.Executor || m.Session != j.ID ||
		m.Block != t.ID || m.Revision.Number != d.Revision || m.Revision.Digest != d.Digest || m.Private ||
		m.TaskHash != j.ManifestHash || m.RequestHash != d.Input.InputHash || m.Locale != t.Locale ||
		m.Resolved.Timezone != t.Timezone || !m.Resolved.At.Equal(j.DueAt) || len(m.Outputs) != len(t.Outputs) ||
		m.Limits.MaxRows > t.Budget.MaxRows || m.Limits.MaxResultBytes > t.Budget.MaxBytes ||
		time.Duration(m.Limits.Timeout) > time.Duration(t.Budget.TimeoutMillis)*time.Millisecond {
		return ErrStale
	}
	policy := "published"
	if t.Type == "block" {
		policy = "certified_only"
	}
	if m.Policy != policy {
		return ErrStale
	}
	for i, output := range m.Outputs {
		if output.ID != t.Outputs[i] || output.Kind == "narrative" && !t.Narrative {
			return ErrStale
		}
	}
	return nil
}

// CheckScheduledComposition checks the full accepted widget selection and each
// occurrence pin, including two widgets referencing different revisions of the
// same block. It grants no data or execution authority by itself.
func CheckScheduledComposition(j jobs.Job, m CompositionManifest) error {
	if !j.Valid() || j.Kind != jobs.ReportingKind || j.Reporting == nil || j.Reporting.Blocked != "" {
		return jobs.ErrAuthority
	}
	d := j.Reporting
	t := d.Target
	if t.ResourceKind() != "report" || m.Kind != "report" || m.ID != j.ID || m.Tenant != j.Tenant || m.Actor != j.Executor ||
		m.Session != j.ID || m.Document != t.ID || m.Revision != d.Revision || m.Digest != d.Digest || m.Private || m.Redacted ||
		m.TaskHash != j.ManifestHash || m.RequestHash != d.Input.InputHash || len(m.Pages) != 1 || m.Pages[0].ID != "main" ||
		m.Pages[0].Locale != t.Locale || m.Pages[0].Timezone != t.Timezone ||
		m.ArtifactLimits.MaxRows > t.Budget.MaxRows || m.ArtifactLimits.MaxResultBytes > t.Budget.MaxBytes ||
		time.Duration(m.Limits.Timeout) > time.Duration(t.Budget.TimeoutMillis)*time.Millisecond {
		return ErrStale
	}
	if t.Type == "saved_question" && (len(m.Pages[0].Widgets) != 1 || m.Pages[0].Widgets[0].Definition.ID != t.Widget) {
		return ErrStale
	}
	pins := map[string]jobs.ReportingPin{}
	for _, p := range d.Pins {
		pins[p.ID] = p
	}
	groups := map[string]CompositionGroup{}
	for _, g := range m.Groups {
		groups[g.ID] = g
	}
	for _, w := range m.Pages[0].Widgets {
		if w.Code != "" {
			return ErrStale
		}
		g := groups[w.Group]
		switch w.Definition.Kind {
		case "block":
			pin, exists := pins[w.Definition.ID]
			if !exists || g.Kind != "block" || pin.Block != g.Block || pin.Revision != g.Revision || pin.Digest != g.Definition ||
				!g.Resolution.At.Equal(j.DueAt) || !g.Resolved.At.Equal(j.DueAt) || g.Narrative && !t.Narrative {
				return ErrStale
			}
			delete(pins, w.Definition.ID)
		case "query":
			if !t.Dynamic || g.Kind != "query" || g.Query == nil || g.Query.Durability != "replayable" {
				return ErrStale
			}
		case "text":
			if t.Type == "saved_question" {
				return ErrStale
			}
		default:
			return ErrStale
		}
	}
	if len(pins) != 0 {
		return ErrStale
	}
	return nil
}
