package foundation

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/internal/telemetry"
)

// Build identifies a compiled binary without requiring Git at runtime.
type Build struct{ Version, Commit, Date string }

// Starter is the command's injected, cancellable service lifecycle.
type Starter func(context.Context, config.Config, io.Writer) error

const usage = "usage: chartworks version | config-check [--defaults | --config PATH] | serve --config PATH [--listen IP:PORT] | mcp\n"

// Command has deterministic exit codes and injectable environment, I/O and startup.
func Command(ctx context.Context, args []string, lookup func(string) (string, bool), stdout, stderr io.Writer, build Build, start Starter) int {
	write := func(w io.Writer, s string) bool { _, err := io.WriteString(w, s); return err == nil }
	if len(args) == 0 {
		write(stderr, usage)
		return 2
	}
	if args[0] == "version" {
		if len(args) != 1 {
			write(stderr, usage)
			return 2
		}
		if !write(stdout, fmt.Sprintf("chartworks %s (%s; %s)\n", build.Version, build.Commit, build.Date)) {
			return 1
		}
		return 0
	}
	if args[0] == "mcp" {
		write(stderr, "MCP transport is not implemented; phase 22 owns it. No listener was started.\n")
		return 3
	}
	if args[0] != "serve" && args[0] != "config-check" {
		write(stderr, usage)
		return 2
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", "", "configuration path")
	listen := flags.String("listen", "", "explicit loopback listener override")
	defaults := flags.Bool("defaults", false, "print typed defaults")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		write(stderr, usage)
		return 2
	}
	if *defaults {
		if args[0] != "config-check" || *path != "" || *listen != "" {
			write(stderr, usage)
			return 2
		}
		if config.WriteDefaults(stdout) != nil {
			return 1
		}
		return 0
	}
	if lookup == nil {
		write(stderr, "configuration environment lookup required\n")
		return 2
	}
	if *path == "" {
		*path, _ = lookup("CHARTWORKS_CONFIG")
	}
	if *path == "" {
		write(stderr, "configuration file required (--config or CHARTWORKS_CONFIG)\n")
		return 2
	}
	// #nosec G304 -- explicit operator-selected config path, never an HTTP parameter.
	f, err := os.Open(*path)
	if err != nil {
		write(stderr, "configuration file could not be read\n")
		return 2
	}
	cfg, err := config.Load(f, lookup, config.Overrides{Listen: *listen})
	closeErr := f.Close()
	if err != nil {
		write(stderr, err.Error()+"\n")
		return 2
	}
	if closeErr != nil {
		write(stderr, "configuration file could not be closed\n")
		return 2
	}
	if args[0] == "config-check" {
		if !write(stdout, "configuration valid; no connections or inference performed\n") {
			return 1
		}
		return 0
	}
	if start == nil {
		write(stderr, "service lifecycle unavailable\n")
		return 1
	}
	if err = start(ctx, cfg, stderr); err != nil {
		write(stderr, "service failed; inspect sanitized dependency status and configuration\n")
		return 1
	}
	return 0
}

// Start connects the actual store before binding, then runs health with trusted JWKS checks.
func Start(ctx context.Context, cfg config.Config, log io.Writer) error {
	v := cfg.Values()
	r, err := telemetry.New(log, v.Telemetry.LogFormat, v.Telemetry.Metrics)
	if err != nil {
		return err
	}
	db, err := postgres.Open(ctx, cfg.StoreDSN(), postgres.Options{MaxConns: v.Store.MaxConns, ConnectTimeout: time.Duration(v.Store.ConnectTimeout), TransactionTimeout: time.Duration(v.Store.TransactionTimeout), MigrationPolicy: v.Store.MigrationPolicy})
	if err != nil {
		return err
	}
	defer db.Close()
	keyProbe, err := auth.New(v.Auth, nil, nil)
	if err != nil {
		return err
	}
	service, err := securityapi.New(db)
	if err != nil {
		return err
	}
	defer keyProbe.Close()
	workCtx, stopWork := context.WithCancel(ctx)
	defer stopWork()
	active, err := setupWork(workCtx, v, db, keyProbe, securityapi.Handler(keyProbe, service, r, v.Telemetry.Metrics), os.LookupEnv, log)
	if err != nil {
		return err
	}
	defer active.close()
	s, err := NewServerWithRegistry(cfg, r, func(ctx context.Context) Dependency { return Dependency{Ready: db.Check(ctx) == nil} }, keyProbe.Check, active.registry, active.handler)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", v.Server.Listen)
	if err != nil {
		return errors.New("foundation: cannot bind configured listener")
	}
	active.run(ctx)
	err = s.Serve(ctx, listener)
	stopWork()
	return err
}
