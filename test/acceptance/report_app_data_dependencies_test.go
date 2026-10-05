package acceptance

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportAppDataDependencyDiscovery(t *testing.T) {
	f, service, author, request, publication, dataset, _ := reportDatasetFixture(t, "dependency-chart")
	ctx := t.Context()
	seed := []string{"reporting.discover", "cw.topic.read:" + publication.Definition.Topic}
	actor := func(tenant, user, session string, scopes []string) identity.Envelope {
		t.Helper()
		claims := f.f.f.token.claims(tenant, user, scopes)
		claims["session"] = session
		e, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	// The fixture's verifier claims helper fixes the ordinary canonical session.
	e := phase27Actor(t, f.f, author.User(), seed)
	topicRequest := reporting.DataDependencyRequest{Topic: reporting.TopicPin{Topic: publication.Definition.Topic}}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	registry, err := reportingapi.DependencyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, reportingapi.DependencyHandler(f.f.f.token.verifier, service, http.NotFoundHandler()))
	bearer := f.f.f.token.sign(t, f.f.f.token.claims(author.Tenant(), author.User(), seed), nil)
	wire, _ := json.Marshal(topicRequest)
	if response := callProtected(t, handler, "POST", reportingapi.DataDependencyDiscoveryPath, bearer, string(wire), map[string]string{"Content-Type": "application/json"}); response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, field := range []string{"references", "tenant", "scopes", "session"} {
		body := strings.TrimSuffix(string(wire), "}") + `,"` + field + `":[]}`
		if response := callProtected(t, handler, "POST", reportingapi.DataDependencyDiscoveryPath, bearer, body, map[string]string{"Content-Type": "application/json"}); response.Code != 400 {
			t.Fatal("untrusted metadata field accepted", field, response.Code)
		}
	}

	out, err := service.DataDependencies(ctx, e, topicRequest)
	if err != nil || out.Topic != request.Intent.Topic || out.Dataset != "" || out.Preparation != "" || len(out.QueryReferences) != 0 {
		t.Fatal(out, err)
	}
	for _, d := range publication.Definition.Datasets {
		for _, ref := range []reporting.ResourceReference{{Kind: "source", Permission: "read", ID: d.Source.Source}, {Kind: "dataset", Permission: "query", ID: d.ID}, {Kind: "execution_context", Permission: "use", ID: d.Source.Context}} {
			if !slices.Contains(out.References, ref) {
				t.Fatal("missing whole-publication dependency", ref)
			}
		}
	}
	if _, err := service.Dataset(ctx, e, reporting.AuthoringDatasetRequest{Topic: out.Topic, Dataset: dataset.ID}); err == nil {
		t.Fatal("discovery seed granted content")
	}
	topicRequest.Topic, topicRequest.Dataset = request.Intent.Topic, dataset.ID
	out, err = service.DataDependencies(ctx, e, topicRequest)
	if err != nil || len(out.QueryReferences) != 3 || !slices.Contains(out.QueryReferences, reporting.ResourceReference{Kind: "source", Permission: "query", ID: dataset.Source.Source}) {
		t.Fatal(out, err)
	}
	for _, change := range []func(*reporting.DataDependencyRequest){func(r *reporting.DataDependencyRequest) { r.Topic.Digest = strings.Repeat("a", 64) }, func(r *reporting.DataDependencyRequest) { r.Dataset = "guessed" }, func(r *reporting.DataDependencyRequest) { r.Topic.Topic = "guessed" }} {
		bad := topicRequest
		change(&bad)
		if got, err := service.DataDependencies(ctx, e, bad); err == nil || got.Version != "" {
			t.Fatal("invalid selection disclosed metadata", got, err)
		}
	}
	for _, scopes := range [][]string{{"cw.topic.read:" + publication.Definition.Topic}, {"reporting.discover", "cw.topic.read:guessed"}, {"reporting.discover", "cw.topic.read:*"}} {
		if _, err := service.DataDependencies(ctx, phase27Actor(t, f.f, author.User(), scopes), topicRequest); err == nil {
			t.Fatal("missing exact root admitted")
		}
	}
	if _, err := service.DataDependencies(ctx, actor("foreign", author.User(), author.Session(), seed), topicRequest); err == nil {
		t.Fatal("foreign tenant admitted")
	}
	if f.attemptCount(t) != before || f.f.model.requests.Load() != models {
		t.Fatal("discovery executed source/model")
	}
	preparation, err := service.PrepareDatasetChart(ctx, author, request)
	if err != nil || preparation.Status != "prepared" {
		t.Fatal(preparation, err)
	}
	privateSeed := append(slices.Clone(seed), "cw.block.read:"+request.NewBlock, "cw.block.write:"+request.NewBlock, "cw.block.preview:"+request.NewBlock, "reporting.preview")
	private := phase27Actor(t, f.f, author.User(), privateSeed)
	byOperation := reporting.DataDependencyRequest{NewBlock: request.NewBlock, Operation: request.Operation}
	out, err = service.DataDependencies(ctx, private, byOperation)
	if err != nil || out.Preparation != preparation.Preparation || out.Dataset != dataset.ID || out.Topic != request.Intent.Topic {
		t.Fatal(out, err)
	}
	// Replay discovery uses original custody even if the proposed topic has moved.
	replay := byOperation
	replay.Topic = request.Intent.Topic
	replay.Topic.Digest = strings.Repeat("a", 64)
	replay.Dataset = "guessed"
	retained, err := service.DataDependencies(ctx, private, replay)
	if err != nil || retained.Topic != out.Topic || retained.Dataset != out.Dataset {
		t.Fatal("replay reinterpreted intent", retained, err)
	}
	byID := reporting.DataDependencyRequest{NewBlock: request.NewBlock, Preparation: preparation.Preparation}
	for _, scopes := range [][]string{privateSeed[:len(privateSeed)-1], slices.DeleteFunc(slices.Clone(privateSeed), func(s string) bool { return s == "cw.block.write:"+request.NewBlock })} {
		if _, err := service.DataDependencies(ctx, phase27Actor(t, f.f, author.User(), scopes), byID); err == nil {
			t.Fatal("private seed incomplete")
		}
	}
	for _, other := range []identity.Envelope{phase27Actor(t, f.f, "different-user", privateSeed), actor(author.Tenant(), author.User(), "different-session", privateSeed)} {
		if got, err := service.DataDependencies(ctx, other, byID); err == nil || got.Version != "" {
			t.Fatal("private custody leaked", got, err)
		}
	}
	raw, _ := json.Marshal(out)
	for _, secret := range []string{publication.Definition.Name, "\"definition\"", "\"sql\"", "\"schema\"", "\"session\"", "\"actor\"", "\"source_operation\""} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("content escaped discovery", secret)
		}
	}
	if _, err := service.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: preparation.Preparation, Digest: preparation.Digest}); err != nil {
		t.Fatal(err)
	}
	out, err = service.DataDependencies(ctx, private, byID)
	if err != nil || out.Preparation != preparation.Preparation || out.Dataset != dataset.ID {
		t.Fatal("consumed custody lost", out, err)
	}
	if f.attemptCount(t) != before+1 || f.f.model.requests.Load() != models {
		t.Fatal("metadata/consume executed another query/model")
	}
}
