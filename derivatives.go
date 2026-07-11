// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import "io"

// Deriver produces derived files (e.g. thumbnails, transcodes) from the bytes
// of an original, keyed by name, mirroring a block registered with the gem's
// `derivatives_processor`. The returned readers are uploaded to the store
// storage by [Attacher.CreateDerivatives].
type Deriver func(original []byte) (map[string]io.Reader, error)

// AddDerivative uploads r to the store storage and records it as the named
// derivative, mirroring `Attacher#add_derivative(name, io)`. It overwrites any
// existing derivative of the same name (the caller is responsible for deleting
// the replaced file if desired).
func (a *Attacher) AddDerivative(name string, r io.Reader, opts *UploadOptions) error {
	file, err := a.store.Upload(r, opts)
	if err != nil {
		return err
	}
	if a.derivatives == nil {
		a.derivatives = map[string]*UploadedFile{}
	}
	a.derivatives[name] = file
	return nil
}

// CreateDerivatives runs deriver against the current attachment's bytes and
// uploads every produced file to the store storage as a named derivative,
// mirroring `Attacher#create_derivatives`. It is a no-op with no error when
// nothing is attached (the gem raises; here the absence is reported by the
// returned files being unchanged).
func (a *Attacher) CreateDerivatives(deriver Deriver) error {
	if a.file == nil {
		return nil
	}
	data, err := a.file.Download()
	if err != nil {
		return err
	}
	produced, err := deriver(data)
	if err != nil {
		return err
	}
	for name, r := range produced {
		if err := a.AddDerivative(name, r, nil); err != nil {
			return err
		}
	}
	return nil
}

// Derivatives returns the recorded derivatives keyed by name (nil if none),
// mirroring `Attacher#derivatives`.
func (a *Attacher) Derivatives() map[string]*UploadedFile { return a.derivatives }

// Derivative returns the named derivative, or nil if absent, mirroring
// `Attacher#derivatives[name]`.
func (a *Attacher) Derivative(name string) *UploadedFile { return a.derivatives[name] }

// DerivativeURL returns the URL of the named derivative, or "" if absent,
// mirroring `Attacher#url(name)` from the derivatives plugin.
func (a *Attacher) DerivativeURL(name string, opts map[string]any) string {
	f := a.derivatives[name]
	if f == nil {
		return ""
	}
	return f.URL(opts)
}

// DeleteDerivatives deletes every recorded derivative from storage and clears
// the set, mirroring `Attacher#delete_derivatives`. It stops and returns on the
// first delete error.
func (a *Attacher) DeleteDerivatives() error {
	for name, f := range a.derivatives {
		if err := f.Delete(); err != nil {
			return err
		}
		delete(a.derivatives, name)
	}
	return nil
}
