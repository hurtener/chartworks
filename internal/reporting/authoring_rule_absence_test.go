package reporting

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAuthoringRuleAbsencePreservesLegacyAndDerivedRevisions(t *testing.T) {
	d := contractDefinition()
	legacy := Revision{Definition: d, Provenance: Provenance{Kind: "manual"}}
	raw, err := json.Marshal(legacy.Provenance)
	if err != nil || string(raw) != `{"kind":"manual"}` {
		t.Fatal("legacy provenance bytes changed", string(raw), err)
	}
	beforeDefinition, beforeExecution := DefinitionDigest(d), ExecutionDigest(d)
	if pins, err := AuthoringRuleAbsence(legacy); err != nil || len(pins) != 0 {
		t.Fatal("legacy policy invented", pins, err)
	}
	marked := clone(legacy)
	marked.Provenance.RuleAbsence = clone(d.Topics)
	if DefinitionDigest(marked.Definition) != beforeDefinition || ExecutionDigest(marked.Definition) != beforeExecution {
		t.Fatal("provenance rewrote definition identity")
	}
	for _, kind := range []string{"copy", "amendment", "restore", "rename", "parameterize"} {
		after := clone(marked)
		after.Provenance.Kind = kind
		after.Provenance.ParentRevision = 1
		if err := RetainAuthoringRuleAbsence(marked, after); err != nil {
			t.Fatal(kind, err)
		}
		after.Provenance.RuleAbsence = nil
		if !errors.Is(RetainAuthoringRuleAbsence(marked, after), ErrStale) {
			t.Fatal("stripped origin fence", kind)
		}
	}
	for _, alter := range []func(*Revision){
		func(r *Revision) { r.Definition.Topics[0].Version = "v2" },
		func(r *Revision) { r.Definition.Topics[0].Digest = strings.Repeat("b", 64) },
		func(r *Revision) { r.Definition.Rules = []RulePin{{Topic: d.Topics[0].Topic}} },
	} {
		bad := clone(marked)
		alter(&bad)
		if _, err := AuthoringRuleAbsence(bad); !errors.Is(err, ErrStale) {
			t.Fatal("origin policy silently rebased", err)
		}
	}
	if err := RetainAuthoringRuleAbsence(legacy, legacy); err != nil {
		t.Fatal("unrelated legacy mutation changed", err)
	}
}

func TestPreparedOriginRulesRejectValidationAndPreviewBeforeRead(t *testing.T) {
	s, repo, b, e, in := preparationServiceFixture(t)
	prepared, err := s.PrepareDatasetChart(t.Context(), e, in)
	if err != nil || prepared.Status != "prepared" {
		t.Fatal(prepared, err)
	}
	created, err := s.CreatePreparedChart(t.Context(), e, AuthoringCreatePreparedRequest{NewBlock: in.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal(err)
	}
	repo.activeRules = true
	before := b.physical.Load()
	blocks, _ := s.blockService()
	request := ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision}
	if _, err := blocks.Validate(t.Context(), e, in.NewBlock, request); !errors.Is(err, ErrStale) {
		t.Fatal("activated rules ignored by native validation", err)
	}
	if _, err := blocks.Preview(t.Context(), e, in.NewBlock, PreviewRequest{ValidateRequest: request, Outputs: []string{"chart"}}); !errors.Is(err, ErrStale) {
		t.Fatal("activated rules ignored by native preview", err)
	}
	if b.physical.Load() != before {
		t.Fatal("rule rejection performed another data read")
	}
}

func TestPreparedOriginRulesActivateDuringValidation(t *testing.T) {
	s, repo, b, e, in := preparationServiceFixture(t)
	prepared, err := s.PrepareDatasetChart(t.Context(), e, in)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreatePreparedChart(t.Context(), e, AuthoringCreatePreparedRequest{NewBlock: in.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal(err)
	}
	b.onRead = func() { repo.mu.Lock(); repo.activeRules = true; repo.mu.Unlock() }
	blocks, _ := s.blockService()
	if _, err := blocks.Validate(t.Context(), e, in.NewBlock, ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision}); !errors.Is(err, ErrStale) {
		t.Fatal("during-read rule activation blessed", err)
	}
	if repo.heads[authoringBlockKey(e.Tenant(), in.NewBlock)].DraftState != "draft" {
		t.Fatal("stale read became native validation")
	}
}

func TestPreparedOriginMappingCopyAndRestoreKeepFence(t *testing.T) {
	s, repo, b, e, in := preparationServiceFixture(t)
	prepared, err := s.PrepareDatasetChart(t.Context(), e, in)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreatePreparedChart(t.Context(), e, AuthoringCreatePreparedRequest{NewBlock: in.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal(err)
	}
	mapping := in.Intent.Mapping
	mapping.Kind = "column"
	edited, err := s.PatchBlockMapping(t.Context(), e, AuthoringBlockMappingRequest{Block: in.NewBlock, ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision, Digest: created.Block.Digest, Output: "chart", Mapping: mapping})
	if err != nil {
		t.Fatal("mapping amendment", err)
	}
	blocks, _ := s.blockService()
	restored, err := blocks.Restore(t.Context(), e, in.NewBlock, RestoreRequest{ExpectedVersion: edited.Block.State.Version, Revision: created.Block.Revision, Note: "Restore exact prepared mapping"})
	if err != nil {
		t.Fatal("restore", err)
	}
	copied, err := s.CopyBlockMapping(t.Context(), e, AuthoringBlockCopyRequest{Block: in.NewBlock, ExpectedVersion: restored.State.Version, Revision: restored.Revision, Digest: restored.Digest, Output: "chart", NewBlock: "block", Mapping: in.Intent.Mapping})
	if err != nil {
		t.Fatal("copy restored origin", err)
	}
	for _, key := range []string{authoringBlockKey(e.Tenant(), in.NewBlock), authoringBlockKey(e.Tenant(), "block")} {
		for _, snapshot := range repo.revisions[key] {
			pins, err := AuthoringRuleAbsence(snapshot.Revision)
			if err != nil || len(pins) != 1 || pins[0] != in.Intent.Topic {
				t.Fatal("derived revision lost exact origin", key, err)
			}
		}
	}
	repo.activeRules = true
	before := b.physical.Load()
	if _, err := blocks.Validate(t.Context(), e, "block", ValidateRequest{ExpectedVersion: copied.Block.State.Version, Revision: copied.Block.Revision}); !errors.Is(err, ErrStale) {
		t.Fatal("copy laundered active rule policy", err)
	}
	if b.physical.Load() != before {
		t.Fatal("derived stale origin executed a query")
	}
}
