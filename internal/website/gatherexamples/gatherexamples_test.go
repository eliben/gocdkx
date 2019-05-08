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

package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/packages/packagestest"
)

func TestGather(t *testing.T) {
	tests := []struct {
		name   string
		module packagestest.Module
		want   map[string]example
	}{
		{
			name: "NoExamples",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\nfunc main() {}\n",
				},
			},
			want: map[string]example{},
		},
		{
			name: "EmptyExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func Example() {}`,
				},
			},
			want: map[string]example{},
		},
		{
			name: "EmptyExampleFoo",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func ExampleFoo() {
}`,
				},
			},
			want: map[string]example{},
		},
		{
			name: "EmptyExampleWithComment",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func Example() {
	// This example is used in http://www.example.com/docs
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.Example": {Code: ""},
			},
		},
		{
			name: "EmptyExampleFooWithComment",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func ExampleFoo() {
	// This example is used in http://www.example.com/docs
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.ExampleFoo": {Code: ""},
			},
		},
		{
			name: "NoImportsExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func Example() {
	// This example is used in http://www.example.com/docs

	// Unattached comment.

	// Outside inner block comment.
	panic("ohai")
	if false {
		// something
	}
	return
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.Example": {Code: "// Unattached comment.\n\n" +
					"// Outside inner block comment.\n" +
					"panic(\"ohai\")\n" +
					"if false {\n\t// something\n}\n" +
					"return"},
			},
		},
		{
			name: "OneImportExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "fmt"

func Example() {
	// This example is used in http://www.example.com/docs
	fmt.Println(42)
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.Example": {
					Imports: "import \"fmt\"",
					Code:    "fmt.Println(42)",
				},
			},
		},
		{
			name: "TwoImportsExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "fmt"
import "math"

func Example() {
	// This example is used in http://www.example.com/docs
	fmt.Println(math.Pi)
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.Example": {
					Imports: "import (\n\t\"fmt\"\n\t\"math\"\n)",
					Code:    "fmt.Println(math.Pi)",
				},
			},
		},
		{
			name: "LogFatalToReturnErr",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "log"

func Example() {
	// This example is used in http://www.example.com/docs
	var err error
	if err != nil {
		log.Fatal(err)
	}
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.Example": {Code: "var err error\n" +
					"if err != nil {\n\treturn err\n}"},
			},
		},
		{
			name: "IgnoreSections",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "context"

func Example() {
	// This example is used in http://www.example.com/docs

	// Variables set up elsewhere:
	ctx := context.Background()

	// do something

	// Ignore unused variables in example:
	_ = ctx
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.Example": {
					Imports: "import \"context\"",
					Code:    "// do something",
				},
			},
		},
		{
			name: "BlankImports",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func Example() {
	// This example is used in http://www.example.com/docs

	// import _ "example.com/bar"
	_ = 42
}`,
				},
			},
			want: map[string]example{
				"example.com/foo.Example": {
					Imports: "import _ \"example.com/bar\"",
					Code:    "_ = 42",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exported := packagestest.Export(t, packagestest.Modules, []packagestest.Module{test.module})
			defer exported.Cleanup()
			exported.Config.Mode = gatherLoadMode
			pkgs, err := packages.Load(exported.Config, "./...")
			if err != nil {
				t.Fatal(err)
			}

			got := gather(pkgs)
			if diff := cmp.Diff(test.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("gather(pkgs) diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatImports(t *testing.T) {
	tests := []struct {
		name         string
		usedPackages map[string]string
		want         string
	}{
		{
			name:         "Empty",
			usedPackages: nil,
			want:         "",
		},
		{
			name:         "One",
			usedPackages: map[string]string{"fmt": ""},
			want:         "import \"fmt\"",
		},
		{
			name: "Two",
			usedPackages: map[string]string{
				"fmt": "",
				"log": "",
			},
			want: "import (\n\t\"fmt\"\n\t\"log\"\n)",
		},
		{
			name: "Renamed",
			usedPackages: map[string]string{
				"fmt": "zzz",
				"log": "aaa",
			},
			want: "import (\n\tzzz \"fmt\"\n\taaa \"log\"\n)",
		},
		{
			name: "StdlibSeparateFromThirdParty",
			usedPackages: map[string]string{
				"context":                      "",
				"fmt":                          "",
				"log":                          "",
				"github.com/google/go-cmp/cmp": "",
				"github.com/eliben/gocdkx/blob":             "",
			},
			want: "import (\n" +
				"\t\"context\"\n" +
				"\t\"fmt\"\n" +
				"\t\"log\"\n" +
				"\n" +
				"\t\"github.com/google/go-cmp/cmp\"\n" +
				"\t\"github.com/eliben/gocdkx/blob\"\n" +
				")",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := formatImports(test.usedPackages)
			if got != test.want {
				t.Errorf("formatImports(%+v) =\n%s\n// want:\n%s", test.usedPackages, got, test.want)
			}
		})
	}
}
