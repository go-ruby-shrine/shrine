// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import "strings"

// LocationContext carries the record context the pretty_location plugin folds
// into a human-readable storage location. In the gem this context comes from
// the model attachment (record class, record id, attachment name); here the
// host supplies it, because the pure-Go core has no ORM. Any empty field is
// omitted (the gem's `compact`).
type LocationContext struct {
	// Namespace is the record's class location, e.g. "photo" or "user/avatar".
	Namespace string
	// Identifier is the record's primary key rendered as a string, e.g. "42".
	Identifier string
	// Name is the attachment name, e.g. "image".
	Name string
	// Metadata seeds the trailing basename's extension (from "filename").
	Metadata Metadata
}

// PrettyLocation builds a human-readable storage location of the form
// "namespace/identifier/name/basename.ext", mirroring the pretty_location
// plugin. The basename (and its extension) come from [Shrine.GenerateLocation],
// so the default random-hex-plus-extension basename is preserved and tests can
// inject a deterministic generator. Empty context fields are dropped, so a
// contextless call returns just the basename — identical to the plain
// generator.
//
// Pass the result as [UploadOptions.Location]:
//
//	loc := s.PrettyLocation(shrine.LocationContext{
//		Namespace: "photo", Identifier: "42", Name: "image",
//		Metadata: shrine.Metadata{"filename": "me.jpg"},
//	})
//	up.Upload(r, &shrine.UploadOptions{Location: loc}) // photo/42/image/<hex>.jpg
func (s *Shrine) PrettyLocation(ctx LocationContext) string {
	basename := s.GenerateLocation(ctx.Metadata)
	parts := make([]string, 0, 4)
	for _, p := range []string{ctx.Namespace, ctx.Identifier, ctx.Name} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	parts = append(parts, basename)
	return strings.Join(parts, "/")
}
