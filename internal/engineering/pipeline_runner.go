package engineering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

type pipelineRunnerReceipt struct {
	RenderedHash, ValidationHash, LineageHash string
	Started, Finished                         time.Time
}
type pipelineRunnerTarget struct {
	Schema, Table, Application string
	Inputs                     []readexec.Relation
}
type boundedPipelineOutput struct {
	mu       sync.Mutex
	data     bytes.Buffer
	limit    int
	exceeded bool
	stop     context.CancelFunc
}

func (b *boundedPipelineOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if b.data.Len()+n > b.limit {
		b.exceeded = true
		if b.stop != nil {
			b.stop()
		}
		return 0, ErrLimit
	}
	return b.data.Write(p)
}

func verifyPipelineRunner(v config.Pipelines) error {
	file, err := os.Open(v.RunnerPath)
	if err != nil {
		return ErrUnavailable
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 512<<20 {
		return ErrInvalid
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return ErrUnavailable
	}
	if hex.EncodeToString(hash.Sum(nil)) != v.RunnerSHA256 {
		return ErrInvalid
	}
	return nil
}

// runnerCommand always executes the pinned operator path without a shell. Raw
// subprocess output stays private and bounded; errors contain no SQL/credentials.
func runnerCommand(ctx context.Context, v config.Pipelines, dir string, env []string, args ...string) ([]byte, error) {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(work, v.RunnerPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = time.Second
	if err := configurePipelineProcess(cmd); err != nil {
		return nil, err
	}
	output := &boundedPipelineOutput{limit: v.MaxOutputBytes, stop: cancel}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	if output.exceeded {
		return nil, ErrLimit
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		stage := "command"
		if len(args) > 0 {
			switch args[0] {
			case "validate", "lineage", "render", "run":
				stage = args[0]
			}
		}
		return nil, fmt.Errorf("pipeline runner %s: %w", stage, ErrUnavailable)
	}
	return append([]byte(nil), output.data.Bytes()...), nil
}
func pipelineAsset(step PipelineStep, target pipelineRunnerTarget, statement string, materialize bool) ([]byte, error) {
	asset := map[string]any{"name": target.Schema + "." + target.Table, "type": "pg.sql", "connection": "managed"}
	dependencies := make([]map[string]string, len(target.Inputs))
	for i, input := range target.Inputs {
		dependencies[i] = map[string]string{"asset": input.Schema + "." + input.Name}
	}
	asset["depends"] = dependencies
	if materialize {
		strategy := map[string]string{"replace": "create+replace", "append": "append", "incremental": "delete+insert", "merge": "merge", "interval": "time_interval", "scd2": "scd2_by_column"}[step.Strategy]
		if strategy == "" {
			return nil, ErrInvalid
		}
		mat := map[string]any{"type": "table", "strategy": strategy}
		if step.Key != "" {
			mat["incremental_key"] = step.Key
		}
		if step.Strategy == "interval" {
			mat["incremental_key"] = step.TimeColumn
			mat["time_granularity"] = "timestamp"
		}
		asset["materialization"] = mat
		columns := make([]map[string]any, len(step.Columns))
		for i, c := range step.Columns {
			columns[i] = map[string]any{"name": c.Name, "type": c.Type, "primary_key": c.PrimaryKey, "update_on_merge": !c.PrimaryKey}
		}
		asset["columns"] = columns
	}
	meta, err := yaml.Marshal(asset)
	if err != nil {
		return nil, ErrInvalid
	}
	return []byte("/* @bruin\n" + string(meta) + "@bruin */\n" + statement + "\n"), nil
}
func runPipelineAsset(ctx context.Context, v config.Pipelines, step PipelineStep, target pipelineRunnerTarget, statement string, writer *pgx.ConnConfig, fresh bool) (receipt pipelineRunnerReceipt, err error) {
	if config.ValidatePipelines(v) != nil || !v.Enabled || writer == nil || !readexec.SQLIdentifier(target.Schema) || !readexec.SQLIdentifier(target.Table) || target.Application == "" {
		return receipt, ErrInvalid
	}
	if err = verifyPipelineRunner(v); err != nil {
		return receipt, fmt.Errorf("pipeline runner artifact: %w", err)
	}
	if err = validatePipelineTempFS(v.TempDir); err != nil {
		return receipt, fmt.Errorf("pipeline runner scratch: %w", err)
	}
	dir, err := os.MkdirTemp(v.TempDir, "cw-pipeline-")
	if err != nil {
		return receipt, ErrUnavailable
	}
	defer os.RemoveAll(dir)
	if err = os.Chmod(dir, 0700); err != nil {
		return receipt, ErrUnavailable
	}
	for _, sub := range []string{"assets", "home", "tmp", ".git"} {
		if err = os.Mkdir(filepath.Join(dir, sub), 0700); err != nil {
			return receipt, ErrUnavailable
		}
	}
	if err = os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/pipeline\n"), 0600); err != nil {
		return receipt, ErrUnavailable
	}
	pipeline := []byte("name: chartworks-managed\ndefault_connections:\n  postgres: managed\n")
	if err = os.WriteFile(filepath.Join(dir, "pipeline.yml"), pipeline, 0600); err != nil {
		return receipt, ErrUnavailable
	}
	// Only the transient environment has the credential. The generated config
	// contains references and is deleted with its private memory-backed directory.
	conf := map[string]any{"default_environment": "managed", "environments": map[string]any{"managed": map[string]any{"connections": map[string]any{"postgres": []any{map[string]any{"name": "managed", "host": writer.Host, "port": writer.Port, "database": writer.Database, "username": writer.User, "password": "${CW_PIPELINE_PASSWORD}", "ssl_mode": "disable", "pool_max_conns": 1, "schema": "pg_catalog"}}}}}}
	tlsEnv, mode, tlsErr := pipelineTLSFiles(dir, writer)
	if tlsErr != nil {
		return receipt, fmt.Errorf("pipeline runner TLS configuration: %w", tlsErr)
	}
	connection := conf["environments"].(map[string]any)["managed"].(map[string]any)["connections"].(map[string]any)["postgres"].([]any)[0].(map[string]any)
	connection["ssl_mode"] = mode
	raw, err := yaml.Marshal(conf)
	if err != nil {
		return receipt, ErrInvalid
	}
	configPath := filepath.Join(dir, ".bruin.yml")
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		return receipt, ErrUnavailable
	}
	timeout := time.Duration(v.Timeout)
	if until, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(until))
	}
	if timeout <= 0 {
		return receipt, context.DeadlineExceeded
	}
	env := []string{"HOME=" + filepath.Join(dir, "home"), "TMPDIR=" + filepath.Join(dir, "tmp"), "PATH=/usr/bin:/bin", "TZ=UTC", "TELEMETRY_OPTOUT=true", "CW_PIPELINE_PASSWORD=" + writer.Password, "PGAPPNAME=" + target.Application, "PGOPTIONS=-c statement_timeout=" + strconv.FormatInt(timeout.Milliseconds(), 10) + " -c lock_timeout=1000 -c idle_in_transaction_session_timeout=2000"}
	env = append(env, tlsEnv...)
	assetPath := filepath.Join(dir, "assets", "step.sql")
	asset, err := pipelineAsset(step, target, statement, true)
	if err != nil {
		return receipt, err
	}
	if err = os.WriteFile(assetPath, asset, 0600); err != nil {
		return receipt, ErrUnavailable
	}
	for index, input := range target.Inputs {
		descriptor := map[string]any{"name": input.Schema + "." + input.Name, "uri": "chartworks:" + input.ID, "type": "pg.source", "connection": "managed"}
		content, encodeErr := yaml.Marshal(descriptor)
		if encodeErr != nil {
			return receipt, ErrInvalid
		}
		if err = os.WriteFile(filepath.Join(dir, "assets", fmt.Sprintf("source_%d.asset.yml", index)), content, 0600); err != nil {
			return receipt, ErrUnavailable
		}
	}
	validate, err := runnerCommand(ctx, v, dir, env, "validate", "--fast", "--output", "json", "--config-file", configPath, dir)
	if err != nil {
		return receipt, err
	}
	if !validPipelineValidation(validate) {
		return receipt, fmt.Errorf("pipeline validator report: %w", ErrInvalid)
	}
	receipt.ValidationHash = readexec.Hash(json.RawMessage(validate))
	lineage, err := runnerCommand(ctx, v, dir, env, "lineage", "--output", "json", assetPath)
	if err != nil {
		return receipt, err
	}
	if !validPipelineLineage(lineage, target, dir) {
		return receipt, fmt.Errorf("pipeline lineage report: %w", ErrInvalid)
	}
	receipt.LineageHash = readexec.Hash(json.RawMessage(lineage))
	args := []string{"render", "--output", "json", "--config-file", configPath}
	if fresh {
		args = append(args, "--full-refresh")
	}
	if step.Strategy == "interval" {
		args = append(args, "--start-date", step.Start, "--end-date", step.End)
	}
	args = append(args, assetPath)
	rendered, err := runnerCommand(ctx, v, dir, env, args...)
	if err != nil {
		return receipt, err
	}
	var doc struct {
		Query string `json:"query"`
	}
	if json.Unmarshal(rendered, &doc) != nil || doc.Query == "" {
		return receipt, fmt.Errorf("pipeline rendered report: %w", ErrInvalid)
	}
	if err = validatePipelineRendered(doc.Query, target.Schema, target.Table); err != nil {
		return receipt, fmt.Errorf("pipeline rendered target: %w", err)
	}
	receipt.RenderedHash = readexec.Hash(doc.Query)
	// Execute the exact inspected renderer bytes as a SQL-only asset. Bruin is
	// not permitted to re-render materialization into a different physical target.
	asset, err = pipelineAsset(step, target, doc.Query, false)
	if err != nil {
		return receipt, err
	}
	if err = os.WriteFile(assetPath, asset, 0600); err != nil {
		return receipt, ErrUnavailable
	}
	receipt.Started = time.Now().UTC()
	_, err = runnerCommand(ctx, v, dir, env, "run", "--no-validation", "--only", "main", "--workers", "1", "--timeout", strconv.Itoa(max(1, int(timeout.Seconds()))), "--no-log-file", "--no-color", "--no-timestamp", "--config-file", configPath, assetPath)
	receipt.Finished = time.Now().UTC()
	if err != nil {
		return receipt, errors.Join(ErrPipelineUncertain, err)
	}
	return receipt, nil
}

