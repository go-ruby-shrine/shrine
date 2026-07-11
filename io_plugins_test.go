// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- data_uri -----------------------------------------------------------

func TestDataURIDifferential(t *testing.T) {
	// Oracles from the shrine gem (data_uri plugin).
	cases := []struct {
		uri      string
		wantMime string
		wantData string
	}{
		{"data:text/plain,hello%20world", "text/plain", "hello world"},
		{"data:text/plain;base64,aGVsbG8=", "text/plain", "hello"},
		{"data:;base64,YWJj", "text/plain", "abc"},
		{"data:,justtext", "text/plain", "justtext"},
		{"data:image/png;base64,aGk=", "image/png", "hi"},
		{"data:,a+b", "text/plain", "a b"},   // '+' decodes to space
		{"data:,a%2Bb", "text/plain", "a+b"}, // %2B decodes to '+'
		{"data:text/plain;charset=utf-8,hi", "text/plain;charset=utf-8", "hi"},
		{"data:text/plain;charset=utf-8;base64,aGk=", "text/plain;charset=utf-8", "hi"},
	}
	for _, c := range cases {
		mt, data, err := DataURI(c.uri)
		if err != nil {
			t.Fatalf("%q: %v", c.uri, err)
		}
		if mt != c.wantMime || string(data) != c.wantData {
			t.Fatalf("%q => %q %q, want %q %q", c.uri, mt, data, c.wantMime, c.wantData)
		}
	}
}

func TestDataURIErrors(t *testing.T) {
	for _, uri := range []string{
		"notadatauri",            // missing scheme
		"data:text/plain",        // missing comma
		"data:;base64,not_b64!!", // bad base64
		"data:,bad%zz",           // bad percent escape
	} {
		if _, _, err := DataURI(uri); !errors.Is(err, ErrInvalidDataURI) {
			t.Fatalf("%q should be ErrInvalidDataURI, got %v", uri, err)
		}
	}
}

func TestAssignDataURI(t *testing.T) {
	s := fixed("loc")
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")

	if err := att.AssignDataURI("data:image/png;base64,aGk=", nil); err != nil {
		t.Fatal(err)
	}
	f := att.Get()
	if f == nil || f.StorageKey != "cache" {
		t.Fatal("data uri should attach to cache")
	}
	if f.MimeType() != "image/png" { // media type seeds mime_type, not re-sniffed
		t.Fatalf("mime_type from data uri: %q", f.MimeType())
	}
	body, _ := f.Download()
	if string(body) != "hi" {
		t.Fatalf("body: %q", body)
	}

	// caller-supplied mime_type override is respected (not clobbered)
	att2, _ := s.Attacher("cache", "store")
	if err := att2.AssignDataURI("data:image/png;base64,aGk=", &UploadOptions{Metadata: Metadata{"mime_type": "x/keep"}}); err != nil {
		t.Fatal(err)
	}
	if att2.Get().MimeType() != "x/keep" {
		t.Fatalf("override mime lost: %q", att2.Get().MimeType())
	}

	// bad uri
	if err := att.AssignDataURI("nope", nil); !errors.Is(err, ErrInvalidDataURI) {
		t.Fatalf("want ErrInvalidDataURI, got %v", err)
	}
}

func TestAssignDataURIUploadError(t *testing.T) {
	s := fixed("loc")
	fs := newFakeFS()
	fs.failWrite = true
	s.Register("cache", NewFileSystemWithFS("/b", fs))
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")
	if err := att.AssignDataURI("data:,hi", nil); err == nil {
		t.Fatal("expected upload error")
	}
}

// ---- remote_url ---------------------------------------------------------

// rtFunc adapts a function to an http.RoundTripper so HTTPDownloader can be
// exercised without opening a socket.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func canned(status int, body string) *http.Client {
	return &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})}
}

