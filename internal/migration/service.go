//nolint:revive // Public service and adapter names are the migration integration seam.
package migration

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

type Repository interface {
	Begin(context.Context, identity.Envelope, Manifest, string, Plan) (Batch, bool, error)
	Batch(context.Context, identity.Envelope, string) (Batch, Manifest, Plan, error)
	Checkpoint(context.Context, identity.Envelope, string, int64, ObjectPlan, string) (Batch, error)
	ApplyCheckpoint(context.Context, identity.Envelope, string, int64, ObjectPlan, func(context.Context) (string, error)) (Batch, error)
	Export(context.Context, identity.Envelope, string, string, int) (Export, error)
	Cutover(context.Context, identity.Envelope, Batch, int64, string, string, string, OccurrenceBoundary) (Cutover, error)
	CurrentCutover(context.Context, identity.Envelope, string) (Cutover, error)
	Rollback(context.Context, identity.Envelope, string, int64, string, []string) (Cutover, error)
	Erase(context.Context, identity.Envelope, string, int) (EraseResult, error)
}

// Adapter is a public domain-service seam. It must call the owning service; it
// may not write another domain package's private tables. Validate has no writes.
type Adapter interface {
	Validate(context.Context, identity.Envelope, Object, Mapping) error
	Apply(context.Context, identity.Envelope, Object, Mapping, string) (destination string, err error)
}

type EvidenceVerifier interface {
	Verify(context.Context, identity.Envelope, Evidence) error
}

type EvidenceVerifierFunc func(context.Context, identity.Envelope, Evidence) error

func (f EvidenceVerifierFunc) Verify(ctx context.Context, e identity.Envelope, evidence Evidence) error {
	return f(ctx, e, evidence)
}

type AdapterFuncs struct {
	ValidateFunc func(context.Context, identity.Envelope, Object, Mapping) error
	ApplyFunc    func(context.Context, identity.Envelope, Object, Mapping, string) (string, error)
}

func (a AdapterFuncs) Validate(ctx context.Context, e identity.Envelope, object Object, mapping Mapping) error {
	if a.ValidateFunc == nil {
		return ErrUnsupported
	}
	return a.ValidateFunc(ctx, e, object, mapping)
}

func (a AdapterFuncs) Apply(ctx context.Context, e identity.Envelope, object Object, mapping Mapping, digest string) (string, error) {
	if a.ApplyFunc == nil {
		return "", ErrUnsupported
	}
	return a.ApplyFunc(ctx, e, object, mapping, digest)
}

type Service struct {
	repo     Repository
	adapters map[Kind]Adapter
	now      func() time.Time
	evidence EvidenceVerifier
}

func New(repo Repository, adapters map[Kind]Adapter, now func() time.Time, verifiers ...EvidenceVerifier) (*Service, error) {
	if repo == nil || len(adapters) == 0 || len(verifiers) > 1 {
		return nil, ErrInvalid
	}
	copy := make(map[Kind]Adapter, len(adapters))
	for kind, adapter := range adapters {
		if _, ok := kindRank[kind]; !ok || adapter == nil {
			return nil, ErrInvalid
		}
		copy[kind] = adapter
	}
	if now == nil {
		now = time.Now
	}
	var evidence EvidenceVerifier
	if len(verifiers) == 1 {
		evidence = verifiers[0]
	}
	return &Service{repo: repo, adapters: copy, now: now, evidence: evidence}, nil
}

func require(e identity.Envelope, action, permission string) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	return access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "tenant", Permission: permission, ID: e.Tenant()})
}

func (s *Service) DryRun(ctx context.Context, e identity.Envelope, in DryRunRequest) (Plan, error) {
	if err := require(e, "migration.read", "read"); err != nil {
		return Plan{}, err
	}
	return s.dryRun(ctx, e, in.Manifest)
}

