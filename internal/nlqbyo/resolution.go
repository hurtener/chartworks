package nlqbyo

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

func admit(ctx context.Context, e identity.Envelope, action string) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	if !e.Has(action) {
		return access.ErrForbidden
	}
	return nil
}

func (s *Service) load(ctx context.Context, e identity.Envelope, in Reference, action string) (Record, store.Scope, error) {
	var zero Record
	var scope store.Scope
	if err := admit(ctx, e, action); err != nil {
		return zero, scope, err
	}
	if in.SchemaVersion != Version {
		return zero, scope, ErrInvalid
	}
	if !ReferenceValid(in) {
		return zero, scope, ErrReplan
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return zero, scope, err
	}
	record, err := s.repo.ReadBYOBundle(ctx, scope, in, e.Session(), s.now())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			err = ErrReplan
		}
		return zero, scope, err
	}
	if record.DataReach != dataReach(e) || !RecordValid(record, scope, s.limits) || !s.now().Before(record.Bundle.ExpiresAt) || record.Session != e.Session() || record.Bundle.Reference != in {
		return zero, scope, ErrReplan
	}
	// Complete dependency reach is rechecked before any current-source lookup.
	resources := []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: record.Bundle.Source}, {Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: in.Context}}
	ids := make([]string, len(record.Bundle.Semantics))
	for i, pin := range record.Bundle.Semantics {
		ids[i] = pin.Topic
		resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: pin.Topic})
	}
	for _, r := range record.Bundle.Requirements.Relations {
		resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: r.ID})
	}
	if err = access.Require(e, action, resources...); err != nil {
		return zero, scope, ErrReplan
	}
	if action == "query.submit" {
		// Context-only authority must not even reach native discovery/planning.
		if err = exec.Require(e, record.Binding, datasetIDs(record.Bundle.Requirements.Relations)); err != nil {
			return zero, scope, err
		}
	}
	pins, _, err := s.current(ctx, e, ids)
	if err != nil {
		return zero, scope, replanError(err)
	}
	if exec.Hash(pins) != exec.Hash(record.Bundle.Semantics) {
		return zero, scope, ErrReplan
	}
	binding, err := s.sources.ContextBinding(ctx, e, record.Bundle.Source, in.Context)
	if err != nil {
		return zero, scope, replanError(err)
	}
	if exec.Hash(binding) != exec.Hash(record.Binding) {
		return zero, scope, ErrReplan
	}
	r := record.Bundle.Requirements
	if r.MaxSQLBytes > s.read.MaxSQLBytes || r.MaxParameters > s.read.MaxParameters || r.Rows > s.read.RowsCeiling || r.Bytes > s.read.BytesCeiling || r.TimeoutMillis > min(time.Duration(s.limits.StepTimeout), time.Duration(s.read.Timeout)).Milliseconds() {
		return zero, scope, ErrReplan
	}
	return record, scope, nil
}

func (s *Service) current(ctx context.Context, e identity.Envelope, ids []string) ([]SemanticPin, []topics.Definition, error) {
	if len(ids) == 0 || len(ids) > 4 {
		return nil, nil, ErrReplan
	}
	pins := make([]SemanticPin, 0, len(ids))
	definitions := make([]topics.Definition, 0, len(ids))
	for _, id := range ids {
		contract, err := s.topics.Contract(ctx, e, id)
		if err != nil {
			return nil, nil, err
		}
		p := contract.Publication
		pin := SemanticPin{Topic: id, Version: p.State.Version, Digest: p.Digest}
		if !p.State.Active || p.State.Archived || p.Definition.Topic != id || p.Definition.Version != pin.Version || !topics.DigestValid(pin.Digest) {
			return nil, nil, ErrReplan
		}
		rules, err := s.rules.Read(ctx, e, id, "")
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, nil, err
		}
		if err == nil {
			if !rules.State.Active || rules.Definition.Topic != id || rules.Definition.TopicVersion != pin.Version || rules.Definition.PackDigest != pin.Digest {
				return nil, nil, ErrReplan
			}
			pin.RuleVersion, pin.RuleDigest = rules.State.Version, rules.Digest
		}
		pins = append(pins, pin)
		definitions = append(definitions, p.Definition)
	}
	return pins, definitions, nil
}

func semanticRelations(binding exec.Binding, definitions []topics.Definition) ([]exec.Relation, error) {
	if !binding.Valid() {
		return nil, ErrReplan
	}
	allowed := map[string]map[string]bool{}
	for _, definition := range definitions {
		for _, dataset := range definition.Datasets {
			if dataset.Source.Source != binding.Source || dataset.Source.Context != binding.Context || dataset.Source.SourceRevision != binding.Revision {
				return nil, ErrReplan
			}
			if allowed[dataset.ID] == nil {
				allowed[dataset.ID] = map[string]bool{}
			}
			for _, column := range dataset.Columns {
				allowed[dataset.ID][column.SourceName] = true
			}
		}
	}
	out := make([]exec.Relation, 0, len(allowed))
	for _, relation := range binding.Relations {
		columns, ok := allowed[relation.ID]
		if !ok {
			continue
		}
		r := relation
		r.Columns = nil
		for _, c := range relation.Columns {
			if columns[c.Name] && c.Safe {
				r.Columns = append(r.Columns, c)
			}
		}
		if len(r.Columns) != len(columns) || len(r.Columns) == 0 {
			return nil, ErrReplan
		}
		sort.Slice(r.Columns, func(i, j int) bool { return r.Columns[i].Name < r.Columns[j].Name })
		out = append(out, r)
	}
	if len(out) == 0 || len(out) != len(allowed) {
		return nil, ErrReplan
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func relationScope(relations []exec.Relation) []exec.RelationScope {
	out := make([]exec.RelationScope, len(relations))
	for i, r := range relations {
		out[i].Dataset = r.ID
		for _, c := range r.Columns {
			out[i].Columns = append(out[i].Columns, c.Name)
		}
	}
	return out
}

func datasetIDs(relations []exec.Relation) []string {
	out := make([]string, len(relations))
	for i, r := range relations {
		out[i] = r.ID
	}
	return out
}
