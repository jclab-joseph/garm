import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import UsagePage from './+page.svelte';
import type { InstanceUsage, UsageSummary } from '$lib/api/generated/api.js';

vi.unmock('$lib/components/PageHeader.svelte');
vi.unmock('$lib/components/Badge.svelte');

vi.mock('$app/state', () => ({
	page: {
		params: {},
		url: new URL('http://localhost/usage?start=2026-09-01&end=2026-09-30')
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	replaceState: vi.fn()
}));

vi.mock('$app/paths', () => ({
	resolve: vi.fn((path: string) => path)
}));

vi.mock('$lib/api/client.js', () => ({
	garmApi: {
		getUsageSummary: vi.fn(),
		listInstanceUsage: vi.fn()
	}
}));

vi.mock('$lib/utils/apiError', () => ({
	extractAPIError: vi.fn((err: any) => err.message || 'Unknown error')
}));

const summary: UsageSummary = {
	start: '2026-09-01T00:00:00Z',
	end: '2026-10-01T00:00:00Z',
	time_zone: 'UTC',
	currency: 'USD',
	total_cost: 12.345,
	billed_seconds: 5400,
	instance_count: 3,
	running_cost: 1.2,
	running_count: 2,
	unpriced_count: 1,
	by_provisioning_model: [
		{ key: 'standard', cost: 10, billed_seconds: 3600, instance_count: 2 },
		{ key: 'spot', cost: 2.345, billed_seconds: 1800, instance_count: 1 }
	],
	by_pool: [{ key: 'pool-1', label: 'n2-standard-4 (my-org)', cost: 12.345, billed_seconds: 5400, instance_count: 3 }],
	by_flavor: [
		{ key: 'n2-standard-4', cost: 10, billed_seconds: 3600, instance_count: 2 },
		{ key: 'e2-small', cost: 2.345, billed_seconds: 1800, instance_count: 1 }
	],
	by_entity: [{ key: 'ent-1', label: 'my-org', cost: 12.345, billed_seconds: 5400, instance_count: 3 }],
	by_day: [
		{ key: '2026-09-01', cost: 4, billed_seconds: 1800, instance_count: 1 },
		{ key: '2026-09-02', cost: 0, billed_seconds: 0, instance_count: 0 },
		{ key: '2026-09-03', cost: 8.345, billed_seconds: 3600, instance_count: 2 }
	],
	warnings: ['1 instance(s) have no price yet and are not included in the totals']
};

const runningSpot: InstanceUsage = {
	id: 'u1',
	instance_name: 'garm-running-1',
	pool_id: 'pool-1234567890',
	flavor: 'e2-small',
	entity_name: 'my-org',
	created_at: '2026-09-03T10:00:00Z',
	started_at: '2026-09-03T10:01:00Z',
	provisioning_model: 'spot',
	price_status: 'ok',
	hourly_price: 0.01234,
	currency: 'USD',
	billed_seconds: 4980,
	cost: 0.02,
	range_billed_seconds: 4980,
	range_cost: 0.02,
	job_repository: 'my-org/app',
	job_name: 'build',
	workflow_run_url: 'https://github.com/my-org/app/actions/runs/1',
	job_url: 'https://github.com/my-org/app/actions/runs/1/job/42',
	workflow_name: 'CI',
	head_branch: 'main',
	run_attempt: 2,
	job_conclusion: 'success',
	price: {
		currency: 'USD',
		hourly_price: 0.01234,
		region: 'us-east4',
		components: [
			{ name: 'vcpu', description: 'Spot E2 Instance Core', sku: 'SKU-1', quantity: 2, unit: 'vCPU', hourly_price: 0.01 },
			{ name: 'memory', description: 'Spot E2 Instance Ram', sku: 'SKU-2', quantity: 2, unit: 'GiB', hourly_price: 0.00234 }
		],
		warnings: ['local SSD is not priced']
	}
};

const endedStandard: InstanceUsage = {
	id: 'u2',
	instance_name: 'garm-ended-1',
	pool_id: 'pool-1234567890',
	flavor: 'n2-standard-4',
	created_at: '2026-09-01T08:00:00Z',
	started_at: '2026-09-01T08:01:00Z',
	ended_at: '2026-09-01T09:01:00Z',
	provisioning_model: 'standard',
	price_status: 'ok',
	hourly_price: 0.2,
	currency: 'USD',
	billed_seconds: 3600,
	cost: 0.2,
	range_billed_seconds: 3600,
	range_cost: 0.2
};

const failed: InstanceUsage = {
	id: 'u3',
	instance_name: 'garm-failed-1',
	created_at: '2026-09-02T08:00:00Z',
	ended_at: '2026-09-02T08:05:00Z',
	price_status: 'error',
	price_error: 'SKU not found'
};

let garmApi: any;
let replaceState: any;

describe('Cost page', () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		garmApi = (await import('$lib/api/client.js')).garmApi;
		replaceState = (await import('$app/navigation')).replaceState;
		garmApi.getUsageSummary.mockResolvedValue(summary);
		garmApi.listInstanceUsage.mockResolvedValue({
			results: [runningSpot, endedStandard, failed],
			pages: 1,
			current_page: 1,
			total_count: 3
		});
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('loads the range from the URL and passes RFC 3339 bounds and the time zone', async () => {
		render(UsagePage);

		await waitFor(() => expect(garmApi.getUsageSummary).toHaveBeenCalledTimes(1));
		const params = garmApi.getUsageSummary.mock.calls[0][0];
		expect(new Date(params.start).getTime()).toBe(new Date(2026, 8, 1).getTime());
		// End date is inclusive for the user: the API end is the next midnight
		expect(new Date(params.end).getTime()).toBe(new Date(2026, 9, 1).getTime());
		expect(params.timeZone).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone);
		expect(params.provisioningModel).toBeUndefined();

		expect(garmApi.listInstanceUsage).toHaveBeenCalledWith(
			expect.objectContaining({ start: params.start, end: params.end, page: 1, pageSize: 25 })
		);

		expect(screen.getByLabelText('From')).toHaveValue('2026-09-01');
		expect(screen.getByLabelText('To')).toHaveValue('2026-09-30');
	});

	it('renders the summary cards, warnings, chart and breakdowns', async () => {
		render(UsagePage);

		await waitFor(() => expect(screen.getByTestId('card-total')).toBeInTheDocument());

		expect(screen.getByRole('heading', { name: 'Cost' })).toBeInTheDocument();
		expect(within(screen.getByTestId('card-total')).getByText('$12.35')).toBeInTheDocument();
		expect(within(screen.getByTestId('card-total')).getByText(/3 instances · 1\.5 h billed/)).toBeInTheDocument();
		expect(within(screen.getByTestId('card-standard')).getByText('$10.00')).toBeInTheDocument();
		expect(within(screen.getByTestId('card-spot')).getByText('$2.35')).toBeInTheDocument();
		expect(within(screen.getByTestId('card-running')).getByText('$1.20')).toBeInTheDocument();
		expect(within(screen.getByTestId('card-running')).getByText(/2 running instances/)).toBeInTheDocument();

		expect(screen.getByTestId('usage-warnings')).toHaveTextContent('have no price yet');

		// Daily chart: one labelled bar per day and a y axis max label
		const chart = screen.getByRole('list', { name: 'Daily estimated cost' });
		expect(within(chart).getAllByRole('listitem')).toHaveLength(3);
		expect(within(chart).getByLabelText(/\$8\.35/)).toBeInTheDocument();
		expect(screen.getByTestId('chart-y-max')).toHaveTextContent('$10.00');

		// Breakdowns with labels and shares
		expect(screen.getByText('By pool')).toBeInTheDocument();
		expect(screen.getByText('n2-standard-4 (my-org)')).toBeInTheDocument();
		expect(screen.getAllByText('81.0%').length).toBeGreaterThan(0);
	});

	it('renders instance runs with model badges, links and costs', async () => {
		render(UsagePage);

		await waitFor(() => expect(screen.getAllByTestId('usage-row')).toHaveLength(3));
		const [first, second, third] = screen.getAllByTestId('usage-row');

		// Running instance links to its page; shows "Running", duration and cost
		expect(within(first).getByRole('link', { name: 'garm-running-1' })).toHaveAttribute('href', '/instances/garm-running-1');
		expect(within(first).getByText('Spot')).toBeInTheDocument();
		expect(within(first).getByText('Running')).toBeInTheDocument();
		expect(within(first).getByText('1h 23m')).toBeInTheDocument();
		expect(within(first).getByText('$0.0123/h')).toBeInTheDocument();
		expect(within(first).getByText('$0.02')).toBeInTheDocument();
		// The job links to the job itself, with its workflow, branch, attempt and result
		expect(within(first).getByRole('link', { name: 'my-org/app · build' })).toHaveAttribute(
			'href',
			'https://github.com/my-org/app/actions/runs/1/job/42'
		);
		expect(within(first).getByText('CI · main · attempt 2')).toBeInTheDocument();
		expect(within(first).getByText('Success')).toBeInTheDocument();

		// Ended instance has no link (the instance no longer exists)
		expect(within(second).queryByRole('link', { name: 'garm-ended-1' })).not.toBeInTheDocument();
		expect(within(second).getByText('garm-ended-1')).toBeInTheDocument();
		expect(within(second).getByText('On-demand')).toBeInTheDocument();
		expect(within(second).getByText('$0.20')).toBeInTheDocument();

		// Price error shows the error as a tooltip
		const errorBadge = within(third).getByText('Price error');
		expect(errorBadge.closest('[title]')).toHaveAttribute('title', 'SKU not found');
	});

	it('expands a row to show the price breakdown', async () => {
		render(UsagePage);

		await waitFor(() => expect(screen.getAllByTestId('usage-row')).toHaveLength(3));
		expect(screen.queryByText('Spot E2 Instance Core')).not.toBeInTheDocument();

		await fireEvent.click(screen.getByRole('button', { name: 'Show price details of garm-running-1' }));

		// The details row follows the expanded row in the desktop table
		const detailsRow = screen.getAllByTestId('usage-row')[0].nextElementSibling as HTMLElement;
		expect(within(detailsRow).getByText('Spot E2 Instance Core')).toBeInTheDocument();
		expect(within(detailsRow).getByText('SKU-2')).toBeInTheDocument();
		expect(within(detailsRow).getByText('local SSD is not priced')).toBeInTheDocument();
		expect(within(detailsRow).getByText('Total per hour')).toBeInTheDocument();
		expect(within(detailsRow).getByText(/us-east4/)).toBeInTheDocument();

		// Job and workflow run links
		const job = within(detailsRow).getByTestId('usage-job-details');
		expect(within(job).getByRole('link', { name: 'https://github.com/my-org/app/actions/runs/1/job/42' })).toBeInTheDocument();
		expect(within(job).getByRole('link', { name: 'https://github.com/my-org/app/actions/runs/1' })).toBeInTheDocument();
		expect(within(job).getByText(/CI · main · attempt 2/)).toBeInTheDocument();
	});

	it('applies a preset and keeps it in the URL', async () => {
		vi.useFakeTimers({ toFake: ['Date'] });
		vi.setSystemTime(new Date(2026, 8, 29, 12, 0));

		render(UsagePage);
		await waitFor(() => expect(garmApi.getUsageSummary).toHaveBeenCalledTimes(1));

		await fireEvent.click(screen.getByRole('button', { name: 'Last month' }));

		await waitFor(() => expect(garmApi.getUsageSummary).toHaveBeenCalledTimes(2));
		const params = garmApi.getUsageSummary.mock.calls[1][0];
		expect(new Date(params.start).getTime()).toBe(new Date(2026, 7, 1).getTime());
		expect(new Date(params.end).getTime()).toBe(new Date(2026, 8, 1).getTime());

		expect(replaceState).toHaveBeenCalled();
		const url = replaceState.mock.calls.at(-1)[0] as URL;
		expect(url.searchParams.get('start')).toBe('2026-08-01');
		expect(url.searchParams.get('end')).toBe('2026-08-31');
		expect(screen.getByRole('button', { name: 'Last month' })).toHaveAttribute('aria-pressed', 'true');
	});

	it('filters both calls by provisioning model', async () => {
		render(UsagePage);
		await waitFor(() => expect(garmApi.getUsageSummary).toHaveBeenCalledTimes(1));

		await fireEvent.change(screen.getByLabelText('Provisioning model'), { target: { value: 'spot' } });

		await waitFor(() => expect(garmApi.getUsageSummary).toHaveBeenCalledTimes(2));
		expect(garmApi.getUsageSummary.mock.calls[1][0].provisioningModel).toBe('spot');
		expect(garmApi.listInstanceUsage.mock.calls.at(-1)[0].provisioningModel).toBe('spot');
	});

	it('shows an error for an invalid custom range without calling the API', async () => {
		render(UsagePage);
		await waitFor(() => expect(garmApi.getUsageSummary).toHaveBeenCalledTimes(1));

		const to = screen.getByLabelText('To');
		await fireEvent.input(to, { target: { value: '2026-08-01' } });
		await fireEvent.change(to);

		expect(screen.getByRole('alert')).toHaveTextContent('must not be before');
		expect(garmApi.getUsageSummary).toHaveBeenCalledTimes(1);
	});

	it('shows errors from the API', async () => {
		garmApi.getUsageSummary.mockRejectedValue(new Error('summary exploded'));
		garmApi.listInstanceUsage.mockRejectedValue(new Error('list exploded'));

		render(UsagePage);

		await waitFor(() => expect(screen.getByText('summary exploded')).toBeInTheDocument());
		expect(screen.getByText('list exploded')).toBeInTheDocument();
	});
});
