package rendering

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/reporting"
)

func TestWorkerResponseIntegrityRejectsEverySealedMismatch(t *testing.T) {
	view := tableView()
	request := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "table", Limit: 100}, Format: "html", Theme: "light", Width: 800, Height: 400}
	good, err := renderSealed(request, view, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	work := SealedWork{Version: WorkerProtocolVersion, Request: request, View: view}
	work.Digest = sealedDigest(work)
	if err := validateWorkerRendition(work, good); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*Rendition){func(r *Rendition) { r.Version = "spoof" }, func(r *Rendition) { r.Format = "svg" }, func(r *Rendition) { r.MediaType = "text/plain" }, func(r *Rendition) { r.Width++ }, func(r *Rendition) { r.Height++ }, func(r *Rendition) { r.Theme = "dark" }, func(r *Rendition) { r.SourceDigest = "spoof" }, func(r *Rendition) { r.Projection.Digest = "spoof" }, func(r *Rendition) { r.Digest = "spoof" }, func(r *Rendition) { r.Content += "<script>bad</script>"; r.Bytes = len(r.Content) }}
	for i, mutate := range mutations {
		bad := good
		mutate(&bad)
		if !errors.Is(validateWorkerRendition(work, bad), ErrWorker) {
			t.Fatalf("mutation %d accepted", i)
		}
	}
}

func TestWorkerExecutableRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "worker")
	if err := os.WriteFile(real, []byte("x"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	o := Options{WorkerVersion: "v", ThemeVersion: "t", MaxTime: 1e9, MaxMemoryBytes: 64 << 20, MaxInputBytes: 1 << 20, MaxOutputBytes: 1 << 20, MaxConcurrent: 1, MaxWidgets: 1, Retention: 6e10, Isolation: "development"}
	if _, err := NewProcess(link, o); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestExpiredDeterministicRenditionCanRegenerate(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now()
	old := Record{Tenant: "t", Rendition: Rendition{ID: "rnd-same", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour), Digest: "old"}}
	if _, err := repo.PutRendition(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	fresh := old
	fresh.Rendition.CreatedAt = now
	fresh.Rendition.ExpiresAt = now.Add(time.Hour)
	fresh.Rendition.Digest = "fresh"
	got, err := repo.PutRendition(t.Context(), fresh)
	if err != nil || got.Rendition.Digest != "fresh" {
		t.Fatal(got.Rendition.Digest, err)
	}
}

func TestStaticParserRejectsObfuscatedExecutableMarkup(t *testing.T) {
	view := tableView()
	for _, format := range []string{"html", "svg"} {
		request := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "table", Limit: 100}, Format: format, Theme: "light", Width: 800, Height: 400}
		out, err := renderSealed(request, view, 1<<20)
		if err != nil || !safeStatic(format, out.Content) {
			t.Fatalf("generated %s rejected: %v", format, err)
		}
	}
	hostile := []string{
		`<!doctype html><html><head></head><body onload = "go()"></body></html>`,
		`<!doctype html><HTML><head></head><body ONLOAD\t=\t"go()"></body></HTML>`,
		`<!doctype html><html><head></head><body o&#x6e;load="go()"></body></html>`,
		`<!doctype html><html><head></head><body><ScRiPt>go()</ScRiPt></body></html>`,
		`<!doctype html><html><head></head><body><span style="background:u&#114;l(https://evil.invalid)">x</span></body></html>`,
		`<!doctype html><html><head></head><body><svg><text onclick &#x3d; "go()">x</text></svg></body></html>`,
	}
	for i, content := range hostile {
		if safeStatic("html", content) {
			t.Fatalf("hostile HTML %d accepted", i)
		}
	}
	for i, content := range []string{`<svg xmlns="http://www.w3.org/2000/svg" onload = "go()"></svg>`, `<SVG xmlns="http://www.w3.org/2000/svg"><script>go()</script></SVG>`, `<svg xmlns="http://www.w3.org/2000/svg"><text style="fill:url(&#x68;ttps://evil.invalid)">x</text></svg>`, `<svg xmlns="http://www.w3.org/2000/svg"><text href="&#x68;ttps://evil.invalid">x</text></svg>`} {
		if safeStatic("svg", content) {
			t.Fatalf("hostile SVG %d accepted", i)
		}
	}
}
