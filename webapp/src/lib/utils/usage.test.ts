import { describe, it, expect } from 'vitest';
import {
	costSoFarText,
	formatCost,
	formatDuration,
	formatHourlyPrice,
	formatHours,
	formatShare,
	groupLabel,
	isRunning,
	matchPreset,
	modelBadge,
	modelCost,
	niceMax,
	parseDateString,
	presetRange,
	rangeDays,
	rangeFromSearchParams,
	rangeToRFC3339,
	rangeToSearchParams,
	toDateString,
	toRFC3339,
	validateRange,
	jobConclusionBadge,
	workflowText
} from './usage.js';
import type { InstanceUsage } from '$lib/api/client.js';

// 2026-09-29 15:30 local time (a Tuesday)
const NOW = new Date(2026, 8, 29, 15, 30);

describe('date helpers', () => {
	it('formats and parses local calendar dates', () => {
		expect(toDateString(new Date(2026, 0, 5, 23, 59))).toBe('2026-01-05');
		const parsed = parseDateString('2026-03-07');
		expect(parsed?.getFullYear()).toBe(2026);
		expect(parsed?.getMonth()).toBe(2);
		expect(parsed?.getDate()).toBe(7);
		expect(parsed?.getHours()).toBe(0);
	});

	it('rejects invalid dates', () => {
		expect(parseDateString('2026-02-30')).toBeNull();
		expect(parseDateString('2026-13-01')).toBeNull();
		expect(parseDateString('yesterday')).toBeNull();
		expect(parseDateString('')).toBeNull();
		expect(parseDateString(null)).toBeNull();
	});
});

describe('presetRange', () => {
	it('this month covers the whole calendar month', () => {
		expect(presetRange('this_month', NOW)).toEqual({ start: '2026-09-01', end: '2026-09-30' });
	});

	it('last month covers the whole previous month', () => {
		expect(presetRange('last_month', NOW)).toEqual({ start: '2026-08-01', end: '2026-08-31' });
		// January rolls back to December of the previous year
		expect(presetRange('last_month', new Date(2026, 0, 15))).toEqual({ start: '2025-12-01', end: '2025-12-31' });
		// Leap year February
		expect(presetRange('last_month', new Date(2028, 2, 3))).toEqual({ start: '2028-02-01', end: '2028-02-29' });
	});

	it('last N days include today', () => {
		expect(presetRange('last_7_days', NOW)).toEqual({ start: '2026-09-23', end: '2026-09-29' });
		expect(presetRange('last_30_days', NOW)).toEqual({ start: '2026-08-31', end: '2026-09-29' });
		expect(rangeDays(presetRange('last_7_days', NOW))).toBe(7);
		expect(rangeDays(presetRange('last_30_days', NOW))).toBe(30);
	});

	it('matches a range back to its preset', () => {
		expect(matchPreset({ start: '2026-09-01', end: '2026-09-30' }, NOW)).toBe('this_month');
		expect(matchPreset({ start: '2026-09-23', end: '2026-09-29' }, NOW)).toBe('last_7_days');
		expect(matchPreset({ start: '2026-09-02', end: '2026-09-30' }, NOW)).toBeNull();
	});
});

