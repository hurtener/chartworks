package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/identity"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	maximumPackBytes          = 1 << 20
	maximumCanonicalTermBytes = 1024
)

// Model is a detached, canonical, immutable-by-API topic definition with lookup indexes.
type Model struct {
	pack   TopicPack
	refs   map[string]struct{}
	terms  map[string]int
	digest string
}

// Compile validates all values and references, rejects ambiguous canonical
// terms and KPI cycles, then returns a deterministic detached representation.
func Compile(input TopicPack) (Model, error) {
	// Reject unbounded slices before allocating copies or sorting caller data.
	if err := validateShape(input); err != nil {
		return Model{}, err
	}
	p := clonePack(input)
	canonicalOrder(&p)
	refs, err := referenceIndex(p)
	if err != nil {
		return Model{}, err
	}
	if err = validateReferences(p, refs); err != nil {
		return Model{}, err
	}
	terms, err := canonicalTerms(p)
	if err != nil {
		return Model{}, err
	}
	if err = kpiCycles(p); err != nil {
		return Model{}, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return Model{}, invalid(CodeInvalidValue, "pack")
	}
	if len(raw) > maximumPackBytes {
		return Model{}, invalid(CodeLimit, "pack")
	}
	sum := sha256.Sum256(raw)
	return Model{pack: p, refs: refs, terms: terms, digest: hex.EncodeToString(sum[:])}, nil
}

// Pack returns a deep copy so compiled state remains safe for concurrent reuse.
func (m Model) Pack() TopicPack { return clonePack(m.pack) }

// Digest is the stable hash of the canonical typed pack.
func (m Model) Digest() string { return m.digest }

// Contains performs exact typed lookup; names and aliases are never reference fallbacks.
func (m Model) Contains(ref Reference) bool {
	if !ref.Valid() || m.refs == nil {
		return false
	}
	_, ok := m.refs[ref.key()]
	return ok
}

// ResolveCanonical resolves a normalized business term to its stable entity.
// It returns a detached definition; unknown terms never become guessed IDs.
func (m Model) ResolveCanonical(term string) (CanonicalEntity, error) {
	key, ok := normalizeTerm(term)
	if !ok || m.terms == nil {
		return CanonicalEntity{}, ErrNotFound
	}
	i, ok := m.terms[key]
	if !ok || i < 0 || i >= len(m.pack.CanonicalEntities) {
		return CanonicalEntity{}, ErrNotFound
	}
	return cloneCanonical(m.pack.CanonicalEntities[i]), nil
}

