package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/hurtener/chartworks/internal/identity"
)

// PortableColumn has a logical slot and semantic expectations, but no physical
// column name, native dialect type, source coordinate, or profile provenance.
type PortableColumn struct {
	Slot     string `json:"slot"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Nullable bool   `json:"nullable"`
}

type PortableDataset struct {
	Slot    string           `json:"slot"`
	Name    string           `json:"name"`
	Columns []PortableColumn `json:"columns"`
}

// PortablePack is a structural allowlist of transferable semantic authoring data.
// Dataset/column references use explicit logical slots. No installation topic ID,
// source/context/profile identifiers, physical column names, credentials, actor,
// session, lifecycle state, or authority fields are representable here. Free text
// remains authoring content; this projection is not a natural-language sanitizer.
type PortablePack struct {
	SchemaVersion     int               `json:"schema_version"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Datasets          []PortableDataset `json:"datasets"`
	Measures          []Measure         `json:"measures"`
	Dimensions        []Dimension       `json:"dimensions"`
	KPIs              []KPI             `json:"kpis"`
	Joins             []Join            `json:"joins"`
	CanonicalEntities []CanonicalEntity `json:"canonical_entities"`
}

type ExportColumnSlot struct {
	Column string
	Slot   string
}

// ExportDatasetSlots explicitly separates current stable coordinates from the
// chosen logical slots. Export never guesses a mapping from display/source names.
type ExportDatasetSlots struct {
	Dataset string
	Slot    string
	Columns []ExportColumnSlot
}

// ExportPortable projects an already compiled model through a complete mapping.
// The result is detached and ordered, but export permission belongs to the service.
func ExportPortable(model Model, mapping []ExportDatasetSlots) (PortablePack, error) {
	if model.Digest() == "" || len(mapping) != len(model.pack.Datasets) {
		return PortablePack{}, invalid(CodeEvidenceMismatch, "export.mapping")
	}
	datasets := map[string]ExportDatasetSlots{}
	slots := map[string]bool{}
	refs := map[Reference]Reference{}
	for _, binding := range mapping {
		if !identity.Identifier(binding.Dataset) || !identity.Identifier(binding.Slot) || len(binding.Columns) < 1 || len(binding.Columns) > 256 {
			return PortablePack{}, invalid(CodeInvalidValue, "export.mapping")
		}
		if _, exists := datasets[binding.Dataset]; exists || slots[binding.Slot] {
			return PortablePack{}, invalid(CodeDuplicateID, "export.mapping")
		}
		datasets[binding.Dataset], slots[binding.Slot] = binding, true
		columnSlots := map[string]bool{}
		for _, column := range binding.Columns {
			from := Reference{Kind: KindColumn, Dataset: binding.Dataset, ID: column.Column}
			if !model.Contains(from) || !identity.Identifier(column.Slot) {
				return PortablePack{}, invalid(CodeInvalidReference, "export.mapping.columns")
			}
			if _, exists := refs[from]; exists || columnSlots[column.Slot] {
				return PortablePack{}, invalid(CodeDuplicateID, "export.mapping.columns")
			}
			refs[from] = Reference{Kind: KindColumn, Dataset: binding.Slot, ID: column.Slot}
			columnSlots[column.Slot] = true
		}
	}
	p := model.Pack()
	out := PortablePack{SchemaVersion: p.SchemaVersion, Name: p.Name, Description: p.Description}
	for _, dataset := range p.Datasets {
		binding, exists := datasets[dataset.ID]
		if !exists || len(binding.Columns) != len(dataset.Columns) {
			return PortablePack{}, invalid(CodeMissingReference, "export.mapping")
		}
		portable := PortableDataset{Slot: binding.Slot, Name: dataset.Name}
		for _, column := range dataset.Columns {
			ref, exists := refs[Reference{Kind: KindColumn, Dataset: dataset.ID, ID: column.ID}]
			if !exists {
				return PortablePack{}, invalid(CodeMissingReference, "export.mapping.columns")
			}
			portable.Columns = append(portable.Columns, PortableColumn{Slot: ref.ID, Name: column.Name, Category: column.Category, Nullable: column.Nullable})
		}
		sort.Slice(portable.Columns, func(i, j int) bool { return portable.Columns[i].Slot < portable.Columns[j].Slot })
		out.Datasets = append(out.Datasets, portable)
	}
	if err := remapColumns(&p, refs); err != nil {
		return PortablePack{}, err
	}
	out.Measures, out.Dimensions, out.KPIs, out.Joins, out.CanonicalEntities = p.Measures, p.Dimensions, p.KPIs, p.Joins, p.CanonicalEntities
	sort.Slice(out.Datasets, func(i, j int) bool { return out.Datasets[i].Slot < out.Datasets[j].Slot })
	if _, err := portableDigest(out); err != nil {
		return PortablePack{}, err
	}
	return out, nil
}

