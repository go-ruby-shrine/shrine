// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fileWith builds a bound UploadedFile carrying the given metadata.
func fileWith(meta Metadata) *UploadedFile {
	s := New()
	s.Register("store", NewMemory())
	return &UploadedFile{ID: "id", StorageKey: "store", Metadata: meta, shrine: s}
}

func TestValidationSizeMessages(t *testing.T) {
	// Oracles from the gem (validation_helpers): the message reports the *limit*
	// in base-1024 human units.
	f := fileWith(Metadata{"size": int64(3)})
	v := NewValidation(f)
	if v.MaxSize(2) {
		t.Fatal("3 > 2 should fail")
	}
	if got := v.Errors[0]; got != "size must not be greater than 2.0 B" {
		t.Fatalf("max_size msg: %q", got)
	}
	if !v.MaxSize(10) {
		t.Fatal("3 <= 10 should pass")
	}

	f2 := fileWith(Metadata{"size": int64(3)})
	v2 := NewValidation(f2)
	if v2.MinSize(10) {
		t.Fatal("3 < 10 should fail")
	}
	if got := v2.Errors[0]; got != "size must not be less than 10.0 B" {
		t.Fatalf("min_size msg: %q", got)
	}
	if !v2.MinSize(2) {
		t.Fatal("3 >= 2 should pass")
	}
}

func TestHumanBytes(t *testing.T) {
	// Oracles from the gem for several magnitudes.
	cases := map[int64]string{
		0:          "0.0 B",
		500:        "500.0 B",
		1536:       "1.5 KB",
		5242880:    "5.0 MB",
		1073741824: "1.0 GB",
		2500000000: "2.3 GB",
		6000000000: "5.6 GB",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestValidationExtension(t *testing.T) {
	// case-insensitive match passes
	v := NewValidation(fileWith(Metadata{"filename": "PHOTO.JPG"}))
	if !v.Extension([]string{"jpg", "png"}) {
		t.Fatalf("case-insensitive extension should pass: %v", v.Errors)
	}
	// mismatch fails with the gem's message
	v2 := NewValidation(fileWith(Metadata{"filename": "doc.txt"}))
	if v2.Extension([]string{"jpg", "png"}) {
		t.Fatal("txt should fail")
	}
	if v2.Errors[0] != "extension must be one of: jpg, png" {
		t.Fatalf("extension msg: %q", v2.Errors[0])
	}
	// null filename (no extension) fails
	v3 := NewValidation(fileWith(Metadata{"filename": nil}))
	if v3.Extension([]string{"jpg"}) {
		t.Fatal("null filename should fail extension")
	}
}

func TestValidationMimeType(t *testing.T) {
	// exact match passes
	v := NewValidation(fileWith(Metadata{"mime_type": "image/jpeg"}))
	if !v.MimeType([]string{"image/jpeg", "image/png"}) {
		t.Fatal("exact mime should pass")
	}
	// case-sensitive: IMAGE/JPEG must NOT match image/jpeg (gem behaviour)
	v2 := NewValidation(fileWith(Metadata{"mime_type": "IMAGE/JPEG"}))
	if v2.MimeType([]string{"image/jpeg"}) {
		t.Fatal("mime is case-sensitive; should fail")
	}
	if v2.Errors[0] != "type must be one of: image/jpeg" {
		t.Fatalf("mime msg: %q", v2.Errors[0])
	}
	// null mime fails
	v3 := NewValidation(fileWith(Metadata{"mime_type": nil}))
	if v3.MimeType([]string{"image/jpeg"}) {
		t.Fatal("null mime should fail")
	}
}

func TestValidationDimensions(t *testing.T) {
	f := fileWith(Metadata{"width": 50, "height": 80})

	v := NewValidation(f)
	if ok, err := v.MaxWidth(10); err != nil || ok {
		t.Fatalf("50 > 10 should fail: ok=%v err=%v", ok, err)
	}
	if v.Errors[0] != "width must not be greater than 10px" {
		t.Fatalf("max_width msg: %q", v.Errors[0])
	}
	if ok, _ := NewValidation(f).MinWidth(10); !ok {
		t.Fatal("50 >= 10 should pass min_width")
	}
	if ok, _ := NewValidation(f).MinWidth(100); ok {
		t.Fatal("50 < 100 should fail min_width")
	}
	if ok, _ := NewValidation(f).MaxHeight(10); ok {
		t.Fatal("80 > 10 should fail max_height")
	}
	if ok, _ := NewValidation(f).MinHeight(10); !ok {
		t.Fatal("80 >= 10 should pass min_height")
	}
	if ok, _ := NewValidation(f).MinHeight(200); ok {
		t.Fatal("80 < 200 should fail min_height")
	}
	if ok, _ := NewValidation(f).MaxWidth(100); !ok {
		t.Fatal("50 <= 100 should pass max_width")
	}
	if ok, _ := NewValidation(f).MaxHeight(100); !ok {
		t.Fatal("80 <= 100 should pass max_height")
	}

	// combined dimensions
	vd := NewValidation(f)
	if ok, _ := vd.MaxDimensions([2]int{10, 10}); ok {
		t.Fatal("should fail max_dimensions")
	}
	if vd.Errors[0] != "dimensions must not be greater than 10x10" {
		t.Fatalf("max_dimensions msg: %q", vd.Errors[0])
	}
	if ok, _ := NewValidation(f).MaxDimensions([2]int{100, 100}); !ok {
		t.Fatal("should pass max_dimensions")
	}
	vmn := NewValidation(f)
	if ok, _ := vmn.MinDimensions([2]int{100, 100}); ok {
		t.Fatal("should fail min_dimensions")
	}
	if vmn.Errors[0] != "dimensions must not be less than 100x100" {
		t.Fatalf("min_dimensions msg: %q", vmn.Errors[0])
	}
	if ok, _ := NewValidation(f).MinDimensions([2]int{1, 1}); !ok {
		t.Fatal("should pass min_dimensions")
	}
}

func TestValidationMissingDimension(t *testing.T) {
	f := fileWith(Metadata{}) // no width/height
	v := NewValidation(f)
	if _, err := v.MaxWidth(10); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MaxWidth missing: %v", err)
	}
	if _, err := v.MinWidth(10); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MinWidth missing: %v", err)
	}
	if _, err := v.MaxHeight(10); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MaxHeight missing: %v", err)
	}
	if _, err := v.MinHeight(10); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MinHeight missing: %v", err)
	}
	if _, err := v.MaxDimensions([2]int{1, 1}); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MaxDimensions missing width: %v", err)
	}
	if _, err := v.MinDimensions([2]int{1, 1}); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MinDimensions missing width: %v", err)
	}
	// height missing but width present exercises bothDimensions' second branch
	fw := fileWith(Metadata{"width": 5})
	if _, err := NewValidation(fw).MaxDimensions([2]int{1, 1}); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MaxDimensions missing height: %v", err)
	}
	if _, err := NewValidation(fw).MinDimensions([2]int{1, 1}); !errors.Is(err, ErrMissingDimension) {
		t.Fatalf("MinDimensions missing height: %v", err)
	}
}

