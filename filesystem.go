// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FS is the filesystem seam [FileSystem] runs over. The default implementation
// ([OSFS]) is backed by the os package; tests inject a fake to exercise the I/O
// error branches hermetically.
type FS interface {
	// MkdirAll creates dir and any missing parents.
	MkdirAll(dir string, perm os.FileMode) error
	// WriteFile writes data to path, truncating an existing file.
	WriteFile(path string, data []byte, perm os.FileMode) error
	// ReadFile returns the contents of path.
	ReadFile(path string) ([]byte, error)
	// Remove deletes path.
	Remove(path string) error
	// Exists reports whether path exists.
	Exists(path string) bool
}

// OSFS is the production [FS], backed by the os package.
type OSFS struct{}

// MkdirAll calls os.MkdirAll.
func (OSFS) MkdirAll(dir string, perm os.FileMode) error { return os.MkdirAll(dir, perm) }

// WriteFile calls os.WriteFile.
func (OSFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// ReadFile calls os.ReadFile.
func (OSFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// Remove calls os.Remove.
func (OSFS) Remove(path string) error { return os.Remove(path) }

// Exists reports whether path exists via os.Stat.
func (OSFS) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FileSystem is a [Storage] that persists files under a base Directory,
// mirroring Shrine::Storage::FileSystem. All I/O goes through the [FS] seam
// (default [OSFS]).
type FileSystem struct {
	// Directory is the base directory ids are stored under.
	Directory string
	// Prefix, if set, is prepended (with "/") to ids in URL. When empty, URL
	// returns the on-disk path.
	Prefix string
	// DirPerm and FilePerm are the permissions for created directories/files.
	DirPerm  os.FileMode
	FilePerm os.FileMode
	// fs is the filesystem seam.
	fs FS
}

// NewFileSystem returns a filesystem storage rooted at directory, using the
// production [OSFS] seam and 0755/0644 permissions.
func NewFileSystem(directory string) *FileSystem {
	return &FileSystem{
		Directory: directory,
		DirPerm:   0o755,
		FilePerm:  0o644,
		fs:        OSFS{},
	}
}

// NewFileSystemWithFS returns a filesystem storage using a custom [FS] seam
// (used by tests to inject failures).
func NewFileSystemWithFS(directory string, fs FS) *FileSystem {
	s := NewFileSystem(directory)
	s.fs = fs
	return s
}

// path is the on-disk location for id.
func (s *FileSystem) path(id string) string {
	return filepath.Join(s.Directory, id)
}

// Upload writes the bytes read from r to the file for id, creating parents.
func (s *FileSystem) Upload(r io.Reader, id string, _ map[string]any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p := s.path(id)
	if err := s.fs.MkdirAll(filepath.Dir(p), s.DirPerm); err != nil {
		return err
	}
	return s.fs.WriteFile(p, data, s.FilePerm)
}

// Open returns a reader over the file for id, or a wrapped [ErrNotFound].
func (s *FileSystem) Open(id string) (io.ReadCloser, error) {
	p := s.path(id)
	if !s.fs.Exists(p) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	data, err := s.fs.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Exists reports whether the file for id exists.
func (s *FileSystem) Exists(id string) bool {
	return s.fs.Exists(s.path(id))
}

// Delete removes the file for id.
func (s *FileSystem) Delete(id string) error {
	return s.fs.Remove(s.path(id))
}

// URL returns Prefix + "/" + id when Prefix is set, else the on-disk path.
func (s *FileSystem) URL(id string, _ map[string]any) string {
	if s.Prefix != "" {
		return s.Prefix + "/" + id
	}
	return s.path(id)
}