func (s *Service) dryRun(ctx context.Context, e identity.Envelope, manifest Manifest) (Plan, error) {
	digest, err := validateManifest(manifest)
	if err != nil {
		return Plan{}, err
	}
	objects, err := ordered(manifest)
	if err != nil {
		return Plan{}, err
	}
	mapping := map[string]Mapping{}
	for _, m := range manifest.Mappings {
		mapping[m.ExternalRef] = m
	}
	plan := Plan{Batch: manifest.Batch, Cohort: manifest.Cohort, Digest: digest, Ready: true, Objects: make([]ObjectPlan, 0, len(objects)), Fields: append([]FieldDisposition(nil), manifest.Fields...), Evidence: append([]Evidence(nil), manifest.Evidence...)}
	for _, evidence := range manifest.Evidence {
		if evidence.Disposition == "required" && (s.evidence == nil || s.evidence.Verify(ctx, e, evidence) != nil) {
			plan.Ready = false
			plan.Limitations = append(plan.Limitations, "required owner evidence unverified: "+evidence.Feature)
		}
	}
	for _, o := range objects {
		p := ObjectPlan{ExternalRef: o.ExternalRef, Kind: o.Kind, DependsOn: append([]string(nil), o.Parents...), Revision: o.Revision, Digest: objectDigest(o), Deletes: o.Deletes}
		mapped := mapping[o.ExternalRef]
		p.Destination = mapped.Destination
		switch {
		case o.Retention.ExpiresAt != nil && !s.now().UTC().Before(o.Retention.ExpiresAt.UTC()):
			p.Action, p.Reason = "retention_quarantine", "source retention expired before import"
			plan.Limitations = append(plan.Limitations, "expired object quarantined: "+o.ExternalRef)
			if o.Kind == KindSource || o.Kind == KindSchedule {
				plan.Ready = false
			}
		case o.Kind == KindCertificate || o.Kind == KindRun || o.Kind == KindArtifact || o.Kind == KindRendition:
			p.Action, p.Reason = "historical_quarantine", "historical evidence requires current authority and fresh attestation"
		case o.Kind == KindTombstone || o.Lifecycle == "deleted":
			p.Action = "tombstone"
		case hasUnsupported(manifest.Fields, o.ExternalRef):
			p.Action, p.Reason = "unsupported_quarantine", "one or more fields are explicitly unsupported"
			plan.Ready = false
		case s.adapters[o.Kind] == nil:
			p.Action, p.Reason = "unsupported_quarantine", "no installed destination adapter"
			plan.Ready = false
		default:
			p.Action = "install_private"
			if err := s.adapters[o.Kind].Validate(ctx, e, o, mapped); err != nil {
				if errors.Is(err, ErrUnsupported) {
					p.Action, p.Reason, plan.Ready = "unsupported_quarantine", "destination rejected capability", false
				} else {
					return Plan{}, err
				}
			}
		}
		plan.Objects = append(plan.Objects, p)
		if (o.Kind == KindSource || o.Kind == KindSchedule) && p.Action != "install_private" {
			plan.Ready = false
		}
	}
	sort.Strings(plan.Limitations)
	return plan, nil
}

func hasUnsupported(fields []FieldDisposition, ref string) bool {
	prefix := ref + "."
	for _, f := range fields {
		if stringsHasPrefix(f.Path, prefix) && f.Status == "unsupported" {
			return true
		}
	}
	return false
}

func stringsHasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}

func (s *Service) Import(ctx context.Context, e identity.Envelope, in ImportRequest) (Batch, error) {
	if err := require(e, "migration.write", "write"); err != nil {
		return Batch{}, err
	}
	if in.Expected < 0 {
		return Batch{}, ErrInvalid
	}
	plan, err := s.dryRun(ctx, e, in.Manifest)
	if err != nil {
		return Batch{}, err
	}
	batch, existing, err := s.repo.Begin(ctx, e, in.Manifest, plan.Digest, plan)
	if err != nil {
		return Batch{}, err
	}
	if existing && batch.Digest != plan.Digest {
		return Batch{}, ErrConflict
	}
	if !existing && in.Expected != 0 {
		return Batch{}, ErrConflict
	}
	if existing && in.Expected != batch.Revision {
		return Batch{}, ErrConflict
	}
	return s.resume(ctx, e, batch, in.Manifest, plan)
}

