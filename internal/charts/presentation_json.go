package charts

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// presentationObject is deliberately local to the new extension. Historical
// mapping fields keep their existing decoder semantics; the new vocabulary is
// closed, case-sensitive, duplicate-free and never accepts explicit null.
func presentationObject(data []byte, allowed, required []string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(data) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrInvalid
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, ErrInvalid
		}
		key, ok := token.(string)
		if !ok || !oneOf(key, allowed...) {
			return nil, ErrInvalid
		}
		if _, exists := fields[key]; exists {
			return nil, ErrInvalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, ErrInvalid
		}
		fields[key] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return nil, ErrInvalid
		}
	}
	return fields, nil
}

func decodePresentationValue(fields map[string]json.RawMessage, key string, value any) error {
	if data, ok := fields[key]; ok {
		if json.Unmarshal(data, value) != nil {
			return ErrInvalid
		}
	}
	return nil
}

func (p *ColumnPresentation) UnmarshalJSON(data []byte) error {
	fields, err := presentationObject(data, []string{"version", "columns"}, []string{"version", "columns"})
	if err != nil {
		return err
	}
	var value ColumnPresentation
	if decodePresentationValue(fields, "version", &value.Version) != nil || decodePresentationValue(fields, "columns", &value.Columns) != nil || value.Version != PresentationVersion || len(value.Columns) == 0 || len(value.Columns) > 256 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range value.Columns {
		if seen[c.Column] {
			return ErrInvalid
		}
		seen[c.Column] = true
	}
	*p = value
	return nil
}

func (o *ColumnDisplayOverride) UnmarshalJSON(data []byte) error {
	fields, err := presentationObject(data, []string{"column", "display_label", "fraction_digits"}, []string{"column"})
	if err != nil {
		return err
	}
	var value ColumnDisplayOverride
	if decodePresentationValue(fields, "column", &value.Column) != nil || decodePresentationValue(fields, "display_label", &value.DisplayLabel) != nil || decodePresentationValue(fields, "fraction_digits", &value.FractionDigits) != nil || !identifier(value.Column) || !validPresentationSet(value.DisplayLabel, value.FractionDigits) {
		return ErrInvalid
	}
	*o = value
	return nil
}

func validPresentationSet(label *string, digits *int) bool {
	return (label != nil || digits != nil) && (label == nil || text(*label, 256)) && (digits == nil || *digits >= 0 && *digits <= 20)
}

func (s *ColumnPresentationSet) UnmarshalJSON(data []byte) error {
	fields, err := presentationObject(data, []string{"display_label", "fraction_digits"}, nil)
	if err != nil {
		return err
	}
	var value ColumnPresentationSet
	if decodePresentationValue(fields, "display_label", &value.DisplayLabel) != nil || decodePresentationValue(fields, "fraction_digits", &value.FractionDigits) != nil || !validPresentationSet(value.DisplayLabel, value.FractionDigits) {
		return ErrInvalid
	}
	*s = value
	return nil
}

func (e *ColumnPresentationEdit) UnmarshalJSON(data []byte) error {
	fields, err := presentationObject(data, []string{"column", "set", "reset"}, []string{"column"})
	if err != nil {
		return err
	}
	var value ColumnPresentationEdit
	if decodePresentationValue(fields, "column", &value.Column) != nil || decodePresentationValue(fields, "set", &value.Set) != nil || decodePresentationValue(fields, "reset", &value.Reset) != nil || validatePresentationEdit(value) != nil {
		return ErrInvalid
	}
	if _, ok := fields["reset"]; ok && len(value.Reset) == 0 {
		return ErrInvalid
	}
	*e = value
	return nil
}

func (p *PresentationPatch) UnmarshalJSON(data []byte) error {
	fields, err := presentationObject(data, []string{"version", "edits"}, []string{"version", "edits"})
	if err != nil {
		return err
	}
	var value PresentationPatch
	if decodePresentationValue(fields, "version", &value.Version) != nil || decodePresentationValue(fields, "edits", &value.Edits) != nil || value.Version != PresentationVersion || len(value.Edits) == 0 || len(value.Edits) > 256 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, edit := range value.Edits {
		if seen[edit.Column] {
			return ErrInvalid
		}
		seen[edit.Column] = true
	}
	*p = value
	return nil
}

// UnmarshalJSON leaves pre-existing mapping fields unchanged in behavior while
// ensuring that a present extension cannot be null, duplicated or mis-cased.
func (m *Mapping) UnmarshalJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token == json.Delim('{') {
		found := false
		for d.More() {
			token, err = d.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok {
				return ErrInvalid
			}
			var value json.RawMessage
			if err = d.Decode(&value); err != nil {
				return err
			}
			if strings.EqualFold(key, "presentation") {
				if key != "presentation" || found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return ErrInvalid
				}
				found = true
			}
		}
	}
	type plain Mapping
	value := plain(*m)
	if err = json.Unmarshal(data, &value); err != nil {
		return err
	}
	*m = Mapping(value)
	return nil
}
