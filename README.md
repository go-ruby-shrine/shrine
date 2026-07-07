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
> seam, and MIME detection is the **`DetectMIME`** seam. The gem's large
> **plugin set is out of scope**; only the `Plugin` registration seam lives here.

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
- **Plugins** — the minimal `Plugin` registration hook (`s.Plugin(p)`); the
  broad plugin set is deferred.

CGO-free, dependency-free (stdlib only), **100% test coverage**, `gofmt` +
`go vet` clean, and green across the six 64-bit Go targets (amd64, arm64,
riscv64, loong64, ppc64le, **s390x** — big-endian) plus `js/wasm` and
`wasip1/wasm`.

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
