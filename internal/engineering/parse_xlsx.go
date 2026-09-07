package engineering

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
)

// xlsxParts never extracts an archive to the filesystem. Every member's actual
// expansion is bounded; central-directory sizes alone are not trusted.
func (p *parser) xlsxParts(raw []byte) (map[string][]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, ErrFormat
	}
	if len(z.File) > p.limits.MaxArchiveEntries {
		return nil, ErrLimit
	}
	parts := map[string][]byte{}
	seen := map[string]bool{}
	var expanded int64
	for _, f := range z.File {
		if err = p.ctx.Err(); err != nil {
			return nil, err
		}
		n := strings.TrimSuffix(f.Name, "/")
		if n == "" || len(n) > 512 || strings.ContainsAny(n, "\\:\x00") || strings.HasPrefix(n, "/") || path.Clean(n) != n || n == ".." || strings.HasPrefix(n, "../") || seen[strings.ToLower(n)] || f.Mode()&fs.ModeType != 0 && !f.FileInfo().IsDir() {
			return nil, ErrFormat
		}
		seen[strings.ToLower(n)] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, ErrFormat
		}
		lower := strings.ToLower(n)
		if (!strings.HasSuffix(lower, ".xml") && !strings.HasSuffix(lower, ".rels")) ||
			strings.Contains(lower, "externallink") || strings.Contains(lower, "embedding") || strings.Contains(lower, "vbaproject") || strings.Contains(lower, "connections") || strings.Contains(lower, "ctrlprop") || strings.Contains(lower, "printersetting") {
			return nil, ErrFormat
		}
		left := p.limits.MaxExpandedBytes - expanded
		if left < 0 {
			return nil, ErrLimit
		}
		compressed := f.CompressedSize64
		// An entry cannot span more bytes than the admitted archive. Check
		// the ZIP64 declaration before narrowing it or adding ratio slack.
		if compressed > uint64(len(raw)) || compressed > 100<<20 {
			return nil, ErrFormat
		}
		if f.UncompressedSize64 > uint64(left) {
			return nil, ErrLimit
		}
		r, e := f.Open()
		if e != nil {
			return nil, ErrFormat
		}
		body, e := io.ReadAll(io.LimitReader(r, left+1))
		closeErr := r.Close()
		if e != nil || closeErr != nil {
			return nil, ErrFormat
		}
		expanded += int64(len(body))
		if expanded > p.limits.MaxExpandedBytes || int64(len(body)) > (int64(compressed)+1)*p.limits.MaxExpansionRatio {
			return nil, ErrLimit
		}
		if e = p.safeXML(body); e != nil {
			return nil, e
		}
		parts[n] = body
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels"} {
		if len(parts[name]) == 0 {
			return nil, ErrFormat
		}
	}
	return parts, nil
}

