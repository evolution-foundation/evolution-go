package send_service

// Sticker payload handling.
//
// `SendSticker` used to run every sticker through `convertToWebP`: fetch the URL, decode with
// `image.Decode` and re-encode with `webp.Encode(Quality: 80)`. That is wrong whenever the source
// is already a WebP — which is the case for every sticker that came from WhatsApp itself:
//
//  1. Animated stickers cannot be sent at all. The registered decoder (chai2010/webp) only reads
//     static WebP, so an animated file fails with `webpDecodeRGBA: failed` and the whole send dies.
//  2. The ones that do go through lose quality for nothing — a perfectly valid WebP is decoded and
//     re-compressed at 80%.
//
// So: if the downloaded bytes are already a valid WebP, they are uploaded untouched. Animation and
// quality survive, and the conversion path is left to the inputs that actually need it (PNG, JPEG).

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"

	"github.com/chai2010/webp"
)

const (
	// maxStickerBytes caps the sticker download. WhatsApp rejects stickers far smaller than this;
	// the limit exists so a hostile URL cannot exhaust the process memory — relevant now that the
	// payload is uploaded rather than decoded, so nothing downstream constrains its size either.
	maxStickerBytes = 10 << 20 // 10 MiB

	// stickerFetchTimeout bounds the download. The sticker URL is supplied by the API caller and
	// may point anywhere, so a server that accepts the connection and then stalls would otherwise
	// hold the goroutine open indefinitely.
	stickerFetchTimeout = 30 * time.Second
)

var stickerFetchClient = &http.Client{Timeout: stickerFetchTimeout}

// stickerWebP fetches the sticker URL and returns WebP bytes ready to upload.
//
// A source that is already a valid WebP is returned untouched; anything else is decoded and
// encoded to WebP as before.
func stickerWebP(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request for sticker URL: %v", err)
	}
	resp, err := stickerFetchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image from URL: %v", err)
	}
	defer resp.Body.Close()

	// Without this, an HTML error page is read as if it were image data: it fails later, deeper,
	// with a decode error that says nothing about the URL having answered 404.
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("sticker URL returned HTTP %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxStickerBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read image from URL: %v", err)
	}
	if len(raw) > maxStickerBytes {
		return nil, fmt.Errorf("sticker exceeds %d bytes", maxStickerBytes)
	}

	if isWebP(raw) {
		return raw, nil
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 80}); err != nil {
		return nil, fmt.Errorf("failed to encode image to WebP: %v", err)
	}
	return buf.Bytes(), nil
}

// isWebP reports whether b is a well-formed WebP container.
//
// It checks the RIFF magic AND that the declared payload size fits in the buffer. That second
// check matters specifically because of the passthrough above: the old conversion path rejected a
// truncated download for free (a truncated file fails to decode), whereas a partial body still
// carries valid RIFF/WEBP magic and would be uploaded as-is, reaching the recipient broken.
func isWebP(b []byte) bool {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return false
	}
	// The RIFF size field counts everything after it, i.e. len(file) - 8. Trailing padding is
	// tolerated (some encoders add it); a payload larger than what we hold means truncation.
	return int(binary.LittleEndian.Uint32(b[4:8]))+8 <= len(b)
}

// webpIsAnimated reports whether a WebP container declares animation.
//
// Only the extended format (VP8X) can be animated, and bit 0x02 of its flags byte is the
// declaration — the same ANIMATION_FLAG libwebp uses. Plain VP8/VP8L are static by definition.
func webpIsAnimated(b []byte) bool {
	if !isWebP(b) || len(b) < 21 || string(b[12:16]) != "VP8X" {
		return false
	}
	return b[20]&0x02 != 0
}