func (s *Service) Resume(ctx context.Context, e identity.Envelope, in ResumeRequest) (Batch, error) {
	if err := require(e, "migration.write", "write"); err != nil {
		return Batch{}, err
	}
	batch, manifest, plan, err := s.repo.Batch(ctx, e, in.Batch)
	if err != nil {
		return Batch{}, err
	}
	if batch.Revision != in.Expected {
		return Batch{}, ErrConflict
	}
	return s.resume(ctx, e, batch, manifest, plan)
}

func (s *Service) resume(ctx context.Context, e identity.Envelope, batch Batch, manifest Manifest, plan Plan) (Batch, error) {
	objects := map[string]Object{}
	mappings := map[string]Mapping{}
	for _, o := range manifest.Objects {
		objects[o.ExternalRef] = o
	}
	for _, m := range manifest.Mappings {
		mappings[m.ExternalRef] = m
	}
	start := batch.Applied + batch.Quarantined
	for i := start; i < len(plan.Objects); i++ {
		if ctx.Err() != nil {
			return batch, ctx.Err()
		}
		item := plan.Objects[i]
		object := objects[item.ExternalRef]
		if err := require(e, "migration.write", "write"); err != nil {
			return batch, err
		}
		if object.Retention.ExpiresAt != nil && !s.now().UTC().Before(object.Retention.ExpiresAt.UTC()) {
			item.Action, item.Reason = "retention_quarantine", "source retention expired before apply"
		}
		destination := item.Destination
		if item.Action == "install_private" || item.Action == "tombstone" {
			adapter := s.adapters[item.Kind]
			if adapter == nil {
				return batch, ErrUnsupported
			}
			if err := adapter.Validate(ctx, e, object, mappings[item.ExternalRef]); err != nil {
				return batch, err
			}
			var err error
			batch, err = s.repo.ApplyCheckpoint(ctx, e, batch.ID, batch.Revision, item, func(ctx context.Context) (string, error) {
				destination, applyErr := adapter.Apply(ctx, e, object, mappings[item.ExternalRef], plan.Digest)
				if applyErr != nil {
					return "", applyErr
				}
				return destination + ":applied", nil
			})
			if err != nil {
				return batch, err
			}
			continue
		}
		var err error
		batch, err = s.repo.Checkpoint(ctx, e, batch.ID, batch.Revision, item, destination+":quarantined")
		if err != nil {
			return batch, err
		}
	}
	return batch, nil
}

func (s *Service) Export(ctx context.Context, e identity.Envelope, in ExportRequest) (Export, error) {
	if err := require(e, "migration.read", "read"); err != nil {
		return Export{}, err
	}
	if err := access.Require(e, "ops.read", access.Tenant(e, "export")); err != nil {
		return Export{}, err
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return Export{}, ErrInvalid
	}
	batch, manifest, _, err := s.repo.Batch(ctx, e, in.Batch)
	if err != nil {
		return Export{}, err
	}
	for _, object := range manifest.Objects {
		if object.Retention.ExpiresAt != nil && !s.now().UTC().Before(object.Retention.ExpiresAt.UTC()) {
			return Export{}, ErrNotReady
		}
	}
	out, err := s.repo.Export(ctx, e, in.Batch, in.After, in.Limit)
	if err != nil {
		return Export{}, err
	}
	if out.Batch.ID != batch.ID || out.Batch.Digest != batch.Digest {
		return Export{}, ErrConflict
	}
	for _, object := range manifest.Objects {
		if object.Retention.ExpiresAt != nil && !s.now().UTC().Before(object.Retention.ExpiresAt.UTC()) {
			return Export{}, ErrNotReady
		}
	}
	return out, nil
}

