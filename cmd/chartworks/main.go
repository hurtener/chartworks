// Command chartworks runs the service or its Pengui-authorized client commands.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hurtener/chartworks/internal/foundation"
)

var version = "dev"
var commit = "none"
var buildDate = "unknown"

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return foundation.CommandWithInput(ctx, os.Args[1:], os.LookupEnv, os.Stdin, os.Stdout, os.Stderr, foundation.Build{Version: version, Commit: commit, Date: buildDate}, foundation.Start)
}
func main() { os.Exit(run()) }
