package engineering

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"math/bits"
	"sort"

	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zstd"
)

// Compact Thrift is preflighted BEFORE the general Parquet decoder can allocate
// from attacker-supplied lengths. This deliberately qualifies a flat, scalar
// subset rather than accepting arbitrary nested schemas or external chunks.
type compactValue struct {
	kind   byte
	n      int64
	b      []byte
	list   []compactValue
	fields map[int]compactValue
}
type compactReader struct {
	data      []byte
	at, nodes int
}

func (r *compactReader) take(n int) ([]byte, error) {
	if n < 0 || n > len(r.data)-r.at {
		return nil, ErrFormat
	}
	b := r.data[r.at : r.at+n]
	r.at += n
	return b, nil
}
func (r *compactReader) unsigned() (uint64, error) {
	var out uint64
	for i := 0; i < 10; i++ {
		b, e := r.take(1)
		if e != nil {
			return 0, e
		}
		if i == 9 && b[0] > 1 {
			return 0, ErrFormat
		}
		out |= uint64(b[0]&127) << uint(i*7)
		if b[0]&128 == 0 {
			return out, nil
		}
	}
	return 0, ErrFormat
}
func (r *compactReader) value(kind byte, depth int, element bool) (compactValue, error) {
	r.nodes++
	v := compactValue{kind: kind}
	if depth > 16 || r.nodes > 65536 {
		return v, ErrLimit
	}
	switch kind {
	case 1, 2:
		if element {
			b, e := r.take(1)
			if e != nil || (b[0] != 1 && b[0] != 2) {
				return v, ErrFormat
			}
			kind = b[0]
		}
		if kind == 1 {
			v.n = 1
		}
	case 3:
		b, e := r.take(1)
		if e != nil {
			return v, e
		}
		v.n = int64(int8(b[0]))
	case 4, 5, 6:
		n, e := r.unsigned()
		if e != nil {
			return v, e
		}
		v.n = int64(n>>1) ^ -int64(n&1)
		if kind == 4 && (v.n < math.MinInt16 || v.n > math.MaxInt16) || kind == 5 && (v.n < math.MinInt32 || v.n > math.MaxInt32) {
			return v, ErrFormat
		}
	case 7:
		b, e := r.take(8)
		if e != nil {
			return v, e
		}
		v.b = b
	case 8:
		n, e := r.unsigned()
		if e != nil || n > 65536 {
			return v, ErrLimit
		}
		v.b, e = r.take(int(n))
		if e != nil {
			return v, e
		}
	case 9, 10:
		h, e := r.take(1)
		if e != nil {
			return v, e
		}
		count := uint64(h[0] >> 4)
		if count == 15 {
			count, e = r.unsigned()
			if e != nil {
				return v, e
			}
		}
		if count > 8192 || int(count) > 65536-r.nodes {
			return v, ErrLimit
		}
		v.list = make([]compactValue, 0, int(count))
		for i := uint64(0); i < count; i++ {
			item, e := r.value(h[0]&15, depth+1, true)
			if e != nil {
				return v, e
			}
			v.list = append(v.list, item)
		}
	case 12:
		v.fields = map[int]compactValue{}
		last := 0
		for {
			h, e := r.take(1)
			if e != nil {
				return v, e
			}
			if h[0] == 0 {
				break
			}
			id := last + int(h[0]>>4)
			if h[0]>>4 == 0 {
				n, e := r.unsigned()
				if e != nil {
					return v, e
				}
				id = int(int64(n>>1) ^ -int64(n&1))
			}
			if id < 1 || id > 32767 || len(v.fields) >= 64 {
				return v, ErrFormat
			}
			if _, ok := v.fields[id]; ok {
				return v, ErrFormat
			}
			item, e := r.value(h[0]&15, depth+1, false)
			if e != nil {
				return v, e
			}
			v.fields[id] = item
			last = id
		}
	default:
		return v, ErrFormat
	}
	return v, nil
}
func compact(data []byte) (compactValue, int, error) {
	r := compactReader{data: data}
	v, e := r.value(12, 0, false)
	return v, r.at, e
}
func (v compactValue) integer(id int) (int64, bool) {
	f, ok := v.fields[id]
	return f.n, ok && (f.kind == 4 || f.kind == 5 || f.kind == 6)
}
func (v compactValue) number(id int) int64 { n, _ := v.integer(id); return n }
func (v compactValue) has(id int) bool     { _, ok := v.fields[id]; return ok }

