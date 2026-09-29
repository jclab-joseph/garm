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
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	runnerErrors "github.com/cloudbase/garm-provider-common/errors"
	"github.com/cloudbase/garm/auth"
	"github.com/cloudbase/garm/params"
)

// maxUsageSummaryRange bounds the date range of a usage summary, which is
// computed in memory from all records in the range.
const maxUsageSummaryRange = 366 * 24 * time.Hour

const unknownGroup = "unknown"

// ListInstanceUsage lists instance runs overlapping the filter date range,
// newest first, with their estimated cost.
func (r *Runner) ListInstanceUsage(ctx context.Context, filter params.InstanceUsageFilter, page, pageSize uint64) (params.InstanceUsagePaginatedResponse, error) {
	if !auth.IsAdmin(ctx) {
		return params.InstanceUsagePaginatedResponse{}, runnerErrors.ErrUnauthorized
	}
	if !filter.Start.IsZero() && !filter.End.IsZero() && !filter.Start.Before(filter.End) {
		return params.InstanceUsagePaginatedResponse{}, runnerErrors.NewBadRequestError("start must be before end")
	}
	filter.Start, filter.End = filter.Start.UTC(), filter.End.UTC()

	ret, err := r.store.ListInstanceUsage(ctx, filter, page, pageSize)
	if err != nil {
		return params.InstanceUsagePaginatedResponse{}, fmt.Errorf("error listing instance usage: %w", err)
	}
	now := time.Now().UTC()
	for i, usage := range ret.Results {
		ret.Results[i] = usage.WithCost(filter.Start, filter.End, now)
	}
	return ret, nil
}

// GetUsageSummary returns the estimated cost of all instances in the filter
// date range. Days are calendar days in loc.
func (r *Runner) GetUsageSummary(ctx context.Context, filter params.InstanceUsageFilter, loc *time.Location) (params.UsageSummary, error) {
	if !auth.IsAdmin(ctx) {
		return params.UsageSummary{}, runnerErrors.ErrUnauthorized
	}
	if filter.Start.IsZero() || filter.End.IsZero() {
		return params.UsageSummary{}, runnerErrors.NewBadRequestError("start and end are required")
	}
	if !filter.Start.Before(filter.End) {
		return params.UsageSummary{}, runnerErrors.NewBadRequestError("start must be before end")
	}
	if filter.End.Sub(filter.Start) > maxUsageSummaryRange {
		return params.UsageSummary{}, runnerErrors.NewBadRequestError("date range must not exceed 366 days")
	}
	if loc == nil {
		loc = time.UTC
	}
	filter.Start, filter.End = filter.Start.UTC(), filter.End.UTC()

	records, err := r.store.ListAllInstanceUsage(ctx, filter)
	if err != nil {
		return params.UsageSummary{}, fmt.Errorf("error listing instance usage: %w", err)
	}
	return SummarizeUsage(records, filter.Start, filter.End, loc, time.Now().UTC()), nil
}

// usageGroups accumulates the usage of named groups.
type usageGroups struct {
	groups map[string]*params.UsageGroup
}

func newUsageGroups() *usageGroups {
	return &usageGroups{groups: map[string]*params.UsageGroup{}}
}

func (g *usageGroups) add(key, label string, cost, seconds float64, countInstance bool) {
	if key == "" {
		key = unknownGroup
	}
	group, ok := g.groups[key]
	if !ok {
		group = &params.UsageGroup{Key: key, Label: label}
		g.groups[key] = group
	}
	group.Cost += cost
	group.BilledSeconds += seconds
	if countInstance {
		group.InstanceCount++
	}
}

// byCost returns the groups, most expensive first.
func (g *usageGroups) byCost() []params.UsageGroup {
	ret := g.list()
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Cost != ret[j].Cost {
			return ret[i].Cost > ret[j].Cost
		}
		return ret[i].Key < ret[j].Key
	})
	return ret
}

// byKey returns the groups sorted by key.
func (g *usageGroups) byKey() []params.UsageGroup {
	ret := g.list()
	sort.Slice(ret, func(i, j int) bool { return ret[i].Key < ret[j].Key })
	return ret
}

func (g *usageGroups) list() []params.UsageGroup {
	ret := make([]params.UsageGroup, 0, len(g.groups))
	for _, group := range g.groups {
		ret = append(ret, *group)
	}
	return ret
}

