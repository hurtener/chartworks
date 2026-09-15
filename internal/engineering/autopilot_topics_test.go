package engineering

import (
	"testing"

	"github.com/hurtener/chartworks/internal/semantics"
)

func TestAmendmentTopicUsesActualEffectsAndPreservesMeaning(t *testing.T) {
	p := AutopilotProposal{Material: ProposalMaterial{Request: AutopilotGoal{Topic: &AutopilotTopicGoal{Topic: "topic", Profile: "profile", Version: "v1", Name: "Original name", ExpectedRevision: 3}}, Topic: &semantics.TopicPack{Topic: "topic", Name: "Reviewed name", Description: "Reviewed description"}}}
	pending := amendmentTopicGoal(p, "drift", "")
	if pending.ExpectedRevision != 3 || pending.Name != "Reviewed name" || p.Material.Request.Topic.ExpectedRevision != 3 {
		t.Fatal("uncommitted topic revision invented or reviewed edit lost", pending)
	}
	p.Effects = []ProposalEffect{{Kind: "topic_draft", Target: "topic", State: "committed", Version: 4}}
	completed := amendmentTopicGoal(p, "drift", "fresh-profile")
	if completed.ExpectedRevision != 4 || completed.Profile != "fresh-profile" || p.Material.Request.Topic.Profile != "profile" {
		t.Fatal("actual topic effect not preserved independently", completed)
	}
	prior := semantics.TopicPack{Topic: "topic", Version: "v1", Name: "Reviewed name", Datasets: []semantics.Dataset{{ID: "dataset", Source: semantics.SourceReference{ProfileVersion: "old"}, Columns: []semantics.Column{{ID: "stable", SourceName: "actual"}}}}, Measures: []semantics.Measure{{ID: "reviewed-measure", Name: "Reviewed measure"}}}
	current := semantics.TopicPack{Topic: "topic", Version: "v2", Datasets: []semantics.Dataset{{ID: "dataset", Source: semantics.SourceReference{ProfileVersion: "fresh"}}}}
	next, err := preserveAmendmentTopic(prior, current)
	if err != nil || next.Version != "v2" || next.Name != prior.Name || len(next.Measures) != 1 || next.Datasets[0].Columns[0].ID != "stable" || next.Datasets[0].Source.ProfileVersion != "fresh" {
		t.Fatal("amendment discarded reviewed semantics", next, err)
	}
	next.Datasets[0].Columns[0].ID = "changed"
	if prior.Datasets[0].Columns[0].ID != "stable" {
		t.Fatal("amendment mutated immutable parent")
	}
	current.Datasets[0].ID = "different"
	if _, err = preserveAmendmentTopic(prior, current); err == nil {
		t.Fatal("amendment silently rebound a different dataset")
	}
}
