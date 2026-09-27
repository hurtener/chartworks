package conceptchoice

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func candidate(n int) string { return fmt.Sprintf("%064x", n) }
func TestSQLRecoveryConceptChoiceGroundedSpans(t *testing.T) {
	for _, q := range []string{"Show retained income and volume", "Mostrá ingresos retenidos y volumen"} {
		a, b := candidate(1), candidate(2)
		quoteA, quoteB := "retained income", "volume"
		if strings.HasPrefix(q, "Mostrá") {
			quoteA, quoteB = "ingresos retenidos", "volumen"
		}
		proposal := Proposal{Decision: "select", Selected: []Selection{{b, quoteB}, {a, quoteA}}}
		p, err := Resolve(context.Background(), q, []string{a, b}, proposal)
		if err != nil || p.Version != Version || len(p.Selected) != 2 || p.Selected[0].ID != a {
			t.Fatal("proof", err)
		}
		if q[p.Selected[0].Start:p.Selected[0].End] != quoteA || Verify(context.Background(), q, []string{a, b}, p) != nil {
			t.Fatal("span or replay")
		}
		clone := p.Clone()
		clone.Selected[0].ID = b
		if p.Selected[0].ID != a {
			t.Fatal("shared proof memory")
		}
	}
}
func TestSQLRecoveryConceptChoiceRejectsUngroundedAndAmbiguous(t *testing.T) {
	a, b := candidate(1), candidate(2)
	q := "income and income for [redacted answer]"
	for _, s := range []Selection{{a, "income"}, {a, "absent"}, {a, "[redacted answer]"}, {a, "answer"}, {a, "redacted"}, {candidate(3), "for"}, {a, " for"}, {a, ""}} {
		if _, err := Resolve(context.Background(), q, []string{a, b}, Proposal{Decision: "select", Selected: []Selection{s}}); !errors.Is(err, ErrChoice) {
			t.Fatal("invalid grounding accepted", s, err)
		}
	}
	for _, selected := range [][]Selection{{{a, "profit"}, {b, "gross profit"}}, {{a, "gross profit"}, {a, "amount"}}} {
		if _, err := Resolve(context.Background(), "gross profit amount", []string{a, b}, Proposal{Decision: "select", Selected: selected}); !errors.Is(err, ErrChoice) {
			t.Fatal("overlap/duplicate ID", err)
		}
	}
}
func TestSQLRecoveryConceptChoiceClosedOutcomes(t *testing.T) {
	a, b := candidate(1), candidate(2)
	for _, proposal := range []Proposal{{Decision: "clarify", Alternatives: []string{b, a}}, {Decision: "no_match"}} {
		p, err := Resolve(context.Background(), "Which income?", []string{a, b}, proposal)
		if err != nil || len(p.Selected) != 0 || Verify(context.Background(), "Which income?", []string{a, b}, p) != nil {
			t.Fatal("closed disposition", err)
		}
	}
	for _, proposal := range []Proposal{{Decision: "ready"}, {Decision: "select"}, {Decision: "clarify", Alternatives: []string{a}}, {Decision: "clarify", Alternatives: []string{a, a}}, {Decision: "select", Selected: []Selection{{a, "income"}}, Alternatives: []string{b}}, {Decision: "no_match", Selected: []Selection{{a, "income"}}}} {
		if _, err := Resolve(context.Background(), "income", []string{a, b}, proposal); !errors.Is(err, ErrChoice) {
			t.Fatal("contradictory decision", err)
		}
	}
}
func TestSQLRecoveryConceptChoiceTamperAndPolicy(t *testing.T) {
	a, b := candidate(1), candidate(2)
	q := "show income"
	p, err := Resolve(context.Background(), q, []string{a, b}, Proposal{Decision: "select", Selected: []Selection{{a, "income"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Proof){func(p *Proof) { p.Version = "other" }, func(p *Proof) { p.Digest = candidate(3) }, func(p *Proof) { p.Selected[0].Start = -1 }, func(p *Proof) { p.Selected[0].End = len(q) + 1 }, func(p *Proof) { p.Selected[0].ID = b }, func(p *Proof) { p.Candidates = candidate(3) }, func(p *Proof) { p.Decision = "no_match" }} {
		bad := p.Clone()
		change(&bad)
		if Verify(context.Background(), q, []string{a, b}, bad) == nil {
			t.Fatal("tampered proof")
		}
	}
	if Verify(context.Background(), q+" now", []string{a, b}, p) == nil || Verify(context.Background(), q, []string{b, a}, p) == nil {
		t.Fatal("changed input reused proof")
	}
}
func TestSQLRecoveryConceptChoiceBoundsCancellation(t *testing.T) {
	for _, ids := range [][]string{nil, {"not-a-hash"}, {candidate(1), candidate(1)}} {
		if _, err := Resolve(context.Background(), "income", ids, Proposal{Decision: "no_match"}); !errors.Is(err, ErrChoice) {
			t.Fatal("bad candidates", err)
		}
	}
	for _, q := range []string{"", string([]byte{0xff}), "bad\x00question", strings.Repeat("x", MaxQuestionBytes+1)} {
		if _, err := Resolve(context.Background(), q, []string{candidate(1)}, Proposal{Decision: "no_match"}); !errors.Is(err, ErrChoice) {
			t.Fatal("bad question", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Resolve(ctx, "income", []string{candidate(1)}, Proposal{Decision: "no_match"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Resolve(nil, "income", []string{candidate(1)}, Proposal{Decision: "no_match"}); !errors.Is(err, ErrChoice) {
		t.Fatal(err)
	}
}
func TestSQLRecoveryConceptChoiceConcurrentDetached(t *testing.T) {
	ids := []string{candidate(1), candidate(2)}
	proposal := Proposal{Decision: "clarify", Alternatives: []string{ids[1], ids[0]}}
	before := append([]string(nil), proposal.Alternatives...)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := Resolve(context.Background(), "income", ids, proposal)
			if err != nil || Verify(context.Background(), "income", ids, p) != nil {
				t.Error("concurrent proof", err)
			}
			p.Alternatives[0] = "changed"
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(before, proposal.Alternatives) {
		t.Fatal("mutated model input")
	}
}
func FuzzConceptChoiceReplay(f *testing.F) {
	for _, s := range []string{"profit", "ingresos", "[redacted answer]", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, q string) {
		if len(q) > MaxQuestionBytes {
			return
		}
		p, err := Resolve(context.Background(), q, []string{candidate(1)}, Proposal{Decision: "select", Selected: []Selection{{candidate(1), q}}})
		if err == nil && Verify(context.Background(), q, []string{candidate(1)}, p) != nil {
			t.Fatal("own proof not replayable")
		}
	})
}
