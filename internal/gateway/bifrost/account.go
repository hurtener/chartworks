// Package bifrost implements Chartworks inference exclusively through the embedded Bifrost Go SDK.
package bifrost

import (
	"context"

	"github.com/maximhq/bifrost/core/schemas"
)

type account struct {
	provider          schemas.ModelProvider
	key, endpoint, ca string
	private           bool
	concurrency       int
	timeout           int
}

func (a *account) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	return []schemas.ModelProvider{a.provider}, nil
}
func (a *account) GetKeysForProvider(_ context.Context, p schemas.ModelProvider) ([]schemas.Key, error) {
	if p != a.provider {
		return nil, nil
	}
	return []schemas.Key{{ID: "configured-key", Name: "configured-key", Value: *schemas.NewSecretVar(a.key), Models: schemas.WhiteList{"*"}, Weight: 1}}, nil
}
func (a *account) GetConfigForProvider(p schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	if p != a.provider {
		return nil, nil
	}
	cfg := &schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: a.endpoint, DefaultRequestTimeoutInSeconds: a.timeout, MaxRetries: 0, MaxConnsPerHost: a.concurrency, AllowPrivateNetwork: a.private}, ConcurrencyAndBufferSize: schemas.ConcurrencyAndBufferSize{Concurrency: a.concurrency, BufferSize: a.concurrency}, Logger: quietLogger{}, OpenAIConfig: &schemas.OpenAIConfig{DisableStore: true}}
	if a.ca != "" {
		cfg.NetworkConfig.CACertPEM = schemas.NewSecretVar(a.ca)
	}
	// v1.6.2's DefaultMaxRetries is zero. Domain retry admission is the only retry owner.
	cfg.CheckAndSetDefaults()
	return cfg, nil
}

// Provider SDK messages can contain raw upstream error bodies. Deliberately suppress them;
// Chartworks emits its own content-free operation outcomes and receipts instead.
type quietLogger struct{}

func (quietLogger) Debug(string, ...any)                   {}
func (quietLogger) Info(string, ...any)                    {}
func (quietLogger) Warn(string, ...any)                    {}
func (quietLogger) Error(string, ...any)                   {}
func (quietLogger) Fatal(string, ...any)                   {}
func (quietLogger) SetLevel(schemas.LogLevel)              {}
func (quietLogger) SetOutputType(schemas.LoggerOutputType) {}
func (quietLogger) LogHTTPRequest(schemas.LogLevel, string) schemas.LogEventBuilder {
	return schemas.NoopLogEvent
}
