package sources

import (
	"context"
	"sort"
	"strconv"
	"strings"

	bruinmysql "github.com/bruin-data/bruin/pkg/mysql"
	"github.com/bruin-data/bruin/pkg/query"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

// Opening each registered table in the same read-only transaction holds its
// metadata lock through validation/read. Information-schema lookup alone is not
// used as a fence against concurrent index/schema DDL.
func lockMySQLRelation(ctx context.Context, session bruinmysql.ReadSession, r config.SourceRelation) error {
	quote := func(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
	rows, err := session.Query(ctx, &query.Query{Query: "SELECT 1 FROM " + quote(r.Schema) + "." + quote(r.Name) + " LIMIT 0"})
	if err != nil {
		return err
	}
	if rows.Next() {
		_ = rows.Close()
		return readexec.ErrBinding
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

func discoverMySQLUniqueKeys(ctx context.Context, session bruinmysql.ReadSession, r config.SourceRelation, columns []readexec.Column) ([][]string, error) {
	rows, err := session.Query(ctx, &query.Query{Query: "SELECT INDEX_NAME,SEQ_IN_INDEX,COLUMN_NAME,SUB_PART,INDEX_TYPE,EXPRESSION FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND NON_UNIQUE=0 ORDER BY INDEX_NAME,SEQ_IN_INDEX LIMIT 513", Args: []any{r.Schema, r.Name}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	allowed := map[string]bool{}
	for _, c := range columns {
		// Text equality also depends on collation/coercibility, which the
		// current binding does not seal. Never borrow text-index uniqueness.
		allowed[c.Name] = c.Safe && c.Category != "text"
	}
	type index struct {
		columns []string
		invalid bool
		next    int
	}
	indexes := map[string]*index{}
	count := 0
	for rows.Next() {
		count++
		if count > 512 {
			return nil, readexec.ErrLimit
		}
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		if len(values) != 6 {
			return nil, readexec.ErrBinding
		}
		name := mysqlText(values[0])
		if name == "" || len(name) > 128 {
			return nil, readexec.ErrBinding
		}
		item := indexes[name]
		if item == nil {
			if len(indexes) >= 64 {
				return nil, readexec.ErrLimit
			}
			item = &index{next: 1}
			indexes[name] = item
		}
		position, err := strconv.Atoi(mysqlText(values[1]))
		if err != nil || position != item.next {
			return nil, readexec.ErrBinding
		}
		item.next++
		column := mysqlText(values[2])
		if !allowed[column] || values[3] != nil || mysqlText(values[4]) != "BTREE" || values[5] != nil || len(item.columns) >= 16 {
			item.invalid = true
		}
		item.columns = append(item.columns, column)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	var keys [][]string
	seen := map[string]bool{}
	for _, item := range indexes {
		if item.invalid || len(item.columns) == 0 {
			continue
		}
		sort.Strings(item.columns)
		key := strings.Join(item.columns, "\x00")
		if !seen[key] {
			keys = append(keys, item.columns)
			seen[key] = true
		}
	}
	if len(keys) > 32 {
		return nil, readexec.ErrLimit
	}
	sort.Slice(keys, func(i, j int) bool { return strings.Join(keys[i], "\x00") < strings.Join(keys[j], "\x00") })
	return keys, ctx.Err()
}
