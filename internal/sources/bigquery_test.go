package sources

import (
	"context"
	"math/big"
	"testing"

	bruinbigquery "github.com/bruin-data/bruin/pkg/bigquery"
	"github.com/bruin-data/bruin/pkg/query"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

type cloudObserverCapture struct {
	calls    []readexec.RemoteQuery
	accepted []bool
}

func (o *cloudObserverCapture) Dispatch(_ context.Context, q readexec.RemoteQuery, accepted bool) error {
	o.calls = append(o.calls, q)
	o.accepted = append(o.accepted, accepted)
	return nil
}
func (*cloudObserverCapture) Check(context.Context) error { return nil }

func TestBigQueryReadMapping(t *testing.T) {
	params, err := bigQueryArguments([]readexec.Parameter{{Kind: "integer", Value: "42"}, {Kind: "number", Value: "123.450"}, {Kind: "null"}})
	if err != nil || len(params) != 3 {
		t.Fatalf("arguments: %v %#v", err, params)
	}
	decimal := params[1].(bruinbigquery.ReadParameter).Value.(*big.Rat)
	if decimal.RatString() != "2469/20" {
		t.Fatalf("decimal changed: %s", decimal)
	}
	capture := &cloudObserverCapture{}
	observer := bigQueryObserver{observer: capture, tag: "cw-read:0123456789abcdef0123456789abcdef"}
	identity := bruinbigquery.ReadIdentity{ProjectID: "project", Location: "us", JobID: "bruin_read_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err = observer.OnDispatch(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	if err = observer.OnAcknowledged(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	if len(capture.calls) != 2 || capture.accepted[0] || !capture.accepted[1] || !capture.calls[1].Valid() || !capture.calls[0].Acknowledges(capture.calls[1]) {
		t.Fatalf("journal mapping: %#v %#v", capture.calls, capture.accepted)
	}
	field, err := cloudField(query.Column{Name: "amount", DatabaseType: "BIGNUMERIC", Precision: 38, Scale: 3, DecimalKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cloudValue(field, query.Column{Scale: 3, DecimalKnown: true}, decimal)
	if err != nil || string(raw) != "123.450" {
		t.Fatalf("precision: %q %v", raw, err)
	}
}
