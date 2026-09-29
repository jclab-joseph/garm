import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/svelte';
import InstanceDetailsPage from './+page.svelte';
import { createMockInstance } from '../../../test/factories.js';

vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((callback) => {
			callback({ params: { id: 'test-instance' }, url: { pathname: '/instances/test-instance' } });
			return () => {};
		})
	}
}));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

vi.unmock('$lib/components/Badge.svelte');

vi.mock('$lib/api/client.js', () => ({
	garmApi: {
		getInstance: vi.fn(),
		deleteInstance: vi.fn(),
		listInstanceUsage: vi.fn()
	}
}));

vi.mock('$lib/stores/websocket.js', () => ({
	websocketStore: { subscribeToEntity: vi.fn(() => vi.fn()) }
}));

describe('Instance details page - cost card', () => {
	let garmApi: any;

	beforeEach(async () => {
		vi.clearAllMocks();
		garmApi = (await import('$lib/api/client.js')).garmApi;
		garmApi.getInstance.mockResolvedValue(createMockInstance({ name: 'test-instance' }));
	});

	it('shows the latest run of the instance', async () => {
		garmApi.listInstanceUsage.mockResolvedValue({
			results: [
				{
					instance_name: 'test-instance',
					started_at: '2026-09-29T10:00:00Z',
					provisioning_model: 'spot',
					price_status: 'ok',
					hourly_price: 0.0421,
					currency: 'USD',
					billed_seconds: 4980,
					cost: 0.06
				}
			]
		});

		render(InstanceDetailsPage);

		const card = await screen.findByTestId('instance-cost-card');
		expect(garmApi.listInstanceUsage).toHaveBeenCalledWith({ instanceName: 'test-instance', pageSize: 1 });
		expect(within(card).getByText('Spot')).toBeInTheDocument();
		expect(within(card).getByText('$0.0421/h')).toBeInTheDocument();
		expect(within(card).getByText('Running for')).toBeInTheDocument();
		expect(within(card).getByText('1h 23m')).toBeInTheDocument();
		expect(within(card).getByText('$0.06')).toBeInTheDocument();
	});

	it('hides the card when the usage request fails', async () => {
		garmApi.listInstanceUsage.mockRejectedValue(new Error('boom'));

		render(InstanceDetailsPage);

		await waitFor(() => expect(screen.getByText('Instance Information')).toBeInTheDocument());
		await waitFor(() => expect(garmApi.listInstanceUsage).toHaveBeenCalled());
		expect(screen.queryByTestId('instance-cost-card')).not.toBeInTheDocument();
		expect(screen.queryByText('boom')).not.toBeInTheDocument();
	});
});
