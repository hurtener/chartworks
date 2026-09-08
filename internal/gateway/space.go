package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// EmbeddingSpace is the gateway's complete non-secret vector identity. Engines
// retain a private value and return detached values, never their configuration.
// Preprocessing includes the exact input and float conversion implementation.
type EmbeddingSpace struct {
	Provider      string `json:"provider"`
	Route         string `json:"route"`
	Endpoint      string `json:"endpoint"`
	Model         string `json:"model"`
	Revision      string `json:"revision"`
	Dimensions    int    `json:"dimensions"`
	Preprocessing string `json:"preprocessing"`
	InputType     string `json:"input_type"`
	Normalization string `json:"normalization"`
}

func (s EmbeddingSpace) Key() string {
	raw, _ := json.Marshal([]any{"chartworks-embedding-space-v1", s.Provider, s.Route, s.Endpoint, s.Model, s.Revision, s.Dimensions, s.Preprocessing, s.InputType, s.Normalization})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
