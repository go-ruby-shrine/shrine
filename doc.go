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
// The plugins real applications depend on are implemented here as pure Go. The
// class-level ones register through the [Plugin] seam ([Shrine.Plugin], whose
// Configure hook receives the Shrine); the attachment-level ones are methods on
// [Attacher]/[UploadedFile]. Behaviour is matched against the shrine gem (v3.8)
// and pinned with differential test oracles.
//
//   - determine_mime_type — content-sniffed mime via a [MIMEAnalyzer] seam
//     ([DetermineMIMEType]; default [ContentAnalyzer] over net/http, plus
//     [ExtensionAnalyzerFor]).
//   - store_dimensions — image width/height via a [DimensionAnalyzer]
//     ([StoreDimensions]; default [ImageDimensions] over the stdlib image
//     decoders), read with [UploadedFile.Width]/[UploadedFile.Height]/
//     [UploadedFile.Dimensions].
//   - add_metadata — pluggable [MetadataExtractor]s ([Shrine.AddMetadata]/
//     [Shrine.AddMetadataKey]); the foundation store_dimensions, signature and
//     refresh_metadata build on.
//   - signature — md5/sha1/sha256/sha384/sha512/crc32 digests in hex/base64/raw
//     ([Signature]), and [SignatureMetadata] to store one per upload.
//   - refresh_metadata — recompute metadata from the stored bytes
//     ([UploadedFile.RefreshMetadata]).
//   - validation_helpers — size/extension/mime/dimension checks with the gem's
//     messages ([Validation]), wired through [Attacher.Validate].
//   - pretty_location — human-readable locations ([Shrine.PrettyLocation]).
//   - derivatives — process/store derived files ([Attacher.CreateDerivatives],
//     [Attacher.AddDerivative], …), serialised in the column data.
//   - cached_attachment_data / restore_cached_data — [Attacher.CachedData],
//     [Attacher.SetCached], [Attacher.RestoreCachedData].
//   - data_uri — parse and attach "data:" URIs ([DataURI],
//     [Attacher.AssignDataURI]).
//   - remote_url — download and attach a URL through a [Downloader] seam
//     ([RemoteURL]).
//   - upload_endpoint / presign_endpoint — [http.Handler]s ([UploadEndpoint],
//     [PresignEndpoint] over the [Presigner] seam).
//   - default_storage — default cache/store ([DefaultStorage],
//     [Shrine.DefaultAttacher]).
//   - activerecord / sequel — model attachment over the [Record] seam
//     ([ModelAttacher], [Shrine.NewActiveRecord]/[Shrine.NewSequel]); the
//     save/destroy callback wiring is host-side.
//
// # Seams left to the host
//
// The parts that need infrastructure the pure-Go core does not carry stay
// injectable: storage backends beyond Memory/FileSystem (S3, GCS, …) are any
// [Storage] (and [Presigner]) implementation; the ORM row is the [Record] seam;
// the remote-url fetch is the [Downloader] seam; the mime and dimension
// analyzers are the [MIMEAnalyzer]/[DimensionAnalyzer] seams. Every test drives
// these through in-memory fakes, so the suite touches no network or disk.
package shrine
