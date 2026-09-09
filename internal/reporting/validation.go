package reporting

import (
	"context"
	"github.com/hurtener/chartworks/internal/sources"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

func (s *Service) validateWork(ctx context.Context, e identity.Envelope, id string, snapshot Snapshot, in ValidateRequest) (ValidationRecord, exec.Result, Resolved, []ResourceReference, error) {
	var record ValidationRecord
	var result exec.Result
	var resolved Resolved
	if !s.CanValidate() {
		return record, result, resolved, nil, ErrUnavailable
	}
	if snapshot.State.Archived || snapshot.State.DraftState == "rejected" && snapshot.PublishedAt == nil {
		return record, result, resolved, nil, store.ErrConflict
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return record, result, resolved, nil, err
	}
	d := snapshot.Revision.Definition
	if err := access.Require(e, "sources.query", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: d.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: d.Context}); err != nil {
		return record, result, resolved, nil, err
	}
	resolved, err := ResolveParameters(d.Parameters, in.Arguments, acceptedResolution(in.Resolution))
	if err != nil {
		return record, result, resolved, nil, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return record, result, resolved, nil, ctx.Err()
	default:
		return record, result, resolved, nil, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.limits.ValidationTimeout))
	defer cancel()
	definitions, refs, err := s.resolveDefinitions(ctx, e, d, true)
	if err != nil {
		return record, result, resolved, nil, err
	}
	binding, err := s.sources.ContextBinding(ctx, e, d.Source, d.Context)
	if err != nil {
		return record, result, resolved, nil, err
	}
	if binding.Tenant != e.Tenant() || binding.Source != d.Source || binding.Context != d.Context {
		return record, result, resolved, nil, ErrStale
	}
	var catalog sources.CatalogIdentity
	if observer, ok := s.sources.(CatalogObserver); ok {
		observed, err := observer.ObserveCatalog(ctx, e, d.Source, d.Context)
		if err != nil {
			return record, result, resolved, nil, err
		}
		if observed.BindingDigest != exec.Hash(binding) {
			return record, result, resolved, nil, ErrStale
		}
		catalog = observed.Identity
	}
	scope, err := validationScope(binding, definitions)
	if err != nil {
		return record, result, resolved, nil, err
	}
	plan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: d.Source, Context: d.Context, SQL: d.SQL, Parameters: resolved.Parameters}, scope)
	if err != nil {
		return record, result, resolved, nil, err
	}
	statement, parameters, err := plan.SQL(e, binding)
	if err != nil || statement != d.SQL || parameterDigest(parameters) != parameterDigest(resolved.Parameters) {
		return record, result, resolved, nil, ErrStale
	}
	receipt := plan.Receipt()
	dependencies, err := deriveDependencies(binding, scope, receipt.Dependencies)
	if err != nil {
		return record, result, resolved, nil, err
	}
	operation, err := newID()
	if err != nil {
		return record, result, resolved, nil, err
	}
	report, err := s.executor.Execute(ctx, e, plan, exec.Options{Operation: operation, Number: 1, Preview: true, Rows: s.limits.PreviewRows, Bytes: s.limits.PreviewBytes})
	if err != nil {
		return record, result, resolved, nil, err
	}
	attempt := report.Attempt
	if !successful(attempt.Status) || report.Result == nil || attempt.Finished == nil || attempt.RemoteState != "stopped" || attempt.Manifest.Operation != operation || attempt.Manifest.Session != e.Session() || attempt.Manifest.Receipt.Manifest != receipt.Manifest || !attempt.Manifest.Preview {
		return record, result, resolved, nil, ErrStale
	}
	if err := checkResult(ctx, d, *report.Result, s.limits); err != nil {
		return record, result, resolved, nil, err
	}
	if err := ctx.Err(); err != nil {
		return record, result, resolved, nil, err
	}
	evidenceID, err := newID()
	if err != nil {
		return record, result, resolved, nil, err
	}
	now := time.Now().UTC()
	record = ValidationRecord{Evidence: Evidence{ID: evidenceID, Revision: snapshot.Revision.Number, RevisionID: snapshot.Revision.ID, DefinitionDigest: snapshot.Revision.Digest, ExecutionDigest: snapshot.Revision.ExecutionDigest, ParameterDigest: parameterDigest(resolved.Parameters), DependencyDigest: DependencyDigest(dependencies, d.Topics), SchemaDigest: digest(report.Result.Schema), CanonicalizationVersion: CanonicalizationVersion, ValidatorVersion: binding.Contract, ValidationManifest: receipt.Manifest, Schema: clone(report.Result.Schema), Attempt: clone(attempt), Actor: e.User(), CreatedAt: now, ExpiresAt: now.Add(time.Duration(s.limits.EvidenceTTL))}, Dependencies: dependencies, BindingDigest: exec.Hash(binding), Topics: clone(d.Topics), Binding: binding.Clone(), Definitions: []topics.Definition{}}
	record.Catalog = clone(catalog)
	record.Evidence.ResolvedAt = resolved.At
	record.Evidence.Timezone = resolved.Timezone
	record.Evidence.Parameters = clone(resolved.Values)
	for _, publication := range definitions {
		record.Definitions = append(record.Definitions, clone(publication.Definition))
	}
	return record, clone(*report.Result), resolved, refs, nil
}

