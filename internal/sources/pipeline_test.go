package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/jackc/pgx/v5"
)

func pipelineFixture() PipelineStage {
	return SealPipelineStage(PipelineStage{Tenant: "tenant", Actor: "actor", Session: "session", Pipeline: "pipeline", Version: 3, Revision: 2, Operation: "operation", Step: "clean", Alias: "managed", Schema: "cw_stage", Table: "cw_p_table", OID: 42, Columns: []string{"id", "value"}, Source: "pipeline.clean", Context: "pipeline.clean:v2", State: "checked"})
}

func TestPrivatePipelineValidationAuthorityIsManifestBound(t *testing.T) {
	stage := pipelineFixture()
	binding := readexec.Binding{
		Tenant: stage.Tenant, Source: "pipeline.next", Context: "pipeline.next:v1", Revision: 1, Dialect: "postgres",
		Contract: "pipeline-contract", Fingerprint: strings.Repeat("a", 64),
		Relations: []readexec.Relation{{ID: "private-dataset", Schema: stage.Schema, Name: stage.Table, Columns: []readexec.Column{{Name: "id", NativeType: "int8", Category: "numeric", Safe: true}}}},
	}
	adapter := &pipelineInput{operation: stage.Operation, targetSource: binding.Source, targetContext: binding.Context, stages: []PipelineStage{stage}, binding: binding}
	e, err := identity.FromVerified(stage.Tenant, stage.Actor, stage.Session, []string{"engineering.pipeline.run", "cw.source.write:" + stage.Pipeline}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := adapter.AuthorizePrivatePipeline(e, binding, []string{"private-dataset"})
	if err != nil || len(proof) != 64 {
		t.Fatal("exact private pipeline manifest rejected", err)
	}
	other, err := identity.FromVerified(stage.Tenant, stage.Actor, stage.Session, []string{"engineering.pipeline.run", "cw.source.write:other-pipeline"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.AuthorizePrivatePipeline(other, binding, []string{"private-dataset"}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("cross-pipeline authority admitted", err)
	}
	adapter.operation = "other-operation"
	if _, err = adapter.AuthorizePrivatePipeline(e, binding, []string{"private-dataset"}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("wrong operation admitted", err)
	}
}

func TestPipelineStageAndPrivateRecordBinding(t *testing.T) {
	stage := pipelineFixture()
	if !stage.Valid() || !stage.Location().Valid() {
		t.Fatal("sealed stage rejected")
	}
	changedLocation := stage.Location()
	changedLocation.OID++
	if changedLocation.Valid() {
		t.Fatal("mutated persisted location accepted")
	}
	for _, mutate := range []func(*PipelineStage){
		func(s *PipelineStage) { s.Revision++ },
		func(s *PipelineStage) { s.OID = 0 },
		func(s *PipelineStage) { s.State = "applied" },
		func(s *PipelineStage) { s.Columns = append(s.Columns, s.Columns[0]) },
		func(s *PipelineStage) { s.Digest = strings.Repeat("g", 64) },
	} {
		changed := stage
		changed.Columns = append([]string(nil), stage.Columns...)
		mutate(&changed)
		if changed.Valid() {
			t.Fatal("mutated stage evidence accepted", changed)
		}
	}
	relation := readexec.Relation{ID: "dataset", Schema: stage.Schema, Name: stage.Table, Columns: []readexec.Column{{Name: "id", NativeType: "int8", Category: "numeric", Safe: true}, {Name: "value", NativeType: "text", Category: "text", Safe: true}}}
	binding := readexec.Binding{Tenant: stage.Tenant, Source: stage.Source, Context: stage.Context, Revision: stage.Revision, Dialect: "postgres", Contract: "pipeline-contract", Fingerprint: strings.Repeat("a", 64), Relations: []readexec.Relation{relation}}
	record := Record{Source: Source{ID: stage.Source, Name: "Pipeline clean", Dialect: "postgres", Revision: stage.Revision, ContextID: stage.Context, Status: "registered"}, Connection: stage.Alias, Binding: binding, Pipeline: ptrLocation(stage.Location())}
	if !record.Valid() {
		t.Fatal("private pipeline record rejected")
	}
	service := &Service{settings: config.Sources{Connections: []config.SourceConnection{{Tenant: stage.Tenant, ID: stage.Alias, ManagedSchema: "cw_managed"}}}}
	connection, err := service.recordConnection(record)
	if err != nil || len(connection.Relations) != 1 || connection.Relations[0].Schema != stage.Schema || connection.Relations[0].Name != stage.Table {
		t.Fatal("private physical location was replaced by ordinary managed location", connection, err)
	}
	record.Pipeline.Table = "replacement"
	if record.Valid() {
		t.Fatal("location detached from bound relation")
	}
}

func TestPipelineReadLocationUsesHeldPhysicalDatabase(t *testing.T) {
	read, location, err := ParseApprovedDSN("postgres://reader:synthetic@localhost:5432/first?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	writer := read.Copy()
	writer.User = "writer"
	writer.Password = "different"
	held := withPipelineReadLocation(context.Background(), read, location)
	if err = RequirePipelineReadLocation(held, writer); err != nil {
		t.Fatal("same database separate writer rejected", err)
	}
	for _, change := range []func(*pgx.ConnConfig){func(c *pgx.ConnConfig) { c.Database = "second" }, func(c *pgx.ConnConfig) { c.Host = "other" }, func(c *pgx.ConnConfig) { c.Port++ }} {
		changed := writer.Copy()
		change(changed)
		if !errors.Is(RequirePipelineReadLocation(held, changed), readexec.ErrBinding) {
			t.Fatal("different physical location admitted")
		}
	}
	if !errors.Is(RequirePipelineReadLocation(context.Background(), writer), readexec.ErrBinding) {
		t.Fatal("missing held native proof admitted")
	}
	//nolint:staticcheck // Deliberately verifies rejection of a nil authority context.
	if !errors.Is(RequirePipelineReadLocation(nil, writer), readexec.ErrBinding) {
		t.Fatal("nil context admitted")
	}
}
