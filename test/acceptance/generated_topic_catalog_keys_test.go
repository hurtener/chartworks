package acceptance

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestGeneratedTopicCatalogKeyEvidenceRecorded(t *testing.T) {
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		r := c.Roles["embedding"]
		r.MaxBatchItems, r.MaxBatchBytes = 64, 65536
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		responses := []string{recordedGeneratedStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	})
	model.mu.Lock()
	bodies := append([]string(nil), model.requestBodies...)
	model.mu.Unlock()
	seen := 0
	for _, body := range bodies {
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(body), &request) != nil {
			continue
		}
		for _, message := range request.Messages {
			if message.Role != "user" {
				continue
			}
			var content string
			if json.Unmarshal(message.Content, &content) != nil {
				continue
			}
			var packet struct {
				Context struct {
					Evidence []struct {
						Relation struct {
							Schema string     `json:"schema"`
							Name   string     `json:"name"`
							Keys   [][]string `json:"non_null_unique_keys"`
							Policy string     `json:"key_evidence"`
						} `json:"relation"`
					} `json:"profile_evidence"`
				} `json:"context"`
			}
			if json.Unmarshal([]byte(content), &packet) != nil || len(packet.Context.Evidence) == 0 {
				continue
			}
			if len(packet.Context.Evidence) != 1 {
				t.Fatal("unselected source evidence entered authoring")
			}
			r := packet.Context.Evidence[0].Relation
			if r.Schema != "analytics" || r.Name != "orders" || r.Policy != "current_authorized_catalog" || !reflect.DeepEqual(r.Keys, [][]string{{"order_id"}}) {
				t.Fatalf("native primary key not projected exactly: %#v", r)
			}
			seen++
		}
	}
	if seen != 3 {
		t.Fatalf("expected both enhancement pages and final review to receive key evidence, got %d", seen)
	}
	// The normal publication and independently calculated gross/month oracles
	// remain unchanged. Key metadata is advisory context, not query authority.
	h.publish(t, current, report)
	h.runQueries(t, t.Context(), current, report, func(sql string) { model.mode.Store(phase18RawResponse(t, sql)) })
	if !report.Complete || len(report.Queries) != 2 {
		t.Fatal("recorded consumer oracle did not complete")
	}
}
