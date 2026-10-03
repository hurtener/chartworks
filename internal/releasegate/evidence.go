// Package releasegate verifies content-free, exact-head release evidence. It
// never promotes fixture or unknown observations to live qualification.
package releasegate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	ErrEvidence = errors.New("release: evidence incomplete or invalid")
	hex64       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40       = regexp.MustCompile(`^[0-9a-f]{40}$`)
	identifier  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

// Bundle is an owner-supplied manifest. Logs and inventory are separate hashed
// files so an assertion in this JSON alone cannot close a gate.
type Bundle struct {
	SchemaVersion int       `json:"schema_version"`
	Head          string    `json:"head"`
	Inventory     File      `json:"cohort_inventory"`
	Records       []Record  `json:"records"`
	Artifacts     Artifacts `json:"artifacts"`
	Review        File      `json:"cumulative_review"`
}

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// A record points to uncached Go JSON test events from a named real boundary.
// Mode distinguishes native/live execution from recorded/fixture evidence.
type Record struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Mode     string `json:"mode"`
	Head     string `json:"head"`
	RunRef   string `json:"run_ref"`
	ExitCode *int   `json:"exit_code"`
	Events   File   `json:"events"`
}

type Artifacts struct {
	Binary        File   `json:"binary"`
	ImageRef      string `json:"image_ref"`
	ImageDigest   string `json:"image_digest"`
	SourceTree    string `json:"source_tree"`
	MigrationsSHA string `json:"migrations_sha256"`
	APISchema     File   `json:"api_schema"`
	Files         []File `json:"files"`
}

type Inventory struct {
	Head           string   `json:"head"`
	Phase34RunRef  string   `json:"phase34_run_ref"`
	SourceManifest File     `json:"source_manifest"`
	Cohorts        []Cohort `json:"cohorts"`
}

type Cohort struct {
	ID      string   `json:"id"`
	Engines []string `json:"engines"`
}

type Review struct {
	Head     string    `json:"head"`
	Reviewer string    `json:"reviewer"`
	RunRef   string    `json:"run_ref"`
	Findings []Finding `json:"findings"`
}

type Finding struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
	State    string `json:"state"`
}

func decodeExact[T any](data []byte, out *T) error {
	if err := uniqueJSON(data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return fmt.Errorf("%w: invalid JSON: %v", ErrEvidence, err)
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("%w: trailing JSON", ErrEvidence)
	}
	return nil
}

// uniqueJSON rejects ambiguous object keys before encoding/json projects a
// document into a struct. Evidence metadata must have one interpretation.
func uniqueJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 32 {
			return fmt.Errorf("%w: JSON depth exceeded", ErrEvidence)
		}
		tok, err := d.Token()
		if err != nil {
			return fmt.Errorf("%w: malformed JSON", ErrEvidence)
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, valid := key.(string)
				if err != nil || !valid || seen[name] {
					return fmt.Errorf("%w: duplicate or malformed JSON field", ErrEvidence)
				}
				seen[name] = true
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("%w: malformed JSON delimiter", ErrEvidence)
		}
		if _, err := d.Token(); err != nil {
			return fmt.Errorf("%w: malformed JSON closing delimiter", ErrEvidence)
		}
		return nil
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", ErrEvidence)
	}
	return nil
}

func Load(dir string) (Bundle, error) {
	var b Bundle
	file, err := os.Open(filepath.Join(dir, "release.json"))
	if err != nil {
		return b, fmt.Errorf("%w: release manifest missing", ErrEvidence)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return b, fmt.Errorf("%w: release manifest missing or oversized", ErrEvidence)
	}
	if err := decodeExact(data, &b); err != nil {
		return b, err
	}
	if b.SchemaVersion != 1 || !hex40.MatchString(b.Head) {
		return b, fmt.Errorf("%w: schema version or head", ErrEvidence)
	}
	return b, nil
}

func artifactPath(dir string, f File) (string, error) {
	if !filepath.IsLocal(f.Path) || !hex64.MatchString(f.SHA256) {
		return "", fmt.Errorf("%w: invalid artifact path or digest", ErrEvidence)
	}
	base, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("%w: artifact directory unavailable", ErrEvidence)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(base, f.Path))
	if err != nil {
		return "", fmt.Errorf("%w: artifact unavailable", ErrEvidence)
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: artifact escaped bundle", ErrEvidence)
	}
	return path, nil
}

// ReadFile rejects absolute paths, traversal, symlink escapes and a mismatched
// content hash. Evidence files may live outside the source checkout.
func ReadFile(dir string, f File, limit int64) ([]byte, error) {
	path, err := artifactPath(dir, f)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: artifact unreadable", ErrEvidence)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: artifact oversized", ErrEvidence)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != f.SHA256 {
		return nil, fmt.Errorf("%w: artifact digest mismatch", ErrEvidence)
	}
	return data, nil
}

// VerifyFile streams a large binary without holding it all in memory.
func VerifyFile(dir string, f File, limit int64) (string, error) {
	path, err := artifactPath(dir, f)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: artifact unreadable", ErrEvidence)
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(file, limit+1))
	if err != nil || n > limit || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return "", fmt.Errorf("%w: artifact digest mismatch or oversized", ErrEvidence)
	}
	return path, nil
}

