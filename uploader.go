// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"io"
)

// UploadOptions tunes an [Uploader.Upload] call, mirroring the keyword options
// of the gem's `Shrine#upload`.
type UploadOptions struct {
	// Location overrides the generated storage id when non-empty
	// (the gem's `location:` option).
	Location string
	// Filename seeds the extracted "filename" metadata (the gem reads
	// io.original_filename); ignored if Metadata already carries "filename".
	Filename string
	// Metadata overrides/augments the extracted metadata (the gem's
	// `metadata:` option). Keys present here win over extraction.
	Metadata Metadata
}

// Uploader uploads to a single named storage, mirroring an instance of the
// Shrine class bound to a storage key (e.g. `Shrine.new(:store)`).
type Uploader struct {
	shrine  *Shrine
	key     string
	storage Storage
}

// StorageKey returns the storage name this uploader is bound to.
func (u *Uploader) StorageKey() string { return u.key }

// Upload extracts metadata from the bytes read from r, generates a location,
// stores the bytes and returns the resulting [UploadedFile]. Mirrors
// `Shrine#upload(io, **options)`.
func (u *Uploader) Upload(r io.Reader, opts *UploadOptions) (*UploadedFile, error) {
	if opts == nil {
		opts = &UploadOptions{}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	meta := u.extractMetadata(data, opts)

	loc := opts.Location
	if loc == "" {
		loc = u.shrine.GenerateLocation(meta)
	}
	if err := u.storage.Upload(bytes.NewReader(data), loc, meta); err != nil {
		return nil, err
	}
	return &UploadedFile{
		ID:         loc,
		StorageKey: u.key,
		Metadata:   meta,
		shrine:     u.shrine,
	}, nil
}

// extractMetadata builds the metadata hash for data, honouring overrides in
// opts. Mirrors `Shrine#extract_metadata`: filename (from the option),
// size (byte length) and mime_type (content sniff) — always present, null when
// unknown.
func (u *Uploader) extractMetadata(data []byte, opts *UploadOptions) Metadata {
	meta := Metadata{}
	for k, v := range opts.Metadata {
		meta[k] = v
	}
	if _, ok := meta["filename"]; !ok {
		if opts.Filename != "" {
			meta["filename"] = opts.Filename
		} else {
			meta["filename"] = nil
		}
	}
	if _, ok := meta["size"]; !ok {
		meta["size"] = int64(len(data))
	}
	if _, ok := meta["mime_type"]; !ok {
		meta["mime_type"] = u.shrine.DetectMIME(data)
	}
	return meta
}
