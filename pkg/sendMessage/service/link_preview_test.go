package send_service

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chai2010/webp"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestLinkPreviewTarget_PrefersExplicitURL(t *testing.T) {
	data := &LinkStruct{Text: "see https://first.example/a and https://second.example/b", Url: "https://second.example/b"}
	if got := linkPreviewTarget(data); got != "https://second.example/b" {
		t.Fatalf("expected the explicit url, got %q", got)
	}
}

func TestLinkPreviewTarget_FallsBackToFirstURLInText(t *testing.T) {
	data := &LinkStruct{Text: "see https://first.example/a and https://second.example/b"}
	if got := linkPreviewTarget(data); got != "https://first.example/a" {
		t.Fatalf("expected the first url of the text, got %q", got)
	}
}

func TestResolveLinkPreview_CallerValuesSkipTheFetch(t *testing.T) {
	data := &LinkStruct{Title: "Caller title", ImgUrl: "https://cdn.example/photo.jpg"}
	fetch := func(string) (string, string, string, error) {
		t.Fatal("the page must not be fetched when title and image were supplied")
		return "", "", "", nil
	}

	preview, err := resolveLinkPreview(data, "https://shop.example/p/1", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.Title != "Caller title" || preview.ImageURL != "https://cdn.example/photo.jpg" {
		t.Fatalf("caller values were not kept: %+v", preview)
	}
}

func TestResolveLinkPreview_FillsOnlyMissingFields(t *testing.T) {
	data := &LinkStruct{Title: "Caller title"}
	calls := 0
	fetch := func(url string) (string, string, string, error) {
		calls++
		return "Page title", "Page description", "https://cdn.example/og.jpg", nil
	}

	preview, err := resolveLinkPreview(data, "https://shop.example/p/1", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one fetch, got %d", calls)
	}
	if preview.Title != "Caller title" {
		t.Fatalf("caller title must win, got %q", preview.Title)
	}
	if preview.Description != "Page description" || preview.ImageURL != "https://cdn.example/og.jpg" {
		t.Fatalf("missing fields were not filled from the page: %+v", preview)
	}
}

func TestResolveLinkPreview_FetchErrorKeepsCallerValues(t *testing.T) {
	data := &LinkStruct{Description: "Caller description"}
	fetch := func(string) (string, string, string, error) {
		return "", "", "", errors.New("403 Forbidden")
	}

	preview, err := resolveLinkPreview(data, "https://shop.example/p/1", fetch)
	if err == nil {
		t.Fatal("expected the fetch error to be reported")
	}
	if preview.Description != "Caller description" {
		t.Fatalf("caller values must survive a failed fetch, got %+v", preview)
	}
}

func TestFetchLinkMetadata_SendsPreviewUserAgentAndReadsOpenGraph(t *testing.T) {
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Generic page title</title>
<meta property="og:title" content="Kit 6 Boxers Microfiber">
<meta property="og:description" content="Deal of the day">
<meta property="og:image" content="https://cdn.example/boxers.jpg">
</head><body></body></html>`))
	}))
	defer server.Close()

	title, description, imageURL, err := fetchLinkMetadata(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUserAgent != linkPreviewUserAgent {
		t.Fatalf("expected the link-preview user agent, got %q", gotUserAgent)
	}
	if title != "Kit 6 Boxers Microfiber" {
		t.Fatalf("og:title must win over <title>, got %q", title)
	}
	if description != "Deal of the day" || imageURL != "https://cdn.example/boxers.jpg" {
		t.Fatalf("unexpected metadata: %q %q", description, imageURL)
	}
}

// A bot wall answers 403 with its own <title>; that title must never become the preview card.
func TestFetchLinkMetadata_RejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><head><title>Access denied</title></head></html>`))
	}))
	defer server.Close()

	if _, _, _, err := fetchLinkMetadata(server.URL); err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
}

