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
	"context"
	"io"
	"math/big"
	"reflect"
	"strings"

	"github.com/eliben/gocdkx/internal/docstore/driver"
)

func (c *collection) RunGetQuery(_ context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var docs []map[string]interface{}
	for _, doc := range c.docs {
		if q.Limit > 0 && len(docs) == q.Limit {
			break
		}
		if filtersMatch(q.Filters, doc) {
			docs = append(docs, doc)
		}
	}
	return &docIterator{docs: docs, fieldPaths: q.FieldPaths}, nil
}

func filtersMatch(fs []driver.Filter, doc map[string]interface{}) bool {
	for _, f := range fs {
		if !filterMatches(f, doc) {
			return false
		}
	}
	return true
}

func filterMatches(f driver.Filter, doc map[string]interface{}) bool {
	docval, err := getAtFieldPath(doc, f.FieldPath)
	// missing or bad field path => no match
	if err != nil {
		return false
	}
	c, ok := compare(docval, f.Value)
	if !ok {
		return false
	}
	return applyComparison(f.Op, c)
}

// op is one of the five permitted docstore operators ("=", "<", etc.)
// c is the result of strings.Compare or the like.
// TODO(jba): dedup from firedocstore/query?
func applyComparison(op string, c int) bool {
	switch op {
	case driver.EqualOp:
		return c == 0
	case ">":
		return c > 0
	case "<":
		return c < 0
	case ">=":
		return c >= 0
	case "<=":
		return c <= 0
	default:
		panic("bad op")
	}
}

func compare(x1, x2 interface{}) (int, bool) {
	v1 := reflect.ValueOf(x1)
	v2 := reflect.ValueOf(x2)
	if v1.Kind() == reflect.String && v2.Kind() == reflect.String {
		return strings.Compare(v1.String(), v2.String()), true
	}
	bf1 := toBigFloat(v1)
	bf2 := toBigFloat(v2)
	if bf1 == nil || bf2 == nil {
		return 0, false
	}
	return bf1.Cmp(bf2), true
}

func toBigFloat(x reflect.Value) *big.Float {
	var f big.Float
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt64(x.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		f.SetUint64(x.Uint())
	case reflect.Float32, reflect.Float64:
		f.SetFloat64(x.Float())
	default:
		return nil
	}
	return &f
}

type docIterator struct {
	docs       []map[string]interface{}
	fieldPaths [][]string
	err        error
}

func (it *docIterator) Next(ctx context.Context, doc driver.Document) error {
	if it.err != nil {
		return it.err
	}
	if len(it.docs) == 0 {
		it.err = io.EOF
		return it.err
	}
	if err := decodeDoc(it.docs[0], doc, it.fieldPaths); err != nil {
		it.err = err
		return it.err
	}
	it.docs = it.docs[1:]
	return nil
}

func (it *docIterator) Stop() { it.err = io.EOF }

func (it *docIterator) As(i interface{}) bool { return false }

func (c *collection) QueryPlan(q *driver.Query) (string, error) {
	return "", nil
}

func (c *collection) RunDeleteQuery(ctx context.Context, q *driver.Query) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, doc := range c.docs {
		if filtersMatch(q.Filters, doc) {
			delete(c.docs, key)
		}
	}
	return nil
}
