package acceptance

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hurtener/chartworks/internal/evaluation"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
)

// Like Phase24's expected-failed evaluation, success of this regression means
// the quality evaluation failed in the exact pinned way. It does not promote
// any expected answer to an expected refusal or claim calibrated quality.
func pairedBaselineFailureGate(report pairedReport, training, heldout []pairedCase) error {
	if err := pairedSharedControlGate(report.SharedControls, heldout); err != nil {
		return err
	}
	if report.GatePassed || report.Calibration != "unknown" {
		return fmt.Errorf("baseline quality failure was relabelled")
	}
	expected := map[string]pairedCase{}
	for _, c := range heldout {
		if !pairedIsSharedControl(c) {
			expected[c.ID] = c
		}
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, cell := range report.Cases {
		if cell.Arm == "D" {
			continue
		}
		if cell.Arm != "A" && cell.Arm != "B" && cell.Arm != "C" {
			return fmt.Errorf("unknown baseline arm")
		}
		want, ok := expected[cell.Case.ID]
		key := cell.Arm + ":" + cell.Case.ID
		if !ok || seen[key] || readexec.Hash(want) != readexec.Hash(cell.Case) || !pairedPredecessorPreserved(cell) || cell.Matches && cell.Actual != cell.Case.Expected {
			return fmt.Errorf("baseline classification or frozen contract changed: %s", key)
		}
		seen[key] = true
		counts[cell.Arm]++
	}
	for _, arm := range []string{"A", "B", "C"} {
		if counts[arm] != 28 {
			return fmt.Errorf("incomplete baseline arm %s", arm)
		}
	}
	expected = map[string]pairedCase{}
	for _, c := range training {
		expected[c.ID] = c
	}
	seen = map[string]bool{}
	for _, cell := range report.Training {
		want, ok := expected[cell.Case.ID]
		if !ok || seen[cell.Case.ID] || cell.Arm != "TRAIN" || readexec.Hash(want) != readexec.Hash(cell.Case) || !pairedPredecessorPreserved(cell) || cell.Matches && cell.Actual != cell.Case.Expected {
			return fmt.Errorf("training baseline changed")
		}
		seen[cell.Case.ID] = true
	}
	if len(seen) != 12 {
		return fmt.Errorf("incomplete training baseline")
	}
	return evaluation.ErrGate
}

func pairedRepairPositiveGate(report pairedReport, heldout []pairedCase) error {
	if report.RepairPolicy != nlqroute.GroundedCalendarPolicy {
		return fmt.Errorf("repair policy changed")
	}
	expected := map[string]pairedCase{}
	for _, c := range heldout {
		if !pairedIsSharedControl(c) {
			expected[c.ID] = c
		}
	}
	seen := map[string]bool{}
	for _, cell := range report.Cases {
		if cell.Arm != "D" {
			continue
		}
		want, ok := expected[cell.Case.ID]
		if !ok || seen[cell.Case.ID] || readexec.Hash(want) != readexec.Hash(cell.Case) || !cell.Matches || cell.Actual != cell.Case.Expected || cell.Learning != "no_examples" {
			return fmt.Errorf("D result or frozen contract changed: %s", cell.Case.ID)
		}
		seen[cell.Case.ID] = true
	}
	if len(seen) != 28 {
		return fmt.Errorf("incomplete D arm")
	}
	if err := pairedSharedControlGate(report.SharedControls, heldout); err != nil {
		return err
	}
	return nil
}

func TestPairedExpectedFailedEvaluationMutations(t *testing.T) {
	_, training, heldout, _, _ := freezePaired(t)
	report := pairedReport{Calibration: "unknown", RepairPolicy: nlqroute.GroundedCalendarPolicy}
	// Synthetic classification-only fixture. Native rows and scoring still run
	// in the recorded tests; this fixture attacks only the gate assertions.
	for _, arm := range []string{"A", "B", "C", "D"} {
		for _, c := range heldout {
			if c.Contract == "stale-profile" || c.Contract == "sensitive-note" || c.Contract == "wrong-context" {
				if arm == "D" {
					report.SharedControls = append(report.SharedControls, pairedCell{Arm: "shared", Case: c, Matches: true, Actual: c.Expected})
				}
				continue
			}
			cell := pairedCell{Arm: arm, Case: c, Matches: true, Actual: c.Expected, Learning: "no_examples"}
			if arm != "D" {
				switch c.ID {
				case "fresh-known-gross-es", "fresh-paid-count-en", "fresh-known-count-en", "fresh-known-count-es", "fresh-cohort-net-en", "fresh-activity-net-en", "fresh-activity-net-es":
					cell.Matches = false
					cell.Actual = "clarify"
					cell.Reason = "invalid_temporal_span"
				case "fresh-monthly-known-en":
					cell.Matches = false
					cell.Actual = "reject"
					cell.Reason = "analytical_group_domain_review_required"
				}
			}
			report.Cases = append(report.Cases, cell)
		}
	}
	for _, c := range training {
		cell := pairedCell{Arm: "TRAIN", Case: c, Matches: true, Actual: "answer"}
		if c.ID != "train-3-en" && c.ID != "train-4-en" && c.ID != "train-6-en" {
			cell.Matches = false
			cell.Actual = "clarify"
			cell.Reason = "invalid_temporal_span"
		}
		report.Training = append(report.Training, cell)
	}
	if err := pairedBaselineFailureGate(report, training, heldout); !errors.Is(err, evaluation.ErrGate) {
		t.Fatal("known failed evaluation not preserved", err)
	}
	if err := pairedRepairPositiveGate(report, heldout); err != nil {
		t.Fatal("positive classification control", err)
	}
	copyReport := func() pairedReport {
		out := report
		out.Cases = append([]pairedCell(nil), report.Cases...)
		out.Training = append([]pairedCell(nil), report.Training...)
		out.SharedControls = append([]pairedCell(nil), report.SharedControls...)
		return out
	}
	for _, mutation := range []struct {
		name   string
		change func(*pairedReport)
	}{
		{"false_quality_pass", func(r *pairedReport) { r.GatePassed = true }},
		{"wrong_failure_classification", func(r *pairedReport) {
			for i := range r.Cases {
				if r.Cases[i].Arm == "A" && !r.Cases[i].Matches {
					r.Cases[i].Actual = "answer"
					break
				}
			}
		}},
		{"failed_answer_relabelled_refusal", func(r *pairedReport) {
			for i := range r.Cases {
				if !r.Cases[i].Matches {
					r.Cases[i].Case.Expected = r.Cases[i].Actual
					break
				}
			}
		}},
		{"false_baseline_correctness", func(r *pairedReport) {
			for i := range r.Cases {
				if !r.Cases[i].Matches {
					r.Cases[i].Matches = true
					break
				}
			}
		}},
		{"missing_baseline_cell", func(r *pairedReport) { r.Cases = r.Cases[1:] }},
		{"shared_cell_substitution", func(r *pairedReport) {
			r.Cases[0].Case = r.SharedControls[0].Case
			r.Cases[0].Matches = true
			r.Cases[0].Actual = r.Cases[0].Case.Expected
		}},
		{"duplicate_shared_control", func(r *pairedReport) { r.SharedControls[1] = r.SharedControls[0] }},
		{"changed_shared_question", func(r *pairedReport) { r.SharedControls[0].Case.Question += " changed" }},
		{"changed_shared_contract", func(r *pairedReport) { r.SharedControls[0].Case.Contract += "-changed" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			changed := copyReport()
			mutation.change(&changed)
			if err := pairedBaselineFailureGate(changed, training, heldout); errors.Is(err, evaluation.ErrGate) || err == nil {
				t.Fatal("mutation accepted as exact failed evaluation")
			}
		})
	}
	changed := copyReport()
	for i := range changed.Cases {
		if changed.Cases[i].Arm == "D" {
			changed.Cases[i].Matches = false
			break
		}
	}
	if err := pairedRepairPositiveGate(changed, heldout); err == nil {
		t.Fatal("altered D correctness accepted")
	}
	changed = copyReport()
	for i := range changed.Cases {
		if changed.Cases[i].Arm == "D" {
			changed.Cases[i].Actual = "answer_mismatch"
			break
		}
	}
	if err := pairedRepairPositiveGate(changed, heldout); err == nil {
		t.Fatal("D outcome mismatch accepted")
	}
	changed = copyReport()
	for i := range changed.Cases {
		if changed.Cases[i].Arm == "D" {
			changed.Cases[i].Case = changed.SharedControls[0].Case
			changed.Cases[i].Actual = changed.Cases[i].Case.Expected
			break
		}
	}
	if err := pairedRepairPositiveGate(changed, heldout); err == nil {
		t.Fatal("shared control substituted for required D cell")
	}
	changed = copyReport()
	changed.SharedControls[1] = changed.SharedControls[0]
	if err := pairedRepairPositiveGate(changed, heldout); err == nil {
		t.Fatal("duplicate shared control accepted by positive gate")
	}
}

func pairedIsSharedControl(c pairedCase) bool {
	return c.Contract == "stale-profile" || c.Contract == "sensitive-note" || c.Contract == "wrong-context"
}
func pairedSharedControlGate(cells []pairedCell, heldout []pairedCase) error {
	expected := map[string]pairedCase{}
	for _, c := range heldout {
		if pairedIsSharedControl(c) {
			expected[c.ID] = c
		}
	}
	if len(expected) != 3 || len(cells) != 3 {
		return fmt.Errorf("incomplete shared controls")
	}
	seen := map[string]bool{}
	for _, cell := range cells {
		want, ok := expected[cell.Case.ID]
		if !ok || seen[cell.Case.ID] || cell.Arm != "shared" || readexec.Hash(want) != readexec.Hash(cell.Case) || !cell.Matches || cell.Actual != cell.Case.Expected {
			return fmt.Errorf("shared control changed or duplicated")
		}
		seen[cell.Case.ID] = true
	}
	return nil
}
