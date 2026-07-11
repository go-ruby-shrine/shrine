// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ErrMissingDimension is returned (wrapped) by the dimension validators when the
// file has no width/height metadata (store_dimensions was not run). It mirrors
// the gem raising `Shrine::Error "width metadata is missing"`.
var ErrMissingDimension = fmt.Errorf("shrine: dimension metadata is missing")

// Validation accumulates validation error messages for one [UploadedFile],
// mirroring the validation_helpers plugin's DSL. Each check appends a
// gem-identical message to [Validation.Errors] and returns whether the file
// passed that check. Wire the collected Errors back through
// [Attacher.Validate]:
//
//	att.Validate = func(f *shrine.UploadedFile) []string {
//		v := shrine.NewValidation(f)
//		v.MaxSize(5 << 20)
//		v.Extension([]string{"jpg", "png"})
//		v.MimeType([]string{"image/jpeg", "image/png"})
//		return v.Errors
//	}
type Validation struct {
	file   *UploadedFile
	Errors []string
}

// NewValidation returns a [Validation] bound to f.
func NewValidation(f *UploadedFile) *Validation {
	return &Validation{file: f}
}

// fail records msg and returns false.
func (v *Validation) fail(msg string) bool {
	v.Errors = append(v.Errors, msg)
	return false
}

// MaxSize validates that the file is at most max bytes, mirroring
// `validate_max_size`. The message reports the limit in human units.
func (v *Validation) MaxSize(max int64) bool {
	if v.file.Size() > max {
		return v.fail("size must not be greater than " + humanBytes(max))
	}
	return true
}

// MinSize validates that the file is at least min bytes, mirroring
// `validate_min_size`.
func (v *Validation) MinSize(min int64) bool {
	if v.file.Size() < min {
		return v.fail("size must not be less than " + humanBytes(min))
	}
	return true
}

// Extension validates that the filename's extension is one of allowed
// (compared case-insensitively), mirroring `validate_extension`. A file with no
// extension (including a null filename) fails.
func (v *Validation) Extension(allowed []string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(v.file.Filename()), "."))
	for _, a := range allowed {
		if strings.ToLower(a) == ext && ext != "" {
			return true
		}
	}
	return v.fail("extension must be one of: " + strings.Join(allowed, ", "))
}

// MimeType validates that the mime_type metadata is one of allowed (compared
// case-sensitively, as the gem does), mirroring `validate_mime_type`. A null
// mime_type fails.
func (v *Validation) MimeType(allowed []string) bool {
	mt := v.file.MimeType()
	for _, a := range allowed {
		if a == mt && mt != "" {
			return true
		}
	}
	return v.fail("type must be one of: " + strings.Join(allowed, ", "))
}

// dimension returns the named dimension metadata, or an [ErrMissingDimension]
// when it is absent/null.
func (v *Validation) dimension(key string) (int, error) {
	if v.file.Metadata[key] == nil {
		return 0, fmt.Errorf("%w: %s", ErrMissingDimension, key)
	}
	return v.file.Metadata.Int(key), nil
}

// MaxWidth validates that the image is at most max pixels wide, mirroring
// `validate_max_width`. It errors if width metadata is missing.
func (v *Validation) MaxWidth(max int) (bool, error) {
	w, err := v.dimension("width")
	if err != nil {
		return false, err
	}
	if w > max {
		return v.fail(fmt.Sprintf("width must not be greater than %dpx", max)), nil
	}
	return true, nil
}

// MinWidth validates that the image is at least min pixels wide, mirroring
// `validate_min_width`.
func (v *Validation) MinWidth(min int) (bool, error) {
	w, err := v.dimension("width")
	if err != nil {
		return false, err
	}
	if w < min {
		return v.fail(fmt.Sprintf("width must not be less than %dpx", min)), nil
	}
	return true, nil
}

// MaxHeight validates that the image is at most max pixels tall, mirroring
// `validate_max_height`.
func (v *Validation) MaxHeight(max int) (bool, error) {
	h, err := v.dimension("height")
	if err != nil {
		return false, err
	}
	if h > max {
		return v.fail(fmt.Sprintf("height must not be greater than %dpx", max)), nil
	}
	return true, nil
}

// MinHeight validates that the image is at least min pixels tall, mirroring
// `validate_min_height`.
func (v *Validation) MinHeight(min int) (bool, error) {
	h, err := v.dimension("height")
	if err != nil {
		return false, err
	}
	if h < min {
		return v.fail(fmt.Sprintf("height must not be less than %dpx", min)), nil
	}
	return true, nil
}

// MaxDimensions validates width and height against the [w, h] ceiling,
// mirroring `validate_max_dimensions`. It errors if either dimension is missing.
func (v *Validation) MaxDimensions(max [2]int) (bool, error) {
	w, h, err := v.bothDimensions()
	if err != nil {
		return false, err
	}
	if w > max[0] || h > max[1] {
		return v.fail(fmt.Sprintf("dimensions must not be greater than %dx%d", max[0], max[1])), nil
	}
	return true, nil
}

// MinDimensions validates width and height against the [w, h] floor, mirroring
// `validate_min_dimensions`.
func (v *Validation) MinDimensions(min [2]int) (bool, error) {
	w, h, err := v.bothDimensions()
	if err != nil {
		return false, err
	}
	if w < min[0] || h < min[1] {
		return v.fail(fmt.Sprintf("dimensions must not be less than %dx%d", min[0], min[1])), nil
	}
	return true, nil
}

// bothDimensions returns width and height, or an [ErrMissingDimension] if either
// is absent.
func (v *Validation) bothDimensions() (int, int, error) {
	w, err := v.dimension("width")
	if err != nil {
		return 0, 0, err
	}
	h, err := v.dimension("height")
	if err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

// humanBytes renders n as the gem's validation messages do: base-1024 units
// (B, KB, MB, …) with one decimal place, e.g. 1536 → "1.5 KB", 0 → "0.0 B".
func humanBytes(n int64) string {
	const unit = 1024.0
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	size := float64(n)
	i := 0
	for size >= unit && i < len(units)-1 {
		size /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", size, units[i])
}