type parquetColumn struct {
	physical  int
	width     int
	optional  bool
	scale     int
	decimal   bool
	date      bool
	timestamp int
	name      string
}
type parquetSpan struct{ start, end int64 }

func (p *parser) parquetPreflight(raw []byte) ([]parquetColumn, error) {
	if len(raw) < 12 || string(raw[:4]) != "PAR1" || string(raw[len(raw)-4:]) != "PAR1" {
		return nil, ErrFormat
	}
	footerSize := int64(binary.LittleEndian.Uint32(raw[len(raw)-8 : len(raw)-4]))
	footerAt := int64(len(raw)) - 8 - footerSize
	if footerSize < 1 || footerSize > 1<<20 || footerAt < 4 {
		return nil, ErrLimit
	}
	file, used, err := compact(raw[footerAt : int64(len(raw))-8])
	if err != nil {
		return nil, err
	}
	if int64(used) != footerSize || file.number(1) != 1 || file.has(8) || file.has(9) {
		return nil, ErrFormat
	}
	rows, ok := file.integer(3)
	if !ok || rows < 0 || rows > int64(p.limits.MaxRows) || rows*int64(len(p.spec.Columns)) > p.limits.MaxCells {
		return nil, ErrLimit
	}
	schema := file.fields[2].list
	if len(schema) != len(p.spec.Columns)+1 || schema[0].number(5) != int64(len(p.spec.Columns)) {
		return nil, ErrFormat
	}
	columns := make([]parquetColumn, len(p.spec.Columns))
	headers := make([]string, len(columns))
	for i, node := range schema[1:] {
		physical, ok := node.integer(1)
		repetition, rok := node.integer(3)
		if !ok || !rok || physical < 0 || physical > 7 || physical == 3 || repetition < 0 || repetition > 1 || node.number(5) != 0 {
			return nil, ErrFormat
		}
		c := parquetColumn{physical: int(physical), optional: repetition == 1, name: string(node.fields[4].b)}
		headers[i] = c.name
		if physical == 7 {
			c.width = int(node.number(2))
			if c.width < 1 || c.width > 32 {
				return nil, ErrLimit
			}
		}
		converted := int64(-1)
		if node.has(6) {
			converted = node.number(6)
		}
		logical := node.fields[10]
		if len(logical.fields) > 1 {
			return nil, ErrFormat
		}
		for id := range logical.fields {
			switch id {
			case 1, 5, 6, 8, 10, 12, 14:
			default:
				return nil, ErrFormat
			}
		}
		if logical.has(10) {
			integer := logical.fields[10]
			if integer.fields[2].n != 1 {
				return nil, ErrFormat
			}
		}
		if converted >= 11 && converted <= 14 || converted == 1 || converted == 2 || converted == 3 || converted == 7 || converted == 8 || converted == 20 || converted == 21 {
			return nil, ErrFormat
		}
		if converted == 5 || logical.has(5) {
			c.decimal = true
			precision := node.number(8)
			c.scale = int(node.number(7))
			if logical.has(5) {
				precision = logical.fields[5].number(2)
				c.scale = int(logical.fields[5].number(1))
			}
			if precision < 1 || precision > 38 || c.scale < 0 || int64(c.scale) > precision || (physical != 1 && physical != 2 && physical != 6 && physical != 7) || p.spec.Columns[i].Type != "decimal" {
				return nil, ErrFormat
			}
		}
		c.date = converted == 6 || logical.has(6)
		if c.date && (physical != 1 || p.spec.Columns[i].Type != "date") {
			return nil, ErrFormat
		}
		if converted == 9 {
			c.timestamp = 1000
		}
		if converted == 10 {
			c.timestamp = 1000000
		}
		if logical.has(8) {
			t := logical.fields[8]
			if t.fields[1].n != 1 {
				return nil, ErrFormat
			}
			u := t.fields[2]
			switch {
			case u.has(1):
				c.timestamp = 1000
			case u.has(2):
				c.timestamp = 1000000
			case u.has(3):
				c.timestamp = 1000000000
			default:
				return nil, ErrFormat
			}
		}
		if c.timestamp != 0 && (physical != 2 || p.spec.Columns[i].Type != "timestamp") {
			return nil, ErrFormat
		}
		columns[i] = c
	}
	if err = p.headers(headers); err != nil {
		return nil, err
	}
	groups := file.fields[4].list
	if len(groups) > 128 || len(groups) == 0 && rows != 0 {
		return nil, ErrLimit
	}
	var totalRows, totalExpanded int64
	spans := []parquetSpan{}
	for _, group := range groups {
		if err = p.ctx.Err(); err != nil {
			return nil, err
		}
		count, ok := group.integer(3)
		if !ok || count < 0 || count > rows-totalRows {
			return nil, ErrFormat
		}
		totalRows += count
		chunks := group.fields[1].list
		if len(chunks) != len(columns) {
			return nil, ErrFormat
		}
		var groupExpanded int64
		for i, chunk := range chunks {
			if len(chunk.fields[1].b) != 0 || chunk.has(8) || chunk.has(9) {
				return nil, ErrFormat
			}
			meta := chunk.fields[3]
			if meta.number(1) != int64(columns[i].physical) || meta.number(5) != count || len(meta.fields[3].list) != 1 || string(meta.fields[3].list[0].b) != columns[i].name {
				return nil, ErrFormat
			}
			for _, enc := range meta.fields[2].list {
				switch enc.n {
				case 0, 2, 3, 4, 6, 8:
				default:
					return nil, ErrFormat
				}
			}
			codec := meta.number(4)
			if codec != 0 && codec != 1 && codec != 2 && codec != 6 {
				return nil, ErrFormat
			}
			size, ok := meta.integer(7)
			expanded, eok := meta.integer(6)
			if !ok || !eok || size < 0 || expanded < 0 || expanded > p.limits.MaxRowGroupBytes-groupExpanded {
				return nil, ErrLimit
			}
			groupExpanded += expanded
			totalExpanded += expanded
			if totalExpanded > p.limits.MaxExpandedBytes || expanded > (size+1)*p.limits.MaxExpansionRatio {
				return nil, ErrLimit
			}
			start := meta.number(9)
			if meta.has(11) && meta.number(11) < start {
				start = meta.number(11)
			}
			if start < 4 || size > footerAt-start || start > footerAt {
				return nil, ErrFormat
			}
			if size == 0 && count == 0 {
				continue
			}
			if meta.number(9) < start || meta.number(9) >= start+size {
				return nil, ErrFormat
			}
			spans = append(spans, parquetSpan{start, start + size})
			if err = p.parquetPages(raw[start:start+size], columns[i], int(codec), count, expanded); err != nil {
				return nil, err
			}
		}
		if group.number(2) != groupExpanded {
			return nil, ErrFormat
		}
	}
	if totalRows != rows {
		return nil, ErrFormat
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	for i := 1; i < len(spans); i++ {
		if spans[i].start < spans[i-1].end {
			return nil, ErrFormat
		}
	}
	return columns, nil
}

func (p *parser) parquetPages(raw []byte, column parquetColumn, codec int, expectedRows, expectedBytes int64) error {
	at, rows, expanded := 0, int64(0), int64(0)
	dictionary := -1
	pages := 0
	for at < len(raw) {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		pages++
		if pages > 65536 {
			return ErrLimit
		}
		end := at + 65536
		if end > len(raw) {
			end = len(raw)
		}
		header, n, err := compact(raw[at:end])
		if err != nil {
			return err
		}
		compressed, ok := header.integer(3)
		size, sok := header.integer(2)
		if !ok || !sok || compressed < 0 || size < 0 || size > p.limits.MaxPageBytes || compressed > p.limits.MaxPageBytes+65536 || int64(n)+compressed > int64(len(raw)-at) {
			return ErrLimit
		}
		at += n
		body := raw[at : at+int(compressed)]
		at += int(compressed)
		expanded += int64(n) + size
		if expanded > expectedBytes {
			return ErrFormat
		}
		if header.has(4) && crc32.ChecksumIEEE(body) != uint32(header.number(4)) {
			return ErrFormat
		}
		kind := header.number(1)
		if kind == 2 {
			h := header.fields[7]
			count := h.number(1)
			if dictionary != -1 || rows != 0 || count < 0 || count > int64(p.limits.MaxRows) || h.number(2) != 0 {
				return ErrFormat
			}
			decoded, e := boundedDecompress(codec, body, int(size))
			if e != nil {
				return e
			}
			if e = guardPlain(decoded, column, int(count), p.limits.MaxCellBytes); e != nil {
				return e
			}
			dictionary = int(count)
			continue
		}
		var count, nonNull int64
		encoding := int64(0)
		var data []byte
		switch kind {
		case 0:
			h := header.fields[5]
			count = h.number(1)
			encoding = h.number(2)
			if count < 1 || count > expectedRows-rows || h.number(3) != 3 || h.number(4) != 3 {
				return ErrFormat
			}
			data, err = boundedDecompress(codec, body, int(size))
			if err != nil {
				return err
			}
			nonNull = count
			if column.optional {
				if len(data) < 4 {
					return ErrFormat
				}
				length := int64(binary.LittleEndian.Uint32(data[:4]))
				if length > int64(len(data)-4) {
					return ErrFormat
				}
				nonNull, err = guardRLE(data[4:4+length], 1, int(count), 2, true)
				if err != nil {
					return err
				}
				data = data[4+length:]
			}
		case 3:
			h := header.fields[8]
			count = h.number(1)
			nulls := h.number(2)
			encoding = h.number(4)
			def := h.number(5)
			rep := h.number(6)
			if count < 1 || count > expectedRows-rows || h.number(3) != count || nulls < 0 || nulls > count || def < 0 || rep != 0 || def > int64(len(body)) || def > size || (!column.optional && (def != 0 || nulls != 0)) {
				return ErrFormat
			}
			nonNull = count
			if column.optional {
				nonNull, err = guardRLE(body[:def], 1, int(count), 2, true)
				if err != nil || nonNull != count-nulls {
					return ErrFormat
				}
			}
			pageCodec := codec
			if h.has(7) && h.fields[7].n == 0 {
				pageCodec = 0
			}
			data, err = boundedDecompress(pageCodec, body[def:], int(size-def))
			if err != nil {
				return err
			}
		default:
			return ErrFormat
		}
		rows += count
		switch encoding {
		case 0:
			if err = guardPlain(data, column, int(nonNull), p.limits.MaxCellBytes); err != nil {
				return err
			}
		case 6:
			if column.physical != 6 {
				return ErrFormat
			}
			if err = guardDeltaLengths(data, int(nonNull), p.limits.MaxCellBytes); err != nil {
				return err
			}
		case 2, 8:
			if dictionary < 0 || len(data) < 1 || data[0] > 32 || int(data[0]) > bits.Len(uint(max(dictionary-1, 0))) {
				return ErrFormat
			}
			if _, err = guardRLE(data[1:], int(data[0]), int(nonNull), dictionary, false); err != nil {
				return err
			}
		default:
			return ErrFormat
		}
	}
	if rows != expectedRows || expanded != expectedBytes {
		return ErrFormat
	}
	return nil
}

func boundedDecompress(codec int, raw []byte, size int) ([]byte, error) {
	if size < 0 || size > 8<<20 {
		return nil, ErrLimit
	}
	var out []byte
	var err error
	switch codec {
	case 0:
		out = raw
	case 1:
		n, e := snappy.DecodedLen(raw)
		if e != nil || n != size {
			return nil, ErrFormat
		}
		out, err = snappy.Decode(make([]byte, 0, size), raw)
	case 2:
		r, e := gzip.NewReader(bytes.NewReader(raw))
		if e != nil {
			return nil, ErrFormat
		}
		out, err = io.ReadAll(io.LimitReader(r, int64(size)+1))
		ce := r.Close()
		if err == nil {
			err = ce
		}
	case 6:
		r, e := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderMaxWindow(8<<20), zstd.WithDecodeAllCapLimit(true))
		if e != nil {
			return nil, ErrFormat
		}
		out, err = r.DecodeAll(raw, make([]byte, 0, size))
		r.Close()
	default:
		return nil, ErrFormat
	}
	if err != nil || len(out) != size {
		return nil, ErrFormat
	}
	return out, nil
}
func guardPlain(data []byte, c parquetColumn, n, maxCell int) error {
	if n < 0 || n > 1000000 {
		return ErrLimit
	}
	width := 0
	switch c.physical {
	case 0:
		if len(data) != (n+7)/8 {
			return ErrFormat
		}
		return nil
	case 1, 4:
		width = 4
	case 2, 5:
		width = 8
	case 7:
		width = c.width
	case 6:
		at := 0
		for i := 0; i < n; i++ {
			if len(data)-at < 4 {
				return ErrFormat
			}
			size := int64(binary.LittleEndian.Uint32(data[at : at+4]))
			at += 4
			if size > int64(maxCell) || size > int64(len(data)-at) {
				return ErrLimit
			}
			at += int(size)
		}
		if at != len(data) {
			return ErrFormat
		}
		return nil
	default:
		return ErrFormat
	}
	if len(data) != n*width {
		return ErrFormat
	}
	return nil
}

