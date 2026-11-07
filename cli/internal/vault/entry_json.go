// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package vault

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// The app and the extension write keys into the encrypted entry JSON that the
// CLI does not model (and future clients will add more). EntryData and
// CustomField therefore keep every unknown key in an unexported map and write
// it back on encode, so editing an entry from the CLI never drops data.

var (
	entryDataKeys   = jsonKeys(reflect.TypeFor[EntryData]())
	customFieldKeys = jsonKeys(reflect.TypeFor[CustomField]())
)

// UnmarshalJSON decodes the modelled keys into the struct and keeps the rest.
func (d *EntryData) UnmarshalJSON(b []byte) error {
	if isJSONNull(b) {
		return nil
	}
	type plain EntryData // no methods → default decoding, no recursion
	var p plain
	extra, err := decodeKnown(b, entryDataKeys, &p)
	if err != nil {
		return err
	}
	p.extra = extra
	*d = EntryData(p)
	return nil
}

// MarshalJSON encodes the struct plus the unknown keys kept from decoding.
func (d EntryData) MarshalJSON() ([]byte, error) {
	type plain EntryData
	return encodeWithExtra(plain(d), d.extra, entryDataKeys)
}

// UnmarshalJSON decodes the modelled keys into the struct and keeps the rest.
func (cf *CustomField) UnmarshalJSON(b []byte) error {
	if isJSONNull(b) {
		return nil
	}
	type plain CustomField
	var p plain
	extra, err := decodeKnown(b, customFieldKeys, &p)
	if err != nil {
		return err
	}
	p.extra = extra
	*cf = CustomField(p)
	return nil
}

// MarshalJSON encodes the struct plus the unknown keys kept from decoding.
func (cf CustomField) MarshalJSON() ([]byte, error) {
	type plain CustomField
	return encodeWithExtra(plain(cf), cf.extra, customFieldKeys)
}

// UnknownField is a key of the entry JSON the CLI does not model, with its value
// as text: a string as it is, anything else as its JSON.
type UnknownField struct {
	Key   string
	Value string
}

// UnknownFields lists what the entry holds beyond the modelled keys, for formats
// that have to write it down, sorted by key. An unknown key of a custom field
// follows as "<label>.<key>". A key without a value (null or "") has nothing to
// keep and is left out.
func (d EntryData) UnknownFields() []UnknownField {
	out := unknownFields("", d.extra)
	for _, cf := range d.CustomFields {
		out = append(out, unknownFields(cf.Label+".", cf.extra)...)
	}
	return out
}

func unknownFields(prefix string, extra map[string]json.RawMessage) []UnknownField {
	out := make([]UnknownField, 0, len(extra))
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		value := string(extra[key])
		var text *string
		if json.Unmarshal(extra[key], &text) == nil {
			value = ""
			if text != nil {
				value = *text
			}
		}
		if value != "" {
			out = append(out, UnknownField{Key: prefix + key, Value: value})
		}
	}
	return out
}

// decodeKnown decodes the keys of the JSON object b that are in known into dst
// and returns all other keys. Keys match exactly, like in the Dart and
// TypeScript clients — encoding/json on its own would also fold "Username"
// into the username field.
func decodeKnown(b []byte, known map[string]bool, dst any) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	modelled := make(map[string]json.RawMessage, len(known))
	for k, v := range raw {
		if known[k] {
			modelled[k] = v
			delete(raw, k)
		}
	}
	mb, err := json.Marshal(modelled)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(mb, dst); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return raw, nil
}

// encodeWithExtra encodes v and merges extra into the resulting object. A key
// the struct models is never taken from extra, so the struct always wins
// (clearing a field cannot resurrect an old value).
func encodeWithExtra(v any, extra map[string]json.RawMessage, known map[string]bool) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil || len(extra) == 0 {
		return b, err
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(b, &merged); err != nil {
		return nil, err
	}
	for k, raw := range extra {
		if !known[k] {
			merged[k] = raw
		}
	}
	return json.Marshal(merged)
}

// jsonKeys returns the object keys the exported fields of struct type t
// encode to.
func jsonKeys(t reflect.Type) map[string]bool {
	keys := make(map[string]bool, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if !f.IsExported() || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		keys[name] = true
	}
	return keys
}

func isJSONNull(b []byte) bool {
	return bytes.Equal(bytes.TrimSpace(b), []byte("null"))
}
