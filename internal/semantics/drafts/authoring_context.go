package drafts

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/sources"
)

// Mandatory authoring material is never silently truncated to fit a prompt.
const maxAuthoringContextBytes = 128 << 10
const maxAuthoringColumns = 1024

type authoringAggregate struct {
	Column     string         `json:"column"`
	Observed   int            `json:"observed"`
	Nulls      int            `json:"nulls"`
	Distinct   int            `json:"sample_distinct"`
	Exact      bool           `json:"distinct_exact"`
	Families   map[string]int `json:"families,omitempty"`
	Disclosure string         `json:"disclosure"`
}
type authoringRelation struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

type authoringEvidence struct {
	Relation       *authoringRelation        `json:"relation,omitempty"`
	MissingColumns []string                  `json:"missing_columns,omitempty"`
	Origin         semantics.SourceReference `json:"origin"`
	ObservedAt     time.Time                 `json:"observed_at"`
	PolicyDigest   string                    `json:"policy_digest"`
	Sampling       engineering.Sampling      `json:"sampling"`
	Columns        []authoringAggregate      `json:"columns"`
}
type authoringRedaction struct {
	Entity  string `json:"entity"`
	Filters int    `json:"filter_literals_omitted"`
	Values  int    `json:"governed_values_omitted"`
}
type authoringContext struct {
	Vocabulary      []AuthoringValue      `json:"authoring_vocabulary,omitempty"`
	CandidateDigest string                `json:"candidate_digest"`
	Digest          string                `json:"digest"`
	Candidate       semantics.TopicPack   `json:"candidate"`
	Evidence        []authoringEvidence   `json:"profile_evidence"`
	Redactions      []authoringRedaction  `json:"redactions"`
	Relationships   []semantics.Reference `json:"allowed_relationship_columns"`
}

func authoringResources(e identity.Envelope, p semantics.TopicPack) []access.Resource {
	out := []access.Resource{{Tenant: e.Tenant(), Kind: "topic", Permission: "write", ID: p.Topic}}
	for _, d := range p.Datasets {
		out = append(out, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: d.Source.Source}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: d.ID}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: d.Source.Context})
	}
	return out
}

