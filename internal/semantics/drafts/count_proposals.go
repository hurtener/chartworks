package drafts

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
)

// CountProposal proposes COUNT(column), never COUNT DISTINCT or an inferred row
// identity. The existing column outcome remains intact; explicit review decides
// whether counting that non-NULL field represents the requested business count.
type CountProposal struct {
	Dataset     string                     `json:"dataset"`
	Column      string                     `json:"column"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Aliases     []string                   `json:"aliases"`
	Unit        string                     `json:"unit"`
	Filters     []semantics.SemanticFilter `json:"-"`
}

// GeneratedCountMeasureID is the stable supplemental COUNT(column) identity.
// Aggregation semantics are part of the identity; callers cannot pick an ID or
// silently reinterpret an existing SUM/dimension as a count.
func GeneratedCountMeasureID(dataset, column string) string {
	if !(semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: column}).Valid() {
		return ""
	}
	digest := sha256.Sum256([]byte("count-column-v1\x00" + dataset + "\x00" + column))
	return "count_" + hex.EncodeToString(digest[:12])
}

func validateCountProposals(selected []semantics.Reference, proposals []CountProposal) error {
	if len(proposals) > 8 {
		return gateway.ErrOutput
	}
	seen := map[string]bool{}
	for _, proposal := range proposals {
		field := semantics.Reference{Kind: semantics.KindColumn, Dataset: proposal.Dataset, ID: proposal.Column}
		id := GeneratedCountMeasureID(proposal.Dataset, proposal.Column)
		if !wantReference(selected, field) || id == "" || seen[id] {
			return gateway.ErrOutput
		}
		seen[id] = true
	}
	return nil
}

func applyCountProposals(model semantics.Model, proposals []CountProposal) (semantics.Model, error) {
	if len(proposals) == 0 {
		return model, nil
	}
	pack := model.Pack()
	for _, proposal := range proposals {
		field := semantics.Reference{Kind: semantics.KindColumn, Dataset: proposal.Dataset, ID: proposal.Column}
		if !model.Contains(field) {
			return semantics.Model{}, gateway.ErrOutput
		}
		value := semantics.Measure{ID: GeneratedCountMeasureID(proposal.Dataset, proposal.Column), Field: field, Aggregation: semantics.AggregationCount, Name: proposal.Name, Description: proposal.Description, Aliases: proposal.Aliases, Unit: proposal.Unit, Filters: proposal.Filters}
		replaced := false
		for i, prior := range pack.Measures {
			if prior.ID == value.ID {
				if prior.Field != field || prior.Aggregation != semantics.AggregationCount {
					return semantics.Model{}, gateway.ErrOutput
				}
				var err error
				value.Filters, err = mergeProtectedFilters(prior.Filters, value.Filters)
				if err != nil {
					return semantics.Model{}, err
				}
				pack.Measures[i] = value
				replaced = true
				break
			}
		}
		if !replaced {
			pack.Measures = append(pack.Measures, value)
		}
	}
	return semantics.Compile(pack)
}

func addCountProposalSchema(properties map[string]any) {
	id := map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
	properties["count_proposals"] = map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"dataset", "column", "name", "description", "aliases", "unit"}, "properties": map[string]any{"dataset": id, "column": id, "name": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}, "description": map[string]any{"type": "string", "maxLength": 4096}, "aliases": map[string]any{"type": "array", "maxItems": 16, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 256}}, "unit": map[string]any{"type": "string", "maxLength": 64}}}}
}
