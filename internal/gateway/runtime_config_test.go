package gateway

import (
	"context"
	"testing"
)

func TestRuntimeConfigIsCanonicalAndChangesEffectiveInput(t *testing.T) {
	cfg := RuntimeConfig{Model: "candidate-model", SystemInstruction: "reviewed candidate prompt", AttemptCostUSD: 0.02}
	cfg.Digest = ConfigurationDigest(cfg)
	ctx, err := WithRuntimeConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	model, system, digest := ApplyRuntimeConfig(ctx, "sql_generation", "baseline-model", "base prompt")
	if model != cfg.Model || system != "base prompt\n"+cfg.SystemInstruction || digest != cfg.Digest {
		t.Fatal(model, system, digest)
	}
	tampered := cfg
	tampered.SystemInstruction = "different"
	if _, err := WithRuntimeConfig(context.Background(), tampered); err == nil {
		t.Fatal("digest did not bind effective prompt")
	}
}
