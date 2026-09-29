/**
 * Helpers for the instance usage / estimated cost views.
 *
 * Dates picked by the user are calendar dates ("YYYY-MM-DD") in the browser's
 * local time zone. The end date is inclusive for the user; the API takes a
 * half-open [start, end) range of RFC 3339 instants, so the API end is local
 * midnight of the day after the end date.
 */

import type { InstanceUsage, UsageGroup, UsageSummary } from '$lib/api/generated/api.js';

export type UsagePreset = 'this_month' | 'last_month' | 'last_7_days' | 'last_30_days';

/** A range of calendar dates, both inclusive, formatted as YYYY-MM-DD. */
export interface DateRange {
	start: string;
	end: string;
}

export type ProvisioningModelFilter = '' | 'standard' | 'spot';

export const USAGE_PRESETS: Array<{ id: UsagePreset; label: string }> = [
	{ id: 'this_month', label: 'This month' },
	{ id: 'last_month', label: 'Last month' },
	{ id: 'last_7_days', label: 'Last 7 days' },
	{ id: 'last_30_days', label: 'Last 30 days' }
];

/** The backend rejects summary ranges longer than this. */
export const MAX_RANGE_DAYS = 366;

const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})$/;

function pad(n: number, width = 2): string {
	return String(Math.abs(n)).padStart(width, '0');
}

