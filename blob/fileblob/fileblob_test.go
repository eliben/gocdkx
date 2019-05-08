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

package fileblob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eliben/gocdkx/blob"
	"github.com/eliben/gocdkx/blob/driver"
	"github.com/eliben/gocdkx/blob/drivertest"
)

type harness struct {
	dir       string
	server    *httptest.Server
	urlSigner URLSigner
	closer    func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	dir := filepath.Join(os.TempDir(), "go-cloud-fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}
	h := &harness{dir: dir}

	localServer := httptest.NewServer(http.HandlerFunc(h.serveSignedURL))
	h.server = localServer

	u, err := url.Parse(h.server.URL)
	if err != nil {
		return nil, err
	}
	h.urlSigner = NewURLSignerHMAC(u, []byte("I'm a secret key"))

	h.closer = func() { _ = os.RemoveAll(dir); localServer.Close() }

	return h, nil
}

func (h *harness) serveSignedURL(w http.ResponseWriter, r *http.Request) {
	objKey, err := h.urlSigner.KeyFromURL(r.Context(), r.URL)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	bucket, err := OpenBucket(h.dir, &Options{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	reader, err := bucket.NewReader(r.Context(), objKey, nil)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer reader.Close()
	io.Copy(w, reader)
}

func (h *harness) HTTPClient() *http.Client {
	return &http.Client{}
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	opts := &Options{
		URLSigner: h.urlSigner,
	}
	return openBucket(h.dir, opts)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyPathError{}})
}

func BenchmarkFileblob(b *testing.B) {
	dir := filepath.Join(os.TempDir(), "go-cloud-fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		b.Fatal(err)
	}
	bkt, err := OpenBucket(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	drivertest.RunBenchmarks(b, bkt)
}

// File-specific unit tests.
func TestNewBucket(t *testing.T) {
	t.Run("BucketDirMissing", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		_, gotErr := OpenBucket(filepath.Join(dir, "notfound"), nil)
		if gotErr == nil {
			t.Errorf("got nil want error")
		}
	})
	t.Run("BucketIsFile", func(t *testing.T) {
		f, err := ioutil.TempFile("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(f.Name())
		_, gotErr := OpenBucket(f.Name(), nil)
		if gotErr == nil {
			t.Errorf("got nil want error")
		}
	})
}

type verifyPathError struct{}

func (verifyPathError) Name() string { return "verify ErrorAs handles os.PathError" }

func (verifyPathError) BucketCheck(b *blob.Bucket) error             { return nil }
func (verifyPathError) BeforeRead(as func(interface{}) bool) error   { return nil }
func (verifyPathError) BeforeWrite(as func(interface{}) bool) error  { return nil }
func (verifyPathError) BeforeCopy(as func(interface{}) bool) error   { return nil }
func (verifyPathError) BeforeList(as func(interface{}) bool) error   { return nil }
func (verifyPathError) AttributesCheck(attrs *blob.Attributes) error { return nil }
func (verifyPathError) ReaderCheck(r *blob.Reader) error             { return nil }
func (verifyPathError) ListObjectCheck(o *blob.ListObject) error     { return nil }

func (verifyPathError) ErrorCheck(b *blob.Bucket, err error) error {
	var perr *os.PathError
	if !b.ErrorAs(err, &perr) {
		return errors.New("want ErrorAs to succeed for PathError")
	}
	wantSuffix := filepath.Join("go-cloud-fileblob", "key-does-not-exist")
	if got := perr.Path; !strings.HasSuffix(got, wantSuffix) {
		return fmt.Errorf("got path %q, want suffix %q", got, wantSuffix)
	}
	return nil
}

func TestOpenBucketFromURL(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "myfile.txt"), []byte("hello world"), 0666); err != nil {
		t.Fatal(err)
	}
	// Convert dir to a URL path, adding a leading "/" if needed on Windows.
	dirpath := filepath.ToSlash(dir)
	if os.PathSeparator != '/' && !strings.HasPrefix(dirpath, "/") {
		dirpath = "/" + dirpath
	}

	tests := []struct {
		URL         string
		Key         string
		WantErr     bool
		WantReadErr bool
		Want        string
	}{
		// Bucket doesn't exist -> error at construction time.
		{"file:///bucket-not-found", "myfile.txt", true, false, ""},
		// File doesn't exist -> error at read time.
		{"file://" + dirpath, "filenotfound.txt", false, true, ""},
		// OK.
		{"file://" + dirpath, "myfile.txt", false, false, "hello world"},
		// OK, host is ignored.
		{"file://localhost" + dirpath, "myfile.txt", false, false, "hello world"},
		// Invalid query parameter.
		{"file://" + dirpath + "?param=value", "myfile.txt", true, false, ""},
	}

	ctx := context.Background()
	for _, test := range tests {
		b, err := blob.OpenBucket(ctx, test.URL)
		t.Logf("%s", test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err != nil {
			continue
		}
		got, err := b.ReadAll(ctx, test.Key)
		if (err != nil) != test.WantReadErr {
			t.Errorf("%s: got read error %v, want error %v", test.URL, err, test.WantReadErr)
		}
		if err != nil {
			continue
		}
		if string(got) != test.Want {
			t.Errorf("%s: got %q want %q", test.URL, got, test.Want)
		}
	}
}
