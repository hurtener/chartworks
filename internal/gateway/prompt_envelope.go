package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// PromptEnvelopeUsage is estimated admission evidence, never reported model usage.
// InputUpperBound counts normalized UTF-8 JSON bytes plus a reviewed protocol
// reserve. This deliberately conservative policy does not assume cl100k matches
// every remote tokenizer. ContextTokens zero means no model window was configured.
type PromptEnvelopeUsage struct {
	Version         string `json:"version"`
	Policy          string `json:"policy"`
	Digest          string `json:"digest"`
	RequestBytes    int    `json:"request_bytes"`
	MaxRequestBytes int    `json:"max_request_bytes"`
	InputUpperBound int    `json:"input_upper_bound"`
	ProtocolReserve int    `json:"protocol_reserve"`
	OutputReserve   int    `json:"output_reserve"`
	ContextTokens   int    `json:"context_tokens,omitempty"`
}

// PromptEnvelope is immutable local preparation, not authority or a dispatch token.
// The adapter must prepare/check it again at actual dispatch. It contains no keys.
type PromptEnvelope struct {
	model, system, provider, configuration                  string
	schema                                                  json.RawMessage
	schemaName                                              string
	maxBytes, contextTokens, protocolReserve, outputReserve int
}

// GenerationEnvelopeProvider exposes the existing adapter's effective bounds for
// local context fitting. It performs no model/source call or budget reservation.
// Production Bifrost implements this; recorded engines may also implement it.
type GenerationEnvelopeProvider interface {
	GenerationEnvelope(context.Context, string, string, *Schema) (PromptEnvelope, error)
}

// NewPromptEnvelope copies an effective, server-owned generation configuration.
func NewPromptEnvelope(provider, model, system, configuration string, schema *Schema, maxBytes, contextTokens, protocolReserve, outputReserve int) (PromptEnvelope, error) {
	if provider == "" || len(provider) > 64 || model == "" || len(model) > 256 || !utf8.ValidString(model) || strings.ContainsAny(model, "\x00\r\n\t") || !utf8.ValidString(system) || strings.ContainsRune(system, 0) || schema == nil || schema.Name() == "" || maxBytes < 1 || maxBytes > 4<<20 || len(system) > 4<<20 || contextTokens < 0 || contextTokens > 16<<20 || protocolReserve < 64 || protocolReserve > 8192 || outputReserve < 1 || outputReserve > 65536 {
		return PromptEnvelope{}, ErrInput
	}
	if configuration != "" && (len(configuration) != 64 || strings.Trim(configuration, "0123456789abcdef") != "") {
		return PromptEnvelope{}, ErrInput
	}
	if contextTokens != 0 && contextTokens <= protocolReserve+outputReserve {
		return PromptEnvelope{}, ErrInput
	}
	return PromptEnvelope{provider: provider, model: model, system: system, configuration: configuration, schema: schema.Document(), schemaName: schema.Name(), maxBytes: maxBytes, contextTokens: contextTokens, protocolReserve: protocolReserve, outputReserve: outputReserve}, nil
}

// Measure checks a complete normalized chat payload with strict output schema,
// effective system/model, JSON escaping and the full output ceiling. The protocol
// reserve covers adapter framing not represented in this normalized payload.
// No prompt text or schema is returned in this admission receipt.
func (e PromptEnvelope) Measure(prompt string) (PromptEnvelopeUsage, bool, error) {
	if e.maxBytes == 0 || prompt == "" || !utf8.ValidString(prompt) || strings.ContainsRune(prompt, 0) {
		return PromptEnvelopeUsage{}, false, ErrInput
	}
	if len(prompt) > 4<<20 {
		return PromptEnvelopeUsage{}, false, ErrInput
	}
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	body := struct {
		Model               string    `json:"model"`
		Messages            []message `json:"messages"`
		ResponseFormat      any       `json:"response_format"`
		MaxCompletionTokens int       `json:"max_completion_tokens"`
		Store               bool      `json:"store"`
	}{e.model, []message{{"system", e.system}, {"user", prompt}}, map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": e.schemaName, "strict": true, "schema": e.schema}}, e.outputReserve, false}
	raw, err := json.Marshal(body)
	if err != nil {
		return PromptEnvelopeUsage{}, false, ErrInput
	}
	out := PromptEnvelopeUsage{Version: "prompt-envelope-v1", Policy: "utf8-json-byte-bound-v1", RequestBytes: len(raw), MaxRequestBytes: e.maxBytes, InputUpperBound: len(raw) + e.protocolReserve, ProtocolReserve: e.protocolReserve, OutputReserve: e.outputReserve, ContextTokens: e.contextTokens}
	identity, _ := json.Marshal([]any{e.provider, e.configuration, out, json.RawMessage(raw)})
	digest := sha256.Sum256(identity)
	out.Digest = hex.EncodeToString(digest[:])
	fits := out.RequestBytes <= e.maxBytes && (e.contextTokens == 0 || out.InputUpperBound+out.OutputReserve <= e.contextTokens)
	return out, fits, nil
}

// String and logging deliberately redact the effective request material.
func (PromptEnvelope) String() string { return "prompt-envelope(redacted)" }

// GoString preserves redaction for Go-syntax formatting.
func (e PromptEnvelope) GoString() string { return e.String() }

// LogValue prevents accidental system/schema logging.
func (e PromptEnvelope) LogValue() slog.Value { return slog.StringValue(e.String()) }
