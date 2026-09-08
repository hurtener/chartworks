package semantics

import "github.com/hurtener/chartworks/internal/identity"

// EntityMutation is one closed put or delete in an atomic draft mutation batch.
type EntityMutation struct {
	Operation string           `json:"operation"`
	Kind      Kind             `json:"kind"`
	ID        string           `json:"id"`
	Measure   *Measure         `json:"measure,omitempty"`
	Dimension *Dimension       `json:"dimension,omitempty"`
	KPI       *KPI             `json:"kpi,omitempty"`
	Join      *Join            `json:"join,omitempty"`
	Canonical *CanonicalEntity `json:"canonical,omitempty"`
}

// DatasetReplacement contains server-derived evidence for one logical dataset move.
type DatasetReplacement struct {
	Dataset string          `json:"dataset"`
	Source  SourceReference `json:"source"`
	Columns []Column        `json:"columns"`
}

// MutateEntities applies one atomic CRUD batch to a detached draft model. The
// final compiler validates every reference, so a delete cannot strand a KPI,
// field, join, or canonical key.
func MutateEntities(model Model, version string, mutations []EntityMutation) (Model, error) {
	if model.Digest() == "" || !identity.Identifier(version) || len(mutations) < 1 || len(mutations) > 128 {
		return Model{}, invalid(CodeInvalidValue, "mutations")
	}
	pack := model.Pack()
	pack.Version = version
	seen := map[string]bool{}
	for i, mutation := range mutations {
		path := "mutations[" + itoa(i) + "]"
		if !identity.Identifier(mutation.ID) || seen[string(mutation.Kind)+"\x00"+mutation.ID] {
			return Model{}, invalid(CodeDuplicateID, path)
		}
		seen[string(mutation.Kind)+"\x00"+mutation.ID] = true
		payloads := boolInt(mutation.Measure != nil) + boolInt(mutation.Dimension != nil) + boolInt(mutation.KPI != nil) + boolInt(mutation.Join != nil) + boolInt(mutation.Canonical != nil)
		switch mutation.Operation {
		case "put":
			if payloads != 1 {
				return Model{}, invalid(CodeInvalidValue, path)
			}
		case "delete":
			if payloads != 0 {
				return Model{}, invalid(CodeInvalidValue, path)
			}
		default:
			return Model{}, invalid(CodeInvalidValue, path+".operation")
		}
		if err := applyEntityMutation(&pack, mutation, path); err != nil {
			return Model{}, err
		}
	}
	return Compile(pack)
}

func applyEntityMutation(pack *TopicPack, mutation EntityMutation, path string) error {
	switch mutation.Kind {
	case KindMeasure:
		return mutateByID(&pack.Measures, mutation.ID, mutation.Operation, mutation.Measure, func(v Measure) string { return v.ID }, path)
	case KindDimension:
		return mutateByID(&pack.Dimensions, mutation.ID, mutation.Operation, mutation.Dimension, func(v Dimension) string { return v.ID }, path)
	case KindKPI:
		return mutateByID(&pack.KPIs, mutation.ID, mutation.Operation, mutation.KPI, func(v KPI) string { return v.ID }, path)
	case KindJoin:
		return mutateByID(&pack.Joins, mutation.ID, mutation.Operation, mutation.Join, func(v Join) string { return v.ID }, path)
	case KindCanonicalEntity:
		return mutateByID(&pack.CanonicalEntities, mutation.ID, mutation.Operation, mutation.Canonical, func(v CanonicalEntity) string { return v.ID }, path)
	default:
		return invalid(CodeInvalidValue, path+".kind")
	}
}

func mutateByID[T any](items *[]T, id, operation string, value *T, identify func(T) string, path string) error {
	index := -1
	for i := range *items {
		if identify((*items)[i]) == id {
			index = i
			break
		}
	}
	if operation == "delete" {
		if index < 0 {
			return invalid(CodeMissingReference, path+".id")
		}
		*items = append((*items)[:index], (*items)[index+1:]...)
		return nil
	}
	if value == nil || identify(*value) != id {
		return invalid(CodeInvalidValue, path+".id")
	}
	if index < 0 {
		*items = append(*items, *value)
	} else {
		(*items)[index] = *value
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// ReplaceDataset moves one logical dataset to reviewed source/profile evidence,
// preserving semantic column IDs while rewriting every dataset-qualified
// reference to the target dataset ID.
func ReplaceDataset(model Model, version, oldDataset string, replacement DatasetReplacement) (Model, error) {
	if model.Digest() == "" || !identity.Identifier(version) || !identity.Identifier(oldDataset) || !identity.Identifier(replacement.Dataset) || replacement.Source.Dataset != replacement.Dataset {
		return Model{}, invalid(CodeInvalidValue, "dataset_replacement")
	}
	pack := model.Pack()
	pack.Version = version
	index := -1
	for i := range pack.Datasets {
		if pack.Datasets[i].ID == oldDataset {
			index = i
			break
		}
	}
	if index < 0 {
		return Model{}, invalid(CodeMissingReference, "dataset_replacement.dataset")
	}
	old := pack.Datasets[index]
	if len(replacement.Columns) != len(old.Columns) {
		return Model{}, invalid(CodeInvalidReference, "dataset_replacement.columns")
	}
	want := map[string]bool{}
	for _, column := range old.Columns {
		want[column.ID] = true
	}
	for _, column := range replacement.Columns {
		if !want[column.ID] {
			return Model{}, invalid(CodeInvalidReference, "dataset_replacement.columns")
		}
		delete(want, column.ID)
	}
	if len(want) != 0 {
		return Model{}, invalid(CodeInvalidReference, "dataset_replacement.columns")
	}
	pack.Datasets[index] = Dataset{ID: replacement.Dataset, Name: old.Name, Source: replacement.Source, Columns: append([]Column(nil), replacement.Columns...)}
	rewrite := func(ref *Reference) {
		if ref.Kind == KindColumn && ref.Dataset == oldDataset {
			ref.Dataset = replacement.Dataset
		}
		if ref.Kind == KindDataset && ref.ID == oldDataset {
			ref.ID = replacement.Dataset
		}
	}
	for i := range pack.Measures {
		rewrite(&pack.Measures[i].Field)
	}
	for i := range pack.Dimensions {
		rewrite(&pack.Dimensions[i].Field)
	}
	for i := range pack.KPIs {
		for j := range pack.KPIs[i].Inputs {
			rewrite(&pack.KPIs[i].Inputs[j])
		}
	}
	for i := range pack.Joins {
		rewrite(&pack.Joins[i].Left)
		rewrite(&pack.Joins[i].Right)
	}
	for i := range pack.CanonicalEntities {
		for j := range pack.CanonicalEntities[i].Keys {
			rewrite(&pack.CanonicalEntities[i].Keys[j])
		}
	}
	return Compile(pack)
}
