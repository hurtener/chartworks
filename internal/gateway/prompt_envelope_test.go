package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func envelopeSchema(t *testing.T) *Schema {
	t.Helper()
	s, err := NewSchema("candidate", []byte(`{"type":"object","additionalProperties":false,"properties":{"sql":{"type":"string"}},"required":["sql"]}`))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSQLRecoveryEnvelopeCountsSchemaEscapesAndOutput(t *testing.T) {
	s := envelopeSchema(t)
	e, err := NewPromptEnvelope("route", "recorded", "system", "", s, 8192, 0, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	prompt := "último mes \" <tag>\n"
	u, ok, err := e.Measure(prompt)
	if err != nil || !ok || u.RequestBytes <= len(prompt)+len("system")+len(s.Document()) || u.InputUpperBound != u.RequestBytes+1024 || u.OutputReserve != 256 || u.ContextTokens != 0 {
		t.Fatal("envelope accounting", u, ok, err)
	}
	window := u.InputUpperBound + u.OutputReserve
	exact, err := NewPromptEnvelope("route", "recorded", "system", "", s, 8192, window, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	at, ok, err := exact.Measure(prompt)
	if err != nil || !ok || at.ContextTokens != window {
		t.Fatal("exact window rejected", err)
	}
	narrow, err := NewPromptEnvelope("route", "recorded", "system", "", s, 8192, window-1, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err = narrow.Measure(prompt)
	if err != nil || ok {
		t.Fatal("one-token-over window accepted", err)
	}
	bytesOnly, err := NewPromptEnvelope("route", "recorded", "system", "", s, u.RequestBytes-1, 0, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err = bytesOnly.Measure(prompt)
	if err != nil || ok {
		t.Fatal("normalized JSON escaped byte cap", err)
	}
}

func TestSQLRecoveryEnvelopeIsDetachedAndRedacted(t *testing.T) {
	e, err := NewPromptEnvelope("route", "model", "private-system-canary", "", envelopeSchema(t), 8192, 0, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := e.Measure("private-question-canary")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(u)
	for _, v := range []string{string(raw), fmt.Sprint(e), fmt.Sprintf("%#v", e), e.LogValue().String()} {
		if strings.Contains(v, "private-system-canary") || strings.Contains(v, "private-question-canary") {
			t.Fatal("envelope exposed text")
		}
	}
	for name, changed := range map[string]PromptEnvelope{
		"same":   e,
		"model":  func() PromptEnvelope { v := e; v.model = "different"; return v }(),
		"policy": func() PromptEnvelope { v := e; v.maxBytes--; return v }(),
		"route":  func() PromptEnvelope { v := e; v.provider = "different"; return v }(),
	} {
		got, _, err := changed.Measure("private-question-canary")
		if err != nil {
			t.Fatal(err)
		}
		if (name == "same") != (got.Digest == u.Digest) {
			t.Fatal("digest omitted effective identity", name)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, ok, err := e.Measure("private-question-canary")
			if err != nil || !ok || got.Digest != u.Digest {
				t.Error("shared envelope changed")
			}
		}()
	}
	wg.Wait()
}

func TestSQLRecoveryEnvelopeRejectsInvalidInput(t *testing.T) {
	e, err := NewPromptEnvelope("route", "model", "system", "", envelopeSchema(t), 8192, 0, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"", "bad\x00value", string([]byte{0xff}), strings.Repeat("x", (4<<20)+1)} {
		if _, _, err := e.Measure(input); err == nil {
			t.Fatal("invalid prompt accepted")
		}
	}
	if _, _, err := (PromptEnvelope{}).Measure("question"); err == nil {
		t.Fatal("zero envelope accepted")
	}
}

func TestSQLRecoveryRuntimeRoleSliceIsImmutable(t *testing.T) {
	cfg := RuntimeConfig{Model: "baseline", Models: []RuntimeModel{{Role: "sqlgen", Model: "reviewed"}}}
	cfg.Digest = ConfigurationDigest(cfg)
	ctx, err := WithRuntimeConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Models[0].Model = "unreviewed"
	model, _, digest := ApplyRuntimeConfig(ctx, "sqlgen", "configured", "system")
	if model != "reviewed" || digest != cfg.Digest {
		t.Fatal("runtime slice changed between fitting and dispatch")
	}
}
