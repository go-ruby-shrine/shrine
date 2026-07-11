// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// ---- pretty_location ----------------------------------------------------

func TestPrettyLocation(t *testing.T) {
	s := New()
	s.GenerateLocation = func(m Metadata) string { // deterministic basename
		if strings.HasSuffix(m.Filename(), ".jpg") {
			return "base.jpg"
		}
		return "base"
	}
	// full context => namespace/id/name/basename.ext (gem shape)
	got := s.PrettyLocation(LocationContext{
		Namespace: "photo", Identifier: "42", Name: "image",
		Metadata: Metadata{"filename": "me.jpg"},
	})
	if got != "photo/42/image/base.jpg" {
		t.Fatalf("full context: %q", got)
	}
	// no context => just the basename
	if got := s.PrettyLocation(LocationContext{Metadata: Metadata{"filename": "me.jpg"}}); got != "base.jpg" {
		t.Fatalf("no context: %q", got)
	}
	// no filename => basename without extension, context still applied
	if got := s.PrettyLocation(LocationContext{Namespace: "photo", Identifier: "42", Name: "image"}); got != "photo/42/image/base" {
		t.Fatalf("no filename: %q", got)
	}
}

// ---- default_storage ----------------------------------------------------

func TestDefaultStorage(t *testing.T) {
	s := New()
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	s.Plugin(&DefaultStorage{Cache: "cache", Store: "store"})

	att, err := s.DefaultAttacher()
	if err != nil {
		t.Fatal(err)
	}
	if att.cache.key != "cache" || att.store.key != "store" {
		t.Fatalf("default attacher wrong storages: %s/%s", att.cache.key, att.store.key)
	}
	up, err := s.DefaultUploader()
	if err != nil || up.key != "store" {
		t.Fatalf("default uploader: %v %v", up, err)
	}
}

func TestDefaultStorageUnset(t *testing.T) {
	s := New() // no default_storage plugin
	if _, err := s.DefaultAttacher(); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("unset default attacher: %v", err)
	}
	if _, err := s.DefaultUploader(); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("unset default uploader: %v", err)
	}
}

// ---- derivatives --------------------------------------------------------

func newDerivAttacher(t *testing.T) (*Shrine, *Attacher) {
	t.Helper()
	s := New()
	n := 0
	s.GenerateLocation = func(Metadata) string { n++; return "d" + string(rune('a'+n)) }
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, err := s.Attacher("cache", "store")
	if err != nil {
		t.Fatal(err)
	}
	return s, att
}

