package acceptance

import (
	"github.com/hurtener/chartworks/internal/rendering"
	"os"
	"strconv"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--sealed-render-worker" {
		memory := int64(1 << 30)
		if raw := os.Getenv("GOMEMLIMIT_BYTES"); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				os.Exit(1)
			}
			memory = value
		}
		if rendering.WorkerMain(os.Stdin, os.Stdout, 16<<20, 16<<20, memory) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
