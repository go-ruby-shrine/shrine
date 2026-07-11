// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import "io"

// Attacher drives the cache→store attachment lifecycle, mirroring
// Shrine::Attacher. Newly assigned files land in the cache storage; finalizing
// promotes them to the permanent store storage and deletes the previously
// stored file (replacement). It tracks whether the attachment has changed.
type Attacher struct {
	cache *Uploader
	store *Uploader

	file     *UploadedFile // the current attachment (may be nil)
	original *UploadedFile // the last finalized attachment, for replacement
	changed  bool

	// Validate, when set, runs on every attachment change and returns the
	// validation error messages, which are recorded in [Attacher.Errors].
	// It is the seam the validation_helpers plugin plugs into (the gem's
	// `Attacher.validate do … end` block); build the messages with a
	// [Validation]. See [Attacher.Valid].
	Validate func(file *UploadedFile) []string
	// Errors holds the messages from the last [Attacher.Validate] run,
	// mirroring `Attacher#errors`.
	Errors []string

	// derivatives holds the processed derived files (the derivatives plugin),
	// keyed by name; nil until the first is added.
	derivatives map[string]*UploadedFile
}

// shrine returns the [Shrine] this attacher's uploaders belong to.
func (a *Attacher) shrine() *Shrine { return a.store.shrine }

// Get returns the current attachment, or nil when nothing is attached. Mirrors
// `Attacher#get`/`#file`.
func (a *Attacher) Get() *UploadedFile { return a.file }

// Changed reports whether the attachment differs from the last finalized state,
// mirroring `Attacher#changed?`.
func (a *Attacher) Changed() bool { return a.changed }

// change sets the current attachment, marks the attacher changed and runs the
// validation seam (if any) against the new file, recording its messages in
// [Attacher.Errors]. Detaching (file == nil) clears the errors without
// validating, matching the gem.
func (a *Attacher) change(file *UploadedFile) {
	a.file = file
	a.changed = true
	a.Errors = nil
	if file != nil && a.Validate != nil {
		a.Errors = a.Validate(file)
	}
}

// Valid reports whether the last attachment change passed validation (no
// recorded errors), mirroring `Attacher#valid?`.
func (a *Attacher) Valid() bool { return len(a.Errors) == 0 }

// Assign uploads r to the cache storage and sets it as the (changed) current
// attachment, mirroring `Attacher#assign(io)` / `#attach_cached`.
func (a *Attacher) Assign(r io.Reader, opts *UploadOptions) error {
	file, err := a.cache.Upload(r, opts)
	if err != nil {
		return err
	}
	a.change(file)
	return nil
}

// Set makes file the current (changed) attachment without uploading — for an
// already-uploaded file (e.g. a cached file submitted by a form) or nil to
// detach. Mirrors `Attacher#set`.
func (a *Attacher) Set(file *UploadedFile) {
	a.change(file)
}

// Promote uploads the current cached attachment to the store storage and makes
// the stored copy the current attachment. A no-op if nothing is attached.
// Mirrors `Attacher#promote`.
func (a *Attacher) Promote() error {
	if a.file == nil {
		return nil
	}
	rc, err := a.file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	stored, err := a.store.Upload(rc, &UploadOptions{Metadata: a.file.Metadata})
	if err != nil {
		return err
	}
	a.file = stored
	return nil
}

// Finalize commits a change: it promotes a cached attachment to the store,
// deletes the previously finalized file (replacement) and clears the changed
// flag. A no-op when nothing changed. Mirrors `Attacher#finalize`.
func (a *Attacher) Finalize() error {
	if !a.changed {
		return nil
	}
	previous := a.original
	if a.file != nil && a.file.StorageKey == a.cache.key {
		if err := a.Promote(); err != nil {
			return err
		}
	}
	if previous != nil {
		if err := previous.Delete(); err != nil {
			return err
		}
	}
	a.original = a.file
	a.changed = false
	return nil
}

// Destroy deletes the current attachment, mirroring `Attacher#destroy`.
func (a *Attacher) Destroy() error {
	if a.file == nil {
		return nil
	}
	return a.file.Delete()
}
