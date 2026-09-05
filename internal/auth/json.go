// Package auth verifies Pengui tokens. It contains no signer or local issuer.
package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// Object accepts bounded, duplicate-free, non-null JSON with exact object keys.
// It is shared by the JOSE and protected request decoders; errors never echo input.
func Object(data []byte, limit int) (map[string]json.RawMessage, error) {
	if len(data) == 0 || len(data) > limit || !utf8.Valid(data) {
		return nil, ErrToken
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if uniqueValue(d, 0) != nil {
		return nil, ErrToken
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrToken
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil || m == nil {
		return nil, ErrToken
	}
	return m, nil
}
