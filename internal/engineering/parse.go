// Package engineering owns managed uploads and versioned profiling evidence.
// It never issues identity tokens or provides an alternative query executor.
package engineering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalid     = errors.New("engineering: invalid request")
	ErrLimit       = errors.New("engineering: configured limit exceeded")
	ErrFormat      = errors.New("engineering: unsupported or unsafe file")
	ErrChecksum    = errors.New("engineering: content checksum mismatch")
	ErrOwnership   = errors.New("engineering: workspace ownership not proven")
	ErrUnavailable = errors.New("engineering: dependency unavailable")
	ErrState       = errors.New("engineering: operation state requires reconciliation")
)

// ParseError identifies only fixed rules and numeric positions, never cell data.
type ParseError struct {
	Code   string `json:"code"`
	Row    int    `json:"row"`
	Column int    `json:"column"`
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("engineering: file %s at row %d column %d", e.Code, e.Row, e.Column)
}
func (e *ParseError) Unwrap() error { return ErrFormat }

// UploadColumn is a declared target type, not an inferred SQL expression.
type UploadColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// UploadSpec is immutable under one upload key. Original filenames and arbitrary
// paths are intentionally absent; the owner chooses a display name and sheet.
type UploadSpec struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Connection string         `json:"connection"`
	Format     string         `json:"format"`
	Sheet      string         `json:"sheet"`
	Columns    []UploadColumn `json:"columns"`
	Bytes      int64          `json:"bytes"`
	SHA256     string         `json:"sha256"`
}

// Valid enforces the same closed input contract before staging or decoding.
func (s UploadSpec) Valid(l config.Uploads) bool {
	if config.ValidateUploads(l) != nil || !identity.Identifier(s.ID) || len(s.ID) > 80 || !identity.Identifier(s.Connection) ||
		len(s.Name) < 1 || len(s.Name) > 128 || strings.TrimSpace(s.Name) != s.Name || !utf8.ValidString(s.Name) ||
		s.Bytes < 1 || s.Bytes > l.MaxBytes || !validHash(s.SHA256) || len(s.Columns) < 1 || len(s.Columns) > l.MaxColumns {
		return false
	}
	for _, r := range s.Name {
		if unicode.IsControl(r) {
			return false
		}
	}
	format := false
	for _, f := range l.Formats {
		format = format || f == s.Format
	}
	if !format || s.Format != "xlsx" && s.Sheet != "" || s.Format == "xlsx" && (s.Sheet == "" || len(s.Sheet) > 128 || !utf8.ValidString(s.Sheet)) {
		return false
	}
	seen := map[string]bool{}
	for _, c := range s.Columns {
		if !readexec.SQLIdentifier(c.Name) || seen[c.Name] || sqlType(c.Type) == "" {
			return false
		}
		seen[c.Name] = true
	}
	return true
}

func validHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func contentHash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func sqlType(kind string) string {
	switch kind {
	case "text":
		return "text"
	case "integer":
		return "bigint"
	case "decimal":
		return "numeric"
	case "number":
		return "double precision"
	case "boolean":
		return "boolean"
	case "date":
		return "date"
	case "timestamp":
		return "timestamp with time zone"
	case "binary":
		return "bytea"
	case "json":
		return "jsonb"
	}
	return ""
}

// NormalizeHeader supplies one deterministic English/Spanish-compatible mapping.
// Unsupported punctuation is rejected, not interpolated into a SQL identifier.
func NormalizeHeader(s string) (string, error) {
	if len(s) < 1 || len(s) > 256 || !utf8.ValidString(s) {
		return "", ErrFormat
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "\ufeff"))
	var out strings.Builder
	underscore := false
	for _, r := range norm.NFKD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == '_' || r == '-' || r == ' ' {
			if out.Len() > 0 {
				underscore = true
			}
			continue
		}
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return "", ErrFormat
		}
		if out.Len() == 0 && r >= '0' && r <= '9' {
			out.WriteString("c_")
		}
		if underscore {
			out.WriteByte('_')
			underscore = false
		}
		out.WriteRune(r)
	}
	n := out.String()
	if !readexec.SQLIdentifier(n) {
		return "", ErrFormat
	}
	return n, nil
}

// Cell preserves exact decimal/integer text and distinguishes NULL from empty text.
type Cell struct {
	Text string
	Null bool
}

// ParseReceipt is structural evidence; no raw cell or bearer is retained in it.
type ParseReceipt struct {
	Rows         int   `json:"rows"`
	Cells        int64 `json:"cells"`
	DecodedBytes int64 `json:"decoded_bytes"`
}

type parser struct {
	ctx     context.Context
	spec    UploadSpec
	limits  config.Uploads
	emit    func([]Cell) error
	receipt ParseReceipt
}

