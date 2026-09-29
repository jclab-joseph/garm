import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import InstancesPage from './+page.svelte';
import { createMockInstance } from '../../test/factories.js';

vi.mock('$app/stores', () => ({}));
vi.mock('$app/navigation', () => ({}));

vi.unmock('$lib/components/PageHeader.svelte');
vi.unmock('$lib/components/DataTable.svelte');
vi.unmock('$lib/components/cells');

vi.mock('$lib/api/client.js', () => ({
	garmApi: {
		listInstances: vi.fn(),
		deleteInstance: vi.fn(),
		listInstanceUsage: vi.fn()
	}
}));

vi.mock('$lib/stores/toast.js', () => ({
	toastStore: { success: vi.fn(), error: vi.fn(), info: vi.fn() }
}));

vi.mock('$lib/stores/websocket.js', () => ({
	websocketStore: { subscribeToEntity: vi.fn(() => vi.fn()) }
}));

const instances = [
	createMockInstance({ id: 'i1', name: 'priced-instance' }),
	createMockInstance({ id: 'i2', name: 'pending-instance' }),
	createMockInstance({ id: 'i3', name: 'unknown-instance' })
];

describe('Instances page - cost column', () => {
	let garmApi: any;

	beforeEach(async () => {
		vi.clearAllMocks();
		garmApi = (await import('$lib/api/client.js')).garmApi;
		garmApi.listInstances.mockResolvedValue(instances);
	});

	it('shows the cost so far from a single running-only usage request', async () => {
		garmApi.listInstanceUsage.mockResolvedValue({
			results: [
				{ instance_name: 'priced-instance', price_status: 'ok', hourly_price: 0.5, cost: 1.25, currency: 'USD' },
				{ instance_name: 'pending-instance', price_status: 'pending' }
			]
		});

		render(InstancesPage);

		await waitFor(() => expect(screen.getByText('$1.25')).toBeInTheDocument());
		expect(screen.getByText('Cost')).toBeInTheDocument();
		expect(screen.getByText('Pending')).toBeInTheDocument();
		expect(garmApi.listInstanceUsage).toHaveBeenCalledTimes(1);
		expect(garmApi.listInstanceUsage).toHaveBeenCalledWith({ runningOnly: true, pageSize: 500 });
	});

	it('still renders the instances when the usage request fails', async () => {
		garmApi.listInstanceUsage.mockRejectedValue(new Error('forbidden'));

		render(InstancesPage);

		await waitFor(() => expect(screen.getAllByText('priced-instance').length).toBeGreaterThan(0));
		expect(screen.queryByText('forbidden')).not.toBeInTheDocument();
		expect(screen.queryByText(/\$\d/)).not.toBeInTheDocument();
	});
});
