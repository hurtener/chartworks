package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// FrozenRunInput is protected Phase 24 consumer material. A selected accepted
// case names a real published block and its run intent, not a stored result.
type FrozenRunInput struct {
	BlockID string               `json:"block_id"`
	Request reporting.RunRequest `json:"request"`
}

func runFrozenInput(ctx context.Context, e identity.Envelope, runs frozenRunRuntime, repo frozenRunReader, in FrozenRunInput) (reporting.RunRecord, error) {
	if ctx == nil || !e.Valid() || runs == nil || repo == nil || !identity.Identifier(in.BlockID) || !identity.Identifier(in.Request.Key) {
		return reporting.RunRecord{}, ErrMode
	}
	admitted, err := runs.Admit(ctx, e, in.BlockID, in.Request)
	if err != nil || !identity.Identifier(admitted.ID) || admitted.Block != in.BlockID {
		return reporting.RunRecord{}, fmt.Errorf("%w: frozen admission: %v", ErrPerformanceEvidence, err)
	}
	finished, err := runs.Run(ctx, e, admitted.ID, false)
	if err != nil || finished.ID != admitted.ID || finished.State != "succeeded" {
		failed, _ := repo.ReadFrozenRun(ctx, e, admitted.ID, true)
		codes := make([]string, 0, len(failed.Outputs))
		for _, output := range failed.Outputs {
			codes = append(codes, output.ID+":"+output.State+":"+output.Code)
		}
		return reporting.RunRecord{}, fmt.Errorf("%w: frozen execution state=%s code=%s outputs=%v: %v", ErrPerformanceEvidence, finished.State, finished.Code, codes, err)
	}
	record, err := repo.ReadFrozenRun(ctx, e, admitted.ID, true)
	if err != nil || record.View.ID != finished.ID || record.View.ManifestDigest != finished.ManifestDigest {
		return reporting.RunRecord{}, fmt.Errorf("%w: frozen receipt: %v", ErrPerformanceEvidence, err)
	}
	return record, nil
}

type frozenOutputSemantic struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Narrative string `json:"narrative,omitempty"`
}

type frozenSemantic struct {
	Result struct {
		Schema     []exec.Field        `json:"schema"`
		Rows       [][]json.RawMessage `json:"rows"`
		Outcome    string              `json:"outcome"`
		Truncation string              `json:"truncation"`
	} `json:"result"`
	Outputs []frozenOutputSemantic `json:"outputs"`
}

// frozenEvidence reads product receipts from the newly admitted run. Copied
// narrative output on a reused run is content, not a fresh model invocation.
// Missing native source duration stays nil and fails the integration gate.
func frozenEvidence(record reporting.RunRecord) (frozenSemantic, int, *int64, gateway.Receipt, error) {
	m := record.Manifest
	v := record.View
	if m == nil || record.Result == nil || v.State != "succeeded" || !identity.Identifier(v.ID) || m.ID != v.ID || m.Block != v.Block || m.ReuseKey == "" || m.ReuseKey != reporting.ReuseIdentity(*m) || v.ManifestDigest != m.Digest() {
		return frozenSemantic{}, 0, nil, gateway.Receipt{}, fmt.Errorf("%w: noncanonical frozen manifest", ErrPerformanceEvidence)
	}
	if v.ReusedFrom != "" && (!identity.Identifier(v.ReusedFrom) || v.ReusedFrom == v.ID || len(v.QueryAttempts) != 0) {
		return frozenSemantic{}, 0, nil, gateway.Receipt{}, ErrPerformanceEvidence
	}
	semantic := frozenSemantic{Outputs: make([]frozenOutputSemantic, 0, len(record.Outputs))}
	semantic.Result.Schema = record.Result.Schema
	semantic.Result.Rows = record.Result.Rows
	semantic.Result.Outcome = record.Result.Outcome
	semantic.Result.Truncation = record.Result.Truncation
	var receipt gateway.Receipt
	for _, out := range record.Outputs {
		if out.State != "succeeded" || !identity.Identifier(out.ID) {
			return frozenSemantic{}, 0, nil, gateway.Receipt{}, ErrPerformanceEvidence
		}
		selected := frozenOutputSemantic{ID: out.ID, Kind: out.Kind}
		if out.Narrative != nil {
			selected.Narrative = out.Narrative.Text
			if v.ReusedFrom == "" {
				receipt.Append(out.Narrative.Receipt)
			}
		}
		semantic.Outputs = append(semantic.Outputs, selected)
	}
	if v.ReusedFrom != "" {
		return semantic, 0, nil, gateway.Receipt{}, nil
	}
	var sourceNS int64
	complete := true
	for _, attempt := range v.QueryAttempts {
		if !identity.Identifier(attempt.ID) || attempt.Status != "succeeded" || attempt.RemoteState != "stopped" || attempt.Finished == nil || attempt.Manifest.Receipt.Dialect != "postgres" || attempt.SourceDurationNS == nil || *attempt.SourceDurationNS <= 0 {
			complete = false
			continue
		}
		sourceNS += *attempt.SourceDurationNS
	}
	if len(v.QueryAttempts) == 0 {
		return frozenSemantic{}, 0, nil, gateway.Receipt{}, ErrPerformanceEvidence
	}
	if !complete || sourceNS <= 0 {
		return semantic, len(v.QueryAttempts), nil, receipt, nil
	}
	return semantic, len(v.QueryAttempts), &sourceNS, receipt, nil
}