// CanonicalKeys resolves a term and returns only its exact keys for one dataset.
func (m Model) CanonicalKeys(term, dataset string) ([]Reference, error) {
	if !identity.Identifier(dataset) {
		return nil, ErrNotFound
	}
	entity, err := m.ResolveCanonical(term)
	if err != nil {
		return nil, err
	}
	out := []Reference{}
	for _, key := range entity.Keys {
		if key.Dataset == dataset {
			out = append(out, key)
		}
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

func validateShape(p TopicPack) error {
	if p.SchemaVersion != SchemaVersion || !identity.Identifier(p.Topic) || !identity.Identifier(p.Version) || !validLine(p.Name, 256) || !validText(p.Description, 4096) {
		return invalid(CodeInvalidValue, "pack")
	}
	if len(p.Datasets) < 1 || len(p.Datasets) > 32 || len(p.Measures) > 1024 || len(p.Dimensions) > 1024 || len(p.KPIs) > 512 || len(p.Joins) > 256 || len(p.CanonicalEntities) > 256 {
		return invalid(CodeLimit, "pack")
	}
	for i, d := range p.Datasets {
		path := "datasets[" + itoa(i) + "]"
		if !identity.Identifier(d.ID) || !validLine(d.Name, 256) || !sourceReferenceValid(d.Source) || len(d.Columns) < 1 || len(d.Columns) > 256 {
			return invalid(CodeInvalidValue, path)
		}
		if d.Source.Dataset != d.ID {
			return invalid(CodeEvidenceMismatch, path+".source.dataset")
		}
		for j, c := range d.Columns {
			if !identity.Identifier(c.ID) || !validLine(c.SourceName, 256) || !validLine(c.Name, 256) || !validLine(c.NativeType, 128) || !validLine(c.Category, 64) {
				return invalid(CodeInvalidValue, path+".columns["+itoa(j)+"]")
			}
		}
	}
	return validateEntities(p)
}

func validateEntities(p TopicPack) error {
	for i, v := range p.Measures {
		if !identity.Identifier(v.ID) || !validLine(v.Name, 256) || !validText(v.Description, 4096) || !v.Aggregation.valid() || !validOptionalLine(v.Unit, 64) {
			return invalid(CodeInvalidValue, "measures["+itoa(i)+"]")
		}
	}
	for i, v := range p.Dimensions {
		if !identity.Identifier(v.ID) || !validLine(v.Name, 256) || !validText(v.Description, 4096) || !v.Role.valid() {
			return invalid(CodeInvalidValue, "dimensions["+itoa(i)+"]")
		}
	}
	for i, v := range p.KPIs {
		if !identity.Identifier(v.ID) || !validLine(v.Name, 256) || !validText(v.Description, 4096) || !validLine(v.Expression, 4096) || len(v.Inputs) < 1 || len(v.Inputs) > 32 {
			return invalid(CodeInvalidValue, "kpis["+itoa(i)+"]")
		}
		// canonicalOrder builds sort keys from these coordinates. Reject
		// malformed/oversized references before copying or constructing keys.
		for j, ref := range v.Inputs {
			if !ref.Valid() {
				return invalid(CodeInvalidReference, "kpis["+itoa(i)+"].inputs["+itoa(j)+"]")
			}
		}
	}
	for i, v := range p.Joins {
		if !identity.Identifier(v.ID) || !validLine(v.Name, 256) || !v.Type.valid() || !v.Cardinality.valid() {
			return invalid(CodeInvalidValue, "joins["+itoa(i)+"]")
		}
	}
	for i, v := range p.CanonicalEntities {
		if !identity.Identifier(v.ID) || v.Revision < 1 || v.Revision >= 1<<62 || !validLine(v.Name, 256) || len(v.Aliases) > 32 || len(v.Keys) < 1 || len(v.Keys) > 32 {
			return invalid(CodeInvalidValue, "canonical_entities["+itoa(i)+"]")
		}
		for j, alias := range v.Aliases {
			if !validLine(alias, 256) {
				return invalid(CodeInvalidValue, "canonical_entities["+itoa(i)+"].aliases["+itoa(j)+"]")
			}
		}
	}
	return nil
}

func referenceIndex(p TopicPack) (map[string]struct{}, error) {
	refs := map[string]struct{}{}
	add := func(ref Reference, path string) error {
		if !ref.Valid() {
			return invalid(CodeInvalidValue, path)
		}
		key := ref.key()
		if _, exists := refs[key]; exists {
			return invalid(CodeDuplicateID, path)
		}
		refs[key] = struct{}{}
		return nil
	}
	for i, d := range p.Datasets {
		if err := add(Reference{Kind: KindDataset, ID: d.ID}, "datasets["+itoa(i)+"].id"); err != nil {
			return nil, err
		}
		sourceNames := map[string]bool{}
		for j, c := range d.Columns {
			path := "datasets[" + itoa(i) + "].columns[" + itoa(j) + "]"
			if sourceNames[c.SourceName] {
				return nil, invalid(CodeDuplicateID, path+".source_name")
			}
			sourceNames[c.SourceName] = true
			if err := add(Reference{Kind: KindColumn, Dataset: d.ID, ID: c.ID}, path+".id"); err != nil {
				return nil, err
			}
		}
	}
	for i, v := range p.Measures {
		if err := add(Reference{Kind: KindMeasure, ID: v.ID}, "measures["+itoa(i)+"].id"); err != nil {
			return nil, err
		}
	}
	for i, v := range p.Dimensions {
		if err := add(Reference{Kind: KindDimension, ID: v.ID}, "dimensions["+itoa(i)+"].id"); err != nil {
			return nil, err
		}
	}
	for i, v := range p.KPIs {
		if err := add(Reference{Kind: KindKPI, ID: v.ID}, "kpis["+itoa(i)+"].id"); err != nil {
			return nil, err
		}
	}
	for i, v := range p.Joins {
		if err := add(Reference{Kind: KindJoin, ID: v.ID}, "joins["+itoa(i)+"].id"); err != nil {
			return nil, err
		}
	}
	canonicalIDs := map[string]bool{}
	for i, v := range p.CanonicalEntities {
		// A pack pins one registry revision per stable entity, not multiple
		// competing definitions merely distinguished by their revision number.
		if canonicalIDs[v.ID] {
			return nil, invalid(CodeDuplicateID, "canonical_entities["+itoa(i)+"].id")
		}
		canonicalIDs[v.ID] = true
		if err := add(v.Reference(), "canonical_entities["+itoa(i)+"].id"); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func validateReferences(p TopicPack, refs map[string]struct{}) error {
	require := func(ref Reference, kinds []Kind, path string) error {
		if !ref.Valid() {
			return invalid(CodeInvalidReference, path)
		}
		allowed := false
		for _, kind := range kinds {
			allowed = allowed || ref.Kind == kind
		}
		if !allowed {
			return invalid(CodeInvalidReference, path)
		}
		if _, ok := refs[ref.key()]; !ok {
			return invalid(CodeMissingReference, path)
		}
		return nil
	}
	for i, v := range p.Measures {
		if err := require(v.Field, []Kind{KindColumn}, "measures["+itoa(i)+"].field"); err != nil {
			return err
		}
	}
	for i, v := range p.Dimensions {
		if err := require(v.Field, []Kind{KindColumn}, "dimensions["+itoa(i)+"].field"); err != nil {
			return err
		}
	}
	for i, v := range p.KPIs {
		seen := map[string]bool{}
		for j, ref := range v.Inputs {
			path := "kpis[" + itoa(i) + "].inputs[" + itoa(j) + "]"
			if err := require(ref, []Kind{KindMeasure, KindKPI}, path); err != nil {
				return err
			}
			if seen[ref.key()] {
				return invalid(CodeInvalidReference, path)
			}
			seen[ref.key()] = true
		}
	}
	sources := make(map[string]SourceReference, len(p.Datasets))
	for _, dataset := range p.Datasets {
		sources[dataset.ID] = dataset.Source
	}
	joinPairs := map[string]bool{}
	for i, v := range p.Joins {
		path := "joins[" + itoa(i) + "]"
		if err := require(v.Left, []Kind{KindColumn}, path+".left"); err != nil {
			return err
		}
		if err := require(v.Right, []Kind{KindColumn}, path+".right"); err != nil {
			return err
		}
		if v.Left.Dataset == v.Right.Dataset || v.Left.key() == v.Right.key() {
			return invalid(CodeInvalidReference, path)
		}
		left, right := sources[v.Left.Dataset], sources[v.Right.Dataset]
		if left.Source != right.Source || left.Context != right.Context || left.SourceRevision != right.SourceRevision {
			return invalid(CodeEvidenceMismatch, path)
		}
		pair := v.Left.key() + "\x01" + v.Right.key()
		reverse := v.Right.key() + "\x01" + v.Left.key()
		if joinPairs[pair] || joinPairs[reverse] {
			return invalid(CodeInvalidReference, path)
		}
		joinPairs[pair] = true
	}
	for i, v := range p.CanonicalEntities {
		seen := map[string]bool{}
		for j, ref := range v.Keys {
			path := "canonical_entities[" + itoa(i) + "].keys[" + itoa(j) + "]"
			if err := require(ref, []Kind{KindColumn}, path); err != nil {
				return err
			}
			if seen[ref.key()] {
				return invalid(CodeInvalidReference, path)
			}
			seen[ref.key()] = true
		}
	}
	return nil
}

func canonicalTerms(p TopicPack) (map[string]int, error) {
	terms := map[string]int{}
	for i, entity := range p.CanonicalEntities {
		for j, value := range append([]string{entity.Name}, entity.Aliases...) {
			term, ok := normalizeTerm(value)
			path := "canonical_entities[" + itoa(i) + "].name"
			if j > 0 {
				path = "canonical_entities[" + itoa(i) + "].aliases[" + itoa(j-1) + "]"
			}
			if !ok {
				return nil, invalid(CodeInvalidValue, path)
			}
			if _, exists := terms[term]; exists {
				return nil, invalid(CodeAmbiguousTerm, path)
			}
			terms[term] = i
		}
	}
	return terms, nil
}

func kpiCycles(p TopicPack) error {
	deps := map[string][]string{}
	for _, kpi := range p.KPIs {
		for _, ref := range kpi.Inputs {
			if ref.Kind == KindKPI {
				deps[kpi.ID] = append(deps[kpi.ID], ref.ID)
			}
		}
	}
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return invalid(CodeReferenceCycle, "kpis")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, next := range deps[id] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range deps {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func sourceReferenceValid(s SourceReference) bool {
	return identity.Identifier(s.Source) && identity.Identifier(s.Context) && identity.Identifier(s.Dataset) && identity.Identifier(s.ProfileVersion) && validDigest(s.ProfileDigest) && s.SourceRevision > 0 && s.SourceRevision < 1<<62
}

func validDigest(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func normalizeTerm(s string) (string, bool) {
	if !utf8.ValidString(s) || len(s) < 1 || len(s) > maximumCanonicalTermBytes || containsControl(s) {
		return "", false
	}
	value := cases.Fold().String(norm.NFKC.String(s))
	value = strings.Join(strings.Fields(value), " ")
	return value, value != "" && len(value) <= maximumCanonicalTermBytes
}

func canonicalOrder(p *TopicPack) {
	sort.Slice(p.Datasets, func(i, j int) bool { return p.Datasets[i].ID < p.Datasets[j].ID })
	for i := range p.Datasets {
		sort.Slice(p.Datasets[i].Columns, func(a, b int) bool { return p.Datasets[i].Columns[a].ID < p.Datasets[i].Columns[b].ID })
	}
	sort.Slice(p.Measures, func(i, j int) bool { return p.Measures[i].ID < p.Measures[j].ID })
	sort.Slice(p.Dimensions, func(i, j int) bool { return p.Dimensions[i].ID < p.Dimensions[j].ID })
	sort.Slice(p.KPIs, func(i, j int) bool { return p.KPIs[i].ID < p.KPIs[j].ID })
	for i := range p.KPIs {
		sort.Slice(p.KPIs[i].Inputs, func(a, b int) bool { return p.KPIs[i].Inputs[a].key() < p.KPIs[i].Inputs[b].key() })
	}
	sort.Slice(p.Joins, func(i, j int) bool { return p.Joins[i].ID < p.Joins[j].ID })
	sort.Slice(p.CanonicalEntities, func(i, j int) bool { return p.CanonicalEntities[i].ID < p.CanonicalEntities[j].ID })
	for i := range p.CanonicalEntities {
		sort.Slice(p.CanonicalEntities[i].Aliases, func(a, b int) bool {
			left, _ := normalizeTerm(p.CanonicalEntities[i].Aliases[a])
			right, _ := normalizeTerm(p.CanonicalEntities[i].Aliases[b])
			return left < right
		})
	}
}

func clonePack(p TopicPack) TopicPack {
	p.Datasets = append([]Dataset(nil), p.Datasets...)
	for i := range p.Datasets {
		p.Datasets[i].Columns = append([]Column(nil), p.Datasets[i].Columns...)
	}
	p.Measures = append([]Measure(nil), p.Measures...)
	p.Dimensions = append([]Dimension(nil), p.Dimensions...)
	p.KPIs = append([]KPI(nil), p.KPIs...)
	for i := range p.KPIs {
		p.KPIs[i].Inputs = append([]Reference(nil), p.KPIs[i].Inputs...)
	}
	p.Joins = append([]Join(nil), p.Joins...)
	p.CanonicalEntities = append([]CanonicalEntity(nil), p.CanonicalEntities...)
	for i := range p.CanonicalEntities {
		p.CanonicalEntities[i].Aliases = append([]string(nil), p.CanonicalEntities[i].Aliases...)
		p.CanonicalEntities[i].Keys = append([]Reference(nil), p.CanonicalEntities[i].Keys...)
	}
	return p
}

func cloneCanonical(v CanonicalEntity) CanonicalEntity {
	v.Aliases = append([]string(nil), v.Aliases...)
	v.Keys = append([]Reference(nil), v.Keys...)
	return v
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
