package reporting

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

const (
	filterCursorMax  = 16 << 10
	filterScalarMax  = 1024
	filterEncodedMax = 6*filterScalarMax + 2
	filterLabelMax   = 1024
)

type filterCursor struct {
	Report         string          `json:"report"`
	Revision       int64           `json:"revision"`
	Filter         string          `json:"filter"`
	Search         string          `json:"search"`
	Limit          int             `json:"limit"`
	Locale         string          `json:"locale"`
	SourceRevision int64           `json:"source_revision"`
	Authority      string          `json:"authority"`
	Type           string          `json:"type"`
	Last           json.RawMessage `json:"last"`
	Expires        int64           `json:"expires"`
}

func filterAuthority(e identity.Envelope) string {
	scopes := e.Scopes()
	sort.Strings(scopes)
	return exec.Hash([]any{e.Tenant(), e.User(), e.Session(), scopes})
}

func (s *Documents) encodeFilterCursor(c filterCursor) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > filterCursorMax/2 {
		return "", ErrInvalid
	}
	mac := hmac.New(sha256.New, s.cursorKey[:])
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Documents) decodeFilterCursor(encoded string) (filterCursor, error) {
	var out filterCursor
	if len(encoded) == 0 || len(encoded) > filterCursorMax || strings.Count(encoded, ".") != 1 {
		return out, ErrInvalid
	}
	parts := strings.Split(encoded, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(raw) > filterCursorMax/2 || base64.RawURLEncoding.EncodeToString(raw) != parts[0] {
		return out, ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || base64.RawURLEncoding.EncodeToString(sig) != parts[1] {
		return out, ErrInvalid
	}
	mac := hmac.New(sha256.New, s.cursorKey[:])
	_, _ = mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(raw, &out) != nil || out.Expires <= time.Now().Unix() || len(out.Last) == 0 || string(out.Last) == "null" {
		return filterCursor{}, ErrStale
	}
	return out, nil
}

func filterSQLName(dialect, value string) string {
	switch dialect {
	case "mysql", "bigquery", "databricks":
		return "`" + value + "`"
	case "sqlserver":
		return "[" + value + "]"
	default:
		return `"` + value + `"`
	}
}

func filterPlaceholder(dialect string, index int) string {
	switch dialect {
	case "postgres":
		return "$" + strconv.Itoa(index)
	case "sqlserver", "bigquery":
		return "@p" + strconv.Itoa(index)
	default:
		return "?"
	}
}

func filterQualified(binding exec.Binding, relation exec.Relation) (string, error) {
	parts := []string{relation.Schema, relation.Name}
	if binding.Catalog != "" && slices.Contains([]string{"sqlserver", "bigquery", "snowflake", "databricks"}, binding.Dialect) {
		parts = append([]string{binding.Catalog}, parts...)
	}
	if binding.Dialect == "bigquery" {
		for _, part := range parts {
			if !exec.SQLIdentifierForDialect(binding.Dialect, part) {
				return "", ErrStale
			}
		}
		return "`" + strings.Join(parts, ".") + "`", nil
	}
	quoted := make([]string, len(parts))
	for i, part := range parts {
		if !exec.SQLIdentifierForDialect(binding.Dialect, part) {
			return "", ErrStale
		}
		quoted[i] = filterSQLName(binding.Dialect, part)
	}
	return strings.Join(quoted, "."), nil
}

func filterOptionStatement(binding exec.Binding, relation exec.Relation, column exec.Column, search, cursor bool, limit int) (string, error) {
	if !slices.Contains([]string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"}, binding.Dialect) || limit < 1 || limit > 201 || !exec.SQLIdentifierForDialect(binding.Dialect, column.Name) {
		return "", ErrUnavailable
	}
	qualified, err := filterQualified(binding, relation)
	if err != nil {
		return "", err
	}
	name, alias := filterSQLName(binding.Dialect, column.Name), filterSQLName(binding.Dialect, "value")
	prefix := "SELECT DISTINCT "
	if binding.Dialect == "sqlserver" {
		prefix += "TOP (" + strconv.Itoa(limit) + ") "
	}
	statement := prefix + name + " AS " + alias + " FROM " + qualified
	where := []string{}
	parameter := 0
	if search {
		parameter++
		where = append(where, "LOWER("+name+") LIKE LOWER("+filterPlaceholder(binding.Dialect, parameter)+") ESCAPE '!'")
	}
	if cursor {
		parameter++
		marker := filterPlaceholder(binding.Dialect, parameter)
		operator := ">"
		if binding.Dialect == "mysql" || binding.Dialect == "sqlserver" {
			operator = "<"
		}
		predicate := name + " " + operator + " " + marker
		if !search {
			predicate += " OR " + name + " IS NULL"
		}
		where = append(where, predicate)
	}
	if len(where) > 0 {
		statement += " WHERE " + strings.Join(where, " AND ")
	}
	switch binding.Dialect {
	case "mysql":
		statement += " ORDER BY " + name + " DESC"
	case "sqlserver":
		// SQL Server sorts NULL first in ascending order and has no NULLS LAST.
		// Descending order supplies the same deterministic null-last keyset shape.
		statement += " ORDER BY " + name + " DESC"
	default:
		statement += " ORDER BY " + name + " ASC NULLS LAST"
	}
	if binding.Dialect != "sqlserver" {
		statement += " LIMIT " + strconv.Itoa(limit)
	}
	return statement, nil
}

func filterParameter(kind string, raw json.RawMessage) (exec.Parameter, error) {
	if len(raw) == 0 || len(raw) > filterEncodedMax || string(raw) == "null" {
		return exec.Parameter{}, ErrInvalid
	}
	var value string
	switch kind {
	case "integer", "decimal", "text", "temporal":
		if json.Unmarshal(raw, &value) != nil {
			return exec.Parameter{}, ErrInvalid
		}
		if len(value) > filterScalarMax || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return exec.Parameter{}, ErrBudget
		}
		parameterKind := kind
		if kind == "decimal" {
			parameterKind = "number"
		}
		if kind == "temporal" {
			parameterKind = "text"
		}
		return exec.Parameter{Kind: parameterKind, Value: value}, nil
	case "boolean", "number":
		if len(raw) > filterScalarMax {
			return exec.Parameter{}, ErrBudget
		}
		return exec.Parameter{Kind: kind, Value: string(raw)}, nil
	default:
		return exec.Parameter{}, ErrUnavailable
	}
}

func filterLabel(raw json.RawMessage, locale string) (string, error) {
	if string(raw) == "null" {
		if strings.HasPrefix(strings.ToLower(locale), "es") {
			return "Nulo", nil
		}
		return "Null", nil
	}
	var value string
	if len(raw) == 0 || len(raw) > filterEncodedMax {
		return "", ErrBudget
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return "", ErrInvalid
		}
	} else {
		value = string(raw)
	}
	if len(value) > filterLabelMax || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return "", ErrBudget
	}
	return value, nil
}

