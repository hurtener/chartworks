package chartworks

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"testing"
)

func TestReportingPNGEnvelopeRoundTrip(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b.Bytes())
	r := ReportingRendition{Format: "png", MediaType: "image/png", Width: 8, Height: 8, Content: base64.StdEncoding.EncodeToString(b.Bytes()), ContentEncoding: "base64", Bytes: b.Len(), Digest: hex.EncodeToString(sum[:])}
	wire, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ReportingRendition
	if err = json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	raw, err := ReportingRenditionBytes(t.Context(), decoded, 1<<20)
	if err != nil || !bytes.Equal(raw, b.Bytes()) {
		t.Fatal("binary envelope lost", err)
	}
	decoded.Content += "\n"
	if _, err = ReportingRenditionBytes(t.Context(), decoded, 1<<20); err == nil {
		t.Fatal("noncanonical content admitted")
	}
}
