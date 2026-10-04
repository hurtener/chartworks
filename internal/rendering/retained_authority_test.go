package rendering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

type authorityViewerFunc func(context.Context, identity.Envelope, reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error)

func (f authorityViewerFunc) View(ctx context.Context, e identity.Envelope, in reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error) {
	return f(ctx, e, in)
}

type noRetainedRender struct{ calls int }

func (p *noRetainedRender) Process(context.Context, SealedWork) (Rendition, error) {
	p.calls++
	return Rendition{}, ErrWorker
}

func TestFullRenditionReauthorizesEveryRetainedWidget(t *testing.T) {
	root := reporting.DeliveryViewResult{Summary: reporting.DeliveryRunSummary{Run: "run", Kind: "report"}, Pages: []reporting.CompositionPageSummary{
		{ID: "one", Widgets: []reporting.CompositionWidgetSummary{{ID: "text", State: "completed", Kind: "text"}}},
		{ID: "two", Widgets: []reporting.CompositionWidgetSummary{{ID: "table", State: "completed", Kind: "block"}}},
	}}
	note := reporting.TextWidget{Format: "plain", Text: "retained note"}
	// Independent reconstruction of the pre-existing provenance contract, so
	// this regression also checks compatibility with an already stored record.
	provenance := sha256.New()
	rootWire, _ := json.Marshal(struct {
		Summary reporting.DeliveryRunSummary
		Pages   []reporting.CompositionPageSummary
	}{root.Summary, root.Pages})
	_, _ = provenance.Write(rootWire)
	noteSum := sha256.Sum256([]byte(note.Format + "\x00" + note.Text))
	_, _ = provenance.Write(noteSum[:])
	_, _ = provenance.Write([]byte(strings.Repeat("a", 64)))
	source := hex.EncodeToString(provenance.Sum(nil))
	request := Request{View: reporting.DeliveryViewRequest{Kind: "report", Run: "run", Limit: 2}, Full: true, Format: "html", Theme: "light", Width: 800, Height: 420}
	actor := authority(t, "reporting.read", "reporting.export", "cw.execution_context.use:one", "cw.execution_context.use:two")
	narrow := authority(t, "reporting.read", "reporting.export", "cw.execution_context.use:one")
	for _, mode := range []string{"child-denial", "redacted-root", "shrunk-root", "changed-child"} {
		t.Run(mode, func(t *testing.T) {
			changed := false
			calls := []string{}
			viewer := authorityViewerFunc(func(_ context.Context, e identity.Envelope, in reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error) {
				calls = append(calls, in.Page+"/"+in.Widget)
				if in.Widget == "" {
					current := root
					if changed && (mode == "redacted-root" || mode == "shrunk-root") {
						current.Pages = current.Pages[:1]
						current.Redacted = mode == "redacted-root"
					}
					return current, nil
				}
				if !e.Has("cw.execution_context.use:" + in.Page) {
					return reporting.DeliveryViewResult{}, access.ErrNotFound
				}
				if in.Widget == "text" {
					return reporting.DeliveryViewResult{Text: &note}, nil
				}
				view := tableView()
				if changed && mode == "changed-child" {
					view.Output.RetainedDigest = strings.Repeat("b", 64)
				}
				return view, nil
			})
			repo := NewMemoryRepository()
			s, err := newTestService(viewer, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			processor := &noRetainedRender{}
			s.repository, s.processor = repo, processor
			record, err := repo.PutRendition(t.Context(), Record{Tenant: actor.Tenant(), Actor: actor.User(), Session: actor.Session(), Request: request, Rendition: Rendition{ID: "saved-full", SourceDigest: source, Content: "previously rendered complete bytes", ExpiresAt: time.Now().Add(time.Hour)}})
			if err != nil {
				t.Fatal(err)
			}
			if read, err := s.Read(t.Context(), actor, ReadRequest{ID: record.Rendition.ID}); err != nil || read.Content != record.Rendition.Content {
				t.Fatal("fully authorized historical record", err)
			}
			initialReads := strings.Join(calls, ",")
			if listed, err := s.List(t.Context(), actor, ListRequest{Limit: 10}); err != nil || len(listed.Items) != 1 || listed.Items[0].Content != record.Rendition.Content {
				t.Fatal("authorized list includes retained bytes", listed, err)
			}
			changed = true
			reader := actor
			if mode == "child-denial" {
				reader = narrow
			}
			if read, err := s.Read(t.Context(), reader, ReadRequest{ID: record.Rendition.ID}); !errors.Is(err, access.ErrNotFound) || read.Content != "" {
				t.Fatal("incomplete current authority exposed saved bytes", read, err)
			}
			if listed, err := s.List(t.Context(), reader, ListRequest{Limit: 10}); err != nil || len(listed.Items) != 0 {
				t.Fatal("list exposed unavailable record or bytes", listed, err)
			}
			if mode == "redacted-root" {
				for _, call := range []func(context.Context, identity.Envelope, Request) (Rendition, error){s.Export, s.Generate} {
					if output, err := call(t.Context(), actor, request); !errors.Is(err, access.ErrNotFound) || output.Content != "" {
						t.Fatal("redacted metadata authorized a full render", err)
					}
				}
			}
			if processor.calls != 0 {
				t.Fatal("retained authorization invoked rendering", processor.calls)
			}
			if initialReads != "/,one/text,two/table" {
				t.Fatal("did not recheck each exact widget", initialReads)
			}
		})
	}
}

func TestSingleRenditionDoesNotSwitchDefaultChild(t *testing.T) {
	viewer := &fixtureViewer{value: tableView()}
	s, err := newTestService(viewer, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	processor := &noRetainedRender{}
	s.processor = processor
	actor := authority(t, "reporting.read")
	record, err := s.repository.PutRendition(t.Context(), Record{Tenant: actor.Tenant(), Request: Request{View: reporting.DeliveryViewRequest{Kind: "report", Run: "run"}}, Rendition: Rendition{ID: "saved-child", SourceDigest: strings.Repeat("a", 64), Content: "first child bytes", ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(t.Context(), actor, ReadRequest{ID: record.Rendition.ID}); err != nil {
		t.Fatal(err)
	}
	// A newly selected visible page can contain identical retained values. Its
	// successful read and equal digest do not authorize the previous page.
	viewer.value.Redacted = true
	if output, err := s.Read(t.Context(), actor, ReadRequest{ID: record.Rendition.ID}); !errors.Is(err, access.ErrNotFound) || output.Content != "" {
		t.Fatal("same-digest default child bypassed revoked page reach", err)
	}
	if listed, err := s.List(t.Context(), actor, ListRequest{Limit: 10}); err != nil || len(listed.Items) != 0 {
		t.Fatal("same-digest replacement exposed saved bytes through list", listed, err)
	}
	explicit := record
	explicit.Rendition.ID = "saved-exact-child"
	explicit.Request.View.Page, explicit.Request.View.Widget = "authorized-page", "authorized-widget"
	if _, err := s.repository.PutRendition(t.Context(), explicit); err != nil {
		t.Fatal(err)
	}
	if output, err := s.Read(t.Context(), actor, ReadRequest{ID: explicit.Rendition.ID}); err != nil || output.Content != explicit.Rendition.Content {
		t.Fatal("unrelated page redaction denied exact authorized child", err)
	}
	viewer.value.Redacted = false
	viewer.value.Output.RetainedDigest = strings.Repeat("b", 64)
	if output, err := s.Read(t.Context(), actor, ReadRequest{ID: record.Rendition.ID}); !errors.Is(err, access.ErrNotFound) || output.Content != "" {
		t.Fatal("default child switched across retained authorization", err)
	}
	if listed, err := s.List(t.Context(), actor, ListRequest{Limit: 10}); err != nil || len(listed.Items) != 0 || processor.calls != 0 {
		t.Fatal("single output list re-rendered or exposed replaced projection", listed, err)
	}
}
