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
	"encoding/json"
	"time"

	commonParams "github.com/cloudbase/garm-provider-common/params"
)

// UsagePriceStatus is the state of the price lookup of an instance usage record.
type UsagePriceStatus string

const (
	// UsagePricePending means the price has not been looked up yet.
	UsagePricePending UsagePriceStatus = "pending"
	// UsagePriceOK means HourlyPrice is set.
	UsagePriceOK UsagePriceStatus = "ok"
	// UsagePriceError means the last lookup failed; it is retried.
	UsagePriceError UsagePriceStatus = "error"
	// UsagePriceUnsupported means the provider can't price instances.
	UsagePriceUnsupported UsagePriceStatus = "unsupported"
)

// UsageEndReason tells why an instance usage record was closed.
type UsageEndReason string

const (
	// UsageEndDeleted means the instance was removed from the provider.
	UsageEndDeleted UsageEndReason = "deleted"
	// UsageEndOrphaned means the instance disappeared from the database
	// without its usage record being closed (should not happen).
	UsageEndOrphaned UsageEndReason = "orphaned"
)

// MinimumBilledDuration is the shortest duration a started instance is
// billed for (Compute Engine bills per second with a one minute minimum).
const MinimumBilledDuration = time.Minute

// InstanceUsage records one run of an instance, from creation to deletion,
// with its estimated price. Unlike instances, usage records are kept after
// the instance is deleted.
//
// swagger:model InstanceUsage
type InstanceUsage struct {
	ID           string `json:"id"`
	InstanceID   string `json:"instance_id"`
	InstanceName string `json:"instance_name"`
	ProviderID   string `json:"provider_id,omitempty"`
	ProviderName string `json:"provider_name"`
	PoolID       string `json:"pool_id,omitempty"`
	ScaleSetID   uint   `json:"scale_set_id,omitempty"`

	EntityType ForgeEntityType `json:"entity_type,omitempty"`
	EntityID   string          `json:"entity_id,omitempty"`
	EntityName string          `json:"entity_name,omitempty"`

	Flavor string              `json:"flavor"`
	Image  string              `json:"image"`
	OSType commonParams.OSType `json:"os_type,omitempty"`
	OSArch commonParams.OSArch `json:"os_arch,omitempty"`
	// ExtraSpecs are the pool (or scale set) extra specs when the instance
	// was created; the price is looked up with them.
	ExtraSpecs json.RawMessage `json:"extra_specs,omitempty"`

	// CreatedAt is when GARM created the instance record.
	CreatedAt time.Time `json:"created_at"`
	// StartedAt is when the provider reported the instance as running.
	// Billing starts here. Instances that never ran have no StartedAt.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// EndedAt is when the instance was removed from the provider. Unset
	// while the instance exists.
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
	EndReason UsageEndReason `json:"end_reason,omitempty"`

	ProvisioningModel commonParams.ProvisioningModel `json:"provisioning_model,omitempty"`
	// HourlyPrice is the estimated price per hour, looked up when the
	// instance was created. Unset until PriceStatus is ok.
	HourlyPrice *float64                    `json:"hourly_price,omitempty"`
	Currency    string                      `json:"currency,omitempty"`
	Price       *commonParams.InstancePrice `json:"price,omitempty"`
	PriceStatus UsagePriceStatus            `json:"price_status"`
	PriceError  string                      `json:"price_error,omitempty"`
	PricedAt    *time.Time                  `json:"priced_at,omitempty"`

	// The last job that ran on the instance, if any.
	JobID          int64  `json:"job_id,omitempty"`
	JobName        string `json:"job_name,omitempty"`
	JobRepository  string `json:"job_repository,omitempty"`
	WorkflowRunURL string `json:"workflow_run_url,omitempty"`

	// The fields below are computed when the record is returned by the API.

	// BilledSeconds is the billed run time so far.
	BilledSeconds float64 `json:"billed_seconds"`
	// Cost is the estimated cost so far. Zero while the price is unknown.
	Cost float64 `json:"cost"`
	// RangeBilledSeconds and RangeCost are the parts of BilledSeconds and
	// Cost that fall in the requested date range.
	RangeBilledSeconds float64 `json:"range_billed_seconds"`
	RangeCost          float64 `json:"range_cost"`
}

// Running reports whether the instance still exists.
func (u InstanceUsage) Running() bool {
	return u.EndedAt == nil
}

