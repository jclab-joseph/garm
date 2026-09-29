// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package v011

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/config"
	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/runner/common"
)

// fakeProvider writes a provider binary that records its environment and
// stdin, and prints output (or fails with it on stderr).
func fakeProvider(t *testing.T, output string, fail bool) (common.Provider, string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nenv > \"$(dirname \"$0\")/env\"\ncat > \"$(dirname \"$0\")/stdin\"\n"
	if fail {
		script += "echo '" + output + "' >&2\nexit 1\n"
	} else {
		script += "echo '" + output + "'\n"
	}
	bin := filepath.Join(dir, "provider")
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	cfgFile := filepath.Join(dir, "provider.toml")
	require.NoError(t, os.WriteFile(cfgFile, nil, 0o600))

	provider, err := NewProvider(context.Background(), &config.Provider{
		Name:         "test",
		ProviderType: params.ExternalProvider,
		External: config.External{
			ProviderExecutable: bin,
			ConfigFile:         cfgFile,
		},
	}, "controller-id")
	require.NoError(t, err)
	return provider, dir
}

func TestGetInstancePrice(t *testing.T) {
	provider, dir := fakeProvider(t, `{"currency":"USD","hourly_price":0.25,"provisioning_model":"spot"}`, false)
	estimator, ok := provider.(common.PriceEstimator)
	require.True(t, ok)

	price, err := estimator.GetInstancePrice(context.Background(), commonParams.BootstrapInstance{
		Name:       "runner-1",
		PoolID:     "pool-1",
		Flavor:     "c4d-standard-8",
		ExtraSpecs: json.RawMessage(`{"spot":true}`),
	}, common.ProviderBaseParams{PoolInfo: params.Pool{ID: "pool-1"}})
	require.NoError(t, err)
	require.Equal(t, 0.25, price.HourlyPrice)
	require.Equal(t, commonParams.ProvisioningModelSpot, price.ProvisioningModel)

	env, err := os.ReadFile(filepath.Join(dir, "env"))
	require.NoError(t, err)
	require.Contains(t, strings.Split(string(env), "\n"), "GARM_COMMAND=GetInstancePrice")
	require.Contains(t, strings.Split(string(env), "\n"), "GARM_POOL_ID=pool-1")
	require.Contains(t, strings.Split(string(env), "\n"), "GARM_CONTROLLER_ID=controller-id")

	stdin, err := os.ReadFile(filepath.Join(dir, "stdin"))
	require.NoError(t, err)
	var bootstrap commonParams.BootstrapInstance
	require.NoError(t, json.Unmarshal(stdin, &bootstrap))
	require.Equal(t, "c4d-standard-8", bootstrap.Flavor)
	require.JSONEq(t, `{"spot":true}`, string(bootstrap.ExtraSpecs))
}

func TestGetInstancePriceNotSupported(t *testing.T) {
	provider, _ := fakeProvider(t, "failed to validate execution environment: unknown GARM_COMMAND: GetInstancePrice", true)
	_, err := provider.(common.PriceEstimator).GetInstancePrice(context.Background(),
		commonParams.BootstrapInstance{Name: "runner-1", Flavor: "f"}, common.ProviderBaseParams{})
	require.ErrorIs(t, err, common.ErrPriceNotSupported)
}
