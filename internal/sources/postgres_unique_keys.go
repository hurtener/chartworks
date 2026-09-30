package sources

import (
	"context"
	"sort"
	"strings"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

// Only valid, ready, immediate, unconditional plain-column indexes qualify.
// INCLUDE columns are not keys; expression/partial/deferrable indexes cannot
// prove fan-out. Hidden or unsafe key columns cannot enter the admitted binding.
func discoverUniqueKeys(ctx context.Context, tx readTransaction, oid int64, columns []readexec.Column) ([][]string, error) {
	rows, err := tx.Query(ctx, `SELECT array_agg(a.attname::text ORDER BY a.attname COLLATE "C") FROM pg_catalog.pg_index i CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum,position) JOIN pg_catalog.pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum AND NOT a.attisdropped WHERE i.indrelid=$1 AND i.indisunique AND i.indisvalid AND i.indisready AND i.indimmediate AND i.indpred IS NULL AND i.indexprs IS NULL AND NOT EXISTS (SELECT 1 FROM unnest(i.indclass) WITH ORDINALITY AS ic(opcoid,position) JOIN pg_catalog.pg_opclass opc ON opc.oid=ic.opcoid JOIN pg_catalog.pg_namespace ns ON ns.oid=opc.opcnamespace JOIN pg_catalog.pg_am am ON am.oid=opc.opcmethod WHERE ic.position<=i.indnkeyatts AND (ns.nspname<>'pg_catalog' OR NOT opc.opcdefault OR am.amname<>'btree')) AND NOT EXISTS (SELECT 1 FROM unnest(i.indcollation) WITH ORDINALITY AS co(coll,position) JOIN unnest(i.indkey) WITH ORDINALITY AS ik(attnum,position) USING(position) JOIN pg_catalog.pg_attribute ca ON ca.attrelid=i.indrelid AND ca.attnum=ik.attnum WHERE co.position<=i.indnkeyatts AND co.coll<>ca.attcollation) AND k.position<=i.indnkeyatts GROUP BY i.indexrelid,i.indnkeyatts HAVING count(*)=i.indnkeyatts ORDER BY i.indexrelid LIMIT 33`, oid)
	if err != nil {
		return nil, safe(err)
	}
	defer rows.Close()
	allowed := map[string]bool{}
	for _, col := range columns {
		allowed[col.Name] = col.Safe
	}
	var keys [][]string
	seen := map[string]bool{}
	count := 0
	for rows.Next() {
		count++
		if count > 32 {
			return nil, readexec.ErrLimit
		}
		var key []string
		if err = rows.Scan(&key); err != nil {
			return nil, safe(err)
		}
		complete := len(key) > 0 && len(key) <= 16
		for _, col := range key {
			complete = complete && allowed[col]
		}
		if !complete {
			continue
		}
		sort.Strings(key)
		canonical := strings.Join(key, "\x00")
		if !seen[canonical] {
			keys = append(keys, key)
			seen[canonical] = true
		}
	}
	if err = rows.Err(); err != nil {
		return nil, safe(err)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.Join(keys[i], "\x00") < strings.Join(keys[j], "\x00") })
	return keys, ctx.Err()
}
