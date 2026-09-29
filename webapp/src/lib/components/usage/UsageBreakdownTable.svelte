<script lang="ts">
	import type { UsageGroup } from '$lib/api/generated/api.js';
	import { formatCost, formatHours, formatShare, groupLabel } from '$lib/utils/usage.js';

	export let title: string;
	export let groups: UsageGroup[] = [];
	export let total: number = 0;
	export let currency: string | undefined = undefined;
</script>

<div class="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
	<div class="px-4 py-3 sm:px-6 border-b border-gray-200 dark:border-gray-700">
		<h3 class="text-base font-medium text-gray-900 dark:text-white">{title}</h3>
	</div>
	{#if groups.length === 0}
		<p class="px-4 py-6 sm:px-6 text-sm text-gray-500 dark:text-gray-400">No usage in this range.</p>
	{:else}
		<div class="overflow-x-auto">
			<table class="min-w-full text-sm">
				<thead class="bg-gray-50 dark:bg-gray-700">
					<tr>
						<th scope="col" class="px-4 sm:px-6 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">Name</th>
						<th scope="col" class="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">Cost</th>
						<th scope="col" class="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider hidden sm:table-cell">Hours</th>
						<th scope="col" class="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider hidden sm:table-cell">Runs</th>
						<th scope="col" class="px-4 sm:px-6 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">Share</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each groups as group (group.key)}
						<tr>
							<td class="px-4 sm:px-6 py-2 text-gray-900 dark:text-white max-w-[14rem] truncate" title={group.key}>
								{groupLabel(group)}
							</td>
							<td class="px-4 py-2 text-right text-gray-900 dark:text-white tabular-nums whitespace-nowrap">{formatCost(group.cost || 0, currency)}</td>
							<td class="px-4 py-2 text-right text-gray-700 dark:text-gray-300 tabular-nums whitespace-nowrap hidden sm:table-cell">{formatHours(group.billed_seconds)}</td>
							<td class="px-4 py-2 text-right text-gray-700 dark:text-gray-300 tabular-nums hidden sm:table-cell">{group.instance_count || 0}</td>
							<td class="px-4 sm:px-6 py-2 text-right text-gray-700 dark:text-gray-300 tabular-nums whitespace-nowrap">{formatShare(group.cost, total)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
