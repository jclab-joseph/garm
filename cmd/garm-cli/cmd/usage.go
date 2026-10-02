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

package cmd

import (
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	apiClientUsage "github.com/cloudbase/garm/client/usage"
	"github.com/cloudbase/garm/cmd/garm-cli/common"
	"github.com/cloudbase/garm/params"
)

var (
	usageStart             string
	usageEnd               string
	usageMonth             string
	usageTimeZone          string
	usagePoolID            string
	usageProvisioningModel string
	usageInstanceName      string
	usageRunningOnly       bool
	usagePage              int64
	usagePageSize          int64
)

var usageCmd = &cobra.Command{
	Use:          "usage",
	Aliases:      []string{"cost"},
	SilenceUsage: true,
	Short:        "Instance usage and estimated cost",
	Long: `Show how long runner instances ran and their estimated cost.

Costs are estimated from the list prices reported by the provider when each
instance was created. Instances running across the date range boundaries are
prorated. The date range defaults to the current month.`,
	Run: nil,
}

// usageRange returns the requested date range. Dates are calendar days in the
// --time-zone (default: local) zone; --end is inclusive.
func usageRange() (time.Time, time.Time, *time.Location, error) {
	loc := time.Local
	if usageTimeZone != "" {
		var err error
		if loc, err = time.LoadLocation(usageTimeZone); err != nil {
			return time.Time{}, time.Time{}, nil, fmt.Errorf("invalid --time-zone: %w", err)
		}
	}
	if usageMonth != "" {
		if usageStart != "" || usageEnd != "" {
			return time.Time{}, time.Time{}, nil, fmt.Errorf("--month can't be combined with --start or --end")
		}
		month, err := time.ParseInLocation("2006-01", usageMonth, loc)
		if err != nil {
			return time.Time{}, time.Time{}, nil, fmt.Errorf("invalid --month (expected YYYY-MM): %w", err)
		}
		return month, month.AddDate(0, 1, 0), loc, nil
	}

	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)
	if usageStart != "" {
		parsed, err := time.ParseInLocation(time.DateOnly, usageStart, loc)
		if err != nil {
			return time.Time{}, time.Time{}, nil, fmt.Errorf("invalid --start (expected YYYY-MM-DD): %w", err)
		}
		start = parsed
	}
	if usageEnd != "" {
		parsed, err := time.ParseInLocation(time.DateOnly, usageEnd, loc)
		if err != nil {
			return time.Time{}, time.Time{}, nil, fmt.Errorf("invalid --end (expected YYYY-MM-DD): %w", err)
		}
		end = parsed.AddDate(0, 0, 1)
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, nil, fmt.Errorf("--start must not be after --end")
	}
	return start, end, loc, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

var usageSummaryCmd = &cobra.Command{
	Use:          "summary",
	Short:        "Show the estimated cost of a date range",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		if needsInit {
			return errNeedsInitError
		}
		start, end, loc, err := usageRange()
		if err != nil {
			return err
		}
		req := apiClientUsage.NewGetUsageSummaryParams().
			WithStart(start.Format(time.RFC3339)).
			WithEnd(end.Format(time.RFC3339)).
			WithTimeZone(optionalString(loc.String())).
			WithPoolID(optionalString(usagePoolID)).
			WithProvisioningModel(optionalString(usageProvisioningModel))
		response, err := apiCli.Usage.GetUsageSummary(req, authToken)
		if err != nil {
			return err
		}
		formatUsageSummary(response.Payload)
		return nil
	},
}

var usageListCmd = &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "List instance runs and their estimated cost",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		if needsInit {
			return errNeedsInitError
		}
		req := apiClientUsage.NewListInstanceUsageParams().
			WithPoolID(optionalString(usagePoolID)).
			WithProvisioningModel(optionalString(usageProvisioningModel)).
			WithInstanceName(optionalString(usageInstanceName)).
			WithPage(&usagePage).
			WithPageSize(&usagePageSize)
		if usageRunningOnly {
			req = req.WithRunningOnly(&usageRunningOnly)
		}
		// A run of one instance or of the running instances is shown whatever
		// its date, unless a range is asked for.
		if (usageInstanceName == "" && !usageRunningOnly) || usageStart != "" || usageEnd != "" || usageMonth != "" {
			start, end, _, err := usageRange()
			if err != nil {
				return err
			}
			req = req.WithStart(optionalString(start.Format(time.RFC3339))).WithEnd(optionalString(end.Format(time.RFC3339)))
		}
		response, err := apiCli.Usage.ListInstanceUsage(req, authToken)
		if err != nil {
			return err
		}
		formatInstanceUsage(response.Payload)
		return nil
	},
}

func formatHours(seconds float64) string {
	return fmt.Sprintf("%.2f", seconds/3600)
}

func formatMoney(amount float64, currency string) string {
	if currency == "" {
		currency = "USD"
	}
	return fmt.Sprintf("%.2f %s", amount, currency)
}

