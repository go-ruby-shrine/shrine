// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

// MetadataExtractor computes extra metadata from an upload, mirroring a block
// registered with the gem's `add_metadata` plugin. It receives the raw content
// and the metadata accumulated so far (the core filename/size/mime_type plus
// any earlier extractors) and returns key/value pairs to merge in. Returning
// nil (or an empty map) adds nothing.
//
// Extractors run during every upload, in registration order, after the core
// keys and before the caller's explicit `metadata:` overrides — exactly where
// the gem runs `add_metadata` blocks.
type MetadataExtractor func(data []byte, meta Metadata) map[string]any

// AddMetadata registers a multi-key [MetadataExtractor], mirroring the gem's
// block form `add_metadata { |io, **| { … } }`.
func (s *Shrine) AddMetadata(ex MetadataExtractor) {
	s.extractors = append(s.extractors, ex)
}

// AddMetadataKey registers a single-key extractor, mirroring the gem's
// `add_metadata(:key) { |io, **| … }`. A nil return from fn stores a null value
// under name, matching the gem (the key is always present once registered).
func (s *Shrine) AddMetadataKey(name string, fn func(data []byte, meta Metadata) any) {
	s.AddMetadata(func(data []byte, meta Metadata) map[string]any {
		return map[string]any{name: fn(data, meta)}
	})
}

// String returns the string value stored under key, or "" if absent/null or not
// a string. It is the generic accessor add_metadata-defined keys are read
// through (the gem defines an `UploadedFile#<key>` reader; Go cannot add methods
// dynamically, so callers read named keys through these helpers).
func (m Metadata) String(key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// Int returns the integer value stored under key, or 0 if absent/null or not a
// number. It accepts the int produced by an extractor and the float64 produced
// by a JSON round-trip.
func (m Metadata) Int(key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
