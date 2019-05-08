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

package dynamodocstore

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/eliben/gocdkx/gcerrors"
	"github.com/eliben/gocdkx/internal/docstore"
	"github.com/eliben/gocdkx/internal/docstore/driver"
	"github.com/eliben/gocdkx/internal/docstore/drivertest"
	"github.com/eliben/gocdkx/internal/testing/setup"
)

// To create the tables and indexes needed for these tests, run create_tables.sh in
// this directory.
//
// The docstore-test-2 table is set up to work with queries on the drivertest.HighScore
// struct like so:
//   table:        "Game" partition key, "Player" sort key
//   local index:  "Game" partition key, "Score" sort key
//   global index: "Player" partition key, "Time" sort key
// The conformance test queries should exercise all of these.

const (
	region          = "us-east-2"
	collectionName1 = "docstore-test-1"
	collectionName2 = "docstore-test-2"
)

type harness struct {
	sess   *session.Session
	closer func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	sess, _, done, state := setup.NewAWSSession(ctx, t, region)
	drivertest.MakeUniqueStringDeterministicForTesting(state)
	return &harness{sess: sess, closer: done}, nil
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) MakeCollection(context.Context) (driver.Collection, error) {
	return newCollection(dyn.New(h.sess), collectionName1, drivertest.KeyField, "")
}

func (h *harness) MakeTwoKeyCollection(context.Context) (driver.Collection, error) {
	return newCollection(dyn.New(h.sess), collectionName2, "Game", "Player")
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) CollectionCheck(coll *docstore.Collection) error {
	var db *dyn.DynamoDB
	if !coll.As(&db) {
		return errors.New("Collection.As failed")
	}
	return nil
}

func (verifyAs) BeforeDo(as func(i interface{}) bool) error {
	return nil
}

func (verifyAs) BeforeQuery(as func(i interface{}) bool) error {
	var si *dyn.ScanInput
	var qi *dyn.QueryInput
	switch {
	case as(&si):
		si.ConsistentRead = aws.Bool(true)
	case as(&qi):
		qi.ConsistentRead = aws.Bool(true)
	default:
		return errors.New("Query.BeforeQuery failed")
	}
	return nil
}

func (verifyAs) QueryCheck(it *docstore.DocumentIterator) error {
	var so *dyn.ScanOutput
	var qo *dyn.QueryOutput
	if !it.As(&so) && !it.As(&qo) {
		return errors.New("DocumentIterator.As failed")
	}
	return nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, &codecTester{}, []drivertest.AsTest{verifyAs{}})
}

// Dynamodocstore-specific tests.

func TestQueryErrors(t *testing.T) {
	// Verify that bad queries return the right errors.
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	dc, err := h.MakeTwoKeyCollection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)

	// Here we are comparing a key field with the wrong type. DynamoDB cares about this
	// because even though it's a document store and hence schemaless, the key fields
	// do have a schema (that is, they have known, fixed types).
	iter := coll.Query().Where("Game", "=", 1).Get(ctx)
	defer iter.Stop()
	err = iter.Next(ctx, &h)
	if c := gcerrors.Code(err); c != gcerrors.InvalidArgument {
		t.Errorf("got %v (code %s, type %T), want InvalidArgument", err, c, err)
	}
}

func TestProcessURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"dynamodb://docstore-test?partition_key=_kind", false},
		// OK.
		{"dynamodb://docstore-test?partition_key=_kind&sort_key=_id", false},
		// OK, overriding region.
		{"dynamodb://docstore-test?partition_key=_kind&region=" + region, false},
		// Unknown parameter.
		{"dynamodb://docstore-test?partition_key=_kind&param=value", true},
		// With path.
		{"dynamodb://docstore-test/subcoll?partition_key=_kind", true},
		// Missing partition_key.
		{"dynamodb://docstore-test?sort_key=_id", true},
	}

	sess, err := session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable})
	if err != nil {
		t.Fatal(err)
	}
	o := &URLOpener{ConfigProvider: sess}
	for _, test := range tests {
		u, err := url.Parse(test.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _, err = o.processURL(u)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
