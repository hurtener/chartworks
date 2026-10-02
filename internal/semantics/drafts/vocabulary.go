package drafts

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

// AuthoringValue is explicit author input, not an observed warehouse sample or
// proof of membership. The exact origin cannot override a sensitive field.
// The first bounded implementation admits text equality/inclusion only.
type AuthoringValue struct {
	ID          string                       `json:"id"`
	Field       semantics.Reference          `json:"field"`
	Origin      semantics.SourceReference    `json:"origin"`
	Kind        string                       `json:"kind" jsonschema:"enum=text"`
	Value       string                       `json:"value"`
	Aliases     []string                     `json:"aliases,omitempty"`
	Sensitivity semantics.LiteralSensitivity `json:"sensitivity" jsonschema:"enum=non_sensitive"`
	Nulls       string                       `json:"nulls" jsonschema:"enum=exclude"`
}

// VocabularyFilterProposal selects approved input IDs; it cannot supply SQL or
// novel literals. It still requires normal explicit topic review/publication.
type VocabularyFilterProposal struct {
	JoinID        string   `json:"join_id,omitempty"`
	Measure       string   `json:"measure"`
	ID            string   `json:"id"`
	Operator      string   `json:"operator"`
	Nulls         string   `json:"nulls"`
	VocabularyIDs []string `json:"vocabulary_ids"`
}
type VocabularyValueProposal struct {
	Dataset       string   `json:"dataset"`
	Column        string   `json:"column"`
	VocabularyIDs []string `json:"vocabulary_ids"`
}

