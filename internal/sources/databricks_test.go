package sources

import (
	"math/big"
	"testing"

	bruindatabricks "github.com/bruin-data/bruin/pkg/databricks"
	"github.com/bruin-data/bruin/pkg/query"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestDatabricksReadMapping(t *testing.T) {
	params, err := databricksArguments([]readexec.Parameter{{Kind: "number", Value: "123.450"}, {Kind: "boolean", Value: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	if params[0].(bruindatabricks.ReadParameter).Value.(*big.Rat).RatString() != "2469/20" {
		t.Fatal("decimal changed")
	}
	capture := &cloudObserverCapture{}
	observer := databricksObserver{observer: capture, tag: "cw-read:0123456789abcdef0123456789abcdef"}
	pending := bruindatabricks.ReadIdentity{Workspace: "https://workspace.example", WarehouseID: "warehouse", AttemptTag: "attempt"}
	ack := pending
	ack.StatementID = "01234567-89ab-cdef-8123-456789abcdef"
	if err = observer.OnDispatch(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	if err = observer.OnAcknowledged(t.Context(), ack); err != nil {
		t.Fatal(err)
	}
	if len(capture.calls) != 2 || capture.calls[0].Controllable() || !capture.calls[1].Controllable() || !capture.calls[0].Acknowledges(capture.calls[1]) {
		t.Fatalf("databricks identity transition: %#v", capture.calls)
	}
	field, err := cloudField(query.Column{Name: "amount", DatabaseType: "DECIMAL(38,3)", Precision: 38, Scale: 3, DecimalKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cloudValue(field, query.Column{Scale: 3, DecimalKnown: true}, params[0].(bruindatabricks.ReadParameter).Value)
	if err != nil || string(raw) != "123.450" {
		t.Fatalf("precision: %q %v", raw, err)
	}
}
