package reporting

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

const (
	filterCursorMax = 16 << 10
	filterValueMax  = 4096
	filterLabelMax  = 1024
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
	if err != nil || len(raw) > filterCursorMax/2 {
		return out, ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return out, ErrInvalid
	}
	mac := hmac.New(sha256.New, s.cursorKey[:])
	_, _ = mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(raw, &out) != nil || out.Expires <= time.Now().Unix() || len(out.Last) == 0 || string(out.Last) == "null" {
		return filterCursor{}, ErrStale
	}
	return out, nil
}

func filterSQLName(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func filterParameter(kind string, raw json.RawMessage) (exec.Parameter, error) {
	if len(raw) == 0 || len(raw) > filterValueMax || string(raw) == "null" {
		return exec.Parameter{}, ErrInvalid
	}
	var value string
	switch kind {
	case "integer", "decimal", "text", "temporal":
		if json.Unmarshal(raw, &value) != nil {
			return exec.Parameter{}, ErrInvalid
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
	if len(raw) == 0 || len(raw) > filterValueMax {
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
	publications, _, err := s.blocks.resolveDefinitions(ctx, e, blockDefinition, true)
	if err != nil {
		return out, err
	}
	var physical string
	found := false
	for _, publication := range publications {
		if publication.Definition.Topic != source.Topic || publication.Definition.Version != source.TopicVersion {
			continue
		}
		for _, dataset := range publication.Definition.Datasets {
			if dataset.ID != source.Dataset {
				continue
			}
			for _, column := range dataset.Columns {
				if column.ID == source.Column {
					physical, found = column.SourceName, true
				}
			}
		}
	}
	if !found {
		return out, ErrStale
	}
	if err := access.Require(e, "sources.query", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: blockDefinition.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: blockDefinition.Context}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: source.Dataset}); err != nil {
		return out, err
	}
	binding, err := s.blocks.sources.ContextBinding(ctx, e, blockDefinition.Source, blockDefinition.Context)
	if err != nil {
		return out, err
	}
	relation, column, ok := filterColumn(binding, source.Dataset, physical)
	if !ok || slices.Contains([]string{"binary", "structured"}, column.Category) {
		return out, ErrUnavailable
	}
	out.SourceRevision = binding.Revision
	columnSQL := filterSQLName(column.Name)
	where := []string{}
	parameters := []exec.Parameter{}
	if in.Search != "" {
		if column.Category != "text" {
			return out, ErrInvalid
		}
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(in.Search)
		parameters = append(parameters, exec.Parameter{Kind: "text", Value: "%" + escaped + "%"})
		where = append(where, "LOWER("+columnSQL+") LIKE LOWER($1)")
	}
	if in.Cursor != "" {
		cursor, cursorErr := s.decodeFilterCursor(in.Cursor)
		if cursorErr != nil || cursor.Report != report || cursor.Revision != in.Revision || cursor.Filter != in.Filter || cursor.Search != in.Search || cursor.Limit != in.Limit || cursor.Locale != in.Locale || cursor.SourceRevision != binding.Revision || cursor.Authority != filterAuthority(e) {
			return out, ErrStale
		}
		parameter, parameterErr := filterParameter(cursor.Type, cursor.Last)
		if parameterErr != nil {
			return out, parameterErr
		}
		parameters = append(parameters, parameter)
		where = append(where, fmt.Sprintf("(%s > $%d OR %s IS NULL)", columnSQL, len(parameters), columnSQL))
	}
	statement := "SELECT DISTINCT " + columnSQL + " AS value FROM " + filterSQLName(relation.Schema) + "." + filterSQLName(relation.Name)
	if len(where) > 0 {
		statement += " WHERE " + strings.Join(where, " AND ")
	}
	statement += " ORDER BY " + columnSQL + " ASC NULLS LAST LIMIT " + strconv.Itoa(in.Limit+1)
	plan, err := s.blocks.validator.ValidateWithin(ctx, e, exec.Request{Source: blockDefinition.Source, Context: blockDefinition.Context, SQL: statement, Parameters: parameters}, []exec.RelationScope{{Dataset: source.Dataset, Columns: []string{column.Name}}})
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
	if !successful(reportResult.Attempt.Status) || reportResult.Result == nil || len(reportResult.Result.Schema) != 1 || reportResult.Result.Schema[0].Name != "value" || len(reportResult.Result.Rows) > in.Limit+1 {
		return out, ErrStale
	}
	result := reportResult.Result
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
		out.Next, err = s.encodeFilterCursor(filterCursor{Report: report, Revision: in.Revision, Filter: in.Filter, Search: in.Search, Limit: in.Limit, Locale: in.Locale, SourceRevision: binding.Revision, Authority: filterAuthority(e), Type: result.Schema[0].Type, Last: append(json.RawMessage(nil), last...), Expires: time.Now().Add(5 * time.Minute).Unix()})
		if err != nil {
			return FilterOptionsPage{}, err
		}
	}
	if current, currentErr := s.blocks.sources.ContextBinding(ctx, e, blockDefinition.Source, blockDefinition.Context); currentErr != nil || exec.Hash(current) != exec.Hash(binding) {
		return FilterOptionsPage{}, ErrStale
	}
	currentBlock, currentErr := s.blocks.repo.ReadBlock(ctx, e, source.Block, Reference{Revision: source.BlockRevision}, Read)
	if currentErr != nil || currentBlock.PublishedAt == nil || currentBlock.State.Archived || currentBlock.Revision.Digest != block.Revision.Digest {
		return FilterOptionsPage{}, ErrStale
	}
	currentPublications, _, currentErr := s.blocks.resolveDefinitions(ctx, e, currentBlock.Revision.Definition, true)
	if currentErr != nil || exec.Hash(currentPublications) != exec.Hash(publications) {
		return FilterOptionsPage{}, ErrStale
	}
	currentDocument, currentErr := s.repo.ReadDocument(ctx, e, "report", report, DocumentReference{Revision: in.Revision}, Read, false)
	if currentErr != nil || currentDocument.State.Archived || currentDocument.State.PublishedRevision != in.Revision || currentDocument.Revision.Digest != snapshot.Revision.Digest {
		return FilterOptionsPage{}, ErrStale
	}
	return out, ctx.Err()
}
