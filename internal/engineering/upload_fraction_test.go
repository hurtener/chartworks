package engineering

import (
	"errors"
	"strings"
	"testing"
)

func TestUploadTimestampFractionBeyondParserPrecision(t *testing.T) {
	column := UploadColumn{Type: "timestamp"}
	for _, input := range []string{
		"2026-01-02T03:04:05.1234560001Z",
		"2026-01-02T03:04:05.0000000001Z",
		"2026-01-02T03:04:05,1234560001Z",
		"2026-01-02T03:04:05.123456" + strings.Repeat("0", 100) + "1Z",
	} {
		if _, err := normalizeCell(column, Cell{Text: input}); !errors.Is(err, ErrFormat) {
			t.Fatalf("fraction silently lost nonzero precision: %q (%v)", input, err)
		}
	}
	for _, input := range []string{
		"2026-01-02T03:04:05.1234560000Z",
		"2026-01-02T03:04:05.123456" + strings.Repeat("0", 100) + "Z",
	} {
		cell, err := normalizeCell(column, Cell{Text: input})
		if err != nil || cell.Null || cell.Text != "2026-01-02T03:04:05.123456Z" {
			t.Fatal("exact trailing zeros changed the timestamp", err, cell)
		}
	}
}