func TestHTTPDownloader(t *testing.T) {
	dl := HTTPDownloader(canned(200, "remote-bytes"))
	rc, name, err := dl("http://example.test/dir/photo.jpg?x=1#frag")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "remote-bytes" {
		t.Fatalf("body: %q", data)
	}
	if name != "photo.jpg" { // query/fragment stripped
		t.Fatalf("filename: %q", name)
	}

	// non-2xx is an error
	if _, _, err := HTTPDownloader(canned(404, "nope"))("http://x/y"); err == nil {
		t.Fatal("404 should error")
	}
	// transport error
	failClient := &http.Client{Transport: rtFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial boom")
	})}
	if _, _, err := HTTPDownloader(failClient)("http://x/y"); err == nil {
		t.Fatal("transport error should propagate")
	}
	// URL with no basename yields empty filename
	_, name2, _ := HTTPDownloader(canned(200, "x"))("http://host/")
	if name2 != "" {
		t.Fatalf("expected empty filename, got %q", name2)
	}
	// default client path (nil) is constructed without panicking
	_ = HTTPDownloader(nil)
}

func TestRemoteURLAssign(t *testing.T) {
	s := fixed("loc")
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")

	r := &RemoteURL{Download: HTTPDownloader(canned(200, "downloaded"))}
	if err := r.AssignTo(att, "http://host/pic.png", nil); err != nil {
		t.Fatal(err)
	}
	f := att.Get()
	if f == nil || f.StorageKey != "cache" {
		t.Fatal("remote url should attach to cache")
	}
	if f.Filename() != "pic.png" { // downloaded filename seeds metadata
		t.Fatalf("filename: %q", f.Filename())
	}
	body, _ := f.Download()
	if string(body) != "downloaded" {
		t.Fatalf("body: %q", body)
	}

	// caller-supplied filename wins over the downloaded one
	att2, _ := s.Attacher("cache", "store")
	if err := r.AssignTo(att2, "http://host/pic.png", &UploadOptions{Filename: "keep.png"}); err != nil {
		t.Fatal(err)
	}
	if att2.Get().Filename() != "keep.png" {
		t.Fatalf("provided filename lost: %q", att2.Get().Filename())
	}
}

func TestRemoteURLErrors(t *testing.T) {
	s := fixed("loc")
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")

	// download error
	r := &RemoteURL{Download: func(string) (io.ReadCloser, string, error) {
		return nil, "", errors.New("down boom")
	}}
	if err := r.AssignTo(att, "http://x", nil); !errors.Is(err, ErrRemoteURLDownload) {
		t.Fatalf("want ErrRemoteURLDownload, got %v", err)
	}

	// too large
	rBig := &RemoteURL{
		Download: func(string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader("way too many bytes")), "f", nil
		},
		MaxSize: 4,
	}
	if err := rBig.AssignTo(att, "http://x", nil); !errors.Is(err, ErrRemoteURLTooLarge) {
		t.Fatalf("want ErrRemoteURLTooLarge, got %v", err)
	}
	// exactly at the limit passes
	rOk := &RemoteURL{
		Download: func(string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader("1234")), "f", nil
		},
		MaxSize: 4,
	}
	if err := rOk.AssignTo(att, "http://x", nil); err != nil {
		t.Fatalf("at-limit body should pass: %v", err)
	}

	// read error mid-stream
	rRead := &RemoteURL{Download: func(string) (io.ReadCloser, string, error) {
		return io.NopCloser(errReader{}), "f", nil
	}}
	if err := rRead.AssignTo(att, "http://x", nil); err == nil {
		t.Fatal("expected read error")
	}

	// upload error
	s2 := fixed("loc")
	fs := newFakeFS()
	fs.failWrite = true
	s2.Register("cache", NewFileSystemWithFS("/b", fs))
	s2.Register("store", NewMemory())
	att2, _ := s2.Attacher("cache", "store")
	rUp := &RemoteURL{Download: func(string) (io.ReadCloser, string, error) {
		return io.NopCloser(strings.NewReader("x")), "f", nil
	}}
	if err := rUp.AssignTo(att2, "http://x", nil); err == nil {
		t.Fatal("expected upload error")
	}
}

// ---- upload_endpoint ----------------------------------------------------

// multipartFile builds a multipart body carrying one file field.
func multipartFile(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(content))
	w.Close()
	return &buf, w.FormDataContentType()
}

