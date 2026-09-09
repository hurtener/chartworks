// Package clientcli implements the thin, token-forwarding operator CLI. Business
// validation, authority and state remain in the registered Chartworks services.
package clientcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

// IO supplies all process I/O and environment access. OpenDescriptor transfers
// ownership of an explicitly selected inherited token descriptor to Command.
// It is never invoked implicitly, or by the configuration inspection command.
type IO struct {
	Stdin io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Lookup func(string) (string, bool)
	HTTP *http.Client
	OpenDescriptor func(int) (*os.File, error)
}

const usage = "usage: chartworks client config|operations|diagnostics|call OPERATION|mcp [--url URL] [--token-env NAME | --token-fd N] [--timeout DURATION] [--execute] [--input -] [--id ID] [--query NAME=VALUE] [--idempotency-key KEY] [--attempts 1..3] [--schemas] [--mcp-token-env NAME]\n"

// Command has deterministic exit codes: 0 success, 1 dependency/output failure,
// 2 invalid invocation, 3 denied authority, 4 conflict/expired object, 124 deadline,
// and 130 cancellation. Neither argument values nor dependency errors are echoed.
func Command(ctx context.Context, args []string, streams IO) int {
	if ctx == nil || streams.Stdout == nil || streams.Stderr == nil || streams.Lookup == nil {
		return 2
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		return writeUsage(streams.Stdout, 0)
	}
	if len(args) == 0 {
		return writeUsage(streams.Stderr, 2)
	}
	command, rest := args[0], args[1:]
	operation := ""
	switch command {
	case "call":
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return writeUsage(streams.Stderr, 2)
		}
		operation, rest = rest[0], rest[1:]
	case "config", "operations", "diagnostics", "mcp":
	default:
		return writeUsage(streams.Stderr, 2)
	}
	base, _ := streams.Lookup("CHARTWORKS_CLIENT_URL")
	defaultTimeout := cw.DefaultRequestTimeout
	if configured, ok := streams.Lookup("CHARTWORKS_CLIENT_TIMEOUT"); ok {
		var err error
		defaultTimeout, err = time.ParseDuration(configured)
		if err != nil {
			return writeUsage(streams.Stderr, 2)
		}
	}
	flags := flag.NewFlagSet("client", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseURL := flags.String("url", base, "trusted backend URL including an optional configured prefix")
	timeout := flags.Duration("timeout", defaultTimeout, "whole-command deadline")
	tokenEnv := flags.String("token-env", "", "name of caller-populated token environment variable")
	tokenFD := flags.Int("token-fd", -1, "explicit inherited regular-file token descriptor (3..1024)")
	mcpEnv := flags.String("mcp-token-env", "", "separate MCP-audience token environment for the matrix")
	input := flags.String("input", "", "read request bytes from standard input: -")
	id := flags.String("id", "", "registered resource path identifier")
	key := flags.String("idempotency-key", "", "stable logical operation key")
	attempts := flags.Int("attempts", 1, "explicit bounded attempts for a replayable operation")
	execute := flags.Bool("execute", false, "acknowledge that an API call may persist, spend or erase")
	schemas := flags.Bool("schemas", false, "include full request and response schemas in the matrix")
	query := queryFlags{values: make(url.Values)}
	flags.Var(&query, "query", "one registered query parameter NAME=VALUE; repeat for distinct names")
	if err := flags.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeUsage(streams.Stdout, 0)
		}
		return writeUsage(streams.Stderr, 2)
	}
	if flags.NArg() != 0 || *timeout <= 0 || *timeout > cw.MaximumRequestTimeout || *attempts < 1 || *attempts > 3 || (*input != "" && *input != "-") || (*tokenFD != -1 && (*tokenFD < 3 || *tokenFD > 1024)) || (*tokenFD != -1 && *tokenEnv != "") {
		return writeUsage(streams.Stderr, 2)
	}
	if command != "call" && (*id != "" || *key != "" || len(query.values) != 0 || *attempts != 1) || command != "operations" && (*schemas || *mcpEnv != "") || command != "call" && command != "mcp" && (*input != "" || *execute) {
		return writeUsage(streams.Stderr, 2)
	}
	if (command == "call" || command == "mcp") && !*execute {
		_, _ = io.WriteString(streams.Stderr, "chartworks: explicit API calls require --execute; Pengui scopes are still enforced\n")
		return 2
	}
	if *tokenEnv == "" {
		*tokenEnv = "CHARTWORKS_TOKEN"
	}
	if !environmentName(*tokenEnv) || *mcpEnv != "" && !environmentName(*mcpEnv) {
		return writeUsage(streams.Stderr, 2)
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	httpClient := http.Client{Timeout: *timeout}
	if streams.HTTP != nil {
		httpClient = *streams.HTTP
		httpClient.Timeout = *timeout
	}
	provider := environmentProvider(streams.Lookup, *tokenEnv)
	// Validate URL and typed bounds before opening or reading a credential source.
	client, err := cw.New(*baseURL, &httpClient, provider)
	if err != nil {
		return writeUsage(streams.Stderr, 2)
	}
	if command == "config" {
		source := "environment"
		if *tokenFD != -1 {
			source = "explicit_regular_file_descriptor"
		}
		return writeJSON(streams.Stdout, struct {
			URL string `json:"base_url"`
			Timeout string `json:"timeout"`
			TokenSource string `json:"token_source"`
		}{*baseURL, timeout.String(), source})
	}
	if *tokenFD != -1 {
		if streams.OpenDescriptor == nil {
			return fail(streams.Stderr, cw.ErrInvalidCall)
		}
		file, openErr := streams.OpenDescriptor(*tokenFD)
		if openErr != nil || file == nil {
			return fail(streams.Stderr, errors.New("credential descriptor unavailable"))
		}
		defer func() { _ = file.Close() }()
		provider = descriptorProvider(file)
		client, err = cw.New(*baseURL, &httpClient, provider)
		if err != nil {
			return fail(streams.Stderr, err)
		}
	}
	switch command {
	case "operations":
		var mcpClient *cw.Client
		if *mcpEnv != "" {
			mcpClient, err = cw.New(*baseURL, &httpClient, environmentProvider(streams.Lookup, *mcpEnv))
			if err != nil {
				return fail(streams.Stderr, err)
			}
		}
		rows, err := client.OperationMatrix(ctx, mcpClient)
		if err != nil {
			return fail(streams.Stderr, err)
		}
		if !*schemas {
			for i := range rows {
				rows[i].RequestSchema, rows[i].ResponseSchema = nil, nil
			}
		}
		return writeJSON(streams.Stdout, rows)
	case "diagnostics":
		checks, err := client.Diagnostics(ctx)
		if err != nil {
			return fail(streams.Stderr, err)
		}
		return writeJSON(streams.Stdout, checks)
	case "mcp":
		// Check the explicitly supplied source before consuming request input.
		if _, err = provider(ctx); err != nil {
			return fail(streams.Stderr, err)
		}
		if *input != "-" || streams.Stdin == nil {
			return fail(streams.Stderr, cw.ErrInvalidCall)
		}
		body, err := readInput(ctx, streams.Stdin, 10<<20)
		if err != nil {
			return fail(streams.Stderr, err)
		}
		if !json.Valid(body) {
			return fail(streams.Stderr, cw.ErrInvalidCall)
		}
		result, err := client.MCP(ctx, body)
		if err != nil {
			return fail(streams.Stderr, err)
		}
		return writeBody(streams.Stdout, result)
	case "call":
		// Metadata lookup supplies the actual body bound before stdin is read.
		// Invoke rechecks the current contract. --execute is mandatory even for
		// generic reads, so a deployment change cannot bypass effect consent.
		rows, err := client.Operations(ctx)
		if err != nil {
			return fail(streams.Stderr, err)
		}
		var selected *cw.OperationInfo
		for i := range rows {
			if rows[i].ID == operation {
				selected = &rows[i]
				break
			}
		}
		if selected == nil {
			return fail(streams.Stderr, cw.ErrUnknownOperation)
		}
		if selected.Audience != "http" {
			return fail(streams.Stderr, cw.ErrInvalidCall)
		}
		var body []byte
		if *input == "-" {
			if streams.Stdin == nil || selected.MaxBodyBytes < 1 {
				return fail(streams.Stderr, cw.ErrInvalidCall)
			}
			body, err = readInput(ctx, streams.Stdin, selected.MaxBodyBytes)
			if err != nil {
				return fail(streams.Stderr, err)
			}
		}
		result, err := client.Invoke(ctx, operation, cw.CallOptions{ResourceID: *id, Query: query.values, Body: body, IdempotencyKey: *key, Attempts: *attempts})
		if err != nil {
			return fail(streams.Stderr, err)
		}
		return writeBody(streams.Stdout, result.Body)
	}
	return 2
}

