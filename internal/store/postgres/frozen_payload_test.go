package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// This tests retained-record decoding only. The real SQL selection and privacy
// predicates remain covered by PostgreSQL acceptance tests.
type frozenPayloadRow struct {
	manifest []byte
	err      error
}

func (r frozenPayloadRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*[]byte) = r.manifest
	return nil
}

type frozenPayloadTx struct {
	pgx.Tx
	row pgx.Row
}

func (tx frozenPayloadTx) QueryRow(context.Context, string, ...any) pgx.Row { return tx.row }

func TestFrozenPayloadRejectsUnreadableManifest(t *testing.T) {
	marker := errors.New("synthetic retained payload read failure")
	for _, tc := range []struct {
		name string
		row  frozenPayloadRow
		want error
	}{
		{"storage failure", frozenPayloadRow{err: marker}, marker},
		{"malformed JSON", frozenPayloadRow{manifest: []byte(`{"version":`)}, store.ErrInvalid},
		{"wrong JSON shape", frozenPayloadRow{manifest: []byte(`[]`)}, store.ErrInvalid},
		{"missing manifest", frozenPayloadRow{manifest: []byte(`null`)}, store.ErrInvalid},
		{"unbound manifest", frozenPayloadRow{manifest: []byte(`{}`)}, store.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out reporting.RunRecord
			err := frozenValuesTx(context.Background(), frozenPayloadTx{row: tc.row}, identity.Envelope{}, frozenHead{}, &out)
			if !errors.Is(err, tc.want) {
				t.Fatalf("payload error %v; want %v", err, tc.want)
			}
			if out.Manifest != nil || out.Result != nil || len(out.Outputs) != 0 {
				t.Fatal("invalid payload exposed values")
			}
		})
	}
}
