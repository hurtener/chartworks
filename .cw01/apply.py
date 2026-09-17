"""Apply the inspected CW-01 export-reader integration and actual consumer tests."""
from pathlib import Path

pending = {}
def edit(path, before, after):
    text = pending.get(path, Path(path).read_text())
    if text.count(before) != 1:
        raise SystemExit(f"{path}: reviewed anchor changed")
    pending[path] = text.replace(before, after, 1)

# The first actual retained clarification export uses this existing access mode.
# publishedArgs -> topicArgs still enforces signed export action/resource reach
# and all persisted source, dataset and execution-context dependencies.
edit("internal/store/postgres/topic_publications.go",
     "if a != drafts.Read && a != drafts.Write && a != drafts.Review && a != drafts.Publish {",
     "if a != drafts.Read && a != drafts.Write && a != drafts.Review && a != drafts.Publish && a != drafts.Export {")

path = "test/acceptance/cw01_consumers_test.go"
edit(path, '\t"github.com/hurtener/chartworks/internal/api"',
     '\t"github.com/hurtener/chartworks/internal/access"\n\t"github.com/hurtener/chartworks/internal/api"\n\t"github.com/hurtener/chartworks/internal/auth"')
edit(path,
     '\tportable, err := client.ExportClarifications(ctx, f.pack.Topic, sdk.ClarificationExportRequest{Version: f.definition.Version})',
     '\tcw01ExportDenials(t, f, server)\n\tportable, err := client.ExportClarifications(ctx, f.pack.Topic, sdk.ClarificationExportRequest{Version: f.definition.Version})')
helper = '''func cw01ExportDenials(t *testing.T, f *cw01Fixture, server *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	before := f.model.requests.Load()
	for _, denied := range []struct {
		name, remove string
	}{
		{"missing-export-action", "topics.export"},
		{"missing-export-resource", "cw.topic.export:*"},
	} {
		t.Run(denied.name, func(t *testing.T) {
			var scopes []string
			removed := false
			for _, scope := range phase18Scopes(f.e.Tenant(), true) {
				if scope == denied.remove {
					removed = true
					continue
				}
				scopes = append(scopes, scope)
			}
			if !removed {
				t.Fatal("denial fixture did not remove the expected export reach")
			}
			claims := f.model.token.claims(f.e.Tenant(), f.e.User(), scopes)
			claims["session"] = f.e.Session()
			token := f.model.token.sign(t, claims, nil)
			envelope, err := f.model.token.verifier.Verify(ctx, token, auth.HTTP)
			if err != nil {
				t.Fatal(err)
			}
			out, err := f.rules.ExportClarifications(ctx, envelope, f.pack.Topic, rulesets.ClarificationExportRequest{Version: f.definition.Version})
			if !errors.Is(err, access.ErrForbidden) || out.RuleDigest != "" || out.Definition.Topic != "" {
				t.Fatal("direct export did not enforce signed export reach")
			}
			client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
			if err != nil {
				t.Fatal(err)
			}
			out, err = client.ExportClarifications(ctx, f.pack.Topic, sdk.ClarificationExportRequest{Version: f.definition.Version})
			var status *sdk.StatusError
			if !errors.As(err, &status) || status.Status != http.StatusForbidden || out.RuleDigest != "" || out.Definition.Topic != "" {
				t.Fatal("HTTP export did not enforce signed export reach")
			}
		})
	}
	if f.model.requests.Load() != before {
		t.Fatal("denied export invoked a provider")
	}
}

'''
edit(path, 'func cw01AuthoringAcceptance(t *testing.T) {', helper + 'func cw01AuthoringAcceptance(t *testing.T) {')
edit("test/acceptance/cw01_test.go",
     '\t\tcw01MigrationAcceptance(t)\n\t\tcw01AuthoringAcceptance(t)\n\t\tcw01ConsumerAcceptance(t)',
     '\t\tt.Run("migration", cw01MigrationAcceptance)\n\t\tt.Run("authoring", cw01AuthoringAcceptance)\n\t\tt.Run("consumers", cw01ConsumerAcceptance)')
for path, text in sorted(pending.items()):
    Path(path).write_text(text)
print("Applied checked export reader and authority/consumer regressions")
