// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
)

// Downloader fetches the bytes at a remote URL, returning a reader over the body
// (the caller closes it) and a suggested filename (may be ""). It is the seam
// the remote_url plugin downloads through — the pure-Go analogue of the gem's
// `down` dependency — so tests inject a fake and never touch the network.
type Downloader func(url string) (body io.ReadCloser, filename string, err error)

// ErrRemoteURLDownload wraps a [Downloader] failure.
var ErrRemoteURLDownload = fmt.Errorf("shrine: remote url download failed")

// ErrRemoteURLTooLarge is returned when a downloaded body exceeds
// [RemoteURL.MaxSize], mirroring the gem's `max_size` guard.
var ErrRemoteURLTooLarge = fmt.Errorf("shrine: remote url exceeds max size")

// HTTPDownloader returns a [Downloader] that GETs the URL with client (or
// [http.DefaultClient] when nil) and derives the filename from the URL path. A
// non-2xx response is an error. The client's transport is the injection point:
// tests supply a canned [http.RoundTripper], so no socket is opened.
func HTTPDownloader(client *http.Client) Downloader {
	if client == nil {
		client = http.DefaultClient
	}
	return func(rawURL string) (io.ReadCloser, string, error) {
		resp, err := client.Get(rawURL)
		if err != nil {
			return nil, "", err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		// Derive the filename from the URL path (query/fragment excluded).
		name := ""
		if u, perr := url.Parse(rawURL); perr == nil {
			name = path.Base(u.Path)
			if name == "." || name == "/" {
				name = ""
			}
		}
		return resp.Body, name, nil
	}
}

// RemoteURL is the remote_url plugin: it downloads a file from a URL through a
// [Downloader] seam and attaches it to the cache storage.
type RemoteURL struct {
	// Download fetches the bytes; required.
	Download Downloader
	// MaxSize caps the downloaded size in bytes; 0 means unlimited. A body
	// larger than MaxSize yields [ErrRemoteURLTooLarge].
	MaxSize int64
}

// AssignTo downloads url and sets the result as a's (changed) cached
// attachment, mirroring `Attacher#assign_remote_url`. The downloaded filename
// seeds the "filename" metadata unless opts already carries one. A download
// failure is wrapped as [ErrRemoteURLDownload]; an over-large body is
// [ErrRemoteURLTooLarge].
func (r *RemoteURL) AssignTo(a *Attacher, url string, opts *UploadOptions) error {
	body, filename, err := r.Download(url)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRemoteURLDownload, err)
	}
	defer body.Close()

	var reader io.Reader = body
	if r.MaxSize > 0 {
		reader = io.LimitReader(body, r.MaxSize+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if r.MaxSize > 0 && int64(len(data)) > r.MaxSize {
		return ErrRemoteURLTooLarge
	}

	if opts == nil {
		opts = &UploadOptions{}
	}
	if opts.Filename == "" && (opts.Metadata == nil || opts.Metadata["filename"] == nil) {
		opts.Filename = filename
	}
	file, err := a.cache.Upload(bytes.NewReader(data), opts)
	if err != nil {
		return err
	}
	a.change(file)
	return nil
}
