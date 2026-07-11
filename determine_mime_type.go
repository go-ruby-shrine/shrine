// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"mime"
	"net/http"
	"path/filepath"
)

// MIMEAnalyzer sniffs a mime type from the leading bytes of a file. It is the
// seam the determine_mime_type plugin swaps in for [Shrine.DetectMIME] — the
// analogue of the gem's `:analyzer` option (`:file`, `:marcel`, `:mimemagic`,
// `:content_type`, …). Two pure-Go analyzers ship here.
type MIMEAnalyzer func(data []byte) string

// ContentAnalyzer detects the mime type from content via
// [net/http.DetectContentType]. It is the default analyzer and the analogue of
// the gem's content-based `:file`/`:marcel` analyzers.
//
// Differential note vs the gem's default `:file` analyzer: DetectContentType
// appends a charset for text (e.g. "text/plain; charset=utf-8") and never
// returns the empty string — an empty or unrecognised file sniffs as
// "text/plain; charset=utf-8" or "application/octet-stream" rather than the
// gem's nil. The recognised binary signatures (PNG, JPEG, GIF, PDF, ZIP, …)
// agree with the gem.
func ContentAnalyzer(data []byte) string {
	return http.DetectContentType(data)
}

// ExtensionAnalyzerFor returns a [MIMEAnalyzer] that maps the given filename's
// extension to a mime type via [mime.TypeByExtension], falling back to
// [ContentAnalyzer] when the extension is unknown. It mirrors the gem's
// `:mini_mime` (extension-based) analyzer. The analyzer ignores its byte
// argument for the extension lookup, so the filename is captured here.
func ExtensionAnalyzerFor(filename string) MIMEAnalyzer {
	return func(data []byte) string {
		if ext := filepath.Ext(filename); ext != "" {
			if mt := mime.TypeByExtension(ext); mt != "" {
				return mt
			}
		}
		return ContentAnalyzer(data)
	}
}

// DetermineMIMEType is the determine_mime_type plugin: it makes uploads sniff
// the mime type from the file's content (not a client-supplied header) via the
// configured [MIMEAnalyzer]. A zero Analyzer uses [ContentAnalyzer].
//
//	s.Plugin(&shrine.DetermineMIMEType{}) // stdlib content sniffing
type DetermineMIMEType struct {
	// Analyzer overrides the mime seam; nil means [ContentAnalyzer].
	Analyzer MIMEAnalyzer
}

// Configure installs the analyzer as [Shrine.DetectMIME].
func (p *DetermineMIMEType) Configure(s *Shrine) {
	if p.Analyzer != nil {
		s.DetectMIME = p.Analyzer
	} else {
		s.DetectMIME = ContentAnalyzer
	}
}
