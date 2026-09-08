package chartworks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/semantics"
)

func TestTopicDiffAcceptsExpandedBoundedCompilerOutput(t *testing.T) {
	build := func(prefix string) semantics.Model {
		p := TopicPack{SchemaVersion: 1, Topic: "large-topic", Version: prefix, Name: "Large synthetic topic"}
		for i := 0; i < 32; i++ {
			id := fmt.Sprintf("dataset-%02d-", i) + strings.Repeat("x", 110)
			d := TopicDataset{ID: id, Name: "Data", Source: TopicSourceReference{Source: "source", Context: "source:v1", Dataset: id, ProfileVersion: "profile", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 1}}
			for j := 0; j < 256; j++ {
				column := fmt.Sprintf("%s%d", prefix, j)
				d.Columns = append(d.Columns, TopicColumn{ID: column, SourceName: column, Name: "Value", NativeType: "integer", Category: "numeric"})
			}
			p.Datasets = append(p.Datasets, d)
		}
		model, err := semantics.Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		return model
	}
	diff, err := semantics.DiffModels(build("a"), build("b"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= 2<<20 || len(raw) > 16<<20 {
		t.Fatal("fixture must exceed an ordinary response while remaining bounded", len(raw))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/topics/large-topic/draft-diff" || r.Method != "POST" {
			t.Error("wrong concrete route")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	c, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.DiffTopicDraft(context.Background(), "large-topic", 1, 2)
	if err != nil || len(out.Changes) != len(diff.Changes) || out.AfterDigest != diff.AfterDigest {
		t.Fatal("bounded expanded diff lost", len(out.Changes), err)
	}
}
func TestTopicSDKRejectsUnsafePathIDs(t *testing.T) {
	c, err := New("https://chartworks.invalid", nil, func(context.Context) (string, error) { t.Fatal("credential callback on invalid path"); return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []string{"", "../topic", "topic?scope=all"} {
		if _, err = c.TopicDraft(ctx, id); err == nil {
			t.Fatal("current path")
		}
		if _, err = c.TopicDraftVersion(ctx, id, 1); err == nil {
			t.Fatal("version path")
		}
		if _, err = c.TopicDraftHistory(ctx, id, 0, 1); err == nil {
			t.Fatal("history path")
		}
		if _, err = c.DiffTopicDraft(ctx, id, 1, 2); err == nil {
			t.Fatal("diff path")
		}
		if _, err = c.ExportTopicDraft(ctx, id, 1, nil); err == nil {
			t.Fatal("export path")
		}
	}
}