// Validate performs one explicitly requested bounded read, records its real
// attempt and observed schema, and discards result rows from block persistence.
func (s *Service) Validate(ctx context.Context, e identity.Envelope, id string, in ValidateRequest) (ValidationResult, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Validate)
	if err != nil {
		return ValidationResult{}, err
	}
	defer cancel()
	snapshot, err := s.repo.ReadBlock(ctx, e, id, revisionReference(in.Revision), Validate)
	if err != nil {
		return ValidationResult{}, err
	}
	record, _, _, refs, err := s.validateWork(ctx, e, id, snapshot, in)
	if err != nil {
		return ValidationResult{}, err
	}
	state, err := s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "validate", ExpectedVersion: in.ExpectedVersion, TargetRevision: snapshot.Revision.Number, TargetDigest: snapshot.Revision.Digest, References: refs, Validation: &record, Watch: record.Dependencies, Topics: record.Topics, CheckCurrent: true})
	if err != nil {
		return ValidationResult{}, err
	}
	return ValidationResult{State: publicState(state, snapshot.PublishedAt == nil), Evidence: clone(record.Evidence)}, nil
}

// Preview never creates a published run or retained artifact. The final metadata
// fence discards values if the selected definition changed during the read.
func (s *Service) Preview(ctx context.Context, e identity.Envelope, id string, in PreviewRequest) (PreviewResult, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Preview)
	if err != nil {
		return PreviewResult{}, err
	}
	defer cancel()
	snapshot, err := s.repo.ReadBlock(ctx, e, id, revisionReference(in.Revision), Preview)
	if err != nil {
		return PreviewResult{}, err
	}
	selected, err := SelectOutputs(snapshot.Revision.Definition.Outputs, in.Outputs)
	if err != nil {
		return PreviewResult{}, err
	}
	record, result, resolved, refs, err := s.validateWork(ctx, e, id, snapshot, in.ValidateRequest)
	if err != nil {
		return PreviewResult{}, err
	}
	_, err = s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "preview", ExpectedVersion: in.ExpectedVersion, TargetRevision: snapshot.Revision.Number, TargetDigest: snapshot.Revision.Digest, References: refs, Validation: &record, Watch: record.Dependencies, Topics: record.Topics, CheckCurrent: true})
	if err != nil {
		return PreviewResult{}, err
	}
	return PreviewResult{ID: id, Revision: snapshot.Revision.Number, Digest: snapshot.Revision.Digest, Private: true, Outputs: selected, Result: result, Attempt: clone(record.Evidence.Attempt), Resolved: clone(resolved.Values), NarrativesGenerated: false}, nil
}

func freshValidation(snapshot Snapshot, evidenceID string, now time.Time) error {
	r := snapshot.Revision
	v := snapshot.Validation
	if v == nil || v.Evidence.ID != evidenceID || !hashValid(v.Evidence.DefinitionDigest) || v.Evidence.DefinitionDigest != r.Digest || v.Evidence.ExecutionDigest != r.ExecutionDigest || v.Evidence.Revision != r.Number || v.Evidence.RevisionID != r.ID || v.Evidence.CanonicalizationVersion != CanonicalizationVersion || v.Evidence.DependencyDigest != DependencyDigest(v.Dependencies, r.Definition.Topics) || !now.Before(v.Evidence.ExpiresAt) || !snapshot.Current || !successful(v.Evidence.Attempt.Status) {
		return ErrStale
	}
	return nil
}
