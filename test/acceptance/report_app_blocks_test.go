package acceptance

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/store"
)

// Real PostgreSQL custody, immutable definitions/results and version CAS. Setup
// uses only synthetic source data and recorded providers. The authoring domain
// below has no source executor or model configured, and counters prove no work.
func TestReportAppBlockMappingAuthoring(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	const sourceID, copyID = "mapping-source", "mapping-copy"
	f.block(t, sourceID, f.base)
	source, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, sourceID, reporting.Reference{}, reporting.Write)
	if err != nil {
		t.Fatal(err)
	}
	// Keep a real frozen result to prove authoring changes never rewrite it.
	run, err := f.runs.Admit(ctx, f.execute, sourceID, reporting.RunRequest{Key: "mapping-before-copy", Outputs: []string{"table-main"}, Locale: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	run, err = f.runs.Run(ctx, f.execute, run.ID, false)
	if err != nil || run.State != "succeeded" {
		t.Fatal("retained setup", run, err)
	}
	retainedBefore, err := f.runs.Output(ctx, f.execute, run.ID, "table-main")
	if err != nil {
		t.Fatal(err)
	}
	_, topics := newPhase18Service(t, f.f)
	metadataBlocks, err := reporting.New(f.f.f.db, topics, nil, nil, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	documents, err := reporting.NewDocuments(f.f.f.db, metadataBlocks, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewAuthoring(documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "topics.read", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.read:" + sourceID, "cw.block.preview:" + sourceID, "cw.block.read:" + copyID, "cw.block.write:" + copyID, "cw.block.preview:" + copyID, "cw.topic.write:" + source.State.Topic}
	for _, ref := range source.References {
		scope := "cw." + ref.Kind + "." + ref.Permission + ":" + ref.ID
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	actor := func(tenant, user string, grants []string) identity.Envelope {
		t.Helper()
		claims := f.f.f.token.claims(tenant, user, grants)
		claims["session"] = "phase27-session"
		e, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	e := actor(f.author.Tenant(), f.author.User(), scopes)
	mapping := reporting.AuthoringChartMapping{Kind: charts.KPI, Bindings: charts.Bindings{Value: "c1"}, Order: []charts.Order{}, Options: charts.DefaultOptions()}
	copyRequest := reporting.AuthoringBlockCopyRequest{Block: sourceID, NewBlock: copyID, Revision: source.Revision.Number, Digest: source.Revision.Digest, ExpectedVersion: source.State.Version, Output: "table-main", Mapping: mapping}
	beforeSource, beforeModel, beforeAttempts := f.f.f.lookups.Load(), f.f.model.requests.Load(), f.attemptCount(t)

	t.Run("sql-free-read-and-purpose-specific-copy-custody", func(t *testing.T) {
		view, err := service.ReadBlock(ctx, e, reporting.AuthoringBlockReadRequest{Block: sourceID, Revision: 1})
		if err != nil || view.Block.Digest != source.Revision.Digest || len(view.OutputColumns) != 2 {
			t.Fatal(view, err)
		}
		raw, _ := json.Marshal(view)
		if strings.Contains(string(raw), `"sql"`) || strings.Contains(string(raw), f.base.SQL) {
			t.Fatal("SQL escaped authoring read")
		}
		redacted, err := f.f.f.db.ReadBlock(ctx, e, sourceID, reporting.Reference{Revision: 1}, reporting.Read)
		if err != nil || redacted.Revision.Definition.SQL != "" {
			t.Fatal("ordinary store read unredacted", err)
		}
		full, err := f.f.f.db.ReadBlockForAuthoringCopy(ctx, e, sourceID, reporting.Reference{Revision: 1})
		if err != nil || full.Revision.Definition.SQL != source.Revision.Definition.SQL {
			t.Fatal("exact copy custody failed", err)
		}
		reader := actor(f.author.Tenant(), f.author.User(), slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == "cw.block.preview:"+sourceID }))
		if _, err := service.CopyBlockMapping(ctx, reader, copyRequest); err == nil {
			t.Fatal("read-only source authority allowed copy")
		}
		if _, err := f.f.f.db.ReadBlockForAuthoringCopy(ctx, reader, sourceID, reporting.Reference{Revision: 1}); err == nil {
			t.Fatal("direct copy seam widened authority")
		}
	})

	t.Run("exact-target-tenant-topic-dependencies-and-wire-denials", func(t *testing.T) {
		for _, missing := range []string{"charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.write:" + copyID, "cw.topic.write:" + source.State.Topic, "cw.execution_context.use:" + f.base.Context} {
			denied := actor(f.author.Tenant(), f.author.User(), slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == missing }))
			if _, err := service.CopyBlockMapping(ctx, denied, copyRequest); err == nil {
				t.Fatal("missing scope admitted", missing)
			}
		}
		foreignScopes := slices.Clone(scopes)
		for i, s := range foreignScopes {
			foreignScopes[i] = strings.ReplaceAll(s, ":"+f.author.Tenant(), ":foreign")
		}
		if _, err := service.CopyBlockMapping(ctx, actor("foreign", f.author.User(), foreignScopes), copyRequest); err == nil {
			t.Fatal("cross-tenant source copy")
		}
		bad := copyRequest
		bad.NewBlock = "other-target"
		if _, err := service.CopyBlockMapping(ctx, e, bad); err == nil {
			t.Fatal("guessed new target accepted")
		}
		bad = copyRequest
		bad.Digest = strings.Repeat("a", 64)
		if _, err := service.CopyBlockMapping(ctx, e, bad); !errors.Is(err, store.ErrConflict) {
			t.Fatal("wrong digest", err)
		}
		bad = copyRequest
		bad.ExpectedVersion++
		if _, err := service.CopyBlockMapping(ctx, e, bad); !errors.Is(err, store.ErrConflict) {
			t.Fatal("wrong head version", err)
		}
		registry, err := reportingapi.AuthoringRegistry()
		if err != nil {
			t.Fatal(err)
		}
		handler := assertRegisteredWireSchemas(t, registry, reportingapi.AuthoringHandler(f.f.f.token.verifier, service, http.NotFoundHandler()))
		raw, _ := json.Marshal(copyRequest)
		var in map[string]any
		_ = json.Unmarshal(raw, &in)
		in["sql"] = "SELECT secret"
		badRaw, _ := json.Marshal(in)
		claims := f.f.f.token.claims(f.author.Tenant(), f.author.User(), scopes)
		response := callProtected(t, handler, "POST", reportingapi.AuthoringPath+"block_copy", f.f.f.token.sign(t, claims, nil), string(badRaw), map[string]string{"Content-Type": "application/json"})
		if response.Code != http.StatusBadRequest {
			t.Fatal("SQL input was not rejected", response.Code, response.Body.String())
		}
	})

	copied, err := service.CopyBlockMapping(ctx, e, copyRequest)
	if err != nil {
		t.Fatal("copy exact published block", err)
	}
	if copied.Block.State.ID != copyID || !copied.Block.Private || copied.Block.State.Version != 1 || copied.Block.State.PublishedRevision != 0 || copied.Block.Evidence != nil || copied.Block.Trust.Certification != "none" || copied.DataValidation != "not_performed" {
		t.Fatal("copy retained approval", copied)
	}
	copiedStored, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, copyID, reporting.Reference{Revision: 1}, reporting.Write)
	if err != nil {
		t.Fatal(err)
	}
	expected := phase27Copy(t, source.Revision.Definition)
	expected.Outputs[0] = copiedStored.Revision.Definition.Outputs[0]
	if !reflect.DeepEqual(expected, copiedStored.Revision.Definition) || copiedStored.Validation != nil || copiedStored.Attestation != nil || copiedStored.PublishedAt != nil {
		t.Fatal("copy altered untargeted definition or transferred approval")
	}
	if _, err := service.CopyBlockMapping(ctx, e, copyRequest); !errors.Is(err, store.ErrConflict) {
		t.Fatal("new-target create CAS missing", err)
	}
	if _, err := f.blocks.Publish(ctx, f.blockAuthor, copyID, reporting.PublishRequest{ExpectedVersion: 1, Evidence: source.Validation.Evidence.ID}); err == nil {
		t.Fatal("source evidence published copied mapping")
	}

	t.Run("private-copy-owner-remains-required", func(t *testing.T) {
		other := actor(f.author.Tenant(), "another-actor", scopes)
		if _, err := service.ReadBlock(ctx, other, reporting.AuthoringBlockReadRequest{Block: copyID, Revision: 1}); err == nil {
			t.Fatal("private copy exposed to other actor")
		}
		patch := reporting.AuthoringBlockMappingRequest{Block: copyID, Revision: 1, Digest: copied.Block.Digest, ExpectedVersion: 1, Output: "table-main", Mapping: mapping}
		if _, err := service.PatchBlockMapping(ctx, other, patch); err == nil {
			t.Fatal("private copy editable by other actor")
		}
	})
	if f.f.f.lookups.Load() != beforeSource || f.f.model.requests.Load() != beforeModel || f.attemptCount(t) != beforeAttempts {
		t.Fatal("mapping authoring executed source or model work")
	}
	// A multirow KPI can bind structurally but fails fresh actual-data validation.
	liveAuthoring, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	validationScopes := append(slices.Clone(scopes), "reporting.validate", "sources.query", "sources.read", "cw.source.query:"+f.base.Source)
	validationActor := actor(f.author.Tenant(), f.author.User(), validationScopes)
	if _, err := liveAuthoring.ValidateBlock(ctx, validationActor, reporting.AuthoringBlockValidateRequest{Block: copyID, Revision: 1, Digest: copied.Block.Digest, ExpectedVersion: 1}); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("schema-only binding substituted for actual validation", err)
	}

	t.Run("private-mapping-cas-and-immutable-source-result", func(t *testing.T) {
		patch := reporting.AuthoringBlockMappingRequest{Block: copyID, Revision: 1, Digest: copied.Block.Digest, ExpectedVersion: 1, Output: "table-main", Mapping: reporting.AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"field_0", "c1"}}, Order: []charts.Order{}, Options: charts.DefaultOptions()}}
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; _, err := service.PatchBlockMapping(ctx, e, patch); results <- err }()
		}
		close(start)
		wg.Wait()
		close(results)
		winners := 0
		for err := range results {
			if err == nil {
				winners++
			} else if !errors.Is(err, store.ErrConflict) {
				t.Fatal(err)
			}
		}
		if winners != 1 {
			t.Fatal("concurrent mapping CAS", winners)
		}
		patch.ExpectedVersion = 2
		if _, err := service.PatchBlockMapping(ctx, e, patch); !errors.Is(err, store.ErrConflict) {
			t.Fatal("fresh CAS rolled back from stale revision", err)
		}
		current, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, copyID, reporting.Reference{Draft: true}, reporting.Write)
		if err != nil || current.Revision.Number != 2 || current.State.Version != 2 || current.Validation != nil {
			t.Fatal(current, err)
		}
		original, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, sourceID, reporting.Reference{Revision: 1}, reporting.Write)
		if err != nil || !reflect.DeepEqual(original.Revision, source.Revision) || original.State.Version != source.State.Version {
			t.Fatal("published source changed", err)
		}
		retainedAfter, err := f.runs.Output(ctx, f.execute, run.ID, "table-main")
		if err != nil || !reflect.DeepEqual(retainedBefore, retainedAfter) {
			t.Fatal("retained result changed", err)
		}
		oldCopy, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, copyID, reporting.Reference{Revision: 1}, reporting.Write)
		if err != nil || !reflect.DeepEqual(oldCopy.Revision, copiedStored.Revision) {
			t.Fatal("retained copy revision changed", err)
		}
		validated, err := liveAuthoring.ValidateBlock(ctx, validationActor, reporting.AuthoringBlockValidateRequest{Block: copyID, Revision: 2, Digest: current.Revision.Digest, ExpectedVersion: 2})
		if err != nil || validated.Evidence.DefinitionDigest != current.Revision.Digest {
			t.Fatal("fresh validation did not bind new mapping", err)
		}
		validationRaw, err := json.Marshal(validated)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{`"sql"`, `"rows"`, `"attempt"`, `"session"`, `"remote"`, `"instructions"`} {
			if strings.Contains(string(validationRaw), private) {
				t.Fatal("private validation payload escaped", private)
			}
		}
		view, err := service.ReadBlock(ctx, e, reporting.AuthoringBlockReadRequest{Block: copyID, Revision: 2})
		if err != nil || !view.Block.Private || view.Block.State.PublishedRevision != 0 {
			t.Fatal("validation implicitly published", err)
		}
	})
}
