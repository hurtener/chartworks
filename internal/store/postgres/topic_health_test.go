package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type topicRowFixture struct {
	body []byte
	err  error
}

func (r topicRowFixture) Scan(values ...any) error {
	if r.err != nil {
		return r.err
	}
	*values[0].(*string) = "topic"
	*values[1].(*int64) = 1
	*values[2].(*string) = "v1"
	*values[3].(*string) = strings.Repeat("0", 64)
	*values[4].(*string) = "actor"
	*values[5].(*string) = "session"
	*values[6].(*time.Time) = time.Unix(1, 0)
	*values[7].(*string) = "change"
	*values[8].(*[]byte) = r.body
	return nil
}

func TestValidHealthIssuesRejectsMalformedAndDuplicateEvidence(t *testing.T) {
	valid := topics.HealthIssue{Source: "source", Context: "context", Dataset: "dataset", Code: "schema_changed"}
	if !validHealthIssues(nil) || !validHealthIssues([]topics.HealthIssue{valid}) {
		t.Fatal("valid health evidence rejected")
	}
	tooMany := make([]topics.HealthIssue, 1025)
	if validHealthIssues(tooMany) {
		t.Fatal("oversized health evidence accepted")
	}
	for _, test := range []struct {
		name   string
		issues []topics.HealthIssue
	}{
		{name: "invalid source", issues: []topics.HealthIssue{{Source: "bad/source", Context: "context", Dataset: "dataset", Code: "source_unavailable"}}},
		{name: "invalid context", issues: []topics.HealthIssue{{Source: "source", Context: "bad/context", Dataset: "dataset", Code: "source_revision_changed"}}},
		{name: "invalid dataset", issues: []topics.HealthIssue{{Source: "source", Context: "context", Dataset: "bad/dataset", Code: "dataset_missing"}}},
		{name: "unknown code", issues: []topics.HealthIssue{{Source: "source", Context: "context", Dataset: "dataset", Code: "unknown"}}},
		{name: "duplicate dataset", issues: []topics.HealthIssue{valid, {Source: "source", Context: "context", Dataset: "dataset", Code: "dataset_missing"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if validHealthIssues(test.issues) {
				t.Fatal("malformed health evidence accepted")
			}
		})
	}
}

func TestScanTopicRejectsCorruptRetainedManifests(t *testing.T) {
	for _, body := range [][]byte{[]byte("{"), []byte("{}")} {
		if _, err := scanTopic(topicRowFixture{body: body}); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("corrupt retained topic accepted", err)
		}
	}
	marker := errors.New("scan failed")
	if _, err := scanTopic(topicRowFixture{err: marker}); !errors.Is(err, marker) {
		t.Fatal("row scan failure lost", err)
	}
}

func TestCanonicalPreflightRejectsDuplicateMeaningAndTermClaims(t *testing.T) {
	one := semantics.CanonicalMeaning{ID: "one", Revision: 1, Name: "Shared"}
	two := semantics.CanonicalMeaning{ID: "two", Revision: 1, Name: "Other", Aliases: []string{"shared"}}
	if _, err := checkCanonicalMeaningsTx(context.Background(), nil, "tenant", []semantics.CanonicalMeaning{one, one}, false); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("duplicate canonical meaning accepted", err)
	}
	if _, err := checkCanonicalMeaningsTx(context.Background(), nil, "tenant", []semantics.CanonicalMeaning{one, two}, false); !errors.Is(err, store.ErrConflict) {
		t.Fatal("duplicate normalized term claim accepted", err)
	}
	if _, err := (&DB{}).CheckCanonicalMeanings(context.Background(), identity.Envelope{}, "topic", drafts.Read, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("unsupported canonical access accepted", err)
	}
}
