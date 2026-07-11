// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"errors"
	"strings"
	"testing"
)

// ---- serialization: ColumnData / LoadColumn -----------------------------

func TestColumnDataRoundTrip(t *testing.T) {
	s := New()
	n := 0
	s.GenerateLocation = func(Metadata) string { n++; return "l" + string(rune('a'+n)) }
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")

	// no attachment => empty column (SQL NULL)
	if d, err := att.ColumnData(); err != nil || d != "" {
		t.Fatalf("empty column: %q %v", d, err)
	}

	// attach + derivatives, then serialise
	if err := att.Assign(strings.NewReader("main"), &UploadOptions{Filename: "m.txt", Location: "main-loc"}); err != nil {
		t.Fatal(err)
	}
	if err := att.AddDerivative("small", strings.NewReader("s"), &UploadOptions{Location: "small-loc"}); err != nil {
		t.Fatal(err)
	}
	data, err := att.ColumnData()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, `"derivatives"`) || !strings.Contains(data, "small-loc") {
		t.Fatalf("column data missing derivatives: %s", data)
	}

	// load into a fresh attacher and verify state
	att2, _ := s.Attacher("cache", "store")
	if err := att2.LoadColumn(data); err != nil {
		t.Fatal(err)
	}
	if att2.Changed() {
		t.Fatal("load must not mark changed")
	}
	if att2.Get().ID != "main-loc" || att2.Get().Filename() != "m.txt" {
		t.Fatalf("loaded main wrong: %+v", att2.Get())
	}
	if d := att2.Derivative("small"); d == nil || d.ID != "small-loc" {
		t.Fatalf("loaded derivative wrong: %+v", d)
	}

	// column with no derivatives omits the key
	att3, _ := s.Attacher("cache", "store")
	att3.Assign(strings.NewReader("x"), &UploadOptions{Location: "only"})
	d3, _ := att3.ColumnData()
	if strings.Contains(d3, "derivatives") {
		t.Fatalf("no-derivatives column should omit key: %s", d3)
	}
}

func TestColumnDataMarshalError(t *testing.T) {
	s := New()
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")
	att.file = &UploadedFile{ID: "x", StorageKey: "store", Metadata: Metadata{"bad": make(chan int)}, shrine: s}
	if _, err := att.ColumnData(); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestLoadColumnErrors(t *testing.T) {
	s := New()
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")

	// empty string clears
	att.file = &UploadedFile{ID: "x", StorageKey: "store", shrine: s}
	att.changed = true
	if err := att.LoadColumn(""); err != nil {
		t.Fatal(err)
	}
	if att.Get() != nil || att.Changed() {
		t.Fatal("empty load should clear file and changed")
	}

	// malformed json
	if err := att.LoadColumn("{bad"); err == nil {
		t.Fatal("expected json error")
	}
	// main file unknown storage
	if err := att.LoadColumn(`{"id":"x","storage":"ghost","metadata":{}}`); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("main unknown storage: %v", err)
	}
	// derivative unknown storage
	col := `{"id":"x","storage":"store","metadata":{},"derivatives":{"s":{"id":"y","storage":"ghost","metadata":{}}}}`
	if err := att.LoadColumn(col); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("derivative unknown storage: %v", err)
	}
}

// ---- model glue (activerecord / sequel) ---------------------------------

// fakeRecord is an in-memory [Record].
type fakeRecord struct {
	value   string
	present bool
}

func (r *fakeRecord) ReadAttachment(string) (string, bool) { return r.value, r.present }
func (r *fakeRecord) WriteAttachment(_ string, v string) {
	r.value = v
	r.present = v != ""
}

func newModelShrine(t *testing.T) *Shrine {
	t.Helper()
	s := New()
	n := 0
	s.GenerateLocation = func(Metadata) string { n++; return "m" + string(rune('a'+n)) }
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	return s
}

