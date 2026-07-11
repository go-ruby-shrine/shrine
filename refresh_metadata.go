// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

// RefreshMetadata re-reads the file's bytes and recomputes its metadata in
// place, mirroring `Shrine::UploadedFile#refresh_metadata!` (the
// refresh_metadata plugin). Size and mime_type are re-sniffed from the actual
// content and every registered add_metadata extractor (e.g. store_dimensions'
// width/height, signature's digest) is re-run; the existing filename is
// preserved (it is context-derived, not present in the bytes), and any custom
// keys not recomputed are kept.
//
// It is the trust boundary for client-supplied cached files: recomputing the
// metadata from the stored bytes discards any values a client may have forged.
func (f *UploadedFile) RefreshMetadata() error {
	up, err := f.shrine.Uploader(f.StorageKey)
	if err != nil {
		return err
	}
	data, err := f.Download()
	if err != nil {
		return err
	}
	fresh := up.extractMetadata(data, &UploadOptions{Filename: f.Metadata.Filename()})
	for k, v := range fresh {
		f.Metadata[k] = v
	}
	return nil
}
