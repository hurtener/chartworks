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
		maxInput, maxOutput := 16<<20, 16<<20
		if raw := os.Getenv("GOMEMLIMIT_BYTES"); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				os.Exit(1)
			}
			memory = value
		}
		for key, target := range map[string]*int{"RENDER_MAX_INPUT_BYTES": &maxInput, "RENDER_MAX_OUTPUT_BYTES": &maxOutput} {
			if raw := os.Getenv(key); raw != "" {
				value, err := strconv.Atoi(raw)
				if err != nil {
					os.Exit(1)
				}
				*target = value
			}
		}
		if rendering.WorkerMain(os.Stdin, os.Stdout, maxInput, maxOutput, memory) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