describe('rangeToRFC3339', () => {
	it('uses local midnight of the start date and of the day after the end date', () => {
		const { start, end } = rangeToRFC3339({ start: '2026-09-01', end: '2026-09-30' });
		expect(start).toMatch(/^2026-09-01T00:00:00[+-]\d{2}:\d{2}$/);
		expect(end).toMatch(/^2026-10-01T00:00:00[+-]\d{2}:\d{2}$/);
		expect(new Date(start).getTime()).toBe(new Date(2026, 8, 1).getTime());
		expect(new Date(end).getTime()).toBe(new Date(2026, 9, 1).getTime());
	});

	it('treats a single day as a full day', () => {
		const { start, end } = rangeToRFC3339({ start: '2026-12-31', end: '2026-12-31' });
		expect(new Date(start).getTime()).toBe(new Date(2026, 11, 31).getTime());
		expect(new Date(end).getTime()).toBe(new Date(2027, 0, 1).getTime());
	});

	it('stays on midnight across DST changes', () => {
		// Whatever the local zone, the end must be exactly local midnight.
		for (const range of [
			{ start: '2026-03-01', end: '2026-03-31' },
			{ start: '2026-10-01', end: '2026-10-31' }
		]) {
			const end = new Date(rangeToRFC3339(range).end);
			expect(end.getHours()).toBe(0);
			expect(end.getMinutes()).toBe(0);
		}
	});

	it('formats the local UTC offset', () => {
		const date = new Date(2026, 5, 15, 8, 9, 10);
		const rfc = toRFC3339(date);
		expect(new Date(rfc).getTime()).toBe(date.getTime());
	});

	it('throws on invalid ranges', () => {
		expect(() => rangeToRFC3339({ start: 'nope', end: '2026-01-01' })).toThrow();
	});
});

describe('validateRange and URL params', () => {
	it('validates ranges', () => {
		expect(validateRange({ start: '2026-09-01', end: '2026-09-30' })).toBeNull();
		expect(validateRange({ start: '2026-09-30', end: '2026-09-01' })).toMatch(/before/);
		expect(validateRange({ start: '', end: '2026-09-01' })).toMatch(/valid/);
		expect(validateRange({ start: '2025-01-01', end: '2026-01-01' })).toBeNull(); // 366 days
		expect(validateRange({ start: '2025-01-01', end: '2026-01-02' })).toMatch(/366/);
	});

	it('reads the range and model from the query, falling back to this month', () => {
		expect(rangeFromSearchParams(new URLSearchParams('start=2026-08-01&end=2026-08-15&model=spot'), NOW)).toEqual({
			range: { start: '2026-08-01', end: '2026-08-15' },
			model: 'spot'
		});
		expect(rangeFromSearchParams(new URLSearchParams('start=bad&model=other'), NOW)).toEqual({
			range: { start: '2026-09-01', end: '2026-09-30' },
			model: ''
		});
	});

	it('writes the range and model to the query', () => {
		expect(rangeToSearchParams({ start: '2026-09-01', end: '2026-09-30' }, '')).toBe('start=2026-09-01&end=2026-09-30');
		expect(rangeToSearchParams({ start: '2026-09-01', end: '2026-09-30' }, 'standard')).toBe(
			'start=2026-09-01&end=2026-09-30&model=standard'
		);
	});
});

describe('formatting', () => {
	it('formats durations', () => {
		expect(formatDuration(undefined)).toBe('0m');
		expect(formatDuration(0)).toBe('0m');
		expect(formatDuration(42)).toBe('42s');
		expect(formatDuration(300)).toBe('5m');
		expect(formatDuration(3600)).toBe('1h');
		expect(formatDuration(4980)).toBe('1h 23m');
		expect(formatDuration(86400 * 2 + 3600 * 3 + 59)).toBe('2d 3h');
		expect(formatDuration(86400)).toBe('1d');
	});

	it('formats costs with 2 decimals and tiny amounts as "< $0.01"', () => {
		expect(formatCost(12.345)).toBe('$12.35');
		expect(formatCost(0)).toBe('$0.00');
		expect(formatCost(0.004)).toBe('< $0.01');
		expect(formatCost(0.0051)).toBe('$0.01');
		expect(formatCost(undefined)).toBe('—');
		expect(formatCost(1234.5, 'EUR')).toBe('€1,234.50');
	});

	it('falls back to USD for an unknown currency code', () => {
		expect(formatCost(1, 'not-a-currency')).toBe('$1.00');
	});

	it('formats hourly prices with 4 decimals', () => {
		expect(formatHourlyPrice(0.04213)).toBe('$0.0421');
		expect(formatHourlyPrice(1.5)).toBe('$1.5000');
		expect(formatHourlyPrice(undefined)).toBe('—');
	});

	it('formats hours and shares', () => {
		expect(formatHours(5400)).toBe('1.5 h');
		expect(formatHours(0)).toBe('0.0 h');
		expect(formatHours(36)).toBe('0.01 h');
		expect(formatShare(25, 100)).toBe('25.0%');
		expect(formatShare(0.01, 100)).toBe('< 0.1%');
		expect(formatShare(5, 0)).toBe('0%');
	});

	it('picks nice axis maxima', () => {
		expect(niceMax(0)).toBe(0);
		expect(niceMax(0.83)).toBe(1);
		expect(niceMax(1.2)).toBe(2);
		expect(niceMax(2.2)).toBe(2.5);
		expect(niceMax(4)).toBe(5);
		expect(niceMax(7)).toBe(10);
		expect(niceMax(10)).toBe(10);
		expect(niceMax(130)).toBe(200);
	});
});

