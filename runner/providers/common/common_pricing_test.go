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

package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	runnerCommon "github.com/cloudbase/garm/runner/common"
)

func TestPriceResultFromExec(t *testing.T) {
	price, err := PriceResultFromExec([]byte(`{"currency":"USD","hourly_price":0.5,"provisioning_model":"spot"}`), nil)
	require.NoError(t, err)
	require.Equal(t, "USD", price.Currency)
	require.Equal(t, 0.5, price.HourlyPrice)

	for _, stderr := range []string{
		"failed to validate execution environment: unknown GARM_COMMAND: GetInstancePrice",
		"invalid command: GetInstancePrice",
		"provider does not support GetInstancePrice",
	} {
		_, err = PriceResultFromExec(nil, errors.New("provider binary failed with stdout: ; stderr: "+stderr))
		require.ErrorIs(t, err, runnerCommon.ErrPriceNotSupported, stderr)
	}

	_, err = PriceResultFromExec(nil, errors.New("provider binary failed: 403 Cloud Billing API disabled"))
	require.Error(t, err)
	require.NotErrorIs(t, err, runnerCommon.ErrPriceNotSupported)

	for _, out := range []string{`not json`, `{"hourly_price":1}`, `{"currency":"USD","hourly_price":-1}`} {
		_, err = PriceResultFromExec([]byte(out), nil)
		require.Error(t, err, out)
	}
}

func TestEgressResultFromExec(t *testing.T) {
	egress, err := EgressResultFromExec([]byte(`{"currency":"USD","cost":0.12,"flows":[{"location_type":"EXTERNAL","bytes":1073741824}]}`), nil)
	require.NoError(t, err)
	require.Equal(t, 0.12, egress.Cost)
	require.Len(t, egress.Flows, 1)

	_, err = EgressResultFromExec(nil, errors.New("provider binary failed with stdout: ; stderr: failed to run command: provider does not support GetInstanceEgress"))
	require.ErrorIs(t, err, runnerCommon.ErrEgressNotSupported)
	_, err = EgressResultFromExec(nil, errors.New("failed to validate execution environment: unknown GARM_COMMAND: GetInstanceEgress"))
	require.ErrorIs(t, err, runnerCommon.ErrEgressNotSupported)
	_, err = EgressResultFromExec(nil, errors.New("403 monitoring.timeSeries.list denied"))
	require.Error(t, err)
	require.NotErrorIs(t, err, runnerCommon.ErrEgressNotSupported)

	for _, out := range []string{`nope`, `{"cost":-1}`, `{"cost":0.5}`} {
		_, err = EgressResultFromExec([]byte(out), nil)
		require.Error(t, err, out)
	}
	// No billed traffic: no currency needed.
	_, err = EgressResultFromExec([]byte(`{"cost":0}`), nil)
	require.NoError(t, err)
}
