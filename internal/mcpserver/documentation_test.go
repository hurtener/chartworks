package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/staticdocs"
)

func fixtureDocumentation(t *testing.T, action string, count, size int) *staticdocs.Catalog {
	t.Helper()
	docs := make([]staticdocs.Document, count)
	for i := range docs {
		docs[i] = staticdocs.Document{Reference: staticdocs.Reference{URI: staticdocs.Namespace + string(rune('a'+i)) + "/v1", Name: "Synthetic contract", Description: "Synthetic data-free documentation for registration tests.", MIMEType: staticdocs.MIME, Version: 1, DocumentRef: "docs/contracts/synthetic-v1.md"}, Text: strings.Repeat("x", size)}
	}
	c, err := staticdocs.New(action, docs)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestDocumentationAssociationBoundsAndCollisions(t *testing.T) {
	binding := testRegistry(t, fixtureCall).bindings[0]
	c := fixtureDocumentation(t, "fixture.read", 1, 100)
	associated, err := WithDocumentation(binding, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WithDocumentation(associated, c); !errors.Is(err, ErrRegistration) {
		t.Fatal("duplicate association", err)
	}
	if _, err := WithDocumentation(binding, fixtureDocumentation(t, "reporting.read", 1, 100)); !errors.Is(err, ErrRegistration) {
		t.Fatal("action substitution", err)
	}
	mutated := binding
	mutated.effects.readOnly = false
	if _, err := WithDocumentation(mutated, c); !errors.Is(err, ErrRegistration) {
		t.Fatal("mutation association", err)
	}
	mutated = associated
	for _, pattern := range []string{staticdocs.Namespace + "{item}/v1", staticdocs.Namespace + "nested/{item}/v1", "chartworks://report_app/{item}/other/v1"} {
		mutated.resource = pattern
		if _, err := NewRegistry([]Binding{mutated}); !errors.Is(err, ErrRegistration) {
			t.Fatal("reserved namespace collision", pattern, err)
		}
	}
	registry, err := NewRegistry([]Binding{associated})
	if err != nil || len(registry.Manifest()) != 1 {
		t.Fatal("documentation added tools", err)
	}
	copied := associated
	copied.name = "second"
	copied.definition.ID = "second"
	copied.resource = ""
	if _, err := NewRegistry([]Binding{associated, copied}); !errors.Is(err, ErrRegistration) {
		t.Fatal("duplicate catalogs", err)
	}
}
func TestDocumentationMountedReadsEnforceResponseCapAndFreshAuthority(t *testing.T) {
	authority := newAuthority(t)
	binding := testRegistry(t, fixtureCall).bindings[0]
	c := fixtureDocumentation(t, "fixture.read", 1, 20<<10)
	binding, err := WithDocumentation(binding, c)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry([]Binding{binding})
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultMCP()
	settings.MaxResponseBytes = 16 << 10
	server, err := New(authority.verifier, registry, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	token := authority.token(t, "tenant", "actor", authority.cfg.MCPAudience(), "mcp.use", "fixture.read")
	client, err := server.Client(func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	uri := c.References()[0].URI
	if _, err := client.ReadResource(t.Context(), uri); err == nil || !strings.Contains(err.Error(), "limit_exceeded") {
		t.Fatal("escaped serialized response cap", err)
	}
	listed, err := client.ListResources(t.Context())
	if err != nil || len(listed.Resources) != 1 {
		t.Fatal(listed, err)
	}
	token = authority.token(t, "tenant", "actor", authority.cfg.MCPAudience(), "mcp.use")
	listed, err = client.ListResources(t.Context())
	if err != nil || len(listed.Resources) != 0 {
		t.Fatal("stale domain action in list", listed, err)
	}
	for _, target := range []string{uri, staticdocs.Namespace + "unknown/v9"} {
		if _, err := client.ReadResource(t.Context(), target); err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatal("native auth must precede content lookup", err)
		}
	}
	token = authority.token(t, "tenant", "actor", authority.cfg.MCPAudience(), "mcp.use", "fixture.read")
	authority.clock.Add(600)
	if _, err := client.ReadResource(t.Context(), uri); err == nil {
		t.Fatal("expired bearer accepted")
	}
}

func TestDocumentationRejectsDisjointCatalogBudgetMultiplication(t *testing.T) {
	// The reserved namespace has exactly one association, a stronger bound than
	// allowing per-tool catalogs. Exercise disjoint URIs at count/byte overflow.
	for _, tc := range []struct {
		name         string
		count, bytes int
	}{{"disjoint", 1, 20}, {"aggregate-count", staticdocs.MaxDocuments, 20}, {"aggregate-bytes", 4, staticdocs.MaxDocumentBytes}} {
		t.Run(tc.name, func(t *testing.T) {
			first := testRegistry(t, fixtureCall).bindings[0]
			first, err := WithDocumentation(first, fixtureDocumentation(t, "fixture.read", tc.count, tc.bytes))
			if err != nil {
				t.Fatal(err)
			}
			second := testRegistry(t, fixtureCall).bindings[0]
			second.name = "second"
			second.definition.ID = "second"
			second.resource = ""
			independent, err := staticdocs.New("fixture.read", []staticdocs.Document{{Reference: staticdocs.Reference{URI: staticdocs.Namespace + "independent/v1", Name: "Independent contract", Description: "Disjoint synthetic resource association for bounds testing.", MIMEType: staticdocs.MIME, Version: 1, DocumentRef: "docs/contracts/independent.md"}, Text: "independent document"}})
			if err != nil {
				t.Fatal(err)
			}
			second, err = WithDocumentation(second, independent)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := NewRegistry([]Binding{first, second}); !errors.Is(err, ErrRegistration) {
				t.Fatal("multiple catalogs amplified the resource budget", err)
			}
		})
	}
}
