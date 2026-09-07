package engineering

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
)

func TestUploadCellLimitAppliesAfterNormalization(t *testing.T) {
	column := UploadColumn{Name: "value", Type: "number"}
	value, err := normalizeCell(column, Cell{Text: "1e7"})
	if err != nil || len(value.Text) <= len("1e7") {
		t.Fatal("fixture did not exercise cell growth", err, value)
	}
	raw := []byte("value\n1e7\n")
	spec := UploadSpec{ID: "normalized-cell", Name: "Normalized cell", Connection: "workspace", Format: "csv", Columns: []UploadColumn{column}, Bytes: int64(len(raw)), SHA256: contentHash(raw)}
	limits := config.DefaultUploads()
	limits.MaxCellBytes = len(value.Text) - 1
	emitted := 0
	receipt, err := Parse(context.Background(), raw, spec, limits, func([]Cell) error {
		emitted++
		return nil
	})
	if !errors.Is(err, ErrLimit) || emitted != 0 || receipt.Rows != 0 {
		t.Fatal("oversized normalized cell reached the workspace consumer", err, receipt, emitted)
	}
	limits.MaxCellBytes = len(value.Text)
	receipt, err = Parse(context.Background(), raw, spec, limits, func(row []Cell) error {
		emitted++
		if len(row) != 1 || row[0] != value {
			t.Fatal("normalization changed the admitted value", row)
		}
		return nil
	})
	if err != nil || emitted != 1 || receipt.Rows != 1 || receipt.DecodedBytes != int64(len(value.Text)) {
		t.Fatal("exact normalized cell bound was rejected", err, receipt, emitted)
	}
}