func TestUploadEndpoint(t *testing.T) {
	s := fixed("up.txt")
	s.Register("cache", NewMemory())
	e := &UploadEndpoint{Shrine: s, StorageKey: "cache"}

	body, ct := multipartFile(t, "file", "photo.txt", "endpoint-bytes")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"storage":"cache"`) {
		t.Fatalf("body: %s", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: %q", ct)
	}
	// the file really landed in the storage
	if !s.Storages()["cache"].Exists("up.txt") {
		t.Fatal("file not stored")
	}
}

func TestUploadEndpointCustomField(t *testing.T) {
	s := fixed("up")
	s.Register("cache", NewMemory())
	e := &UploadEndpoint{Shrine: s, StorageKey: "cache", FieldName: "avatar", MaxMemory: 1 << 20}
	body, ct := multipartFile(t, "avatar", "a.bin", "x")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("custom field status %d", rec.Code)
	}
}

func TestUploadEndpointErrors(t *testing.T) {
	s := fixed("up")
	s.Register("cache", NewMemory())

	// 405 non-POST
	e := &UploadEndpoint{Shrine: s, StorageKey: "cache"}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/upload", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}

	// 400 invalid multipart body
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad multipart, got %d", rec.Code)
	}

	// 400 missing file field
	body, ct := multipartFile(t, "other", "f", "x")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing file, got %d", rec.Code)
	}

	// 500 unknown storage
	eBad := &UploadEndpoint{Shrine: s, StorageKey: "ghost"}
	body, ct = multipartFile(t, "file", "f", "x")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	eBad.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 unknown storage, got %d", rec.Code)
	}

	// 500 upload failure (storage write fails)
	fs := newFakeFS()
	fs.failWrite = true
	sFail := fixed("up")
	sFail.Register("cache", NewFileSystemWithFS("/b", fs))
	eFail := &UploadEndpoint{Shrine: sFail, StorageKey: "cache"}
	body, ct = multipartFile(t, "file", "f", "x")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	eFail.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 upload failure, got %d", rec.Code)
	}
}

// ---- presign_endpoint ---------------------------------------------------

// fakePresigner is a Presigner that echoes the id, or fails when told to.
type fakePresigner struct {
	fail bool
	seen string
}

func (p *fakePresigner) Presign(id string, _ map[string]any) (PresignData, error) {
	if p.fail {
		return PresignData{}, errors.New("presign boom")
	}
	p.seen = id
	return PresignData{
		Method:  "PUT",
		URL:     "https://bucket.example/" + id,
		Fields:  map[string]string{"key": id},
		Headers: map[string]string{"Content-Type": "application/octet-stream"},
	}, nil
}

func TestPresignEndpoint(t *testing.T) {
	p := &fakePresigner{}
	e := &PresignEndpoint{Presigner: p, GenerateID: func() string { return "fixed-id" }}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/presign", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if p.seen != "fixed-id" {
		t.Fatalf("presigner saw %q", p.seen)
	}
	if !strings.Contains(rec.Body.String(), `"method":"PUT"`) ||
		!strings.Contains(rec.Body.String(), "fixed-id") {
		t.Fatalf("body: %s", rec.Body.String())
	}

	// default GenerateID (random) still succeeds and produces a non-empty id
	p2 := &fakePresigner{}
	e2 := &PresignEndpoint{Presigner: p2}
	rec = httptest.NewRecorder()
	e2.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/presign", nil))
	if rec.Code != http.StatusOK || p2.seen == "" {
		t.Fatalf("default id: code=%d seen=%q", rec.Code, p2.seen)
	}
}

func TestPresignEndpointErrors(t *testing.T) {
	// 405 non-GET
	e := &PresignEndpoint{Presigner: &fakePresigner{}}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/presign", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
	// 500 presign failure
	eFail := &PresignEndpoint{Presigner: &fakePresigner{fail: true}, GenerateID: func() string { return "x" }}
	rec = httptest.NewRecorder()
	eFail.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/presign", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

func TestWriteJSONMarshalError(t *testing.T) {
	// Directly exercise the encoder's failure branch with an unserialisable value.
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]any{"bad": make(chan int)})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 on marshal error, got %d", rec.Code)
	}
}