func TestFetchLinkMetadata_ResolvesRelativeImageURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><meta property="og:image" content="/img/product.jpg"></head></html>`))
	}))
	defer server.Close()

	_, _, imageURL, err := fetchLinkMetadata(server.URL + "/p/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if imageURL != server.URL+"/img/product.jpg" {
		t.Fatalf("expected an absolute image url, got %q", imageURL)
	}
}

func TestFetchLinkPreviewResource_RejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 2048))
	}))
	defer server.Close()

	if _, err := fetchLinkPreviewResource(server.URL, 1024); err == nil {
		t.Fatal("expected an error for a body above the limit")
	}
}

func TestFetchLinkPreviewResource_RejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := fetchLinkPreviewResource(server.URL, 1024); err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
}

func TestPrepareLinkPreviewImage_KeepsSizeAndEncodesJPEG(t *testing.T) {
	prepared, err := prepareLinkPreviewImage(encodePNG(t, 800, 400))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJPEG(t, prepared.HighQuality, 800, 400)
	if prepared.Width != 800 || prepared.Height != 400 {
		t.Fatalf("expected 800x400, got %dx%d", prepared.Width, prepared.Height)
	}
	assertJPEG(t, prepared.Inline, 72, 36)
}

// Storefront CDNs serve og:image as WebP; decoding relies on the webp decoder
// that this package registers in the image package.
func TestPrepareLinkPreviewImage_AcceptsWebP(t *testing.T) {
	img, _, err := image.Decode(bytes.NewReader(encodePNG(t, 640, 320)))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	webpData, err := webp.EncodeRGBA(img, 80)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}

	prepared, err := prepareLinkPreviewImage(webpData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJPEG(t, prepared.HighQuality, 640, 320)
}

func TestPrepareLinkPreviewImage_DownscalesLargeImages(t *testing.T) {
	prepared, err := prepareLinkPreviewImage(encodePNG(t, 3000, 1500))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJPEG(t, prepared.HighQuality, linkPreviewMaxSide, linkPreviewMaxSide/2)
	if prepared.Width != linkPreviewMaxSide || prepared.Height != linkPreviewMaxSide/2 {
		t.Fatalf("expected %dx%d, got %dx%d", linkPreviewMaxSide, linkPreviewMaxSide/2, prepared.Width, prepared.Height)
	}
}

// pngDeclaring returns a PNG whose header claims width x height pixels while the
// file itself stays tiny: the shape of a decompression bomb.
func pngDeclaring(t *testing.T, width, height uint32) []byte {
	t.Helper()
	data := encodePNG(t, 1, 1)
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestCheckLinkPreviewDimensions_RejectsDecompressionBomb(t *testing.T) {
	err := checkLinkPreviewDimensions(pngDeclaring(t, 20000, 20000))
	if err == nil || !strings.Contains(err.Error(), "20000x20000") {
		t.Fatalf("expected the oversized image to be rejected before decoding, got %v", err)
	}
}

func TestCheckLinkPreviewDimensions_AcceptsRegularPhoto(t *testing.T) {
	if err := checkLinkPreviewDimensions(encodePNG(t, 800, 400)); err != nil {
		t.Fatalf("expected a regular photo to pass, got %v", err)
	}
}

func TestPrepareLinkPreviewImage_RejectsDecompressionBomb(t *testing.T) {
	if _, err := prepareLinkPreviewImage(pngDeclaring(t, 20000, 20000)); err == nil {
		t.Fatal("expected the oversized image to be rejected")
	}
}

func TestPrepareLinkPreviewImage_RejectsNonImage(t *testing.T) {
	if _, err := prepareLinkPreviewImage([]byte("<html>captcha</html>")); err == nil {
		t.Fatal("expected an error for bytes that are not an image")
	}
}

func TestApplyLinkThumbnailUpload_SetsHighQualityFields(t *testing.T) {
	ext := &waE2E.ExtendedTextMessage{}
	uploaded := whatsmeow.UploadResponse{
		DirectPath:    "/v/t62.36244-24/thumb",
		MediaKey:      []byte("media-key"),
		FileSHA256:    []byte("sha256"),
		FileEncSHA256: []byte("enc-sha256"),
	}

	applyLinkThumbnailUpload(ext, uploaded, 1200, 630)

	if ext.GetThumbnailDirectPath() != "/v/t62.36244-24/thumb" {
		t.Fatalf("direct path not set: %q", ext.GetThumbnailDirectPath())
	}
	if string(ext.GetMediaKey()) != "media-key" || string(ext.GetThumbnailSHA256()) != "sha256" ||
		string(ext.GetThumbnailEncSHA256()) != "enc-sha256" {
		t.Fatal("upload hashes or media key not copied")
	}
	if ext.GetThumbnailWidth() != 1200 || ext.GetThumbnailHeight() != 630 {
		t.Fatalf("dimensions not set: %dx%d", ext.GetThumbnailWidth(), ext.GetThumbnailHeight())
	}
	if ext.GetMediaKeyTimestamp() == 0 {
		t.Fatal("media key timestamp not set")
	}
}

// assertJPEG decodes data as JPEG and checks its dimensions.
func assertJPEG(t *testing.T, data []byte, wantWidth, wantHeight int) {
	t.Helper()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a decodable image: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("expected jpeg, got %q", format)
	}
	if cfg.Width != wantWidth || cfg.Height != wantHeight {
		t.Fatalf("expected %dx%d, got %dx%d", wantWidth, wantHeight, cfg.Width, cfg.Height)
	}
}
