// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// pngBytes encodes a w×h opaque PNG (header is all [image.DecodeConfig] reads).
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{1, 2, 3, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gifBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), []color.Color{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ---- add_metadata -------------------------------------------------------

func TestAddMetadataMultiAndSingleKey(t *testing.T) {
	s := fixed("loc")
	// multi-key block that can see earlier metadata (size)
	s.AddMetadata(func(data []byte, meta Metadata) map[string]any {
		return map[string]any{"byte_count": len(data), "saw_size": meta.Size()}
	})
	// single-key form; nil return stores a null value
	s.AddMetadataKey("tag", func(data []byte, _ Metadata) any {
		if len(data) == 0 {
			return nil
		}
		return "nonempty"
	})
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, err := up.Upload(strings.NewReader("abcd"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Metadata.Int("byte_count") != 4 || f.Metadata.Int("saw_size") != 4 {
		t.Fatalf("multi-key extractor wrong: %v", f.Metadata)
	}
	if f.Metadata.String("tag") != "nonempty" {
		t.Fatalf("single-key extractor wrong: %v", f.Metadata["tag"])
	}
	// null branch
	f2, _ := up.Upload(strings.NewReader(""), nil)
	if v, ok := f2.Metadata["tag"]; !ok || v != nil {
		t.Fatalf("nil single-key should store null present, got %v ok=%v", v, ok)
	}
}

func TestExplicitOverrideBeatsExtractor(t *testing.T) {
	s := fixed("loc")
	s.AddMetadataKey("k", func([]byte, Metadata) any { return "extracted" })
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("x"), &UploadOptions{Metadata: Metadata{"k": "override"}})
	if f.Metadata.String("k") != "override" {
		t.Fatalf("explicit override must win, got %v", f.Metadata["k"])
	}
}

func TestMetadataStringAndInt(t *testing.T) {
	m := Metadata{"s": "hi", "i": 7, "i64": int64(9), "f": 3.0, "bad": []int{}}
	if m.String("s") != "hi" || m.String("bad") != "" || m.String("missing") != "" {
		t.Fatal("String accessor")
	}
	if m.Int("i") != 7 || m.Int("i64") != 9 || m.Int("f") != 3 || m.Int("bad") != 0 {
		t.Fatal("Int accessor")
	}
}

// ---- determine_mime_type ------------------------------------------------

func TestDetermineMIMEType(t *testing.T) {
	s := fixed("loc")
	s.Plugin(&DetermineMIMEType{}) // default ContentAnalyzer
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")

	// Differential oracle vs the gem's content analyzer: PNG signature sniffs
	// as image/png (agrees); text carries the stdlib charset suffix (documented
	// divergence from the gem's :file analyzer).
	f, _ := up.Upload(bytes.NewReader(pngBytes(t, 2, 2)), nil)
	if f.MimeType() != "image/png" {
		t.Fatalf("png mime: %q", f.MimeType())
	}
	f2, _ := up.Upload(strings.NewReader("hello world"), nil)
	if !strings.HasPrefix(f2.MimeType(), "text/plain") {
		t.Fatalf("text mime: %q", f2.MimeType())
	}
}

func TestDetermineMIMETypeCustomAnalyzer(t *testing.T) {
	s := fixed("loc")
	s.Plugin(&DetermineMIMEType{Analyzer: func([]byte) string { return "x/custom" }})
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("x"), nil)
	if f.MimeType() != "x/custom" {
		t.Fatalf("custom analyzer: %q", f.MimeType())
	}
}

func TestExtensionAnalyzer(t *testing.T) {
	// known extension resolves without touching content
	a := ExtensionAnalyzerFor("photo.png")
	if got := a([]byte("not an image")); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("ext .png: %q", got)
	}
	// unknown extension falls back to content sniffing
	b := ExtensionAnalyzerFor("file.unknownext")
	if got := b(pngBytes(t, 1, 1)); got != "image/png" {
		t.Fatalf("unknown ext fallback: %q", got)
	}
	// no extension falls back to content
	c := ExtensionAnalyzerFor("noext")
	if got := c([]byte("hello")); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("no ext fallback: %q", got)
	}
}

