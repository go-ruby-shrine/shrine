// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package shrine is a pure-Go (CGO-free) reimplementation of the deterministic
// core of Ruby's `shrine` gem — the file-attachment toolkit. It reproduces the
// pieces that need no Ruby runtime: the [Storage] abstraction and its built-in
// Memory / FileSystem backends, the named-storage registry, the [Uploader]
// (Shrine class) that extracts metadata and generates a location, the
// [UploadedFile] value with its `{"id","storage","metadata"}` JSON
// representation, and the [Attacher] cache→store promotion lifecycle.
//
// It is the file-attachment layer for
// [go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but a
// standalone, reusable module.
//
// # What it is — and isn't
//
// Everything shrine does around the bytes is deterministic and needs no
// interpreter, so it lives here as pure Go: registering named storages,
// extracting the filename/size/mime_type metadata (mime by content sniffing via
// [net/http.DetectContentType]), generating a storage location (random id +
// extension by default), uploading to a storage, JSON-encoding/decoding the
// UploadedFile value, and driving the cache→store promotion lifecycle on the
// Attacher.
//
// The pieces that touch the outside world are host seams, so tests stay
// hermetic:
//
//   - The filesystem [Storage] runs over an injectable [FS] seam. The default
//     ([OSFS]) is backed by the os package; tests inject a fake to exercise the
//     I/O error branches without a real disk.
//   - Location generation is the [Shrine.GenerateLocation] seam (default: random
//     hex + extension); a test injects a deterministic generator.
//   - MIME detection is the [Shrine.DetectMIME] seam (default:
//     net/http.DetectContentType).
//
// # Flow
//
//	s := shrine.New()
//	s.Register("cache", shrine.NewMemory())
//	s.Register("store", shrine.NewMemory())
//
//	up, _ := s.Uploader("store")
//	file, _ := up.Upload(strings.NewReader("hello"), &shrine.UploadOptions{Filename: "a.txt"})
//	json, _ := file.ToJSON() // {"id":"….txt","storage":"store","metadata":{…}}
//
//	// rehydrate from the JSON representation
//	file2, _ := s.UploadedFile(json)
//	data, _ := file2.Download()
//
//	// attach lifecycle: assign to cache, finalize promotes to store
//	att, _ := s.Attacher("cache", "store")
//	att.Assign(strings.NewReader("hi"), &shrine.UploadOptions{Filename: "b.txt"})
//	att.Finalize()
//
// # Plugins
//
// The gem's plugin system is a large surface that mostly needs the interpreter;
// only the registration seam lives here. Register a [Plugin] with
// [Shrine.Plugin]; its Configure hook receives the Shrine instance. The wider
// plugin set is deferred to the rbgo binding.
package shrine
