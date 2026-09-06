// Package config loads a bounded, value-redacting, immutable configuration snapshot.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const maxDocumentBytes = 1 << 20

// Error deliberately carries a field and fixed rule, never the rejected value.
type Error struct{ Field, Rule string }

func (e *Error) Error() string         { return "configuration " + e.Field + ": " + e.Rule }
func invalid(field, rule string) error { return &Error{Field: field, Rule: rule} }

// Duration is a JSON duration string with explicit units.
type Duration time.Duration

// UnmarshalJSON accepts explicit duration strings only.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) != nil {
		return invalid("duration", "expected duration string")
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return invalid("duration", "invalid duration")
	}
	*d = Duration(v)
	return nil
}

// MarshalJSON preserves explicit duration units.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

// Server contains only implemented transport controls.
type Server struct {
	Listen            string   `json:"listen"`
	ReadHeaderTimeout Duration `json:"read_header_timeout"`
	ReadTimeout       Duration `json:"read_timeout"`
	WriteTimeout      Duration `json:"write_timeout"`
	IdleTimeout       Duration `json:"idle_timeout"`
	ShutdownGrace     Duration `json:"shutdown_grace"`
	MaxBodyBytes      int64    `json:"max_body_bytes"`
	MaxHeaderBytes    int      `json:"max_header_bytes"`
}

// Auth configures the Pengui verifier and its shared key cache, never a local issuer.
type Auth struct {
	Issuer           string    `json:"issuer"`
	JWKSURL          string    `json:"jwks_url"`
	Audience         string    `json:"audience,omitempty"`
	Audiences        Audiences `json:"audiences,omitempty"`
	MaxTokenBytes    int       `json:"max_token_bytes"`
	MaxClaimBytes    int       `json:"max_claim_bytes"`
	MaxScopes        int       `json:"max_scopes"`
	MaxScopeBytes    int       `json:"max_scope_bytes"`
	Algorithms       []string  `json:"algorithms"`
	JWKSMaxStale     Duration  `json:"jwks_max_stale"`
	RefreshInterval  Duration  `json:"refresh_interval"`
	RequestTimeout   Duration  `json:"request_timeout"`
	ClockSkew        Duration  `json:"clock_skew"`
	MaxTokenLifetime Duration  `json:"max_token_lifetime"`
}

// Store contains a reference to a DSN, never a literal credential.
type Store struct {
	DSN                string   `json:"dsn"`
	MaxConns           int32    `json:"max_conns"`
	ConnectTimeout     Duration `json:"connect_timeout"`
	TransactionTimeout Duration `json:"transaction_timeout"`
	MigrationPolicy    string   `json:"migration_policy"`
}

// Telemetry configures the implemented logger and in-process metrics exporter.
type Telemetry struct {
	LogFormat string `json:"log_format"`
	Metrics   bool   `json:"metrics"`
	OTel      bool   `json:"otel"`
}

// Features are explicit enablement, not claims that later phases are implemented.
type Features struct {
	Gateway   bool `json:"gateway"`
	MCP       bool `json:"mcp"`
	Reporting bool `json:"reporting"`
	Renderer  bool `json:"renderer"`
}

