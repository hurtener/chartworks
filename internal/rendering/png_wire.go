package rendering

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"io"
)

// renditionContent validates both encoded and decoded byte budgets. PNG uses a
// canonical base64 representation; historical text formats retain raw UTF-8.
func renditionContent(ctx context.Context, r Rendition, maxBytes int) ([]byte, error) {
	if ctx == nil || maxBytes < 1 || maxBytes > 64<<20 || len(r.Content) > maxBytes {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.Format != "png" {
		if r.ContentEncoding != "" {
			return nil, ErrInvalid
		}
		return []byte(r.Content), nil
	}
	if r.ContentEncoding != "base64" || r.MediaType != "image/png" || base64.StdEncoding.DecodedLen(len(r.Content)) > maxBytes {
		return nil, ErrInvalid
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(r.Content)
	if err != nil || len(raw) > maxBytes || base64.StdEncoding.EncodeToString(raw) != r.Content {
		return nil, ErrInvalid
	}
	if err = validatePNGBytes(ctx, raw, r.Width, r.Height); err != nil {
		return nil, err
	}
	return raw, nil
}

// validatePNGBytes admits only the exact inert subset produced by image/png for
// our RGBA canvas. CRC and bounded scanline decompression are checked without
// allocating a second decoded image in the supervising service.
func validatePNGBytes(ctx context.Context, raw []byte, width, height int) error {
	if ctx == nil || width < 1 || height < 1 || width > 4096 || height > 4096 || int64(width)*int64(height) > maxRasterPixels || len(raw) < 8 || !bytes.Equal(raw[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return ErrInvalid
	}
	cursor, chunks, bpp := 8, 0, 0
	seenHeader, seenData, seenEnd := false, false, false
	stream := &pngDataReader{}
	for cursor < len(raw) {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunks++
		if chunks > 4096 || len(raw)-cursor < 12 {
			return ErrInvalid
		}
		size := binary.BigEndian.Uint32(raw[cursor : cursor+4])
		if uint64(size) > uint64(len(raw)-cursor-12) {
			return ErrInvalid
		}
		n := int(size)
		typ := string(raw[cursor+4 : cursor+8])
		body := raw[cursor+8 : cursor+8+n]
		sum := binary.BigEndian.Uint32(raw[cursor+8+n : cursor+12+n])
		if crc32.ChecksumIEEE(raw[cursor+4:cursor+8+n]) != sum {
			return ErrInvalid
		}
		cursor += 12 + n
		switch typ {
		case "IHDR":
			if seenHeader || chunks != 1 || n != 13 || int64(binary.BigEndian.Uint32(body[:4])) != int64(width) || int64(binary.BigEndian.Uint32(body[4:8])) != int64(height) || body[8] != 8 || body[10] != 0 || body[11] != 0 || body[12] != 0 {
				return ErrInvalid
			}
			switch body[9] {
			case 2:
				bpp = 3
			case 6:
				bpp = 4
			default:
				return ErrInvalid
			}
			seenHeader = true
		case "IDAT":
			if !seenHeader || seenEnd {
				return ErrInvalid
			}
			seenData = true
			stream.chunks = append(stream.chunks, body)
			stream.remaining += len(body)
		case "IEND":
			if !seenHeader || !seenData || seenEnd || n != 0 || cursor != len(raw) {
				return ErrInvalid
			}
			seenEnd = true
		default:
			return ErrInvalid
		}
	}
	if !seenEnd || stream.remaining == 0 {
		return ErrInvalid
	}
	z, err := zlib.NewReader(stream)
	if err != nil {
		return ErrInvalid
	}
	defer z.Close()
	row := make([]byte, 1+width*bpp)
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := io.ReadFull(z, row); err != nil || row[0] > 4 {
			return ErrInvalid
		}
	}
	var extra [1]byte
	n, err := io.ReadFull(z, extra[:])
	if n != 0 || err != io.EOF || stream.remaining != 0 {
		return ErrInvalid
	}
	return nil
}

type pngDataReader struct {
	chunks                   [][]byte
	index, offset, remaining int
}

func (r *pngDataReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for r.index < len(r.chunks) && r.offset == len(r.chunks[r.index]) {
		r.index++
		r.offset = 0
	}
	if r.index == len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.index][r.offset:])
	r.offset += n
	r.remaining -= n
	return n, nil
}
func (r *pngDataReader) ReadByte() (byte, error) {
	var one [1]byte
	_, err := r.Read(one[:])
	return one[0], err
}

// ContentBytes validates and decodes the bounded rendition content for clients
// serving the declared media type. Digest and Bytes describe these decoded bytes.
func ContentBytes(ctx context.Context, r Rendition, maxBytes int) ([]byte, error) {
	media := map[string]string{"json": "application/json", "csv": "text/csv; charset=utf-8", "html": "text/html; charset=utf-8", "svg": "image/svg+xml", "png": "image/png"}[r.Format]
	if media == "" || r.MediaType != media {
		return nil, ErrInvalid
	}
	raw, err := renditionContent(ctx, r, maxBytes)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if r.Bytes != len(raw) || r.Digest != hex.EncodeToString(sum[:]) {
		return nil, ErrInvalid
	}
	return raw, nil
}
