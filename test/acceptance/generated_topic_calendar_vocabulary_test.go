package acceptance

import (
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/semantics"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestGeneratedTopicCalendarVocabularyRecorded(t *testing.T) {
	for _, calendar := range semantics.EnhancementCalendarNames() {
		t.Run(calendar, func(t *testing.T) {
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
				responses := []string{recordedGeneratedStep(t, columns, calendar)}
				if last {
					responses = append(responses, "topic_quality_echo")
				}
				model.mu.Lock()
				model.chatSequence = responses
				model.mu.Unlock()
			})
			reloaded, err := h.client.TopicDraftVersion(t.Context(), current.Pack.Topic, current.Metadata.Revision)
			if err != nil || reloaded.Metadata.Digest != current.Metadata.Digest {
				t.Fatal("canonical checkpoint reload", err)
			}
			found := false
			for _, dimension := range reloaded.Pack.Dimensions {
				if dimension.Field.ID == "ordered_at" {
					found = dimension.Temporal != nil && dimension.Temporal.Calendar == "gregorian" && dimension.Temporal.Timezone == "UTC"
				}
			}
			if !found {
				t.Fatal("display calendar spelling reached retained topic")
			}
			h.publish(t, current, report)
			h.runQueries(t, t.Context(), current, report, func(sql string) { model.mode.Store(phase18RawResponse(t, sql)) })
			if !report.Complete || len(report.Steps) != 2 || len(report.Queries) != 2 {
				t.Fatal("canonical topic did not reach independent gross/month result oracles")
			}
			assertRecordedAuthoringEnvelopes(t, report)
		})
	}
}

func TestGeneratedTopicUnsupportedCalendarDoesNotSave(t *testing.T) {
	model := newGatewayFixture(t, recordedLiveChatCaps)
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	compiled, err := semantics.Compile(h.scaffold.Pack)
	if err != nil {
		t.Fatal(err)
	}
	columns := semantics.GenerationColumns(compiled)
	found := false
	for _, column := range columns {
		found = found || column.ID == "ordered_at"
	}
	if !found {
		t.Fatal("fixture lacks the temporal proposal")
	}
	model.mode.Store(recordedGeneratedStep(t, columns, "fiscal"))
	before := model.requests.Load()
	_, err = h.client.EnhanceTopicDraft(t.Context(), h.scaffold.Pack.Topic, sdk.EnhanceTopicRequest{Expected: h.scaffold.Metadata.Revision, Version: "invalid-calendar", Cursor: 0, Limit: len(columns), Change: "Synthetic unsupported calendar proposal"})
	if err == nil || model.requests.Load() != before+1 {
		t.Fatal("unsupported calendar was admitted or retried")
	}
	head, err := h.client.TopicDraft(t.Context(), h.scaffold.Pack.Topic)
	if err != nil || head.Metadata.Revision != h.scaffold.Metadata.Revision || head.Metadata.Digest != h.scaffold.Metadata.Digest {
		t.Fatal("invalid proposal changed durable checkpoint", err)
	}
}
