package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func good() Values {
	v := Defaults()
	v.Auth.Issuer = "https://issuer.example"
	v.Auth.JWKSURL = "https://issuer.example/keys"
	v.Auth.Audience = "test"
	return v
}
func TestConfigurationRules(t *testing.T) {
	tests := []func(*Values){
		func(v *Values) { v.Server.Listen = "bad" }, func(v *Values) { v.Server.Listen = "127.0.0.1:99999" }, func(v *Values) { v.Server.Listen = "localhost:8080" }, func(v *Values) { v.Server.ReadTimeout = 0 }, func(v *Values) { v.Server.MaxHeaderBytes = 0 },
		func(v *Values) { v.Auth.Issuer = "https://user:secret@issuer.example" }, func(v *Values) { v.Auth.Audience = "" }, func(v *Values) { v.Auth.Algorithms = nil }, func(v *Values) { v.Auth.Algorithms = []string{"RS256", "RS256"} }, func(v *Values) { v.Auth.RefreshInterval = v.Auth.JWKSMaxStale }, func(v *Values) { v.Auth.ClockSkew = Duration(2 * time.Minute) },
		func(v *Values) { v.Store.MaxConns = 101 }, func(v *Values) { v.Store.MigrationPolicy = "down" }, func(v *Values) { v.Telemetry.LogFormat = "unknown" }, func(v *Values) { v.Gateway.MaxAttemptsPerCall = 10 },
		func(v *Values) { v.Gateway.Bifrost.Providers = []Provider{{Name: "local"}} }, func(v *Values) { v.Gateway.Bifrost.Providers = []Provider{{Name: "openrouter", APIKey: "secret"}} }, func(v *Values) {
			v.Gateway.Bifrost.Providers = []Provider{{Name: "openrouter", APIKey: "env:KEY", BaseURL: "http://remote.example"}}
		},
		func(v *Values) { v.Gateway.Roles = map[string]Role{"unknown": {}} }, func(v *Values) { v.Gateway.Roles = map[string]Role{"sqlgen": {Model: "test", Provider: "missing"}} },
	}
	for i, f := range tests {
		v := good()
		f(&v)
		if validate(v) == nil {
			t.Errorf("invalid case %d accepted", i)
		}
	}
	v := good()
	v.Gateway.Bifrost.Providers = []Provider{{Name: "remote", Type: "openrouter", APIKey: "env:KEY", BaseURL: "https://provider.example"}}
	v.Gateway.Bifrost.Providers = append(v.Gateway.Bifrost.Providers, Provider{Name: "reranker", Type: "cohere", APIKey: "env:KEY"})
	for _, name := range []string{"embedding", "enhance", "sqlgen", "sqlfix", "clarify", "pipeline_draft", "profile_summary", "rerank", "narrative", "visual_rank"} {
		v.Gateway.Roles[name] = Role{Provider: "remote", Model: "model", Timeout: Duration(time.Second), Dimensions: 1024, MaxBatchItems: 64, MaxBatchBytes: 1000, MaxCandidates: 64, MaxTokens: 100, OnFailure: ""}
	}
	rr := v.Gateway.Roles["rerank"]
	rr.Provider = "reranker"
	v.Gateway.Roles["rerank"] = rr
	if e := validate(v); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"duration", "embedding", "candidates", "failure", "tokens"} {
		bad := v
		bad.Gateway.Roles = map[string]Role{}
		for k, r := range v.Gateway.Roles {
			bad.Gateway.Roles[k] = r
		}
		r := bad.Gateway.Roles["embedding"]
		switch field {
		case "duration":
			r.Timeout = 0
		case "embedding":
			r.Dimensions = 0
		case "candidates":
			r = bad.Gateway.Roles["rerank"]
			r.MaxCandidates = 0
			bad.Gateway.Roles["rerank"] = r
		case "failure":
			r.OnFailure = "guess"
		case "tokens":
			r.MaxTokens = -1
		}
		if field != "candidates" {
			bad.Gateway.Roles["embedding"] = r
		}
		if validate(bad) == nil {
			t.Error("invalid role setting accepted")
		}
	}
	data, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	cfg, e := Load(bytes.NewReader(data), func(string) (string, bool) { return "fixture", true }, Overrides{})
	if e != nil {
		t.Fatal(e)
	}
	copy := cfg.Values()
	copy.Gateway.Bifrost.Providers[0].Name = "changed"
	delete(copy.Gateway.Roles, "embedding")
	if cfg.Values().Gateway.Bifrost.Providers[0].Name != "remote" || len(cfg.Values().Gateway.Roles) != 10 {
		t.Fatal("shared configuration mutated")
	}
}
func TestDecodeEdges(t *testing.T) {
	for _, s := range []string{"", "env:", "env:a", "env:" + strings.Repeat("X", 129), "env:1BAD", "secret"} {
		if _, e := reference(s); e == nil {
			t.Error("bad secret reference accepted")
		}
	}
	if _, e := reference("env:GOOD_1"); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"", strings.Repeat("x", 129), "field-unsupported"} {
		if safeField(s) != "document" {
			t.Error("unsafe path")
		}
	}
	if safeField("store.dsn") != "store.dsn" {
		t.Fatal("known path lost")
	}
	lookup := func(string) (string, bool) { return "fixture", true }
	if _, e := Load(nil, lookup, Overrides{}); e == nil {
		t.Fatal("nil reader")
	}
	for _, data := range []string{`{"server":{"mystery":"secret"}}`, `{"server":{"listen":3}}`, `{"gateway":{"roles":{"sqlgen":{"bogus":1}}}}`, `{"auth":{"issuer":"env:bad"}}`, `{"auth":{"issuer":"env:MISSING"}}`, `{"auth":true}`, strings.Repeat(`{"a":`, 34) + `1` + strings.Repeat(`}`, 34)} {
		_, e := Load(strings.NewReader(data), func(string) (string, bool) { return "", false }, Overrides{})
		if e == nil {
			t.Errorf("bad document accepted")
		}
		if strings.Contains(e.Error(), "secret") {
			t.Fatal("error echoed value")
		}
	}
	if e := checkShape([]byte(`[]`), reflect.TypeOf(Values{}), "document"); e != nil {
		t.Fatal(e)
	}
	var d Duration
	if d.UnmarshalJSON([]byte(`"5s"`)) != nil || time.Duration(d) != 5*time.Second {
		t.Fatal("duration parse")
	}
	if d.UnmarshalJSON([]byte(`false`)) == nil || d.UnmarshalJSON([]byte(`"bad"`)) == nil {
		t.Fatal("bad duration accepted")
	}
	var output bytes.Buffer
	if WriteDefaults(&output) != nil || !json.Valid(output.Bytes()) {
		t.Fatal("defaults output")
	}
	if e := WriteDefaults(errorWriter{}); e == nil || strings.Contains(e.Error(), "CANARY") {
		t.Fatal("output error leak")
	}
	if _, e := Load(strings.NewReader(`{}`), nil, Overrides{}); e == nil {
		t.Fatal("nil lookup")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("CANARY") }
func FuzzLoad(f *testing.F) {
	for _, s := range []string{`{}`, `{"auth":null}`, `{"server":{"listen":"127.0.0.1:0"}}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<20 {
			return
		}
		c, e := Load(strings.NewReader(s), func(string) (string, bool) { return "secret-canary", true }, Overrides{})
		if e != nil && strings.Contains(e.Error(), "secret-canary") {
			t.Fatal("secret leaked")
		}
		if e == nil {
			if _, err := json.Marshal(c); err != nil {
				t.Fatal(err)
			}
		}
	})
}

var _ io.Writer = errorWriter{}