func (p *parser) safeXML(body []byte) error {
	d := xml.NewDecoder(bytes.NewReader(body))
	depth, tokens := 0, int64(0)
	for {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		t, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ErrFormat
		}
		tokens++
		if tokens > p.limits.MaxCells*16+65536 {
			return ErrLimit
		}
		switch v := t.(type) {
		case xml.StartElement:
			depth++
			if depth > 32 || len(v.Attr) > 64 {
				return ErrLimit
			}
			switch strings.ToLower(v.Name.Local) {
			case "f", "formula", "hyperlink", "externalreference", "oleobject", "control", "embeddedobject":
				return ErrFormat
			}
			seen := map[string]bool{}
			for _, a := range v.Attr {
				key := a.Name.Space + "/" + a.Name.Local
				if seen[key] || len(a.Value) > p.limits.MaxCellBytes {
					return ErrFormat
				}
				seen[key] = true
				if a.Name.Local == "TargetMode" && a.Value != "Internal" || a.Name.Local == "ContentType" && (strings.Contains(strings.ToLower(a.Value), "macro") || strings.Contains(strings.ToLower(a.Value), "vba")) {
					return ErrFormat
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return ErrFormat
		case xml.ProcInst:
			if v.Target != "xml" {
				return ErrFormat
			}
		case xml.CharData:
			if len(v) > p.limits.MaxCellBytes {
				return ErrLimit
			}
		}
	}
	if depth != 0 {
		return ErrFormat
	}
	return nil
}
func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

type workbookSheet struct{ name, id string }

func (p *parser) workbook(body []byte) ([]workbookSheet, bool, error) {
	d := xml.NewDecoder(bytes.NewReader(body))
	sheets := []workbookSheet{}
	names, ids := map[string]bool{}, map[string]bool{}
	date1904 := false
	for {
		t, e := d.Token()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return nil, false, ErrFormat
		}
		if v, ok := t.(xml.StartElement); ok {
			switch v.Name.Local {
			case "workbookPr":
				switch attr(v, "date1904") {
				case "1", "true":
					date1904 = true
				case "", "0", "false":
				default:
					return nil, false, ErrFormat
				}
			case "sheet":
				name, id := attr(v, "name"), attr(v, "id")
				if name == "" || id == "" || len(name) > 128 || names[name] || ids[id] {
					return nil, false, ErrFormat
				}
				names[name] = true
				ids[id] = true
				sheets = append(sheets, workbookSheet{name, id})
				if len(sheets) > p.limits.MaxSheets {
					return nil, false, ErrLimit
				}
			}
		}
	}
	if len(sheets) == 0 {
		return nil, false, ErrFormat
	}
	return sheets, date1904, nil
}
func relationships(body []byte, base string) (map[string]string, error) {
	d := xml.NewDecoder(bytes.NewReader(body))
	out := map[string]string{}
	for {
		t, e := d.Token()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return nil, ErrFormat
		}
		v, ok := t.(xml.StartElement)
		if !ok || v.Name.Local != "Relationship" {
			continue
		}
		id, target := attr(v, "Id"), attr(v, "Target")
		if id == "" || target == "" || out[id] != "" || strings.ContainsAny(target, "\\:\x00?#") || strings.Contains(target, "..") || attr(v, "TargetMode") == "External" {
			return nil, ErrFormat
		}
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = path.Join(base, target)
		}
		if path.Clean(target) != target || strings.HasPrefix(target, "/") {
			return nil, ErrFormat
		}
		out[id] = target
	}
	return out, nil
}
func (p *parser) sharedStrings(body []byte) ([]string, error) {
	if len(body) == 0 {
		return nil, nil
	}
	d := xml.NewDecoder(bytes.NewReader(body))
	out := []string{}
	inSI, inText := false, false
	var s strings.Builder
	for {
		t, e := d.Token()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return nil, ErrFormat
		}
		switch v := t.(type) {
		case xml.StartElement:
			if v.Name.Local == "si" {
				if inSI {
					return nil, ErrFormat
				}
				inSI = true
				s.Reset()
			}
			if v.Name.Local == "t" && inSI {
				inText = true
			}
		case xml.CharData:
			if inText {
				if s.Len()+len(v) > p.limits.MaxCellBytes {
					return nil, ErrLimit
				}
				s.Write(v)
			}
		case xml.EndElement:
			if v.Name.Local == "t" {
				inText = false
			}
			if v.Name.Local == "si" {
				out = append(out, s.String())
				inSI = false
				if int64(len(out)) > p.limits.MaxCells {
					return nil, ErrLimit
				}
			}
		}
	}
	return out, nil
}
func (p *parser) xlsx(raw []byte) error {
	parts, err := p.xlsxParts(raw)
	if err != nil {
		return err
	}
	root, err := relationships(parts["_rels/.rels"], "")
	if err != nil {
		return err
	}
	workbookFound := false
	for _, target := range root {
		workbookFound = workbookFound || target == "xl/workbook.xml"
	}
	if !workbookFound {
		return ErrFormat
	}
	sheets, date1904, err := p.workbook(parts["xl/workbook.xml"])
	if err != nil {
		return err
	}
	rels, err := relationships(parts["xl/_rels/workbook.xml.rels"], "xl")
	if err != nil {
		return err
	}
	selected := ""
	for _, sheet := range sheets {
		target := rels[sheet.id]
		if !strings.HasPrefix(target, "xl/worksheets/") || !strings.HasSuffix(target, ".xml") || len(parts[target]) == 0 {
			return ErrFormat
		}
		if sheet.name == p.spec.Sheet {
			selected = target
		}
	}
	if selected == "" {
		return &ParseError{Code: "sheet_not_found"}
	}
	shared, err := p.sharedStrings(parts["xl/sharedStrings.xml"])
	if err != nil {
		return err
	}
	return p.worksheet(parts[selected], shared, date1904)
}
func cellPosition(ref string) (int, int, error) {
	col, i := 0, 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		if i >= 3 {
			return 0, 0, ErrFormat
		}
		col = col*26 + int(ref[i]-'A'+1)
		i++
	}
	if col == 0 || i == len(ref) || ref[i] == '0' {
		return 0, 0, ErrFormat
	}
	row, e := strconv.Atoi(ref[i:])
	if e != nil || row < 1 {
		return 0, 0, ErrFormat
	}
	return col - 1, row, nil
}
func excelTemporal(s string, date1904 bool, timestamp bool) (string, error) {
	serial, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(serial) || math.IsInf(serial, 0) || serial < 0 || serial > 2958465 {
		return "", ErrFormat
	}
	days := int(serial)
	fraction := serial - float64(days)
	base := time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
		if days == 60 {
			return "", ErrFormat
		}
		if days > 60 {
			days--
		}
	}
	v := base.AddDate(0, 0, days).Add(time.Duration(math.Round(fraction * float64(24*time.Hour))))
	if v.Year() > 9999 || !timestamp && fraction != 0 {
		return "", ErrFormat
	}
	if timestamp {
		return v.Format(time.RFC3339Nano), nil
	}
	return v.Format("2006-01-02"), nil
}
func (p *parser) worksheet(body []byte, shared []string, date1904 bool) error {
	d := xml.NewDecoder(bytes.NewReader(body))
	lastRow, rowIndex, nextCol, cellCol := 0, 0, 0, -1
	var row []Cell
	var text strings.Builder
	kind := ""
	inRow, inCell, inValue, hasValue := false, false, false, false
	for {
		if e := p.ctx.Err(); e != nil {
			return e
		}
		t, e := d.Token()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return ErrFormat
		}
		switch v := t.(type) {
		case xml.StartElement:
			switch v.Name.Local {
			case "row":
				if inRow {
					return ErrFormat
				}
				inRow = true
				nextCol = 0
				rowIndex = lastRow + 1
				if s := attr(v, "r"); s != "" {
					rowIndex, e = strconv.Atoi(s)
					if e != nil {
						return ErrFormat
					}
				}
				if rowIndex <= lastRow || rowIndex > p.limits.MaxRows+1 || lastRow == 0 && rowIndex != 1 {
					return ErrLimit
				}
				for i := lastRow + 1; i < rowIndex; i++ {
					empty := make([]Cell, len(p.spec.Columns))
					for j := range empty {
						empty[j].Null = true
					}
					if e = p.row(empty); e != nil {
						return e
					}
				}
				row = make([]Cell, len(p.spec.Columns))
				for i := range row {
					row[i].Null = true
				}
			case "c":
				if !inRow || inCell {
					return ErrFormat
				}
				inCell = true
				hasValue = false
				text.Reset()
				kind = attr(v, "t")
				cellCol = nextCol
				if ref := attr(v, "r"); ref != "" {
					var r int
					cellCol, r, e = cellPosition(ref)
					if e != nil || r != rowIndex {
						return ErrFormat
					}
				}
				if cellCol < nextCol || cellCol >= len(row) {
					return ErrLimit
				}
				nextCol = cellCol + 1
			case "v", "t":
				if inCell {
					inValue = true
					hasValue = true
				}
			}
		case xml.CharData:
			if inValue {
				if text.Len()+len(v) > p.limits.MaxCellBytes {
					return ErrLimit
				}
				text.Write(v)
			}
		case xml.EndElement:
			switch v.Name.Local {
			case "v", "t":
				inValue = false
			case "c":
				if !inCell {
					return ErrFormat
				}
				inCell = false
				if !hasValue {
					continue
				}
				s := text.String()
				switch kind {
				case "s":
					index, err := strconv.Atoi(s)
					if err != nil || index < 0 || index >= len(shared) {
						return ErrFormat
					}
					s = shared[index]
				case "b":
					switch s {
					case "1":
						s = "true"
					case "0":
						s = "false"
					default:
						return ErrFormat
					}
				case "", "n":
					if rowIndex > 1 && (p.spec.Columns[cellCol].Type == "date" || p.spec.Columns[cellCol].Type == "timestamp") {
						s, e = excelTemporal(s, date1904, p.spec.Columns[cellCol].Type == "timestamp")
						if e != nil {
							return e
						}
					}
				case "d", "inlineStr", "str":
				default:
					return ErrFormat
				}
				row[cellCol] = Cell{Text: s}
			case "row":
				if !inRow || inCell {
					return ErrFormat
				}
				inRow = false
				if rowIndex == 1 {
					headers := make([]string, len(row))
					for i, c := range row {
						if c.Null {
							return ErrFormat
						}
						headers[i] = c.Text
					}
					e = p.headers(headers)
				} else {
					e = p.row(row)
				}
				if e != nil {
					return e
				}
				lastRow = rowIndex
			}
		}
	}
	if lastRow == 0 || inRow || inCell {
		return ErrFormat
	}
	return nil
}
