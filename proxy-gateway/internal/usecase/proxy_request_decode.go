package usecase

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
)

// normalizeCapturedBody adapts a raw on-the-wire capture into a form that
// renders sensibly in the dashboard. The client already received the original
// bytes — this only changes what we persist for display.
//
//   - Opaque binary content-types (gRPC, octet-stream, images, etc.) are
//     replaced with a short placeholder so users see "what" was sent rather
//     than UTF-8-mangled garbage.
//   - gzip / deflate payloads are decompressed so the user sees the same JSON
//     (or text) their client sees after the SDK decodes the response.
//
// truncated combines the original capture-truncation flag with any new
// truncation introduced while bounding the decompressed output.
func normalizeCapturedBody(raw []byte, contentEncoding, contentType string, cap int, captureTruncated bool) ([]byte, bool) {
	if len(raw) == 0 {
		return raw, captureTruncated
	}
	if isOpaqueBinaryType(contentType) {
		return []byte(fmt.Sprintf("<binary %s, %d bytes>", canonicalContentType(contentType), len(raw))), captureTruncated
	}
	enc := strings.ToLower(strings.TrimSpace(contentEncoding))
	if enc == "" || enc == "identity" {
		// Callers reuse a pooled buffer; the sink may marshal the payload
		// later, so we hand back an independent copy.
		out := make([]byte, len(raw))
		copy(out, raw)
		return out, captureTruncated
	}
	decoded, decodeTrunc, ok := decompressCapped(raw, enc, cap)
	if !ok {
		// Decoder failed (corrupt or truncated compressed stream). A raw
		// dump of compressed bytes would just look like binary noise, so
		// substitute a placeholder that at least identifies the situation.
		hint := "decode failed"
		if captureTruncated {
			hint = "truncated capture"
		}
		return []byte(fmt.Sprintf("<%s body, %s, %d bytes>", enc, hint, len(raw))), captureTruncated
	}
	return decoded, captureTruncated || decodeTrunc
}

// decompressCapped streams raw through a gzip / deflate decoder and returns
// at most cap bytes of decoded output. The third return value reports decoder
// success; the second reports whether the decoded output hit the cap (or the
// decoder errored part-way through, in which case we keep what we already
// have rather than dropping a partial result).
func decompressCapped(raw []byte, encoding string, cap int) ([]byte, bool, bool) {
	r, ok := newDecoder(raw, encoding)
	if !ok {
		return nil, false, false
	}
	defer r.Close()

	if cap <= 0 {
		// Defensive: callers always pass a positive cap, but never let an
		// adversarial payload trigger an unbounded allocation.
		cap = 1 << 20
	}
	// Read one byte past cap so we can detect "exactly at cap" vs "would have
	// produced more".
	limited := io.LimitReader(r, int64(cap)+1)
	out := bytes.NewBuffer(make([]byte, 0, min(cap, len(raw)*4)))
	_, err := io.Copy(out, limited)
	hitCap := out.Len() > cap
	if hitCap {
		out.Truncate(cap)
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		// Partial decode: keep what we have if any, otherwise fail.
		if out.Len() == 0 {
			return nil, false, false
		}
		return out.Bytes(), true, true
	}
	return out.Bytes(), hitCap, true
}

func newDecoder(raw []byte, encoding string) (io.ReadCloser, bool) {
	switch encoding {
	case "gzip", "x-gzip":
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, false
		}
		return gz, true
	case "deflate":
		// RFC 7230 says deflate is zlib-wrapped; many servers ship raw flate.
		// Try zlib first (cheap header sniff), fall back to raw flate.
		if zr, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
			return zr, true
		}
		return flate.NewReader(bytes.NewReader(raw)), true
	}
	return nil, false
}

// isOpaqueBinaryType returns true for content-types whose payload is not
// text and cannot be meaningfully rendered as a string in the dashboard.
func isOpaqueBinaryType(ct string) bool {
	ct = canonicalContentType(ct)
	switch {
	case strings.HasPrefix(ct, "application/grpc"):
		return true
	case ct == "application/octet-stream":
		return true
	case ct == "application/x-protobuf",
		ct == "application/protobuf",
		ct == "application/vnd.google.protobuf":
		return true
	case ct == "application/pdf":
		return true
	case ct == "application/zip",
		ct == "application/x-zip-compressed",
		ct == "application/x-tar",
		ct == "application/gzip",
		ct == "application/x-gzip",
		ct == "application/x-bzip2":
		return true
	case strings.HasPrefix(ct, "image/"),
		strings.HasPrefix(ct, "audio/"),
		strings.HasPrefix(ct, "video/"),
		strings.HasPrefix(ct, "font/"):
		return true
	}
	return false
}

// canonicalContentType lowercases and strips the parameter portion of a
// Content-Type header so the switches above can match plain MIME types.
func canonicalContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}
