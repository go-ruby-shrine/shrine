// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import "encoding/json"

// columnJSON is the wire shape persisted to a model's attachment column: the
// attached file's {id, storage, metadata} plus an optional "derivatives" map of
// name → {id, storage, metadata}. It matches the gem's derivatives-aware column
// data exactly (the "derivatives" key is omitted when there are none).
type columnJSON struct {
	ID          string            `json:"id"`
	Storage     string            `json:"storage"`
	Metadata    map[string]any    `json:"metadata"`
	Derivatives map[string]ufJSON `json:"derivatives,omitempty"`
}

// ColumnData serialises the current attachment (and any derivatives) to the
// JSON stored in a model's attachment column, mirroring
// `Attacher#column_data`. It returns ("", nil) when nothing is attached, so the
// host writes SQL NULL.
func (a *Attacher) ColumnData() (string, error) {
	if a.file == nil {
		return "", nil
	}
	col := columnJSON{
		ID:       a.file.ID,
		Storage:  a.file.StorageKey,
		Metadata: map[string]any(a.file.Metadata),
	}
	if len(a.derivatives) > 0 {
		col.Derivatives = make(map[string]ufJSON, len(a.derivatives))
		for name, d := range a.derivatives {
			col.Derivatives[name] = ufJSON{
				ID:       d.ID,
				Storage:  d.StorageKey,
				Metadata: map[string]any(d.Metadata),
			}
		}
	}
	b, err := json.Marshal(col)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// LoadColumn replaces the attacher's state from column JSON produced by
// [Attacher.ColumnData] (or the gem), binding the attached file and its
// derivatives to the registry and marking the attacher unchanged (this is a
// load, not an assignment, so no validation runs). An empty string clears the
// attachment. It mirrors `Attacher#load_column`.
func (a *Attacher) LoadColumn(data string) error {
	if data == "" {
		a.file = nil
		a.original = nil
		a.derivatives = nil
		a.changed = false
		a.Errors = nil
		return nil
	}
	var col columnJSON
	if err := json.Unmarshal([]byte(data), &col); err != nil {
		return err
	}
	s := a.shrine()
	file, err := s.hydrate(ufJSON{ID: col.ID, Storage: col.Storage, Metadata: col.Metadata})
	if err != nil {
		return err
	}
	var derivs map[string]*UploadedFile
	if len(col.Derivatives) > 0 {
		derivs = make(map[string]*UploadedFile, len(col.Derivatives))
		for name, raw := range col.Derivatives {
			d, err := s.hydrate(raw)
			if err != nil {
				return err
			}
			derivs[name] = d
		}
	}
	a.file = file
	a.original = file
	a.derivatives = derivs
	a.changed = false
	a.Errors = nil
	return nil
}
