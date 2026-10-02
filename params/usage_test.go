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

package params

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func timePtr(t time.Time) *time.Time { return &t }

func floatPtr(f float64) *float64 { return &f }

func TestInstanceUsageWithCost(t *testing.T) {
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := day.Add(48 * time.Hour)

	tests := []struct {
		name                 string
		usage                InstanceUsage
		rangeStart, rangeEnd time.Time
		seconds, rangeSecs   float64
		cost, rangeCost      float64
	}{
		{
			name:    "never started",
			usage:   InstanceUsage{HourlyPrice: floatPtr(1)},
			seconds: 0, rangeSecs: 0, cost: 0, rangeCost: 0,
		},
		{
			name: "inside the range",
			usage: InstanceUsage{
				StartedAt: timePtr(day.Add(time.Hour)), EndedAt: timePtr(day.Add(3 * time.Hour)),
				HourlyPrice: floatPtr(0.5),
			},
			rangeStart: day, rangeEnd: day.Add(24 * time.Hour),
			seconds: 7200, rangeSecs: 7200, cost: 1, rangeCost: 1,
		},
		{
			name: "prorated at the range start",
			usage: InstanceUsage{
				StartedAt: timePtr(day.Add(-time.Hour)), EndedAt: timePtr(day.Add(3 * time.Hour)),
				HourlyPrice: floatPtr(1),
			},
			rangeStart: day, rangeEnd: day.Add(24 * time.Hour),
			seconds: 4 * 3600, rangeSecs: 3 * 3600, cost: 4, rangeCost: 3,
		},
		{
			name: "running instance billed until now, prorated at the range end",
			usage: InstanceUsage{
				StartedAt:   timePtr(day.Add(23 * time.Hour)),
				HourlyPrice: floatPtr(2),
			},
			rangeStart: day, rangeEnd: day.Add(24 * time.Hour),
			seconds: 25 * 3600, rangeSecs: 3600, cost: 50, rangeCost: 2,
		},
		{
			name: "one minute minimum",
			usage: InstanceUsage{
				StartedAt: timePtr(day), EndedAt: timePtr(day.Add(10 * time.Second)),
				HourlyPrice: floatPtr(3.6),
			},
			seconds: 60, rangeSecs: 60, cost: 0.06, rangeCost: 0.06,
		},
		{
			name: "unknown price",
			usage: InstanceUsage{
				StartedAt: timePtr(day), EndedAt: timePtr(day.Add(time.Hour)),
			},
			rangeStart: day, rangeEnd: day.Add(time.Hour),
			seconds: 3600, rangeSecs: 3600, cost: 0, rangeCost: 0,
		},
		{
			name: "outside the range",
			usage: InstanceUsage{
				StartedAt: timePtr(day.Add(-2 * time.Hour)), EndedAt: timePtr(day.Add(-time.Hour)),
				HourlyPrice: floatPtr(1),
			},
			rangeStart: day, rangeEnd: day.Add(time.Hour),
			seconds: 3600, rangeSecs: 0, cost: 1, rangeCost: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.usage.WithCost(tt.rangeStart, tt.rangeEnd, now)
			require.InDelta(t, tt.seconds, got.BilledSeconds, 1e-9)
			require.InDelta(t, tt.rangeSecs, got.RangeBilledSeconds, 1e-9)
			require.InDelta(t, tt.cost, got.Cost, 1e-9)
			require.InDelta(t, tt.rangeCost, got.RangeCost, 1e-9)
		})
	}
}

func TestInstanceUsageWithEgressCost(t *testing.T) {
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	usage := InstanceUsage{
		StartedAt:           timePtr(day.Add(-time.Hour)),
		EndedAt:             timePtr(day.Add(3 * time.Hour)),
		HourlyPrice:         floatPtr(1),
		EgressCost:          floatPtr(0.4),
		EgressExternalBytes: 4000,
	}
	// 3 of the 4 hours are in the range: egress is prorated by run time.
	got := usage.WithCost(day, day.Add(24*time.Hour), day.Add(48*time.Hour))
	require.InDelta(t, 3, got.RangeCost, 1e-9)
	require.InDelta(t, 0.3, got.RangeEgressCost, 1e-9)
	require.InDelta(t, 3000, got.RangeExternalBytes, 1e-9)

	// No egress measurement yet.
	usage.EgressCost = nil
	usage.EgressExternalBytes = 0
	got = usage.WithCost(day, day.Add(24*time.Hour), day.Add(48*time.Hour))
	require.Zero(t, got.RangeEgressCost)
	require.Zero(t, got.RangeExternalBytes)
}
