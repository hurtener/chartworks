package reporting

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFilterCursorTamperExpiryAndTypedLabels(t *testing.T) {
	service := &Documents{cursorKey: [32]byte{1, 2, 3, 4}}
	cursor := filterCursor{Report: "report", Revision: 1, Filter: "region", Search: "nor", Limit: 20, Locale: "en-US", SourceRevision: 3, Authority: strings.Repeat("a", 64), Type: "text", Last: json.RawMessage(`"North"`), Expires: time.Now().Add(time.Minute).Unix()}
	encoded, err := service.encodeFilterCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := service.decodeFilterCursor(encoded)
	if err != nil || decoded.Report != cursor.Report || string(decoded.Last) != string(cursor.Last) {
		t.Fatal("cursor round trip", decoded, err)
	}
	tampered := encoded[:len(encoded)-1] + "x"
	if strings.HasSuffix(encoded, "x") {
		tampered = encoded[:len(encoded)-1] + "y"
	}
	if _, err := service.decodeFilterCursor(tampered); err == nil {
		t.Fatal("tampered cursor accepted")
	}
	cursor.Expires = time.Now().Add(-time.Second).Unix()
	expired, err := service.encodeFilterCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.decodeFilterCursor(expired); !errors.Is(err, ErrStale) {
		t.Fatal("expired cursor classification", err)
	}
	for _, test := range []struct {
		raw, locale, label string
	}{{`null`, "es-AR", "Nulo"}, {`true`, "en-US", "true"}, {`"9007199254740993.125"`, "en-US", "9007199254740993.125"}} {
		label, err := filterLabel(json.RawMessage(test.raw), test.locale)
		if err != nil || label != test.label {
			t.Fatal(test.raw, label, err)
		}
	}
}

func TestFilterOptionSourceValidation(t *testing.T) {
	valid := Parameter{Name: "region", Type: "dimension_value", Required: true, Dimension: &DimensionReference{Topic: "topic", Version: "v1", Dimension: "region"}}
	source := &FilterOptionSource{Version: 1, Block: "block", BlockRevision: 1, Topic: "topic", TopicVersion: "v1", Dataset: "sales", Column: "region"}
	if !validFilterOptionSource(valid, source) {
		t.Fatal("valid exact source rejected")
	}
	bad := *source
	bad.BlockRevision = 0
	if validFilterOptionSource(valid, &bad) || validFilterOptionSource(Parameter{Name: "period", Type: "relative_period"}, source) {
		t.Fatal("floating or non-scalar option source accepted")
	}
}
