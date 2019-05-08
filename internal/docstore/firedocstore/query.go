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

// TODO(jba): figure out how to get filters with complex values to work (since they
// are represented as arrays of floats). Also, uints: since they are represented as
// int64s, the sign is wrong. Since you can only compare complex numbers for
// equality, maybe it could work if Firestore arrays can be compared for equality.

package firedocstore

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/big"
	"path"
	"reflect"
	"strings"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/eliben/gocdkx/internal/docstore/driver"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

func (c *collection) RunGetQuery(ctx context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	return c.newDocIterator(ctx, q)
}

func (c *collection) newDocIterator(ctx context.Context, q *driver.Query) (*docIterator, error) {
	sq, localFilters, err := c.queryToProto(q)
	if err != nil {
		return nil, err
	}
	req := &pb.RunQueryRequest{
		Parent:    path.Dir(c.collPath),
		QueryType: &pb.RunQueryRequest_StructuredQuery{sq},
	}
	if q.BeforeQuery != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**pb.RunQueryRequest)
			if !ok {
				return false
			}
			*p = req
			return true
		}
		if err := q.BeforeQuery(asFunc); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	sc, err := c.client.RunQuery(ctx, req)
	if err != nil {
		return nil, err
	}
	return &docIterator{
		streamClient: sc,
		nameField:    c.nameField,
		localFilters: localFilters,
		cancel:       cancel,
	}, nil
}

////////////////////////////////////////////////////////////////
// The code below is adapted from cloud.google.com/go/firestore.

type docIterator struct {
	streamClient pb.Firestore_RunQueryClient
	nameField    string
	localFilters []driver.Filter
	// We call cancel to make sure the stream client doesn't leak resources.
	// We don't need to call it if Recv() returns a non-nil error.
	// See https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
	cancel func()
}

func (it *docIterator) Next(ctx context.Context, doc driver.Document) error {
	res, err := it.nextResponse(ctx)
	if err != nil {
		return err
	}
	return decodeDoc(res.Document, doc, it.nameField)
}

func (it *docIterator) nextResponse(ctx context.Context) (*pb.RunQueryResponse, error) {
	for {
		res, err := it.streamClient.Recv()
		if err != nil {
			return nil, err
		}
		// No document => partial progress; keep receiving.
		if res.Document == nil {
			continue
		}
		match, err := it.evaluateLocalFilters(res.Document)
		if err != nil {
			return nil, err
		}
		if match {
			return res, nil
		}
	}
}

// Report whether the filters are true of the document.
func (it *docIterator) evaluateLocalFilters(pdoc *pb.Document) (bool, error) {
	if len(it.localFilters) == 0 {
		return true, nil
	}
	// TODO(jba): optimization: evaluate the filter directly on the proto document, without decoding.
	m := map[string]interface{}{}
	doc, err := driver.NewDocument(m)
	if err != nil {
		return false, err
	}
	if err := decodeDoc(pdoc, doc, it.nameField); err != nil {
		return false, err
	}
	for _, f := range it.localFilters {
		if !evaluateFilter(f, doc) {
			return false, nil
		}
	}
	return true, nil
}

func evaluateFilter(f driver.Filter, doc driver.Document) bool {
	val, err := doc.Get(f.FieldPath)
	if err != nil {
		// Treat a missing field as false.
		return false
	}
	lhs := reflect.ValueOf(val)
	rhs := reflect.ValueOf(f.Value)
	if lhs.Kind() == reflect.String {
		if rhs.Kind() != reflect.String {
			return false
		}
		return applyComparison(f.Op, strings.Compare(lhs.String(), rhs.String()))
	}
	// Compare numbers by using big.Float. This is expensive
	// but simpler to code and more clearly correct. In particular,
	// it will get the right answer for some mixed-type comparisons
	// that are hard to do otherwise. For example, comparing the max int64
	// with a float64: float64(math.MaxInt64) == float64(math.MaxInt64-1)
	// is true in Go.
	// TODO(jba): handle complex
	lf := toBigFloat(lhs)
	rf := toBigFloat(rhs)
	// If either one is not a number, return false.
	if lf == nil || rf == nil {
		return false
	}
	return applyComparison(f.Op, lf.Cmp(rf))
}

// op is one of the five permitted docstore operators ("=", "<", etc.)
// c is the result of strings.Compare or the like.
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

func (it *docIterator) Stop() { it.cancel() }

func (it *docIterator) As(i interface{}) bool {
	p, ok := i.(*pb.Firestore_RunQueryClient)
	if !ok {
		return false
	}
	*p = it.streamClient
	return true
}

