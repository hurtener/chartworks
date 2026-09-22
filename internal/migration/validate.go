package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

const (
	maxObjects = 10000
	maxBytes   = 10 << 20
)

var kindRank = map[Kind]int{
	KindSource: 0, KindUpload: 1, KindProfile: 2, KindTopic: 3,
	KindRule: 4, KindTemplate: 5, KindBlock: 6, KindReport: 7,
	KindDashboard: 8, KindFilter: 9, KindSchedule: 10, KindRun: 11,
	KindArtifact: 12, KindRendition: 13, KindCertificate: 14,
	KindTombstone: 15, KindCalibration: 16,
}

var forbiddenKeys = map[string]bool{
	"token": true, "access_token": true, "refresh_token": true, "bearer": true,
	"password": true, "secret": true, "client_secret": true, "api_key": true,
	"private_key": true, "credential": true, "credentials": true, "grant": true,
	"role": true, "roles": true, "user": true, "users": true,
}

var requiredFeatures = func() map[string]bool {
	out := map[string]bool{}
	for _, prefix := range []struct {
		name  string
		count int
	}{{"B", 20}, {"R", 16}, {"Q", 10}, {"N", 16}} {
		for i := 1; i <= prefix.count; i++ {
			out[prefix.name+twoDigits(i)] = true
		}
	}
	return out
}()

type calibrationThreshold struct {
	Template  string  `json:"template"`
	Threshold float64 `json:"threshold"`
}

type calibrationPayload struct {
	PromptPack            string                 `json:"prompt_pack"`
	FallbackPromptPack    string                 `json:"fallback_prompt_pack,omitempty"`
	OptimizationRevision  string                 `json:"optimization_revision"`
	Locale                string                 `json:"locale"`
	Temperature           float64                `json:"temperature"`
	MaxOutputTokens       int                    `json:"max_output_tokens"`
	ExamplePolicyRevision string                 `json:"example_policy_revision"`
	TemplateThresholds    []calibrationThreshold `json:"template_thresholds"`
}

func twoDigits(i int) string {
	return fmt.Sprintf("%02d", i)
}

