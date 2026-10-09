package acceptance

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
)

func TestSourceCatalogPageNative(t *testing.T) {
	f := newSourceFixture(t, nil)
	ctx := t.Context()
	for _, id := range []string{"a", "Z", "z", "aa"} {
		f.create(t, id)
	}
	before := f.lookups.Load()
	registry, err := sourceapi.SourceRegistry(true, false)
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, sourceapi.Handler(f.token.verifier, f.s, nil, http.NotFoundHandler()))
	// The excluded a sorts before aa. Signed filtering must happen before LIMIT.
	token := f.token.sign(t, f.token.claims("source-a", "reader", []string{"sources.read", "cw.tenant.read:source-a", "cw.source.read:Z", "cw.source.read:aa", "cw.source.read:z"}), nil)
	var found []string
	e := f.token.envelope(t, "source-a", "reader", "sources.read", "cw.tenant.read:source-a", "cw.source.read:Z", "cw.source.read:aa", "cw.source.read:z")
	after := ""
	for {
		out, err := f.s.ListPage(ctx, e, sources.SourceListRequest{After: after, Limit: 1})
		if err != nil || len(out.Items) != 1 {
			t.Fatal(out, err)
		}
		found = append(found, out.Items[0].ID)
		if out.Next == "" {
			break
		}
		if out.Next <= after {
			t.Fatal("non-advancing cursor")
		}
		after = out.Next
	}
	if !reflect.DeepEqual(found, []string{"Z", "aa", "z"}) {
		t.Fatal("filter/cursor ordering", found)
	}
	for _, in := range []sources.SourceListRequest{{Limit: 0}, {Limit: 33}, {After: "../invalid", Limit: 1}} {
		if _, err := f.s.ListPage(ctx, e, in); err == nil {
			t.Fatal("invalid page admitted", in)
		}
	}
	foreign := f.token.envelope(t, "other", "reader", "sources.read", "cw.tenant.read:other", "cw.source.read:Z")
	if out, err := f.s.ListPage(ctx, foreign, sources.SourceListRequest{Limit: 32}); err != nil || len(out.Items) != 0 {
		t.Fatal("cross-tenant catalog", out, err)
	}
	if out, err := f.s.ListPage(ctx, e, sources.SourceListRequest{After: "z", Limit: 32}); err != nil || out.Items == nil || len(out.Items) != 0 || out.Next != "" {
		t.Fatal("terminal empty page", out, err)
	}
	response := callProtected(t, handler, "POST", "/v1/sources/list", token, `{"after":"Z","limit":1}`, map[string]string{"Content-Type": "application/json"})
	var page sources.SourcePage
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].ID != "aa" || page.Next != "aa" {
		t.Fatal("registered HTTP page", response.Code, response.Body.String())
	}
	if before != f.lookups.Load() {
		t.Fatal("catalog resolved warehouse credentials")
	}
}

func TestSourceCatalogSurfaces(t *testing.T) {
	f := newPhase23Fixture(t)
	beforeSource, beforeModel := f.domain.f.lookups.Load(), f.domain.model.requests.Load()
	for _, surface := range f.surfaces(t) {
		t.Run(surface.name, func(t *testing.T) {
			first := phase23Call[sources.SourcePage](t, surface, "listSourcePage", "", sources.SourceListRequest{Limit: 1}, nil)
			if len(first.Items) != 1 {
				t.Fatal("missing native catalog")
			}
			after := first.Items[0].ID
			next := phase23Call[sources.SourcePage](t, surface, "listSourcePage", "", sources.SourceListRequest{After: after, Limit: 32}, nil)
			for _, item := range next.Items {
				if item.ID <= after {
					t.Fatal("cursor ignored")
				}
			}
		})
	}
	if beforeSource != f.domain.f.lookups.Load() || beforeModel != f.domain.model.requests.Load() {
		t.Fatal("catalog executed source/model work")
	}
}