type filterOptionResolution struct {
	binding      exec.Binding
	relation     exec.Relation
	physical     exec.Column
	semantic     semantics.Column
	publications []topics.Published
	scope        []exec.RelationScope
}

func validateFilterOptionResult(result *exec.Result, limit int) error {
	if result == nil || len(result.Schema) != 1 || result.Schema[0].Name != "value" || len(result.Rows) > limit+1 {
		return ErrStale
	}
	if result.Outcome == "truncated" || result.Truncation != "" {
		return ErrBudget
	}
	if result.Outcome != "succeeded" && result.Outcome != "empty" {
		return ErrStale
	}
	return nil
}

func filterResultMatches(parameter Parameter, field exec.Field) bool {
	kind := parameter.Type
	if scalar := listScalarType(kind); scalar != "" {
		kind = scalar
	}
	switch kind {
	case "dimension_value":
		return field.Type == "text"
	case "number":
		return field.Type == "decimal" || field.Type == "number" || field.Type == "integer"
	case "integer":
		return field.Type == "integer"
	case "boolean":
		return field.Type == "boolean"
	case "date", "datetime":
		return field.Type == "temporal"
	default:
		return false
	}
}

func filterOptionType(parameter Parameter, column semantics.Column) bool {
	kind := parameter.Type
	if scalar := listScalarType(kind); scalar != "" {
		kind = scalar
	}
	category, native := strings.ToLower(column.Category), strings.ToLower(column.NativeType)
	switch kind {
	case "dimension_value":
		return category == "text" || category == "string"
	case "number":
		return category == "decimal" || category == "number" || category == "numeric"
	case "integer":
		return category == "integer" || category == "numeric" && slices.Contains([]string{"smallint", "integer", "bigint", "int2", "int4", "int8", "tinyint", "int", "int64"}, native)
	case "boolean":
		return category == "boolean"
	case "date":
		return category == "date" || category == "temporal" && strings.Contains(native, "date") && !strings.Contains(native, "time")
	case "datetime":
		return category == "datetime" || category == "temporal" && (strings.Contains(native, "time") || strings.Contains(native, "timestamp"))
	default:
		return false
	}
}

