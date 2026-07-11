// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

// DefaultStorage is the default_storage plugin: it records the cache and store
// storage names so attachers can be created without naming them each time, via
// [Shrine.DefaultAttacher] and [Shrine.DefaultUploader].
//
//	s.Plugin(&shrine.DefaultStorage{Cache: "cache", Store: "store"})
//	att, _ := s.DefaultAttacher()
type DefaultStorage struct {
	// Cache and Store are the default storage names.
	Cache string
	Store string
}

// Configure records the default cache/store names on s.
func (p *DefaultStorage) Configure(s *Shrine) {
	s.defaultCache = p.Cache
	s.defaultStore = p.Store
}

// DefaultAttacher returns an [Attacher] over the storages configured by the
// default_storage plugin, or a wrapped [ErrUnknownStorage] if either default is
// unset or unregistered.
func (s *Shrine) DefaultAttacher() (*Attacher, error) {
	return s.Attacher(s.defaultCache, s.defaultStore)
}

// DefaultUploader returns an [Uploader] over the default store storage,
// mirroring uploading to the default storage without naming it.
func (s *Shrine) DefaultUploader() (*Uploader, error) {
	return s.Uploader(s.defaultStore)
}
