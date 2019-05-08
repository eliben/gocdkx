// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memdocstore

import (
	"fmt"
	"reflect"

	"github.com/eliben/gocdkx/internal/docstore"
	"github.com/eliben/gocdkx/internal/docstore/driver"
)

// encodeDoc encodes a driver.Document as a map[string]interface{}.
func encodeDoc(doc driver.Document) (map[string]interface{}, error) {
	var e encoder
	if err := doc.Encode(&e); err != nil {
		return nil, err
	}
	return e.val.(map[string]interface{}), nil
}

type encoder struct {
	val interface{}
}

func (e *encoder) EncodeNil()                                { e.val = nil }
func (e *encoder) EncodeBool(x bool)                         { e.val = x }
func (e *encoder) EncodeInt(x int64)                         { e.val = x }
func (e *encoder) EncodeUint(x uint64)                       { e.val = int64(x) }
func (e *encoder) EncodeBytes(x []byte)                      { e.val = x }
func (e *encoder) EncodeFloat(x float64)                     { e.val = x }
func (e *encoder) EncodeComplex(x complex128)                { e.val = x }
func (e *encoder) EncodeString(x string)                     { e.val = x }
func (e *encoder) ListIndex(int)                             { panic("impossible") }
func (e *encoder) MapKey(string)                             { panic("impossible") }
func (e *encoder) EncodeSpecial(reflect.Value) (bool, error) { return false, nil } // no special handling

func (e *encoder) EncodeList(n int) driver.Encoder {
	// All slices and arrays are encoded as []interface{}
	s := make([]interface{}, n)
	e.val = s
	return &listEncoder{s: s}
}

type listEncoder struct {
	s []interface{}
	encoder
}

func (e *listEncoder) ListIndex(i int) { e.s[i] = e.val }

type mapEncoder struct {
	m map[string]interface{}
	encoder
}

func (e *encoder) EncodeMap(n int) driver.Encoder {
	m := make(map[string]interface{}, n)
	e.val = m
	return &mapEncoder{m: m}
}

func (e *mapEncoder) MapKey(k string) { e.m[k] = e.val }

////////////////////////////////////////////////////////////////

// decodeDoc decodes m into ddoc.
func decodeDoc(m map[string]interface{}, ddoc driver.Document, fps [][]string) error {
	var m2 map[string]interface{}
	if len(fps) == 0 {
		m2 = m
	} else {
		// Make a document to decode from that has only the field paths and the revision field.
		// (We don't need the key field because ddoc must already have it.)
		m2 = map[string]interface{}{docstore.RevisionField: m[docstore.RevisionField]}
		for _, fp := range fps {
			val, err := getAtFieldPath(m, fp)
			if err != nil {
				return err
			}
			if err := setAtFieldPath(m2, fp, val); err != nil {
				return err
			}
		}
	}
	return ddoc.Decode(decoder{m2})
}

type decoder struct {
	val interface{}
}

func (d decoder) String() string {
	return fmt.Sprint(d.val)
}

func (d decoder) AsNull() bool {
	return d.val == nil
}

func (d decoder) AsBool() (bool, bool) {
	b, ok := d.val.(bool)
	return b, ok
}

func (d decoder) AsString() (string, bool) {
	s, ok := d.val.(string)
	return s, ok
}

func (d decoder) AsInt() (int64, bool) {
	i, ok := d.val.(int64)
	return i, ok
}

func (d decoder) AsUint() (uint64, bool) {
	i, ok := d.val.(int64)
	return uint64(i), ok
}

func (d decoder) AsFloat() (float64, bool) {
	f, ok := d.val.(float64)
	return f, ok
}

func (d decoder) AsComplex() (complex128, bool) {
	c, ok := d.val.(complex128)
	return c, ok
}

func (d decoder) AsBytes() ([]byte, bool) {
	bs, ok := d.val.([]byte)
	return bs, ok
}

func (d decoder) AsInterface() (interface{}, error) {
	return d.val, nil
}

func (d decoder) ListLen() (int, bool) {
	if s, ok := d.val.([]interface{}); ok {
		return len(s), true
	}
	return 0, false
}

func (d decoder) DecodeList(f func(i int, d2 driver.Decoder) bool) {
	for i, e := range d.val.([]interface{}) {
		if !f(i, decoder{e}) {
			return
		}
	}
}

func (d decoder) MapLen() (int, bool) {
	if m, ok := d.val.(map[string]interface{}); ok {
		return len(m), true
	}
	return 0, false
}

func (d decoder) DecodeMap(f func(key string, d2 driver.Decoder) bool) {
	for k, v := range d.val.(map[string]interface{}) {
		if !f(k, decoder{v}) {
			return
		}
	}
}

func (decoder) AsSpecial(reflect.Value) (bool, interface{}, error) {
	return false, nil, nil
}
