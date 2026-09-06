package sources

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/jackc/pgx/v5/pgconn"
	"math"
	"testing"
	"time"
)

func TestReadNativeResultTypes(t *testing.T) {
	for _, oid := range []uint32{16, 17, 20, 21, 23, 25, 114, 700, 701, 790, 1042, 1043, 1082, 1083, 1114, 1184, 1186, 1266, 1700, 2950, 3802} {
		f, err := resultField("value", oid)
		if err != nil || f.Name != "value" || f.NativeType == "" {
			t.Fatal("qualified output missing", oid, err)
		}
	}
	if _, err := resultField("value", 999999); !errors.Is(err, readexec.ErrType) {
		t.Fatal("unqualified native output")
	}
	for _, c := range []struct {
		n    int64
		want string
	}{{0, "0.00"}, {1, "0.01"}, {1234, "12.34"}, {-1, "-0.01"}, {math.MinInt64, "-92233720368547758.08"}, {math.MaxInt64, "92233720368547758.07"}} {
		var raw bytes.Buffer
		if err := binary.Write(&raw, binary.BigEndian, c.n); err != nil {
			t.Fatal(err)
		}
		got, err := moneyDecimal(raw.Bytes())
		if err != nil || string(got) != c.want {
			t.Fatal("money precision", c.n, string(got), err)
		}
	}
	if _, err := moneyDecimal([]byte{1}); err == nil {
		t.Fatal("malformed money coerced")
	}
	if err := readFailure(context.Background(), &pgconn.PgError{Code: "57014", Message: "PRIVATE"}); !errors.Is(err, readexec.ErrTimeout) {
		t.Fatal("server timeout unclassified")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := readFailure(cancelled, errors.New("PRIVATE")); !errors.Is(err, readexec.ErrCancelled) {
		t.Fatal("cancellation unclassified")
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if err := readFailure(expired, errors.New("PRIVATE")); !errors.Is(err, readexec.ErrTimeout) {
		t.Fatal("deadline unclassified")
	}
	if err := readFailure(expired, readexec.ErrUncertain); !errors.Is(err, readexec.ErrUncertain) {
		t.Fatal("unknown outcome disguised as definite timeout")
	}
}

func TestReadNativeTransactionTimeoutCodes(t *testing.T) {
	for _, code := range []string{"25P03", "25P04"} {
		if err := readFailure(context.Background(), &pgconn.PgError{Code: code, Message: "PRIVATE_ERROR_CANARY"}); !errors.Is(err, readexec.ErrTimeout) {
			t.Fatal("transaction deadline lost its typed category", code, err)
		}
	}
}