// ---- store_dimensions ---------------------------------------------------

func TestStoreDimensions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      []byte
		wantW     int
		wantH     int
		wantImage bool
	}{
		{"png", pngBytes(t, 12, 7), 12, 7, true},
		{"jpeg", jpegBytes(t, 5, 9), 5, 9, true},
		{"gif", gifBytes(t, 3, 4), 3, 4, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixed("loc")
			s.Plugin(&StoreDimensions{})
			s.Register("store", NewMemory())
			up, _ := s.Uploader("store")
			f, _ := up.Upload(bytes.NewReader(tc.data), nil)
			if f.Width() != tc.wantW || f.Height() != tc.wantH {
				t.Fatalf("dims %dx%d, want %dx%d", f.Width(), f.Height(), tc.wantW, tc.wantH)
			}
			d, ok := f.Dimensions()
			if !ok || d != [2]int{tc.wantW, tc.wantH} {
				t.Fatalf("Dimensions %v ok=%v", d, ok)
			}
		})
	}
}

func TestStoreDimensionsNonImage(t *testing.T) {
	s := fixed("loc")
	s.Plugin(&StoreDimensions{})
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("not an image"), nil)
	if _, ok := f.Metadata["width"]; !ok {
		t.Fatal("width key must be present")
	}
	if f.Metadata["width"] != nil || f.Metadata["height"] != nil {
		t.Fatalf("non-image dims must be null: %v", f.Metadata)
	}
	if f.Width() != 0 || f.Height() != 0 {
		t.Fatal("zero dims for non-image")
	}
	if d, ok := f.Dimensions(); ok {
		t.Fatalf("Dimensions must be absent, got %v", d)
	}
}

func TestStoreDimensionsCustomAnalyzerAndPartial(t *testing.T) {
	s := fixed("loc")
	s.Plugin(&StoreDimensions{Analyzer: func([]byte) (int, int, bool) { return 100, 200, true }})
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("x"), nil)
	if f.Width() != 100 || f.Height() != 200 {
		t.Fatalf("custom analyzer dims: %dx%d", f.Width(), f.Height())
	}
	// Dimensions with only width present
	delete(f.Metadata, "height")
	if _, ok := f.Dimensions(); ok {
		t.Fatal("missing height should make Dimensions absent")
	}
}

func TestImageDimensionsDirect(t *testing.T) {
	if w, h, ok := ImageDimensions(pngBytes(t, 8, 6)); !ok || w != 8 || h != 6 {
		t.Fatalf("ImageDimensions %dx%d ok=%v", w, h, ok)
	}
	if _, _, ok := ImageDimensions([]byte("nope")); ok {
		t.Fatal("non-image should not decode")
	}
}

// ---- signature ----------------------------------------------------------

func TestSignatureDifferential(t *testing.T) {
	// Oracles captured from the shrine gem (v3.8.0) for content "hello world".
	data := []byte("hello world")
	cases := []struct {
		algo   SignatureAlgorithm
		format SignatureFormat
		want   string
	}{
		{MD5, Hex, "5eb63bbbe01eeed093cb22bb8f5acdc3"},
		{MD5, Base64, "XrY7u+Ae7tCTyyK7j1rNww=="},
		{SHA1, Hex, "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed"},
		{SHA256, Hex, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"},
		{SHA256, Base64, "uU0nuZNNPgilLlLX2n2r+sSE7+N6U4DukIj3rOLvzek="},
		// crc32: the gem's digest is the decimal checksum string; hex/base64
		// encode the bytes of that string, none returns the string itself.
		{CRC32, None, "222957957"},
		{CRC32, Hex, "323232393537393537"},
		{CRC32, Base64, "MjIyOTU3OTU3"},
	}
	for _, c := range cases {
		got, err := Signature(data, c.algo, c.format)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.algo, c.format, err)
		}
		if got != c.want {
			t.Fatalf("%s/%s = %q, want %q", c.algo, c.format, got, c.want)
		}
	}
}

