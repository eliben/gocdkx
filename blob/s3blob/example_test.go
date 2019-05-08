// Copyright 2018 The Go Cloud Development Kit Authors
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

package s3blob_test

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/eliben/gocdkx/blob"
	"github.com/eliben/gocdkx/blob/s3blob"
)

func Example() {
	// This example is used in https://github.com/eliben/gocdkx/howto/blob/open-bucket/#s3-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	// The region must match the region for "my-bucket".
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-1"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create a *blob.Bucket.
	bucket, err := s3blob.OpenBucket(ctx, sess, "my-bucket", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()
}

func Example_openBucket() {
	// This example is used in https://github.com/eliben/gocdkx/howto/blob/open-bucket/#s3

	// import _ "github.com/eliben/gocdkx/blob/s3blob"

	// Variables set up elsewhere:
	ctx := context.Background()

	// OpenBucket creates a *blob.Bucket from a URL.
	bucket, err := blob.OpenBucket(ctx, "s3://my-bucket?region=us-west-1")
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()
}