/** Formats a Date as YYYY-MM-DD using its local calendar date. */
export function toDateString(date: Date): string {
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/**
 * Parses YYYY-MM-DD into a Date at local midnight. Returns null for anything
 * that is not a real calendar date (e.g. 2026-02-30).
 */
export function parseDateString(value: string | null | undefined): Date | null {
	if (!value) return null;
	const match = DATE_RE.exec(value);
	if (!match) return null;
	const year = Number(match[1]);
	const month = Number(match[2]) - 1;
	const day = Number(match[3]);
	const date = new Date(year, month, day);
	if (date.getFullYear() !== year || date.getMonth() !== month || date.getDate() !== day) {
		return null;
	}
	return date;
}

function addDays(date: Date, days: number): Date {
	// Use the calendar constructor rather than adding milliseconds so DST
	// transitions do not shift the result off midnight.
	return new Date(date.getFullYear(), date.getMonth(), date.getDate() + days);
}

/** Returns the date range of a preset, relative to `now` (local time). */
export function presetRange(preset: UsagePreset, now: Date = new Date()): DateRange {
	const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
	switch (preset) {
		case 'this_month':
			return {
				start: toDateString(new Date(today.getFullYear(), today.getMonth(), 1)),
				end: toDateString(new Date(today.getFullYear(), today.getMonth() + 1, 0))
			};
		case 'last_month':
			return {
				start: toDateString(new Date(today.getFullYear(), today.getMonth() - 1, 1)),
				end: toDateString(new Date(today.getFullYear(), today.getMonth(), 0))
			};
		case 'last_7_days':
			return { start: toDateString(addDays(today, -6)), end: toDateString(today) };
		case 'last_30_days':
			return { start: toDateString(addDays(today, -29)), end: toDateString(today) };
	}
}

/** Returns the preset that produces exactly `range` today, if any. */
export function matchPreset(range: DateRange, now: Date = new Date()): UsagePreset | null {
	for (const { id } of USAGE_PRESETS) {
		const candidate = presetRange(id, now);
		if (candidate.start === range.start && candidate.end === range.end) return id;
	}
	return null;
}

/**
 * Formats a Date as an RFC 3339 timestamp with the local UTC offset,
 * e.g. 2026-09-01T00:00:00+09:00.
 */
export function toRFC3339(date: Date): string {
	const offsetMinutes = -date.getTimezoneOffset();
	const sign = offsetMinutes >= 0 ? '+' : '-';
	const offset = `${sign}${pad(Math.floor(Math.abs(offsetMinutes) / 60))}:${pad(Math.abs(offsetMinutes) % 60)}`;
	return (
		`${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
		`T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}${offset}`
	);
}

/**
 * Converts an inclusive calendar date range to the half-open RFC 3339 range the
 * API expects: start = local midnight of the start date, end = local midnight
 * of the day after the end date.
 */
export function rangeToRFC3339(range: DateRange): { start: string; end: string } {
	const start = parseDateString(range.start);
	const end = parseDateString(range.end);
	if (!start || !end) {
		throw new Error(`invalid date range ${range.start} - ${range.end}`);
	}
	return { start: toRFC3339(start), end: toRFC3339(addDays(end, 1)) };
}

/** Number of calendar days in an inclusive range (0 when invalid). */
export function rangeDays(range: DateRange): number {
	const start = parseDateString(range.start);
	const end = parseDateString(range.end);
	if (!start || !end) return 0;
	// Round to absorb DST hour shifts.
	return Math.round((end.getTime() - start.getTime()) / 86_400_000) + 1;
}

/** Returns a user facing error for an invalid range, or null when it is valid. */
export function validateRange(range: DateRange): string | null {
	const start = parseDateString(range.start);
	const end = parseDateString(range.end);
	if (!start || !end) return 'Please pick a valid start and end date.';
	if (end < start) return 'The end date must not be before the start date.';
	if (rangeDays(range) > MAX_RANGE_DAYS) return `The range can be at most ${MAX_RANGE_DAYS} days.`;
	return null;
}

/**
 * Reads the range and provisioning model filter from URL query parameters,
 * falling back to "this month" when they are missing or invalid.
 */
export function rangeFromSearchParams(
	params: URLSearchParams,
	now: Date = new Date()
): { range: DateRange; model: ProvisioningModelFilter } {
	const candidate = { start: params.get('start') || '', end: params.get('end') || '' };
	const range = validateRange(candidate) === null ? candidate : presetRange('this_month', now);
	const modelParam = params.get('model');
	const model: ProvisioningModelFilter =
		modelParam === 'standard' || modelParam === 'spot' ? modelParam : '';
	return { range, model };
}

/** Builds the query string (without "?") that encodes the range and filter. */
export function rangeToSearchParams(range: DateRange, model: ProvisioningModelFilter): string {
	const params = new URLSearchParams({ start: range.start, end: range.end });
	if (model) params.set('model', model);
	return params.toString();
}

/** The browser's IANA time zone, e.g. "Europe/Berlin". */
export function browserTimeZone(): string | undefined {
	try {
		return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
	} catch {
		return undefined;
	}
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

const DEFAULT_CURRENCY = 'USD';

function moneyFormatter(currency: string | undefined, digits: number): Intl.NumberFormat {
	try {
		return new Intl.NumberFormat(undefined, {
			style: 'currency',
			currency: currency || DEFAULT_CURRENCY,
			minimumFractionDigits: digits,
			maximumFractionDigits: digits
		});
	} catch {
		// Unknown currency code: fall back to USD rather than breaking the page.
		return new Intl.NumberFormat(undefined, {
			style: 'currency',
			currency: DEFAULT_CURRENCY,
			minimumFractionDigits: digits,
			maximumFractionDigits: digits
		});
	}
}

/**
 * Formats a cost with 2 decimals. Tiny non-zero amounts that would round to
 * zero are shown as "< $0.01". Missing values are shown as an em dash.
 */
export function formatCost(value: number | null | undefined, currency?: string): string {
	if (value === null || value === undefined || Number.isNaN(value)) return '—';
	const formatter = moneyFormatter(currency, 2);
	if (value !== 0 && Math.abs(value) < 0.005) {
		return `< ${formatter.format(value < 0 ? -0.01 : 0.01)}`;
	}
	return formatter.format(value);
}

/** Formats an hourly price with 4 decimals, e.g. "$0.0421". */
export function formatHourlyPrice(value: number | null | undefined, currency?: string): string {
	if (value === null || value === undefined || Number.isNaN(value)) return '—';
	return moneyFormatter(currency, 4).format(value);
}

/**
 * Formats a number of seconds as a compact duration: "42s", "5m", "1h 23m",
 * "2d 3h".
 */
export function formatDuration(seconds: number | null | undefined): string {
	if (seconds === null || seconds === undefined || Number.isNaN(seconds) || seconds <= 0) return '0m';
	const total = Math.floor(seconds);
	if (total < 60) return `${total}s`;
	const days = Math.floor(total / 86400);
	const hours = Math.floor((total % 86400) / 3600);
	const minutes = Math.floor((total % 3600) / 60);
	if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
	if (hours > 0) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
	return `${minutes}m`;
}

/** Formats billed seconds as hours with one decimal, e.g. "12.5 h". */
export function formatHours(seconds: number | null | undefined): string {
	const hours = (seconds || 0) / 3600;
	const digits = hours > 0 && hours < 0.1 ? 2 : 1;
	return `${hours.toLocaleString(undefined, { minimumFractionDigits: digits, maximumFractionDigits: digits })} h`;
}

/** Formats cost / total as a percentage, e.g. "42.1%". */
export function formatShare(value: number | undefined, total: number | undefined): string {
	if (!total || !value) return '0%';
	const pct = (value / total) * 100;
	if (pct > 0 && pct < 0.1) return '< 0.1%';
	return `${pct.toFixed(1)}%`;
}

/** Formats a YYYY-MM-DD day key as a short local label, e.g. "Sep 3". */
export function formatDayLabel(key: string | undefined, options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' }): string {
	const date = parseDateString(key);
	if (!date) return key || '';
	return date.toLocaleDateString(undefined, options);
}

// ---------------------------------------------------------------------------
// Record helpers
// ---------------------------------------------------------------------------

/** A usage record without ended_at belongs to an instance that still exists. */
export function isRunning(record: InstanceUsage): boolean {
	return !record.ended_at;
}

export type BadgeVariant = 'success' | 'error' | 'warning' | 'info' | 'gray' | 'purple' | 'blue';

/** Badge describing the pricing state / provisioning model of a record. */
export function modelBadge(record: InstanceUsage): { text: string; variant: BadgeVariant; title?: string } {
	switch (record.price_status) {
		case 'pending':
			return { text: 'Pending price', variant: 'gray', title: 'The price has not been looked up yet' };
		case 'unsupported':
			return {
				text: 'Unsupported',
				variant: 'gray',
				title: record.price_error || 'The provider does not support price estimates'
			};
		case 'error':
			return { text: 'Price error', variant: 'error', title: record.price_error || 'Price lookup failed' };
	}
	if (record.provisioning_model === 'spot') return { text: 'Spot', variant: 'purple' };
	if (record.provisioning_model === 'standard') return { text: 'On-demand', variant: 'blue' };
	return { text: 'Pending price', variant: 'gray', title: 'The price has not been looked up yet' };
}

/** Human readable name of a provisioning model key. */
export function modelLabel(key: string | undefined): string {
	if (key === 'standard') return 'On-demand';
	if (key === 'spot') return 'Spot';
	return 'Unknown';
}

/** Display name of a summary group: its label, else its key. */
export function groupLabel(group: UsageGroup): string {
	if (group.label) return group.label;
	if (!group.key || group.key === 'unknown') return 'Unknown';
	return group.key;
}

/** Cost of one provisioning model ("standard" / "spot") in a summary. */
export function modelCost(summary: UsageSummary | null | undefined, key: 'standard' | 'spot'): number {
	return summary?.by_provisioning_model?.find((g) => g.key === key)?.cost ?? 0;
}

/** Number of instances of one provisioning model in a summary. */
export function modelCount(summary: UsageSummary | null | undefined, key: 'standard' | 'spot'): number {
	return summary?.by_provisioning_model?.find((g) => g.key === key)?.instance_count ?? 0;
}

/**
 * Picks a "nice" upper bound for a chart axis (1, 2, 2.5 or 5 times a power of
 * ten) that is >= max.
 */
export function niceMax(max: number): number {
	if (!(max > 0)) return 0;
	const magnitude = Math.pow(10, Math.floor(Math.log10(max)));
	for (const step of [1, 2, 2.5, 5, 10]) {
		const candidate = step * magnitude;
		if (candidate >= max - magnitude * 1e-9) return candidate;
	}
	return 10 * magnitude;
}

/**
 * Cost of a run so far for compact displays (instance list / detail): the
 * formatted cost once priced, "Pending" while the price is being looked up and
 * an em dash when it cannot be priced or there is no record.
 */
export function costSoFarText(record: InstanceUsage | null | undefined, fallbackCurrency?: string): string {
	if (!record) return '—';
	if (record.price_status === 'pending') return 'Pending';
	if (record.hourly_price === undefined || record.hourly_price === null) return '—';
	return formatCost(record.cost ?? 0, record.currency || fallbackCurrency);
}
