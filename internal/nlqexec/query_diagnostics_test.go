package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/exec/querydiagnostic"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/store"
)

func TestSQLRecoverySpecificValidationDiagnostics(t *testing.T) {
	for _, code := range querydiagnostic.Codes() {
		t.Run(code, func(t *testing.T) {
			e := testEnvelope(t)
			a, generation, call, budget := testGeneration(t, e)
			original := []exec.Parameter{{Kind: "integer", Value: "93471928"}}
			model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT missing FROM analytics.sales WHERE id=$1", original, "sqlgen"), recoveryCandidate("SELECT id FROM analytics.sales WHERE id=$1", []exec.Parameter{{Kind: "integer", Value: "0"}}, "sqlfix")}}}
			validator := &sequenceValidator{errors: []error{exec.QueryRejection(code), nil}}
			svc := &Service{engine: model, validator: validator}
			candidate, fixes, _, _, err := svc.generateAndValidate(context.Background(), e, a, generation, call, budget, "")
			if err != nil || fixes != 1 || len(model.requests) != 2 || validator.calls != 2 || !sameParameters(candidate.Parameters, original) {
				t.Fatal("bounded diagnostic correction", err)
			}
			prompt := model.requests[1].Prompt
			if !strings.Contains(prompt, code) || !strings.Contains(prompt, "diagnostic_guidance") || !strings.Contains(prompt, querydiagnostic.Hint(code)) {
				t.Fatal("missing server-owned specific guidance")
			}
			if strings.Contains(prompt, original[0].Value) || strings.Contains(model.requests[1].System, original[0].Value) {
				t.Fatal("diagnostic copied private binding")
			}
		})
	}
}
func TestSQLRecoverySpecificDiagnosticReceiptGate(t *testing.T) {
	// Pure candidate shape/complexity rejections keep the existing validation
	// repair. Only the contradictory joined source failure is terminal.
	for _, err := range []error{exec.ErrUnsafe, exec.ErrUnsupported, exec.ErrLimit} {
		if !validationRepairable(err) {
			t.Fatal("candidate validation repair regressed", err)
		}
	}
	now := time.Now().UTC()
	for _, code := range querydiagnostic.Codes() {
		report := exec.ExecutionReport{Attempt: exec.Attempt{Status: "failed", Code: code, RemoteState: "stopped", Finished: &now}}
		for _, err := range []error{nil, exec.QueryRejection(code)} {
			if !executionRepairable(report, err) {
				t.Fatal("confirmed rejection cannot reach existing repair", code)
			}
		}
		for _, terminal := range []error{exec.ErrUncertain, exec.ErrTimeout, exec.ErrCancelled, exec.ErrBinding, exec.ErrLimit, exec.ErrType, exec.ErrUnsupported, exec.ErrUnsafe, exec.ErrReplay, store.ErrInvalid, store.ErrNotFound, context.Canceled, context.DeadlineExceeded, store.ErrUnavailable, store.ErrConflict, access.ErrForbidden, access.ErrNotFound, access.ErrUnauthenticated} {
			if executionRepairable(report, errors.Join(exec.QueryRejection(code), terminal)) || validationRepairable(errors.Join(exec.QueryRejection(code), terminal)) {
				t.Fatal("terminal cause allowed repair", code, terminal)
			}
		}
		for _, change := range []func(*exec.ExecutionReport){func(r *exec.ExecutionReport) { r.Attempt.RemoteState = "unknown" }, func(r *exec.ExecutionReport) { r.Attempt.RemoteState = "running" }, func(r *exec.ExecutionReport) { r.Attempt.RemoteState = "not_issued" }, func(r *exec.ExecutionReport) { r.Attempt.Finished = nil }, func(r *exec.ExecutionReport) { r.Attempt.Status = "uncertain" }, func(r *exec.ExecutionReport) { r.Result = &exec.Result{} }, func(r *exec.ExecutionReport) { r.Attempt.Code = "query_error PRIVATE" }} {
			bad := report
			change(&bad)
			if executionRepairable(bad, nil) {
				t.Fatal("unproven diagnostic receipt authorized repair")
			}
			if code != "query_error" && executionRepairable(bad, exec.QueryRejection(code)) {
				t.Fatal("typed diagnostic overrode an unproven receipt")
			}
		}
		if code != "query_error" {
			if executionRepairable(report, exec.ErrQuery) {
				t.Fatal("mismatched generic error expanded into specific diagnosis")
			}
			bad := report
			bad.Attempt.Rows = 1
			if executionRepairable(bad, exec.QueryRejection(code)) {
				t.Fatal("partial results authorized repair")
			}
		}
	}
}
func TestSQLRecoveryDiagnosticPacketRejectsUntrustedReason(t *testing.T) {
	e := testEnvelope(t)
	_, generation, _, _ := testGeneration(t, e)
	candidate := generatedCandidate{SQL: "SELECT id FROM analytics.sales"}
	for _, code := range []string{"22012", "query_error PRIVATE", "PRIVATE_ERROR", "query_grouping\x00"} {
		if _, err := validationRepairContext(context.Background(), generation, candidate, code); !errors.Is(err, ErrGeneration) {
			t.Fatal("raw diagnostic reached model")
		}
	}
	if code := validationCode(errors.New("query_grouping PRIVATE"), candidate.SQL); code != "validation_failed" {
		t.Fatal("diagnosed raw error string", code)
	}
}