// Parse streams normalized rows into an owned staging transaction. The consumer
// must roll back on ANY error; emitted prefixes are never complete datasets.
func Parse(ctx context.Context, raw []byte, spec UploadSpec, limits config.Uploads, emit func([]Cell) error) (out ParseReceipt, err error) {
	if ctx == nil || emit == nil || !spec.Valid(limits) {
		return out, ErrInvalid
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if int64(len(raw)) != spec.Bytes || contentHash(raw) != spec.SHA256 {
		return out, ErrChecksum
	}
	// Third-party binary decoding is additionally preflight-bounded before it is
	// called. A malformed decoder panic must not crash the service or publish rows.
	defer func() {
		if recover() != nil {
			out = ParseReceipt{}
			err = ErrFormat
		}
	}()
	p := &parser{ctx: ctx, spec: spec, limits: limits, emit: emit}
	switch spec.Format {
	case "csv":
		err = p.csv(raw)
	case "xlsx":
		err = p.xlsx(raw)
	case "parquet":
		err = p.parquet(raw)
	default:
		err = ErrFormat
	}
	if err != nil {
		return ParseReceipt{}, err
	}
	if err = ctx.Err(); err != nil {
		return ParseReceipt{}, err
	}
	return p.receipt, nil
}

func (p *parser) headers(names []string) error {
	if len(names) != len(p.spec.Columns) {
		return &ParseError{Code: "header_count", Row: 1}
	}
	seen := map[string]bool{}
	for i, name := range names {
		n, e := NormalizeHeader(name)
		if e != nil || seen[n] || n != p.spec.Columns[i].Name {
			return &ParseError{Code: "header", Row: 1, Column: i + 1}
		}
		seen[n] = true
	}
	return nil
}
func (p *parser) row(cells []Cell) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if len(cells) != len(p.spec.Columns) {
		return &ParseError{Code: "column_count", Row: p.receipt.Rows + 2}
	}
	if p.receipt.Rows >= p.limits.MaxRows || p.receipt.Cells+int64(len(cells)) > p.limits.MaxCells {
		return ErrLimit
	}
	for i := range cells {
		if len(cells[i].Text) > p.limits.MaxCellBytes {
			return ErrLimit
		}
		v, err := normalizeCell(p.spec.Columns[i], cells[i])
		if err != nil {
			return &ParseError{Code: "cell_type", Row: p.receipt.Rows + 2, Column: i + 1}
		}
		// Canonical numeric/temporal text can be longer than the input cell.
		// Enforce both representations before handing the row to the writer.
		if len(v.Text) > p.limits.MaxCellBytes {
			return ErrLimit
		}
		cells[i] = v
		p.receipt.DecodedBytes += int64(len(v.Text))
		if p.receipt.DecodedBytes > p.limits.MaxExpandedBytes {
			return ErrLimit
		}
	}
	if err := p.emit(cells); err != nil {
		return err
	}
	p.receipt.Rows++
	p.receipt.Cells += int64(len(cells))
	return nil
}
func (p *parser) csv(raw []byte) error {
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return ErrFormat
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = len(p.spec.Columns)
	header, err := r.Read()
	if err != nil {
		return &ParseError{Code: "header", Row: 1}
	}
	if err = p.headers(header); err != nil {
		return err
	}
	for {
		if err = p.ctx.Err(); err != nil {
			return err
		}
		values, e := r.Read()
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return &ParseError{Code: "csv_record", Row: p.receipt.Rows + 2}
		}
		cells := make([]Cell, len(values))
		for i, v := range values {
			cells[i] = Cell{Text: v, Null: v == `\N`}
		}
		if err = p.row(cells); err != nil {
			return err
		}
	}
}
func normalizeCell(column UploadColumn, cell Cell) (Cell, error) {
	if cell.Null {
		if !column.Nullable {
			return Cell{}, ErrFormat
		}
		return Cell{Null: true}, nil
	}
	s := cell.Text
	if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
		return Cell{}, ErrFormat
	}
	switch column.Type {
	case "text":
		n := strings.TrimSpace(norm.NFKC.String(s))
		if n != "" && strings.ContainsRune("=+-@", rune(n[0])) {
			return Cell{}, ErrFormat
		}
	case "integer":
		n, e := strconv.ParseInt(s, 10, 64)
		if e != nil {
			return Cell{}, ErrFormat
		}
		s = strconv.FormatInt(n, 10)
	case "decimal":
		if !readexec.Decimal(s) {
			return Cell{}, ErrFormat
		}
	case "number":
		n, e := strconv.ParseFloat(s, 64)
		if e != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return Cell{}, ErrFormat
		}
		s = strconv.FormatFloat(n, 'g', -1, 64)
	case "boolean":
		if s != "true" && s != "false" {
			return Cell{}, ErrFormat
		}
	case "date":
		v, e := time.Parse("2006-01-02", s)
		if e != nil || v.Year() < 1 || v.Format("2006-01-02") != s {
			return Cell{}, ErrFormat
		}
	case "timestamp":
		// Inspect every fractional digit before parsing. Checking only the
		// resulting nanoseconds could miss a nonzero digit beyond that precision.
		if dot := strings.IndexAny(s, ".,"); dot >= 0 {
			for i := dot + 1; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
				if i > dot+6 && s[i] != '0' {
					return Cell{}, ErrFormat
				}
			}
		}
		v, e := time.Parse(time.RFC3339Nano, s)
		// The managed PostgreSQL workspace has microsecond resolution and no
		// year zero. Never silently discard precision or cross supported years.
		if e != nil || v.Nanosecond()%1000 != 0 || v.UTC().Year() < 1 || v.UTC().Year() > 9999 {
			return Cell{}, ErrFormat
		}
		s = v.UTC().Format(time.RFC3339Nano)
	case "binary":
		b, e := hex.DecodeString(s)
		if e != nil {
			return Cell{}, ErrFormat
		}
		s = hex.EncodeToString(b)
	case "json":
		if !json.Valid([]byte(s)) || jsonDepth([]byte(s)) > 32 {
			return Cell{}, ErrFormat
		}
	default:
		return Cell{}, ErrFormat
	}
	return Cell{Text: s}, nil
}
func jsonDepth(b []byte) int {
	d := json.NewDecoder(bytes.NewReader(b))
	depth, maximum := 0, 0
	for {
		t, e := d.Token()
		if e != nil {
			break
		}
		if v, ok := t.(json.Delim); ok {
			switch v {
			case '{', '[':
				depth++
				if depth > maximum {
					maximum = depth
				}
			case '}', ']':
				depth--
			}
		}
	}
	return maximum
}
