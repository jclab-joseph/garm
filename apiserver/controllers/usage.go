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
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
	// The container image has no zoneinfo; timeZone needs the embedded copy.
	_ "time/tzdata"

	gErrors "github.com/cloudbase/garm-provider-common/errors"
	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/params"
)

// parseUsageFilter reads the instance usage filter from the query string.
func parseUsageFilter(query url.Values) (params.InstanceUsageFilter, error) {
	var filter params.InstanceUsageFilter
	for name, dst := range map[string]*time.Time{"start": &filter.Start, "end": &filter.End} {
		value := query.Get(name)
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return params.InstanceUsageFilter{}, gErrors.NewBadRequestError("invalid %s %q: expected an RFC 3339 time", name, value)
		}
		*dst = parsed
	}
	filter.PoolID = query.Get("poolID")
	if value := query.Get("scaleSetID"); value != "" {
		id, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return params.InstanceUsageFilter{}, gErrors.NewBadRequestError("invalid scaleSetID %q", value)
		}
		filter.ScaleSetID = uint(id)
	}
	filter.EntityID = query.Get("entityID")
	filter.InstanceName = query.Get("instanceName")
	filter.ProviderName = query.Get("providerName")
	if value := query.Get("provisioningModel"); value != "" {
		model := commonParams.ProvisioningModel(value)
		if model != commonParams.ProvisioningModelStandard && model != commonParams.ProvisioningModelSpot {
			return params.InstanceUsageFilter{}, gErrors.NewBadRequestError("invalid provisioningModel %q", value)
		}
		filter.ProvisioningModel = model
	}
	if value := query.Get("runningOnly"); value != "" {
		runningOnly, err := strconv.ParseBool(value)
		if err != nil {
			return params.InstanceUsageFilter{}, gErrors.NewBadRequestError("invalid runningOnly %q", value)
		}
		filter.RunningOnly = runningOnly
	}
	return filter, nil
}

// swagger:route GET /usage/instances usage ListInstanceUsage
//
// List instance runs and their estimated cost, newest first. Runs that
// overlap the date range are listed; range_cost is the part of the cost
// that falls in the range.
//
//	Parameters:
//	  + name: start
//	    description: Start of the date range (RFC 3339, inclusive).
//	    type: string
//	    in: query
//	    required: false
//	  + name: end
//	    description: End of the date range (RFC 3339, exclusive).
//	    type: string
//	    in: query
//	    required: false
//	  + name: poolID
//	    description: Only list instances of this pool.
//	    type: string
//	    in: query
//	    required: false
//	  + name: scaleSetID
//	    description: Only list instances of this scale set.
//	    type: integer
//	    in: query
//	    required: false
//	  + name: entityID
//	    description: Only list instances of this repository, organization or enterprise.
//	    type: string
//	    in: query
//	    required: false
//	  + name: instanceName
//	    description: Only list runs of the instance with this name.
//	    type: string
//	    in: query
//	    required: false
//	  + name: providerName
//	    description: Only list instances of this provider.
//	    type: string
//	    in: query
//	    required: false
//	  + name: provisioningModel
//	    description: Only list instances with this provisioning model (standard or spot).
//	    type: string
//	    in: query
//	    required: false
//	  + name: runningOnly
//	    description: Only list instances that still exist.
//	    type: boolean
//	    in: query
//	    required: false
//	  + name: page
//	    description: The page at which to list.
//	    type: integer
//	    in: query
//	    required: false
//	  + name: pageSize
//	    description: Number of items per page.
//	    type: integer
//	    in: query
//	    required: false
//
//	Responses:
//	  200: InstanceUsagePaginatedResponse
//	  400: APIErrorResponse
func (a *APIController) ListInstanceUsageHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	filter, err := parseUsageFilter(query)
	if err != nil {
		handleError(ctx, w, err)
		return
	}

	var page, pageSize uint64 = 1, 50
	if value := query.Get("page"); value != "" {
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if value := query.Get("pageSize"); value != "" {
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil && parsed > 0 {
			pageSize = min(parsed, 500)
		}
	}

	ret, err := a.r.ListInstanceUsage(ctx, filter, page, pageSize)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ret); err != nil {
		slog.With(slog.Any("error", err)).ErrorContext(ctx, "failed to encode response")
	}
}

// swagger:route GET /usage/summary usage GetUsageSummary
//
// Get the estimated cost of all instances in a date range. The cost of
// instances that run across the range boundaries is prorated.
//
//	Parameters:
//	  + name: start
//	    description: Start of the date range (RFC 3339, inclusive).
//	    type: string
//	    in: query
//	    required: true
//	  + name: end
//	    description: End of the date range (RFC 3339, exclusive). At most 366 days after start.
//	    type: string
//	    in: query
//	    required: true
//	  + name: timeZone
//	    description: IANA time zone of the daily breakdown (default UTC).
//	    type: string
//	    in: query
//	    required: false
//	  + name: poolID
//	    description: Only include instances of this pool.
//	    type: string
//	    in: query
//	    required: false
//	  + name: scaleSetID
//	    description: Only include instances of this scale set.
//	    type: integer
//	    in: query
//	    required: false
//	  + name: entityID
//	    description: Only include instances of this repository, organization or enterprise.
//	    type: string
//	    in: query
//	    required: false
//	  + name: providerName
//	    description: Only include instances of this provider.
//	    type: string
//	    in: query
//	    required: false
//	  + name: provisioningModel
//	    description: Only include instances with this provisioning model (standard or spot).
//	    type: string
//	    in: query
//	    required: false
//
//	Responses:
//	  200: UsageSummary
//	  400: APIErrorResponse
func (a *APIController) GetUsageSummaryHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	filter, err := parseUsageFilter(query)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	loc := time.UTC
	if tz := query.Get("timeZone"); tz != "" {
		loc, err = time.LoadLocation(tz)
		if err != nil {
			handleError(ctx, w, gErrors.NewBadRequestError("invalid timeZone %q", tz))
			return
		}
	}

	summary, err := a.r.GetUsageSummary(ctx, filter, loc)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		slog.With(slog.Any("error", err)).ErrorContext(ctx, "failed to encode response")
	}
}