// ImportColumnBinding is caller-supplied destination evidence. Its shape is not
// proof that a source/profile was consulted or that current authority permits it.
type ImportColumnBinding struct {
	Slot       string
	ID         string
	SourceName string
	NativeType string
	Category   string
	Nullable   bool
}

type ImportDatasetBinding struct {
	Slot    string
	Source  SourceReference
	Columns []ImportColumnBinding
}

type DraftBindings struct {
	Topic    string
	Version  string
	Datasets []ImportDatasetBinding
}

// DraftCandidate is structurally compiled untrusted authoring data. It carries
// no publication stage, approved registry revision, or verified-binding proof.
// A service must revalidate every supplied binding and exact canonical revision
// against current source/profile health and Pengui authority before persistence.
type DraftCandidate struct {
	model Model
}

func (d DraftCandidate) Pack() TopicPack { return d.model.Pack() }
func (d DraftCandidate) Digest() string  { return d.model.Digest() }

// ImportDraftCandidate binds explicit logical slots and runs the same semantic
// compiler. It does not import into storage, approve revisions, or execute work.
func ImportDraftCandidate(input PortablePack, bindings DraftBindings) (DraftCandidate, error) {
	if err := portableBounds(input); err != nil {
		return DraftCandidate{}, err
	}
	if !identity.Identifier(bindings.Topic) || !identity.Identifier(bindings.Version) || len(bindings.Datasets) != len(input.Datasets) {
		return DraftCandidate{}, invalid(CodeInvalidValue, "import.bindings")
	}
	bySlot := map[string]ImportDatasetBinding{}
	for _, binding := range bindings.Datasets {
		if !identity.Identifier(binding.Slot) || !sourceReferenceValid(binding.Source) || len(binding.Columns) < 1 || len(binding.Columns) > 256 {
			return DraftCandidate{}, invalid(CodeInvalidValue, "import.bindings")
		}
		if _, exists := bySlot[binding.Slot]; exists {
			return DraftCandidate{}, invalid(CodeDuplicateID, "import.bindings")
		}
		bySlot[binding.Slot] = binding
	}
	p := TopicPack{SchemaVersion: input.SchemaVersion, Topic: bindings.Topic, Version: bindings.Version, Name: input.Name, Description: input.Description}
	refs := map[Reference]Reference{}
	seen := map[string]bool{}
	for _, dataset := range input.Datasets {
		if !identity.Identifier(dataset.Slot) || seen[dataset.Slot] {
			return DraftCandidate{}, invalid(CodeInvalidValue, "import.datasets")
		}
		seen[dataset.Slot] = true
		binding, exists := bySlot[dataset.Slot]
		if !exists || len(binding.Columns) != len(dataset.Columns) {
			return DraftCandidate{}, invalid(CodeMissingReference, "import.bindings")
		}
		columns := map[string]ImportColumnBinding{}
		for _, column := range binding.Columns {
			if !identity.Identifier(column.Slot) || !identity.Identifier(column.ID) {
				return DraftCandidate{}, invalid(CodeInvalidValue, "import.bindings.columns")
			}
			if _, exists := columns[column.Slot]; exists {
				return DraftCandidate{}, invalid(CodeDuplicateID, "import.bindings.columns")
			}
			columns[column.Slot] = column
		}
		bound := Dataset{ID: binding.Source.Dataset, Name: dataset.Name, Source: binding.Source}
		for _, column := range dataset.Columns {
			from := Reference{Kind: KindColumn, Dataset: dataset.Slot, ID: column.Slot}
			if !from.Valid() {
				return DraftCandidate{}, invalid(CodeInvalidReference, "import.columns")
			}
			if _, exists := refs[from]; exists {
				return DraftCandidate{}, invalid(CodeDuplicateID, "import.columns")
			}
			physical, exists := columns[column.Slot]
			if !exists {
				return DraftCandidate{}, invalid(CodeMissingReference, "import.bindings.columns")
			}
			if physical.Category != column.Category || physical.Nullable != column.Nullable {
				return DraftCandidate{}, invalid(CodeEvidenceMismatch, "import.bindings.columns")
			}
			refs[from] = Reference{Kind: KindColumn, Dataset: bound.ID, ID: physical.ID}
			bound.Columns = append(bound.Columns, Column{ID: physical.ID, SourceName: physical.SourceName, Name: column.Name, NativeType: physical.NativeType, Category: physical.Category, Nullable: physical.Nullable})
		}
		p.Datasets = append(p.Datasets, bound)
	}
	// Clone the semantic collections before rewriting any supplied reference.
	semantic := clonePack(TopicPack{Measures: input.Measures, Dimensions: input.Dimensions, KPIs: input.KPIs, Joins: input.Joins, CanonicalEntities: input.CanonicalEntities})
	if err := remapColumns(&semantic, refs); err != nil {
		return DraftCandidate{}, err
	}
	if _, err := portableDigest(input); err != nil {
		return DraftCandidate{}, err
	}
	p.Measures, p.Dimensions, p.KPIs, p.Joins, p.CanonicalEntities = semantic.Measures, semantic.Dimensions, semantic.KPIs, semantic.Joins, semantic.CanonicalEntities
	model, err := Compile(p)
	if err != nil {
		return DraftCandidate{}, err
	}
	return DraftCandidate{model: model}, nil
}