func TestModelAttacherLifecycle(t *testing.T) {
	s := newModelShrine(t)
	rec := &fakeRecord{}

	m, err := s.NewActiveRecord("cache", "store", rec, "avatar_data")
	if err != nil {
		t.Fatal(err)
	}
	// fresh record: nothing loaded
	if m.Get() != nil {
		t.Fatal("fresh record has no attachment")
	}

	// assign to cache, save promotes to store and writes the column
	if err := m.Assign(strings.NewReader("hello"), &UploadOptions{Filename: "h.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if !rec.present || !strings.Contains(rec.value, `"storage":"store"`) {
		t.Fatalf("column not written: %q", rec.value)
	}
	if m.Get().StorageKey != "store" {
		t.Fatalf("attachment should be stored, got %q", m.Get().StorageKey)
	}
	stored := m.Get()

	// save with no change is a no-op
	before := rec.value
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if rec.value != before {
		t.Fatal("no-change save should not rewrite column")
	}

	// re-load from the persisted column in a new attacher
	m2, err := s.NewSequel("cache", "store", rec, "avatar_data")
	if err != nil {
		t.Fatal(err)
	}
	if m2.Get() == nil || m2.Get().ID != stored.ID {
		t.Fatalf("reload mismatch: %+v", m2.Get())
	}

	// destroy deletes bytes and clears the column
	if err := m2.Destroy(); err != nil {
		t.Fatal(err)
	}
	if rec.present || rec.value != "" {
		t.Fatalf("destroy should clear column, got %q", rec.value)
	}
	if stored.Exists() {
		t.Fatal("destroy should delete stored bytes")
	}
}

func TestModelAttacherErrors(t *testing.T) {
	s := newModelShrine(t)

	// unknown storage
	if _, err := s.NewActiveRecord("cache", "ghost", &fakeRecord{}, "c"); !errors.Is(err, ErrUnknownStorage) {
		t.Fatalf("unknown storage: %v", err)
	}
	// load error from a malformed persisted column
	if _, err := s.NewSequel("cache", "store", &fakeRecord{value: "{bad", present: true}, "c"); err == nil {
		t.Fatal("expected load error on construct")
	}

	// Save finalize error: store write fails
	sf := New()
	sf.GenerateLocation = func(Metadata) string { return "g" }
	sf.Register("cache", NewMemory())
	fs := newFakeFS()
	sf.Register("store", NewFileSystemWithFS("/b", fs))
	mf, _ := sf.NewActiveRecord("cache", "store", &fakeRecord{}, "c")
	mf.Assign(strings.NewReader("x"), nil)
	fs.failWrite = true
	if err := mf.Save(); err == nil {
		t.Fatal("expected finalize error")
	}

	// Save ColumnData error: unserialisable metadata survives promotion
	sc := New()
	sc.GenerateLocation = func(Metadata) string { return "g" }
	sc.Register("cache", NewMemory())
	sc.Register("store", NewMemory())
	mc, _ := sc.NewActiveRecord("cache", "store", &fakeRecord{}, "c")
	if err := mc.Assign(strings.NewReader("x"), &UploadOptions{Metadata: Metadata{"bad": make(chan int)}}); err != nil {
		t.Fatal(err)
	}
	if err := mc.Save(); err == nil {
		t.Fatal("expected column-data marshal error on save")
	}
}

func TestModelDestroyErrors(t *testing.T) {
	// DeleteDerivatives error
	s := New()
	s.GenerateLocation = func(Metadata) string { return "g" }
	s.Register("cache", NewMemory())
	fs := newFakeFS()
	s.Register("store", NewFileSystemWithFS("/b", fs))
	m, _ := s.NewActiveRecord("cache", "store", &fakeRecord{}, "c")
	if err := m.AddDerivative("d", strings.NewReader("y"), &UploadOptions{Location: "dv"}); err != nil {
		t.Fatal(err)
	}
	fs.failRemove = true
	if err := m.Destroy(); err == nil {
		t.Fatal("expected delete-derivatives error")
	}

	// Attacher.Destroy error (file delete fails), no derivatives
	s2 := New()
	s2.GenerateLocation = func(Metadata) string { return "g" }
	fs2 := newFakeFS()
	s2.Register("cache", NewFileSystemWithFS("/b", fs2))
	s2.Register("store", NewMemory())
	m2, _ := s2.NewActiveRecord("cache", "store", &fakeRecord{}, "c")
	m2.Assign(strings.NewReader("x"), &UploadOptions{Location: "cf"})
	fs2.failRemove = true
	if err := m2.Destroy(); err == nil {
		t.Fatal("expected attacher destroy error")
	}
}
