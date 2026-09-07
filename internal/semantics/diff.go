package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeRemoved  ChangeKind = "removed"
	ChangeModified ChangeKind = "modified"
)

// EntityChange identifies content changes without exposing source names,
// provenance payloads, or authoring text. Canonical entities match by stable ID;
// changing their exact revision is a modification, not an unrelated new entity.
type EntityChange struct {
	Kind         Kind       `json:"kind"`
	Dataset      string     `json:"dataset,omitempty"`
	ID           string     `json:"id"`
	Change       ChangeKind `json:"change"`
	BeforeDigest string     `json:"before_digest,omitempty"`
	AfterDigest  string     `json:"after_digest,omitempty"`
}

type ChangeCount struct {
	Kind     Kind `json:"kind"`
	Added    int  `json:"added"`
	Removed  int  `json:"removed"`
	Modified int  `json:"modified"`
}

// VersionDiff compares two compiled definitions of the same topic. It is not
// persisted lifecycle history, review approval, or evidence of an actor's actions.
type VersionDiff struct {
	Topic           string         `json:"topic"`
	BeforeVersion   string         `json:"before_version"`
	AfterVersion    string         `json:"after_version"`
	BeforeDigest    string         `json:"before_digest"`
	AfterDigest     string         `json:"after_digest"`
	MetadataChanged bool           `json:"metadata_changed"`
	Changes         []EntityChange `json:"changes"`
	Counts          []ChangeCount  `json:"counts"`
}

// DiffModels distinguishes dataset metadata/provenance from individual column
// changes. It compares exact canonical content; it does not infer renames or moves.
func DiffModels(before, after Model) (VersionDiff, error) {
	if before.Digest() == "" || after.Digest() == "" || before.pack.Topic != after.pack.Topic {
		return VersionDiff{}, invalid(CodeEvidenceMismatch, "diff.topic")
	}
	out := VersionDiff{Topic: before.pack.Topic, BeforeVersion: before.pack.Version, AfterVersion: after.pack.Version, BeforeDigest: before.Digest(), AfterDigest: after.Digest(), MetadataChanged: before.pack.Name != after.pack.Name || before.pack.Description != after.pack.Description, Changes: []EntityChange{}, Counts: []ChangeCount{}}
	left, right := entityDigests(before), entityDigests(after)
	keys := make([]Reference, 0, len(left)+len(right))
	for ref := range left {
		keys = append(keys, ref)
	}
	for ref := range right {
		if _, exists := left[ref]; !exists {
			keys = append(keys, ref)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].key() < keys[j].key() })
	counts := map[Kind]ChangeCount{}
	for _, ref := range keys {
		previous, next := left[ref], right[ref]
		if previous == next {
			continue
		}
		change := EntityChange{Kind: ref.Kind, Dataset: ref.Dataset, ID: ref.ID, BeforeDigest: previous, AfterDigest: next, Change: ChangeModified}
		count := counts[ref.Kind]
		count.Kind = ref.Kind
		switch {
		case previous == "":
			change.Change = ChangeAdded
			count.Added++
		case next == "":
			change.Change = ChangeRemoved
			count.Removed++
		default:
			count.Modified++
		}
		out.Changes = append(out.Changes, change)
		counts[ref.Kind] = count
	}
	for _, count := range counts {
		out.Counts = append(out.Counts, count)
	}
	sort.Slice(out.Counts, func(i, j int) bool { return out.Counts[i].Kind < out.Counts[j].Kind })
	return out, nil
}

// Entity hash keys deliberately omit a canonical registry revision so a changed
// revision remains a modification of the same identity. They are not public refs.
func entityDigests(model Model) map[Reference]string {
	out := map[Reference]string{}
	add := func(ref Reference, value any) {
		// Every value is a closed, previously compiled semantic struct.
		body, _ := json.Marshal(value)
		sum := sha256.Sum256(body)
		out[ref] = hex.EncodeToString(sum[:])
	}
	for _, dataset := range model.pack.Datasets {
		add(Reference{Kind: KindDataset, ID: dataset.ID}, struct {
			ID     string          `json:"id"`
			Name   string          `json:"name"`
			Source SourceReference `json:"source"`
		}{dataset.ID, dataset.Name, dataset.Source})
		for _, column := range dataset.Columns {
			add(Reference{Kind: KindColumn, Dataset: dataset.ID, ID: column.ID}, column)
		}
	}
	for _, measure := range model.pack.Measures {
		add(Reference{Kind: KindMeasure, ID: measure.ID}, measure)
	}
	for _, dimension := range model.pack.Dimensions {
		add(Reference{Kind: KindDimension, ID: dimension.ID}, dimension)
	}
	for _, kpi := range model.pack.KPIs {
		add(Reference{Kind: KindKPI, ID: kpi.ID}, kpi)
	}
	for _, join := range model.pack.Joins {
		add(Reference{Kind: KindJoin, ID: join.ID}, join)
	}
	for _, entity := range model.pack.CanonicalEntities {
		add(Reference{Kind: KindCanonicalEntity, ID: entity.ID}, entity)
	}
	return out
}
