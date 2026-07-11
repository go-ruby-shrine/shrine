// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"encoding/json"
	"net/http"
)

// UploadEndpoint is the upload_endpoint plugin: an [http.Handler] that accepts a
// multipart POST, uploads the "file" part to a named storage (typically the
// cache) and returns the resulting [UploadedFile] as JSON. It is the Go
// analogue of the gem's Rack app; mount it under any router.
//
// Responses: 200 with the UploadedFile JSON on success; 405 for a non-POST
// method; 400 when the "file" part is missing; 500 when the storage is unknown
// or the upload fails.
type UploadEndpoint struct {
	// Shrine is the attachment context; required.
	Shrine *Shrine
	// StorageKey names the storage uploads land in (e.g. "cache"); required.
	StorageKey string
	// FieldName is the multipart field the file arrives in; "" means "file".
	FieldName string
	// MaxMemory bounds the in-memory multipart buffer in bytes; 0 uses the
	// net/http default (32 MiB).
	MaxMemory int64
}

// ServeHTTP implements [http.Handler].
func (e *UploadEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	field := e.FieldName
	if field == "" {
		field = "file"
	}
	maxMem := e.MaxMemory
	if maxMem == 0 {
		maxMem = 32 << 20
	}
	if err := r.ParseMultipartForm(maxMem); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	up, err := e.Shrine.Uploader(e.StorageKey)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "unknown storage")
		return
	}
	uf, err := up.Upload(file, &UploadOptions{Filename: header.Filename})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "upload failed")
		return
	}
	writeJSON(w, http.StatusOK, uf)
}

// PresignData is the direct-upload descriptor a [Presigner] returns and the
// presign_endpoint serialises, mirroring the gem's presign response
// ({method, url, fields, headers}).
type PresignData struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Fields  map[string]string `json:"fields"`
	Headers map[string]string `json:"headers"`
}

// Presigner is a storage capable of authorising a direct client upload — the
// seam the presign_endpoint depends on. The built-in Memory/FileSystem storages
// cannot presign; an S3-style backend (added by the host) satisfies this.
type Presigner interface {
	// Presign authorises an upload to id and returns the descriptor a client
	// posts the bytes with. opts are backend-specific and may be nil.
	Presign(id string, opts map[string]any) (PresignData, error)
}

// PresignEndpoint is the presign_endpoint plugin: an [http.Handler] that
// generates a storage location, asks a [Presigner] to authorise a direct upload
// to it and returns the [PresignData] as JSON. The presigning storage is the
// seam; this endpoint has no built-in backend.
//
// Responses: 200 with the PresignData JSON on success; 405 for a non-GET
// method; 500 when presigning fails.
type PresignEndpoint struct {
	// Presigner authorises the upload; required.
	Presigner Presigner
	// GenerateID produces the storage location to presign; nil means a random
	// hex id.
	GenerateID func() string
	// Options are passed through to [Presigner.Presign]; may be nil.
	Options map[string]any
}

// ServeHTTP implements [http.Handler].
func (e *PresignEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	gen := e.GenerateID
	if gen == nil {
		gen = func() string { return randomHex(16) }
	}
	data, err := e.Presigner.Presign(gen(), e.Options)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "presign failed")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// writeJSON writes v as a JSON response with the given status. A marshalling
// failure downgrades to a 500 with a plain body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeJSONError writes {"error": msg} with the given status.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
