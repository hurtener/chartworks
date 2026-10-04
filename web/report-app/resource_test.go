package reportapp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	reportviewer "github.com/hurtener/chartworks/web/report-viewer"
)

func TestCompiledReportApp(t *testing.T) {
	html := HTML()
	t.Logf("MCP resource: %d bytes; remaining: %d", len(html), maxResourceBytes-len(html))
	if URI != "ui://chartworks/report-app/v1" || html != HTML() || !strings.HasPrefix(html, "<!doctype html>") || len(html) > maxResourceBytes {
		t.Fatal("unstable or unbounded report app")
	}
	script, css := compiledAssets()
	assertResourceCSP(t, html, script, css)
	for _, directive := range []string{"default-src 'none'", "connect-src 'none'", "frame-src 'none'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(html, directive) {
			t.Fatal("missing CSP", directive)
		}
	}
	for _, forbidden := range []string{"localStorage", "sessionStorage", "indexedDB", "document.cookie", "fetch(", "XMLHttpRequest", ".innerHTML", "<script src=", "import {"} {
		if strings.Contains(html, forbidden) {
			t.Fatal("unsafe resource primitive", forbidden)
		}
	}
	// Minified identifiers are not stable evidence. The source/graph test below
	// binds this bundle to one canonical presenter and excludes the viewer host.
	for _, forbidden := range []string{"class Viewer", "class Bridge", "report-viewer\"", "report-viewer'", "sourceMappingURL", "</script"} {
		if strings.Contains(script, forbidden) {
			t.Fatal("unused viewer host or unsafe inline source in compiled app", forbidden)
		}
	}
	for _, pattern := range []string{`\beval\s*\(`, `\bnew\s+Function\b`, `\bimport\s*\(`, `(?m)^\s*(?:import|export)\s`} {
		if regexp.MustCompile(pattern).MatchString(script) {
			t.Fatal("compiled app is not a self-contained CSP-safe IIFE", pattern)
		}
	}
	if path := os.Getenv("CHARTWORKS_REPORT_APP_HTML_OUT"); path != "" {
		if err := os.WriteFile(path, []byte(html), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReportAppJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for app source verification:", err)
	}
	for _, file := range []string{"model.test.mjs", "dom.test.mjs", "allocation.test.mjs", "pages.test.mjs", "mapping.test.mjs", "formatting.test.mjs", "formatting-bundle.test.mjs", "dataset.test.mjs", "dataset-filter.test.mjs", "preparation.test.mjs", "preparation-bundle.test.mjs", "grid.test.mjs", "filters.test.mjs", "publication.test.mjs", "publication-controls.test.mjs", "publication-browser-fixture.test.mjs", "browser-geometry.test.mjs", "browser-filter-inspector.test.mjs", "catalog.test.mjs", "catalog-browser-fixture.test.mjs", "filters-browser-fixture.test.mjs", "filters-browser-pointer.test.mjs", "filters-bundle.test.mjs", "bridge.test.mjs", "retained.test.mjs", "app.test.mjs", "browser_cleanup.test.mjs", "bundle.test.mjs"} {
		command := exec.Command(node, "--test", file)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", file, err, out)
		}
	}
	script, _ := compiledAssets()
	path := filepath.Join(t.TempDir(), "compiled-app.mjs")
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", path).CombinedOutput(); err != nil {
		t.Fatalf("compiled resource JS: %v\n%s", err, out)
	}
}

