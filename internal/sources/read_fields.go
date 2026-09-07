package sources

import (
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// readResultFields resolves deferred native errors before treating absent fields
// as a schema. pgx may return nil field descriptions after a server error and
// only publish that error from Err after Close. Passing that nil schema to the
// collector would disguise transaction timeouts as a row/schema limit failure.
func readResultFields(rows pgx.Rows) ([]pgconn.FieldDescription, error) {
	fields := rows.FieldDescriptions()
	if len(fields) == 0 {
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, readexec.ErrType
	}
	return fields, nil
}
