// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"fmt"
	"io"
)

// Memory is an in-process [Storage] backed by a map, mirroring
// Shrine::Storage::Memory. It is handy for tests and ephemeral caches. URLs are
// of the form "memory://<id>".
type Memory struct {
	store map[string][]byte
}

// NewMemory returns an empty in-memory storage.
func NewMemory() *Memory {
	return &Memory{store: map[string][]byte{}}
}

// Upload buffers the bytes read from r under id.
func (m *Memory) Upload(r io.Reader, id string, _ map[string]any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.store[id] = data
	return nil
}

// Open returns a reader over the bytes stored at id, or a wrapped [ErrNotFound].
func (m *Memory) Open(id string) (io.ReadCloser, error) {
	data, ok := m.store[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Exists reports whether id is present.
func (m *Memory) Exists(id string) bool {
	_, ok := m.store[id]
	return ok
}

// Delete removes id (a no-op if absent).
func (m *Memory) Delete(id string) error {
	delete(m.store, id)
	return nil
}

// URL returns "memory://<id>".
func (m *Memory) URL(id string, _ map[string]any) string {
	return "memory://" + id
}
