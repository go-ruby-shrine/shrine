<p align="center"><img src="https://go-ruby-shrine.github.io/logo.png" alt="go-ruby-shrine/shrine" width="720"></p>

# shrine — go-ruby-shrine

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-shrine.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the deterministic core of Ruby's
[`shrine`](https://github.com/shrinerb/shrine) gem** — the file-attachment
toolkit. It reproduces the [`Storage`](storage.go) abstraction and its built-in
Memory / FileSystem backends, the named-storage registry, the `Shrine` uploader
(metadata extraction + location generation), the `UploadedFile` value with its
`{"id","storage","metadata"}` JSON representation, and the `Attacher`
cache→store promotion lifecycle — **without any Ruby runtime**.

It is the attachment layer for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but a
**standalone, reusable** module.

> **What it is — and isn't.** Everything shrine does *around* the bytes is
> deterministic and needs **no interpreter**, so it lives here as pure Go:
> registering named storages, extracting `filename`/`size`/`mime_type` metadata
> (mime by content sniffing via `net/http.DetectContentType`), generating a
> storage location, uploading, JSON-encoding/decoding the `UploadedFile`, and
> driving the cache→store promotion on the `Attacher`. The pieces that touch the
> outside world are **host seams**, so tests stay hermetic: the filesystem
> storage runs over an injectable **`FS`** seam (default `OSFS`; a fake drives
> the I/O error branches), location generation is the **`GenerateLocation`**
> seam, and MIME detection is the **`DetectMIME`** seam. The plugins real apps
> rely on are implemented here (see **[Plugins](#plugins)**); the parts that need
> infrastructure the pure-Go core does not carry — storage backends beyond
> Memory/FileSystem (S3, …), the ORM row, the remote-URL fetch — stay as
> injectable seams.

## Features

Faithful port of the shrine attachment core:

- **`Storage`** — `Upload(r, id, meta)` / `Open(id)` / `Exists(id)` /
  `Delete(id)` / `URL(id, options)`, with two built-ins: **`Memory`** (map-backed,
  `memory://` URLs) and **`FileSystem`** (over the injectable `FS` seam, path or
  `Prefix`-based URLs).
- **Registry** — `New()` plus `Register(name, storage)` / `Storages()`
  (the gem's `Shrine.storages[:cache]` / `[:store]`).
- **`Uploader`** — `s.Uploader(name)` then `Upload(io, *UploadOptions)`: extracts
  metadata (filename from the option, size from the byte length, mime by content
  sniff), generates a location (default random hex + extension), stores, and
  returns an `UploadedFile`. `UploadOptions` carries `Location` / `Filename` /
  `Metadata` overrides.
- **`UploadedFile`** — `{ID, StorageKey, Metadata}` with `Data()` / `ToJSON()`
  (the gem's `{"id","storage","metadata"}` shape), `Open` / `Download` /
  `Stream` / `URL` / `Exists` / `Delete`, and `Replace`.
  `s.UploadedFile(json)` rehydrates one and binds it to the registry.
- **`Attacher`** — `s.Attacher("cache","store")`, then `Assign` (upload to
  cache) → `Finalize` / `Promote` (promote to store, delete the replaced file) →
  `Destroy`, with `Changed()` state and `Set` for an already-uploaded file (or
  `nil` to detach).
- **Seams** — `GenerateLocation func(Metadata) string`,
  `DetectMIME func([]byte) string`, and the filesystem `FS` interface.
- **[Plugins](#plugins)** — the core plugin set real apps rely on, in pure Go.

CGO-free, dependency-free (stdlib only), **100% test coverage**, `gofmt` +
`go vet` clean, and green across the six 64-bit Go targets (amd64, arm64,
riscv64, loong64, ppc64le, **s390x** — big-endian) plus `js/wasm` and
`wasip1/wasm`.

## Plugins

The plugins real applications depend on are implemented here in **pure Go**, on
top of the uploader/storage core. Class-level plugins register through the
`Plugin` seam (`s.Plugin(&shrine.DetermineMIMEType{})`); attachment-level plugins
are methods on `Attacher` / `UploadedFile`. Every behaviour is checked against
the **shrine gem (v3.8)** and pinned with differential test oracles (signature
digests, `data:` decoding, validation messages, `pretty_location` shape, …).

| plugin | this package | notes |
| --- | --- | --- |
| `determine_mime_type` | `DetermineMIMEType` + `MIMEAnalyzer` (`ContentAnalyzer`, `ExtensionAnalyzerFor`) | content sniff via `net/http.DetectContentType` |
| `store_dimensions` | `StoreDimensions` + `DimensionAnalyzer` (`ImageDimensions`); `f.Width()`/`.Height()`/`.Dimensions()` | pure-Go `image.DecodeConfig` (png/jpeg/gif) |
| `add_metadata` | `s.AddMetadata` / `s.AddMetadataKey`; `Metadata.String`/`.Int` | foundation for the metadata plugins |
| `signature` | `Signature(...)` + `SignatureMetadata` | md5/sha1/sha256/sha384/sha512/crc32 × hex/base64/none |
| `refresh_metadata` | `f.RefreshMetadata()` | recompute metadata from stored bytes |
| `validation_helpers` | `Validation` + `att.Validate` / `att.Valid()` | gem-identical messages; ext case-insensitive, mime case-sensitive |
| `pretty_location` | `s.PrettyLocation(LocationContext{...})` | `namespace/id/name/basename.ext` |
| `derivatives` | `att.CreateDerivatives` / `AddDerivative` / `Derivative[URL]` / `DeleteDerivatives` | serialised under the column's `"derivatives"` key |
| `cached_attachment_data` | `att.CachedData()` / `att.SetCached()` | hidden-field round-trip |
| `restore_cached_data` | `att.RestoreCachedData()` | re-extracts metadata from bytes (anti-tamper) |
| `data_uri` | `DataURI(uri)` / `att.AssignDataURI()` | RFC 2397 parsing (base64 + form-unescape) |
| `remote_url` | `RemoteURL{Download, MaxSize}` + `HTTPDownloader` | download via the `Downloader` seam |
| `upload_endpoint` | `UploadEndpoint` (`http.Handler`) | multipart → cache, JSON response |
| `presign_endpoint` | `PresignEndpoint` over the `Presigner` seam | direct-upload descriptor |
| `default_storage` | `DefaultStorage` + `s.DefaultAttacher()` / `s.DefaultUploader()` | |
| `activerecord` / `sequel` | `ModelAttacher` (`s.NewActiveRecord` / `s.NewSequel`) over the `Record` seam | callback wiring is host-side |
| `column`/`entity`/`model` serialization | `att.ColumnData()` / `att.LoadColumn()` | derivatives-aware column data |

**Left as injectable seams** (need infrastructure the pure-Go core does not
carry): storage backends beyond Memory/FileSystem — **S3, GCS, …** — are any
`Storage` (and `Presigner`) implementation; the ORM row is the `Record` seam;
the remote-URL fetch is the `Downloader` seam; the mime/dimension analyzers are
the `MIMEAnalyzer` / `DimensionAnalyzer` seams. Beyond that, the interpreter-only
plugins (e.g. `backgrounding`, `rack_response`, `mirroring`, `infer_extension`)
remain for the rbgo binding.

> **MIME divergence.** `net/http.DetectContentType` appends a charset to text
> (`text/plain; charset=utf-8`) and never returns nil, where the gem's default
> `:file` analyzer returns a bare type or nil for empty/unknown content. The
> recognised binary signatures (PNG/JPEG/GIF/PDF/ZIP/…) agree with the gem.

## Install

```sh
go get github.com/go-ruby-shrine/shrine
```

## Usage

```go
package main

import (
	"fmt"
	"strings"

	"github.com/go-ruby-shrine/shrine"
)

func main() {
	s := shrine.New()
	s.Register("cache", shrine.NewMemory())
	s.Register("store", shrine.NewFileSystem("/var/uploads"))

	// Direct upload.
	up, _ := s.Uploader("store")
	file, _ := up.Upload(strings.NewReader("hello"), &shrine.UploadOptions{Filename: "hello.txt"})
	data, _ := file.ToJSON()
	fmt.Println(data) // {"id":"….txt","storage":"store","metadata":{…}}

	// Rehydrate the reference and read it back.
	file2, _ := s.UploadedFile(data)
	body, _ := file2.Download()
	fmt.Println(string(body)) // hello

	// Attach lifecycle: assign to cache, finalize promotes to store.
	att, _ := s.Attacher("cache", "store")
	att.Assign(strings.NewReader("world"), &shrine.UploadOptions{Filename: "w.txt"})
	att.Finalize() // promoted to store; a replaced file is deleted
	fmt.Println(att.Get().StorageKey) // store
}
```

### Injecting the seams (tests / hosts)

```go
s := shrine.New()
s.GenerateLocation = func(m shrine.Metadata) string { return "fixed-id-" + m.Filename() }
s.DetectMIME = func([]byte) string { return "application/octet-stream" }

// Filesystem storage over a fake FS keeps tests off the disk.
s.Register("store", shrine.NewFileSystemWithFS("/base", myFakeFS{}))
```

## Value model

| gem                                          | this package                                       |
| -------------------------------------------- | -------------------------------------------------- |
| `Shrine.storages[:cache] = …`                | `s.Register("cache", …)` / `s.Storages()`          |
| `Shrine.new(:store).upload(io)`              | `s.Uploader("store")` then `up.Upload(io, opts)`   |
| `Shrine::UploadedFile#data / #to_json`       | `(*UploadedFile).Data()` / `.ToJSON()`             |
| `uploaded_file.url / #download / #metadata`  | `.URL(opts)` / `.Download()` / `.Metadata`         |
| `Shrine.uploaded_file(data)`                 | `s.UploadedFile(json)`                             |
| `Attacher#assign / #finalize / #destroy`     | `att.Assign / .Finalize / .Destroy`                |
| `Attacher#changed?`                          | `att.Changed()`                                    |
| `Shrine::Storage::Memory / ::FileSystem`     | `NewMemory()` / `NewFileSystem(dir)`               |
| the storage I/O                              | `FS` seam (`OSFS` prod, fake in tests)             |

### JSON data shape

```json
{"id":"49f7c1….txt","storage":"store","metadata":{"filename":"hello.txt","mime_type":"text/plain; charset=utf-8","size":5}}
```

## Tests & coverage

The suite is deterministic and touches no real network. Filesystem I/O runs over
the `FS` seam — a fake drives every error branch (mkdir/write/read/remove
failure) while `OSFS` is exercised against `t.TempDir()`; a `DoerFunc`-style
`errReader`/`errWriter` cover the streaming error paths; the entropy source and
the location/MIME seams are injected. **No test opens a real socket**, so the
cross-arch qemu lanes and the Windows lane all hold coverage at **100%**.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-shrine/shrine authors.