func portableBounds(p PortablePack) error {
	if p.SchemaVersion != SchemaVersion || !validLine(p.Name, 256) || !validText(p.Description, 4096) {
		return invalid(CodeInvalidValue, "portable")
	}
	if len(p.Datasets) < 1 || len(p.Datasets) > 32 || len(p.Measures) > 1024 || len(p.Dimensions) > 1024 || len(p.KPIs) > 512 || len(p.Joins) > 256 || len(p.CanonicalEntities) > 256 {
		return invalid(CodeLimit, "portable")
	}
	for _, dataset := range p.Datasets {
		if len(dataset.Columns) < 1 || len(dataset.Columns) > 256 {
			return invalid(CodeLimit, "portable.columns")
		}
		if !identity.Identifier(dataset.Slot) || !validLine(dataset.Name, 256) {
			return invalid(CodeInvalidValue, "portable.datasets")
		}
		for _, column := range dataset.Columns {
			if !identity.Identifier(column.Slot) || !validLine(column.Name, 256) || !validLine(column.Category, 64) {
				return invalid(CodeInvalidValue, "portable.columns")
			}
		}
	}
	for _, kpi := range p.KPIs {
		if len(kpi.Inputs) > 32 {
			return invalid(CodeLimit, "portable.kpis")
		}
	}
	for _, entity := range p.CanonicalEntities {
		if len(entity.Keys) > 32 || len(entity.Aliases) > 32 {
			return invalid(CodeLimit, "portable.canonical_entities")
		}
	}
	return validateEntities(TopicPack{Measures: p.Measures, Dimensions: p.Dimensions, KPIs: p.KPIs, Joins: p.Joins, CanonicalEntities: p.CanonicalEntities})
}

func portableDigest(p PortablePack) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", invalid(CodeInvalidValue, "portable")
	}
	if len(raw) > maximumPackBytes {
		return "", invalid(CodeLimit, "portable")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func remapColumns(p *TopicPack, refs map[Reference]Reference) error {
	remap := func(ref *Reference) error {
		if !ref.Valid() {
			return invalid(CodeInvalidReference, "portable.references")
		}
		if ref.Kind != KindColumn {
			return nil
		}
		mapped, exists := refs[*ref]
		if !exists {
			return invalid(CodeMissingReference, "portable.references")
		}
		*ref = mapped
		return nil
	}
	for i := range p.Measures {
		if err := remap(&p.Measures[i].Field); err != nil {
			return err
		}
	}
	for i := range p.Dimensions {
		if err := remap(&p.Dimensions[i].Field); err != nil {
			return err
		}
	}
	for i := range p.KPIs {
		for j := range p.KPIs[i].Inputs {
			if err := remap(&p.KPIs[i].Inputs[j]); err != nil {
				return err
			}
		}
	}
	for i := range p.Joins {
		if err := remap(&p.Joins[i].Left); err != nil {
			return err
		}
		if err := remap(&p.Joins[i].Right); err != nil {
			return err
		}
	}
	for i := range p.CanonicalEntities {
		for j := range p.CanonicalEntities[i].Keys {
			if err := remap(&p.CanonicalEntities[i].Keys[j]); err != nil {
				return err
			}
		}
	}
	return nil
}
