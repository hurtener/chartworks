package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

type frozenOutputRow struct {
	state   string
	payload []byte
	err     error
}

func (r frozenOutputRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*string) = r.state
	*dest[1].(*[]byte) = r.payload
	return nil
}

type retainedOutputTx struct {
	pgx.Tx
	row pgx.Row
}

func (tx retainedOutputTx) QueryRow(context.Context, string, ...any) pgx.Row { return tx.row }

// Only decoding and immutable checkpoint replay are simulated. Unexpected SQL
// writes panic through the absent transaction implementation.
func TestFrozenOutputReplayRejectsChangedOrCorruptCheckpoint(t *testing.T) {
	output := reporting.RetainedOutput{ID: "table", Kind: "table", State: "failed", Code: "output_failed"}
	output.Digest = output.ContentDigest()
	same, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	changed := output
	changed.Code = "narrative_failed"
	changed.Digest = changed.ContentDigest()
	different, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	marker := errors.New("synthetic checkpoint read failure")
	for _, tc := range []struct {
		name string
		row  frozenOutputRow
		want error
	}{
		{"storage failure", frozenOutputRow{err: marker}, marker},
		{"corrupt checkpoint", frozenOutputRow{state: "failed", payload: []byte(`{`)}, store.ErrInvalid},
		{"changed completed output", frozenOutputRow{state: "failed", payload: different}, store.ErrConflict},
		{"identical completed output", frozenOutputRow{state: "failed", payload: same}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := reporting.RunManifest{Outputs: []reporting.Output{{ID: "table", Kind: "table"}}}
			err := frozenOutputTx(context.Background(), retainedOutputTx{row: tc.row}, identity.Envelope{}, frozenHead{view: reporting.RunView{State: "normalized"}}, reporting.RunWrite{Manifest: manifest, Output: &output, Kind: "output"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("checkpoint replay %v; want %v", err, tc.want)
			}
		})
	}
}

func TestFrozenNarrativeCheckpointPreservesReservation(t *testing.T) {
	m := reporting.RunManifest{Outputs: []reporting.Output{{ID: "summary", Kind: "narrative", Narrative: &reporting.Narrative{MaxCalls: 1, MaxTokens: 256}}}}
	started := reporting.RetainedOutput{ID: "summary", Kind: "narrative", State: "indeterminate", Code: "narrative_indeterminate", ReservedCalls: 1, ReservedTokens: 256}
	prior, err := json.Marshal(started)
	if err != nil {
		t.Fatal(err)
	}
	finished := started
	finished.State, finished.Code = "failed", "narrative_failed"
	finished.ReservedTokens = 128
	finished.Digest = finished.ContentDigest()
	for _, tc := range []struct {
		name   string
		kind   string
		row    frozenOutputRow
		output reporting.RetainedOutput
		want   error
	}{
		{"duplicate reservation", "output_start", frozenOutputRow{state: "indeterminate", payload: prior}, started, store.ErrConflict},
		{"changed paid reservation", "output", frozenOutputRow{state: "indeterminate", payload: prior}, finished, store.ErrInvalid},
		{"completion without reservation", "output", frozenOutputRow{err: pgx.ErrNoRows}, finished, store.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := frozenOutputTx(context.Background(), retainedOutputTx{row: tc.row}, identity.Envelope{}, frozenHead{view: reporting.RunView{State: "normalized"}}, reporting.RunWrite{Manifest: m, Output: &tc.output, Kind: tc.kind})
			if !errors.Is(err, tc.want) {
				t.Fatalf("narrative reservation %v; want %v", err, tc.want)
			}
		})
	}
}
