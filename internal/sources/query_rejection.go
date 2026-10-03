package sources

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/exec/querydiagnostic"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

// postgresQueryRejection is shared by native EXPLAIN and actual read failures.
// Only already identified PgErrors at those SQL boundaries can carry a reason.
// A cancellation, lost journal or changed authority dominates a joined PgError.
func postgresQueryRejection(ctx context.Context, err error) error {
	if ctx == nil {
		return readexec.ErrBinding
	}
	if err == nil {
		return nil
	}
	for _, terminal := range []error{readexec.ErrUncertain, readexec.ErrCancelled, readexec.ErrTimeout, readexec.ErrBinding, readexec.ErrType, readexec.ErrLimit, readexec.ErrUnsupported, readexec.ErrUnsafe, readexec.ErrReplay, store.ErrUnavailable, store.ErrConflict, store.ErrInvalid, store.ErrNotFound, access.ErrUnauthenticated, access.ErrForbidden, access.ErrNotFound, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, terminal) {
			return safe(terminal)
		}
	}
	if ctx.Err() != nil {
		return readFailure(ctx, ctx.Err())
	}
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return safe(err)
	}
	if pg.Code == "57014" || pg.Code == "25P03" || pg.Code == "25P04" {
		return readexec.ErrTimeout
	}
	if code := querydiagnostic.Postgres(pg.Code); code != "" {
		return readexec.QueryRejection(code)
	}
	return safe(err)
}
