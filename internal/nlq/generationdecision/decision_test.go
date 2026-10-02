package generationdecision

import (
	"strings"
	"sync"
	"testing"
)

func TestSQLRecoveryDecisionClosedUnion(t *testing.T) {
	for _, c := range []struct {
		out     string
		qs      []string
		sql     bool
		p, a, b int
		ok      bool
	}{
		{Ready, nil, true, 0, 0, 0, true}, {Ready, nil, true, 1, 2, 3, true},
		{Clarify, []string{"Which metric?"}, false, 0, 0, 0, true}, {Insufficient, []string{"¿Qué relación está revisada?"}, false, 0, 0, 0, true},
		{"", nil, true, 0, 0, 0, false}, {"run", nil, true, 0, 0, 0, false}, {Ready, nil, false, 0, 0, 0, false}, {Ready, []string{"Which metric?"}, true, 0, 0, 0, false},
		{Clarify, nil, false, 0, 0, 0, false}, {Clarify, []string{"Which metric?"}, true, 0, 0, 0, false}, {Clarify, []string{"Which metric?"}, false, 1, 0, 0, false},
		{Insufficient, []string{"Which metric?"}, false, 0, 1, 0, false}, {Insufficient, []string{"Which metric?"}, false, 0, 0, 1, false}, {Ready, nil, true, -1, 0, 0, false},
	} {
		if err := Check(c.out, c.qs, c.sql, c.p, c.a, c.b); (err == nil) != c.ok {
			t.Fatalf("outcome=%q expected valid=%v", c.out, c.ok)
		}
	}
}
func TestSQLRecoveryDecisionBoundsAndPublicProjection(t *testing.T) {
	for _, qs := range [][]string{{}, {""}, {" padded"}, {"line\nline"}, {"x\x00y"}, {string([]byte{0xff})}, {strings.Repeat("ñ", 257)}, make([]string, 9)} {
		if Public(Problem{Version: Version, Outcome: Clarify, Questions: qs}) != nil {
			t.Fatal("invalid question projection")
		}
	}
	p := Problem{Version: Version, Outcome: Clarify, Questions: []string{"Qué importe?"}}
	out := Public(p)
	if out == nil || !MatchesCode(out, ErrorCode(Clarify)) || MatchesCode(out, ErrorCode(Insufficient)) {
		t.Fatal("incorrect discriminator")
	}
	out.Questions[0] = "mutated"
	if p.Questions[0] != "Qué importe?" {
		t.Fatal("question slice alias")
	}
	for _, bad := range []Problem{{Version: "future", Outcome: Clarify, Questions: p.Questions}, {Version: Version, Outcome: Ready, Questions: p.Questions}} {
		if Public(bad) != nil {
			t.Fatal("unknown public decision")
		}
	}
	if ErrorCode("private-canary") != "" || MatchesCode(nil, "foo") {
		t.Fatal("untrusted code generated")
	}
}
func TestSQLRecoveryDecisionConcurrent(t *testing.T) {
	p := Problem{Version: Version, Outcome: Insufficient, Questions: []string{"Which reviewed source?"}}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := Public(p)
			if q == nil || !MatchesCode(q, ErrorCode(Insufficient)) {
				t.Error("unstable decision")
			}
			q.Questions[0] = "caller-owned"
		}()
	}
	wg.Wait()
	if p.Questions[0] != "Which reviewed source?" {
		t.Fatal("shared input mutated")
	}
}
func FuzzGenerationDecision(f *testing.F) {
	for _, s := range []string{"ready", "clarify", "insufficient_context", "", "\x00"} {
		f.Add(s, "Which metric?")
	}
	f.Fuzz(func(t *testing.T, kind, question string) {
		if len(question) > 1024 || len(kind) > 128 {
			return
		}
		p := Problem{Version: Version, Outcome: kind, Questions: []string{question}}
		q := Public(p)
		if q != nil {
			if Check(kind, q.Questions, false, 0, 0, 0) != nil || ErrorCode(kind) == "" {
				t.Fatal("public invalid union")
			}
		}
	})
}