// Converts the query to a Firestore proto. Also returns filters that need to be
// evaluated on the client.
func (c *collection) queryToProto(q *driver.Query) (*pb.StructuredQuery, []driver.Filter, error) {
	// The collection ID is the last component of the collection path.
	collID := path.Base(c.collPath)
	p := &pb.StructuredQuery{
		From: []*pb.StructuredQuery_CollectionSelector{{CollectionId: collID}},
	}
	if len(q.FieldPaths) > 0 {
		p.Select = &pb.StructuredQuery_Projection{}
		for _, fp := range q.FieldPaths {
			p.Select.Fields = append(p.Select.Fields, fieldRef(fp))
		}
	}
	if q.Limit > 0 {
		p.Limit = &wrappers.Int32Value{Value: int32(q.Limit)}
	}

	// TODO(jba): make sure we retrieve the fields needed for local filters.
	sendFilters, localFilters := splitFilters(q.Filters)
	// If there is only one filter, use it directly. Otherwise, construct
	// a CompositeFilter.
	var pfs []*pb.StructuredQuery_Filter
	for _, f := range sendFilters {
		pf, err := filterToProto(f)
		if err != nil {
			return nil, nil, err
		}
		pfs = append(pfs, pf)
	}
	if len(pfs) == 1 {
		p.Where = pfs[0]
	} else if len(pfs) > 1 {
		p.Where = &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_CompositeFilter{&pb.StructuredQuery_CompositeFilter{
				Op:      pb.StructuredQuery_CompositeFilter_AND,
				Filters: pfs,
			}},
		}
	}
	// TODO(jba): order
	// TODO(jba): cursors (start/end)
	return p, localFilters, nil
}

// splitFilters separates the list of query filters into those we can send to the Firestore service,
// and those we must evaluate here on the client.
func splitFilters(fs []driver.Filter) (sendToFirestore, evaluateLocally []driver.Filter) {
	// Enforce that only one field can have an inequality.
	var rangeFP []string
	for _, f := range fs {
		if f.Op == driver.EqualOp {
			sendToFirestore = append(sendToFirestore, f)
		} else {
			if rangeFP == nil || fpEqual(rangeFP, f.FieldPath) {
				// Multiple inequality filters on the same field are OK.
				rangeFP = f.FieldPath
				sendToFirestore = append(sendToFirestore, f)
			} else {
				evaluateLocally = append(evaluateLocally, f)
			}
		}
	}
	return sendToFirestore, evaluateLocally
}

func filterToProto(f driver.Filter) (*pb.StructuredQuery_Filter, error) {
	// "= nil" and "= NaN" are handled specially.
	if uop, ok := unaryOpFor(f.Value); ok {
		if f.Op != driver.EqualOp {
			return nil, fmt.Errorf("firestore: must use '=' when comparing %v", f.Value)
		}
		return &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_UnaryFilter{
				UnaryFilter: &pb.StructuredQuery_UnaryFilter{
					OperandType: &pb.StructuredQuery_UnaryFilter_Field{
						Field: fieldRef(f.FieldPath),
					},
					Op: uop,
				},
			},
		}, nil
	}
	var op pb.StructuredQuery_FieldFilter_Operator
	switch f.Op {
	case "<":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN
	case "<=":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN_OR_EQUAL
	case ">":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN
	case ">=":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN_OR_EQUAL
	case driver.EqualOp:
		op = pb.StructuredQuery_FieldFilter_EQUAL
	// TODO(jba): can we support array-contains portably?
	// case "array-contains":
	// 	op = pb.StructuredQuery_FieldFilter_ARRAY_CONTAINS
	default:
		return nil, fmt.Errorf("invalid operator %q", f.Op)
	}
	pv, err := encodeValue(f.Value)
	if err != nil {
		return nil, err
	}
	return &pb.StructuredQuery_Filter{
		FilterType: &pb.StructuredQuery_Filter_FieldFilter{
			FieldFilter: &pb.StructuredQuery_FieldFilter{
				Field: fieldRef(f.FieldPath),
				Op:    op,
				Value: pv,
			},
		},
	}, nil
}

func unaryOpFor(value interface{}) (pb.StructuredQuery_UnaryFilter_Operator, bool) {
	switch {
	case value == nil:
		return pb.StructuredQuery_UnaryFilter_IS_NULL, true
	case isNaN(value):
		return pb.StructuredQuery_UnaryFilter_IS_NAN, true
	default:
		return pb.StructuredQuery_UnaryFilter_OPERATOR_UNSPECIFIED, false
	}
}

func isNaN(x interface{}) bool {
	switch x := x.(type) {
	case float32:
		return math.IsNaN(float64(x))
	case float64:
		return math.IsNaN(x)
	default:
		return false
	}
}

func fieldRef(fp []string) *pb.StructuredQuery_FieldReference {
	return &pb.StructuredQuery_FieldReference{FieldPath: toServiceFieldPath(fp)}
}

func (c *collection) QueryPlan(q *driver.Query) (string, error) {
	return "unknown", nil
}

// For delete and update queries, limit the number of write actions per RPC, to bound
// client memory.
// This is a variable so it can be modified for tests.
var maxWritesPerRPC = 1000

func (c *collection) RunDeleteQuery(ctx context.Context, q *driver.Query) error {
	q.FieldPaths = [][]string{{"__name__"}}
	iter, err := c.newDocIterator(ctx, q)
	if err != nil {
		return err
	}
	defer iter.Stop()

	opts := &driver.RunActionsOptions{}
	var pws []*pb.Write
	for {
		res, err := iter.nextResponse(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		pws = append(pws, &pb.Write{
			Operation: &pb.Write_Delete{Delete: res.Document.Name},
		})
		if len(pws) >= maxWritesPerRPC {
			_, err := c.commit(ctx, pws, opts)
			if err != nil {
				return err
			}
			pws = pws[:0]
		}
	}
	if len(pws) > 0 {
		_, err = c.commit(ctx, pws, opts)
		return err
	}
	return nil
}
