package engineering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestPipelineValidationRejectsSuccessfulErrorReports(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{}`, `[{"pipeline":"other","issues":{}}]`, `[{"pipeline":"chartworks-managed"}]`, `[{"pipeline":"chartworks-managed","issues":{"step":[{"severity":"critical"}]}}]`, `[{"pipeline":"chartworks-managed","issues":{},"error":"oops"}]`, `{"error":"scratch full"}[{"pipeline":"chartworks-managed","issues":{}}]`, `[{"pipeline":"chartworks-managed","issues":{}}] {}`} {
		if validPipelineValidation([]byte(raw)) {
			t.Fatal("accepted invalid validator report", raw)
		}
	}
	if !validPipelineValidation([]byte(`[{"pipeline":"chartworks-managed","issues":{}}]`)) {
		t.Fatal("empty known report rejected")
	}
}
func TestPipelineRenderedTargets(t *testing.T) {
	allowed := []string{
		`BEGIN;DROP TABLE IF EXISTS cw_stage.output;CREATE TABLE cw_stage.output AS SELECT id FROM inputs.sales;COMMIT;`,
		`INSERT INTO cw_stage.output SELECT id FROM inputs.sales`,
		`BEGIN;CREATE TEMP TABLE __bruin_tmp AS SELECT id FROM inputs.sales;DELETE FROM cw_stage.output WHERE id IN(SELECT id FROM __bruin_tmp);INSERT INTO cw_stage.output SELECT * FROM __bruin_tmp;DROP TABLE __bruin_tmp;COMMIT`,
		`UPDATE cw_stage.output SET id=1`,
		`MERGE INTO cw_stage.output AS target USING inputs.sales AS source ON target.id=source.id WHEN MATCHED THEN UPDATE SET id=source.id`,
		`BEGIN;SET LOCAL TIME ZONE 'UTC';INSERT INTO cw_stage.output SELECT id FROM inputs.sales;COMMIT`,
	}
	for _, sql := range allowed {
		if err := validatePipelineRendered(sql, "cw_stage", "output"); err != nil {
			t.Fatal("valid materializer output rejected", sql, err)
		}
	}
	for _, sql := range []string{``, `not sql`, `DROP TABLE inputs.sales`, `DROP TABLE cw_stage.output CASCADE`, `DELETE FROM inputs.sales`, `CREATE TABLE cw_stage.other AS SELECT 1`, `COPY cw_stage.output FROM PROGRAM 'id'`, `SET search_path=public`, `SET TIME ZONE 'UTC'`, `ROLLBACK`, `CREATE TEMP TABLE arbitrary AS SELECT 1`, `SELECT pg_sleep(1)`} {
		if err := validatePipelineRendered(sql, "cw_stage", "output"); !errors.Is(err, readexec.ErrUnsafe) {
			t.Fatal("unowned rendered mutation accepted", sql, err)
		}
	}
}
func TestPipelineRunnerArtifactAndOutputCaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner")
	data := []byte("synthetic artifact")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	v := config.DefaultPipelines()
	v.RunnerPath = path
	v.RunnerSHA256 = hex.EncodeToString(hash[:])
	if err := verifyPipelineRunner(v); err != nil {
		t.Fatal(err)
	}
	v.RunnerSHA256 = "bad"
	if !errors.Is(verifyPipelineRunner(v), ErrInvalid) {
		t.Fatal("changed runner accepted")
	}
	v.RunnerPath = dir
	if !errors.Is(verifyPipelineRunner(v), ErrInvalid) {
		t.Fatal("directory runner accepted")
	}
	v.RunnerPath = filepath.Join(dir, "missing")
	if !errors.Is(verifyPipelineRunner(v), ErrUnavailable) {
		t.Fatal("missing runner accepted")
	}
	output := boundedPipelineOutput{limit: 4}
	if n, err := output.Write([]byte("1234")); n != 4 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := output.Write([]byte("5")); n != 0 || !errors.Is(err, ErrLimit) || !bytes.Equal(output.data.Bytes(), []byte("1234")) {
		t.Fatal("output cap changed accepted bytes", n, err)
	}
	if _, err := runPipelineAsset(context.Background(), config.DefaultPipelines(), PipelineStep{}, pipelineRunnerTarget{}, "", nil, true); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid invocation accepted", err)
	}
}

func TestPipelineLineageAndDerivedGraph(t *testing.T) {
	target := pipelineRunnerTarget{Schema: "cw_stage", Table: "output", Inputs: []readexec.Relation{{ID: "input", Schema: "analytics", Name: "sales"}}}
	good := `{"name":"cw_stage.output","type":"pg.sql","upstreams":[{"name":"analytics.sales","type":"pg.source","executable_file":{"name":"source_0.asset.yml","path":"/private/assets/source_0.asset.yml","content":""},"definition_file":{"name":"source_0.asset.yml","path":"/private/assets/source_0.asset.yml","type":"yaml"}}],"downstream":[]}`
	if !validPipelineLineage([]byte(good), target, "/private") {
		t.Fatal("known dependency rejected")
	}
	for _, raw := range []string{`{}`, good + ` {}`, strings.Replace(good, "pg.source", "python", 1), strings.Replace(good, "analytics.sales", "analytics.secret", 1), strings.Replace(good, "/private/assets", "/unowned/assets", 1), strings.Replace(good, `"content":""`, `"content":"unexpected SQL"`, 1)} {
		if validPipelineLineage([]byte(raw), target, "/private") {
			t.Fatal("unbound lineage accepted", raw)
		}
	}
	for _, sql := range []string{`SELECT id FROM {{step.first}}`, `WITH x AS(SELECT id FROM {{step.first}}) SELECT id FROM x`} {
		if err := validatePipelineDerivedSQL(PipelineStep{SQL: sql, FromSteps: []string{"first"}}); err != nil {
			t.Fatal("valid derived graph rejected", err)
		}
	}
	for _, sql := range []string{`SELECT id FROM baseline.secret`, `SELECT 1`, `DELETE FROM {{step.first}}`, `WITH x AS(DELETE FROM {{step.first}} RETURNING id) SELECT id FROM x`, `SELECT id INTO other FROM {{step.first}}`, `SELECT id FROM {{step.first}};SELECT 1`, `SELECT id FROM {{step.first}} UNION ALL SELECT id FROM baseline.secret`} {
		if !errors.Is(validatePipelineDerivedSQL(PipelineStep{SQL: sql, FromSteps: []string{"first"}}), ErrInvalid) {
			t.Fatal("undeclared derived input accepted", sql)
		}
	}
	if !errors.Is(pipelineFailure(errors.New("private database diagnostic")), ErrUnavailable) {
		t.Fatal("driver text leaked")
	}
	if !errors.Is(pipelineFailure(ErrPipelineQuality), ErrPipelineQuality) {
		t.Fatal("quality type lost")
	}
	if pipelineFailure(nil) != nil {
		t.Fatal("nil failure")
	}
}

func FuzzPipelineDerivedGraph(f *testing.F) {
	f.Add("SELECT id FROM {{step.first}}")
	f.Add("WITH x AS (DELETE FROM {{step.first}} RETURNING id) SELECT id FROM x")
	f.Fuzz(func(t *testing.T, sql string) {
		if len(sql) > 65536 {
			return
		}
		_ = validatePipelineDerivedSQL(PipelineStep{SQL: sql, FromSteps: []string{"first"}})
	})
}
