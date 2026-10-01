package nlqroute

import (
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"strings"
)

// requestedNetMeaning refuses substituting a gross measure for an explicitly
// requested net concept. Only reviewed display names/aliases participate;
// physical column names and vector similarity never establish business meaning.
func requestedNetMeaning(in RouteRequest, admitted []admittedTopic) *Clarification {
	q := normalizedPhrase(unquotedIntent(in.Question))
	net := false
	phrases := []string{"net revenue", "net sales"}
	if in.Locale == nlq.LanguageSpanish {
		phrases = []string{"ventas netas", "ingresos netos", "importe neto"}
	}
	for _, p := range phrases {
		net = net || containsPhrase(q, p)
	}
	if !net {
		return nil
	}
	selected := explicitMetricRoots(in, admitted)
	names := map[string]bool{}
	chosen := false
	check := func(topic string, ref semantics.Reference, name string, aliases []string) {
		labels := append([]string{name}, aliases...)
		for _, label := range labels {
			for _, word := range strings.Fields(normalizedPhrase(label)) {
				if word == "net" || word == "neto" || word == "neta" || word == "netos" || word == "netas" {
					names[topic+"\x00"+string(ref.Kind)+"\x00"+ref.ID] = true
					chosen = chosen || selected[topic][ref]
				}
			}
		}
	}
	for _, item := range admitted {
		for _, m := range item.publication.Definition.Measures {
			check(item.id, semantics.Reference{Kind: semantics.KindMeasure, ID: m.ID}, m.Name, m.Aliases)
		}
		for _, k := range item.publication.Definition.KPIs {
			check(item.id, semantics.Reference{Kind: semantics.KindKPI, ID: k.ID}, k.Name, k.Aliases)
		}
	}
	if chosen {
		return nil
	}
	reason := "reviewed_metric_meaning_required"
	prompt := "Choose a reviewed net metric and its business population; a gross metric cannot substitute for net."
	if len(names) > 1 {
		reason = "ambiguous_metric_meaning"
		prompt = "Choose the reviewed net definition, including whether refunds follow the order cohort or refund activity."
	}
	if len(names) == 1 && len(selected) == 0 {
		return nil
	}
	if in.Locale == nlq.LanguageSpanish {
		prompt = "Elige una métrica neta revisada y su población; una métrica bruta no sustituye el neto."
		if reason == "ambiguous_metric_meaning" {
			prompt = "Elige la definición neta revisada y si las devoluciones corresponden a la cohorte de pedidos o a la actividad de devoluciones."
		}
	}
	return &Clarification{Reason: reason, Outcome: semantics.ClarificationConflicting, Prompt: prompt}
}
