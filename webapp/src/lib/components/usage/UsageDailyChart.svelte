<script lang="ts">
	import type { UsageGroup } from '$lib/api/generated/api.js';
	import { formatCost, formatDayLabel, formatHours, niceMax } from '$lib/utils/usage.js';

	/** One entry per calendar day, chronological (UsageSummary.by_day). */
	export let days: UsageGroup[] = [];
	export let currency: string | undefined = undefined;

	$: maxCost = days.reduce((max, d) => Math.max(max, d.cost || 0), 0);
	$: axisMax = niceMax(maxCost);
	// Label every day for short ranges; otherwise about 6 evenly spaced labels.
	$: labelEvery = days.length <= 14 ? 1 : Math.ceil(days.length / 6);

	function barHeight(cost: number | undefined): number {
		if (!axisMax || !cost) return 0;
		return Math.max((cost / axisMax) * 100, 0.5);
	}

	function dayTitle(day: UsageGroup): string {
		const date = formatDayLabel(day.key, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' });
		const runs = day.instance_count === 1 ? '1 instance' : `${day.instance_count || 0} instances`;
		return `${date}: ${formatCost(day.cost || 0, currency)} (${formatHours(day.billed_seconds)}, ${runs})`;
	}
</script>

{#if days.length === 0}
	<p class="text-sm text-gray-500 dark:text-gray-400">No data for this range.</p>
{:else}
	<div class="flex gap-2">
		<!-- Y axis labels -->
		<div class="flex flex-col justify-between h-48 text-right text-xs text-gray-500 dark:text-gray-400 tabular-nums shrink-0" aria-hidden="true">
			<span data-testid="chart-y-max">{formatCost(axisMax, currency)}</span>
			<span>{formatCost(0, currency)}</span>
		</div>

		<div class="flex-1 min-w-0">
			<!-- Plot area -->
			<div class="relative h-48">
				<!-- Recessive grid: top, middle, baseline -->
				<div class="absolute inset-x-0 top-0 border-t border-dashed border-gray-200 dark:border-gray-700" aria-hidden="true"></div>
				<div class="absolute inset-x-0 top-1/2 border-t border-dashed border-gray-200 dark:border-gray-700" aria-hidden="true"></div>
				<div class="absolute inset-x-0 bottom-0 border-t border-gray-300 dark:border-gray-600" aria-hidden="true"></div>

				<ul class="relative flex items-end h-full gap-[2px]" aria-label="Daily estimated cost">
					{#each days as day (day.key)}
						<li class="group relative flex-1 h-full flex items-end min-w-0" aria-label={dayTitle(day)} title={dayTitle(day)}>
							<!-- Hover target spans the full column height; the bar is anchored to the baseline -->
							<div
								class="w-full rounded-t bg-blue-600 dark:bg-blue-400 group-hover:bg-blue-700 dark:group-hover:bg-blue-300 transition-colors"
								style="height: {barHeight(day.cost)}%"
							></div>
							<div class="pointer-events-none absolute bottom-full left-1/2 -translate-x-1/2 mb-1 whitespace-nowrap rounded bg-gray-900 dark:bg-gray-700 px-2 py-1 text-xs text-white shadow-lg opacity-0 group-hover:opacity-100 transition-opacity z-10" aria-hidden="true">
								<div class="font-semibold">{formatDayLabel(day.key, { weekday: 'short', month: 'short', day: 'numeric' })}</div>
								<div class="tabular-nums">{formatCost(day.cost || 0, currency)} · {formatHours(day.billed_seconds)}</div>
							</div>
						</li>
					{/each}
				</ul>
			</div>

			<!-- X axis labels -->
			<div class="flex gap-[2px] mt-1 text-xs text-gray-500 dark:text-gray-400" aria-hidden="true">
				{#each days as day, i (day.key)}
					<div class="flex-1 min-w-0 text-center overflow-visible whitespace-nowrap">
						{#if i % labelEvery === 0}{formatDayLabel(day.key)}{/if}
					</div>
				{/each}
			</div>
		</div>
	</div>
{/if}
