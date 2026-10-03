//go:build ignore

package main

import (
	"os"

	"github.com/hurtener/chartworks/internal/rendering"
)

func main() {
	if os.Getenv("SECRET_CANARY") != "" || os.Getenv("GOMAXPROCS") != "1" {
		os.Exit(7)
	}
	if rendering.WorkerMain(os.Stdin, os.Stdout, 4<<20, 4<<20, 1<<30) != nil {
		os.Exit(1)
	}
}
