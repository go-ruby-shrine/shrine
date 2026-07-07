// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import "errors"

// ErrUnknownStorage is returned (wrapped) when a storage name is not present in
// the registry — for example when rehydrating an [UploadedFile] whose "storage"
// key is not registered, or when asking for an [Uploader] on an unknown name.
// It mirrors the gem's Shrine::Error "storage :name isn't registered".
var ErrUnknownStorage = errors.New("shrine: unknown storage")

// ErrNotFound is returned (wrapped) when opening an id that does not exist in a
// storage.
var ErrNotFound = errors.New("shrine: file not found")
