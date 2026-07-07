// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"strings"
	"testing"
)

// newAttacher wires a deterministic cache→store attacher over two memory
// storages, with a counter-driven location generator so cache and store ids
// differ.
func newAttacher(t *testing.T) (*Shrine, *Attacher) {
	t.Helper()
	s := New()
	n := 0
	s.GenerateLocation = func(Metadata) string {
		n++
		return "loc-" + string(rune('a'+n))
	}
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, err := s.Attacher("cache", "store")
	if err != nil {
		t.Fatal(err)
	}
	return s, att
}

func TestAttacherAssignPromoteDestroy(t *testing.T) {
	s, att := newAttacher(t)

	if att.Changed() {
		t.Fatal("fresh attacher must not be changed")
	}
	if att.Get() != nil {
		t.Fatal("fresh attacher has no file")
	}

	// assign -> lands in cache, marked changed
	if err := att.Assign(strings.NewReader("payload"), &UploadOptions{Filename: "f.txt"}); err != nil {
		t.Fatal(err)
	}
	if !att.Changed() {
		t.Fatal("assign must mark changed")
	}
	cached := att.Get()
	if cached.StorageKey != "cache" {
		t.Fatalf("assigned file should be in cache, got %q", cached.StorageKey)
	}
	if !s.Storages()["cache"].Exists(cached.ID) {
		t.Fatal("cache must hold the file")
	}

	// finalize -> promote to store, clears changed
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}
	if att.Changed() {
		t.Fatal("finalize must clear changed")
	}
	stored := att.Get()
	if stored.StorageKey != "store" {
		t.Fatalf("finalized file should be in store, got %q", stored.StorageKey)
	}
	body, _ := stored.Download()
	if string(body) != "payload" {
		t.Fatalf("promoted content mismatch: %q", body)
	}

	// finalize again is a no-op (not changed)
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}

	// destroy -> removes stored file
	if err := att.Destroy(); err != nil {
		t.Fatal(err)
	}
	if stored.Exists() {
		t.Fatal("destroy must delete the stored file")
	}
}

func TestAttacherReplaceOnFinalize(t *testing.T) {
	s, att := newAttacher(t)

	// first attachment, finalized to store
	if err := att.Assign(strings.NewReader("v1"), nil); err != nil {
		t.Fatal(err)
	}
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}
	first := att.Get()

	// second attachment; finalize must delete the first (replacement)
	if err := att.Assign(strings.NewReader("v2"), nil); err != nil {
		t.Fatal(err)
	}
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}
	if first.Exists() {
		t.Fatal("previously stored file must be deleted on replacement")
	}
	second := att.Get()
	if !s.Storages()["store"].Exists(second.ID) {
		t.Fatal("new file must be stored")
	}
}

func TestAttacherSetAlreadyStored(t *testing.T) {
	s, att := newAttacher(t)
	// upload directly to store, then Set it: Finalize must NOT re-promote
	// (already in store) but must still commit.
	up, _ := s.Uploader("store")
	f, _ := up.Upload(strings.NewReader("direct"), &UploadOptions{Location: "direct-loc"})
	att.Set(f)
	if !att.Changed() {
		t.Fatal("Set must mark changed")
	}
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}
	if att.Get().StorageKey != "store" || att.Get().ID != "direct-loc" {
		t.Fatalf("stored file should be unchanged: %+v", att.Get())
	}
	if att.Changed() {
		t.Fatal("finalize should clear changed")
	}
}

func TestAttacherSetNilDetach(t *testing.T) {
	s, att := newAttacher(t)
	// finalize a file first so there is an original to delete on detach
	if err := att.Assign(strings.NewReader("x"), nil); err != nil {
		t.Fatal(err)
	}
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}
	original := att.Get()

	// detach: Set(nil), finalize deletes the previous stored file
	att.Set(nil)
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}
	if att.Get() != nil {
		t.Fatal("detach should clear the file")
	}
	if original.Exists() {
		t.Fatal("previous file should be deleted on detach")
	}
	// destroy with nil file is a no-op
	if err := att.Destroy(); err != nil {
		t.Fatal(err)
	}
	_ = s
}

func TestAttacherErrorBranches(t *testing.T) {
	// Assign upload error (read failure)
	_, att := newAttacher(t)
	if err := att.Assign(errReader{}, nil); err == nil {
		t.Fatal("expected assign upload error")
	}

	// Promote with nothing attached is a no-op
	_, att2 := newAttacher(t)
	if err := att2.Promote(); err != nil {
		t.Fatal(err)
	}

	// Promote open error: cached file whose storage cannot open it.
	// Assign to cache, then delete the underlying cache bytes.
	s3, att3 := newAttacher(t)
	if err := att3.Assign(strings.NewReader("x"), nil); err != nil {
		t.Fatal(err)
	}
	_ = s3.Storages()["cache"].Delete(att3.Get().ID)
	if err := att3.Promote(); err == nil {
		t.Fatal("expected promote open error")
	}

	// Finalize promote error propagates
	s4 := New()
	s4.GenerateLocation = func(Metadata) string { return "l" }
	s4.Register("cache", NewMemory())
	// store storage whose upload fails
	fs := newFakeFS()
	fs.failWrite = true
	s4.Register("store", NewFileSystemWithFS("/b", fs))
	att4, _ := s4.Attacher("cache", "store")
	if err := att4.Assign(strings.NewReader("x"), nil); err != nil {
		t.Fatal(err)
	}
	if err := att4.Finalize(); err == nil {
		t.Fatal("expected finalize promote error")
	}
}

func TestAttacherFinalizeDeletePreviousError(t *testing.T) {
	// previous stored file whose Delete fails during replacement
	s := New()
	s.GenerateLocation = func(Metadata) string { return "gen" }
	s.Register("cache", NewMemory())
	fs := newFakeFS()
	s.Register("store", NewFileSystemWithFS("/b", fs))
	att, _ := s.Attacher("cache", "store")

	if err := att.Assign(strings.NewReader("v1"), &UploadOptions{Location: "c1"}); err != nil {
		t.Fatal(err)
	}
	if err := att.Finalize(); err != nil {
		t.Fatal(err)
	}
	// second round; make the previous store file's Delete fail
	if err := att.Assign(strings.NewReader("v2"), &UploadOptions{Location: "c2"}); err != nil {
		t.Fatal(err)
	}
	fs.failRemove = true
	if err := att.Finalize(); err == nil {
		t.Fatal("expected delete-previous error on finalize")
	}
}

func TestAttacherDestroyDeleteError(t *testing.T) {
	s := New()
	s.GenerateLocation = func(Metadata) string { return "g" }
	fs := newFakeFS()
	s.Register("cache", NewFileSystemWithFS("/b", fs))
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")
	if err := att.Assign(strings.NewReader("x"), &UploadOptions{Location: "c"}); err != nil {
		t.Fatal(err)
	}
	fs.failRemove = true
	if err := att.Destroy(); err == nil {
		t.Fatal("expected destroy delete error")
	}
}
