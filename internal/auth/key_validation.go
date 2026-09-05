package auth

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/identity"
	"io"
	"math/big"
	"strings"
)

func str(m map[string]json.RawMessage, k string) string {
	var s string
	_ = json.Unmarshal(m[k], &s)
	return s
}
func decode(s string) []byte {
	v, e := base64.RawURLEncoding.Strict().DecodeString(s)
	if e != nil {
		return nil
	}
	return v
}
func validKeys(data []byte, allowed []string) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	if uniqueValue(d, 0) != nil {
		return false
	}
	if _, e := d.Token(); !errors.Is(e, io.EOF) {
		return false
	}
	var doc struct {
		Keys []map[string]json.RawMessage `json:"keys"`
	}
	if json.Unmarshal(data, &doc) != nil || len(doc.Keys) == 0 || len(doc.Keys) > 32 {
		return false
	}
	algorithms := map[string]bool{}
	for _, a := range allowed {
		algorithms[a] = true
	}
	seen := map[string]bool{}
	for _, k := range doc.Keys {
		for _, field := range []string{"kid", "kty", "use", "alg", "n", "e", "crv", "x", "y"} {
			if raw, present := k[field]; present {
				var v string
				if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &v) != nil {
					return false
				}
			}
		}
		kid := str(k, "kid")
		if !identity.Identifier(kid) || seen[kid] {
			return false
		}
		seen[kid] = true
		for _, name := range []string{"d", "p", "q", "dp", "dq", "qi", "oth", "k"} {
			if _, ok := k[name]; ok {
				return false
			}
		}
		if use := str(k, "use"); use != "" && use != "sig" {
			return false
		}
		if ops, exists := k["key_ops"]; exists {
			var values []string
			if json.Unmarshal(ops, &values) != nil || len(values) != 1 || values[0] != "verify" {
				return false
			}
		}
		alg := str(k, "alg")
		if alg != "" && !algorithms[alg] {
			return false
		}
		switch str(k, "kty") {
		case "RSA":
			if alg != "" && !strings.HasPrefix(alg, "RS") {
				return false
			}
			if alg == "" && !algorithms["RS256"] && !algorithms["RS384"] && !algorithms["RS512"] {
				return false
			}
			n := new(big.Int).SetBytes(decode(str(k, "n")))
			eb := decode(str(k, "e"))
			if n.BitLen() < 2048 || n.BitLen() > 8192 || len(eb) == 0 || len(eb) > 4 {
				return false
			}
			exponent := new(big.Int).SetBytes(eb).Int64()
			if exponent < 3 || exponent > 2147483647 || exponent%2 == 0 {
				return false
			}
		case "EC":
			var curve ecdh.Curve
			var size int
			var expected string
			switch str(k, "crv") {
			case "P-256":
				curve = ecdh.P256()
				size = 32
				expected = "ES256"
			case "P-384":
				curve = ecdh.P384()
				size = 48
				expected = "ES384"
			case "P-521":
				curve = ecdh.P521()
				size = 66
				expected = "ES512"
			default:
				return false
			}
			if !algorithms[expected] || (alg != "" && alg != expected) {
				return false
			}
			x, y := decode(str(k, "x")), decode(str(k, "y"))
			if len(x) != size || len(y) != size {
				return false
			}
			point := append([]byte{4}, x...)
			point = append(point, y...)
			if _, e := curve.NewPublicKey(point); e != nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func uniqueValue(d *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("invalid key document")
	}
	v, e := d.Token()
	if e != nil || v == nil {
		return errors.New("invalid key document")
	}
	if delimiter, ok := v.(json.Delim); ok {
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return errors.New("duplicate key field")
				}
				seen[key] = true
				if e = uniqueValue(d, depth+1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = uniqueValue(d, depth+1); e != nil {
					return e
				}
			}
		default:
			return errors.New("invalid key document")
		}
		_, e = d.Token()
		return e
	}
	return nil
}
