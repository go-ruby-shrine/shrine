// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"bytes"
	"image"

	// Register the standard image decoders so [image.DecodeConfig] recognises
	// PNG/JPEG/GIF headers. Decoding only the config reads the header, not the
	// pixels, so this is cheap.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// DimensionAnalyzer reports the pixel width and height of an image from its
// bytes, and whether they could be determined. It is the seam the
// store_dimensions plugin uses — the analogue of the gem's `:analyzer` option
// (`:fastimage`, `:mini_magick`, `:ruby_vips`, …).
type DimensionAnalyzer func(data []byte) (width, height int, ok bool)

// ImageDimensions is the default [DimensionAnalyzer]: it reads the image header
// via [image.DecodeConfig], recognising the formats whose decoders are
// registered (PNG, JPEG, GIF here). It returns ok=false for non-image or
// unrecognised data, mirroring the gem returning nil dimensions.
func ImageDimensions(data []byte) (int, int, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// StoreDimensions is the store_dimensions plugin: it adds "width" and "height"
// metadata to every upload of an image, and returns null for both on
// non-images — matching the gem, which always defines the keys. A zero Analyzer
// uses [ImageDimensions].
//
//	s.Plugin(&shrine.StoreDimensions{})
//	// later: f.Width(), f.Height(), f.Dimensions()
type StoreDimensions struct {
	// Analyzer overrides the dimension analyzer; nil means [ImageDimensions].
	Analyzer DimensionAnalyzer
}

// Configure registers a [MetadataExtractor] that stores width/height.
func (p *StoreDimensions) Configure(s *Shrine) {
	analyzer := p.Analyzer
	if analyzer == nil {
		analyzer = ImageDimensions
	}
	s.AddMetadata(func(data []byte, _ Metadata) map[string]any {
		w, h, ok := analyzer(data)
		if !ok {
			return map[string]any{"width": nil, "height": nil}
		}
		return map[string]any{"width": w, "height": h}
	})
}

// Width returns the "width" metadata in pixels (the store_dimensions plugin's
// `UploadedFile#width`), or 0 if absent/null.
func (f *UploadedFile) Width() int { return f.Metadata.Int("width") }

// Height returns the "height" metadata in pixels
// (`UploadedFile#height`), or 0 if absent/null.
func (f *UploadedFile) Height() int { return f.Metadata.Int("height") }

// Dimensions returns the [width, height] pair and whether both are present,
// mirroring `UploadedFile#dimensions` (which returns nil when either is
// missing).
func (f *UploadedFile) Dimensions() ([2]int, bool) {
	_, wok := f.Metadata["width"]
	_, hok := f.Metadata["height"]
	if f.Metadata["width"] == nil || f.Metadata["height"] == nil || !wok || !hok {
		return [2]int{}, false
	}
	return [2]int{f.Width(), f.Height()}, true
}
