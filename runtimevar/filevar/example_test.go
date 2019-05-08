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

package filevar_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/eliben/gocdkx/runtimevar"
	"github.com/eliben/gocdkx/runtimevar/filevar"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func Example() {
	// Create a temporary file to hold our config.
	f, err := ioutil.TempFile("", "")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := f.Write([]byte(`{"Server": "foo.com", "Port": 80}`)); err != nil {
		log.Fatal(err)
	}

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a *runtimevar.Variable pointing at f.
	v, err := filevar.OpenVariable(f.Name(), decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	cfg := snapshot.Value.(MyConfig)
	fmt.Printf("%s running on port %d", cfg.Server, cfg.Port)

	// Output:
	// foo.com running on port 80
}

func Example_openVariable() {
	// OpenVariable creates a *runtimevar.Variable from a URL.
	ctx := context.Background()
	v, err := runtimevar.OpenVariable(ctx, "file:///path/to/config.json?decoder=json")
	if err != nil {
		log.Fatal(err)
	}

	snapshot, err := v.Latest(ctx)
	_, _ = snapshot, err
}