// AdmitAuthoringVocabulary returns a canonical detached catalog against a
// compiled draft. Callers must additionally perform current authority/profile
// checks before disclosure; this function grants neither access nor publication.
func AdmitAuthoringVocabulary(model semantics.Model, input []AuthoringValue) ([]AuthoringValue, error) {
	if model.Digest() == "" || len(input) > 32 {
		return nil, store.ErrInvalid
	}
	out := append([]AuthoringValue{}, input...)
	pack := model.Pack()
	ids := map[string]bool{}
	terms := map[string]bool{}
	validText := func(v string, max int) bool {
		return len(v) > 0 && len(v) <= max && strings.TrimSpace(v) == v && utf8.ValidString(v) && !strings.ContainsAny(v, "\x00\r\n")
	}
	for i := range out {
		v := &out[i]
		if !identity.Identifier(v.ID) || ids[v.ID] || !v.Field.Valid() || v.Field.Kind != semantics.KindColumn || v.Kind != "text" || v.Sensitivity != semantics.LiteralNonSensitive || v.Nulls != "exclude" || !validText(v.Value, 256) || len(v.Aliases) > 8 {
			return nil, store.ErrInvalid
		}
		ids[v.ID] = true
		found := false
		for _, d := range pack.Datasets {
			if d.ID != v.Field.Dataset {
				continue
			}
			if d.Source != v.Origin {
				return nil, readexec.ErrBinding
			}
			for _, c := range d.Columns {
				if c.ID == v.Field.ID {
					if c.Sensitivity != semantics.LiteralNonSensitive || c.Category != "text" && c.Category != "string" {
						return nil, store.ErrInvalid
					}
					found = true
				}
			}
		}
		if !found {
			return nil, readexec.ErrBinding
		}
		v.Aliases = append([]string(nil), v.Aliases...)
		sort.Strings(v.Aliases)
		for _, term := range append([]string{v.Value}, v.Aliases...) {
			if !validText(term, 256) {
				return nil, store.ErrInvalid
			}
			key := readexec.Hash(v.Field) + ":" + strings.ToLower(strings.Join(strings.Fields(term), " "))
			if terms[key] {
				return nil, store.ErrConflict
			}
			terms[key] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > 32<<10 {
		return nil, gateway.ErrBudget
	}
	return out, nil
}

// ResolveAuthoringFilter resolves only IDs in a validated caller-input catalog.
// It produces proposal material, never a publication or source authority proof.
func ResolveAuthoringFilter(model semantics.Model, catalog []AuthoringValue, proposal VocabularyFilterProposal) (semantics.SemanticFilter, error) {
	catalog, err := AdmitAuthoringVocabulary(model, catalog)
	if err != nil {
		return semantics.SemanticFilter{}, err
	}
	if !identity.Identifier(proposal.ID) || proposal.Operator != "eq" && proposal.Operator != "in" || proposal.Nulls != "exclude" || proposal.Operator == "eq" && len(proposal.VocabularyIDs) != 1 {
		return semantics.SemanticFilter{}, gateway.ErrOutput
	}
	values, err := selectAuthoringValues(catalog, proposal.VocabularyIDs)
	if err != nil {
		return semantics.SemanticFilter{}, err
	}
	dataset := ""
	for _, m := range model.Pack().Measures {
		if m.ID == proposal.Measure {
			dataset = m.Field.Dataset
			break
		}
	}
	if dataset == "" {
		for _, field := range semantics.GenerationColumns(model) {
			if semantics.GeneratedEntityID(semantics.EnhancementMeasure, field.Dataset, field.ID) == proposal.Measure || GeneratedCountMeasureID(field.Dataset, field.ID) == proposal.Measure {
				dataset = field.Dataset
				break
			}
		}
	}
	if dataset == "" {
		return semantics.SemanticFilter{}, gateway.ErrOutput
	}
	if values[0].Field.Dataset != dataset {
		if _, ok := semantics.PopulationRelationship(model.Pack(), dataset, values[0].Field.Dataset, proposal.JoinID); !ok {
			return semantics.SemanticFilter{}, gateway.ErrOutput
		}
	} else if proposal.JoinID != "" {
		return semantics.SemanticFilter{}, gateway.ErrOutput
	}
	out := semantics.SemanticFilter{Relationship: proposal.JoinID, ID: proposal.ID, Field: values[0].Field, Operator: proposal.Operator}
	for _, v := range values {
		out.Values = append(out.Values, v.Value)
	}
	return out, nil
}

// ResolveAuthoringValues preserves explicit sensitivity and fixed caller-input
// provenance; it never asserts that a value was observed in warehouse rows.
func ResolveAuthoringValues(model semantics.Model, catalog []AuthoringValue, field semantics.Reference, ids []string) ([]semantics.GovernedValue, error) {
	catalog, err := AdmitAuthoringVocabulary(model, catalog)
	if err != nil {
		return nil, err
	}
	values, err := selectAuthoringValues(catalog, ids)
	if err != nil {
		return nil, err
	}
	if !model.Contains(field) || field != values[0].Field {
		return nil, gateway.ErrOutput
	}
	out := []semantics.GovernedValue{}
	for _, v := range values {
		out = append(out, semantics.GovernedValue{ID: v.ID, Value: v.Value, Aliases: append([]string(nil), v.Aliases...), Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "author_input", Evidence: readexec.Hash(v), Policy: "non_sensitive_text_exclude_null"}})
	}
	return out, nil
}

func selectAuthoringValues(catalog []AuthoringValue, ids []string) ([]AuthoringValue, error) {
	if len(ids) < 1 || len(ids) > 32 {
		return nil, gateway.ErrOutput
	}
	byID := map[string]AuthoringValue{}
	for _, v := range catalog {
		byID[v.ID] = v
	}
	out := []AuthoringValue{}
	seen := map[string]bool{}
	for _, id := range ids {
		v, ok := byID[id]
		if !ok || seen[id] {
			return nil, gateway.ErrOutput
		}
		seen[id] = true
		if len(out) > 0 && out[0].Field != v.Field {
			return nil, gateway.ErrOutput
		}
		out = append(out, v)
	}
	return out, nil
}

func resolveVocabularyProposals(pack semantics.TopicPack, wire *enhancementWire, catalog []AuthoringValue) error {
	if len(wire.FilterProposals) > 32 || len(wire.ValueProposals) > 32 {
		return gateway.ErrOutput
	}
	model, err := semantics.Compile(pack)
	if err != nil {
		return err
	}
	for _, proposal := range wire.FilterProposals {
		filter, err := ResolveAuthoringFilter(model, catalog, proposal)
		if err != nil {
			return err
		}
		matched := false
		for i := range wire.Results {
			r := &wire.Results[i]
			if r.Kind == semantics.EnhancementMeasure && semantics.GeneratedEntityID(r.Kind, r.Dataset, r.Column) == proposal.Measure {
				r.Filters = append(r.Filters, filter)
				matched = true
			}
		}
		for i := range wire.CountProposals {
			r := &wire.CountProposals[i]
			if GeneratedCountMeasureID(r.Dataset, r.Column) == proposal.Measure {
				r.Filters = append(r.Filters, filter)
				matched = true
			}
		}
		if !matched {
			return gateway.ErrOutput
		}
	}
	for _, proposal := range wire.ValueProposals {
		field := semantics.Reference{Kind: semantics.KindColumn, Dataset: proposal.Dataset, ID: proposal.Column}
		values, err := ResolveAuthoringValues(model, catalog, field, proposal.VocabularyIDs)
		if err != nil {
			return err
		}
		matched := false
		for i := range wire.Results {
			r := &wire.Results[i]
			if r.Dataset == proposal.Dataset && r.Column == proposal.Column && r.Kind == semantics.EnhancementDimension && r.Role == semantics.DimensionCategorical {
				r.Values = append(r.Values, values...)
				matched = true
			}
		}
		if !matched {
			return gateway.ErrOutput
		}
	}
	return nil
}

func addVocabularySchemas(properties map[string]any) {
	properties["group_domain"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"policy", "domain"}, "properties": map[string]any{"policy": map[string]any{"const": "metric-group-domain-v1"}, "domain": map[string]any{"enum": []string{"raw_source_groups", "qualifying_population"}}}}
	id := map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
	ids := map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": id}
	properties["filter_proposals"] = map[string]any{"type": "array", "maxItems": 32, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"measure", "id", "operator", "nulls", "vocabulary_ids"}, "properties": map[string]any{"measure": id, "id": id, "join_id": id, "operator": map[string]any{"enum": []string{"eq", "in"}}, "nulls": map[string]any{"const": "exclude"}, "vocabulary_ids": ids}}}
	properties["value_proposals"] = map[string]any{"type": "array", "maxItems": 32, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"dataset", "column", "vocabulary_ids"}, "properties": map[string]any{"dataset": id, "column": id, "vocabulary_ids": ids}}}
}

func mergeProtectedFilters(existing, proposed []semantics.SemanticFilter) ([]semantics.SemanticFilter, error) {
	out := append([]semantics.SemanticFilter(nil), existing...)
	for _, p := range proposed {
		found := false
		for _, v := range existing {
			if p.ID == v.ID {
				if !reflect.DeepEqual(p, v) {
					return nil, gateway.ErrOutput
				}
				found = true
				break
			}
		}
		if !found {
			out = append(out, p)
		}
	}
	return out, nil
}
func mergeProtectedValues(existing, proposed []semantics.GovernedValue) ([]semantics.GovernedValue, error) {
	out := append([]semantics.GovernedValue(nil), existing...)
	for _, p := range proposed {
		found := false
		for _, v := range existing {
			if p.ID == v.ID {
				if p.Value != v.Value || p.Sensitivity != v.Sensitivity || !reflect.DeepEqual(p.Aliases, v.Aliases) {
					return nil, gateway.ErrOutput
				}
				found = true
				break
			}
		}
		if !found {
			out = append(out, p)
		}
	}
	return out, nil
}