func VerifyInventory(dir, head string, f File) (Inventory, error) {
	var inv Inventory
	data, err := ReadFile(dir, f, 1<<20)
	if err != nil {
		return inv, err
	}
	if err = decodeExact(data, &inv); err != nil {
		return inv, err
	}
	if inv.Head != head || !reviewableRef(inv.Phase34RunRef) || len(inv.Cohorts) == 0 {
		return inv, fmt.Errorf("%w: Phase 34 cohort inventory absent or stale", ErrEvidence)
	}
	if _, err := ReadFile(dir, inv.SourceManifest, 16<<20); err != nil {
		return inv, fmt.Errorf("%w: Phase 34 source manifest missing: %v", ErrEvidence, err)
	}
	seen := map[string]bool{}
	for _, c := range inv.Cohorts {
		if !identifier.MatchString(c.ID) || seen[c.ID] || len(c.Engines) == 0 {
			return inv, fmt.Errorf("%w: invalid cohort inventory", ErrEvidence)
		}
		seen[c.ID] = true
		engines := map[string]bool{}
		for _, engine := range c.Engines {
			if !requiredEngine(engine) || engines[engine] {
				return inv, fmt.Errorf("%w: invalid cohort engine", ErrEvidence)
			}
			engines[engine] = true
		}
	}
	return inv, nil
}

var Engines = []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"}
var Operations = []string{"container_boot", "dependency_readiness", "shutdown_recovery", "backup_restore", "jwks_rotation", "retention_erasure"}

func requiredEngine(s string) bool {
	for _, name := range Engines {
		if name == s {
			return true
		}
	}
	return false
}

// VerifyRecords requires one exact-head, successful real test event per required
// boundary. A skipped nested case or package failure invalidates the whole log.
func VerifyRecords(dir string, b Bundle, kind string, ids []string) error {
	required := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !identifier.MatchString(id) || required[id] {
			return fmt.Errorf("%w: invalid required evidence set", ErrEvidence)
		}
		required[id] = true
	}
	seen := map[string]bool{}
	for _, r := range b.Records {
		if r.Kind != kind {
			continue
		}
		if !required[r.ID] || seen[r.ID] || r.Head != b.Head || r.ExitCode == nil || *r.ExitCode != 0 || !reviewableRef(r.RunRef) {
			return fmt.Errorf("%w: duplicate, stale or unreviewable %s evidence", ErrEvidence, kind)
		}
		if (kind == "engine" && (r.ID == "postgres" || r.ID == "mysql" || r.ID == "sqlserver") && r.Mode != "native" && r.Mode != "live") || ((kind != "engine" || r.ID == "bigquery" || r.ID == "snowflake" || r.ID == "databricks") && r.Mode != "live") {
			return fmt.Errorf("%w: %s %s is not live evidence", ErrEvidence, kind, r.ID)
		}
		data, err := ReadFile(dir, r.Events, 16<<20)
		if err != nil {
			return err
		}
		name := map[string]string{"authority": "TestReleaseAuthority", "operation": "TestReleaseOperation/", "engine": "TestReleaseEngine/", "cohort": "TestReleaseCohort/"}[kind]
		if name == "" {
			return fmt.Errorf("%w: unknown record kind", ErrEvidence)
		}
		if kind != "authority" {
			name += r.ID
		}
		if err := VerifyGoEvents(data, name); err != nil {
			return fmt.Errorf("%w: %s %s: %v", ErrEvidence, kind, r.ID, err)
		}
		seen[r.ID] = true
	}
	if len(seen) != len(required) {
		missing := []string{}
		for id := range required {
			if !seen[id] {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		return fmt.Errorf("%w: missing %s evidence: %s", ErrEvidence, kind, strings.Join(missing, ", "))
	}
	return nil
}

// VerifyGoEvents accepts the Go test JSON stream only when the named test and
// package passed and no child or package failed or skipped.
func VerifyGoEvents(data []byte, name string) error {
	if name == "" {
		return ErrEvidence
	}
	d := json.NewDecoder(bytes.NewReader(data))
	passed, packagePassed := 0, false
	var pkg string
	for {
		var raw json.RawMessage
		err := d.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil || uniqueJSON(raw) != nil {
			return fmt.Errorf("%w: malformed Go event stream", ErrEvidence)
		}
		var event struct {
			Action  string `json:"Action"`
			Package string `json:"Package"`
			Test    string `json:"Test"`
		}
		if json.Unmarshal(raw, &event) != nil || event.Package == "" {
			return fmt.Errorf("%w: malformed Go event stream", ErrEvidence)
		}
		if pkg == "" {
			pkg = event.Package
		} else if pkg != event.Package {
			return fmt.Errorf("%w: multiple test packages", ErrEvidence)
		}
		if event.Action == "fail" || event.Action == "skip" {
			return fmt.Errorf("%w: failed or skipped test", ErrEvidence)
		}
		if event.Test == "" && event.Action == "pass" {
			packagePassed = true
		}
		if event.Test == name && event.Action == "pass" {
			passed++
		}
	}
	if passed != 1 || !packagePassed {
		return fmt.Errorf("%w: named test/package did not pass exactly once", ErrEvidence)
	}
	return nil
}

func VerifyReview(dir string, b Bundle) error {
	data, err := ReadFile(dir, b.Review, 1<<20)
	if err != nil {
		return err
	}
	var review Review
	if err = decodeExact(data, &review); err != nil {
		return err
	}
	if review.Head != b.Head || strings.TrimSpace(review.Reviewer) == "" || !reviewableRef(review.RunRef) {
		return fmt.Errorf("%w: cumulative review not pinned", ErrEvidence)
	}
	seen := map[string]bool{}
	for _, f := range review.Findings {
		if f.ID == "" || seen[f.ID] || f.State != "resolved" || (f.Priority != "P0" && f.Priority != "P1" && f.Priority != "P2" && f.Priority != "P3") {
			return fmt.Errorf("%w: unresolved or malformed review finding", ErrEvidence)
		}
		seen[f.ID] = true
	}
	return nil
}

func reviewableRef(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.TrimSpace(u.Path) != ""
}
