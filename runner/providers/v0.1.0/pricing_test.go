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

package v010

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestGetInstanceEgress(t *testing.T) {
	provider, dir := fakeProvider(t, `{"currency":"USD","cost":0.12,"sent_bytes":2000,"flows":[{"location_type":"EXTERNAL","continent":"America","bytes":1073741824}]}`, false)
	estimator, ok := provider.(common.EgressEstimator)
	require.True(t, ok)

	start := time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)
	egress, err := estimator.GetInstanceEgress(context.Background(), "us-east4-a/garm-runner", start, start.Add(time.Hour),
		common.ProviderBaseParams{PoolInfo: params.Pool{ID: "pool-1"}})
	require.NoError(t, err)
	require.Equal(t, 0.12, egress.Cost)
	require.Equal(t, int64(2000), egress.SentBytes)

	env, err := os.ReadFile(filepath.Join(dir, "env"))
	require.NoError(t, err)
	lines := strings.Split(string(env), "\n")
	require.Contains(t, lines, "GARM_COMMAND=GetInstanceEgress")
	require.Contains(t, lines, "GARM_INSTANCE_ID=us-east4-a/garm-runner")
	require.Contains(t, lines, "GARM_EGRESS_START=2026-10-02T01:00:00Z")
	require.Contains(t, lines, "GARM_EGRESS_END=2026-10-02T02:00:00Z")
}

func TestGetInstanceEgressNotSupported(t *testing.T) {
	provider, _ := fakeProvider(t, "failed to validate execution environment: unknown GARM_COMMAND: GetInstanceEgress", true)
	_, err := provider.(common.EgressEstimator).GetInstanceEgress(context.Background(), "x", time.Now().Add(-time.Hour), time.Now(), common.ProviderBaseParams{})
	require.ErrorIs(t, err, common.ErrEgressNotSupported)
}