// authoringContext checks the complete dependency reach before reading any
// profile. Profiles are immutable evidence, not proof of uniqueness or meaning.
func (s *Service) authoringContext(ctx context.Context, e identity.Envelope, model semantics.Model, vocabulary ...[]AuthoringValue) (authoringContext, error) {
	p := model.Pack()
	if err := RequirePack(e, p, Write); err != nil {
		return authoringContext{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	profiles := make(map[string]engineering.ProfileEvidence, len(p.Datasets))
	catalogs := map[string]sources.Discovery{}
	relations := map[string]authoringRelation{}
	for _, d := range p.Datasets {
		evidence, err := s.profiles.Evidence(ctx, e, d.Source.ProfileVersion)
		if err != nil {
			return authoringContext{}, err
		}
		catalog, found := catalogs[d.Source.Source]
		if !found {
			catalog, err = s.sources.Discover(ctx, e, d.Source.Source)
			if err != nil {
				return authoringContext{}, err
			}
			catalogs[d.Source.Source] = catalog
		}
		relation, matchErr := matchAuthoringRelation(d, evidence.Profile, catalog)
		if matchErr != nil {
			return authoringContext{}, matchErr
		}
		relations[d.ID] = relation
		profiles[d.ID] = evidence
	}
	out, err := buildAuthoringContext(model, profiles, vocabulary...)
	if err != nil {
		return authoringContext{}, err
	}
	for i := range out.Evidence {
		relation := relations[out.Evidence[i].Origin.Dataset]
		out.Evidence[i].Relation = &relation
	}
	return sealAuthoringContext(out)
}

func buildAuthoringContext(model semantics.Model, profiles map[string]engineering.ProfileEvidence, vocabulary ...[]AuthoringValue) (authoringContext, error) {
	p := model.Pack()
	out := authoringContext{CandidateDigest: model.Digest(), Candidate: p, Redactions: []authoringRedaction{}, Relationships: semantics.GenerationColumns(model)}
	if len(vocabulary) > 0 {
		var err error
		out.Vocabulary, err = AdmitAuthoringVocabulary(model, vocabulary[0])
		if err != nil {
			return authoringContext{}, err
		}
	}
	if len(out.Relationships) > maxAuthoringColumns {
		return authoringContext{}, gateway.ErrBudget
	}
	for _, d := range p.Datasets {
		evidence, ok := profiles[d.ID]
		pr := evidence.Profile
		if !ok || !evidence.Active || pr.Version != d.Source.ProfileVersion || pr.Source != d.Source.Source || pr.Context != d.Source.Context || pr.Dataset != d.ID || pr.SourceRevision != d.Source.SourceRevision || pr.DeterministicHash() != d.Source.ProfileDigest {
			return authoringContext{}, readexec.ErrBinding
		}
		item := authoringEvidence{Origin: d.Source, ObservedAt: pr.ObservedAt, PolicyDigest: pr.PolicyHash, Sampling: pr.Sampling, Columns: []authoringAggregate{}}
		actual := map[string]readexec.Column{}
		for _, c := range pr.Schema {
			actual[c.Name] = c
		}
		stats := map[string]engineering.ColumnProfile{}
		for _, c := range pr.Columns {
			stats[c.Name] = c
		}
		for _, c := range d.Columns {
			schema, ok := actual[c.SourceName]
			if !ok || !schema.Safe || schema.NativeType != c.NativeType || schema.Category != c.Category || schema.Nullable != c.Nullable {
				return authoringContext{}, readexec.ErrBinding
			}
			st, ok := stats[c.SourceName]
			if !ok {
				item.MissingColumns = append(item.MissingColumns, c.ID)
				continue
			}
			a := authoringAggregate{Column: c.ID, Observed: st.Observed, Nulls: st.Nulls, Distinct: st.Distinct, Exact: st.DistinctExact, Disclosure: "aggregate_counts_only"}
			if c.Sensitivity == semantics.LiteralNonSensitive {
				a.Disclosure = "aggregate_counts_and_type_families"
				a.Families = map[string]int{}
				for family, count := range st.Families {
					switch family {
					case "null", "integer", "decimal", "number", "boolean", "binary", "temporal", "json", "text", "empty_text", "whitespace_text", "email_like", "uuid_like", "date_like":
						a.Families[family] = count
					default:
						return authoringContext{}, readexec.ErrBinding
					}
				}
			}
			// Minimum, maximum, freshness values, summary prose and raw rows are
			// structurally absent even when a profile's own policy permits them.
			item.Columns = append(item.Columns, a)
		}
		out.Evidence = append(out.Evidence, item)
	}
	redactAuthoringCandidate(&out)
	sort.Slice(out.Relationships, func(i, j int) bool {
		a, b := out.Relationships[i], out.Relationships[j]
		if a.Dataset != b.Dataset {
			return a.Dataset < b.Dataset
		}
		return a.ID < b.ID
	})
	return sealAuthoringContext(out)
}

func sealAuthoringContext(out authoringContext) (authoringContext, error) {
	out.Digest = ""
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > maxAuthoringContextBytes {
		return authoringContext{}, gateway.ErrBudget
	}
	out.Digest = readexec.Hash(out)
	return out, nil
}

func matchAuthoringRelation(dataset semantics.Dataset, profile engineering.Profile, catalog sources.Discovery) (authoringRelation, error) {
	if catalog.ContextID != dataset.Source.Context || catalog.Revision != dataset.Source.SourceRevision {
		return authoringRelation{}, readexec.ErrBinding
	}
	for _, relation := range catalog.Relations {
		if relation.ID == dataset.ID && relation.Schema != "" && relation.Name != "" && reflect.DeepEqual(relation.Columns, profile.Schema) {
			return authoringRelation{Schema: relation.Schema, Name: relation.Name}, nil
		}
	}
	return authoringRelation{}, readexec.ErrBinding
}

func redactAuthoringCandidate(out *authoringContext) {
	p := &out.Candidate
	allowed := func(field semantics.Reference, value string) bool {
		for _, v := range out.Vocabulary {
			if v.Field == field && v.Value == value {
				return true
			}
		}
		return false
	}
	redactFilters := func(filters []semantics.SemanticFilter) int {
		count := 0
		for i := range filters {
			complete := true
			for _, value := range filters[i].Values {
				complete = complete && allowed(filters[i].Field, value)
			}
			if !complete {
				count += len(filters[i].Values)
				filters[i].Values = nil
			}
		}
		return count
	}
	for i := range p.Measures {
		v := &p.Measures[i]
		if count := redactFilters(v.Filters); count > 0 {
			out.Redactions = append(out.Redactions, authoringRedaction{Entity: "measure:" + v.ID, Filters: count})
		}
	}
	for i := range p.Dimensions {
		v := &p.Dimensions[i]
		count := redactFilters(v.Filters)
		retained := []semantics.GovernedValue{}
		omitted := 0
		for _, value := range v.Values {
			permitted := false
			for _, entry := range out.Vocabulary {
				if entry.Field == v.Field && entry.ID == value.ID && entry.Value == value.Value && value.Sensitivity == semantics.LiteralNonSensitive && reflect.DeepEqual(append([]string{}, entry.Aliases...), append([]string{}, value.Aliases...)) {
					permitted = true
					break
				}
			}
			if permitted {
				retained = append(retained, value)
			} else {
				omitted++
			}
		}
		if count+omitted > 0 {
			out.Redactions = append(out.Redactions, authoringRedaction{Entity: "dimension:" + v.ID, Filters: count, Values: omitted})
		}
		v.Values = retained
	}
	for i := range p.KPIs {
		v := &p.KPIs[i]
		if count := redactFilters(v.Filters); count > 0 {
			out.Redactions = append(out.Redactions, authoringRedaction{Entity: "kpi:" + v.ID, Filters: count})
		}
	}
}

// Model output cannot erase literals withheld from its input. Changing a
// protected entity's kind requires an explicit entity-edit request instead.
func preserveProtectedEnhancementMeaning(p semantics.TopicPack, proposals []semantics.Enhancement) error {
	for i := range proposals {
		item := &proposals[i]
		ref := semantics.Reference{Kind: semantics.KindColumn, Dataset: item.Dataset, ID: item.Column}
		for _, v := range p.Measures {
			if v.Field == ref && v.ID == semantics.GeneratedEntityID(semantics.EnhancementMeasure, item.Dataset, item.Column) && len(v.Filters) > 0 {
				if item.Kind != semantics.EnhancementMeasure {
					return gateway.ErrOutput
				}
				var err error
				item.Filters, err = mergeProtectedFilters(v.Filters, item.Filters)
				if err != nil {
					return err
				}
			}
		}
		for _, v := range p.Dimensions {
			if v.Field == ref && v.ID == semantics.GeneratedEntityID(semantics.EnhancementDimension, item.Dataset, item.Column) && len(v.Filters)+len(v.Values) > 0 {
				if item.Kind != semantics.EnhancementDimension {
					return gateway.ErrOutput
				}
				var err error
				item.Filters, err = mergeProtectedFilters(v.Filters, item.Filters)
				if err != nil {
					return err
				}
				item.Values, err = mergeProtectedValues(v.Values, item.Values)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// The model cannot invent an evidence origin. Existing proposals keep their
// original origin on exact replay; newly proposed relationships bind this packet.
func sealRelationshipProvenance(p semantics.TopicPack, proposed []semantics.RelationshipDecision, digest string) {
	for i := range proposed {
		proposed[i].Evidence.Provenance = "authoring:" + digest
		for _, prior := range p.RelationshipDecisions {
			if prior.ID == proposed[i].ID {
				proposed[i].Evidence.Provenance = prior.Evidence.Provenance
				break
			}
		}
	}
}
