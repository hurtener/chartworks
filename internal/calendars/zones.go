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
	index, indexErr = parseArchive(archive, archiveSHA256)
}

// parseArchive verifies the pinned bytes before indexing a bounded ZIP. It is
// deliberately independent of process-wide caches, so malformed distributions
// can be tested without changing the trusted embedded database or global state.
func parseArchive(data []byte, expectedHash string) (map[string]*zip.File, error) {
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expectedHash {
		return nil, ErrZone
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > 2048 {
		return nil, ErrZone
	}
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if file.UncompressedSize64 == 0 || file.UncompressedSize64 > 65536 || files[file.Name] != nil {
			return nil, ErrZone
		}
		files[file.Name] = file
	}
	return files, nil
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
