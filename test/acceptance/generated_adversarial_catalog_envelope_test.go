package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/semantics"
)

// The original four-dataset, two-pass fixture exceeded the unchanged 65,536
// operation reservation after current catalog keys entered authoring context.
// Keep all its generated semantics, explicit operator review and publication.
func TestGeneratedAdversarialCatalogEnvelopeRecorded(t *testing.T) {
	h, fixture, current, datasets := generateAdversarialPublished(t, true)
	keys := map[string][][]string{
		datasets["customers"]:   {{"customer_id", "division_id"}},
		datasets["orders"]:      {{"division_id", "order_id"}},
		datasets["refunds"]:     {{"division_id", "refund_id"}},
		datasets["order_lines"]: {{"division_id", "line_id"}},
	}
	profiles := map[string]engineering.Profile{}
	for _, d := range current.Pack.Datasets {
		e, err := h.f.service.Evidence(t.Context(), h.author, d.Source.ProfileVersion)
		if err != nil {
			t.Fatal("retained source profile", err)
		}
		profiles[d.ID] = e.Profile
	}
	fixture.mu.Lock()
	bodies := append([]string(nil), fixture.requestBodies...)
	fixture.mu.Unlock()
	seen, reviews := 0, 0
	for _, body := range bodies {
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(body), &request) != nil {
			continue
		}
		for _, message := range request.Messages {
			if message.Role != "user" {
				continue
			}
			var packet struct {
				Context  json.RawMessage `json:"context"`
				Coverage []string        `json:"coverage"`
			}
			if json.Unmarshal([]byte(message.Content), &packet) != nil || len(packet.Context) == 0 {
				continue
			}
			var material struct {
				Digest    string              `json:"digest"`
				Candidate semantics.TopicPack `json:"candidate"`
				Evidence  []struct {
					Origin       semantics.SourceReference `json:"origin"`
					ObservedAt   time.Time                 `json:"observed_at"`
					PolicyDigest string                    `json:"policy_digest"`
					Sampling     engineering.Sampling      `json:"sampling"`
					Relation     struct {
						Schema   string     `json:"schema"`
						Name     string     `json:"name"`
						Keys     [][]string `json:"non_null_unique_keys"`
						Evidence string     `json:"key_evidence"`
					} `json:"relation"`
					Columns struct {
						Encoding string              `json:"encoding"`
						Fields   []string            `json:"fields"`
						Rows     [][]json.RawMessage `json:"rows"`
					} `json:"columns"`
				} `json:"profile_evidence"`
			}
			if json.Unmarshal(packet.Context, &material) != nil || len(material.Evidence) != 4 || len(material.Candidate.Datasets) != 4 {
				t.Fatal("generation/review lost whole-candidate source evidence")
			}
			needle := []byte(`"digest":"` + material.Digest + `"`)
			if bytes.Count(packet.Context, needle) != 1 {
				t.Fatal("missing exact context digest")
			}
			unsigned := bytes.Replace(packet.Context, needle, []byte(`"digest":""`), 1)
			sum := sha256.Sum256(unsigned)
			if hex.EncodeToString(sum[:]) != material.Digest {
				t.Fatal("context digest did not cover exact table, keys and provenance")
			}
			for _, evidence := range material.Evidence {
				profile, ok := profiles[evidence.Origin.Dataset]
				if !ok || evidence.Origin.Source != profile.Source || evidence.Origin.Context != profile.Context || evidence.Origin.SourceRevision != profile.SourceRevision || evidence.Origin.ProfileVersion != profile.Version || evidence.Origin.ProfileDigest != profile.DeterministicHash() || evidence.PolicyDigest != profile.PolicyHash || !evidence.ObservedAt.Equal(profile.ObservedAt) || !reflect.DeepEqual(evidence.Sampling, profile.Sampling) {
					t.Fatal("current source/profile provenance changed")
				}
				relation := evidence.Relation
				name := ""
				for table, id := range datasets {
					if id == evidence.Origin.Dataset {
						name = "adv_" + table
					}
				}
				if relation.Schema != "analytics" || relation.Name != name || relation.Evidence != "current_authorized_catalog" || !reflect.DeepEqual(relation.Keys, keys[evidence.Origin.Dataset]) {
					t.Fatal("complete non-null catalog key lost or changed")
				}
				columns := evidence.Columns
				fields := []string{"column", "observed", "nulls", "sample_distinct", "distinct_exact", "families", "disclosure"}
				if columns.Encoding != "profile-column-table-v1" || !reflect.DeepEqual(columns.Fields, fields) || len(columns.Rows) != len(profile.Columns) {
					t.Fatal("profile field/row omitted or reinterpreted")
				}
				seenColumns := map[string]bool{}
				for _, row := range columns.Rows {
					if len(row) != len(fields) {
						t.Fatal("ragged profile evidence")
					}
					object := map[string]json.RawMessage{}
					for i, field := range fields {
						object[field] = row[i]
					}
					raw, _ := json.Marshal(object)
					var aggregate struct {
						Column     string         `json:"column"`
						Observed   int            `json:"observed"`
						Nulls      int            `json:"nulls"`
						Distinct   int            `json:"sample_distinct"`
						Exact      bool           `json:"distinct_exact"`
						Families   map[string]int `json:"families"`
						Disclosure string         `json:"disclosure"`
					}
					if json.Unmarshal(raw, &aggregate) != nil || seenColumns[aggregate.Column] {
						t.Fatal("ambiguous aggregate identity")
					}
					seenColumns[aggregate.Column] = true
					found := false
					for _, stat := range profile.Columns {
						if stat.Name != aggregate.Column {
							continue
						}
						found = true
						if aggregate.Observed != stat.Observed || aggregate.Nulls != stat.Nulls || aggregate.Distinct != stat.Distinct || aggregate.Exact != stat.DistinctExact {
							t.Fatal("profile count or exactness changed")
						}
						if stat.Name != "status_code" {
							if aggregate.Disclosure != "aggregate_counts_only" || aggregate.Families != nil {
								t.Fatal("sensitive or unclassified family disclosed")
							}
						} else if aggregate.Disclosure != "aggregate_counts_and_type_families" || !reflect.DeepEqual(aggregate.Families, stat.Families) {
							t.Fatal("safe family or disclosure omitted")
						}
					}
					if !found {
						t.Fatal("unknown profile column")
					}
				}
			}
			seen++
			if len(packet.Coverage) > 0 {
				reviews++
			}
		}
	}
	if seen != 24 || reviews != 2 {
		t.Fatalf("expected both 11-page generations and both independent reviews, got packets=%d reviews=%d", seen, reviews)
	}
	// The original independent answer oracles continue beyond publication.
	evaluateAdversarialGrossCases(t, h, fixture, current, datasets)
	if current.Quality.Status != "needs_review" {
		t.Fatal("compaction waived the independent advisory")
	}
}