func (s *Documents) resolveFilterOption(ctx context.Context, e identity.Envelope, definition Definition, source FilterOptionSource, parameter Parameter) (filterOptionResolution, error) {
	publications, _, err := s.blocks.resolveDefinitions(ctx, e, definition, true)
	if err != nil {
		return filterOptionResolution{}, err
	}
	binding, err := s.blocks.sources.ContextBinding(ctx, e, definition.Source, definition.Context)
	if err != nil {
		if errors.Is(err, exec.ErrBinding) {
			return filterOptionResolution{}, ErrStale
		}
		return filterOptionResolution{}, err
	}
	scope, err := validationScope(binding, publications)
	if err != nil {
		return filterOptionResolution{}, err
	}
	var semantic semantics.Column
	foundSemantic := false
	for _, publication := range publications {
		if publication.Definition.Topic != source.Topic || publication.Definition.Version != source.TopicVersion {
			continue
		}
		for _, dataset := range publication.Definition.Datasets {
			if dataset.ID != source.Dataset || dataset.Source.Source != definition.Source || dataset.Source.Context != definition.Context || dataset.Source.SourceRevision != binding.Revision {
				continue
			}
			for _, column := range dataset.Columns {
				if column.ID == source.Column {
					semantic, foundSemantic = column, true
				}
			}
		}
	}
	if !foundSemantic || !filterOptionType(parameter, semantic) {
		return filterOptionResolution{}, ErrStale
	}
	relation, physical, foundPhysical := filterColumn(binding, source.Dataset, semantic.SourceName)
	if !foundPhysical || physical.NativeType != semantic.NativeType || physical.Category != semantic.Category || physical.Nullable != semantic.Nullable || !physical.Safe || slices.Contains([]string{"binary", "structured"}, physical.Category) {
		return filterOptionResolution{}, ErrStale
	}
	allowed := false
	for _, item := range scope {
		if item.Dataset == source.Dataset && slices.Contains(item.Columns, physical.Name) {
			allowed = true
		}
	}
	if !allowed {
		return filterOptionResolution{}, ErrStale
	}
	return filterOptionResolution{binding: binding, relation: relation, physical: physical, semantic: semantic, publications: publications, scope: scope}, nil
}

func filterColumn(binding exec.Binding, dataset, name string) (exec.Relation, exec.Column, bool) {
	for _, relation := range binding.Relations {
		if relation.ID != dataset {
			continue
		}
		for _, column := range relation.Columns {
			if column.Name == name && column.Safe {
				return relation, column, true
			}
		}
	}
	return exec.Relation{}, exec.Column{}, false
}

