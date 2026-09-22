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
	if raw := os.Getenv("GOMEMLIMIT_BYTES"); raw != "" {
		if n, e := strconv.ParseInt(raw, 10, 64); e == nil {
			memory = n
		}
	}
	if err := rendering.WorkerMain(os.Stdin, os.Stdout, 16<<20, 16<<20, memory); err != nil {
		os.Exit(1)
	}
}
