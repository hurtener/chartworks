package engineering

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	pqsnap "github.com/parquet-go/parquet-go/compress/snappy"
)

// Exercise each allocation boundary separately so a standard writer regression
// cannot be hidden by simply bypassing the aggregate preflight.
func TestParquetMetadataAndPageContracts(t *testing.T) {
	raw := parquetFixture(t, &pqsnap.Codec{})
	spec := parseSpec("parquet", raw, UploadColumn{"id", "integer", false}, UploadColumn{"name", "text", true}, UploadColumn{"active", "boolean", false}, UploadColumn{"ratio", "number", false})
	p := parser{ctx: context.Background(), spec: spec, limits: config.DefaultUploads(), emit: func([]Cell) error { return nil }}
	size := int(binary.LittleEndian.Uint32(raw[len(raw)-8 : len(raw)-4]))
	file, used, err := compact(raw[len(raw)-8-size : len(raw)-8])
	if err != nil || used != size {
		t.Fatal("bounded footer", err, used, size)
	}
	t.Log("file version", file.number(1), "rows", file.number(3), "root", file.fields[2].list[0])
	for i, node := range file.fields[2].list[1:] {
		t.Log("schema", i, node)
	}
	for _, group := range file.fields[4].list {
		for i, chunk := range group.fields[1].list {
			m := chunk.fields[3]
			start := m.number(9)
			if m.has(11) && m.number(11) < start {
				start = m.number(11)
			}
			c := parquetColumn{physical: int(m.number(1)), optional: i == 1}
			e := p.parquetPages(raw[start:start+m.number(7)], c, int(m.number(4)), m.number(5), m.number(6))
			if e != nil {
				h, n, _ := compact(raw[start:])
				body := raw[start+int64(n) : start+int64(n)+h.number(3)]
				v2 := h.fields[8]
				def := v2.number(5)
				data, de := boundedDecompress(int(m.number(4)), body[def:], int(h.number(2)-def))
				t.Log("encodings", m.fields[2], "definitions", body[:def], "decoded", data, "decode error", de)
				t.Errorf("column %d page bounds: %v", i, e)
			}
		}
	}
	if _, err = p.parquetPreflight(raw); err != nil {
		t.Fatalf("complete metadata preflight: %v", err)
	}
}
