package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

func TestDraftCatalogDeniesBeforeDatabase(t *testing.T) {
	db := &DB{}
	now := time.Now()
	envelope := func(scopes ...string) identity.Envelope {
		e, err := identity.FromVerified("tenant", "author", "session", scopes, now.Add(time.Hour), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	for _, tc := range []struct {
		e     identity.Envelope
		after string
		limit int
		want  error
	}{
		{e: identity.Envelope{}, limit: 10, want: access.ErrUnauthenticated},
		{e: envelope("reporting.read", "cw.report.read:report"), limit: 10, want: access.ErrForbidden},
		{e: envelope("reporting.write"), limit: 10, want: access.ErrNotFound},
		{e: envelope("reporting.write", "cw.report.write:*"), limit: 10, want: access.ErrForbidden},
		{e: envelope("reporting.write", "cw.report.write:report", "cw.source.read:*"), limit: 10, want: access.ErrForbidden},
		{e: envelope("reporting.write", "cw.report.write:report"), limit: 101, want: store.ErrInvalid},
		{e: envelope("reporting.write", "cw.report.write:report"), after: "../report", limit: 10, want: store.ErrInvalid},
	} {
		out, err := db.ListDocumentDrafts(t.Context(), tc.e, tc.after, tc.limit)
		if !errors.Is(err, tc.want) || len(out.Items) != 0 || out.Next != "" {
			t.Fatal("storage admitted unbounded authoring reach", out, err)
		}
	}
}
