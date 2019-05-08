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

package mongodocstore

import (
	"context"
	"fmt"
	"io"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"github.com/eliben/gocdkx/internal/docstore/driver"
)

func (c *collection) RunGetQuery(ctx context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	opts := options.Find()
	if len(q.FieldPaths) > 0 {
		opts.Projection = c.projectionDoc(q.FieldPaths)
	}
	if q.Limit > 0 {
		lim := int64(q.Limit)
		opts.Limit = &lim
	}
	filter := bson.D{} // must be a zero-length slice, not nil
	for _, f := range q.Filters {
		bf, err := c.filterToBSON(f)
		if err != nil {
			return nil, err
		}
		filter = append(filter, bf)
	}
	cursor, err := c.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("Find: %v", err)
	}
	return &docIterator{cursor: cursor, idField: c.idField, ctx: ctx}, nil
}

var mongoQueryOps = map[string]string{
	driver.EqualOp: "$eq",
	">":            "$gt",
	">=":           "$gte",
	"<":            "$lt",
	"<=":           "$lte",
}

// filtersToBSON converts a []driver.Filter to the MongoDB equivalent, expressed
// as a bson.D (list of key-value pairs).
func (c *collection) filtersToBSON(fs []driver.Filter) (bson.D, error) {
	filter := bson.D{} // must be a zero-length slice, not nil
	for _, f := range fs {
		bf, err := c.filterToBSON(f)
		if err != nil {
			return nil, err
		}
		filter = append(filter, bf)
	}
	return filter, nil
}

// filterToBSON converts a driver.Filter to the MongoDB equivalent, expressed
// as a bson.E (key-value pair).
// The MongoDB document corresponding to "field op value" is
//   {field: {mop: value}}
// where mop is the mongo version of op (see the mongoQueryOps map above).
func (c *collection) filterToBSON(f driver.Filter) (bson.E, error) {
	key := c.toMongoFieldPath(f.FieldPath)
	if c.idField != "" && key == c.idField {
		key = mongoIDField
	}
	val, err := encodeValue(f.Value)
	if err != nil {
		return bson.E{}, err
	}
	op := mongoQueryOps[f.Op]
	if op == "" {
		return bson.E{}, fmt.Errorf("no mongo operator for %q", f.Op)
	}
	return bson.E{Key: key, Value: bson.D{{Key: op, Value: val}}}, nil
}

type docIterator struct {
	cursor  *mongo.Cursor
	idField string
	ctx     context.Context // remember for Stop
}

func (it *docIterator) Next(ctx context.Context, doc driver.Document) error {
	m, err := it.nextMap(ctx)
	if err != nil {
		return err
	}
	return decodeDoc(m, doc, it.idField)
}

func (it *docIterator) nextMap(ctx context.Context) (map[string]interface{}, error) {
	if !it.cursor.Next(ctx) {
		if it.cursor.Err() != nil {
			return nil, it.cursor.Err()
		}
		return nil, io.EOF
	}
	var m map[string]interface{}
	if err := it.cursor.Decode(&m); err != nil {
		return nil, fmt.Errorf("cursor.Decode: %v", err)
	}
	return m, nil
}

func (it *docIterator) Stop() {
	// Ignore error on Close.
	_ = it.cursor.Close(it.ctx)
}

func (it *docIterator) As(i interface{}) bool {
	p, ok := i.(**mongo.Cursor)
	if !ok {
		return false
	}
	*p = it.cursor
	return true
}

func (c *collection) QueryPlan(q *driver.Query) (string, error) {
	return "unknown", nil
}

func (c *collection) RunDeleteQuery(ctx context.Context, q *driver.Query) error {
	filter, err := c.filtersToBSON(q.Filters)
	if err != nil {
		return err
	}
	_, err = c.coll.DeleteMany(ctx, filter, nil)
	return err
}
