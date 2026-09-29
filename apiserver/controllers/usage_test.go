// Copyright 2026 Cloudbase Solutions SRL
//
//	Licensed under the Apache License, Version 2.0 (the "License"); you may
//	not use this file except in compliance with the License. You may obtain
//	a copy of the License at
//
//	     http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//	WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//	License for the specific language governing permissions and limitations
//	under the License.
package controllers

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commonParams "github.com/cloudbase/garm-provider-common/params"
)

func TestParseUsageFilter(t *testing.T) {
	query := url.Values{
		"start":             {"2026-09-01T00:00:00+09:00"},
		"end":               {"2026-10-01T00:00:00Z"},
		"poolID":            {"pool"},
		"scaleSetID":        {"7"},
		"entityID":          {"entity"},
		"instanceName":      {"runner"},
		"providerName":      {"gcp"},
		"provisioningModel": {"spot"},
		"runningOnly":       {"true"},
	}
	filter, err := parseUsageFilter(query)
	require.NoError(t, err)
	require.True(t, filter.Start.Equal(time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)))
	require.True(t, filter.End.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, "pool", filter.PoolID)
	require.Equal(t, uint(7), filter.ScaleSetID)
	require.Equal(t, "entity", filter.EntityID)
	require.Equal(t, "runner", filter.InstanceName)
	require.Equal(t, "gcp", filter.ProviderName)
	require.Equal(t, commonParams.ProvisioningModelSpot, filter.ProvisioningModel)
	require.True(t, filter.RunningOnly)

	filter, err = parseUsageFilter(url.Values{})
	require.NoError(t, err)
	require.True(t, filter.Start.IsZero())

	for name, value := range map[string]string{
		"start":             "2026-09-01",
		"end":               "yesterday",
		"scaleSetID":        "x",
		"provisioningModel": "preemptible",
		"runningOnly":       "maybe",
	} {
		_, err := parseUsageFilter(url.Values{name: {value}})
		require.Error(t, err, name)
	}
}
