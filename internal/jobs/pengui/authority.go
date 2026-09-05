// Package pengui implements the actual Pengui execution-authority/v1 HTTP consumer.
// It never signs, renews locally, or persists an end-user token.
package pengui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
)

// Credential is an operator-resolved existing Pengui broker client, not Chartworks user auth.
type Credential struct{ ClientID, Secret string }

func (Credential) String() string               { return "pengui-broker-credential(redacted)" }
func (c Credential) GoString() string           { return c.String() }
func (Credential) MarshalJSON() ([]byte, error) { return []byte(`"redacted"`), nil }

// Provider owns immutable issuer endpoint/credential references and a bounded dedicated client.
type Provider struct {
	endpoint    string
	credentials map[string]Credential
	verifier    *auth.Verifier
	client      *http.Client
}

func New(endpoint string, credentials map[string]Credential, verifier *auth.Verifier, client *http.Client) (*Provider, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, "/exchange/execution-authority") || verifier == nil || len(credentials) == 0 || len(credentials) > 128 {
		return nil, jobs.ErrInvalid
	}
	copied := map[string]Credential{}
	for tenant, c := range credentials {
		if !identity.Identifier(tenant) || c.ClientID == "" || len(c.ClientID) > 128 || c.Secret == "" || len(c.Secret) > 256 {
			return nil, jobs.ErrInvalid
		}
		copied[tenant] = c
	}
	c := http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	if client != nil {
		c = *client
	}
	c.Timeout = 3 * time.Second
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("broker redirect refused") }
	return &Provider{endpoint: endpoint, credentials: copied, verifier: verifier, client: &c}, nil
}
func (p *Provider) Close() { p.client.CloseIdleConnections() }

type request struct {
	Version  int    `json:"version"`
	Binding  string `json:"binding_id"`
	Job      string `json:"job_id"`
	Manifest string `json:"manifest_hash"`
}
type response struct {
	Version         int    `json:"version"`
	AccessToken     string `json:"access_token"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	Binding         string `json:"binding_id"`
	BindingRevision int64  `json:"binding_revision"`
}

func (p *Provider) Acquire(ctx context.Context, j jobs.Job) (identity.Envelope, error) {
	if !j.Valid() {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	credential, ok := p.credentials[j.Tenant]
	if !ok {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	body, err := json.Marshal(request{Version: 1, Binding: j.BindingID, Job: j.ID, Manifest: j.ManifestHash})
	if err != nil {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return identity.Envelope{}, jobs.ErrTransient
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(credential.ClientID, credential.Secret)
	resp, err := p.client.Do(req)
	if err != nil {
		return identity.Envelope{}, jobs.ErrTransient
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return identity.Envelope{}, jobs.ErrTransient
	}
	if resp.StatusCode != http.StatusOK {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	object, err := auth.Object(data, 64<<10)
	if err != nil || len(object) != 6 {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	for _, key := range []string{"version", "access_token", "token_type", "expires_in", "binding_id", "binding_revision"} {
		if object[key] == nil {
			return identity.Envelope{}, jobs.ErrAuthority
		}
	}
	var result response
	if json.Unmarshal(data, &result) != nil || result.Version != 1 || result.TokenType != "Bearer" || result.Binding != j.BindingID || result.BindingRevision < 1 || result.ExpiresIn < 1 || result.ExpiresIn > 60 {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	e, err := p.verifier.VerifyExecution(ctx, result.AccessToken, j.BindingID, j.ID, j.ManifestHash, result.BindingRevision)
	if err != nil || jobs.AssertExecution(e, j) != nil {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	return e, nil
}

var _ jobs.Authority = (*Provider)(nil)
