package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/jackc/pgx/v5"
)

type pipelineFixture struct {
	*engineeringFixture
	pipelines *engineering.PipelineService
}

func newPipelineFixture(t *testing.T, model gateway.Engine, change func(*config.Values)) *pipelineFixture {
	t.Helper()
	runner := os.Getenv("CHARTWORKS_TEST_BRUIN_PATH")
	if !filepath.IsAbs(runner) {
		t.Fatal("CHARTWORKS_TEST_BRUIN_PATH must name the absolute pinned Bruin executable")
	}
	artifact, err := os.ReadFile(runner)
	if err != nil {
		t.Fatal("read pinned Bruin executable", err)
	}
	digest := sha256.Sum256(artifact)
	tempDir := os.Getenv("CHARTWORKS_TEST_PIPELINE_TMPDIR")
	if !filepath.IsAbs(tempDir) {
		t.Fatal("CHARTWORKS_TEST_PIPELINE_TMPDIR must name the absolute executable private tmpfs")
	}
	base := newEngineeringFixture(t, func(v *config.Values) {
		v.Pipelines.Enabled = true
		v.Pipelines.RunnerPath = runner
		v.Pipelines.RunnerSHA256 = hex.EncodeToString(digest[:])
		v.Pipelines.TempDir = tempDir
		if change != nil {
			change(v)
		}
	}, model)
	if _, err := base.admin.Exec(context.Background(), "GRANT USAGE ON SCHEMA analytics TO "+pgx.Identifier{base.writer}.Sanitize()+"; GRANT SELECT ON ALL TABLES IN SCHEMA analytics TO "+pgx.Identifier{base.writer}.Sanitize()); err != nil {
		t.Fatal("grant pipeline fixture input reads", err)
	}
	tlsRoot := os.Getenv("CHARTWORKS_TEST_PIPELINE_TLS_ROOT")
	if !filepath.IsAbs(tlsRoot) {
		t.Fatal("CHARTWORKS_TEST_PIPELINE_TLS_ROOT required; managed runner acceptance uses verify-full TLS")
	}
	base.mu.Lock()
	writerURL, parseErr := url.Parse(base.sourceFixture.values["CHARTWORKS_SOURCE_WRITE"])
	if parseErr != nil {
		base.mu.Unlock()
		t.Fatal("managed writer URL", parseErr)
	}
	writerURL.Host = "localhost:" + writerURL.Port()
	query := writerURL.Query()
	query.Set("sslmode", "verify-full")
	query.Set("sslrootcert", tlsRoot)
	writerURL.RawQuery = query.Encode()
	base.sourceFixture.values["CHARTWORKS_SOURCE_WRITE"] = writerURL.String()
	base.mu.Unlock()
	service, err := engineering.NewPipelineService(base.db, base.s, base.validator, model, base.values, base.lookup)
	if err != nil {
		t.Fatal("pipeline service", err)
	}
	t.Cleanup(service.Close)
	return &pipelineFixture{engineeringFixture: base, pipelines: service}
}

func (f *pipelineFixture) definition(t *testing.T, source sources.Source, id string) engineering.PipelineDefinition {
	t.Helper()
	binding, err := f.s.Binding(context.Background(), f.e, source.ID, source.ContextID)
	if err != nil || len(binding.Relations) == 0 {
		t.Fatal("pipeline source binding", err, binding)
	}
	relation := binding.Relations[0]
	for _, candidate := range binding.Relations {
		if candidate.Schema == "analytics" && candidate.Name == "sales" {
			relation = candidate
			break
		}
	}
	if relation.Schema != "analytics" || relation.Name != "sales" {
		t.Fatal("synthetic sales relation absent", binding)
	}
	return engineering.PipelineDefinition{
		ID: id, Name: "Synthetic managed pipeline", Connection: "workspace",
		Steps: []engineering.PipelineStep{{
			ID: "output", Source: source.ID, Context: source.ContextID,
			SQL: "SELECT id::bigint AS id FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize(), Inputs: []string{relation.ID},
			DependsOn: []string{}, FromSteps: []string{}, Strategy: "replace",
			Columns: []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}}, Checks: []engineering.PipelineCheck{},
		}},
	}
}
