package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/staticdocs"
)

func TestReportAuthoringEmbedsExactPublicContracts(t *testing.T) {
	catalog, err := ReportAuthoring()
	if err != nil {
		t.Fatal(err)
	}
	refs := catalog.References()
	if len(refs) != 11 || catalog.Action() != "reporting.read" {
		t.Fatal("catalog changed", refs)
	}
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"reporting.read"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, ref := range refs {
		path := strings.TrimPrefix(ref.DocumentRef, "docs/")
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := catalog.Read(t.Context(), e, ref.URI)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(source)
		if d.Text != string(source) || ref.SHA256 != hex.EncodeToString(digest[:]) || ref.Bytes != len(source) || ref.MIMEType != staticdocs.MIME {
			t.Fatal("embedded contract drift", ref)
		}
		total += ref.Bytes
	}
	t.Logf("catalog: %d documents; %d UTF-8 bytes", len(refs), total)
	if total > staticdocs.MaxCatalogBytes {
		t.Fatal(total)
	}
}
