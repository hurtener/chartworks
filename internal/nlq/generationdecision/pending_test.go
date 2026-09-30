package generationdecision

import (
	"strings"
	"testing"
	"time"
)

func TestGenerationPendingPublicOrigin(t *testing.T) {
	p := Problem{Version: Version, Outcome: Clarify, Questions: []string{"Which metric?"}, QueryID: "pending", Resume: "plan", AnswerContext: strings.Repeat("a", 64), ExpiresAt: time.Now().UTC().Add(time.Minute), Choices: []Choice{{Kind: "measure", ID: "revenue"}}}
	got := Public(p)
	if got == nil {
		t.Fatal("valid pending origin rejected")
	}
	got.Choices[0].ID = "changed"
	if p.Choices[0].ID != "revenue" {
		t.Fatal("mutable choice alias")
	}
	for _, edit := range []func(*Problem){
		func(p *Problem) { p.QueryID = "" }, func(p *Problem) { p.AnswerContext = "invalid" }, func(p *Problem) { p.ExpiresAt = time.Time{} },
		func(p *Problem) { p.Choices = []Choice{{Kind: "sql", ID: "arbitrary"}} }, func(p *Problem) { p.Choices = []Choice{{Kind: "measure", ID: "invalid id"}} },
		func(p *Problem) { p.Choices = append(p.Choices, p.Choices[0]) },
	} {
		bad := p
		bad.Choices = append([]Choice(nil), p.Choices...)
		edit(&bad)
		if Public(bad) != nil {
			t.Fatal("invalid pending projection accepted")
		}
	}
	legacy := Problem{Version: Version, Outcome: Clarify, Questions: []string{"Which metric?"}}
	if Public(legacy) == nil {
		t.Fatal("legacy decision broken")
	}
	legacy.Choices = p.Choices
	if Public(legacy) != nil {
		t.Fatal("unbound reviewed choices accepted")
	}
}
