package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/jackc/pgx/v5"
)

type proposalSourceRow struct {
	revision  int64
	contextID string
	err       error
}

func (r proposalSourceRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*int64) = r.revision
	*dest[1].(*string) = r.contextID
	return nil
}

type proposalSourceTx struct {
	pgx.Tx
	row pgx.Row
}

func (tx proposalSourceTx) QueryRow(context.Context, string, ...any) pgx.Row { return tx.row }

func TestProposalSourceFenceRejectsChangedPartition(t *testing.T) {
	marker := errors.New("synthetic source metadata failure")
	material := engineering.ProposalMaterial{Binding: readexec.Binding{Source: "source", Revision: 2, Context: "context"}}
	for _, tc := range []struct {
		name string
		row  proposalSourceRow
		want error
	}{
		{"read failure", proposalSourceRow{err: marker}, marker},
		{"removed source", proposalSourceRow{err: pgx.ErrNoRows}, pgx.ErrNoRows},
		{"new revision", proposalSourceRow{revision: 3, contextID: "context"}, engineering.ErrProposalDrift},
		{"changed context", proposalSourceRow{revision: 2, contextID: "other-context"}, engineering.ErrProposalDrift},
		{"exact pin", proposalSourceRow{revision: 2, contextID: "context"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := proposalSourceFence(context.Background(), proposalSourceTx{row: tc.row}, identity.Envelope{}, material)
			if !errors.Is(err, tc.want) {
				t.Fatalf("source pin %v; want %v", err, tc.want)
			}
		})
	}
}
