// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- test doubles -------------------------------------------------------

// errReader fails on the first Read, driving the io.ReadAll error branches.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

// errWriter fails on the first Write, driving the Stream/io.Copy error branch.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write boom") }

// fakeFS is an in-memory FS that can be told to fail any operation, exercising
// the FileSystem storage's I/O error branches without touching disk.
type fakeFS struct {
	files                                      map[string][]byte
	failMkdir, failWrite, failRead, failRemove bool
}

func newFakeFS() *fakeFS { return &fakeFS{files: map[string][]byte{}} }

func (f *fakeFS) MkdirAll(string, os.FileMode) error {
	if f.failMkdir {
		return errors.New("mkdir boom")
	}
	return nil
}

func (f *fakeFS) WriteFile(path string, data []byte, _ os.FileMode) error {
	if f.failWrite {
		return errors.New("write boom")
	}
	f.files[path] = data
	return nil
}

func (f *fakeFS) ReadFile(path string) ([]byte, error) {
	if f.failRead {
		return nil, errors.New("read boom")
	}
	return f.files[path], nil
}

func (f *fakeFS) Remove(path string) error {
	if f.failRemove {
		return errors.New("remove boom")
	}
	delete(f.files, path)
	return nil
}

func (f *fakeFS) Exists(path string) bool {
	_, ok := f.files[path]
	return ok
}

// fixed builds a Shrine whose GenerateLocation is deterministic, so ids are
// predictable in assertions.
func fixed(loc string) *Shrine {
	s := New()
	s.GenerateLocation = func(Metadata) string { return loc }
	return s
}

// ---- Shrine registry & seams -------------------------------------------

func TestRegisterAndStorages(t *testing.T) {
	s := New()
	mem := NewMemory()
	s.Register("cache", mem)
	if s.Storages()["cache"] != mem {
		t.Fatal("storage not registered")
	}
}

func TestUploaderUnknownStorage(t *testing.T) {
	s := New()
	if _, err := s.Uploader("nope"); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("want ErrUnknownStorage, got %v", err)
	}
	up, err := s.Uploader("nope")
	if up != nil || err == nil {
		t.Fatal("expected nil uploader")
	}
}

func TestAttacherUnknownStorage(t *testing.T) {
	s := New()
	s.Register("cache", NewMemory())
	if _, err := s.Attacher("cache", "store"); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("store missing should error, got %v", err)
	}
	if _, err := s.Attacher("cache-missing", "cache"); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("cache missing should error, got %v", err)
	}
	if _, err := s.Attacher("cache", "cache"); err != nil {
		t.Fatalf("both present should succeed, got %v", err)
	}
}

// ---- plugin seam --------------------------------------------------------

type recordingPlugin struct{ configured bool }

func (p *recordingPlugin) Configure(*Shrine) { p.configured = true }

func TestPlugin(t *testing.T) {
	s := New()
	p := &recordingPlugin{}
	s.Plugin(p)
	if !p.configured {
		t.Fatal("Configure not invoked")
	}
	if len(s.Plugins()) != 1 || s.Plugins()[0] != p {
		t.Fatal("plugin not recorded")
	}
}

// ---- location & mime seams ---------------------------------------------

func TestDefaultGenerateLocation(t *testing.T) {
	// with an extension
	if got := defaultGenerateLocation(Metadata{"filename": "photo.JPG"}); !strings.HasSuffix(got, ".JPG") {
		t.Fatalf("want .JPG suffix, got %q", got)
	}
	// filename without extension
	if got := defaultGenerateLocation(Metadata{"filename": "noext"}); strings.Contains(got, ".") {
		t.Fatalf("unexpected extension in %q", got)
	}
	// no filename at all
	if got := defaultGenerateLocation(Metadata{}); len(got) != 32 {
		t.Fatalf("want 32 hex chars, got %q", got)
	}
}

func TestRandomHexPanicsOnEntropyFailure(t *testing.T) {
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	defer func() {
		randRead = orig
		if recover() == nil {
			t.Fatal("expected panic on entropy failure")
		}
	}()
	_ = randomHex(4)
}

