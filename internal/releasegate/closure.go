package releasegate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type phaseRegistry struct {
	Phases map[string]struct {
		Status          string `json:"status"`
		AcceptanceCount int    `json:"acceptance_count"`
	} `json:"phases"`
}

type coverageMap struct {
	Features map[string]struct {
		Disposition string   `json:"disposition"`
		Acceptance  []string `json:"acceptance"`
	} `json:"features"`
	Gates map[string]struct {
		Acceptance []string `json:"acceptance"`
	} `json:"gates"`
}

var criterion = regexp.MustCompile(`^([0-9]{2})\.AC([0-9]{2})$`)

// VerifyClosure joins the actual strict runner's successful prior-phase events
// to every required feature/gate row. With requireShipped, absent receipts and
// any non-shipped phase fail; Q11 must remain discarded source debt.
func VerifyClosure(root string, receipts map[string][]string, requireShipped bool) error {
	var registry phaseRegistry
	var coverage coverageMap
	for _, item := range []struct {
		path string
		out  any
	}{{"docs/plans/phase-registry.json", &registry}, {"docs/plans/coverage.json", &coverage}} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil || json.Unmarshal(data, item.out) != nil {
			return fmt.Errorf("%w: phase or coverage registry unreadable", ErrEvidence)
		}
	}
	if len(registry.Phases) != 34 || len(coverage.Features) != 63 || len(coverage.Gates) != 41 {
		return fmt.Errorf("%w: expected 34 phases, 63 features and 41 gates", ErrEvidence)
	}
	for n := 1; n <= 34; n++ {
		id := fmt.Sprintf("%02d", n)
		p, ok := registry.Phases[id]
		if !ok || p.AcceptanceCount < 1 {
			return fmt.Errorf("%w: phase %s missing", ErrEvidence, id)
		}
		if !requireShipped {
			continue
		}
		if p.Status != "shipped" {
			return fmt.Errorf("%w: phase %s not shipped", ErrEvidence, id)
		}
		if id == "25" { // the strict runner verifies the parent after this call.
			continue
		}
		if len(receipts[id]) != p.AcceptanceCount {
			return fmt.Errorf("%w: phase %s receipts missing", ErrEvidence, id)
		}
		seen := map[string]bool{}
		for _, ac := range receipts[id] {
			if seen[ac] {
				return fmt.Errorf("%w: duplicate phase %s receipt", ErrEvidence, id)
			}
			seen[ac] = true
		}
		for c := 1; c <= p.AcceptanceCount; c++ {
			if !seen[fmt.Sprintf("TestPhase%s/AC%02d", id, c)] {
				return fmt.Errorf("%w: phase %s criterion missing", ErrEvidence, id)
			}
		}
	}
	for prefix, count := range map[string]int{"B": 20, "R": 16, "Q": 11, "N": 16} {
		for n := 1; n <= count; n++ {
			id := fmt.Sprintf("%s%02d", prefix, n)
			row, ok := coverage.Features[id]
			if !ok || len(row.Acceptance) == 0 {
				return fmt.Errorf("%w: feature %s missing", ErrEvidence, id)
			}
			if id == "Q11" {
				if row.Disposition != "discarded_stub" {
					return fmt.Errorf("%w: Q11 cannot become implemented functionality", ErrEvidence)
				}
			} else if row.Disposition != "required" {
				return fmt.Errorf("%w: required feature %s discarded", ErrEvidence, id)
			}
			for _, ref := range row.Acceptance {
				if err := verifyReference(ref, registry, receipts, requireShipped); err != nil {
					return fmt.Errorf("feature %s: %w", id, err)
				}
			}
		}
	}
	for n := 1; n <= 41; n++ {
		id := fmt.Sprintf("G%02d", n)
		row, ok := coverage.Gates[id]
		if !ok || len(row.Acceptance) == 0 {
			return fmt.Errorf("%w: gate %s missing", ErrEvidence, id)
		}
		for _, ref := range row.Acceptance {
			if err := verifyReference(ref, registry, receipts, requireShipped); err != nil {
				return fmt.Errorf("gate %s: %w", id, err)
			}
		}
	}
	return nil
}

func verifyReference(ref string, registry phaseRegistry, receipts map[string][]string, live bool) error {
	m := criterion.FindStringSubmatch(ref)
	if m == nil {
		return fmt.Errorf("%w: malformed criterion %s", ErrEvidence, ref)
	}
	p, ok := registry.Phases[m[1]]
	if !ok {
		return fmt.Errorf("%w: unknown criterion %s", ErrEvidence, ref)
	}
	var n int
	_, _ = fmt.Sscanf(m[2], "%d", &n)
	if n < 1 || n > p.AcceptanceCount {
		return fmt.Errorf("%w: out-of-range criterion %s", ErrEvidence, ref)
	}
	if live && m[1] != "25" {
		want := "TestPhase" + m[1] + "/AC" + m[2]
		found := false
		for _, actual := range receipts[m[1]] {
			found = found || actual == want
		}
		if !found {
			return fmt.Errorf("%w: criterion %s did not pass", ErrEvidence, ref)
		}
	}
	return nil
}

// SourceFileDigests is deterministic and includes each public contract schema,
// migration, reference config and active release document, not archive history.
func SourceFileDigests(root string) (map[string]string, error) {
	paths := []string{"AGENTS.md", "CLAUDE.md", "Dockerfile", "go.mod", "go.sum", "README.md", "RFC-001-Chartworks.md", "RFC-002-Governed-Reporting.md", "docs/configuration.md", "docs/plans/README.md", "docs/plans/COMMON.md", "docs/plans/phase-25-e2e-release.md", "docs/plans/phase-34-migration-parity-cutover.md", "docs/plans/phase-registry.json", "docs/plans/coverage.json", "docs/contracts/pengui-authority.md", "docs/contracts/warehouse-drivers.md", "docs/contracts/performance-evidence-v1.md", "docs/contracts/release-evidence-v1.md"}
	for _, pattern := range []string{"docs/contracts/chartworks-*-operations.json", "examples/chartworks.*.json", "internal/store/postgres/migrations/*.sql"} {
		found, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil || len(found) == 0 {
			return nil, fmt.Errorf("%w: source declaration %s missing", ErrEvidence, pattern)
		}
		for _, path := range found {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil, ErrEvidence
			}
			paths = append(paths, rel)
		}
	}
	sort.Strings(paths)
	out := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, fmt.Errorf("%w: declared source file missing", ErrEvidence)
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(path)] = hex.EncodeToString(sum[:])
	}
	return out, nil
}

func VerifySourceFiles(root string, declared []File) error {
	want, err := SourceFileDigests(root)
	if err != nil {
		return err
	}
	if len(declared) != len(want) {
		return fmt.Errorf("%w: document/schema/migration inventory incomplete", ErrEvidence)
	}
	seen := map[string]bool{}
	for _, f := range declared {
		if !filepath.IsLocal(f.Path) || seen[f.Path] || want[f.Path] != f.SHA256 {
			return fmt.Errorf("%w: document/schema/migration digest mismatch", ErrEvidence)
		}
		seen[f.Path] = true
	}
	return nil
}

func VerifyAPISchema(dir string, file File) error {
	data, err := ReadFile(dir, file, 16<<20)
	if err != nil {
		return err
	}
	var schema struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if json.Unmarshal(data, &schema) != nil || !strings.HasPrefix(schema.OpenAPI, "3.") || len(schema.Paths) == 0 {
		return fmt.Errorf("%w: binary API schema missing", ErrEvidence)
	}
	return nil
}
