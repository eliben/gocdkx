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
// limtations under the License.

package vault

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/eliben/gocdkx/secrets"
	"github.com/eliben/gocdkx/secrets/driver"
	"github.com/eliben/gocdkx/secrets/drivertest"
)

// To run these tests against a real Vault server, first run ./localvault.sh.
// Then wait a few seconds for the server to be ready.

const (
	keyID1     = "test-secrets"
	keyID2     = "test-secrets2"
	apiAddress = "http://127.0.0.1:8200"
	testToken  = "faketoken"
)

type harness struct {
	client *api.Client
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, driver.Keeper, error) {
	return &keeper{keyID: keyID1, client: h.client}, &keeper{keyID: keyID2, client: h.client}, nil
}

func (h *harness) Close() {}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	c, err := Dial(ctx, &Config{
		Token: testToken,
		APIConfig: api.Config{
			Address: apiAddress,
		},
	})
	if err != nil {
		return nil, err
	}
	c.SetClientTimeout(3 * time.Second)
	// Enable the Transit Secrets Engine to use Vault as an Encryption as a Service.
	if _, err := c.Logical().Write("sys/mounts/transit", map[string]interface{}{"type": "transit"}); err != nil {
		t.Skip(err, "run secrets/vault/localvault.sh to start a dev vault container")
	}

	return &harness{
		client: c,
	}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (v verifyAs) Name() string {
	return "verify As function"
}

func (v verifyAs) ErrorCheck(k *secrets.Keeper, err error) error {
	var s string
	if k.ErrorAs(err, &s) {
		return errors.New("Keeper.ErrorAs expected to fail")
	}
	return nil
}

// Vault-specific tests.

func TestNoSessionProvidedError(t *testing.T) {
	if _, err := Dial(context.Background(), nil); err == nil {
		t.Error("got nil, want no auth Config provided")
	}
}

func TestNoConnectionError(t *testing.T) {
	ctx := context.Background()

	// Dial calls vault's NewClient method, which doesn't make the connection. Try
	// doing encryption which should fail by no connection.
	client, err := Dial(ctx, &Config{
		Token: "<Client (Root) Token>",
		APIConfig: api.Config{
			Address: apiAddress,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	keeper := OpenKeeper(client, "my-key", nil)
	defer keeper.Close()
	if _, err := keeper.Encrypt(ctx, []byte("test")); err == nil {
		t.Error("got nil, want connection refused")
	}
}

func fakeConnectionStringInEnv() func() {
	oldURLVal := os.Getenv("VAULT_SERVER_URL")
	oldTokenVal := os.Getenv("VAULT_SERVER_TOKEN")
	os.Setenv("VAULT_SERVER_URL", "http://myvaultserver")
	os.Setenv("VAULT_SERVER_TOKEN", "faketoken")
	return func() {
		os.Setenv("VAULT_SERVER_URL", oldURLVal)
		os.Setenv("VAULT_SERVER_TOKEN", oldTokenVal)
	}
}

func TestOpenKeeper(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"vault://mykey", false},
		// Invalid parameter.
		{"vault://mykey?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		keeper, err := secrets.OpenKeeper(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			if err = keeper.Close(); err != nil {
				t.Errorf("%s: got error during close: %v", test.URL, err)
			}
		}
	}
}
