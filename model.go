// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

// Record is the ORM-row seam the activerecord and sequel glue plug into. It
// abstracts reading and writing a single attachment column on a persisted
// record. The pure-Go core has no ORM, so the host adapts its model rows to
// this interface; the actual save/destroy *callbacks* (which is all that
// distinguishes the gem's `activerecord` and `sequel` plugins) are wired
// host-side around [ModelAttacher.Save] / [ModelAttacher.Destroy].
type Record interface {
	// ReadAttachment returns the serialized column value and whether it is
	// present/non-null.
	ReadAttachment(column string) (value string, present bool)
	// WriteAttachment stores the serialized column value ("" for SQL NULL).
	WriteAttachment(column string, value string)
}

// ModelAttacher binds an [Attacher] to a [Record] column, mirroring the model
// attachment the activerecord/sequel plugins install. It reads the column on
// load, promotes on save and clears on destroy, persisting the serialized data
// (including derivatives) back through the [Record] seam.
type ModelAttacher struct {
	*Attacher
	record Record
	column string
}

// NewActiveRecord binds a [ModelAttacher] for record's column, using the named
// cache/store storages, and loads the current column value. It is the
// activerecord plugin's model attachment; the difference from [NewSequel] is
// only the host-side callback wiring. It returns a wrapped [ErrUnknownStorage]
// if a storage is missing, or a load error for a malformed column value.
func (s *Shrine) NewActiveRecord(cacheKey, storeKey string, record Record, column string) (*ModelAttacher, error) {
	return s.newModelAttacher(cacheKey, storeKey, record, column)
}

// NewSequel is the sequel plugin's model attachment. It is identical to
// [Shrine.NewActiveRecord] at the pure-Go layer (see [Record]).
func (s *Shrine) NewSequel(cacheKey, storeKey string, record Record, column string) (*ModelAttacher, error) {
	return s.newModelAttacher(cacheKey, storeKey, record, column)
}

// newModelAttacher builds a [ModelAttacher] and loads the record's column.
func (s *Shrine) newModelAttacher(cacheKey, storeKey string, record Record, column string) (*ModelAttacher, error) {
	att, err := s.Attacher(cacheKey, storeKey)
	if err != nil {
		return nil, err
	}
	m := &ModelAttacher{Attacher: att, record: record, column: column}
	if err := m.Load(); err != nil {
		return nil, err
	}
	return m, nil
}

// Load reads the record's attachment column into the attacher, mirroring the
// gem loading the attachment when a record is instantiated.
func (m *ModelAttacher) Load() error {
	value, present := m.record.ReadAttachment(m.column)
	if !present {
		value = ""
	}
	return m.LoadColumn(value)
}

// Save finalizes the attachment (promotes a cached file to store, deletes any
// replaced file) and writes the new serialized column value back to the record,
// mirroring the gem's before/after-save callbacks. It is a no-op when nothing
// changed.
func (m *ModelAttacher) Save() error {
	if !m.Changed() {
		return nil
	}
	if err := m.Finalize(); err != nil {
		return err
	}
	data, err := m.ColumnData()
	if err != nil {
		return err
	}
	m.record.WriteAttachment(m.column, data)
	return nil
}

// Destroy deletes the attachment (and its derivatives) and clears the record's
// column, mirroring the gem's after-destroy callback.
func (m *ModelAttacher) Destroy() error {
	if err := m.DeleteDerivatives(); err != nil {
		return err
	}
	if err := m.Attacher.Destroy(); err != nil {
		return err
	}
	m.record.WriteAttachment(m.column, "")
	return nil
}