func formatUsageSummary(summary params.UsageSummary) {
	if outputFormat == common.OutputFormatJSON {
		printAsJSON(summary)
		return
	}
	t := table.NewWriter()
	t.SetTitle(fmt.Sprintf("%s - %s (%s)", summary.Start.Format(time.RFC3339), summary.End.Format(time.RFC3339), summary.TimeZone))
	t.AppendHeader(table.Row{"Group", "Key", "Instances", "Hours", "Cost"})
	t.AppendRow(table.Row{"total", "", summary.InstanceCount, formatHours(summary.BilledSeconds), formatMoney(summary.TotalCost, summary.Currency)})
	t.AppendRow(table.Row{"compute", "", "", "", formatMoney(summary.ComputeCost, summary.Currency)})
	t.AppendRow(table.Row{"egress", formatBytesIEC(summary.ExternalBytes) + " to the internet (estimate)", "", "", formatMoney(summary.EgressCost, summary.Currency)})
	t.AppendRow(table.Row{"running", "", summary.RunningCount, "", formatMoney(summary.RunningCost, summary.Currency)})
	t.AppendSeparator()
	groups := []struct {
		name   string
		groups []params.UsageGroup
	}{
		{"model", summary.ByProvisioningModel},
		{"pool", summary.ByPool},
		{"flavor", summary.ByFlavor},
		{"entity", summary.ByEntity},
	}
	for _, g := range groups {
		for _, group := range g.groups {
			key := group.Key
			if group.Label != "" {
				key = fmt.Sprintf("%s [%s]", group.Label, group.Key)
			}
			t.AppendRow(table.Row{g.name, key, group.InstanceCount, formatHours(group.BilledSeconds), formatMoney(group.Cost, summary.Currency)})
		}
		t.AppendSeparator()
	}
	for _, day := range summary.ByDay {
		if day.BilledSeconds == 0 {
			continue
		}
		t.AppendRow(table.Row{"day", day.Key, "", formatHours(day.BilledSeconds), formatMoney(day.Cost, summary.Currency)})
	}
	fmt.Println(t.Render())
	for _, warning := range summary.Warnings {
		fmt.Printf("warning: %s\n", warning)
	}
}

func formatInstanceUsage(response params.InstanceUsagePaginatedResponse) {
	if outputFormat == common.OutputFormatJSON {
		printAsJSON(response)
		return
	}
	t := table.NewWriter()
	t.AppendHeader(table.Row{"Instance", "Flavor", "Model", "Started", "Ended", "Hours", "Hourly", "Cost", "Egress", "Price", "Job"})
	for _, u := range response.Results {
		started, ended := "", "running"
		if u.StartedAt != nil {
			started = u.StartedAt.Local().Format(time.DateTime)
		}
		if u.EndedAt != nil {
			ended = u.EndedAt.Local().Format(time.DateTime)
		}
		hourly := ""
		if u.HourlyPrice != nil {
			hourly = fmt.Sprintf("%.4f %s", *u.HourlyPrice, u.Currency)
		}
		job := u.JobName
		if u.JobRepository != "" {
			job = fmt.Sprintf("%s: %s", u.JobRepository, u.JobName)
		}
		if u.WorkflowName != "" {
			job = fmt.Sprintf("%s (%s)", job, u.WorkflowName)
		}
		if u.JobConclusion != "" {
			job = fmt.Sprintf("%s [%s]", job, u.JobConclusion)
		}
		t.AppendRow(table.Row{
			u.InstanceName, u.Flavor, u.ProvisioningModel, started, ended,
			formatHours(u.RangeBilledSeconds), hourly, formatMoney(u.RangeCost+u.RangeEgressCost, u.Currency), egressColumn(u), u.PriceStatus, job,
		})
	}
	fmt.Println(t.Render())
	fmt.Printf("page %d of %d (%d runs)\n", response.CurrentPage, response.Pages, response.TotalCount)
}

func init() {
	for _, cmd := range []*cobra.Command{usageSummaryCmd, usageListCmd} {
		cmd.Flags().StringVar(&usageStart, "start", "", "First day of the date range (YYYY-MM-DD). Default: first day of this month.")
		cmd.Flags().StringVar(&usageEnd, "end", "", "Last day of the date range, inclusive (YYYY-MM-DD). Default: last day of this month.")
		cmd.Flags().StringVar(&usageMonth, "month", "", "Month to show (YYYY-MM), instead of --start and --end.")
		cmd.Flags().StringVar(&usageTimeZone, "time-zone", "", "IANA time zone of the dates (default: local).")
		cmd.Flags().StringVar(&usagePoolID, "pool", "", "Only include instances of this pool ID.")
		cmd.Flags().StringVar(&usageProvisioningModel, "provisioning-model", "", "Only include standard or spot instances.")
	}
	usageListCmd.Flags().StringVar(&usageInstanceName, "instance", "", "Only list runs of this instance.")
	usageListCmd.Flags().BoolVar(&usageRunningOnly, "running", false, "Only list instances that still exist.")
	usageListCmd.Flags().Int64Var(&usagePage, "page", 1, "Page to list.")
	usageListCmd.Flags().Int64Var(&usagePageSize, "page-size", 50, "Runs per page.")

	usageCmd.AddCommand(usageSummaryCmd, usageListCmd)
	rootCmd.AddCommand(usageCmd)
}

// formatBytesIEC formats a byte count with binary units, e.g. 1.5 GiB.
func formatBytesIEC(bytes float64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	unit := 0
	for bytes >= 1024 && unit < len(units)-1 {
		bytes /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%.0f %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", bytes, units[unit])
}

func egressColumn(u params.InstanceUsage) string {
	switch u.EgressStatus {
	case params.UsageEgressOK:
		text := formatBytesIEC(float64(u.EgressExternalBytes))
		if !u.EgressFinal {
			text += " (so far)"
		}
		return text
	case params.UsageEgressPending:
		return ""
	default:
		return string(u.EgressStatus)
	}
}
