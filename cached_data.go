// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import "fmt"

// ErrNotCached is returned (wrapped) when a file submitted as a cached
// attachment does not live in the cache storage, mirroring the gem's
// `Shrine::Plugins::Cached...`/`NotCached` guard.
var ErrNotCached = fmt.Errorf("shrine: file is not in the cache storage")

// CachedData returns the JSON representation of the current attachment when it
// is a cached file, to be round-tripped through a hidden form field so a failed
// form submission can retain the upload. It returns ("", nil) when nothing is
// attached or the attachment is already stored. It mirrors the
// cached_attachment_data plugin's `Attacher#cached_data`.
func (a *Attacher) CachedData() (string, error) {
	if a.file == nil || a.file.StorageKey != a.cache.key {
		return "", nil
	}
	return a.file.ToJSON()
}

// SetCached rehydrates a cached file from the JSON produced by [CachedData] and
// sets it as the (changed) attachment, without re-reading its bytes. It mirrors
// the plain cached_attachment_data assignment: the client-supplied metadata is
// trusted as-is. Use [RestoreCachedData] instead when the metadata must be
// recomputed from the stored bytes. It returns [ErrNotCached] if the rehydrated
// file is not in the cache storage.
func (a *Attacher) SetCached(json string) error {
	f, err := a.cachedFile(json)
	if err != nil {
		return err
	}
	a.change(f)
	return nil
}

// RestoreCachedData rehydrates a cached file from its JSON, recomputes its
// metadata from the actual stored bytes (discarding any client-forged values),
// then sets it as the attachment. It mirrors the restore_cached_data plugin
// layered on cached_attachment_data. It returns [ErrNotCached] for a non-cache
// file and propagates a [RefreshMetadata] error (e.g. the bytes are gone).
func (a *Attacher) RestoreCachedData(json string) error {
	f, err := a.cachedFile(json)
	if err != nil {
		return err
	}
	if err := f.RefreshMetadata(); err != nil {
		return err
	}
	a.change(f)
	return nil
}

// cachedFile rehydrates json against this attacher's Shrine and verifies the
// file is in the cache storage.
func (a *Attacher) cachedFile(json string) (*UploadedFile, error) {
	f, err := a.shrine().UploadedFile(json)
	if err != nil {
		return nil, err
	}
	if f.StorageKey != a.cache.key {
		return nil, fmt.Errorf("%w: %q", ErrNotCached, f.StorageKey)
	}
	return f, nil
}
