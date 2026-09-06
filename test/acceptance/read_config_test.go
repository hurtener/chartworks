package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
)

func TestReadExecutionReferenceExcerpt(t *testing.T) {
	excerpt, err := os.ReadFile("../../examples/chartworks.execution.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(excerpt, &document); err != nil {
		t.Fatal(err)
	}
	// Compose the excerpt with the required Pengui verifier configuration.
	// Load applies its own defaults; serializing all default Values first can
	// introduce unrelated nil slices that the closed document format rejects.
	document["auth"] = json.RawMessage(`{"issuer":"https://issuer.example","jwks_url":"https://issuer.example/keys","audience":"test"}`)
	input, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(key string) (string, bool) {
		return "postgres://synthetic:synthetic@localhost:5434/reference?sslmode=disable", key == "CHARTWORKS_STORE_URL"
	}
	loaded, err := config.Load(bytes.NewReader(input), lookup, config.Overrides{})
	if err != nil {
		t.Fatal("actual configuration decoder rejected execution excerpt", err)
	}
	v := loaded.Values()
	if v.Exec.RowsCeiling != 100000 || v.Exec.Timeout != config.Duration(time.Minute) || v.Server.WriteTimeout != config.Duration(75*time.Second) {
		t.Fatal("reference did not match default contract")
	}
	for _, change := range []func(map[string]any){
		func(exec map[string]any) { exec["rows_ceiling"] = 100001 },
		func(exec map[string]any) { exec["skip_validation"] = true },
	} {
		var execution map[string]any
		if err = json.Unmarshal(document["exec"], &execution); err != nil {
			t.Fatal(err)
		}
		change(execution)
		value, err := json.Marshal(execution)
		if err != nil {
			t.Fatal(err)
		}
		badDocument := map[string]json.RawMessage{"auth": document["auth"], "exec": value, "server": document["server"]}
		bad, err := json.Marshal(badDocument)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = config.Load(bytes.NewReader(bad), lookup, config.Overrides{}); err == nil {
			t.Fatal("invalid execution configuration")
		}
	}
}
