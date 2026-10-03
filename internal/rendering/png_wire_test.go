package rendering

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func TestPNGWireRejectsCorruptionAndBudgetEscape(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	r := Rendition{Format: "png", MediaType: "image/png", ContentEncoding: "base64", Width: 8, Height: 8, Content: base64.StdEncoding.EncodeToString(raw)}
	if got, err := renditionContent(context.Background(), r, 4096); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("valid PNG: %v", err)
	}
	cases := map[string][]byte{"trailing": append(append([]byte{}, raw...), 0), "truncated": raw[:len(raw)-1]}
	corrupt := append([]byte{}, raw...)
	corrupt[20] ^= 1
	cases["crc"] = corrupt
	unknown := append([]byte{}, raw...)
	copy(unknown[12:16], "tEXt")
	binary.BigEndian.PutUint32(unknown[29:33], crc32.ChecksumIEEE(unknown[12:29]))
	cases["unknown_chunk"] = unknown
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			x := r
			x.Content = base64.StdEncoding.EncodeToString(data)
			if _, err := renditionContent(context.Background(), x, 4096); err == nil {
				t.Fatal("accepted malformed image")
			}
		})
	}
	for name, change := range map[string]func(*Rendition){"dimensions": func(x *Rendition) { x.Width = 9 }, "encoding": func(x *Rendition) { x.ContentEncoding = "" }, "media": func(x *Rendition) { x.MediaType = "text/plain" }, "noncanonical": func(x *Rendition) { x.Content += "\n" }} {
		t.Run(name, func(t *testing.T) {
			x := r
			change(&x)
			if _, err := renditionContent(context.Background(), x, 4096); err == nil {
				t.Fatal("accepted invalid envelope")
			}
		})
	}
	if _, err := renditionContent(context.Background(), r, len(r.Content)-1); err == nil {
		t.Fatal("encoded budget ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := renditionContent(ctx, r, 4096); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestPNGSealedProjectionAndRawDigest(t *testing.T) {
	g := chartfixtures.Produce(charts.Bar, "binding")
	view := reporting.DeliveryViewResult{Output: &reporting.ViewerOutput{State: "succeeded", RetainedDigest: "retained", Chart: g.Output}, Timezone: "UTC"}
	req := Request{Format: "png", Theme: "light", Width: 800, Height: 420}
	r, err := renderSealedContext(t.Context(), req, view, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := renditionContent(t.Context(), r, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if r.Bytes != len(raw) || r.Digest != hex.EncodeToString(sum[:]) || r.ContentEncoding != "base64" {
		t.Fatal("binary identity mismatch")
	}
	w := SealedWork{Version: WorkerProtocolVersion, Request: req, View: view}
	w.Digest = sealedDigest(w)
	if !validWorkerRequest(w) || validateWorkerRendition(t.Context(), w, r) != nil {
		t.Fatal("valid sealed PNG rejected")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if !errors.Is(validateWorkerRendition(ctx, w, r), context.Canceled) {
		t.Fatal("cancellation after worker return was ignored")
	}
	r.Bytes++
	if validateWorkerRendition(t.Context(), w, r) == nil {
		t.Fatal("tampered binary size accepted")
	}
	w.Request.Full = true
	if validWorkerRequest(w) {
		t.Fatal("unqualified full PNG admitted")
	}
}

func TestPNGWireBoundsDecodedScanlines(t *testing.T) {
	chunk := func(kind string, data []byte) []byte {
		b := make([]byte, 12+len(data))
		binary.BigEndian.PutUint32(b, uint32(len(data)))
		copy(b[4:8], kind)
		copy(b[8:], data)
		binary.BigEndian.PutUint32(b[8+len(data):], crc32.ChecksumIEEE(b[4:8+len(data)]))
		return b
	}
	makeImage := func(rows []byte, tail []byte) []byte {
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		_, _ = w.Write(rows)
		_ = w.Close()
		header := make([]byte, 13)
		binary.BigEndian.PutUint32(header, 1)
		binary.BigEndian.PutUint32(header[4:], 1)
		header[8], header[9] = 8, 6
		b := []byte{137, 80, 78, 71, 13, 10, 26, 10}
		b = append(b, chunk("IHDR", header)...)
		b = append(b, chunk("IDAT", append(z.Bytes(), tail...))...)
		return append(b, chunk("IEND", nil)...)
	}
	if err := validatePNGBytes(t.Context(), makeImage([]byte{0, 1, 2, 3, 4}, nil), 1, 1); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"extra_scanline": makeImage(make([]byte, 10), nil), "short_scanline": makeImage(make([]byte, 4), nil), "invalid_filter": makeImage([]byte{5, 0, 0, 0, 0}, nil), "compressed_trailing": makeImage(make([]byte, 5), []byte{0})} {
		t.Run(name, func(t *testing.T) {
			if validatePNGBytes(t.Context(), raw, 1, 1) == nil {
				t.Fatal("invalid decoded stream accepted")
			}
		})
	}
}

func TestPNGManagedRetainedLifecycle(t *testing.T) {
	f := &fixtureViewer{value: tableView()}
	s, err := newTestService(f, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "table", Limit: 100}, Format: "png", Theme: "light", Width: 800, Height: 420}
	e := authority(t, "reporting.read", "reporting.export")
	first, err := s.Generate(t.Context(), e, req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Generate(t.Context(), e, req)
	if err != nil || again.ID != first.ID || again.Digest != first.Digest || again.Content != first.Content {
		t.Fatal("replay changed", err)
	}
	read, err := s.Read(t.Context(), e, ReadRequest{ID: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ContentBytes(t.Context(), read, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Export(t.Context(), authority(t, "reporting.read"), req); err == nil {
		t.Fatal("missing export authority admitted")
	}
	req.Full = true
	if _, err := s.Export(t.Context(), e, req); err == nil {
		t.Fatal("unsupported full report raster admitted")
	}
}

func TestPNGDisclosureSurvivesBinaryProjection(t *testing.T) {
	v := tableView()
	v.Output.AmountCompleteness = []reporting.AmountDisclosure{retainedAmount()}
	v.Output.Table.RowIndices = []int{2, 0}
	req := Request{Format: "png", Theme: "light", Width: 800, Height: 420}
	r, err := renderSealedContext(t.Context(), req, v, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Projection.AmountCoverage) != 1 || r.Projection.AmountCoverage[0].Status != "incomplete" || r.Projection.AmountCoverage[0].Evidence != "reviewed_definition" {
		t.Fatal("coverage identity lost")
	}
	raw, err := ContentBytes(t.Context(), r, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	footerInk := false
	for y := 340; y < 420 && !footerInk; y++ {
		for x := 0; x < 800; x++ {
			rr, gg, bb, _ := im.At(x, y).RGBA()
			if rr < 60000 || gg < 60000 || bb < 60000 {
				footerInk = true
				break
			}
		}
	}
	if !footerInk {
		t.Fatal("mandatory visible footer absent")
	}
	d := v.Output.AmountCompleteness
	v.Output.AmountCompleteness = nil
	plain, err := renderSealedContext(t.Context(), req, v, 2<<20)
	if err != nil || plain.Digest == r.Digest {
		t.Fatal("disclosure not rasterized", err)
	}
	v.Output.AmountCompleteness = d
	req.Width, req.Height = 320, 200
	v.Output.AmountCompleteness[0].Label = strings.Repeat("long reviewed label ", 10)
	if _, err := renderSealedContext(t.Context(), req, v, 2<<20); err == nil {
		t.Fatal("viewport silently dropped disclosure")
	}
	req.Width, req.Height = 800, 420
	v.Output.AmountCompleteness[0].Evidence = "forged"
	if _, err := renderSealedContext(t.Context(), req, v, 2<<20); err == nil {
		t.Fatal("forged disclosure admitted")
	}
}
