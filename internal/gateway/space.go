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

// Key returns the stable persisted digest of the complete embedding descriptor.
func (s EmbeddingSpace) Key() string {
	// Preserve the phase 07 persisted key format. EmbeddingSpace has the exact
	// field order and JSON tags of the original vindex.Space value, so the full
	// descriptor remains bound without invalidating ready stored generations.
	raw, _ := json.Marshal(s)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
