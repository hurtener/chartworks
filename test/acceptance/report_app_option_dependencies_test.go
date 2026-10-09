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

func TestReportAppOptionDependencyDiscovery(t *testing.T) {
	f, s, author, prepare, _, _ := filteredDatasetFixture(t)
	ctx := t.Context()
	in := optionsForPreparation(prepare, 901)
	seed := []string{"reporting.discover", "reporting.preview", "cw.topic.read:" + prepare.Intent.Topic.Topic, "cw.block.read:" + prepare.NewBlock, "cw.block.write:" + prepare.NewBlock, "cw.block.preview:" + prepare.NewBlock}
	e := phase27Actor(t, f.f, author.User(), seed)
	request := reporting.OptionDependencyRequest{Mode: "search", Target: in.Target, Operation: in.Operation}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	m, err := s.OptionDependencies(ctx, e, request)
	if err != nil || m.Version != "report-option-dependencies-v1" || m.Original || m.Operation != in.Operation {
		t.Fatal(m, err)
	}
	for _, action := range []string{"charts.bind", "reporting.read", "reporting.write", "reporting.preview", "reporting.validate", "sources.query", "topics.read"} {
		if !slices.Contains(m.Actions, action) {
			t.Fatal("missing action", action)
		}
	}
	if _, err := s.DatasetOptions(ctx, e, in); err == nil {
		t.Fatal("metadata seed executed source")
	}
	registry, err := reportingapi.DependencyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, reportingapi.DependencyHandler(f.f.f.token.verifier, s, http.NotFoundHandler()))
	bearer := f.f.f.token.sign(t, f.f.f.token.claims(author.Tenant(), author.User(), seed), nil)
	wire, _ := json.Marshal(request)
	if response := callProtected(t, handler, "POST", reportingapi.OptionDependencyDiscoveryPath, bearer, string(wire), map[string]string{"Content-Type": "application/json"}); response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, field := range []string{"search", "cursor", "references", "tenant", "session"} {
		body := strings.TrimSuffix(string(wire), "}") + `,"` + field + `":"injected"}`
		if response := callProtected(t, handler, "POST", reportingapi.OptionDependencyDiscoveryPath, bearer, body, map[string]string{"Content-Type": "application/json"}); response.Code != 400 {
			t.Fatal("open discovery DTO", field, response.Code)
		}
	}
	for _, mode := range []string{"status", "control"} {
		unknown := request
		unknown.Mode = mode
		if got, err := s.OptionDependencies(ctx, e, unknown); err == nil || got.Version != "" {
			t.Fatal("missing custody fabricated", got, err)
		}
	}
	if f.attemptCount(t) != before || f.f.model.requests.Load() != models {
		t.Fatal("discovery executed")
	}
	page, err := s.DatasetOptions(ctx, author, in)
	if err != nil || !page.ValuesAvailable {
		t.Fatal(page, err)
	}
	for _, mode := range []string{"search", "status", "control"} {
		request.Mode = mode
		m, err = s.OptionDependencies(ctx, e, request)
		if err != nil || !m.Original || m.Operation != in.Operation {
			t.Fatal("original custody", m, err)
		}
	}
	raw, _ := json.Marshal(m)
	for _, field := range []string{"\"options\"", "\"sql\"", "\"search\"", "\"cursor\"", "\"actor\"", "\"session\"", "\"source_operation\""} {
		if strings.Contains(string(raw), field) {
			t.Fatal("metadata exposed payload", field)
		}
	}
	for _, scopes := range [][]string{seed[1:], slices.DeleteFunc(slices.Clone(seed), func(s string) bool { return s == "cw.block.write:"+prepare.NewBlock }), slices.DeleteFunc(slices.Clone(seed), func(s string) bool { return s == "reporting.preview" })} {
		if got, err := s.OptionDependencies(ctx, phase27Actor(t, f.f, author.User(), scopes), request); err == nil || got.Version != "" {
			t.Fatal("incomplete root disclosed custody", got, err)
		}
	}
	otherSession, err := f.f.f.token.verifier.Verify(ctx, phase27Token(t, f.f, author.User(), "different-option-login", seed), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	claims := f.f.f.token.claims("foreign", author.User(), seed)
	foreign, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []identity.Envelope{phase27Actor(t, f.f, "other-option-actor", seed), otherSession, foreign} {
		if got, err := s.OptionDependencies(ctx, other, request); err == nil || got.Version != "" {
			t.Fatal("foreign custody disclosed", got, err)
		}
	}
	changed := phase27Copy(t, request)
	changed.Target.Dataset.Dimension = "other"
	if got, err := s.OptionDependencies(ctx, e, changed); err == nil || got.Version != "" {
		t.Fatal("target digest bypass", got, err)
	}
	// The response alone is not authority: a complete current envelope is still
	// needed for status. Exact replay and status never reconstruct values.
	status, err := s.OptionStatus(ctx, author, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err != nil || status.ValuesAvailable || !status.NewOperationAllowed {
		t.Fatal(status, err)
	}
	if f.attemptCount(t) != before+1 || f.f.model.requests.Load() != models {
		t.Fatal("metadata or status repeated source/model")
	}
}
