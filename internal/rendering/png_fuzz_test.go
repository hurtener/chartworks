package rendering

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func FuzzPNGClosedByteContract(f *testing.F) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		f.Fatal(err)
	}
	f.Add(b.Bytes())
	f.Add([]byte{})
	f.Add(append(append([]byte{}, b.Bytes()...), 0))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64<<10 {
			return
		}
		if validatePNGBytes(t.Context(), raw, 8, 8) == nil {
			decoded, err := png.Decode(bytes.NewReader(raw))
			if err != nil || decoded.Bounds() != image.Rect(0, 0, 8, 8) {
				t.Fatal("admitted PNG differs from bounded decoder", err)
			}
		}
	})
}