func TestDetectMIMESeam(t *testing.T) {
	s := fixed("id")
	s.DetectMIME = func([]byte) string { return "application/x-custom" }
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, err := up.Upload(strings.NewReader("data"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.MimeType() != "application/x-custom" {
		t.Fatalf("mime seam not used: %q", f.MimeType())
	}
}

// ---- Uploader.Upload ----------------------------------------------------

func TestUploadHappyPathAndMetadata(t *testing.T) {
	s := fixed("loc.txt")
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	if up.StorageKey() != "store" {
		t.Fatalf("bad key %q", up.StorageKey())
	}
	f, err := up.Upload(strings.NewReader("hello world"), &UploadOptions{Filename: "greeting.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != "loc.txt" || f.StorageKey != "store" {
		t.Fatalf("bad file %+v", f)
	}
	if f.Filename() != "greeting.txt" || f.Size() != 11 {
		t.Fatalf("bad metadata %+v", f.Metadata)
	}
	if f.MimeType() == "" {
		t.Fatal("mime not detected")
	}
}

func TestUploadNilOptionsAndNullFilename(t *testing.T) {
	s := fixed("x")
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, err := up.Upload(strings.NewReader("body"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Filename() != "" {
		t.Fatalf("expected null filename, got %q", f.Filename())
	}
	if v, ok := f.Metadata["filename"]; !ok || v != nil {
		t.Fatalf("filename key should be present and nil, got %v ok=%v", v, ok)
	}
}

func TestUploadMetadataOverrides(t *testing.T) {
	s := fixed("x")
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, err := up.Upload(strings.NewReader("body"), &UploadOptions{
		Metadata: Metadata{"filename": "override.bin", "size": int64(999), "mime_type": "x/y"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.Filename() != "override.bin" || f.Size() != 999 || f.MimeType() != "x/y" {
		t.Fatalf("overrides ignored: %+v", f.Metadata)
	}
}

func TestUploadExplicitLocation(t *testing.T) {
	s := New() // default (random) generator must NOT be used
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, err := up.Upload(strings.NewReader("body"), &UploadOptions{Location: "fixed/here.dat"})
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != "fixed/here.dat" {
		t.Fatalf("explicit location ignored: %q", f.ID)
	}
}

func TestUploadReadError(t *testing.T) {
	s := fixed("x")
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	if _, err := up.Upload(errReader{}, nil); err == nil {
		t.Fatal("expected read error")
	}
}

func TestUploadStorageError(t *testing.T) {
	s := fixed("x")
	// filesystem storage whose write fails
	fs := newFakeFS()
	fs.failWrite = true
	s.Register("store", NewFileSystemWithFS("/base", fs))
	up, _ := s.Uploader("store")
	if _, err := up.Upload(strings.NewReader("body"), nil); err == nil {
		t.Fatal("expected storage upload error")
	}
}

// ---- UploadedFile JSON round-trip & accessors --------------------------

func TestJSONRoundTrip(t *testing.T) {
	s := fixed("abc123.txt")
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("hello"), &UploadOptions{Filename: "h.txt"})

	js, err := f.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	// gem key order: id, storage, metadata
	if !strings.HasPrefix(js, `{"id":"abc123.txt","storage":"store","metadata":{`) {
		t.Fatalf("unexpected JSON shape: %s", js)
	}

	// Data() (Ruby #data hash)
	d := f.Data()
	if d["id"] != "abc123.txt" || d["storage"] != "store" {
		t.Fatalf("bad Data(): %v", d)
	}

	// rehydrate
	f2, err := s.UploadedFile(js)
	if err != nil {
		t.Fatal(err)
	}
	if f2.ID != f.ID || f2.StorageKey != f.StorageKey {
		t.Fatalf("round-trip mismatch: %+v", f2)
	}
	if f2.Size() != 5 { // size survives JSON round-trip as int64
		t.Fatalf("size not normalized: %d", f2.Size())
	}
	if f2.Filename() != "h.txt" {
		t.Fatalf("filename lost: %q", f2.Filename())
	}
	body, err := f2.Download()
	if err != nil || string(body) != "hello" {
		t.Fatalf("download failed: %q %v", body, err)
	}
}

func TestToJSONError(t *testing.T) {
	f := &UploadedFile{ID: "x", StorageKey: "store", Metadata: Metadata{"bad": make(chan int)}}
	if _, err := f.ToJSON(); err == nil {
		t.Fatal("expected marshal error for unserializable metadata")
	}
}

func TestUploadedFileRehydrateErrors(t *testing.T) {
	s := New()
	if _, err := s.UploadedFile("{not json"); err == nil {
		t.Fatal("expected JSON error")
	}
	if _, err := s.UploadedFile(`{"id":"x","storage":"ghost","metadata":{}}`); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("want ErrUnknownStorage, got %v", err)
	}
}

func TestRehydrateNullSizeAndNoMetadata(t *testing.T) {
	s := New()
	s.Register("store", NewMemory())
	// size null -> not coerced; stays nil
	f, err := s.UploadedFile(`{"id":"x","storage":"store","metadata":{"size":null}}`)
	if err != nil {
		t.Fatal(err)
	}
	if f.Size() != 0 {
		t.Fatalf("null size should read 0, got %d", f.Size())
	}
	// absent metadata object -> empty Metadata
	f2, err := s.UploadedFile(`{"id":"x","storage":"store"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(f2.Metadata) != 0 {
		t.Fatalf("expected empty metadata, got %v", f2.Metadata)
	}
}

func TestMetadataAccessorsEmpty(t *testing.T) {
	m := Metadata{}
	if m.Filename() != "" || m.MimeType() != "" || m.Size() != 0 {
		t.Fatal("empty metadata accessors should be zero values")
	}
	// size as float64 (JSON) and default branch
	if (Metadata{"size": 42.0}).Size() != 42 {
		t.Fatal("float64 size not handled")
	}
	if (Metadata{"size": "notanumber"}).Size() != 0 {
		t.Fatal("bad size type should read 0")
	}
}

// ---- UploadedFile IO operations & storage resolution -------------------

func TestUploadedFileIOAndURL(t *testing.T) {
	s := fixed("k")
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("payload"), nil)

	if !f.Exists() {
		t.Fatal("should exist")
	}
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "payload" {
		t.Fatalf("bad open: %q", got)
	}
	var buf bytes.Buffer
	if err := f.Stream(&buf); err != nil || buf.String() != "payload" {
		t.Fatalf("bad stream: %q %v", buf.String(), err)
	}
	if f.URL(nil) != "memory://k" {
		t.Fatalf("bad url: %q", f.URL(nil))
	}
	if err := f.Delete(); err != nil {
		t.Fatal(err)
	}
	if f.Exists() {
		t.Fatal("should be gone after delete")
	}
}

func TestUploadedFileUnknownStorage(t *testing.T) {
	s := New() // storage "ghost" not registered
	f := &UploadedFile{ID: "k", StorageKey: "ghost", Metadata: Metadata{}, shrine: s}

	if _, err := f.Open(); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("Open: %v", err)
	}
	if _, err := f.Download(); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("Download: %v", err)
	}
	if err := f.Stream(io.Discard); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("Stream: %v", err)
	}
	if f.URL(nil) != "" {
		t.Fatalf("URL should be empty, got %q", f.URL(nil))
	}
	if f.Exists() {
		t.Fatal("Exists should be false")
	}
	if err := f.Delete(); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("Delete: %v", err)
	}
}

func TestUploadedFileOpenNotFound(t *testing.T) {
	s := New()
	s.Register("store", NewMemory())
	f := &UploadedFile{ID: "absent", StorageKey: "store", Metadata: Metadata{}, shrine: s}
	if _, err := f.Open(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := f.Download(); err == nil {
		t.Fatal("Download of absent should error")
	}
}

func TestStreamCopyError(t *testing.T) {
	s := New()
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("data"), &UploadOptions{Location: "l"})
	if err := f.Stream(errWriter{}); err == nil {
		t.Fatal("expected copy error")
	}
}

// ---- UploadedFile.Replace ----------------------------------------------

func TestReplace(t *testing.T) {
	s := fixed("new-loc")
	s.Register("store", NewMemory())
	up, _ := s.Uploader("store")
	old, _ := up.Upload(strings.NewReader("old"), &UploadOptions{Location: "old-loc"})

	repl, err := old.Replace(strings.NewReader("new"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if repl.ID != "new-loc" {
		t.Fatalf("bad replacement id %q", repl.ID)
	}
	if old.Exists() {
		t.Fatal("old file should be deleted")
	}
	body, _ := repl.Download()
	if string(body) != "new" {
		t.Fatalf("bad replacement body %q", body)
	}
}

func TestReplaceErrors(t *testing.T) {
	// unknown storage
	s := New()
	f := &UploadedFile{ID: "x", StorageKey: "ghost", Metadata: Metadata{}, shrine: s}
	if _, err := f.Replace(strings.NewReader("y"), nil); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("want unknown storage, got %v", err)
	}

	// upload error (read failure)
	s2 := New()
	s2.Register("store", NewMemory())
	up, _ := s2.Uploader("store")
	f2, _ := up.Upload(strings.NewReader("a"), &UploadOptions{Location: "l"})
	if _, err := f2.Replace(errReader{}, nil); err == nil {
		t.Fatal("expected upload error in replace")
	}

	// delete error after successful upload
	fs := newFakeFS()
	s3 := fixed("nl")
	s3.Register("store", NewFileSystemWithFS("/b", fs))
	up3, _ := s3.Uploader("store")
	f3, _ := up3.Upload(strings.NewReader("a"), &UploadOptions{Location: "ol"})
	fs.failRemove = true
	if _, err := f3.Replace(strings.NewReader("b"), nil); err == nil {
		t.Fatal("expected delete error in replace")
	}
}

// ---- Memory storage -----------------------------------------------------

func TestMemoryStorage(t *testing.T) {
	m := NewMemory()
	if err := m.Upload(strings.NewReader("x"), "id", nil); err != nil {
		t.Fatal(err)
	}
	if !m.Exists("id") {
		t.Fatal("should exist")
	}
	if _, err := m.Open("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := m.Upload(errReader{}, "id", nil); err == nil {
		t.Fatal("expected read error")
	}
	if err := m.Delete("id"); err != nil {
		t.Fatal(err)
	}
}

// ---- FileSystem storage (real OSFS + fake FS error branches) -----------

func TestFileSystemOSFS(t *testing.T) {
	dir := t.TempDir()
	s := NewFileSystem(dir)
	id := "sub/dir/file.txt"
	if err := s.Upload(strings.NewReader("disk-bytes"), id, nil); err != nil {
		t.Fatal(err)
	}
	if !s.Exists(id) {
		t.Fatal("should exist on disk")
	}
	// verify on-disk content directly
	onDisk, err := os.ReadFile(filepath.Join(dir, id))
	if err != nil || string(onDisk) != "disk-bytes" {
		t.Fatalf("bad on-disk content: %q %v", onDisk, err)
	}
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "disk-bytes" {
		t.Fatalf("bad open: %q", got)
	}
	// URL: default (path) and with prefix
	if s.URL(id, nil) != filepath.Join(dir, id) {
		t.Fatalf("bad path url: %q", s.URL(id, nil))
	}
	s.Prefix = "https://cdn.example"
	if s.URL(id, nil) != "https://cdn.example/"+id {
		t.Fatalf("bad prefix url: %q", s.URL(id, nil))
	}
	// not found
	if _, err := s.Open("no/such"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if s.Exists(id) {
		t.Fatal("should be deleted")
	}
	// OSFS.ReadFile error branch: exists reports true but read fails — remove
	// between checks is racy; instead cover via the fakeFS test below.
}

func TestFileSystemErrorBranches(t *testing.T) {
	// read (io.ReadAll) failure
	fs := newFakeFS()
	s := NewFileSystemWithFS("/base", fs)
	if err := s.Upload(errReader{}, "id", nil); err == nil {
		t.Fatal("expected read error")
	}
	// mkdir failure
	fs.failMkdir = true
	if err := s.Upload(strings.NewReader("x"), "id", nil); err == nil {
		t.Fatal("expected mkdir error")
	}
	fs.failMkdir = false
	// write failure
	fs.failWrite = true
	if err := s.Upload(strings.NewReader("x"), "id", nil); err == nil {
		t.Fatal("expected write error")
	}
	fs.failWrite = false
	// successful write, then ReadFile failure on Open
	if err := s.Upload(strings.NewReader("x"), "id", nil); err != nil {
		t.Fatal(err)
	}
	fs.failRead = true
	if _, err := s.Open("id"); err == nil {
		t.Fatal("expected ReadFile error")
	}
	fs.failRead = false
	// Remove failure
	fs.failRemove = true
	if err := s.Delete("id"); err == nil {
		t.Fatal("expected remove error")
	}
}

func TestOSFSReadFileErrorAndMkdirError(t *testing.T) {
	// Cover OSFS.ReadFile's error return: a path that Exists() sees but
	// os.ReadFile rejects — a directory reads as an error.
	dir := t.TempDir()
	fsys := OSFS{}
	if err := fsys.MkdirAll(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.ReadFile(filepath.Join(dir, "adir")); err == nil {
		t.Fatal("reading a directory should error")
	}
	// OSFS.WriteFile error: write into a non-existent parent.
	if err := fsys.WriteFile(filepath.Join(dir, "nope", "f"), []byte("x"), 0o644); err == nil {
		t.Fatal("write into missing parent should error")
	}
	// OSFS.Remove error: remove a non-existent path.
	if err := fsys.Remove(filepath.Join(dir, "ghost")); err == nil {
		t.Fatal("removing missing path should error")
	}
	// OSFS.Exists false branch.
	if fsys.Exists(filepath.Join(dir, "ghost")) {
		t.Fatal("ghost should not exist")
	}
}