// guardRLE checks run lengths and dictionary indices without allocating a decoded
// vector. Only the final bit-packed run may have up to seven padding values.
func guardRLE(data []byte, width, n, ceiling int, ones bool) (int64, error) {
	if width < 0 || width > 32 || n < 0 || n > 1000000 || ceiling < 0 {
		return 0, ErrFormat
	}
	r := compactReader{data: data}
	produced := 0
	var sum int64
	for r.at < len(data) {
		header, e := r.unsigned()
		if e != nil || header < 2 {
			return 0, ErrFormat
		}
		count := header >> 1
		if header&1 == 0 {
			if count > uint64(n-produced) {
				return 0, ErrFormat
			}
			raw, e := r.take((width + 7) / 8)
			if e != nil {
				return 0, e
			}
			var value uint64
			for i, b := range raw {
				value |= uint64(b) << uint(8*i)
			}
			if value >= uint64(ceiling) || width < 32 && value >= (uint64(1)<<uint(width)) {
				return 0, ErrFormat
			}
			if ones {
				sum += int64(value * count)
			}
			produced += int(count)
		} else {
			if count > uint64((n-produced+7)/8) {
				return 0, ErrFormat
			}
			values := int(count) * 8
			raw, e := r.take(int(count) * width)
			if e != nil {
				return 0, e
			}
			for i := 0; i < values; i++ {
				var value uint64
				for bit := 0; bit < width; bit++ {
					pos := i*width + bit
					value |= uint64((raw[pos/8]>>uint(pos%8))&1) << uint(bit)
				}
				if value >= uint64(ceiling) {
					return 0, ErrFormat
				}
				if ones && produced+i < n {
					sum += int64(value)
				}
			}
			produced += values
			if produced > n && r.at != len(data) {
				return 0, ErrFormat
			}
		}
	}
	if produced < n || produced > n+7 {
		return 0, ErrFormat
	}
	return sum, nil
}
