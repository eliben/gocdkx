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

package runtimevar_test

import (
	"context"
	"fmt"
	"log"

	"github.com/eliben/gocdkx/runtimevar"
	_ "github.com/eliben/gocdkx/runtimevar/constantvar"
)

func ExampleOpenVariable() {
	// Connect to a Variable using a URL.
	// This example uses "constantvar", an in-memory implementation.
	// We need to add a blank import line to register the constantvar provider's
	// URLOpener, which implements runtimevar.VariableURLOpener:
	// import _ "github.com/eliben/gocdkx/runtimevar/constantvar"
	// constantvar registers for the "constant" scheme.
	// All runtimevar.OpenVariable URLs also work with "runtimevar+" or "runtimevar+variable+" prefixes,
	// e.g., "runtimevar+constant://..." or "runtimevar+variable+constant://...".
	ctx := context.Background()
	v, err := runtimevar.OpenVariable(ctx, "constant://?val=hello+world&decoder=string")
	if err != nil {
		log.Fatal(err)
	}

	// Now we can use the Variable as normal.
	snapshot, err := v.Latest(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// It's safe to cast the Value to string since we used the string decoder.
	fmt.Printf("%s\n", snapshot.Value.(string))

	// Output:
	// hello world
}
