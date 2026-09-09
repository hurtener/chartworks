package reporting

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/hurtener/chartworks/internal/chartdata"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"golang.org/x/text/language"
)

func digest(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func clone[T any](v T) T {
	var out T
	raw, _ := json.Marshal(v)
	_ = json.Unmarshal(raw, &out)
	return out
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func hashValid(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && strings.ToLower(s) == s
}

func text(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 32 && r != '\n' && r != '\t' || r == 127 {
			return false
		}
	}
	return true
}

func note(s string) bool { return strings.TrimSpace(s) != "" && text(s, 2048) }

func locale(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	l, err := language.Parse(s)
	return err == nil && l.String() == s && l != language.Und
}

func metadataValid(ms []Localized, limits config.Reporting) bool {
	if len(ms) == 0 || len(ms) > limits.MaxLocales {
		return false
	}
	seen := map[string]bool{}
	for _, m := range ms {
		if !locale(m.Locale) || seen[m.Locale] || strings.TrimSpace(m.Title) == "" || !text(m.Title, 256) || strings.TrimSpace(m.Question) == "" || !text(m.Question, 2048) || !text(m.Description, 4096) || len(m.Aliases) > limits.MaxAliases {
			return false
		}
		seen[m.Locale] = true
		aliases := map[string]bool{normalizeQuestion(m.Question): true}
		for _, a := range m.Aliases {
			n := normalizeQuestion(a)
			if n == "" || !text(a, 2048) || aliases[n] {
				return false
			}
			aliases[n] = true
		}
	}
	return true
}

// DefinitionDigest pins canonicalization independently from execution semantics.
// SQL bytes are never silently normalized or rewritten before hashing/execution.
func DefinitionDigest(d Definition) string {
	return digest(struct {
		Version    string
		Definition Definition
	}{CanonicalizationVersion, d})
}

func ExecutionDigest(d Definition) string {
	return digest(struct {
		Version    string
		Source     string
		Context    string
		Topics     []TopicPin
		Template   *TemplatePin
		SQL        string
		Parameters []Parameter
		Schema     []exec.Field
	}{CanonicalizationVersion, d.Source, d.Context, d.Topics, d.Template, d.SQL, d.Parameters, d.ExpectedSchema})
}

func validateDefinition(ctx context.Context, d Definition, limits config.Reporting, captured bool) error {
	if ctx == nil || limits.Validate() != nil || d.SchemaVersion != SchemaVersion || !metadataValid(d.Metadata, limits) || !identity.Identifier(d.Source) || !identity.Identifier(d.Context) || len(d.Topics) == 0 || len(d.Topics) > 8 || strings.TrimSpace(d.SQL) == "" || !text(d.SQL, limits.MaxSQLBytes) || len(d.ExpectedSchema) == 0 || len(d.ExpectedSchema) > limits.MaxSchemaColumns || len(d.Outputs) == 0 || len(d.Outputs) > limits.MaxOutputs {
		return ErrInvalid
	}
	encoded, err := json.Marshal(d)
	if err != nil || len(encoded) > limits.MaxDefinitionBytes {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, pin := range d.Topics {
		if !identity.Identifier(pin.Topic) || !identity.Identifier(pin.Version) || !hashValid(pin.Digest) || seen[pin.Topic] {
			return ErrInvalid
		}
		seen[pin.Topic] = true
	}
	if d.Template != nil && (!captured || !identity.Identifier(d.Template.ID) || !identity.Identifier(d.Template.Version) || !hashValid(d.Template.Digest)) {
		return ErrInvalid
	}
	if err := validateDeclarations(d.Parameters, limits.MaxParameters); err != nil {
		return err
	}
	fields := map[string]exec.Field{}
	for _, f := range d.ExpectedSchema {
		if strings.TrimSpace(f.Name) == "" || !text(f.Name, 256) || !text(f.NativeType, 128) || f.NativeType == "" || f.Encoding == "" || !text(f.Encoding, 64) || fields[f.Name].Name != "" || chartType(f.Type) == "" {
			return ErrInvalid
		}
		fields[f.Name] = f
	}
	outputIDs := map[string]bool{}
	for _, o := range d.Outputs {
		if !identity.Identifier(o.ID) || outputIDs[o.ID] {
			return ErrInvalid
		}
		outputIDs[o.ID] = true
		if err := validateOutput(ctx, o, fields, limits); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func chartType(t string) string {
	switch t {
	case "integer", "decimal", "number", "boolean", "binary":
		return t
	case "text", "string", "uuid":
		return "text"
	case "date", "time", "timestamp", "timestamptz", "datetime", "temporal":
		return "temporal"
	case "json", "jsonb", "structured":
		return "structured"
	default:
		return ""
	}
}

func chartLimits(limits config.Reporting) charts.Limits {
	out := charts.Defaults()
	out.MaxColumns, out.MaxRows, out.MaxBytes = limits.MaxSchemaColumns, limits.PreviewRows, limits.PreviewBytes
	if out.MaxCellBytes > out.MaxBytes {
		out.MaxCellBytes = out.MaxBytes
	}
	return out
}

func validateOutput(ctx context.Context, o Output, fields map[string]exec.Field, limits config.Reporting) error {
	if o.Kind == "narrative" {
		if o.Mapping != nil || o.Narrative == nil {
			return ErrInvalid
		}
		return validateNarrative(*o.Narrative, fields)
	}
	if !slices.Contains([]string{"chart", "kpi", "table"}, o.Kind) || o.Mapping == nil || o.Narrative != nil {
		return ErrInvalid
	}
	m := *o.Mapping
	if o.Kind == "kpi" && m.Kind != charts.KPI || o.Kind == "table" && m.Kind != charts.Table || o.Kind == "chart" && (m.Kind == charts.KPI || m.Kind == charts.Table) {
		return ErrInvalid
	}
	for _, c := range m.Columns {
		f, ok := fields[c.Name]
		if !ok || chartType(f.Type) != c.Type {
			return ErrInvalid
		}
	}
	d := charts.Data{Version: charts.Version, Columns: clone(m.Columns), Rows: [][]charts.Cell{}, Completeness: charts.Completeness{Status: "complete_result"}}
	if err := charts.ValidateMapping(ctx, d, m, chartLimits(limits)); err != nil {
		return ErrInvalid
	}
	return nil
}

func validateNarrative(n Narrative, fields map[string]exec.Field) error {
	if !slices.Contains([]string{"summary", "comparison", "explanation"}, n.Type) || strings.TrimSpace(n.Instructions) == "" || !text(n.Instructions, 4096) || len(n.Fields) == 0 || len(n.Fields) > 128 || len(n.RedactedFields) > 128 || !slices.Contains([]string{"first_rows", "aggregate_evidence"}, n.Reduction) || n.MaxRows < 1 || n.MaxRows > 1000 || n.MaxBytes < 128 || n.MaxBytes > 65536 || n.MaxCharacters < 1 || n.MaxCharacters > 16384 || n.MaxCalls < 1 || n.MaxCalls > 4 || n.MaxTokens < 64 || n.MaxTokens > 32768 || n.TimeoutMillis < 100 || n.TimeoutMillis > 60000 || !identity.Identifier(n.PromptVersion) || !identity.Identifier(n.ModelVersion) || !identity.Identifier(n.SchemaVersion) || !locale(n.Locale) || !slices.Contains([]string{"neutral", "concise", "technical"}, n.Tone) || !n.RequireEvidence || !n.RequireCaveats {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, names := range [][]string{n.Fields, n.RedactedFields} {
		for _, name := range names {
			if fields[name].Name == "" || seen[name] {
				return ErrInvalid
			}
			seen[name] = true
		}
	}
	return nil
}

// SelectOutputs returns saved definitions in their original stable order. An
// empty selection means all; duplicate/unknown IDs fail rather than being dropped.
func SelectOutputs(all []Output, selected []string) ([]Output, error) {
	if len(all) == 0 || len(all) > 64 || len(selected) > len(all) {
		return nil, ErrInvalid
	}
	available := map[string]bool{}
	for _, output := range all {
		if !identity.Identifier(output.ID) || available[output.ID] {
			return nil, ErrInvalid
		}
		available[output.ID] = true
	}
	wanted := map[string]bool{}
	for _, id := range selected {
		if !available[id] || wanted[id] {
			return nil, ErrInvalid
		}
		wanted[id] = true
	}
	out := []Output{}
	for _, o := range all {
		if len(selected) == 0 || wanted[o.ID] {
			out = append(out, clone(o))
		}
	}
	return out, nil
}

// checkResult validates the observed schema against the exact draft and checks
// saved mappings against the real normalized values without selecting new charts.
func checkResult(ctx context.Context, d Definition, result exec.Result, limits config.Reporting) error {
	if digest(result.Schema) != digest(d.ExpectedSchema) || result.Bytes > limits.PreviewBytes {
		return ErrStale
	}
	normalized, err := chartdata.FromReadResult(ctx, result, chartLimits(limits))
	if err != nil {
		return ErrStale
	}
	positions := map[string]int{}
	for i, f := range result.Schema {
		positions[f.Name] = i
	}
	for _, output := range d.Outputs {
		if output.Mapping == nil {
			continue
		}
		data := charts.Data{Version: charts.Version, Columns: clone(output.Mapping.Columns), Rows: make([][]charts.Cell, len(normalized.Rows)), Completeness: normalized.Completeness}
		for rowIndex, row := range normalized.Rows {
			data.Rows[rowIndex] = make([]charts.Cell, len(data.Columns))
			for i, column := range data.Columns {
				index, ok := positions[column.Name]
				if !ok {
					return ErrStale
				}
				data.Rows[rowIndex][i] = row[index]
			}
		}
		if err := charts.ValidateMapping(ctx, data, *output.Mapping, chartLimits(limits)); err != nil {
			return ErrStale
		}
	}
	return ctx.Err()
}