// Provider is a non-secret reference to remote Bifrost configuration.
type Provider struct {
	Type    string `json:"type,omitempty"`
	Name    string `json:"name"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
}

// Role is the shared remote-provider configuration contract. No inference occurs here.
type Role struct {
	ModelRevision string   `json:"model_revision,omitempty"`
	Enabled       bool     `json:"enabled,omitempty"`
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	Dimensions    int      `json:"dimensions,omitempty"`
	Timeout       Duration `json:"timeout"`
	MaxTokens     int      `json:"max_tokens,omitempty"`
	MaxBatchItems int      `json:"max_batch_items,omitempty"`
	MaxBatchBytes int      `json:"max_batch_bytes,omitempty"`
	MaxCandidates int      `json:"max_candidates,omitempty"`
	OnFailure     string   `json:"on_failure,omitempty"`
}

// Gateway preserves the Bifrost-only production contract for phase 05.
type Gateway struct {
	Limits  GatewayLimits `json:"limits"`
	Driver  string        `json:"driver"`
	Bifrost struct {
		Providers []Provider `json:"providers"`
	} `json:"bifrost"`
	Roles              map[string]Role `json:"roles"`
	MaxAttemptsPerCall int             `json:"max_attempts_per_call"`
}

// Values is a detached serializable configuration, containing secret references only.
type Values struct {
	Uploads   Uploads        `json:"uploads"`
	Profiling Profiling      `json:"profiling"`
	Sources   Sources        `json:"sources"`
	Exec      ReadValidation `json:"exec"`
	Jobs      Jobs           `json:"jobs"`
	Server    Server         `json:"server"`
	Auth      Auth           `json:"auth"`
	Store     Store          `json:"store"`
	Telemetry Telemetry      `json:"telemetry"`
	Features  Features       `json:"features"`
	Gateway   Gateway        `json:"gateway"`
}

// Config is immutable after Load. Its resolved credential has no printable projection.
type Config struct {
	values Values
	dsn    string
}

func (c Config) String() string { return "configuration(redacted)" }

// GoString prevents detailed formatting from printing a resolved credential.
func (c Config) GoString() string { return c.String() }

// MarshalJSON emits detached values and secret references, never credential bytes.
func (c Config) MarshalJSON() ([]byte, error) { return json.Marshal(c.values) }

// Values returns a deep copy, so consumers cannot race by mutating the live snapshot.
func (c Config) Values() Values {
	v := c.values
	v.Sources = c.values.Sources.Clone()
	v.Uploads = c.values.Uploads.Clone()
	v.Profiling = c.values.Profiling.Clone()
	v.Jobs.Credentials = append([]BrokerCredential{}, v.Jobs.Credentials...)
	v.Auth.Algorithms = append([]string(nil), v.Auth.Algorithms...)
	v.Gateway.Bifrost.Providers = append([]Provider{}, v.Gateway.Bifrost.Providers...)
	v.Gateway.Roles = make(map[string]Role, len(c.values.Gateway.Roles))
	for k, r := range c.values.Gateway.Roles {
		v.Gateway.Roles[k] = r
	}
	return v
}

// StoreDSN is for connection construction only. Never log its return value.
func (c Config) StoreDSN() string { return c.dsn }

// Defaults is also the source for config-check --defaults and the reference document.
func Defaults() Values {
	v := Values{
		Uploads:   DefaultUploads(),
		Profiling: DefaultProfiling(),
		Sources:   DefaultSources(),
		Exec:      DefaultReadValidation(),
		Server:    Server{Listen: "127.0.0.1:8080", ReadHeaderTimeout: Duration(5 * time.Second), ReadTimeout: Duration(15 * time.Second), WriteTimeout: Duration(75 * time.Second), IdleTimeout: Duration(time.Minute), ShutdownGrace: Duration(10 * time.Second), MaxBodyBytes: 10 << 20, MaxHeaderBytes: 32 << 10},
		Auth:      Auth{MaxTokenBytes: 32768, MaxClaimBytes: 24576, MaxScopes: 32, MaxScopeBytes: 4096, Algorithms: []string{"RS256", "ES256"}, JWKSMaxStale: Duration(5 * time.Minute), RefreshInterval: Duration(time.Minute), RequestTimeout: Duration(3 * time.Second), ClockSkew: Duration(30 * time.Second), MaxTokenLifetime: Duration(15 * time.Minute)},
		Store:     Store{DSN: "env:CHARTWORKS_STORE_URL", MaxConns: 10, ConnectTimeout: Duration(5 * time.Second), TransactionTimeout: Duration(5 * time.Second), MigrationPolicy: "apply"},
		Jobs:      DefaultJobs(),
		Telemetry: Telemetry{LogFormat: "json", Metrics: true},
		Gateway:   Gateway{Limits: DefaultGatewayLimits(), Driver: "bifrost", MaxAttemptsPerCall: 2, Roles: map[string]Role{}},
	}
	v.Gateway.Bifrost.Providers = []Provider{}
	v.Jobs.Credentials = []BrokerCredential{}
	return v
}

// Overrides are explicit CLI overrides, applied after defaults, file, and env references.
type Overrides struct{ Listen string }

// Load accepts JSON only, rejects duplicate/null/unknown fields, and resolves explicit env references.
func Load(r io.Reader, lookup func(string) (string, bool), override Overrides) (Config, error) {
	if r == nil || lookup == nil {
		return Config{}, invalid("document", "reader and environment lookup required")
	}
	b, err := io.ReadAll(io.LimitReader(r, maxDocumentBytes+1))
	if err != nil || len(b) > maxDocumentBytes {
		return Config{}, invalid("document", "unreadable or too large")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if err = checkJSON(dec, 0); err != nil {
		return Config{}, err
	}
	if _, err = dec.Token(); !errors.Is(err, io.EOF) {
		return Config{}, invalid("document", "exactly one object required")
	}
	if err = checkShape(b, reflect.TypeOf(Values{}), "document"); err != nil {
		return Config{}, err
	}
	v := Defaults()
	dec = json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&v); err != nil {
		var field *Error
		if errors.As(err, &field) {
			return Config{}, field
		}
		var typ *json.UnmarshalTypeError
		if errors.As(err, &typ) {
			return Config{}, invalid(safeField(typ.Field), "wrong type")
		}
		// No decoder text is returned: it may contain arbitrary input or a credential.
		return Config{}, invalid("document", "unknown or retired field, or invalid JSON")
	}
	resolve := func(field string, value *string) error {
		if !strings.HasPrefix(*value, "env:") {
			return nil
		}
		name, e := reference(*value)
		if e != nil {
			return invalid(field, "invalid environment reference")
		}
		x, ok := lookup(name)
		if !ok || strings.TrimSpace(x) == "" {
			return invalid(field, "missing environment value")
		}
		*value = x
		return nil
	}
	for _, item := range []struct {
		name  string
		value *string
	}{{"server.listen", &v.Server.Listen}, {"auth.issuer", &v.Auth.Issuer}, {"auth.jwks_url", &v.Auth.JWKSURL}, {"auth.audience", &v.Auth.Audience}, {"auth.audiences.http", &v.Auth.Audiences.HTTP}, {"auth.audiences.mcp", &v.Auth.Audiences.MCP}, {"auth.audiences.jobs", &v.Auth.Audiences.Jobs}, {"jobs.broker_url", &v.Jobs.BrokerURL}} {
		if err = resolve(item.name, item.value); err != nil {
			return Config{}, err
		}
	}
	if override.Listen != "" {
		v.Server.Listen = override.Listen
	}
	if err = validate(v); err != nil {
		return Config{}, err
	}
	name, err := reference(v.Store.DSN)
	if err != nil {
		return Config{}, invalid("store.dsn", "must be an env:NAME reference")
	}
	dsn, ok := lookup(name)
	if !ok || strings.TrimSpace(dsn) == "" {
		return Config{}, invalid("store.dsn", "missing environment value")
	}
	return Config{values: v, dsn: dsn}, nil
}
func reference(s string) (string, error) {
	if !strings.HasPrefix(s, "env:") {
		return "", invalid("secret", "environment reference required")
	}
	n := strings.TrimPrefix(s, "env:")
	if len(n) == 0 || len(n) > 128 {
		return "", invalid("secret", "invalid reference")
	}
	for i, c := range n {
		if (c >= 'A' && c <= 'Z') || c == '_' || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return "", invalid("secret", "invalid reference")
	}
	return n, nil
}
func safeField(s string) string {
	if len(s) == 0 || len(s) > 128 {
		return "document"
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || c == '_' || c == '.' {
			continue
		}
		return "document"
	}
	return s
}
func secureURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
func validate(v Values) error {
	h, p, e := net.SplitHostPort(v.Server.Listen)
	port, portErr := strconv.Atoi(p)
	if e != nil || portErr != nil || port < 0 || port > 65535 || p == "" || h == "" {
		return invalid("server.listen", "host:port required")
	}
	// External listener/TLS exposure remains with phase21; operational routes now require JWTs.
	ip := net.ParseIP(h)
	if ip == nil || !ip.IsLoopback() {
		return invalid("server.listen", "foundation must bind an explicit loopback IP")
	}
	for _, item := range []struct {
		name  string
		value Duration
		max   time.Duration
	}{
		{"server.read_header_timeout", v.Server.ReadHeaderTimeout, time.Minute}, {"server.read_timeout", v.Server.ReadTimeout, 5 * time.Minute}, {"server.write_timeout", v.Server.WriteTimeout, 5 * time.Minute}, {"server.idle_timeout", v.Server.IdleTimeout, 10 * time.Minute}, {"server.shutdown_grace", v.Server.ShutdownGrace, time.Minute}, {"store.connect_timeout", v.Store.ConnectTimeout, time.Minute}, {"store.transaction_timeout", v.Store.TransactionTimeout, time.Minute}, {"auth.request_timeout", v.Auth.RequestTimeout, time.Minute}, {"auth.jwks_max_stale", v.Auth.JWKSMaxStale, time.Hour}, {"auth.refresh_interval", v.Auth.RefreshInterval, time.Hour}, {"auth.max_token_lifetime", v.Auth.MaxTokenLifetime, 24 * time.Hour},
	} {
		if time.Duration(item.value) <= 0 || time.Duration(item.value) > item.max {
			return invalid(item.name, "duration out of bounds")
		}
	}
	if v.Auth.RefreshInterval >= v.Auth.JWKSMaxStale {
		return invalid("auth.refresh_interval", "must be below maximum stale age")
	}
	if v.Auth.ClockSkew < 0 || v.Auth.ClockSkew > Duration(time.Minute) {
		return invalid("auth.clock_skew", "must be between zero and one minute")
	}
	if v.Server.MaxBodyBytes < 1 || v.Server.MaxBodyBytes > 100<<20 || v.Server.MaxHeaderBytes < 1024 || v.Server.MaxHeaderBytes > 1<<20 {
		return invalid("server", "byte limit out of bounds")
	}
	if !secureURL(v.Auth.Issuer) {
		return invalid("auth.issuer", "HTTPS issuer without credentials, query or fragment required")
	}
	if !secureURL(v.Auth.JWKSURL) {
		return invalid("auth.jwks_url", "trusted HTTPS URL required")
	}
	if err := ValidateAuth(v.Auth); err != nil {
		return err
	}
	seen := map[string]bool{}
	if len(v.Auth.Algorithms) == 0 {
		return invalid("auth.algorithms", "asymmetric allowlist required")
	}
	for _, a := range v.Auth.Algorithms {
		if (a != "RS256" && a != "ES256" && a != "RS384" && a != "ES384" && a != "RS512" && a != "ES512") || seen[a] {
			return invalid("auth.algorithms", "invalid or duplicate algorithm")
		}
		seen[a] = true
	}
	if v.Store.TransactionTimeout < Duration(time.Millisecond) {
		return invalid("store.transaction_timeout", "minimum duration is one millisecond")
	}
	if v.Store.MaxConns < 1 || v.Store.MaxConns > 100 {
		return invalid("store.max_conns", "must be between 1 and 100")
	}
	if v.Store.MigrationPolicy != "apply" && v.Store.MigrationPolicy != "check" {
		return invalid("store.migration_policy", "expected apply or check")
	}
	if v.Telemetry.LogFormat != "json" && v.Telemetry.LogFormat != "text" {
		return invalid("telemetry.log_format", "expected json or text")
	}
	if v.Telemetry.OTel {
		return invalid("telemetry.otel", "export not implemented; disable explicitly")
	}
	if v.Features.MCP || v.Features.Reporting || v.Features.Renderer {
		return invalid("features", "requested capability is not implemented in phases 01-02")
	}
	if err := ValidateJobs(v.Jobs, v.Auth); err != nil {
		return err
	}
	if err := ValidateSources(v.Sources); err != nil {
		return err
	}
	if err := ValidateReadValidation(v.Exec); err != nil {
		return err
	}
	if err := ValidateUploads(v.Uploads); err != nil {
		return err
	}
	if err := ValidateProfiling(v.Profiling); err != nil {
		return err
	}
	if (v.Uploads.Enabled || v.Profiling.Enabled) && !v.Sources.Enabled {
		return invalid("engineering", "source access must be explicitly enabled")
	}
	return ValidateGateway(v.Gateway, v.Features.Gateway)
}

func checkJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return invalid("document", "nesting limit exceeded")
	}
	t, e := d.Token()
	if e != nil || t == nil {
		return invalid("document", "invalid or null JSON")
	}
	delim, ok := t.(json.Delim)
	if !ok {
		if depth == 0 {
			return invalid("document", "object required")
		}
		return nil
	}
	switch {
	case delim == '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return invalid("document", "invalid JSON")
			}
			s, ok := k.(string)
			if !ok || seen[s] {
				return invalid("document", "duplicate field")
			}
			seen[s] = true
			if err = checkJSON(d, depth+1); err != nil {
				return err
			}
		}
	case delim == '[' && depth > 0:
		for d.More() {
			if e = checkJSON(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return invalid("document", "object required")
	}
	if _, e = d.Token(); e != nil {
		return invalid("document", "invalid JSON")
	}
	return nil
}

// WriteDefaults produces the exact typed defaults, with no resolved credentials.
func WriteDefaults(w io.Writer) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	if err := e.Encode(Defaults()); err != nil {
		return invalid("defaults", "output failed")
	}
	return nil
}

// checkShape reports a known containing field rather than echoing an unknown input key.
func checkShape(data []byte, typ reflect.Type, path string) error {
	switch typ.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if json.Unmarshal(data, &object) != nil {
			return nil
		}
		known := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			known[strings.Split(field.Tag.Get("json"), ",")[0]] = field.Type
		}
		for key, value := range object {
			child, ok := known[key]
			if !ok {
				return invalid(path, "unknown or retired field")
			}
			if e := checkShape(value, child, path+"."+key); e != nil {
				return e
			}
		}
	case reflect.Map:
		var object map[string]json.RawMessage
		if json.Unmarshal(data, &object) != nil {
			return nil
		}
		for _, value := range object {
			if e := checkShape(value, typ.Elem(), path+".entry"); e != nil {
				return e
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if json.Unmarshal(data, &values) != nil {
			return nil
		}
		for _, value := range values {
			if e := checkShape(value, typ.Elem(), path+".item"); e != nil {
				return e
			}
		}
	}
	return nil
}