// BilledInterval returns the billed interval of the instance as of now. ok is
// false for instances that never ran. Started instances are billed for at
// least MinimumBilledDuration.
func (u InstanceUsage) BilledInterval(now time.Time) (start, end time.Time, ok bool) {
	if u.StartedAt == nil {
		return time.Time{}, time.Time{}, false
	}
	start = *u.StartedAt
	end = now
	if u.EndedAt != nil {
		end = *u.EndedAt
	}
	if end.Sub(start) < MinimumBilledDuration {
		end = start.Add(MinimumBilledDuration)
	}
	return start, end, true
}

// WithCost returns u with the computed cost fields set, for the whole run and
// for the part of it that overlaps [rangeStart, rangeEnd). A zero rangeStart
// or rangeEnd leaves that side of the range open.
func (u InstanceUsage) WithCost(rangeStart, rangeEnd, now time.Time) InstanceUsage {
	u.BilledSeconds, u.Cost, u.RangeBilledSeconds, u.RangeCost = 0, 0, 0, 0
	start, end, ok := u.BilledInterval(now)
	if !ok {
		return u
	}
	u.BilledSeconds = end.Sub(start).Seconds()

	overlapStart, overlapEnd := start, end
	if !rangeStart.IsZero() && rangeStart.After(overlapStart) {
		overlapStart = rangeStart
	}
	if !rangeEnd.IsZero() && rangeEnd.Before(overlapEnd) {
		overlapEnd = rangeEnd
	}
	if overlapEnd.After(overlapStart) {
		u.RangeBilledSeconds = overlapEnd.Sub(overlapStart).Seconds()
	}

	if u.HourlyPrice != nil {
		u.Cost = *u.HourlyPrice * u.BilledSeconds / 3600
		u.RangeCost = *u.HourlyPrice * u.RangeBilledSeconds / 3600
	}
	return u
}

// InstanceUsageFilter selects instance usage records.
type InstanceUsageFilter struct {
	// Start and End select records whose run overlaps [Start, End). Zero
	// values leave that side open.
	Start time.Time
	End   time.Time

	PoolID            string
	ScaleSetID        uint
	EntityID          string
	InstanceName      string
	ProviderName      string
	ProvisioningModel commonParams.ProvisioningModel
	// RunningOnly selects instances that still exist.
	RunningOnly bool
}

// SetInstanceUsagePriceParams is the outcome of a price lookup.
type SetInstanceUsagePriceParams struct {
	Status UsagePriceStatus
	Price  *commonParams.InstancePrice
	Error  string
}

// swagger:model InstanceUsagePaginatedResponse
type InstanceUsagePaginatedResponse = PaginatedResponse[InstanceUsage]

// UsageGroup is the usage of a group of instances in a UsageSummary.
//
// swagger:model UsageGroup
type UsageGroup struct {
	// Key identifies the group (e.g. a pool ID, a flavor, a day).
	Key string `json:"key"`
	// Label is a human readable name of the group, when Key is an ID.
	Label         string  `json:"label,omitempty"`
	Cost          float64 `json:"cost"`
	BilledSeconds float64 `json:"billed_seconds"`
	InstanceCount int     `json:"instance_count"`
}

// UsageSummary is the estimated cost of all instances in a date range. Costs
// of instances that run across the range boundaries are prorated.
//
// swagger:model UsageSummary
type UsageSummary struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	TimeZone string    `json:"time_zone"`
	Currency string    `json:"currency,omitempty"`

	TotalCost     float64 `json:"total_cost"`
	BilledSeconds float64 `json:"billed_seconds"`
	InstanceCount int     `json:"instance_count"`
	// RunningCost and RunningCount cover instances that still exist.
	RunningCost  float64 `json:"running_cost"`
	RunningCount int     `json:"running_count"`
	// UnpricedCount counts instances whose price is not known (yet); their
	// cost is not included in the totals.
	UnpricedCount int `json:"unpriced_count"`

	ByProvisioningModel []UsageGroup `json:"by_provisioning_model"`
	ByPool              []UsageGroup `json:"by_pool"`
	ByFlavor            []UsageGroup `json:"by_flavor"`
	ByEntity            []UsageGroup `json:"by_entity"`
	// ByDay splits the costs by calendar day in TimeZone.
	ByDay []UsageGroup `json:"by_day"`

	Warnings []string `json:"warnings,omitempty"`
}