func TestDerivativesCreateAndAccess(t *testing.T) {
	s, att := newDerivAttacher(t)
	if err := att.Assign(strings.NewReader("original"), nil); err != nil {
		t.Fatal(err)
	}
	if err := att.Promote(); err != nil { // move to store
		t.Fatal(err)
	}
	err := att.CreateDerivatives(func(orig []byte) (map[string]io.Reader, error) {
		return map[string]io.Reader{
			"small": strings.NewReader("small:" + string(orig)),
			"large": strings.NewReader("large:" + string(orig)),
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(att.Derivatives()) != 2 {
		t.Fatalf("want 2 derivatives, got %d", len(att.Derivatives()))
	}
	small := att.Derivative("small")
	if small == nil {
		t.Fatal("small derivative missing")
	}
	body, _ := small.Download()
	if string(body) != "small:original" {
		t.Fatalf("small content: %q", body)
	}
	if small.StorageKey != "store" {
		t.Fatalf("derivative should be in store, got %q", small.StorageKey)
	}
	if !s.Storages()["store"].Exists(small.ID) {
		t.Fatal("derivative not stored")
	}
	if att.DerivativeURL("small", nil) != "memory://"+small.ID {
		t.Fatalf("derivative url: %q", att.DerivativeURL("small", nil))
	}
	if att.DerivativeURL("missing", nil) != "" {
		t.Fatal("missing derivative url should be empty")
	}
	if att.Derivative("missing") != nil {
		t.Fatal("missing derivative should be nil")
	}

	// delete
	if err := att.DeleteDerivatives(); err != nil {
		t.Fatal(err)
	}
	if len(att.Derivatives()) != 0 {
		t.Fatal("derivatives not cleared")
	}
	if small.Exists() {
		t.Fatal("derivative bytes should be gone")
	}
}

func TestDerivativesNoAttachment(t *testing.T) {
	_, att := newDerivAttacher(t)
	// no file attached: create is a no-op
	if err := att.CreateDerivatives(func([]byte) (map[string]io.Reader, error) {
		t.Fatal("deriver must not run with no attachment")
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDerivativesErrors(t *testing.T) {
	// deriver returns an error
	_, att := newDerivAttacher(t)
	att.Assign(strings.NewReader("x"), nil)
	att.Promote()
	boom := errors.New("boom")
	if err := att.CreateDerivatives(func([]byte) (map[string]io.Reader, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("deriver error should propagate: %v", err)
	}

	// download error: attached file whose bytes are gone
	s2, att2 := newDerivAttacher(t)
	att2.Assign(strings.NewReader("x"), nil)
	_ = s2.Storages()["cache"].Delete(att2.Get().ID)
	if err := att2.CreateDerivatives(func([]byte) (map[string]io.Reader, error) { return nil, nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("download error should propagate: %v", err)
	}

	// AddDerivative upload error (store write fails)
	s3 := New()
	s3.GenerateLocation = func(Metadata) string { return "g" }
	s3.Register("cache", NewMemory())
	fs := newFakeFS()
	fs.failWrite = true
	s3.Register("store", NewFileSystemWithFS("/b", fs))
	att3, _ := s3.Attacher("cache", "store")
	if err := att3.AddDerivative("x", strings.NewReader("y"), nil); err == nil {
		t.Fatal("expected AddDerivative upload error")
	}
	// and via CreateDerivatives
	att3b, _ := s3.Attacher("cache", "store")
	att3b.file = &UploadedFile{ID: "c", StorageKey: "cache", Metadata: Metadata{}, shrine: s3}
	_ = s3.Storages()["cache"].Upload(strings.NewReader("orig"), "c", nil)
	if err := att3b.CreateDerivatives(func([]byte) (map[string]io.Reader, error) {
		return map[string]io.Reader{"a": strings.NewReader("b")}, nil
	}); err == nil {
		t.Fatal("expected CreateDerivatives upload error")
	}
}

func TestDeleteDerivativesError(t *testing.T) {
	s := New()
	s.GenerateLocation = func(Metadata) string { return "g" }
	s.Register("cache", NewMemory())
	fs := newFakeFS()
	s.Register("store", NewFileSystemWithFS("/b", fs))
	att, _ := s.Attacher("cache", "store")
	if err := att.AddDerivative("x", strings.NewReader("y"), &UploadOptions{Location: "dv"}); err != nil {
		t.Fatal(err)
	}
	fs.failRemove = true
	if err := att.DeleteDerivatives(); err == nil {
		t.Fatal("expected delete derivative error")
	}
}

// ---- cached_attachment_data / restore_cached_data -----------------------

func TestCachedData(t *testing.T) {
	s := fixed("c.txt")
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")

	// nothing attached
	if d, err := att.CachedData(); err != nil || d != "" {
		t.Fatalf("empty cached data: %q %v", d, err)
	}
	// cached file yields its JSON
	if err := att.Assign(strings.NewReader("hi"), &UploadOptions{Filename: "h.txt"}); err != nil {
		t.Fatal(err)
	}
	data, err := att.CachedData()
	if err != nil || !strings.Contains(data, `"storage":"cache"`) {
		t.Fatalf("cached data: %q %v", data, err)
	}
	// once promoted (stored), cached_data is empty
	if err := att.Promote(); err != nil {
		t.Fatal(err)
	}
	if d, _ := att.CachedData(); d != "" {
		t.Fatalf("stored file should have empty cached data, got %q", d)
	}
}

func TestCachedDataToJSONError(t *testing.T) {
	s := New()
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")
	att.file = &UploadedFile{ID: "x", StorageKey: "cache", Metadata: Metadata{"bad": make(chan int)}, shrine: s}
	if _, err := att.CachedData(); err == nil {
		t.Fatal("expected ToJSON marshal error")
	}
}

func TestSetCached(t *testing.T) {
	s := fixed("c.txt")
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")
	up, _ := s.Uploader("cache")
	f, _ := up.Upload(strings.NewReader("payload"), &UploadOptions{Filename: "p.txt"})
	js, _ := f.ToJSON()

	if err := att.SetCached(js); err != nil {
		t.Fatal(err)
	}
	if !att.Changed() || att.Get().StorageKey != "cache" {
		t.Fatal("SetCached should attach the cached file")
	}
	// bad json
	if err := att.SetCached("{bad"); err == nil {
		t.Fatal("expected json error")
	}
	// not-cached (store) file rejected
	up2, _ := s.Uploader("store")
	sf, _ := up2.Upload(strings.NewReader("x"), &UploadOptions{Location: "sl"})
	sjs, _ := sf.ToJSON()
	if err := att.SetCached(sjs); !errors.Is(err, ErrNotCached) {
		t.Fatalf("want ErrNotCached, got %v", err)
	}
}

func TestRestoreCachedData(t *testing.T) {
	s := fixed("c.txt")
	s.Plugin(&StoreDimensions{})
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")

	// upload a real image to cache, then hand-forge tampered metadata in the JSON
	up, _ := s.Uploader("cache")
	img := pngBytes(t, 6, 9)
	f, _ := up.Upload(bytes.NewReader(img), &UploadOptions{Filename: "i.png"})
	// tamper: claim tiny size and wrong dimensions
	f.Metadata["size"] = int64(1)
	f.Metadata["width"] = 1
	js, _ := f.ToJSON()

	if err := att.RestoreCachedData(js); err != nil {
		t.Fatal(err)
	}
	got := att.Get()
	if got.Size() == 1 || got.Width() != 6 || got.Height() != 9 {
		t.Fatalf("metadata not restored from bytes: %v", got.Metadata)
	}

	// bad json
	if err := att.RestoreCachedData("{bad"); err == nil {
		t.Fatal("expected json error")
	}
	// not cached
	up2, _ := s.Uploader("store")
	sf, _ := up2.Upload(strings.NewReader("x"), &UploadOptions{Location: "sl"})
	sjs, _ := sf.ToJSON()
	if err := att.RestoreCachedData(sjs); !errors.Is(err, ErrNotCached) {
		t.Fatalf("want ErrNotCached, got %v", err)
	}
	// refresh error: cached file whose bytes are gone
	up3, _ := s.Uploader("cache")
	gf, _ := up3.Upload(strings.NewReader("x"), &UploadOptions{Location: "gone"})
	gjs, _ := gf.ToJSON()
	_ = s.Storages()["cache"].Delete("gone")
	if err := att.RestoreCachedData(gjs); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want refresh not-found, got %v", err)
	}
}