func (s *Service) Cutover(ctx context.Context, e identity.Envelope, in CutoverRequest) (Cutover, error) {
	if err := require(e, "migration.cutover", "write"); err != nil {
		return Cutover{}, err
	}
	batch, manifest, plan, err := s.repo.Batch(ctx, e, in.Batch)
	if err != nil {
		return Cutover{}, err
	}
	if !plan.Ready || batch.State != "complete" || manifest.Boundary == nil || !identity.Identifier(in.Route) || in.Expected == 0 && !identity.Identifier(in.PreviousRoute) || !identity.Identifier(in.OperatorRef) {
		return Cutover{}, ErrNotReady
	}
	for _, object := range manifest.Objects {
		if (object.Kind == KindSource || object.Kind == KindSchedule) && object.Retention.ExpiresAt != nil && !s.now().UTC().Before(object.Retention.ExpiresAt.UTC()) {
			return Cutover{}, ErrNotReady
		}
	}
	if s.evidence == nil {
		return Cutover{}, ErrNotReady
	}
	for _, evidence := range manifest.Evidence {
		if evidence.Disposition == "required" {
			if err := s.evidence.Verify(ctx, e, evidence); err != nil {
				return Cutover{}, ErrNotReady
			}
		}
	}
	mappings := make(map[string]Mapping, len(manifest.Mappings))
	for _, mapping := range manifest.Mappings {
		mappings[mapping.ExternalRef] = mapping
	}
	for _, object := range manifest.Objects {
		if object.Kind != KindSource {
			continue
		}
		mapping := mappings[object.ExternalRef]
		var binding sourceBindingPayload
		if json.Unmarshal([]byte(object.Payload), &binding) != nil || mapping.Destination == "" {
			return Cutover{}, ErrNotReady
		}
		if err := access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: mapping.Destination}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: binding.Context}); err != nil {
			return Cutover{}, err
		}
		adapter := s.adapters[KindSource]
		if adapter == nil {
			return Cutover{}, ErrNotReady
		}
		if err := adapter.Validate(ctx, e, object, mapping); err != nil {
			if errors.Is(err, access.ErrUnauthenticated) || errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
				return Cutover{}, err
			}
			return Cutover{}, ErrNotReady
		}
	}
	return s.repo.Cutover(ctx, e, batch, in.Expected, in.Route, in.PreviousRoute, in.OperatorRef, *manifest.Boundary)
}

func (s *Service) Rollback(ctx context.Context, e identity.Envelope, in RollbackRequest) (Cutover, error) {
	if err := require(e, "migration.cutover", "write"); err != nil {
		return Cutover{}, err
	}
	if !identity.Identifier(in.Cohort) || !identity.Identifier(in.OperatorRef) || len(in.Effects) > 100 {
		return Cutover{}, ErrInvalid
	}
	for _, effect := range in.Effects {
		if len(effect) < 3 || len(effect) > 256 || !utf8.ValidString(effect) || strings.TrimSpace(effect) != effect || strings.ContainsAny(effect, "\x00\r\n") {
			return Cutover{}, ErrInvalid
		}
	}
	return s.repo.Rollback(ctx, e, in.Cohort, in.Expected, in.OperatorRef, in.Effects)
}

func (s *Service) Erase(ctx context.Context, e identity.Envelope, in EraseRequest) (EraseResult, error) {
	if err := require(e, "migration.erase", "erase"); err != nil {
		return EraseResult{}, err
	}
	if !identity.Identifier(in.Batch) || in.Limit < 1 || in.Limit > 1000 {
		return EraseResult{}, ErrInvalid
	}
	return s.repo.Erase(ctx, e, in.Batch, in.Limit)
}
