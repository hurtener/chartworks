package staticdocs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

func fixtureDocument() Document {
	return Document{Reference: Reference{URI: Namespace + "workflow/v1", Name: "Synthetic workflow", Description: "A synthetic public contract used only for testing.", MIMEType: MIME, Version: 1, DocumentRef: "docs/contracts/synthetic-v1.md"}, Text: "# Synthetic workflow\nNo tenant data.\n"}
}
func fixtureAuthority(t *testing.T, now func() time.Time, scopes ...string) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "actor", "session", scopes, now().Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestCatalogImmutableBoundedAndAuthenticated(t *testing.T) {
	input := []Document{fixtureDocument()}
	catalog, err := New("reporting.read", input)
	if err != nil {
		t.Fatal(err)
	}
	input[0].Text = "mutated"
	refs := catalog.References()
	refs[0].URI = "mutated"
	e := fixtureAuthority(t, time.Now, "reporting.read")
	out, err := catalog.Read(t.Context(), e, fixtureDocument().Reference.URI)
	digest := sha256.Sum256([]byte(fixtureDocument().Text))
	if err != nil || out.Text != fixtureDocument().Text || out.Reference.SHA256 != hex.EncodeToString(digest[:]) || out.Reference.Bytes != len(out.Text) {
		t.Fatal(out, err)
	}
	out.Text = "mutated response"
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			d, err := catalog.Read(t.Context(), e, fixtureDocument().Reference.URI)
			if err != nil || d.Text != fixtureDocument().Text {
				t.Error(d, err)
			}
		})
	}
	wg.Wait()
	for _, uri := range []string{"", "docs/contracts/synthetic-v1.md", Namespace + "workflow/v2", Namespace + "workflow/v01", Namespace + "workflow/v1?x=1", Namespace + "workflow/v1#fragment", Namespace + "%77orkflow/v1", Namespace + "../workflow/v1", "https://example.invalid"} {
		if _, err := catalog.Read(t.Context(), e, uri); !errors.Is(err, access.ErrNotFound) {
			t.Fatal(uri, err)
		}
		if _, err := catalog.Read(t.Context(), fixtureAuthority(t, time.Now, "reporting.write"), uri); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("lookup preceded native authority", uri, err)
		}
	}
	if _, err := catalog.Read(t.Context(), identity.Envelope{}, fixtureDocument().Reference.URI); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := catalog.Read(nil, e, fixtureDocument().Reference.URI); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := catalog.Read(ctx, e, fixtureDocument().Reference.URI); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	clock := time.Now()
	expired := fixtureAuthority(t, func() time.Time { return clock }, "reporting.read")
	clock = clock.Add(2 * time.Hour)
	if _, err := catalog.Read(t.Context(), expired, fixtureDocument().Reference.URI); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
}
func TestCatalogRejectsMalformedVersionsDuplicatesAndAggregate(t *testing.T) {
	for _, uri := range []string{Namespace + "workflow/v2", Namespace + "workflow/v01", Namespace + "workflow/v1?", Namespace + "workflow/v1#", Namespace + "../v1", Namespace + "%77orkflow/v1", "chartworks://other/docs/workflow/v1", Namespace + "workflow/v1/extra"} {
		d := fixtureDocument()
		d.Reference.URI = uri
		if _, err := New("reporting.read", []Document{d}); err == nil {
			t.Fatal(uri)
		}
	}
	for _, edit := range []func(*Document){func(d *Document) { d.Reference.Version = 0 }, func(d *Document) { d.Reference.MIMEType = "text/html" }, func(d *Document) { d.Reference.DocumentRef = "docs/../AGENTS.md" }, func(d *Document) { d.Text = strings.Repeat("x", MaxDocumentBytes+1) }, func(d *Document) { d.Text = "\xff" }, func(d *Document) { d.Reference.Name = "name\n" }} {
		d := fixtureDocument()
		edit(&d)
		if _, err := New("reporting.read", []Document{d}); err == nil {
			t.Fatal("invalid document admitted")
		}
	}
	if _, err := New("reporting.read", []Document{fixtureDocument(), fixtureDocument()}); err == nil {
		t.Fatal("duplicate admitted")
	}
	for _, n := range []int{MaxDocuments + 1, 5} {
		ds := make([]Document, n)
		for i := range ds {
			ds[i] = fixtureDocument()
			ds[i].Reference.URI = fmt.Sprintf("%sdoc%d/v1", Namespace, i)
			if n == 5 {
				ds[i].Text = strings.Repeat("x", MaxDocumentBytes)
			}
		}
		if _, err := New("reporting.read", ds); err == nil {
			t.Fatal("catalog bound ignored", n)
		}
	}
}
