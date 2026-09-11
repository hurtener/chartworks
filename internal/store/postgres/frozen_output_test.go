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
	"github.com/jackc/pgx/v5/pgconn"
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

func TestFrozenNarrativeReservationRejectsExhaustedBudget(t *testing.T) {
	m := reporting.RunManifest{Outputs: []reporting.Output{{ID: "summary", Kind: "narrative", Narrative: &reporting.Narrative{MaxCalls: 1, MaxTokens: 256}}}}
	m.Limits.NarrativeCalls, m.Limits.NarrativeTokens = 1, 256
	o := reporting.RetainedOutput{ID: "summary", Kind: "narrative", State: "indeterminate", Code: "narrative_indeterminate", ReservedCalls: 1, ReservedTokens: 256}
	for _, tc := range []struct {
		name          string
		calls, tokens int
	}{
		{"calls exhausted", 1, 0},
		{"tokens exhausted", 0, 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := frozenHead{view: reporting.RunView{State: "normalized", ReservedCalls: tc.calls, ReservedTokens: tc.tokens}}
			err := frozenOutputTx(context.Background(), retainedOutputTx{row: frozenOutputRow{err: pgx.ErrNoRows}}, identity.Envelope{}, h, reporting.RunWrite{Manifest: m, Output: &o, Kind: "output_start"})
			if !errors.Is(err, reporting.ErrBudget) {
				t.Fatal("exhausted narrative budget", err)
			}
		})
	}
}

type failingOutputWriteTx struct {
	retainedOutputTx
	failure error
	calls   int
}

func (tx *failingOutputWriteTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.calls++
	return pgconn.CommandTag{}, tx.failure
}

func TestFrozenOutputStopsAfterAccountingWriteFailure(t *testing.T) {
	marker := errors.New("synthetic accounting write failure")
	for _, narrative := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained bytes", true: "narrative reservation"}[narrative], func(t *testing.T) {
			output := reporting.RetainedOutput{ID: "output", Kind: "table", State: "failed", Code: "output_failed"}
			manifest := reporting.RunManifest{Outputs: []reporting.Output{{ID: "output", Kind: "table"}}}
			kind := "output"
			if narrative {
				kind = "output_start"
				output.Kind, output.State, output.Code = "narrative", "indeterminate", "narrative_indeterminate"
				output.ReservedCalls, output.ReservedTokens = 1, 256
				manifest.Outputs[0].Kind = "narrative"
				manifest.Outputs[0].Narrative = &reporting.Narrative{MaxCalls: 1, MaxTokens: 256}
				manifest.Limits.NarrativeCalls, manifest.Limits.NarrativeTokens = 1, 256
			} else {
				output.Digest = output.ContentDigest()
			}
			tx := &failingOutputWriteTx{retainedOutputTx: retainedOutputTx{row: frozenOutputRow{err: pgx.ErrNoRows}}, failure: marker}
			err := frozenOutputTx(context.Background(), tx, identity.Envelope{}, frozenHead{maximum: 1 << 20, view: reporting.RunView{State: "normalized"}}, reporting.RunWrite{Manifest: manifest, Output: &output, Kind: kind})
			if !errors.Is(err, marker) || tx.calls != 1 {
				t.Fatal("output proceeded past failed accounting", err, tx.calls)
			}
		})
	}
}
