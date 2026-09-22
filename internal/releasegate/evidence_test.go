package releasegate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func intPtr(v int) *int { return &v }

func writeEvidence(t *testing.T, dir, name string, data []byte) File {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return File{Path: name, SHA256: hex.EncodeToString(sum[:])}
}

func testEvents(name string) []byte {
	return []byte(`{"Action":"run","Package":"example.test/release","Test":"` + name + `"}` + "\n" +
		`{"Action":"pass","Package":"example.test/release","Test":"` + name + `"}` + "\n" +
		`{"Action":"pass","Package":"example.test/release"}` + "\n")
}

func TestEvidenceRejectsFixtureSkipTamperAndMissing(t *testing.T) {
	dir := t.TempDir()
	good := writeEvidence(t, dir, "postgres.jsonl", testEvents("TestReleaseEngine/postgres"))
	bundle := Bundle{SchemaVersion: 1, Head: testHead, Records: []Record{{Kind: "engine", ID: "postgres", Mode: "native", Head: testHead, RunRef: "synthetic-run", ExitCode: intPtr(0), Events: good}}}
	if err := VerifyRecords(dir, bundle, "engine", []string{"postgres"}); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Bundle){
		"fixture": func(b *Bundle) { b.Records[0].Mode = "fixture" },
		"unknown": func(b *Bundle) { b.Records[0].Mode = "unknown" },
		"stale":   func(b *Bundle) { b.Records[0].Head = strings.Repeat("b", 40) },
		"missing": func(b *Bundle) { b.Records = nil },
		"hash":    func(b *Bundle) { b.Records[0].Events.SHA256 = strings.Repeat("0", 64) },
		"no-exit": func(b *Bundle) { b.Records[0].ExitCode = nil },
	} {
		t.Run(name, func(t *testing.T) {
			copy := bundle
			copy.Records = append([]Record(nil), bundle.Records...)
			mutate(&copy)
			if err := VerifyRecords(dir, copy, "engine", []string{"postgres"}); !errors.Is(err, ErrEvidence) {
				t.Fatal("untrusted evidence passed", err)
			}
		})
	}
	for name, events := range map[string][]byte{
		"skip":       append(testEvents("TestReleaseEngine/postgres"), []byte(`{"Action":"skip","Package":"example.test/release","Test":"TestReleaseEngine/postgres/child"}`+"\n")...),
		"fail":       []byte(`{"Action":"fail","Package":"example.test/release","Test":"TestReleaseEngine/postgres"}`),
		"duplicate":  []byte(`{"Action":"fail","Action":"pass","Package":"example.test/release","Test":"TestReleaseEngine/postgres"}`),
		"no-parent":  []byte(`{"Action":"pass","Package":"example.test/release","Test":"TestReleaseEngine/postgres"}`),
		"wrong-test": testEvents("TestReleaseEngine/mysql"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := VerifyGoEvents(events, "TestReleaseEngine/postgres"); !errors.Is(err, ErrEvidence) {
				t.Fatal("bad test stream passed", err)
			}
		})
	}
	var decoded Bundle
	if err := decodeExact([]byte(`{"head":"`+testHead+`","head":"`+testHead+`"}`), &decoded); !errors.Is(err, ErrEvidence) {
		t.Fatal("duplicate release field accepted", err)
	}
}

func TestCohortInventoryAndReviewFailClosed(t *testing.T) {
	dir := t.TempDir()
	manifest := writeEvidence(t, dir, "phase34-manifest.json", []byte(`{"synthetic":true}`))
	inv := Inventory{Head: testHead, Phase34RunRef: "synthetic-phase34-run", SourceManifest: manifest, Cohorts: []Cohort{{ID: "alpha", Engines: []string{"postgres", "bigquery"}}}}
	data, _ := json.Marshal(inv)
	file := writeEvidence(t, dir, "inventory.json", data)
	if _, err := VerifyInventory(dir, testHead, file); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Inventory{{Head: testHead}, {Head: strings.Repeat("b", 40), Phase34RunRef: inv.Phase34RunRef, SourceManifest: manifest, Cohorts: inv.Cohorts}, {Head: testHead, Phase34RunRef: inv.Phase34RunRef, SourceManifest: manifest, Cohorts: []Cohort{{ID: "alpha", Engines: []string{"postgres", "postgres"}}}}} {
		data, _ := json.Marshal(bad)
		badFile := writeEvidence(t, dir, "bad-inventory.json", data)
		if _, err := VerifyInventory(dir, testHead, badFile); !errors.Is(err, ErrEvidence) {
			t.Fatal("bad inventory passed", err)
		}
	}
	review := Review{Head: testHead, Reviewer: "reviewer-1", RunRef: "synthetic-run", Findings: []Finding{{ID: "finding-1", Priority: "P1", State: "resolved"}}}
	data, _ = json.Marshal(review)
	b := Bundle{Head: testHead, Review: writeEvidence(t, dir, "review.json", data)}
	if err := VerifyReview(dir, b); err != nil {
		t.Fatal(err)
	}
	review.Findings[0].State = "open"
	data, _ = json.Marshal(review)
	b.Review = writeEvidence(t, dir, "review-open.json", data)
	if err := VerifyReview(dir, b); !errors.Is(err, ErrEvidence) {
		t.Fatal("open P1 passed", err)
	}
	outsideDir := t.TempDir()
	outside := writeEvidence(t, outsideDir, "outside", []byte("secret"))
	if err := os.Symlink(filepath.Join(outsideDir, "outside"), filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(dir, File{Path: "escape", SHA256: outside.SHA256}, 1024); !errors.Is(err, ErrEvidence) {
		t.Fatal("escaped evidence directory", err)
	}
}

func TestSourceAndCoverageInventory(t *testing.T) {
	root := filepath.Join("..", "..")
	if err := VerifyClosure(root, nil, false); err != nil {
		t.Fatal(err)
	}
	digests, err := SourceFileDigests(root)
	if err != nil || len(digests) < 40 {
		t.Fatal("source file inventory incomplete", err, len(digests))
	}
	files := make([]File, 0, len(digests))
	for path, hash := range digests {
		files = append(files, File{Path: path, SHA256: hash})
	}
	if err := VerifySourceFiles(root, files); err != nil {
		t.Fatal(err)
	}
	files[0].SHA256 = strings.Repeat("0", 64)
	if err := VerifySourceFiles(root, files); !errors.Is(err, ErrEvidence) {
		t.Fatal("changed document accepted", err)
	}
	if err := VerifyClosure(root, nil, true); !errors.Is(err, ErrEvidence) {
		t.Fatal("missing actual phase results accepted", err)
	}
}
