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

/*
Package cloud contains a library and tools for open cloud development in Go.

The Go Cloud Development Kit (Go CDK) allows application developers to
seamlessly deploy cloud applications on any combination of cloud providers.
It does this by providing stable, idiomatic interfaces for common uses like
storage and databases. Think `database/sql` for cloud products.

At the core of the Go CDK are common "portable types" implemented by cloud
providers. For example, objects of the blob.Bucket portable type can be created
using gcsblob.OpenBucket, s3blob.OpenBucket, or any other provider. Then, the
blob.Bucket can be used throughout your application without worrying about
the underlying implementation.

The Go CDK works well with a code generator called Wire
(https://github.com/google/wire/blob/master/README.md). It creates
human-readable code that only imports the cloud SDKs for providers you use. This
allows the Go CDK to grow to support any number of cloud providers, without
increasing compile times or binary sizes, and avoiding any side effects from
`init()` functions.

For non-reference documentation, see https://github.com/eliben/gocdkx/

URLs

See https://github.com/eliben/gocdkx/concepts/urls/ for a discussion of URLs in the Go CDK.

Escaping the abstraction

It is not feasible or desirable for APIs like blob.Bucket to encompass the full
functionality of every provider. Rather, we intend to provide a subset of the
most commonly used functionality. There will be cases where a developer wants
to access provider-specific functionality, such as unexposed APIs or data
fields, errors or options. This can be accomplished using As functions.


As

As functions in the APIs provide the user a way to escape the Go CDK
abstraction to access provider-specific types. They might be used as an interim
solution until a feature request to the Go CDK is implemented. Or, the Go CDK
may choose not to support specific features, and the use of As will be
permanent.

Using As implies that the resulting code is no longer portable; the
provider-specific code will need to be ported in order to switch providers.
Therefore, it should be avoided if possible.

Each API will include examples demonstrating how to use its various As
functions, and each provider implementation will document what types it
supports for each.

Usage:

1. Declare a variable of the provider-specific type you want to access.

2. Pass a pointer to it to As.

3. If the type is supported, As will return true and copy the
provider-specific type into your variable. Otherwise, it will return false.

Provider-specific types that are intended to be mutable will be exposed
as a pointer to the underlying type.
*/
package cloud // import "github.com/eliben/gocdkx"
