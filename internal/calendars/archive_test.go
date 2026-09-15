package calendars

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
)

func TestArchiveDistributionBoundaries(t *testing.T) {
	build := func(names []string, size int) []byte {
		t.Helper()
		var data bytes.Buffer
		writer := zip.NewWriter(&data)
		for _, name := range names {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write(bytes.Repeat([]byte{'x'}, size)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return data.Bytes()
	}
	hash := func(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
	many := make([]string, 2049)
	for i := range many {
		many[i] = fmt.Sprintf("zone-%04d", i)
	}
	for _, test := range []struct {
		name   string
		data   []byte
		digest string
	}{
		{"wrong-pin", archive, hash([]byte("not the pinned archive"))},
		{"malformed-zip", []byte("not a ZIP"), hash([]byte("not a ZIP"))},
		{"empty-zip", build(nil, 1), ""},
		{"too-many-entries", build(many, 1), ""},
		{"duplicate-entry", build([]string{"zone", "zone"}, 1), ""},
		{"empty-entry", build([]string{"zone"}, 0), ""},
		{"oversized-entry", build([]string{"zone"}, 65537), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.digest == "" {
				test.digest = hash(test.data)
			}
			entries, err := parseArchive(test.data, test.digest)
			if !errors.Is(err, ErrZone) || entries != nil {
				t.Fatal("invalid distribution produced usable entries", len(entries), err)
			}
		})
	}
	entries, err := parseArchive(archive, archiveSHA256)
	if err != nil || entries["America/New_York"] == nil || entries["America/Argentina/Buenos_Aires"] == nil {
		t.Fatal("trusted distribution not usable", len(entries), err)
	}
}
