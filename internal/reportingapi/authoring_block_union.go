package reportingapi

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/invopop/jsonschema"
)

// These transport-only unions preserve the value-typed legacy domain and SDK
// mapping requests. Member presence chooses one purpose; it never grants reach.
type authoringBlockMappingTransport struct {
	Block           string                           `json:"block"`
	ExpectedVersion int64                            `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64                            `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string                           `json:"digest"`
	Output          string                           `json:"output"`
	Mapping         *reporting.AuthoringChartMapping `json:"mapping,omitempty"`
	Presentation    *charts.PresentationPatch        `json:"presentation,omitempty"`
}

type authoringBlockCopyTransport struct {
	Block           string                           `json:"block"`
	ExpectedVersion int64                            `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64                            `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string                           `json:"digest"`
	Output          string                           `json:"output"`
	NewBlock        string                           `json:"new_block"`
	Mapping         *reporting.AuthoringChartMapping `json:"mapping,omitempty"`
	Presentation    *charts.PresentationPatch        `json:"presentation,omitempty"`
}

// JSONSchemaExtend is consumed by the ordinary shared SchemaFor reflection on
// both surfaces, including MCP's independent typed-schema parity check. The
// allOf constraints deliberately survive NullableCollections: legacy mapping
// collection/pointer nullability stays unchanged, while these new members and
// the patch's presence-aware values are non-null. No global decoder relaxation.
func (authoringBlockMappingTransport) JSONSchemaExtend(schema *jsonschema.Schema) {
	authoringBlockUnionSchema(schema)
}

func (authoringBlockCopyTransport) JSONSchemaExtend(schema *jsonschema.Schema) {
	authoringBlockUnionSchema(schema)
}

func authoringBlockUnionSchema(schema *jsonschema.Schema) {
	schema.OneOf = []*jsonschema.Schema{{Required: []string{"mapping"}}, {Required: []string{"presentation"}}}
	overlap := func(field string) map[string]any {
		return map[string]any{"not": map[string]any{
			"required": []string{"set", "reset"},
			"properties": map[string]any{
				"set":   map[string]any{"required": []string{field}},
				"reset": map[string]any{"contains": map[string]any{"const": field}},
			},
		}}
	}
	schema.AllOf = []*jsonschema.Schema{{Extras: map[string]any{"properties": map[string]any{
		"mapping": map[string]any{"type": "object"},
		"presentation": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"edits": map[string]any{
					"type": "array", "minItems": 1, "maxItems": 256,
					"items": map[string]any{
						"anyOf": []any{map[string]any{"required": []string{"set"}}, map[string]any{"required": []string{"reset"}}},
						"allOf": []any{overlap("display_label"), overlap("fraction_digits")},
						"properties": map[string]any{
							"set": map[string]any{
								"type": "object", "minProperties": 1,
								"properties": map[string]any{
									"display_label":   map[string]any{"type": "string", "maxLength": 256},
									"fraction_digits": map[string]any{"type": "integer", "minimum": 0, "maximum": 20},
								},
							},
							"reset": map[string]any{"type": "array", "minItems": 1, "maxItems": 2, "uniqueItems": true, "items": map[string]any{"enum": []string{"display_label", "fraction_digits"}}},
						},
					},
				},
			},
		},
	}}}}
}

func (in *authoringBlockMappingTransport) UnmarshalJSON(raw []byte) error {
	if err := validateAuthoringBlockUnionJSON(raw, false); err != nil {
		return err
	}
	type wire authoringBlockMappingTransport
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil {
		return reporting.ErrInvalid
	}
	*in = authoringBlockMappingTransport(decoded)
	return nil
}

func (in *authoringBlockCopyTransport) UnmarshalJSON(raw []byte) error {
	if err := validateAuthoringBlockUnionJSON(raw, true); err != nil {
		return err
	}
	type wire authoringBlockCopyTransport
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil {
		return reporting.ErrInvalid
	}
	*in = authoringBlockCopyTransport(decoded)
	return nil
}

func validateAuthoringBlockUnionJSON(raw []byte, copyRequest bool) error {
	decoded, err := gateway.DecodeJSON(raw, MaxBodyBytes)
	if err != nil {
		return reporting.ErrInvalid
	}
	fields, ok := decoded.(map[string]any)
	if !ok {
		return reporting.ErrInvalid
	}
	required := []string{"block", "expected_version", "revision", "digest", "output"}
	if copyRequest {
		required = append(required, "new_block")
	}
	allowed := map[string]bool{"mapping": true, "presentation": true}
	for _, field := range required {
		allowed[field] = true
		if fields[field] == nil {
			return reporting.ErrInvalid
		}
	}
	for field, value := range fields {
		if !allowed[field] || value == nil {
			return reporting.ErrInvalid
		}
	}
	_, mapping := fields["mapping"]
	_, presentation := fields["presentation"]
	if mapping == presentation {
		return reporting.ErrInvalid
	}
	return nil
}

func amendAuthoringBlock(service *reporting.Authoring) func(context.Context, identity.Envelope, authoringBlockMappingTransport) (reporting.AuthoringBlockView, error) {
	return func(ctx context.Context, e identity.Envelope, in authoringBlockMappingTransport) (reporting.AuthoringBlockView, error) {
		if (in.Mapping == nil) == (in.Presentation == nil) {
			return reporting.AuthoringBlockView{}, reporting.ErrInvalid
		}
		if in.Mapping != nil {
			return service.PatchBlockMapping(ctx, e, reporting.AuthoringBlockMappingRequest{Block: in.Block, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Digest: in.Digest, Output: in.Output, Mapping: *in.Mapping})
		}
		return service.PatchBlockPresentation(ctx, e, reporting.AuthoringBlockPresentationRequest{Block: in.Block, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Digest: in.Digest, Output: in.Output, Presentation: *in.Presentation})
	}
}

func copyAuthoringBlock(service *reporting.Authoring) func(context.Context, identity.Envelope, authoringBlockCopyTransport) (reporting.AuthoringBlockView, error) {
	return func(ctx context.Context, e identity.Envelope, in authoringBlockCopyTransport) (reporting.AuthoringBlockView, error) {
		if (in.Mapping == nil) == (in.Presentation == nil) {
			return reporting.AuthoringBlockView{}, reporting.ErrInvalid
		}
		if in.Mapping != nil {
			return service.CopyBlockMapping(ctx, e, reporting.AuthoringBlockCopyRequest{Block: in.Block, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Digest: in.Digest, Output: in.Output, NewBlock: in.NewBlock, Mapping: *in.Mapping})
		}
		return service.CopyBlockPresentation(ctx, e, reporting.AuthoringBlockPresentationCopyRequest{Block: in.Block, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Digest: in.Digest, Output: in.Output, NewBlock: in.NewBlock, Presentation: *in.Presentation})
	}
}
