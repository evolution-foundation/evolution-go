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