func TestSignatureRawAndVariants(t *testing.T) {
	data := []byte("hello world")
	// None (raw bytes) for a cryptographic hash: length check.
	raw, _ := Signature(data, SHA512, None)
	if len(raw) != 64 {
		t.Fatalf("sha512 raw len %d", len(raw))
	}
	if _, err := Signature(data, SHA384, Hex); err != nil {
		t.Fatal(err)
	}
	if _, err := Signature(data, SHA1, Base64); err != nil {
		t.Fatal(err)
	}
	if _, err := Signature(data, MD5, None); err != nil {
		t.Fatal(err)
	}
}

func TestSignatureErrors(t *testing.T) {
	if _, err := Signature(nil, "blake3", Hex); !errors.Is(err, ErrUnknownAlgorithm) {
		t.Fatalf("want ErrUnknownAlgorithm, got %v", err)
	}
	if _, err := Signature(nil, MD5, "base32"); !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("want ErrUnknownFormat, got %v", err)
	}
	// crc32 with a bad format still hits the format switch default
	if _, err := Signature(nil, CRC32, "weird"); !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("crc32 bad format: %v", err)
	}
}

func TestSignatureMetadataPlugin(t *testing.T) {
	s := fixed("loc")
	s.Plugin(&SignatureMetadata{Key: "md5", Algorithm: MD5}) // default format Hex
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("hello world"), nil)
	if f.Metadata.String("md5") != "5eb63bbbe01eeed093cb22bb8f5acdc3" {
		t.Fatalf("md5 metadata: %v", f.Metadata["md5"])
	}
	// unknown algorithm stores a null value rather than failing the upload
	s2 := fixed("loc")
	s2.Plugin(&SignatureMetadata{Key: "bad", Algorithm: "nope", Format: Hex})
	s2.Register("store", NewMemory())
	up2, _ := s2.Uploader("store")
	f2, _ := up2.Upload(strings.NewReader("x"), nil)
	if v, ok := f2.Metadata["bad"]; !ok || v != nil {
		t.Fatalf("bad algo should store null, got %v ok=%v", v, ok)
	}
}

// ---- refresh_metadata ---------------------------------------------------

func TestRefreshMetadata(t *testing.T) {
	s := fixed("loc")
	s.Plugin(&StoreDimensions{})
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(bytes.NewReader(pngBytes(t, 4, 5)), &UploadOptions{Filename: "a.png"})

	// Forge tampered metadata, then refresh from the real stored bytes.
	f.Metadata["size"] = int64(999999)
	f.Metadata["width"] = 1
	f.Metadata["mime_type"] = "text/lies"
	if err := f.RefreshMetadata(); err != nil {
		t.Fatal(err)
	}
	if f.Size() == 999999 || f.Width() != 4 || f.Height() != 5 {
		t.Fatalf("metadata not refreshed: %v", f.Metadata)
	}
	if f.MimeType() != "image/png" {
		t.Fatalf("mime not refreshed: %q", f.MimeType())
	}
	if f.Filename() != "a.png" { // filename preserved
		t.Fatalf("filename should be preserved: %q", f.Filename())
	}
}

func TestRefreshMetadataErrors(t *testing.T) {
	// unknown storage
	s := New()
	f := &UploadedFile{ID: "x", StorageKey: "ghost", Metadata: Metadata{}, shrine: s}
	if err := f.RefreshMetadata(); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("want unknown storage, got %v", err)
	}
	// download failure (bytes gone)
	s2 := New()
	s2.Register("store", NewMemory())
	f2 := &UploadedFile{ID: "absent", StorageKey: "store", Metadata: Metadata{}, shrine: s2}
	if err := f2.RefreshMetadata(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}