type queryFlags struct{ values url.Values }
func (*queryFlags) String() string { return "" }
func (q *queryFlags) Set(value string) error {
	name, content, ok := strings.Cut(value, "=")
	if !ok || name == "" || len(value) > 16<<10 || len(q.values) >= 64 {
		return cw.ErrInvalidCall
	}
	q.values.Add(name, content)
	return nil
}

func environmentName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for i, r := range name {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}
func environmentProvider(lookup func(string) (string, bool), name string) cw.TokenProvider {
	return func(ctx context.Context) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		token, ok := lookup(name)
		if !ok || token == "" {
			return "", errors.New("credential unavailable")
		}
		return token, nil
	}
}
func descriptorProvider(file *os.File) cw.TokenProvider {
	return func(ctx context.Context) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 64<<10 {
			return "", errors.New("credential descriptor unavailable")
		}
		// Regular-file ReadAt is bounded and does not advance the descriptor.
		// Pipes/sockets are rejected rather than spawning an uncancellable read.
		data := make([]byte, (64<<10)+1)
		n, err := file.ReadAt(data, 0)
		if err != nil && !errors.Is(err, io.EOF) || n > 64<<10 {
			return "", errors.New("credential descriptor unavailable")
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return string(bytes.TrimSpace(data[:n])), nil
	}
}
func readInput(ctx context.Context, reader io.Reader, limit int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		done := make(chan struct{})
		stop := context.AfterFunc(ctx, func() { _ = closer.Close(); close(done) })
		defer func() {
			if !stop() {
				<-done
			}
		}()
	}
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, cw.ErrInvalidCall
	}
	return data, nil
}
func writeUsage(writer io.Writer, code int) int {
	if _, err := io.WriteString(writer, usage); err != nil {
		return 1
	}
	return code
}
func writeJSON(writer io.Writer, value any) int {
	if json.NewEncoder(writer).Encode(value) != nil {
		return 1
	}
	return 0
}
func writeBody(writer io.Writer, body []byte) int {
	if len(body) == 0 {
		return 0
	}
	if _, err := writer.Write(body); err != nil {
		return 1
	}
	if !bytes.HasSuffix(body, []byte("\n")) {
		if _, err := io.WriteString(writer, "\n"); err != nil {
			return 1
		}
	}
	return 0
}
func fail(writer io.Writer, err error) int {
	code := 1
	var status *cw.StatusError
	switch {
	case errors.Is(err, context.Canceled):
		code = 130
	case errors.Is(err, context.DeadlineExceeded):
		code = 124
	case errors.Is(err, cw.ErrInvalidCall), errors.Is(err, cw.ErrUnknownOperation), errors.Is(err, cw.ErrUnsafeRetry):
		code = 2
	case errors.As(err, &status):
		if status.Status == 401 || status.Status == 403 {
			code = 3
		} else if status.Status == 409 || status.Status == 410 {
			code = 4
		}
	}
	if status != nil {
		_, _ = fmt.Fprintf(writer, "chartworks: request rejected (HTTP %d)\n", status.Status)
	} else {
		_, _ = io.WriteString(writer, "chartworks: command failed; check supplied configuration, authority and operation status\n")
	}
	return code
}