// FilterOptions performs one fresh, bounded, model-free distinct-value read.
// It intentionally has no result cache: every page revalidates current signed
// reach, the immutable report coordinates, active semantics and source revision.
func (s *Documents) FilterOptions(ctx context.Context, e identity.Envelope, report string, in FilterOptionsRequest) (FilterOptionsPage, error) {
	out := FilterOptionsPage{Report: report, Revision: in.Revision, Filter: in.Filter, Options: []FilterOption{}}
	if s == nil || s.blocks == nil || !s.blocks.CanValidate() || ctx == nil {
		return out, ErrUnavailable
	}
	if !identity.Identifier(report) || in.Revision < 1 || in.Revision > 256 || !identity.Identifier(in.Filter) || in.Limit < 1 || in.Limit > 200 || len(in.Search) > 256 || !utf8.ValidString(in.Search) || strings.ContainsRune(in.Search, 0) || !locale(in.Locale) || len(in.Cursor) > filterCursorMax {
		return out, ErrInvalid
	}
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	if !e.Has("reporting.execute") {
		return out, access.ErrForbidden
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	snapshot, err := s.repo.ReadDocument(ctx, e, "report", report, DocumentReference{Revision: in.Revision}, Read, false)
	if err != nil {
		return out, err
	}
	if snapshot.State.Archived || snapshot.State.PublishedRevision != in.Revision {
		return out, store.ErrConflict
	}
	definition, err := ProjectStoredDocument(snapshot.Revision.Raw, "report")
	if err != nil {
		return out, err
	}
	var filter ReportFilter
	for _, candidate := range definition.Filters {
		if candidate.Parameter.Name == in.Filter {
			filter = candidate
			break
		}
	}
	if filter.Parameter.Name == "" || filter.Options == nil {
		return out, store.ErrNotFound
	}
	source := *filter.Options
	block, err := s.blocks.repo.ReadBlock(ctx, e, source.Block, Reference{Revision: source.BlockRevision}, Read)
	if err != nil {
		return out, err
	}
	if block.PublishedAt == nil || block.State.Archived {
		return out, ErrStale
	}
	blockDefinition := block.Revision.Definition
	if err := access.Require(e, "sources.query", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: blockDefinition.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: blockDefinition.Context}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: source.Dataset}); err != nil {
		return out, err
	}
	resolved, err := s.resolveFilterOption(ctx, e, blockDefinition, source, filter.Parameter)
	if err != nil {
		return out, err
	}
	out.SourceRevision = resolved.binding.Revision
	parameters := []exec.Parameter{}
	if in.Search != "" {
		if resolved.physical.Category != "text" {
			return out, ErrInvalid
		}
		escaped := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`).Replace(in.Search)
		parameters = append(parameters, exec.Parameter{Kind: "text", Value: "%" + escaped + "%"})
	}
	if in.Cursor != "" {
		cursor, cursorErr := s.decodeFilterCursor(in.Cursor)
		if cursorErr != nil || cursor.Report != report || cursor.Revision != in.Revision || cursor.Filter != in.Filter || cursor.Search != in.Search || cursor.Limit != in.Limit || cursor.Locale != in.Locale || cursor.SourceRevision != resolved.binding.Revision || cursor.Authority != filterAuthority(e) {
			return out, ErrStale
		}
		parameter, parameterErr := filterParameter(cursor.Type, cursor.Last)
		if parameterErr != nil {
			return out, parameterErr
		}
		parameters = append(parameters, parameter)
	}
	statement, err := filterOptionStatement(resolved.binding, resolved.relation, resolved.physical, in.Search != "", in.Cursor != "", in.Limit+1)
	if err != nil {
		return out, err
	}
	plan, err := s.blocks.validator.ValidateWithin(ctx, e, exec.Request{Source: blockDefinition.Source, Context: blockDefinition.Context, SQL: statement, Parameters: parameters}, resolved.scope)
	if err != nil {
		return out, err
	}
	operation, err := newID()
	if err != nil {
		return out, err
	}
	reportResult, err := s.blocks.executor.Execute(ctx, e, plan, exec.Options{Operation: operation, Number: 1, Preview: true, Rows: in.Limit + 1, Bytes: 512 << 10})
	if err != nil {
		return out, err
	}
	if !successful(reportResult.Attempt.Status) {
		return out, ErrStale
	}
	result := reportResult.Result
	if err := validateFilterOptionResult(result, in.Limit); err != nil {
		return FilterOptionsPage{}, err
	}
	if !filterResultMatches(filter.Parameter, result.Schema[0]) {
		return FilterOptionsPage{}, ErrStale
	}
	for _, row := range result.Rows[:min(len(result.Rows), in.Limit)] {
		if len(row) != 1 {
			return FilterOptionsPage{}, ErrStale
		}
		label, labelErr := filterLabel(row[0], in.Locale)
		if labelErr != nil {
			return FilterOptionsPage{}, labelErr
		}
		out.Options = append(out.Options, FilterOption{Value: append(json.RawMessage(nil), row[0]...), Label: label})
	}
	out.Complete = len(result.Rows) <= in.Limit
	if !out.Complete {
		last := result.Rows[in.Limit-1][0]
		if string(last) == "null" {
			return FilterOptionsPage{}, ErrStale
		}
		if _, parameterErr := filterParameter(result.Schema[0].Type, last); parameterErr != nil {
			return FilterOptionsPage{}, parameterErr
		}
		out.Next, err = s.encodeFilterCursor(filterCursor{Report: report, Revision: in.Revision, Filter: in.Filter, Search: in.Search, Limit: in.Limit, Locale: in.Locale, SourceRevision: resolved.binding.Revision, Authority: filterAuthority(e), Type: result.Schema[0].Type, Last: append(json.RawMessage(nil), last...), Expires: time.Now().Add(5 * time.Minute).Unix()})
		if err != nil {
			return FilterOptionsPage{}, err
		}
	}
	currentBlock, currentErr := s.blocks.repo.ReadBlock(ctx, e, source.Block, Reference{Revision: source.BlockRevision}, Read)
	if currentErr != nil || currentBlock.PublishedAt == nil || currentBlock.State.Archived || currentBlock.Revision.Digest != block.Revision.Digest {
		return FilterOptionsPage{}, ErrStale
	}
	currentResolution, currentErr := s.resolveFilterOption(ctx, e, currentBlock.Revision.Definition, source, filter.Parameter)
	if currentErr != nil || exec.Hash(currentResolution.binding) != exec.Hash(resolved.binding) || exec.Hash(currentResolution.publications) != exec.Hash(resolved.publications) || exec.Hash(currentResolution.scope) != exec.Hash(resolved.scope) || exec.Hash(currentResolution.semantic) != exec.Hash(resolved.semantic) || exec.Hash(currentResolution.physical) != exec.Hash(resolved.physical) {
		return FilterOptionsPage{}, ErrStale
	}
	currentDocument, currentErr := s.repo.ReadDocument(ctx, e, "report", report, DocumentReference{Revision: in.Revision}, Read, false)
	if currentErr != nil || currentDocument.State.Archived || currentDocument.State.PublishedRevision != in.Revision || currentDocument.Revision.Digest != snapshot.Revision.Digest {
		return FilterOptionsPage{}, ErrStale
	}
	return out, ctx.Err()
}
