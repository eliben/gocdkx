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

// Package sdserver provides the diagnostic hooks for a server using
// Stackdriver.
package sdserver // import "github.com/eliben/gocdkx/server/sdserver"

import (
	"fmt"
	"os"

	"github.com/google/wire"
	"github.com/eliben/gocdkx/gcp"
	"github.com/eliben/gocdkx/internal/useragent"
	"github.com/eliben/gocdkx/requestlog"
	"github.com/eliben/gocdkx/server"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	"go.opencensus.io/trace"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server given a GCP token source and a GCP project ID.
var Set = wire.NewSet(
	server.Set,
	NewExporter,
	monitoredresource.Autodetect,
	wire.Bind((*trace.Exporter)(nil), (*stackdriver.Exporter)(nil)),
	NewRequestLogger,
	wire.Bind((*requestlog.Logger)(nil), (*requestlog.StackdriverLogger)(nil)),
)

// NewExporter returns a new OpenCensus Stackdriver exporter.
//
// The second return value is a Wire cleanup function that calls Flush
// on the exporter.
func NewExporter(id gcp.ProjectID, ts gcp.TokenSource, mr monitoredresource.Interface) (*stackdriver.Exporter, func(), error) {
	opts := []option.ClientOption{
		option.WithTokenSource(oauth2.TokenSource(ts)),
		useragent.ClientOption("server"),
	}
	exp, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID:               string(id),
		MonitoringClientOptions: opts,
		TraceClientOptions:      opts,
		MonitoredResource:       mr,
	})
	if err != nil {
		return nil, nil, err
	}

	return exp, func() { exp.Flush() }, err
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.StackdriverLogger {
	// For now, request logs are written to stdout and get picked up by fluentd.
	// This also works when running locally.
	return requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) })
}
