// Command chartworks-renderer is the isolated, credential-free static renderer.
package main

import (
	"os"
	"strconv"

	"github.com/hurtener/chartworks/internal/rendering"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--sealed-render-worker" {
		os.Exit(64)
	}
	memory := int64(256 << 20)
	maxInput, maxOutput := 16<<20, 16<<20
	if raw := os.Getenv("GOMEMLIMIT_BYTES"); raw != "" {
		if n, e := strconv.ParseInt(raw, 10, 64); e == nil {
			memory = n
		}
	}
	for key, target := range map[string]*int{"RENDER_MAX_INPUT_BYTES": &maxInput, "RENDER_MAX_OUTPUT_BYTES": &maxOutput} {
		if raw := os.Getenv(key); raw != "" {
			if n, e := strconv.Atoi(raw); e == nil && n >= 1024 && n <= 64<<20 {
				*target = n
			} else {
				os.Exit(64)
			}
		}
	}
	if err := rendering.WorkerMain(os.Stdin, os.Stdout, maxInput, maxOutput, memory); err != nil {
		os.Exit(1)
	}
}
