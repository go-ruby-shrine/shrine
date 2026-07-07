// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

// Metadata is the open metadata hash carried by an [UploadedFile], mirroring the
// gem's Shrine metadata Hash. The three core keys shrine extracts are
// "filename", "size" and "mime_type"; plugins (and callers) may add more. Size
// is stored as an int64 so it round-trips through JSON as a number.
type Metadata map[string]any

// Filename returns the "filename" value, or "" if absent/null.
func (m Metadata) Filename() string {
	if s, ok := m["filename"].(string); ok {
		return s
	}
	return ""
}

// Size returns the "size" value in bytes, or 0 if absent/null. It accepts both
// the int64 produced on upload and the float64 produced by JSON decoding.
func (m Metadata) Size() int64 {
	switch v := m["size"].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// MimeType returns the "mime_type" value, or "" if absent/null.
func (m Metadata) MimeType() string {
	if s, ok := m["mime_type"].(string); ok {
		return s
	}
	return ""
}

// normalizeMetadata copies a decoded metadata map, coercing a JSON "size"
// number (float64) back to int64 so the value survives a JSON round-trip.
func normalizeMetadata(m map[string]any) Metadata {
	if m == nil {
		return Metadata{}
	}
	out := make(Metadata, len(m))
	for k, v := range m {
		if k == "size" {
			if f, ok := v.(float64); ok {
				out[k] = int64(f)
				continue
			}
		}
		out[k] = v
	}
	return out
}
