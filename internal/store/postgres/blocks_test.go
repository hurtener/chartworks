package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestBlockGuardsRejectIncompleteEvidenceBeforeSQL(t *testing.T) {
	ctx := context.Background()
	// Nil transactions make any accidental SQL access fail the test.
	if _, err := blockGrants(identity.Envelope{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := blockTx(ctx, nil, identity.Envelope{}, "block", reporting.Reference{}, reporting.Read); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if err := insertBlockRevision(ctx, nil, identity.Envelope{}, reporting.Mutation{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	for _, mutation := range []reporting.Mutation{{}, {Topics: []reporting.TopicPin{{Topic: "topic"}}}, {Watch: []reporting.Dependency{{Source: "source"}}}} {
		if err := blockCurrentFence(ctx, nil, identity.Envelope{}, mutation); !errors.Is(err, store.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if err := verifyBlockAttempt(ctx, nil, identity.Envelope{}, reporting.Mutation{}, reporting.Snapshot{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestBlockConsistencyRequiresCompleteRevisionCoordinates(t *testing.T) {
	valid := reporting.Snapshot{State: reporting.State{ID: "block", DraftRevision: 2}, Revision: reporting.Revision{Number: 1, ID: "revision"}}
	if err := blockConsistency(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*reporting.Snapshot){
		func(s *reporting.Snapshot) { s.State.ID = "" },
		func(s *reporting.Snapshot) { s.Revision.Number = 0 },
		func(s *reporting.Snapshot) { s.Revision.ID = "" },
		func(s *reporting.Snapshot) { s.Revision.Number = 3 },
	} {
		bad := valid
		mutate(&bad)
		if err := blockConsistency(bad); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("invalid coordinates accepted: %+v %v", bad, err)
		}
	}
}

// Only the retained-record decoding seam is simulated here; SQL behavior is
// exercised by the real PostgreSQL acceptance tests.
type blockValidationRow struct {
	body []byte
	err  error
}

func (r blockValidationRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*[]byte) = r.body
	return nil
}

type blockValidationTx struct {
	pgx.Tx
	row pgx.Row
}

func (tx blockValidationTx) QueryRow(context.Context, string, ...any) pgx.Row { return tx.row }

func TestBlockFreshRejectsMissingOrMalformedRetainedValidation(t *testing.T) {
	marker := errors.New("retained record unavailable")
	for _, tc := range []struct {
		name string
		row  blockValidationRow
		want error
	}{
		{"query failure", blockValidationRow{err: marker}, marker},
		{"malformed record", blockValidationRow{body: []byte(`{"evidence":42}`)}, reporting.ErrStale},
		{"unhealthy snapshot", blockValidationRow{body: []byte(`{}`)}, reporting.ErrStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := blockFresh(context.Background(), blockValidationTx{row: tc.row}, identity.Envelope{}, reporting.Mutation{}, reporting.Snapshot{})
			if !errors.Is(err, tc.want) {
				t.Fatalf("retained evidence: %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBlockHealthRejectsUnencodableObservationBeforeSQL(t *testing.T) {
	observed := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := setBlockHealth(context.Background(), nil, "tenant", "block", 1, reporting.Health{ObservedAt: &observed}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
}
