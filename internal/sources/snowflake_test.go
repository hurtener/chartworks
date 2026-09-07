package sources

import (
	"testing"

	bruinsnowflake "github.com/bruin-data/bruin/pkg/snowflake"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestSnowflakeReadMapping(t *testing.T) {
	if _, err := snowflakeArguments([]readexec.Parameter{{Kind: "number", Value: "1.25"}}); err != readexec.ErrUnsupported {
		t.Fatalf("inexact decimal accepted: %v", err)
	}
	capture := &cloudObserverCapture{}
	observer := snowflakeObserver{observer: capture, tag: "cw-read:0123456789abcdef0123456789abcdef"}
	pending := bruinsnowflake.ReadIdentity{RequestID: "01234567-89ab-5def-8123-456789abcdef", QueryTag: "cw:attempt", Account: "account", Database: "database", SessionID: 7}
	ack := pending
	ack.QueryID = "01b-query"
	if err := observer.OnDispatch(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	if err := observer.OnAcknowledged(t.Context(), ack); err != nil {
		t.Fatal(err)
	}
	if len(capture.calls) != 2 || capture.calls[0].Controllable() || !capture.calls[1].Controllable() || !capture.calls[0].Acknowledges(capture.calls[1]) {
		t.Fatalf("snowflake identity transition: %#v", capture.calls)
	}
	for native, want := range map[string]string{"NUMBER(38,3)": "decimal", "BINARY": "binary", "TIMESTAMP_TZ": "temporal", "VARIANT": "structured"} {
		if got := snowflakeCategory(native); got != want {
			t.Fatalf("%s: %s", native, got)
		}
	}
}