func poolGroup(u params.InstanceUsage) (string, string) {
	label := u.Flavor
	if u.EntityName != "" {
		label = fmt.Sprintf("%s (%s)", u.Flavor, u.EntityName)
	}
	switch {
	case u.PoolID != "":
		return u.PoolID, label
	case u.ScaleSetID != 0:
		return fmt.Sprintf("scaleset-%d", u.ScaleSetID), label
	}
	return "", label
}

// SummarizeUsage computes the usage summary of records in [start, end). The
// cost of runs that cross the range boundaries is prorated by run time.
func SummarizeUsage(records []params.InstanceUsage, start, end time.Time, loc *time.Location, now time.Time) params.UsageSummary {
	summary := params.UsageSummary{
		Start:    start,
		End:      end,
		TimeZone: loc.String(),
	}
	byModel := newUsageGroups()
	byPool := newUsageGroups()
	byFlavor := newUsageGroups()
	byEntity := newUsageGroups()
	byDay := newUsageGroups()
	currencies := map[string]struct{}{}

	// Every day in the range is listed, including days without usage.
	for day := startOfDay(start.In(loc)); day.Before(end); day = day.AddDate(0, 0, 1) {
		byDay.add(day.Format(time.DateOnly), "", 0, 0, false)
	}

	for _, record := range records {
		u := record.WithCost(start, end, now)
		summary.InstanceCount++
		summary.TotalCost += u.RangeCost
		summary.BilledSeconds += u.RangeBilledSeconds
		if u.Running() {
			summary.RunningCount++
			summary.RunningCost += u.RangeCost
		}
		if u.HourlyPrice == nil && u.RangeBilledSeconds > 0 {
			summary.UnpricedCount++
		}
		if u.Currency != "" {
			currencies[u.Currency] = struct{}{}
		}

		byModel.add(string(u.ProvisioningModel), "", u.RangeCost, u.RangeBilledSeconds, true)
		poolKey, poolLabel := poolGroup(u)
		byPool.add(poolKey, poolLabel, u.RangeCost, u.RangeBilledSeconds, true)
		byFlavor.add(u.Flavor, "", u.RangeCost, u.RangeBilledSeconds, true)
		byEntity.add(u.EntityID, u.EntityName, u.RangeCost, u.RangeBilledSeconds, true)
		addDailyUsage(byDay, u, start, end, loc, now)
	}

	summary.ByProvisioningModel = byModel.byCost()
	summary.ByPool = byPool.byCost()
	summary.ByFlavor = byFlavor.byCost()
	summary.ByEntity = byEntity.byCost()
	summary.ByDay = byDay.byKey()

	names := make([]string, 0, len(currencies))
	for currency := range currencies {
		names = append(names, currency)
	}
	sort.Strings(names)
	switch len(names) {
	case 0:
	case 1:
		summary.Currency = names[0]
	default:
		summary.Currency = names[0]
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("prices use several currencies (%s); totals add them up as is", strings.Join(names, ", ")))
	}
	if summary.UnpricedCount > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d instance(s) have no price yet and are not included in the totals", summary.UnpricedCount))
	}
	return summary
}

// addDailyUsage splits the in-range run time of u over calendar days.
func addDailyUsage(byDay *usageGroups, u params.InstanceUsage, start, end time.Time, loc *time.Location, now time.Time) {
	runStart, runEnd, ok := u.BilledInterval(now)
	if !ok {
		return
	}
	if runStart.Before(start) {
		runStart = start
	}
	if runEnd.After(end) {
		runEnd = end
	}
	for dayStart := startOfDay(runStart.In(loc)); dayStart.Before(runEnd); {
		dayEnd := dayStart.AddDate(0, 0, 1)
		from, to := runStart, runEnd
		if dayStart.After(from) {
			from = dayStart
		}
		if dayEnd.Before(to) {
			to = dayEnd
		}
		if to.After(from) {
			seconds := to.Sub(from).Seconds()
			cost := 0.0
			if u.HourlyPrice != nil {
				cost = *u.HourlyPrice * seconds / 3600
			}
			byDay.add(dayStart.Format(time.DateOnly), "", cost, seconds, false)
		}
		dayStart = dayEnd
	}
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
