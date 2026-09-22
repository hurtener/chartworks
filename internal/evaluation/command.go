package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

const commandUsage = "usage: chartworks eval gate|inspect --suite PATH [--run-id ID]\n"

// Command runs a reviewed fixture manifest. Live execution is available only
// through Service with injected production dependencies and current authority.
func Command(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil || stdout == nil || stderr == nil || len(args) == 0 {
		_, _ = io.WriteString(stderr, commandUsage)
		return 2
	}
	verb := args[0]
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
