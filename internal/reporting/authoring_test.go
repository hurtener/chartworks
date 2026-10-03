package reporting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

func authoringEnvelope(t *testing.T, scopes ...string) identity.Envelope {
	t.Helper()
	now := time.Now()
	e, err := identity.FromVerified("tenant", "author", "session", scopes, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAuthoringCapabilitiesRequireExactSignedReach(t *testing.T) {
	s := &Authoring{}
	for _, tc := range []struct {
		name                                                    string
		scopes                                                  []string
		builder, consumer, create, open, save, preview, execute bool
	}{
		{name: "actions alone", scopes: []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.execute"}},
		{name: "descriptive role", scopes: []string{"reporting.read", "admin", "builder"}},
		{name: "consumer", scopes: []string{"reporting.read", "cw.report.read:report"}, consumer: true},
		{name: "unrelated target", scopes: []string{"reporting.read", "reporting.write", "cw.report.write:other"}, builder: true},
		{name: "exact writer", scopes: []string{"reporting.read", "reporting.write", "cw.report.write:report"}, builder: true, save: true},
		{name: "writer with parent", scopes: []string{"reporting.read", "reporting.write", "cw.report.write:report", "cw.tenant.write:tenant"}, builder: true, create: true, save: true},
		{name: "full exact target", scopes: []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.execute", "cw.report.read:report", "cw.report.write:report", "cw.report.preview:report", "cw.report.execute:report", "cw.tenant.write:tenant"}, builder: true, consumer: true, create: true, open: true, save: true, preview: true, execute: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := s.Capabilities(t.Context(), authoringEnvelope(t, tc.scopes...), AuthoringCapabilitiesRequest{Report: "report"})
			if err != nil || out.Version != AuthoringVersion || out.Builder != tc.builder || out.Consumer != tc.consumer || out.CanCreate != tc.create || out.CanOpen != tc.open || out.CanSave != tc.save || out.CanPreview != tc.preview || out.CanExecute != tc.execute {
				t.Fatal(out, err)
			}
		})
	}
	e := authoringEnvelope(t, "reporting.read", "reporting.write", "cw.report.write:*", "cw.tenant.write:tenant")
	out, err := s.Capabilities(t.Context(), e, AuthoringCapabilitiesRequest{})
	if err != nil || out.Builder || out.CanCreate || out.CanSave || out.CanOpen || out.CanPreview {
		t.Fatal("empty target became an operation grant", out, err)
	}
	if _, err = s.Capabilities(t.Context(), e, AuthoringCapabilitiesRequest{Report: "*"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("wildcard target admitted", err)
	}
	if _, err = s.Capabilities(t.Context(), identity.Envelope{}, AuthoringCapabilitiesRequest{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = s.Capabilities(t.Context(), authoringEnvelope(t, "reporting.write", "cw.report.write:report"), AuthoringCapabilitiesRequest{}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = s.Capabilities(ctx, e, AuthoringCapabilitiesRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestAuthoringDenialPrecedesRepositoryAccess(t *testing.T) {
	// Deliberately absent dependencies make any premature I/O panic.
	s := &Authoring{}
	reader := authoringEnvelope(t, "reporting.read", "cw.report.read:report")
	if _, err := s.Create(t.Context(), reader, AuthoringCreateRequest{ID: "report", Definition: documentFixture()}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	writer := authoringEnvelope(t, "reporting.read", "reporting.write", "cw.report.write:report")
	if _, err := s.Create(t.Context(), writer, AuthoringCreateRequest{ID: "report", Definition: documentFixture()}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("target write substituted for tenant creation authority", err)
	}
	if _, err := s.Read(t.Context(), reader, AuthoringReadRequest{Report: "report"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Drafts(t.Context(), reader, DraftListRequest{Limit: 10}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Preview(t.Context(), reader, AuthoringPreviewRequest{Report: "report", Revision: 1}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	for _, revision := range []int64{0, -1, 257} {
		if _, err := s.Save(t.Context(), writer, AuthoringSaveRequest{Report: "report", Revision: revision, ExpectedVersion: 1}); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := s.Preview(t.Context(), writer, AuthoringPreviewRequest{Report: "report", Revision: revision}); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, err := NewAuthoring(nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestAuthoringManualLaneRejectsModelIntentBeforeIO(t *testing.T) {
	s := &Authoring{}
	e := authoringEnvelope(t, "reporting.read", "reporting.write", "cw.tenant.write:tenant", "cw.report.write:report")
	for _, w := range []Widget{
		{ID: "live", Kind: "query", Query: &QueryWidget{Durability: "replayable", Question: "Synthetic question"}},
		{ID: "narrative", Kind: "block", Block: &BlockWidget{Block: "block", Narrative: true}},
		{ID: "disguised", Kind: "text", Query: &QueryWidget{Durability: "replayable"}},
	} {
		d := documentFixture()
		d.Widgets = []Widget{w}
		if _, err := s.Create(t.Context(), e, AuthoringCreateRequest{ID: "report", Definition: d}); !errors.Is(err, ErrInvalid) {
			t.Fatal("manual creation admitted model intent", err)
		}
		if _, err := s.Save(t.Context(), e, AuthoringSaveRequest{Report: "report", Revision: 1, ExpectedVersion: 1, Definition: d}); !errors.Is(err, ErrInvalid) {
			t.Fatal("manual save admitted model intent", err)
		}
	}
	broad := authoringEnvelope(t, "reporting.read", "reporting.write", "cw.tenant.write:tenant", "cw.report.write:*")
	if _, err := s.Create(t.Context(), broad, AuthoringCreateRequest{ID: "report", Definition: documentFixture()}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("broad editing authority accepted", err)
	}
	if _, err := s.Read(t.Context(), broad, AuthoringReadRequest{Report: "report", Revision: 1}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Preview(t.Context(), broad, AuthoringPreviewRequest{Report: "report", Revision: 1}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Execute(t.Context(), broad, AuthoringExecuteRequest{Run: "run"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
}
