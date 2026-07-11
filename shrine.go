// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
)

// Shrine is a configured attachment context: the named-storage registry plus
// the location and MIME seams. It plays the role of the gem's Shrine class
// (which holds `Shrine.storages` and the uploader behaviour).
type Shrine struct {
	storages   map[string]Storage
	plugins    []Plugin
	extractors []MetadataExtractor

	// GenerateLocation produces the storage id for an upload from its metadata.
	// The default is a random hex string plus the filename's extension. Replace
	// it to make locations deterministic (tests) or content-addressed.
	GenerateLocation func(meta Metadata) string

	// DetectMIME sniffs the mime type of the leading bytes of a file. The
	// default is net/http.DetectContentType.
	DetectMIME func(data []byte) string

	// defaultCache and defaultStore are the storage names used by
	// [Shrine.DefaultAttacher], set by the default_storage plugin.
	defaultCache string
	defaultStore string
}

// Plugin is the minimal plugin-registration seam. Configure is called with the
// [Shrine] when the plugin is registered via [Shrine.Plugin]. The gem's wider
// plugin set is out of scope; this is the extension point a host wires into.
type Plugin interface {
	Configure(s *Shrine)
}

// New returns a Shrine with an empty registry and the default seams.
func New() *Shrine {
	return &Shrine{
		storages:         map[string]Storage{},
		GenerateLocation: defaultGenerateLocation,
		DetectMIME:       http.DetectContentType,
	}
}

// Register adds st to the registry under name (e.g. "cache" or "store"),
// mirroring assigning to `Shrine.storages[:name]`.
func (s *Shrine) Register(name string, st Storage) {
	s.storages[name] = st
}

// Storages returns the storage registry, mirroring `Shrine.storages`.
func (s *Shrine) Storages() map[string]Storage {
	return s.storages
}

// Plugin registers p and invokes its Configure hook.
func (s *Shrine) Plugin(p Plugin) {
	s.plugins = append(s.plugins, p)
	p.Configure(s)
}

// Plugins returns the registered plugins in registration order.
func (s *Shrine) Plugins() []Plugin {
	return s.plugins
}

// Uploader returns an [Uploader] bound to the storage registered under name, or
// a wrapped [ErrUnknownStorage].
func (s *Shrine) Uploader(name string) (*Uploader, error) {
	st, ok := s.storages[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownStorage, name)
	}
	return &Uploader{shrine: s, key: name, storage: st}, nil
}

// Attacher returns an [Attacher] whose cache and store uploaders are bound to
// the named storages, or a wrapped [ErrUnknownStorage] if either is missing.
func (s *Shrine) Attacher(cacheKey, storeKey string) (*Attacher, error) {
	cache, err := s.Uploader(cacheKey)
	if err != nil {
		return nil, err
	}
	store, err := s.Uploader(storeKey)
	if err != nil {
		return nil, err
	}
	return &Attacher{cache: cache, store: store}, nil
}

// UploadedFile rehydrates an [UploadedFile] from its JSON representation
// (`{"id","storage","metadata"}`), binding it to this Shrine so its storage can
// be resolved. It returns a wrapped [ErrUnknownStorage] if the "storage" name
// is not registered. Mirrors `Shrine.uploaded_file(data)`.
func (s *Shrine) UploadedFile(data string) (*UploadedFile, error) {
	var raw ufJSON
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, err
	}
	return s.hydrate(raw)
}

// hydrate binds a decoded [ufJSON] to this Shrine, returning a wrapped
// [ErrUnknownStorage] if its storage name is not registered. It is the shared
// core of [Shrine.UploadedFile] and the derivatives-aware column loader.
func (s *Shrine) hydrate(raw ufJSON) (*UploadedFile, error) {
	if _, ok := s.storages[raw.Storage]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownStorage, raw.Storage)
	}
	return &UploadedFile{
		ID:         raw.ID,
		StorageKey: raw.Storage,
		Metadata:   normalizeMetadata(raw.Metadata),
		shrine:     s,
	}, nil
}

// randRead is the entropy seam for location generation (crypto/rand by
// default); overridden in tests to exercise the failure path.
var randRead = rand.Read

// randomHex returns n random bytes hex-encoded. A crypto/rand failure is
// catastrophic and unrecoverable, so it panics — matching the gem, which lets
// SecureRandom raise.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := randRead(b); err != nil {
		panic(fmt.Errorf("shrine: reading random bytes: %w", err))
	}
	return hex.EncodeToString(b)
}

// defaultGenerateLocation is the default [Shrine.GenerateLocation]: a random hex
// id, suffixed with the filename's extension when there is one.
func defaultGenerateLocation(meta Metadata) string {
	name := randomHex(16)
	if fn := meta.Filename(); fn != "" {
		if ext := filepath.Ext(fn); ext != "" {
			return name + ext
		}
	}
	return name
}
