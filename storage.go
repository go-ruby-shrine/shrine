// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import "io"

// Storage is the abstract file backend, mirroring Shrine::Storage. A concrete
// storage persists opaque bytes under a caller-chosen id (the "location"). The
// package ships two implementations, [Memory] and [FileSystem]; a host can add
// its own (S3, GCS, …) by satisfying this interface.
//
// This maps to the gem's storage contract:
//
//	upload(io, id, shrine_metadata:) -> Upload(r, id, meta)
//	open(id)                         -> Open(id)
//	exists?(id)                      -> Exists(id)
//	delete(id)                       -> Delete(id)
//	url(id, **options)               -> URL(id, options)
type Storage interface {
	// Upload persists the bytes read from r under id. meta carries the
	// extracted shrine metadata (advisory; backends may ignore it).
	Upload(r io.Reader, id string, meta map[string]any) error
	// Open returns a reader over the stored bytes, or a wrapped [ErrNotFound]
	// if id is absent. The caller closes it.
	Open(id string) (io.ReadCloser, error)
	// Exists reports whether id is present.
	Exists(id string) bool
	// Delete removes id. Deleting an absent id is not an error.
	Delete(id string) error
	// URL returns a locator for id. options are backend-specific and may be nil.
	URL(id string, options map[string]any) string
}