func validateManifest(m Manifest) (string, error) {
	if m.Version != ManifestVersion || !identity.Identifier(m.Batch) || !identity.Identifier(m.Cohort) || !identity.Identifier(m.SourceSnapshot) || !identity.Identifier(m.Engine) || !identity.Identifier(m.Dialect) || len(m.Objects) == 0 || len(m.Objects) > maxObjects {
		return "", ErrInvalid
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > maxBytes {
		return "", ErrLimit
	}
	refs := map[string]Object{}
	mappings := map[string]Mapping{}
	for _, x := range m.Mappings {
		if _, ok := kindRank[x.Kind]; !ok || !identity.Identifier(x.ExternalRef) || !identity.Identifier(x.Destination) || x.Revision < 1 || mappings[x.ExternalRef].ExternalRef != "" {
			return "", ErrInvalid
		}
		mappings[x.ExternalRef] = x
	}
	fieldPaths := map[string]bool{}
	for _, f := range m.Fields {
		if len(f.Path) < 3 || len(f.Path) > 512 || fieldPaths[f.Path] || (f.Status != "retained" && f.Status != "transformed" && f.Status != "dropped" && f.Status != "unsupported") || (f.Status != "retained" && strings.TrimSpace(f.Reason) == "") {
			return "", ErrInvalid
		}
		fieldPaths[f.Path] = true
	}
	for _, o := range m.Objects {
		if _, ok := kindRank[o.Kind]; !ok || !identity.Identifier(o.ExternalRef) || refs[o.ExternalRef].ExternalRef != "" || o.Revision < 1 || o.PayloadVersion != "v1" || len(o.Payload) < 2 || len(o.Payload) > 1<<20 || !json.Valid([]byte(o.Payload)) || !identity.Identifier(o.Origin) || (o.Lifecycle != "private_draft" && o.Lifecycle != "historical" && o.Lifecycle != "deleted") {
			return "", ErrInvalid
		}
		if o.Kind == KindTombstone {
			if o.Deletes == nil || o.Deletes.Kind == KindTombstone || o.Deletes.Kind == KindCalibration || !identity.Identifier(o.Deletes.ExternalRef) || o.Deletes.Revision < 1 || o.Lifecycle != "deleted" {
				return "", ErrInvalid
			}
		} else if o.Deletes != nil {
			return "", ErrInvalid
		}
		var payload any
		if json.Unmarshal([]byte(o.Payload), &payload) != nil || containsForbidden(payload) {
			return "", ErrInvalid
		}
		object, ok := payload.(map[string]any)
		if !ok || len(object) == 0 {
			return "", ErrInvalid
		}
		for key := range object {
			if !fieldPaths[o.ExternalRef+"."+key] {
				return "", ErrInvalid
			}
		}
		if o.Retention.ExpiresAt != nil && (o.Retention.ExpiresAt.IsZero() || o.Retention.ExpiresAt.Before(time.Unix(0, 0))) {
			return "", ErrInvalid
		}
		refs[o.ExternalRef] = o
	}
	for _, o := range m.Objects {
		seen := map[string]bool{}
		for _, parent := range o.Parents {
			p, ok := refs[parent]
			if !ok || seen[parent] || kindRank[p.Kind] >= kindRank[o.Kind] {
				return "", ErrInvalid
			}
			seen[parent] = true
		}
		if o.Kind == KindSource {
			if mapping, ok := mappings[o.ExternalRef]; !ok || mapping.Kind != KindSource {
				return "", ErrInvalid
			}
		}
	}
	features := map[string]bool{}
	for _, e := range m.Evidence {
		known := requiredFeatures[e.Feature] || e.Feature == "Q11"
		if !known || features[e.Feature] || (e.Disposition != "required" && e.Disposition != "excluded") || (e.Outcome != "passed" && e.Outcome != "failed" && e.Outcome != "unsupported") || !identity.Identifier(e.Reference) || !identity.Identifier(e.Source) || !identity.Identifier(e.SourceVersion) {
			return "", ErrInvalid
		}
		if e.Feature == "Q11" && (e.Disposition != "excluded" || e.Outcome != "unsupported") || e.Feature != "Q11" && e.Disposition != "required" {
			return "", ErrInvalid
		}
		features[e.Feature] = true
	}
	for feature := range requiredFeatures {
		if !features[feature] {
			return "", ErrInvalid
		}
	}
	if !features["Q11"] {
		return "", ErrInvalid
	}
	if m.Calibration != nil {
		c := m.Calibration
		if c.State != "review_candidate" || !identity.Identifier(c.Revision) || !identity.Identifier(c.ModelVersion) || !identity.Identifier(c.EmbeddingSpace) || !identity.Identifier(c.BudgetVersion) || len(c.Payload) > 1<<20 || !json.Valid([]byte(c.Payload)) {
			return "", ErrInvalid
		}
		var payload any
		if json.Unmarshal([]byte(c.Payload), &payload) != nil || containsForbidden(payload) || !validCalibrationPayload(c.Payload) {
			return "", ErrInvalid
		}
	}
	if m.Boundary != nil && (!identity.Identifier(m.Boundary.Stream) || m.Boundary.ScheduleVersion < 1 || m.Boundary.ResumeAfter.IsZero()) {
		return "", ErrInvalid
	}
	canonical := m
	canonical.Objects = append([]Object(nil), m.Objects...)
	canonical.Mappings = append([]Mapping(nil), m.Mappings...)
	canonical.Fields = append([]FieldDisposition(nil), m.Fields...)
	canonical.Evidence = append([]Evidence(nil), m.Evidence...)
	sort.Slice(canonical.Objects, func(i, j int) bool { return canonical.Objects[i].ExternalRef < canonical.Objects[j].ExternalRef })
	sort.Slice(canonical.Mappings, func(i, j int) bool { return canonical.Mappings[i].ExternalRef < canonical.Mappings[j].ExternalRef })
	sort.Slice(canonical.Fields, func(i, j int) bool { return canonical.Fields[i].Path < canonical.Fields[j].Path })
	sort.Slice(canonical.Evidence, func(i, j int) bool { return canonical.Evidence[i].Feature < canonical.Evidence[j].Feature })
	canonicalRaw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(canonicalRaw)
	return hex.EncodeToString(sum[:]), nil
}

func validCalibrationPayload(raw string) bool {
	var payload calibrationPayload
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&payload) != nil || !identity.Identifier(payload.PromptPack) || payload.FallbackPromptPack != "" && !identity.Identifier(payload.FallbackPromptPack) || !identity.Identifier(payload.OptimizationRevision) || !identity.Identifier(payload.Locale) || !identity.Identifier(payload.ExamplePolicyRevision) || payload.Temperature < 0 || payload.Temperature > 2 || payload.MaxOutputTokens < 1 || payload.MaxOutputTokens > 1_000_000 || len(payload.TemplateThresholds) < 1 || len(payload.TemplateThresholds) > 256 {
		return false
	}
	seen := map[string]bool{}
	for _, threshold := range payload.TemplateThresholds {
		if !identity.Identifier(threshold.Template) || threshold.Threshold < 0 || threshold.Threshold > 1 || seen[threshold.Template] {
			return false
		}
		seen[threshold.Template] = true
	}
	return true
}

func objectDigest(o Object) string {
	raw, _ := json.Marshal(o)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func containsForbidden(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			n := strings.ToLower(strings.TrimSpace(key))
			if forbiddenKeys[n] || strings.Contains(n, "credential") || strings.HasSuffix(n, "_token") || containsForbidden(value) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if containsForbidden(value) {
				return true
			}
		}
	}
	return false
}

func ordered(m Manifest) ([]Object, error) {
	byRef := map[string]Object{}
	degree := map[string]int{}
	children := map[string][]string{}
	for _, o := range m.Objects {
		byRef[o.ExternalRef] = o
		degree[o.ExternalRef] = len(o.Parents)
		for _, p := range o.Parents {
			children[p] = append(children[p], o.ExternalRef)
		}
	}
	ready := []string{}
	for ref, d := range degree {
		if d == 0 {
			ready = append(ready, ref)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		a, b := byRef[ready[i]], byRef[ready[j]]
		if kindRank[a.Kind] != kindRank[b.Kind] {
			return kindRank[a.Kind] < kindRank[b.Kind]
		}
		return ready[i] < ready[j]
	})
	out := make([]Object, 0, len(m.Objects))
	for len(ready) > 0 {
		ref := ready[0]
		ready = ready[1:]
		out = append(out, byRef[ref])
		for _, child := range children[ref] {
			degree[child]--
			if degree[child] == 0 {
				ready = append(ready, child)
			}
		}
		sort.Slice(ready, func(i, j int) bool {
			a, b := byRef[ready[i]], byRef[ready[j]]
			if kindRank[a.Kind] != kindRank[b.Kind] {
				return kindRank[a.Kind] < kindRank[b.Kind]
			}
			return ready[i] < ready[j]
		})
	}
	if len(out) != len(m.Objects) {
		return nil, ErrInvalid
	}
	return out, nil
}