describe('record helpers', () => {
	it('detects running records', () => {
		expect(isRunning({ instance_name: 'a' })).toBe(true);
		expect(isRunning({ instance_name: 'a', ended_at: '2026-09-01T00:00:00Z' })).toBe(false);
	});

	it('maps price status and model to badges', () => {
		expect(modelBadge({ price_status: 'ok', provisioning_model: 'spot' })).toMatchObject({ text: 'Spot', variant: 'purple' });
		expect(modelBadge({ price_status: 'ok', provisioning_model: 'standard' })).toMatchObject({ text: 'On-demand' });
		expect(modelBadge({ price_status: 'pending' })).toMatchObject({ text: 'Pending price' });
		expect(modelBadge({ price_status: 'unsupported' })).toMatchObject({ text: 'Unsupported' });
		expect(modelBadge({ price_status: 'error', price_error: 'boom' })).toEqual({ text: 'Price error', variant: 'error', title: 'boom' });
		expect(modelBadge({})).toMatchObject({ text: 'Pending price' });
	});

	it('labels groups and reads model costs', () => {
		expect(groupLabel({ key: 'pool-1', label: 'n2-standard-2 (org)' })).toBe('n2-standard-2 (org)');
		expect(groupLabel({ key: 'e2-small' })).toBe('e2-small');
		expect(groupLabel({ key: 'unknown' })).toBe('Unknown');
		const summary = { by_provisioning_model: [{ key: 'spot', cost: 3 }, { key: 'standard', cost: 7 }] };
		expect(modelCost(summary, 'standard')).toBe(7);
		expect(modelCost(summary, 'spot')).toBe(3);
		expect(modelCost(null, 'spot')).toBe(0);
	});

	it('formats the cost so far', () => {
		expect(costSoFarText(undefined)).toBe('—');
		expect(costSoFarText({ price_status: 'pending' })).toBe('Pending');
		expect(costSoFarText({ price_status: 'unsupported' })).toBe('—');
		expect(costSoFarText({ price_status: 'ok', hourly_price: 0.1, cost: 1.5, currency: 'USD' })).toBe('$1.50');
	});
});

describe('job details', () => {
	it('maps job conclusions to badges', () => {
		expect(jobConclusionBadge(undefined)).toBeNull();
		expect(jobConclusionBadge('success')).toEqual({ text: 'Success', variant: 'success' });
		expect(jobConclusionBadge('failure')).toEqual({ text: 'Failure', variant: 'error' });
		expect(jobConclusionBadge('timed_out')).toEqual({ text: 'timed out', variant: 'error' });
		expect(jobConclusionBadge('cancelled')).toEqual({ text: 'Cancelled', variant: 'gray' });
		expect(jobConclusionBadge('skipped')).toEqual({ text: 'skipped', variant: 'gray' });
	});

	it('describes the workflow, branch and attempt', () => {
		expect(workflowText({ workflow_name: 'CI', head_branch: 'main' } as InstanceUsage)).toBe('CI · main');
		expect(workflowText({ workflow_name: 'CI', head_branch: 'main', run_attempt: 1 } as InstanceUsage)).toBe('CI · main');
		expect(workflowText({ workflow_name: 'CI', run_attempt: 3 } as InstanceUsage)).toBe('CI · attempt 3');
		expect(workflowText({} as InstanceUsage)).toBe('');
	});
});