// pipelineTLSFiles preserves only the approved DSN TLS settings. Certificates
// and key bytes remain in the private tmpfs directory, never argv or logs.
func pipelineTLSFiles(dir string, writer *pgx.ConnConfig) ([]string, string, error) {
	u, err := url.Parse(writer.ConnString())
	if err != nil {
		return nil, "", ErrInvalid
	}
	mode := u.Query().Get("sslmode")
	if mode == "disable" && writer.TLSConfig == nil {
		return nil, mode, nil
	}
	if mode != "verify-full" || writer.TLSConfig == nil {
		return nil, "", ErrInvalid
	}
	env := []string{}
	for key, name := range map[string]string{"sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY"} {
		path := u.Query().Get(key)
		if path == "" {
			continue
		}
		if key == "sslrootcert" && path == "system" {
			env = append(env, name+"=system")
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, "", ErrUnavailable
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, 1<<20+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(raw) == 0 || len(raw) > 1<<20 {
			return nil, "", ErrInvalid
		}
		target := filepath.Join(dir, key+".pem")
		if os.WriteFile(target, raw, 0600) != nil {
			return nil, "", ErrUnavailable
		}
		env = append(env, name+"="+target)
	}
	// An implicit ambient certificate could differ in the child environment.
	// Require the operator to name that trust/key material explicitly.
	if writer.TLSConfig.RootCAs != nil && u.Query().Get("sslrootcert") == "" || len(writer.TLSConfig.Certificates) > 0 && (u.Query().Get("sslcert") == "" || u.Query().Get("sslkey") == "") {
		return nil, "", ErrInvalid
	}
	return env, mode, nil
}

// Exit status alone is not reliable for this pinned validator. Require exactly
// one known pipeline report and no asset or pipeline issues of any severity.
func validPipelineValidation(raw []byte) bool {
	var reports []struct {
		Pipeline string                       `json:"pipeline"`
		Issues   map[string][]json.RawMessage `json:"issues"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&reports) != nil || len(reports) != 1 || reports[0].Pipeline != "chartworks-managed" || reports[0].Issues == nil {
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return false
	}
	for _, issues := range reports[0].Issues {
		if len(issues) > 0 {
			return false
		}
	}
	return true
}

func validPipelineLineage(raw []byte, target pipelineRunnerTarget, dir string) bool {
	type file struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Content string `json:"content,omitempty"`
		Type    string `json:"type,omitempty"`
	}
	var report struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Upstreams []struct {
			Name       string `json:"name"`
			Type       string `json:"type"`
			Executable file   `json:"executable_file"`
			Definition file   `json:"definition_file"`
		} `json:"upstreams"`
		Downstream []json.RawMessage `json:"downstream"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil {
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return false
	}
	if report.Name != target.Schema+"."+target.Table || report.Type != "pg.sql" || len(report.Downstream) != 0 || len(report.Upstreams) != len(target.Inputs) {
		return false
	}
	expected := map[string]string{}
	for index, input := range target.Inputs {
		expected[input.Schema+"."+input.Name] = fmt.Sprintf("source_%d.asset.yml", index)
	}
	for _, upstream := range report.Upstreams {
		name, ok := expected[upstream.Name]
		if !ok || upstream.Type != "pg.source" {
			return false
		}
		path := filepath.Join(dir, "assets", name)
		if upstream.Executable.Name != name || upstream.Executable.Path != path || upstream.Executable.Content != "" || upstream.Executable.Type != "" || upstream.Definition.Name != name || upstream.Definition.Path != path || upstream.Definition.Type != "yaml" || upstream.Definition.Content != "" {
			return false
		}
		delete(expected, upstream.Name)
	}
	return len(expected) == 0
}
