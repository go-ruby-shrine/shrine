// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidDataURI is returned (wrapped) by [DataURI] for a value that is not a
// parseable data URI, mirroring the data_uri plugin's `ParseError`.
var ErrInvalidDataURI = fmt.Errorf("shrine: invalid data URI")

// dataURIPrefix is the required scheme.
const dataURIPrefix = "data:"

// base64Marker is the token that flags base64-encoded payloads.
const base64Marker = ";base64"

// DataURI parses an RFC 2397 "data:" URI into its media type and decoded bytes,
// mirroring `Shrine.data_uri(uri)` from the data_uri plugin. The media type
// defaults to "text/plain" when omitted (keeping any parameters such as
// ";charset=utf-8"). A ";base64" payload is base64-decoded; otherwise it is
// form-unescaped (percent escapes decoded and "+" treated as space), matching
// the gem. It returns a wrapped [ErrInvalidDataURI] for a malformed URI or an
// undecodable payload.
func DataURI(uri string) (mimeType string, data []byte, err error) {
	if !strings.HasPrefix(uri, dataURIPrefix) {
		return "", nil, fmt.Errorf("%w: missing data: scheme", ErrInvalidDataURI)
	}
	rest := uri[len(dataURIPrefix):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", nil, fmt.Errorf("%w: missing comma", ErrInvalidDataURI)
	}
	meta, payload := rest[:comma], rest[comma+1:]

	isBase64 := strings.HasSuffix(meta, base64Marker)
	if isBase64 {
		meta = meta[:len(meta)-len(base64Marker)]
	}
	mimeType = meta
	if mimeType == "" {
		mimeType = "text/plain"
	}

	if isBase64 {
		data, err = base64.StdEncoding.DecodeString(payload)
	} else {
		var s string
		s, err = url.QueryUnescape(payload)
		data = []byte(s)
	}
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidDataURI, err)
	}
	return mimeType, data, nil
}

// AssignDataURI parses a "data:" URI, uploads its bytes to the cache storage and
// sets the result as the (changed) attachment, mirroring
// `Attacher#assign_data_uri`. The parsed media type seeds the "mime_type"
// metadata override (so it is not re-sniffed), matching the gem. It returns a
// wrapped [ErrInvalidDataURI] on a bad URI or the upload error otherwise.
func (a *Attacher) AssignDataURI(uri string, opts *UploadOptions) error {
	mimeType, data, err := DataURI(uri)
	if err != nil {
		return err
	}
	if opts == nil {
		opts = &UploadOptions{}
	}
	if opts.Metadata == nil {
		opts.Metadata = Metadata{}
	}
	if _, ok := opts.Metadata["mime_type"]; !ok {
		opts.Metadata["mime_type"] = mimeType
	}
	file, err := a.cache.Upload(bytes.NewReader(data), opts)
	if err != nil {
		return err
	}
	a.change(file)
	return nil
}
