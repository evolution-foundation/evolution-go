package send_service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestImageDimensions_PNGLandscape(t *testing.T) {
	w, h := imageDimensions(encodePNG(t, 800, 400))
	if w == nil || h == nil {
		t.Fatal("expected dimensions, got nil")
	}
	if *w != 800 || *h != 400 {
		t.Fatalf("expected 800x400, got %dx%d", *w, *h)
	}
}

func TestImageDimensions_JPEGPortrait(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1080, 1350))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}

	w, h := imageDimensions(buf.Bytes())
	if w == nil || h == nil {
		t.Fatal("expected dimensions, got nil")
	}
	if *w != 1080 || *h != 1350 {
		t.Fatalf("expected 1080x1350, got %dx%d", *w, *h)
	}
}

func TestImageDimensions_InvalidDataReturnsNil(t *testing.T) {
	w, h := imageDimensions([]byte("not an image"))
	if w != nil || h != nil {
		t.Fatal("expected nil dimensions for invalid data")
	}
}

// encodeJPEG builds a plain JPEG of the given size for testing.
func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}
	return buf.Bytes()
}

// withEXIFOrientation inserts a minimal big-endian EXIF APP1 segment carrying the
// given orientation right after the SOI marker of a JPEG.
func withEXIFOrientation(t *testing.T, jpg []byte, orientation uint16) []byte {
	t.Helper()
	if len(jpg) < 2 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Fatal("not a JPEG")
	}
	tiff := []byte{
		// header: big-endian TIFF, IFD0 at offset 8
		'M', 'M', 0x00, 0x2A, 0x00, 0x00, 0x00, 0x08,
		// one entry: Orientation (0x0112), SHORT, count 1, value
		0x00, 0x01,
		0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, byte(orientation >> 8), byte(orientation), 0x00, 0x00,
		// no next IFD
		0x00, 0x00, 0x00, 0x00,
	}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	size := len(payload) + 2
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte(size >> 8), byte(size)}
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

func TestJPEGOrientation(t *testing.T) {
	plain := encodeJPEG(t, 80, 40)
	if got := jpegOrientation(plain); got != 1 {
		t.Fatalf("JPEG without EXIF: expected 1, got %d", got)
	}
	if got := jpegOrientation(encodePNG(t, 80, 40)); got != 1 {
		t.Fatalf("PNG: expected 1, got %d", got)
	}
	for _, o := range []uint16{2, 3, 6, 8} {
		if got := jpegOrientation(withEXIFOrientation(t, plain, o)); got != int(o) {
			t.Fatalf("expected orientation %d, got %d", o, got)
		}
	}
}

func TestImageDimensions_EXIFRotatedJPEG(t *testing.T) {
	src := encodeJPEG(t, 800, 400)
	cases := []struct {
		orientation uint16
		w, h        uint32
	}{
		{1, 800, 400},
		{3, 800, 400}, // upside down: same size
		{6, 400, 800}, // stored sideways, displayed portrait
		{8, 400, 800},
	}
	for _, tc := range cases {
		w, h := imageDimensions(withEXIFOrientation(t, src, tc.orientation))
		if w == nil || h == nil {
			t.Fatalf("orientation %d: expected dimensions, got nil", tc.orientation)
		}
		if *w != tc.w || *h != tc.h {
			t.Fatalf("orientation %d: expected %dx%d, got %dx%d", tc.orientation, tc.w, tc.h, *w, *h)
		}
	}
}

func TestMakeJPEGThumbnail_FollowsEXIFOrientation(t *testing.T) {
	thumb := makeJPEGThumbnail(withEXIFOrientation(t, encodeJPEG(t, 800, 400), 6), 72)
	if thumb == nil {
		t.Fatal("expected a thumbnail, got nil")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("thumbnail is not a decodable image: %v", err)
	}
	// 72x36 from the stored landscape pixels, rotated to the displayed portrait.
	if cfg.Width != 36 || cfg.Height != 72 {
		t.Fatalf("expected a 36x72 portrait thumbnail, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestOrientRGBA_Rotate90Clockwise(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, red)
	src.Set(1, 0, blue)
	dst := orientRGBA(src, 6)
	if dst.Bounds().Dx() != 1 || dst.Bounds().Dy() != 2 {
		t.Fatalf("expected 1x2, got %v", dst.Bounds())
	}
	if dst.RGBAAt(0, 0) != red || dst.RGBAAt(0, 1) != blue {
		t.Fatalf("rotation 90 clockwise should put the left pixel on top, got %v / %v", dst.RGBAAt(0, 0), dst.RGBAAt(0, 1))
	}
}