// ---- wired into the Attacher -------------------------------------------

func TestAttacherValidationSeam(t *testing.T) {
	s := fixed("loc.txt")
	s.Register("cache", NewMemory())
	s.Register("store", NewMemory())
	att, _ := s.Attacher("cache", "store")
	att.Validate = func(f *UploadedFile) []string {
		v := NewValidation(f)
		v.MaxSize(3)
		v.Extension([]string{"png"})
		return v.Errors
	}

	// invalid: too big and wrong extension
	if err := att.Assign(strings.NewReader("way too long"), &UploadOptions{Filename: "f.txt"}); err != nil {
		t.Fatal(err)
	}
	if att.Valid() {
		t.Fatal("should be invalid")
	}
	if len(att.Errors) != 2 {
		t.Fatalf("want 2 errors, got %v", att.Errors)
	}

	// valid attachment clears errors
	if err := att.Assign(strings.NewReader("ok"), &UploadOptions{Filename: "f.png"}); err != nil {
		t.Fatal(err)
	}
	if !att.Valid() || att.Errors != nil {
		t.Fatalf("should be valid with no errors, got %v", att.Errors)
	}

	// detaching clears errors without validating
	att.Set(nil)
	if !att.Valid() {
		t.Fatal("detach should be valid")
	}
}

func TestValidationErrorsSliceShape(t *testing.T) {
	// A passing validation yields a nil (not empty-but-present) slice.
	v := NewValidation(fileWith(Metadata{"size": int64(1)}))
	v.MaxSize(10)
	if !reflect.DeepEqual(v.Errors, []string(nil)) {
		t.Fatalf("passing validation should leave Errors nil, got %#v", v.Errors)
	}
}
