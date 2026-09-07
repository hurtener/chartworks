package sources

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Model pgx's documented deferred-error contract: Err is intentionally empty
// until Close, even though FieldDescriptions is already missing.
type deferredFieldRows struct {
	pgx.Rows
	fields []pgconn.FieldDescription
	closed bool
	cause  error
}

func (r *deferredFieldRows) FieldDescriptions() []pgconn.FieldDescription { return r.fields }
func (r *deferredFieldRows) Close()                                       { r.closed = true }
func (r *deferredFieldRows) Err() error {
	if r.closed {
		return r.cause
	}
	return nil
}

func TestDeferredNativeErrorPrecedesMissingSchema(t *testing.T) {
	for _, code := range []string{"57014", "25P03", "25P04"} {
		t.Run(code, func(t *testing.T) {
			native := &pgconn.PgError{Code: code, Message: "PRIVATE_NATIVE_DIAGNOSTIC"}
			rows := &deferredFieldRows{cause: fmt.Errorf("native wrapper: %w", native)}
			fields, err := readResultFields(rows)
			if fields != nil || !rows.closed || !errors.Is(err, native) || readFailure(context.Background(), err) != readexec.ErrTimeout {
				t.Fatal("deferred native timeout became a schema or limit error", code, err)
			}
		})
	}
	rows := &deferredFieldRows{cause: errors.New("PRIVATE_NATIVE_DIAGNOSTIC")}
	if fields, err := readResultFields(rows); fields != nil || !rows.closed || readFailure(context.Background(), err) != store.ErrUnavailable {
		t.Fatal("unknown native error was exposed or became an empty success", err)
	}
	rows = &deferredFieldRows{}
	if fields, err := readResultFields(rows); fields != nil || !rows.closed || !errors.Is(err, readexec.ErrType) {
		t.Fatal("missing fields without a native error were accepted", err)
	}
	fields := []pgconn.FieldDescription{{Name: "id", DataTypeOID: 20}}
	rows = &deferredFieldRows{fields: fields}
	got, err := readResultFields(rows)
	if err != nil || rows.closed || !reflect.DeepEqual(got, fields) {
		t.Fatal("valid schema was changed or its stream closed prematurely", err)
	}
}

func TestDeferredTimeoutRetainsCancellationAndUncertaintyPriority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	native := &pgconn.PgError{Code: "25P04"}
	if readFailure(ctx, native) != readexec.ErrCancelled {
		t.Fatal("explicit cancellation was relabeled as server timeout")
	}
	if readFailure(ctx, readexec.ErrUncertain) != readexec.ErrUncertain {
		t.Fatal("unproven termination was disguised as confirmed cancellation")
	}
}
