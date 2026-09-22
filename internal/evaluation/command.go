package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const commandUsage = "usage: chartworks eval gate|inspect --suite PATH [--run-id ID] | perf-inspect|perf-smoke --profile PATH [--report PATH]\n"

// Command runs a reviewed fixture manifest. Live execution is available only
// through Service with injected production dependencies and current authority.
func Command(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil || stdout == nil || stderr == nil || len(args) == 0 {
		_, _ = io.WriteString(stderr, commandUsage)
		return 2
	}
	verb := args[0]
	if verb == "perf-inspect" || verb == "perf-smoke" {
		return performanceCommand(ctx, verb, args[1:], stdout, stderr)
	}
	if verb != "gate" && verb != "inspect" {
		_, _ = io.WriteString(stderr, commandUsage)
		return 2
	}
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("suite", "", "reviewed suite manifest")
	runID := fs.String("run-id", "fixture-run", "bounded run identifier")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *path == "" || !identifier(*runID) {
		_, _ = io.WriteString(stderr, commandUsage)
		return 2
	}
	// #nosec G304 -- this is an explicit operator-selected local manifest.
	f, err := os.Open(*path)
	if err != nil {
		_, _ = io.WriteString(stderr, "evaluation manifest unavailable\n")
		return 2
	}
	defer func() { _ = f.Close() }()
	r := io.LimitReader(f, 1<<20+1)
	raw, err := io.ReadAll(r)
	if err != nil || len(raw) > 1<<20 {
		_, _ = io.WriteString(stderr, "evaluation manifest invalid\n")
		return 2
	}
	if err = rejectDuplicateJSON(raw); err != nil {
		_, _ = io.WriteString(stderr, "evaluation manifest invalid\n")
		return 2
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var suite Suite
	if dec.Decode(&suite) != nil || suite.Validate() != nil {
		_, _ = io.WriteString(stderr, "evaluation manifest invalid\n")
		return 2
	}
	if verb == "inspect" {
		summary := struct {
			ID          string    `json:"id"`
			Revision    int64     `json:"revision"`
			Mode        Mode      `json:"mode"`
			Calibration string    `json:"calibration"`
			Cases       int       `json:"cases"`
			Threshold   Threshold `json:"threshold"`
		}{suite.ID, suite.Revision, suite.Mode, suite.Calibration, len(suite.Cases), suite.Threshold}
		return encodeOutput(stdout, summary)
	}
	if suite.Mode != Fixture {
		_, _ = io.WriteString(stderr, "live evaluation requires the authority-bound runtime\n")
		return 2
	}
	report, evalErr := Evaluate(ctx, *runID, suite, nil, nil)
	if encodeOutput(stdout, report) != 0 {
		return 1
	}
	if evalErr != nil {
		_, _ = fmt.Fprintln(stderr, "evaluation gate failed")
		return 1
	}
	return 0
}

func performanceCommand(ctx context.Context, verb string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("eval-performance", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("profile", "", "performance profile")
	reportPath := fs.String("report", "", "protected report output")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *path == "" {
		_, _ = io.WriteString(stderr, commandUsage)
		return 2
	}
	raw, err := readBoundedJSON(*path)
	if err != nil || rejectDuplicateJSON(raw) != nil {
		_, _ = io.WriteString(stderr, "performance profile invalid\n")
		return 2
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var profile PerformanceManifest
	if dec.Decode(&profile) != nil {
		_, _ = io.WriteString(stderr, "performance profile invalid\n")
		return 2
	}
	runtimeEnvironment := RuntimePerformanceEnvironment(profile.Environment.RunnerLabel)
	profile.Environment.OS = runtimeEnvironment.OS
	profile.Environment.Architecture = runtimeEnvironment.Architecture
	profile.Environment.CPUs = runtimeEnvironment.CPUs
	profile.Environment.GoVersion = runtimeEnvironment.GoVersion
	if profile.Validate() != nil {
		_, _ = io.WriteString(stderr, "performance profile invalid\n")
		return 2
	}
	if verb == "perf-inspect" {
		summary := struct {
			ID           string                  `json:"id"`
			Kind         PerformanceProfileKind  `json:"kind"`
			EvidenceMode PerformanceEvidenceMode `json:"evidence_mode"`
			Environment  PerformanceEnvironment  `json:"environment"`
			Steps        int                     `json:"steps"`
		}{profile.ID, profile.Kind, profile.EvidenceMode, profile.Environment, len(profile.Steps)}
		return encodeOutput(stdout, summary)
	}
	if profile.Kind != PerformanceSmoke || profile.EvidenceMode != PerformanceSynthetic {
		_, _ = io.WriteString(stderr, "final, integration, and live performance profiles require the release runtime\n")
		return 2
	}
	report, runErr := MeasurePerformance(ctx, profile, newSyntheticPerformanceRunner(), nil)
	if *reportPath != "" {
		if err = writePerformanceReport(*reportPath, report); err != nil {
			_, _ = io.WriteString(stderr, "performance report unavailable\n")
			return 1
		}
	}
	if encodeOutput(stdout, report) != 0 {
		return 1
	}
	if runErr != nil {
		_, _ = io.WriteString(stderr, "performance correctness or reuse gate failed\n")
		return 1
	}
	return 0
}

func readBoundedJSON(path string) ([]byte, error) {
	// #nosec G304 -- these commands intentionally read an operator-selected manifest.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, ErrInvalid
	}
	return raw, nil
}

func writePerformanceReport(path string, report PerformanceReport) error {
	if path == "" || report.EvidenceHash == "" {
		return ErrInvalid
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".chartworks-performance-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(raw, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpPath, path)
	}
	if err != nil {
		return err
	}
	ok = true
	return nil
}

func encodeOutput(w io.Writer, v any) int {
	e := json.NewEncoder(w)
	e.SetEscapeHTML(true)
	if e.Encode(v) != nil {
		return 1
	}
	return 0
}

func rejectDuplicateJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return ErrInvalid
		}
		tok, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return ErrInvalid
				}
				seen[key] = true
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		case '[':
			for d.More() {
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		return ErrInvalid
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}