func TestEmbeddedResourceRegistration(t *testing.T) {
	for _, parents := range [][]string{nil, {}, {"*"}, {"null"}, {"http://host.example"}, {"https://host.example/"}, {"https://user:password@host.example"}, {"https://host.example/path"}, {"https://host.example?token=x"}, {"https://*.example"}, {"https://host.example#fragment"}, {"https://host.example:443"}, {"https://HOST.example"}, {"https://host.example:"}, {"https://host.example:06543"}, {"https://host.example:999999"}, {"https://host.example", "https://host.example"}} {
		if _, err := EmbeddedHTML(parents); err == nil {
			t.Fatalf("accepted unregistered/non-origin parents %v", parents)
		}
	}
	html, err := EmbeddedHTML([]string{"https://report-host.example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(html) > maxResourceBytes || html == HTML() || !strings.Contains(html, `const REPORT_APP_EMBEDDED_PARENTS=["https://report-host.example"];`) {
		t.Fatal("embedded resource did not select explicit registered entrypoint")
	}
	script, css := compiledAssets()
	assertResourceCSP(t, html, "const REPORT_APP_EMBEDDED_PARENTS=[\"https://report-host.example\"];\n"+script, css)
	parents := make([]string, 16)
	for i := range parents {
		parents[i] = fmt.Sprintf("https://host-%d.example", i)
	}
	if full, err := EmbeddedHTML(parents); err != nil || len(full) > maxResourceBytes {
		t.Fatalf("maximum registered parent count: %v", err)
	} else {
		t.Logf("Embedded resource with 16 registered parents: %d bytes; remaining: %d", len(full), maxResourceBytes-len(full))
	}
	if _, err := EmbeddedHTML([]string{"https://" + strings.Repeat("x", maxResourceBytes) + ".example"}); err == nil {
		t.Fatal("accepted an oversized embedded resource")
	}
	if path := os.Getenv("CHARTWORKS_REPORT_APP_EMBEDDED_HTML_OUT"); path != "" {
		if err := os.WriteFile(path, []byte(html), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertResourceCSP(t *testing.T, html, script, css string) {
	t.Helper()
	for _, asset := range []struct{ value, tag string }{{script, "<script type=\"module\">"}, {css, "<style>"}} {
		if !strings.Contains(html, asset.tag+asset.value+"</") {
			t.Fatal("HTML changed exact compiled asset bytes")
		}
		sum := sha256.Sum256([]byte(asset.value))
		if !strings.Contains(html, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
			t.Fatal("missing exact asset CSP hash")
		}
	}
}

// The manifest is build evidence, never runtime configuration. Go verifies all
// source/configuration digests and independently follows authored static imports;
// CI additionally regenerates with the pinned compiler and compares exact bytes.
type assetDigest struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type bundleInput struct {
	assetDigest
	Imports       []string `json:"imports"`
	BytesInOutput int      `json:"bytesInOutput"`
}

type assetManifest struct {
	Version int `json:"version"`
	Bundler struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"bundler"`
	EntryPoint    string        `json:"entryPoint"`
	Inputs        []bundleInput `json:"inputs"`
	Configuration []assetDigest `json:"configuration"`
	Styles        []assetDigest `json:"styles"`
	Output        assetDigest   `json:"output"`
}

func checksum(path string, data []byte) assetDigest {
	sum := sha256.Sum256(data)
	return assetDigest{Path: path, Bytes: len(data), SHA256: hex.EncodeToString(sum[:])}
}

var staticModuleImport = regexp.MustCompile(`(?m)^[\t ]*import\s+(?:[a-zA-Z0-9_$\s{},*]+\s+from\s+)?['"]([^'"]+)['"]\s*;`)
var moduleImportStart = regexp.MustCompile(`(?m)^[\t ]*import\b`)
var unsupportedModuleImport = regexp.MustCompile(`\b(?:import|require)\s*\(|(?m)^[\t ]*export[^;]*\bfrom\s*['"]`)

func verifyGeneratedAssets(source fs.FS, manifest assetManifest, script []byte) error {
	if manifest.Version != 1 || manifest.Bundler.Name != "esbuild" || manifest.EntryPoint != "web/report-app/app.js" || manifest.Output.Path != "web/report-app/generated/report-app.js" {
		return fmt.Errorf("unknown report app build manifest")
	}
	if manifest.Output != checksum(manifest.Output.Path, script) {
		return fmt.Errorf("generated script digest mismatch")
	}
	seen := map[string]bool{}
	check := func(asset assetDigest) error {
		if !fs.ValidPath(asset.Path) || !strings.HasPrefix(asset.Path, "web/") || seen[asset.Path] {
			return fmt.Errorf("invalid or duplicate input %q", asset.Path)
		}
		seen[asset.Path] = true
		data, err := fs.ReadFile(source, asset.Path)
		if err != nil {
			return err
		}
		if asset != checksum(asset.Path, data) {
			return fmt.Errorf("stale generated asset: %s changed; run npm run build in web/report-app", asset.Path)
		}
		return nil
	}
	configuration := []string{"web/report-app/build.mjs", "web/report-app/package.json", "web/report-app/package-lock.json"}
	styles := []string{"web/report-viewer/styles.css", "web/report-app/styles.css"}
	for _, group := range []struct {
		assets   []assetDigest
		required []string
	}{{manifest.Configuration, configuration}, {manifest.Styles, styles}} {
		if len(group.assets) != len(group.required) {
			return fmt.Errorf("missing build configuration or style input")
		}
		for i, asset := range group.assets {
			if asset.Path != group.required[i] {
				return fmt.Errorf("unexpected configuration/style input %q", asset.Path)
			}
			if err := check(asset); err != nil {
				return err
			}
		}
	}
	packageData, err := fs.ReadFile(source, configuration[1])
	if err != nil {
		return err
	}
	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(packageData, &pkg); err != nil {
		return err
	}
	lockData, err := fs.ReadFile(source, configuration[2])
	if err != nil {
		return err
	}
	var lock struct {
		LockfileVersion int `json:"lockfileVersion"`
		Packages        map[string]struct {
			Version         string            `json:"version"`
			Resolved        string            `json:"resolved"`
			Integrity       string            `json:"integrity"`
			DevDependencies map[string]string `json:"devDependencies"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		return err
	}
	version := pkg.DevDependencies["esbuild"]
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) || manifest.Bundler.Version != version || lock.LockfileVersion != 3 || lock.Packages[""].DevDependencies["esbuild"] != version || lock.Packages["node_modules/esbuild"].Version != version {
		return fmt.Errorf("build tool is not pinned consistently")
	}
	for name, dependency := range lock.Packages {
		if name != "" && (!strings.HasPrefix(dependency.Resolved, "https://registry.npmjs.org/") || !strings.HasPrefix(dependency.Integrity, "sha512-")) {
			return fmt.Errorf("unverified build dependency %q", name)
		}
	}
	inputs := map[string]bundleInput{}
	previous := ""
	for _, input := range manifest.Inputs {
		if input.Path <= previous || !strings.HasSuffix(input.Path, ".js") || input.BytesInOutput < 0 {
			return fmt.Errorf("invalid bundle graph input %q", input.Path)
		}
		previous = input.Path
		if err := check(input.assetDigest); err != nil {
			return err
		}
		inputs[input.Path] = input
	}
	if input, ok := inputs["web/report-viewer/presentation.js"]; !ok || input.BytesInOutput < 1 {
		return fmt.Errorf("canonical retained presentation is missing")
	}
	if _, ok := inputs["web/report-viewer/app.js"]; ok {
		return fmt.Errorf("unused viewer host was bundled")
	}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true
		input, ok := inputs[name]
		if !ok {
			return fmt.Errorf("reachable authored input missing from manifest: %s", name)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		matches := staticModuleImport.FindAllSubmatch(data, -1)
		if len(matches) != len(moduleImportStart.FindAll(data, -1)) || unsupportedModuleImport.Match(data) {
			return fmt.Errorf("unsupported module import syntax in %s", name)
		}
		imports := []string{}
		for _, match := range matches {
			specifier := string(match[1])
			if !strings.HasPrefix(specifier, ".") || !strings.HasSuffix(specifier, ".js") {
				return fmt.Errorf("non-local authored import in %s", name)
			}
			dependency := path.Clean(path.Join(path.Dir(name), specifier))
			if !fs.ValidPath(dependency) {
				return fmt.Errorf("invalid dependency path %s", dependency)
			}
			imports = append(imports, dependency)
			if err := visit(dependency); err != nil {
				return err
			}
		}
		slices.Sort(imports)
		imports = slices.Compact(imports)
		if !slices.Equal(imports, input.Imports) {
			return fmt.Errorf("source import graph mismatch: %s", name)
		}
		return nil
	}
	if err := visit(manifest.EntryPoint); err != nil {
		return err
	}
	if len(visited) != len(inputs) {
		return fmt.Errorf("unreachable bundle input")
	}
	return nil
}

func readAssetManifest(t *testing.T) assetManifest {
	t.Helper()
	data, err := os.ReadFile("generated/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest assetManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestGeneratedAssetSourceIdentity(t *testing.T) {
	manifest := readAssetManifest(t)
	if err := verifyGeneratedAssets(os.DirFS("../.."), manifest, []byte(appScript)); err != nil {
		t.Fatal(err)
	}
	canonical, css := reportviewer.Assets()
	for name, expected := range map[string]string{"../report-viewer/presentation.js": canonical, "../report-viewer/styles.css": css} {
		actual, err := os.ReadFile(name)
		if err != nil || string(actual) != expected {
			t.Fatalf("shared renderer asset is not canonical: %s: %v", name, err)
		}
	}
	for _, name := range []string{"renderRetainedOutput", "validateRetainedView", "renderChart"} {
		if strings.Count(canonical, "function "+name+"(") != 1 {
			t.Fatalf("shared renderer duplicates or omits %s", name)
		}
	}
}

func TestGeneratedAssetsRejectDrift(t *testing.T) {
	manifest := readAssetManifest(t)
	files := fstest.MapFS{}
	assets := append(append([]assetDigest{}, manifest.Configuration...), manifest.Styles...)
	for _, input := range manifest.Inputs {
		assets = append(assets, input.assetDigest)
	}
	for _, asset := range assets {
		data, err := os.ReadFile(filepath.Join("../..", filepath.FromSlash(asset.Path)))
		if err != nil {
			t.Fatal(err)
		}
		files[asset.Path] = &fstest.MapFile{Data: data}
	}
	if err := verifyGeneratedAssets(files, manifest, []byte(appScript)); err != nil {
		t.Fatal(err)
	}
	for _, asset := range assets {
		t.Run(asset.Path, func(t *testing.T) {
			original := files[asset.Path]
			files[asset.Path] = &fstest.MapFile{Data: append(append([]byte{}, original.Data...), '\n')}
			defer func() { files[asset.Path] = original }()
			if err := verifyGeneratedAssets(files, manifest, []byte(appScript)); err == nil {
				t.Fatal("accepted stale source/configuration")
			}
		})
	}
	if err := verifyGeneratedAssets(files, manifest, []byte(appScript+"\n")); err == nil {
		t.Fatal("accepted modified compiled artifact")
	}
	missing := manifest
	missing.Inputs = append([]bundleInput{}, manifest.Inputs...)
	for i, input := range missing.Inputs {
		if input.Path == "web/report-app/filters.js" {
			missing.Inputs = append(missing.Inputs[:i], missing.Inputs[i+1:]...)
			break
		}
	}
	if err := verifyGeneratedAssets(files, missing, []byte(appScript)); err == nil {
		t.Fatal("accepted incomplete transitive source manifest")
	}
}
