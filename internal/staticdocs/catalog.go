// Package staticdocs supplies immutable compiled documentation, not an agent
// configuration, authority source or executable tenant-data loader.
package staticdocs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

const (
	Namespace        = "chartworks://report_app/docs/"
	MIME             = "text/markdown"
	MaxDocuments     = 16
	MaxDocumentBytes = 64 << 10
	MaxCatalogBytes  = 256 << 10
)

var ErrCatalog = errors.New("documentation: invalid catalog")

// Reference is content-free discovery metadata. SHA256 identifies the exact
// UTF-8 bytes returned by Read; DocumentRef identifies their repository source.
type Reference struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mime_type"`
	Version     int    `json:"version"`
	SHA256      string `json:"sha256"`
	Bytes       int    `json:"bytes"`
	DocumentRef string `json:"document_ref"`
}

type Document struct {
	Reference Reference `json:"reference"`
	Text      string    `json:"text"`
}

// Catalog can only be constructed from bounded static strings. No callback,
// filesystem path supplied by a caller, URL fetch or mutable byte slice exists.
type Catalog struct {
	action    string
	documents []Document
}

func New(action string, documents []Document) (*Catalog, error) {
	if len(action) < 3 || len(action) > 64 || strings.ContainsAny(action, " \t\r\n\x00") || len(documents) < 1 || len(documents) > MaxDocuments {
		return nil, ErrCatalog
	}
	out := &Catalog{action: action, documents: make([]Document, 0, len(documents))}
	seen, total := map[string]bool{}, 0
	for _, document := range documents {
		ref := document.Reference
		if !validReference(ref) || seen[ref.URI] || len(document.Text) < 1 || len(document.Text) > MaxDocumentBytes || !utf8.ValidString(document.Text) || strings.ContainsRune(document.Text, 0) {
			return nil, ErrCatalog
		}
		seen[ref.URI] = true
		total += len(document.Text)
		if total > MaxCatalogBytes {
			return nil, ErrCatalog
		}
		digest := sha256.Sum256([]byte(document.Text))
		ref.SHA256, ref.Bytes = hex.EncodeToString(digest[:]), len(document.Text)
		out.documents = append(out.documents, Document{Reference: ref, Text: document.Text})
	}
	return out, nil
}

func validReference(ref Reference) bool {
	if ref.Version < 1 || ref.Version > 999 || ref.MIMEType != MIME || len(ref.URI) > 256 || len(ref.Name) < 1 || len(ref.Name) > 80 || len(ref.Description) < 20 || len(ref.Description) > 512 || !utf8.ValidString(ref.Name+ref.Description) || strings.ContainsAny(ref.Name+ref.Description, "\x00\r\n") {
		return false
	}
	u, err := url.Parse(ref.URI)
	if err != nil || u.Scheme != "chartworks" || u.Host != "report_app" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.Opaque != "" || strings.ContainsAny(ref.URI, "%?#@\\\x00\r\n\t") {
		return false
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) != 4 || parts[0] != "" || parts[1] != "docs" || !identity.Identifier(parts[2]) || parts[2] == "." || parts[2] == ".." || parts[3] != "v"+strconv.Itoa(ref.Version) {
		return false
	}
	if !strings.HasPrefix(ref.DocumentRef, "docs/") || !strings.HasSuffix(ref.DocumentRef, ".md") || len(ref.DocumentRef) > 256 || path.Clean(ref.DocumentRef) != ref.DocumentRef || strings.ContainsAny(ref.DocumentRef, "\\?#\x00\r\n\t") {
		return false
	}
	return true
}

func (c *Catalog) Action() string {
	if c == nil {
		return ""
	}
	return c.action
}

// References returns detached metadata, never the full documents.
func (c *Catalog) References() []Reference {
	out := []Reference{}
	if c != nil {
		for _, d := range c.documents {
			out = append(out, d.Reference)
		}
	}
	return out
}

// Read is the single HTTP/MCP core. Static content needs the native read action
// and a current verified envelope; content never grants target or data reach.
func (c *Catalog) Read(ctx context.Context, e identity.Envelope, uri string) (Document, error) {
	if ctx == nil || !e.Valid() {
		return Document{}, access.ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if c == nil {
		return Document{}, fmt.Errorf("%w: unavailable", ErrCatalog)
	}
	if !e.Has(c.action) {
		return Document{}, access.ErrForbidden
	}
	for _, d := range c.documents {
		if d.Reference.URI == uri {
			return d, nil
		}
	}
	return Document{}, access.ErrNotFound
}
