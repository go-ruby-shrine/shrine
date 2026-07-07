// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"encoding/json"
	"fmt"
	"io"
)

// UploadedFile is the value returned by an upload and the reference persisted in
// place of a file, mirroring Shrine::UploadedFile. It carries the storage id,
// the storage name and the extracted [Metadata], and knows how to reach its
// bytes through the bound [Shrine]'s registry.
type UploadedFile struct {
	// ID is the storage location (Shrine's "id").
	ID string
	// StorageKey is the name of the storage holding the file (Shrine's
	// "storage").
	StorageKey string
	// Metadata is the extracted metadata hash.
	Metadata Metadata

	shrine *Shrine
}

// ufJSON is the wire shape of an UploadedFile: `{"id","storage","metadata"}`,
// in the gem's key order.
type ufJSON struct {
	ID       string         `json:"id"`
	Storage  string         `json:"storage"`
	Metadata map[string]any `json:"metadata"`
}

// Data returns the plain representation of the file — the Ruby Hash that
// `Shrine::UploadedFile#data` returns: {"id" => …, "storage" => …,
// "metadata" => {…}}.
func (f *UploadedFile) Data() map[string]any {
	return map[string]any{
		"id":       f.ID,
		"storage":  f.StorageKey,
		"metadata": map[string]any(f.Metadata),
	}
}

// MarshalJSON encodes the file in the gem's `{"id","storage","metadata"}` shape.
func (f *UploadedFile) MarshalJSON() ([]byte, error) {
	return json.Marshal(ufJSON{
		ID:       f.ID,
		Storage:  f.StorageKey,
		Metadata: map[string]any(f.Metadata),
	})
}

// ToJSON returns the JSON representation, mirroring
// `Shrine::UploadedFile#to_json`.
func (f *UploadedFile) ToJSON() (string, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Filename returns the "filename" metadata.
func (f *UploadedFile) Filename() string { return f.Metadata.Filename() }

// Size returns the "size" metadata in bytes.
func (f *UploadedFile) Size() int64 { return f.Metadata.Size() }

// MimeType returns the "mime_type" metadata.
func (f *UploadedFile) MimeType() string { return f.Metadata.MimeType() }

// storage resolves the [Storage] the file lives in, or a wrapped
// [ErrUnknownStorage].
func (f *UploadedFile) storage() (Storage, error) {
	st, ok := f.shrine.storages[f.StorageKey]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownStorage, f.StorageKey)
	}
	return st, nil
}

// Open returns a reader over the file's bytes (the caller closes it), mirroring
// `Shrine::UploadedFile#open`/`#to_io`.
func (f *UploadedFile) Open() (io.ReadCloser, error) {
	st, err := f.storage()
	if err != nil {
		return nil, err
	}
	return st.Open(f.ID)
}

// Download reads and returns all of the file's bytes, mirroring
// `Shrine::UploadedFile#download`.
func (f *UploadedFile) Download() ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Stream copies the file's bytes to w, mirroring `Shrine::UploadedFile#stream`.
func (f *UploadedFile) Stream(w io.Writer) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	return err
}

// URL returns a locator for the file, or "" if its storage is unknown. Mirrors
// `Shrine::UploadedFile#url`.
func (f *UploadedFile) URL(options map[string]any) string {
	st, err := f.storage()
	if err != nil {
		return ""
	}
	return st.URL(f.ID, options)
}

// Exists reports whether the file is present in its storage, mirroring
// `Shrine::UploadedFile#exists?`.
func (f *UploadedFile) Exists() bool {
	st, err := f.storage()
	if err != nil {
		return false
	}
	return st.Exists(f.ID)
}

// Delete removes the file from its storage, mirroring
// `Shrine::UploadedFile#delete`.
func (f *UploadedFile) Delete() error {
	st, err := f.storage()
	if err != nil {
		return err
	}
	return st.Delete(f.ID)
}

// Replace uploads new bytes to the same storage, deletes this (now stale) file
// and returns the new [UploadedFile]. It mirrors the gem's replace semantics
// (upload the replacement, delete the replaced) at the file level.
func (f *UploadedFile) Replace(r io.Reader, opts *UploadOptions) (*UploadedFile, error) {
	up, err := f.shrine.Uploader(f.StorageKey)
	if err != nil {
		return nil, err
	}
	replacement, err := up.Upload(r, opts)
	if err != nil {
		return nil, err
	}
	if err := f.Delete(); err != nil {
		return nil, err
	}
	return replacement, nil
}
