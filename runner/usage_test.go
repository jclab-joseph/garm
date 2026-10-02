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

package runner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/params"
)

func usageRecord(name, poolID, flavor string, model commonParams.ProvisioningModel, price *float64, started, ended *time.Time) params.InstanceUsage {
	u := params.InstanceUsage{
		InstanceName:      name,
		PoolID:            poolID,
		Flavor:            flavor,
		EntityID:          "org-id",
		EntityName:        "jclab",
		ProvisioningModel: model,
		HourlyPrice:       price,
		StartedAt:         started,
		EndedAt:           ended,
	}
	if price != nil {
		u.Currency = "USD"
	}
	return u
}

func TestSummarizeUsage(t *testing.T) {
	seoul, err := time.LoadLocation("Asia/Seoul")
	require.NoError(t, err)
	// Two calendar days in Seoul (UTC+9).
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, seoul).UTC()
	end := start.Add(48 * time.Hour)
	now := end.Add(time.Hour)
	at := func(h float64) *time.Time {
		t := start.Add(time.Duration(h * float64(time.Hour)))
		return &t
	}
	price := func(p float64) *float64 { return &p }

	records := []params.InstanceUsage{
		// 2h before the range, 2h inside: half of the cost counts.
		usageRecord("across-start", "pool-a", "c4d-standard-8", commonParams.ProvisioningModelSpot, price(1), at(-2), at(2)),
		// Across midnight (Seoul) of day 1 -> day 2.
		usageRecord("midnight", "pool-a", "c4d-standard-8", commonParams.ProvisioningModelSpot, price(1), at(23), at(25)),
		// Still running at the end of the range.
		usageRecord("running", "pool-b", "c4a-standard-8", commonParams.ProvisioningModelStandard, price(2), at(47), nil),
		// Not priced yet.
		usageRecord("unpriced", "pool-b", "c4a-standard-8", "", nil, at(10), at(11)),
		// Never started (e.g. failed to create).
		usageRecord("failed", "pool-b", "c4a-standard-8", "", nil, nil, at(1)),
	}

	summary := SummarizeUsage(records, start, end, seoul, now)
	require.Equal(t, "Asia/Seoul", summary.TimeZone)
	require.Equal(t, "USD", summary.Currency)
	require.Equal(t, 5, summary.InstanceCount)
	require.Equal(t, 1, summary.RunningCount)
	require.Equal(t, 1, summary.UnpricedCount)
	require.Len(t, summary.Warnings, 1)
	// 2 (across-start) + 2 (midnight) + 1h * 2 (running, prorated at end)
	require.InDelta(t, 6, summary.TotalCost, 1e-9)
	require.InDelta(t, 2, summary.RunningCost, 1e-9)
	require.InDelta(t, (2+2+1+1)*3600, summary.BilledSeconds, 1e-6)

	require.Equal(t, []params.UsageGroup{
		{Key: "spot", Cost: 4, BilledSeconds: 4 * 3600, InstanceCount: 2},
		{Key: "standard", Cost: 2, BilledSeconds: 3600, InstanceCount: 1},
		{Key: "unknown", Cost: 0, BilledSeconds: 3600, InstanceCount: 2},
	}, roundGroups(summary.ByProvisioningModel))

	require.Len(t, summary.ByPool, 2)
	require.Equal(t, "pool-a", summary.ByPool[0].Key)
	require.Equal(t, "c4d-standard-8 (jclab)", summary.ByPool[0].Label)
	require.InDelta(t, 4, summary.ByPool[0].Cost, 1e-9)
	require.Equal(t, "pool-b", summary.ByPool[1].Key)
	require.Equal(t, 3, summary.ByPool[1].InstanceCount)

	require.Len(t, summary.ByEntity, 1)
	require.Equal(t, "jclab", summary.ByEntity[0].Label)
	require.InDelta(t, 6, summary.ByEntity[0].Cost, 1e-9)

	// Days are Seoul calendar days.
	require.Len(t, summary.ByDay, 2)
	require.Equal(t, "2026-09-01", summary.ByDay[0].Key)
	require.Equal(t, "2026-09-02", summary.ByDay[1].Key)
	// Day 1: across-start 2h + midnight 1h (all $1/h), unpriced 1h ($0).
	require.InDelta(t, 3, summary.ByDay[0].Cost, 1e-9)
	require.InDelta(t, 4*3600, summary.ByDay[0].BilledSeconds, 1e-6)
	// Day 2: midnight 1h ($1) + running 1h ($2).
	require.InDelta(t, 3, summary.ByDay[1].Cost, 1e-9)

	dayTotal := 0.0
	for _, d := range summary.ByDay {
		dayTotal += d.Cost
	}
	require.InDelta(t, summary.TotalCost, dayTotal, 1e-9)
}

func TestSummarizeUsageEgress(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	price, egress := 1.0, 0.8
	// 4h run, 2h before the range start: half of compute and egress count.
	started, ended := start.Add(-2*time.Hour), start.Add(2*time.Hour)
	u := usageRecord("a", "pool-a", "c4d-standard-8", commonParams.ProvisioningModelSpot, &price, &started, &ended)
	u.EgressCost = &egress
	u.EgressCurrency = "USD"
	u.EgressExternalBytes = 1000

	summary := SummarizeUsage([]params.InstanceUsage{u}, start, end, time.UTC, end)
	require.Empty(t, summary.Warnings)
	require.InDelta(t, 2, summary.ComputeCost, 1e-9)
	require.InDelta(t, 0.4, summary.EgressCost, 1e-9)
	require.InDelta(t, 2.4, summary.TotalCost, 1e-9)
	require.InDelta(t, 500, summary.ExternalBytes, 1e-9)
	require.InDelta(t, 2.4, summary.ByPool[0].Cost, 1e-9)
	require.InDelta(t, 0.4, summary.ByPool[0].EgressCost, 1e-9)
	require.InDelta(t, 2.4, summary.ByDay[0].Cost, 1e-9)
	require.InDelta(t, 0.4, summary.ByDay[0].EgressCost, 1e-9)
	require.Zero(t, summary.ByDay[1].Cost)
}

func TestSummarizeUsageEmpty(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	summary := SummarizeUsage(nil, start, start.Add(72*time.Hour), time.UTC, start)
	require.Zero(t, summary.TotalCost)
	require.Empty(t, summary.Currency)
	require.Empty(t, summary.Warnings)
	require.Len(t, summary.ByDay, 3, "days without usage are listed")
}

func TestSummarizeUsageMixedCurrencies(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	started := start.Add(time.Hour)
	price := 1.0
	a := usageRecord("a", "p", "f", commonParams.ProvisioningModelStandard, &price, &started, nil)
	b := a
	b.Currency = "EUR"
	summary := SummarizeUsage([]params.InstanceUsage{a, b}, start, start.Add(24*time.Hour), time.UTC, start.Add(2*time.Hour))
	require.Len(t, summary.Warnings, 1)
	require.Contains(t, summary.Warnings[0], "EUR, USD")
}

func roundGroups(groups []params.UsageGroup) []params.UsageGroup {
	for i := range groups {
		groups[i].Cost = float64(int64(groups[i].Cost*1e6+0.5)) / 1e6
		groups[i].BilledSeconds = float64(int64(groups[i].BilledSeconds + 0.5))
	}
	return groups
}
