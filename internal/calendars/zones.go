// Package calendars supplies one pinned, credential-free timezone database.
package calendars

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

//go:embed zoneinfo.zip
var archive []byte
var indexOnce sync.Once
var index map[string]*zip.File
var indexErr error
var locations sync.Map

// ErrZone rejects unknown or unsafe names and corrupted archive data.
var ErrZone = errors.New("calendar: timezone unavailable")

func archiveIndex() {
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != archiveSHA256 {
		indexErr = ErrZone
		return
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || len(reader.File) > 2048 {
		indexErr = ErrZone
		return
	}
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if file.UncompressedSize64 == 0 || file.UncompressedSize64 > 65536 || files[file.Name] != nil {
			indexErr = ErrZone
			return
		}
		files[file.Name] = file
	}
	index = files
}

// Location ignores mutable system files and environment-selected databases.
// Successful entries alone are cached, bounding cache size by the archive.
func Location(name string) (*time.Location, error) {
	if name == "UTC" {
		return time.UTC, nil
	}
	if len(name) == 0 || len(name) > 128 || name == "Local" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00\r\n") {
		return nil, ErrZone
	}
	indexOnce.Do(archiveIndex)
	if indexErr != nil {
		return nil, indexErr
	}
	if value, ok := locations.Load(name); ok {
		return value.(*time.Location), nil
	}
	file := index[name]
	if file == nil {
		return nil, ErrZone
	}
	reader, err := file.Open()
	if err != nil {
		return nil, ErrZone
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, 65537))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(data) > 65536 {
		return nil, ErrZone
	}
	zone, err := time.LoadLocationFromTZData(name, data)
	if err != nil {
		return nil, ErrZone
	}
	actual, _ := locations.LoadOrStore(name, zone)
	return actual.(*time.Location), nil
}
