package drafts

import (
	"encoding/json"
	"fmt"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
)

// qualityEntityReference is a request-local choice, never a persisted identity.
// Some strict providers reject quotes inside enum literals, including the quotes
// in canonical JSON column coordinates. Only these review choices use handles.
type qualityEntityReference struct {
	Handle string `json:"handle"`
	Entity string `json:"entity"`
}

type qualityReviewTransport struct {
	domain, schema *gateway.Schema
	references     []qualityEntityReference
	entities       map[string]string
}

func newQualityReviewTransport(candidateDigest, contextDigest string, coverage []string) (qualityReviewTransport, error) {
	out := qualityReviewTransport{}
	domain, err := qualityReviewSchema(candidateDigest, contextDigest, coverage)
	if err != nil {
		return out, err
	}
	out.domain = domain
	out.entities = make(map[string]string, len(coverage))
	handles := make([]string, len(coverage))
	for i, entity := range coverage {
		handle := fmt.Sprintf("e%06d", i)
		handles[i] = handle
		out.references = append(out.references, qualityEntityReference{Handle: handle, Entity: entity})
		out.entities[handle] = entity
	}
	var wire map[string]any
	if json.Unmarshal(domain.Document(), &wire) != nil {
		return qualityReviewTransport{}, gateway.ErrInput
	}
	finding := wire["properties"].(map[string]any)["findings"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	finding["entities"].(map[string]any)["items"].(map[string]any)["enum"] = handles
	raw, err := json.Marshal(wire)
	if err != nil {
		return qualityReviewTransport{}, gateway.ErrInput
	}
	out.schema, err = gateway.NewSchema("topic_quality_review", raw)
	return out, err
}

func (t qualityReviewTransport) domainDigest() string { return readexec.Hash(t.domain.Document()) }

// decode validates the handle-only response before resolving any reference, then
// validates the restored response against the original exact canonical schema.
// No guessed prefixes, quote rewriting or fallback to provider-supplied IDs.
func (t qualityReviewTransport) decode(raw []byte) (qualityWire, error) {
	var wire qualityWire
	if t.schema.Validate(raw, 64<<10) != nil {
		return wire, gateway.ErrOutput
	}
	if json.Unmarshal(raw, &wire) != nil {
		return qualityWire{}, gateway.ErrOutput
	}
	for i := range wire.Findings {
		for j, handle := range wire.Findings[i].Entities {
			entity, ok := t.entities[handle]
			if !ok {
				return qualityWire{}, gateway.ErrOutput
			}
			wire.Findings[i].Entities[j] = entity
		}
	}
	restored, err := json.Marshal(wire)
	if err != nil || t.domain.Validate(restored, 64<<10) != nil {
		return qualityWire{}, gateway.ErrOutput
	}
	return wire, nil
}
