<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { garmApi } from '$lib/api/client.js';
	import type { InstanceUsage, UsageSummary } from '$lib/api/client.js';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Button from '$lib/components/Button.svelte';
	import Badge from '$lib/components/Badge.svelte';
	import LoadingState from '$lib/components/LoadingState.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import BackendPagination from '$lib/components/BackendPagination.svelte';
	import UsageDailyChart from '$lib/components/usage/UsageDailyChart.svelte';
	import UsageBreakdownTable from '$lib/components/usage/UsageBreakdownTable.svelte';
	import { extractAPIError } from '$lib/utils/apiError';
	import { formatDateTime } from '$lib/utils/format';
	import {
		USAGE_PRESETS,
		browserTimeZone,
		formatCost,
		formatDuration,
		formatHourlyPrice,
		formatHours,
		isRunning,
		matchPreset,
		modelBadge,
		jobConclusionBadge,
		workflowText,
		rangeTotalCost,
		formatBytes,
		egressText,
		modelCost,
		modelCount,
		presetRange,
		rangeFromSearchParams,
		rangeToRFC3339,
		rangeToSearchParams,
		validateRange,
		type DateRange,
		type ProvisioningModelFilter,
		type UsagePreset
	} from '$lib/utils/usage.js';

	const PAGE_SIZE_STORAGE_KEY = 'garm_usage_page_size';

	// Selected range (calendar dates in the browser's time zone, end inclusive)
	let range: DateRange = presetRange('this_month');
	let model: ProvisioningModelFilter = '';
	// Values of the custom date inputs; applied when they form a valid range
	let draftStart = range.start;
	let draftEnd = range.end;
	let rangeError = '';
	const timeZone = browserTimeZone();

	// Summary
	let summary: UsageSummary | null = null;
	let summaryLoading = true;
	let summaryError = '';

	// Instance runs (backend pagination)
	let runs: InstanceUsage[] = [];
	let runsLoading = true;
	let runsError = '';
	let currentPage = 1;
	let pageSize = 25;
	let totalPages = 1;
	let totalRuns = 0;
	let expanded = new Set<string>();

	// Guards against slow responses of an older request overwriting newer data
	let summaryRequest = 0;
	let runsRequest = 0;

	$: activePreset = matchPreset(range);
	$: currency = summary?.currency || 'USD';
	$: totalCost = summary?.total_cost || 0;

	function apiRange() {
		return rangeToRFC3339(range);
	}

	async function loadSummary() {
		const request = ++summaryRequest;
		summaryLoading = true;
		summaryError = '';
		try {
			const { start, end } = apiRange();
			const result = await garmApi.getUsageSummary({
				start,
				end,
				timeZone,
				provisioningModel: model || undefined
			});
			if (request !== summaryRequest) return;
			summary = result;
		} catch (err) {
			if (request !== summaryRequest) return;
			summaryError = extractAPIError(err);
			summary = null;
		} finally {
			if (request === summaryRequest) summaryLoading = false;
		}
	}

	async function loadRuns() {
		const request = ++runsRequest;
		runsLoading = true;
		runsError = '';
		try {
			const { start, end } = apiRange();
			const result = await garmApi.listInstanceUsage({
				start,
				end,
				provisioningModel: model || undefined,
				page: currentPage,
				pageSize
			});
			if (request !== runsRequest) return;
			runs = result.results || [];
			totalPages = result.pages || 1;
			totalRuns = result.total_count || 0;
		} catch (err) {
			if (request !== runsRequest) return;
			runsError = extractAPIError(err);
			runs = [];
		} finally {
			if (request === runsRequest) runsLoading = false;
		}
	}

	function loadAll() {
		loadSummary();
		loadRuns();
	}

	function syncUrl() {
		try {
			const url = new URL(window.location.href);
			url.search = rangeToSearchParams(range, model);
			replaceState(url, {});
		} catch (err) {
			// The URL is a convenience (reload / share); never break the page over it.
			console.debug('Failed to update the URL', err);
		}
	}

	function applyRange(next: DateRange) {
		const invalid = validateRange(next);
		if (invalid) {
			rangeError = invalid;
			return;
		}
		rangeError = '';
		range = next;
		draftStart = next.start;
		draftEnd = next.end;
		currentPage = 1;
		expanded = new Set();
		syncUrl();
		loadAll();
	}

	function selectPreset(preset: UsagePreset) {
		applyRange(presetRange(preset));
	}

	function handleDateChange() {
		applyRange({ start: draftStart, end: draftEnd });
	}

	function handleModelChange() {
		currentPage = 1;
		expanded = new Set();
		syncUrl();
		loadAll();
	}

	function handlePageChange(event: CustomEvent<{ page: number }>) {
		currentPage = event.detail.page;
		loadRuns();
	}

	function handlePageSizeChange(event: CustomEvent<{ pageSize: number }>) {
		pageSize = event.detail.pageSize;
		try {
			localStorage.setItem(PAGE_SIZE_STORAGE_KEY, String(pageSize));
		} catch {
			// ignore storage errors
		}
		currentPage = 1;
		loadRuns();
	}

	function toggle(run: InstanceUsage, index: number) {
		const key = rowKey(run, index);
		const next = new Set(expanded);
		if (next.has(key)) next.delete(key);
		else next.add(key);
		expanded = next;
	}

	function rowKey(run: InstanceUsage, index: number): string {
		return run.id || `${run.instance_name}-${run.created_at}-${index}`;
	}

	function recordCurrency(run: InstanceUsage): string {
		return run.currency || currency;
	}

	function poolText(run: InstanceUsage): string {
		if (run.pool_id) return `Pool ${run.pool_id.slice(0, 8)}`;
		if (run.scale_set_id) return `Scale set ${run.scale_set_id}`;
		return '';
	}

	function jobText(run: InstanceUsage): string {
		return [run.job_repository, run.job_name].filter(Boolean).join(' · ');
	}

	function instanceHref(name: string): string {
		return resolve(`/instances/${encodeURIComponent(name)}`);
	}

	onMount(() => {
		try {
			const saved = parseInt(localStorage.getItem(PAGE_SIZE_STORAGE_KEY) || '', 10);
			if (!isNaN(saved) && saved > 0) pageSize = saved;
		} catch {
			// ignore storage errors
		}

		const fromUrl = rangeFromSearchParams(page.url.searchParams);
		range = fromUrl.range;
		model = fromUrl.model;
		draftStart = range.start;
		draftEnd = range.end;
		loadAll();
	});

	const inputClass =
		'block w-full rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-1.5 text-sm shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 [color-scheme:light] dark:[color-scheme:dark]';
	const thClass = 'px-3 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider whitespace-nowrap';
	const linkClass = 'text-blue-600 dark:text-blue-400 hover:text-blue-500 dark:hover:text-blue-300 hover:underline';
</script>

<svelte:head>
	<title>Cost - GARM</title>
</svelte:head>

<div class="space-y-6">
	<PageHeader title="Cost" description="Estimated cost of runner instances, based on provider list prices">
		<svelte:fragment slot="actions">
			<Button
				variant="secondary"
				on:click={loadAll}
				disabled={summaryLoading || runsLoading}
				icon="<path stroke-linecap='round' stroke-linejoin='round' stroke-width='2' d='M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15' />"
			>
				Refresh
			</Button>
		</svelte:fragment>
	</PageHeader>

	<!-- Range and filter controls -->
	<div class="bg-white dark:bg-gray-800 shadow rounded-lg p-4">
		<div class="flex flex-col lg:flex-row lg:items-end gap-4">
			<div>
				<span class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Range</span>
				<div class="flex flex-wrap gap-2" role="group" aria-label="Date range presets">
					{#each USAGE_PRESETS as preset (preset.id)}
						<button
							type="button"
							on:click={() => selectPreset(preset.id)}
							aria-pressed={activePreset === preset.id}
							class="px-3 py-1.5 text-sm font-medium rounded-md border cursor-pointer transition-colors {activePreset === preset.id
								? 'bg-blue-600 border-blue-600 text-white dark:bg-blue-500 dark:border-blue-500'
								: 'bg-white dark:bg-gray-700 border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-600'}"
						>
							{preset.label}
						</button>
					{/each}
				</div>
			</div>
			<div class="grid grid-cols-2 gap-3 sm:flex sm:items-end">
				<div>
					<label for="usage-start" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">From</label>
					<input id="usage-start" type="date" class={inputClass} bind:value={draftStart} max={draftEnd || undefined} on:change={handleDateChange} />
				</div>
				<div>
					<label for="usage-end" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">To</label>
					<input id="usage-end" type="date" class={inputClass} bind:value={draftEnd} min={draftStart || undefined} on:change={handleDateChange} />
				</div>
			</div>
			<div class="sm:w-44">
				<label for="usage-model" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Provisioning model</label>
				<select id="usage-model" class={inputClass} bind:value={model} on:change={handleModelChange}>
					<option value="">All</option>
					<option value="standard">On-demand</option>
					<option value="spot">Spot</option>
				</select>
			</div>
		</div>
		{#if rangeError}
			<p class="mt-2 text-sm text-red-600 dark:text-red-400" role="alert">{rangeError}</p>
		{/if}
		{#if timeZone}
			<p class="mt-2 text-xs text-gray-500 dark:text-gray-400">Days are in your local time zone ({timeZone}). Costs of runs crossing the range boundaries are prorated.</p>
		{/if}
	</div>

	<!-- Summary -->
	{#if summaryLoading && !summary}
		<div class="bg-white dark:bg-gray-800 shadow rounded-lg">
			<LoadingState message="Loading cost summary..." />
		</div>
	{:else if summaryError}
		<div class="bg-white dark:bg-gray-800 shadow rounded-lg">
			<ErrorState title="Failed to load cost summary" message={summaryError} showRetry={true} onRetry={loadSummary} />
		</div>
	{:else if summary}
		<div class="space-y-6 transition-opacity {summaryLoading ? 'opacity-60' : ''}" aria-busy={summaryLoading}>
			{#if summary.warnings && summary.warnings.length > 0}
				<div class="rounded-md border border-yellow-200 dark:border-yellow-800 bg-yellow-50 dark:bg-yellow-900/20 p-3 text-sm text-yellow-800 dark:text-yellow-200" data-testid="usage-warnings">
					<div class="flex gap-2">
						<svg class="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
						</svg>
						<ul class="space-y-1">
							{#each summary.warnings as warning}
								<li>{warning}</li>
							{/each}
						</ul>
					</div>
				</div>
			{/if}

			<div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-5 gap-4">
				<div class="bg-white dark:bg-gray-800 shadow rounded-lg p-5" data-testid="card-total">
					<p class="text-sm font-medium text-gray-500 dark:text-gray-400">Total cost</p>
					<p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white tabular-nums">{formatCost(summary.total_cost || 0, currency)}</p>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
						{summary.instance_count || 0} {summary.instance_count === 1 ? 'instance' : 'instances'} · {formatHours(summary.billed_seconds)} billed
					</p>
					<p class="text-xs text-gray-500 dark:text-gray-400">
						compute {formatCost(summary.compute_cost || 0, currency)} · egress {formatCost(summary.egress_cost || 0, currency)}
					</p>
				</div>
				<div class="bg-white dark:bg-gray-800 shadow rounded-lg p-5" data-testid="card-standard">
					<p class="text-sm font-medium text-gray-500 dark:text-gray-400">On-demand</p>
					<p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white tabular-nums">{formatCost(modelCost(summary, 'standard'), currency)}</p>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{modelCount(summary, 'standard')} {modelCount(summary, 'standard') === 1 ? 'instance' : 'instances'}</p>
				</div>
				<div class="bg-white dark:bg-gray-800 shadow rounded-lg p-5" data-testid="card-spot">
					<p class="text-sm font-medium text-gray-500 dark:text-gray-400">Spot</p>
					<p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white tabular-nums">{formatCost(modelCost(summary, 'spot'), currency)}</p>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{modelCount(summary, 'spot')} {modelCount(summary, 'spot') === 1 ? 'instance' : 'instances'}</p>
				</div>
				<div class="bg-white dark:bg-gray-800 shadow rounded-lg p-5" data-testid="card-egress">
					<p class="text-sm font-medium text-gray-500 dark:text-gray-400" title="Internet egress estimated from sampled VPC flow metrics; may be lower than billed">Egress (estimate)</p>
					<p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white tabular-nums">{formatCost(summary.egress_cost || 0, currency)}</p>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{formatBytes(summary.external_bytes)} to the internet</p>
				</div>
				<div class="bg-white dark:bg-gray-800 shadow rounded-lg p-5" data-testid="card-running">
					<p class="text-sm font-medium text-gray-500 dark:text-gray-400">Currently running</p>
					<p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white tabular-nums">{formatCost(summary.running_cost || 0, currency)}</p>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{summary.running_count || 0} running {summary.running_count === 1 ? 'instance' : 'instances'}, cost in range</p>
				</div>
			</div>

			<!-- Daily chart -->
			<div class="bg-white dark:bg-gray-800 shadow rounded-lg p-4 sm:p-6">
				<div class="flex items-baseline justify-between mb-4">
					<h2 class="text-lg font-medium text-gray-900 dark:text-white">Daily cost</h2>
					{#if summary.time_zone}
						<span class="text-xs text-gray-500 dark:text-gray-400">{summary.time_zone}</span>
					{/if}
				</div>
				<UsageDailyChart days={summary.by_day || []} {currency} />
			</div>

			<!-- Breakdowns -->
			<div class="grid grid-cols-1 xl:grid-cols-2 2xl:grid-cols-3 gap-6">
				<UsageBreakdownTable title="By pool" groups={summary.by_pool || []} total={totalCost} {currency} />
				<UsageBreakdownTable title="By flavor" groups={summary.by_flavor || []} total={totalCost} {currency} />
				<UsageBreakdownTable title="By entity" groups={summary.by_entity || []} total={totalCost} {currency} />
			</div>
		</div>
	{/if}

	<!-- Instance runs -->
	<div class="space-y-3">
		<h2 class="text-lg font-medium text-gray-900 dark:text-white">Instance runs</h2>
		<div class="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
			{#if runsLoading && runs.length === 0}
				<LoadingState message="Loading instance runs..." />
			{:else if runsError}
				<ErrorState title="Failed to load instance runs" message={runsError} showRetry={true} onRetry={loadRuns} />
			{:else if runs.length === 0}
				<EmptyState title="No instance runs" message="No instances ran in this range." iconType="cog" />
			{:else}
				<!-- Mobile: cards -->
				<ul class="sm:hidden divide-y divide-gray-200 dark:divide-gray-700">
					{#each runs as run, index (rowKey(run, index))}
						{@const badge = modelBadge(run)}
						{@const open = expanded.has(rowKey(run, index))}
						<li class="px-4 py-4">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0">
									{#if isRunning(run) && run.instance_name}
										<a href={instanceHref(run.instance_name)} class="block truncate text-sm font-medium font-mono {linkClass}">{run.instance_name}</a>
									{:else}
										<span class="block truncate text-sm font-medium font-mono text-gray-900 dark:text-white">{run.instance_name}</span>
									{/if}
									<p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">{run.flavor || '—'}{run.entity_name ? ` · ${run.entity_name}` : ''}</p>
									{#if jobText(run)}
										<p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400" title={jobText(run)}>{[workflowText(run), jobText(run)].filter(Boolean).join(' · ')}</p>
									{/if}
								</div>
								<div class="text-right shrink-0">
									<p class="text-sm font-semibold text-gray-900 dark:text-white tabular-nums">{formatCost(rangeTotalCost(run), recordCurrency(run))}</p>
									<p class="text-xs text-gray-500 dark:text-gray-400 tabular-nums">{formatDuration(run.range_billed_seconds)}</p>
								</div>
							</div>
							<div class="mt-2 flex flex-wrap items-center gap-2">
								<span title={badge.title}><Badge variant={badge.variant} text={badge.text} /></span>
								{#if isRunning(run)}
									<Badge variant="success" text="Running" />
								{/if}
								<span class="text-xs text-gray-500 dark:text-gray-400">{formatDateTime(run.started_at || run.created_at)}</span>
								<button type="button" class="ml-auto text-xs font-medium cursor-pointer {linkClass}" aria-expanded={open} on:click={() => toggle(run, index)}>
									{open ? 'Hide details' : 'Details'}
								</button>
							</div>
							{#if open}
								<div class="mt-3">
									{@render details(run)}
								</div>
							{/if}
						</li>
					{/each}
				</ul>

				<!-- Desktop: table -->
				<div class="hidden sm:block overflow-x-auto">
					<table class="min-w-full text-sm">
						<thead class="bg-gray-50 dark:bg-gray-700">
							<tr>
								<th scope="col" class="w-8 px-2 py-3"><span class="sr-only">Details</span></th>
								<th scope="col" class={thClass}>Instance</th>
								<th scope="col" class={thClass}>Pool / Flavor</th>
								<th scope="col" class={thClass}>Model</th>
								<th scope="col" class={thClass}>Time</th>
								<th scope="col" class="{thClass} text-right">Duration</th>
								<th scope="col" class="{thClass} text-right">Cost</th>
								<th scope="col" class="{thClass} text-right">Hourly</th>
								<th scope="col" class={thClass}>Job</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
							{#each runs as run, index (rowKey(run, index))}
								{@const badge = modelBadge(run)}
								{@const open = expanded.has(rowKey(run, index))}
								<tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50" data-testid="usage-row">
									<td class="px-2 py-3 align-top">
										<button
											type="button"
											class="p-1 rounded text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white cursor-pointer"
											aria-expanded={open}
											aria-label="{open ? 'Hide' : 'Show'} price details of {run.instance_name}"
											on:click={() => toggle(run, index)}
										>
											<svg class="h-4 w-4 transition-transform {open ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
											</svg>
										</button>
									</td>
									<td class="px-4 py-3 align-top max-w-[16rem]">
										{#if isRunning(run) && run.instance_name}
											<a href={instanceHref(run.instance_name)} class="block truncate font-mono {linkClass}" title={run.instance_name}>{run.instance_name}</a>
										{:else}
											<span class="block truncate font-mono text-gray-900 dark:text-white" title={run.instance_name}>{run.instance_name}</span>
										{/if}
										{#if run.entity_name}
											<span class="block truncate text-xs text-gray-500 dark:text-gray-400">{run.entity_name}</span>
										{/if}
									</td>
									<td class="px-4 py-3 align-top max-w-[14rem]">
										<span class="block truncate text-gray-900 dark:text-white" title={run.flavor}>{run.flavor || '—'}</span>
										<span class="block truncate text-xs text-gray-500 dark:text-gray-400" title={run.pool_id || (run.scale_set_id ? String(run.scale_set_id) : '')}>{poolText(run)}</span>
									</td>
									<td class="px-4 py-3 align-top whitespace-nowrap">
										<span title={badge.title}><Badge variant={badge.variant} text={badge.text} /></span>
									</td>
									<td class="px-3 py-3 align-top whitespace-nowrap text-gray-700 dark:text-gray-300">
										<span class="block">{run.started_at ? formatDateTime(run.started_at) : '—'}</span>
										<span class="block text-xs text-gray-500 dark:text-gray-400">
											{#if isRunning(run)}
												<Badge variant="success" text="Running" />
											{:else}
												→ {formatDateTime(run.ended_at)}
											{/if}
										</span>
									</td>
									<td class="px-3 py-3 align-top whitespace-nowrap text-right tabular-nums text-gray-700 dark:text-gray-300">{formatDuration(run.range_billed_seconds)}</td>
									<td class="px-3 py-3 align-top whitespace-nowrap text-right tabular-nums" title="compute {formatCost(run.range_cost ?? 0, recordCurrency(run))} + egress {formatCost(run.range_egress_cost ?? 0, recordCurrency(run))}">
										<span class="block font-medium text-gray-900 dark:text-white">{formatCost(rangeTotalCost(run), recordCurrency(run))}</span>
										{#if egressText(run)}
											<span class="block text-xs text-gray-500 dark:text-gray-400">{egressText(run)}</span>
										{/if}
									</td>
									<td class="px-3 py-3 align-top whitespace-nowrap text-right tabular-nums text-gray-700 dark:text-gray-300">
										{run.hourly_price !== undefined && run.hourly_price !== null ? `${formatHourlyPrice(run.hourly_price, recordCurrency(run))}/h` : '—'}
									</td>
									<td class="px-3 py-3 align-top max-w-[14rem]">
										{#if jobText(run)}
											{@const conclusion = jobConclusionBadge(run.job_conclusion)}
											{#if run.job_url || run.workflow_run_url}
												<a href={run.job_url || run.workflow_run_url} target="_blank" rel="noopener noreferrer" class="block truncate {linkClass}" title={jobText(run)}>{jobText(run)}</a>
											{:else}
												<span class="block truncate text-gray-700 dark:text-gray-300" title={jobText(run)}>{jobText(run)}</span>
											{/if}
											<span class="flex items-center gap-1 min-w-0 text-xs text-gray-500 dark:text-gray-400">
												{#if conclusion}<Badge variant={conclusion.variant} text={conclusion.text} />{/if}
												{#if workflowText(run)}<span class="truncate" title={workflowText(run)}>{workflowText(run)}</span>{/if}
											</span>
										{:else}
											<span class="text-gray-400 dark:text-gray-500">—</span>
										{/if}
									</td>
								</tr>
								{#if open}
									<tr class="bg-gray-50 dark:bg-gray-900/40">
										<td></td>
										<td colspan="8" class="px-4 py-4">
											{@render details(run)}
										</td>
									</tr>
								{/if}
							{/each}
						</tbody>
					</table>
				</div>

				<BackendPagination
					{currentPage}
					{totalPages}
					totalItems={totalRuns}
					{pageSize}
					loading={runsLoading}
					itemName="runs"
					on:pageChange={handlePageChange}
					on:pageSizeChange={handlePageSizeChange}
				/>
			{/if}
		</div>
	</div>
</div>

{#snippet details(run: InstanceUsage)}
	{@const cur = recordCurrency(run)}
	<div class="space-y-3 text-sm">
		<dl class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-2">
			<div>
				<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Whole run cost</dt>
				<dd class="text-gray-900 dark:text-white tabular-nums">{formatCost(run.cost ?? 0, cur)} ({formatDuration(run.billed_seconds)})</dd>
			</div>
			<div>
				<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Provider / region</dt>
				<dd class="text-gray-900 dark:text-white">{run.provider_name || '—'}{run.price?.region ? ` / ${run.price.region}` : ''}</dd>
			</div>
			<div>
				<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Image / arch</dt>
				<dd class="text-gray-900 dark:text-white break-all">{run.image || '—'}{run.os_arch ? ` (${run.os_arch})` : ''}</dd>
			</div>
			<div>
				<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Created / end reason</dt>
				<dd class="text-gray-900 dark:text-white">{formatDateTime(run.created_at)}{run.end_reason ? ` · ${run.end_reason}` : ''}</dd>
			</div>
		</dl>
		{#if run.job_name || run.workflow_run_url}
			<dl class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-2" data-testid="usage-job-details">
				<div>
					<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Workflow</dt>
					<dd class="text-gray-900 dark:text-white">
						{run.workflow_name || '—'}{run.head_branch ? ` · ${run.head_branch}` : ''}{run.run_attempt && run.run_attempt > 1 ? ` · attempt ${run.run_attempt}` : ''}
					</dd>
				</div>
				<div>
					<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Job</dt>
					<dd class="text-gray-900 dark:text-white">
						{#if run.job_url}
							<a href={run.job_url} target="_blank" rel="noopener noreferrer" class="break-all {linkClass}">{run.job_url}</a>
						{:else}
							{jobText(run) || '—'}
						{/if}
					</dd>
				</div>
				<div>
					<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Workflow run</dt>
					<dd class="text-gray-900 dark:text-white">
						{#if run.workflow_run_url}
							<a href={run.workflow_run_url} target="_blank" rel="noopener noreferrer" class="break-all {linkClass}">{run.workflow_run_url}</a>
						{:else}—{/if}
					</dd>
				</div>
				<div>
					<dt class="text-xs font-medium text-gray-500 dark:text-gray-400">Job result / time</dt>
					<dd class="text-gray-900 dark:text-white">
						{run.job_conclusion || (run.job_completed_at ? '—' : 'in progress')}
						{#if run.job_started_at} · {formatDateTime(run.job_started_at)}{#if run.job_completed_at}{' → '}{formatDateTime(run.job_completed_at)}{/if}{/if}
					</dd>
				</div>
			</dl>
		{/if}

		{#if run.price_error}
			<p class="text-red-700 dark:text-red-300">Price error: {run.price_error}</p>
		{/if}

		{#if run.price?.components && run.price.components.length > 0}
			<div class="overflow-x-auto">
				<table class="min-w-full text-xs">
					<thead>
						<tr class="text-left text-gray-500 dark:text-gray-400 uppercase tracking-wider">
							<th scope="col" class="py-1 pr-4 font-medium">Component</th>
							<th scope="col" class="py-1 pr-4 font-medium">Description</th>
							<th scope="col" class="py-1 pr-4 font-medium text-right">Quantity</th>
							<th scope="col" class="py-1 pr-4 font-medium">SKU</th>
							<th scope="col" class="py-1 font-medium text-right">Hourly</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each run.price.components as component}
							<tr>
								<td class="py-1 pr-4 text-gray-900 dark:text-white whitespace-nowrap">{component.name || '—'}</td>
								<td class="py-1 pr-4 max-w-[18rem] truncate text-gray-700 dark:text-gray-300" title={component.description}>{component.description || '—'}</td>
								<td class="py-1 pr-4 text-right tabular-nums text-gray-700 dark:text-gray-300 whitespace-nowrap">{component.quantity ?? '—'} {component.unit || ''}</td>
								<td class="py-1 pr-4 font-mono text-gray-500 dark:text-gray-400 whitespace-nowrap">{component.sku || '—'}</td>
								<td class="py-1 text-right tabular-nums text-gray-900 dark:text-white whitespace-nowrap">{formatHourlyPrice(component.hourly_price, run.price.currency || cur)}</td>
							</tr>
						{/each}
					</tbody>
					{#if run.price.hourly_price !== undefined}
						<tfoot>
							<tr class="border-t border-gray-300 dark:border-gray-600">
								<td colspan="4" class="py-1 pr-4 font-medium text-gray-900 dark:text-white">Total per hour</td>
								<td class="py-1 text-right tabular-nums font-medium text-gray-900 dark:text-white">{formatHourlyPrice(run.price.hourly_price, run.price.currency || cur)}</td>
							</tr>
						</tfoot>
					{/if}
				</table>
			</div>
		{:else}
			<p class="text-gray-500 dark:text-gray-400">No price breakdown available.</p>
		{/if}

		{#if run.price?.warnings && run.price.warnings.length > 0}
			<ul class="list-disc pl-5 text-yellow-800 dark:text-yellow-200">
				{#each run.price.warnings as warning}
					<li>{warning}</li>
				{/each}
			</ul>
		{/if}

		{@render egressDetails(run)}
	</div>
{/snippet}

{#snippet egressDetails(run: InstanceUsage)}
	{@const cur = run.egress_currency || recordCurrency(run)}
	<div class="space-y-2" data-testid="usage-egress-details">
		<h3 class="text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">Egress</h3>
		{#if run.egress_status === 'ok' && run.egress}
			<p class="text-gray-900 dark:text-white">
				{formatCost(run.egress_cost ?? 0, cur)} for {formatBytes(run.egress_external_bytes)} to the internet
				· {formatBytes(run.egress_bytes)} in total{run.egress_sent_bytes ? ` (interface counter: ${formatBytes(run.egress_sent_bytes)})` : ''}
				{#if !run.egress_final}<span class="text-gray-500 dark:text-gray-400">· so far, refreshed while the instance runs</span>{/if}
			</p>
			<p class="text-xs text-gray-500 dark:text-gray-400">
				Estimated from sampled VPC flow metrics, without packet headers: may be lower than billed. Only internet traffic is priced.
			</p>
			<div class="overflow-x-auto">
				<table class="min-w-full text-xs">
					<thead>
						<tr class="text-left text-gray-500 dark:text-gray-400 uppercase tracking-wider">
							<th scope="col" class="py-1 pr-4 font-medium">Destination</th>
							<th scope="col" class="py-1 pr-4 font-medium">Location</th>
							<th scope="col" class="py-1 font-medium text-right">Sent</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each run.egress.flows || [] as flow}
							<tr>
								<td class="py-1 pr-4 text-gray-900 dark:text-white whitespace-nowrap">{flow.location_type}</td>
								<td class="py-1 pr-4 text-gray-700 dark:text-gray-300">{[flow.country, flow.continent].filter(Boolean).join(', ') || '—'}</td>
								<td class="py-1 text-right tabular-nums text-gray-900 dark:text-white whitespace-nowrap">{formatBytes(flow.bytes)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			{#if run.egress.charges && run.egress.charges.length > 0}
				<div class="overflow-x-auto">
					<table class="min-w-full text-xs">
						<thead>
							<tr class="text-left text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								<th scope="col" class="py-1 pr-4 font-medium">Billed as</th>
								<th scope="col" class="py-1 pr-4 font-medium">SKU</th>
								<th scope="col" class="py-1 pr-4 font-medium text-right">Sent</th>
								<th scope="col" class="py-1 pr-4 font-medium text-right">Per GiB</th>
								<th scope="col" class="py-1 font-medium text-right">Cost</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
							{#each run.egress.charges as charge}
								<tr>
									<td class="py-1 pr-4 max-w-[18rem] truncate text-gray-900 dark:text-white" title={charge.description}>{charge.destination}</td>
									<td class="py-1 pr-4 font-mono text-gray-500 dark:text-gray-400 whitespace-nowrap">{charge.sku || '—'}</td>
									<td class="py-1 pr-4 text-right tabular-nums text-gray-700 dark:text-gray-300 whitespace-nowrap">{formatBytes(charge.bytes)}</td>
									<td class="py-1 pr-4 text-right tabular-nums text-gray-700 dark:text-gray-300 whitespace-nowrap">{formatHourlyPrice(charge.unit_price, cur)}</td>
									<td class="py-1 text-right tabular-nums text-gray-900 dark:text-white whitespace-nowrap">{formatCost(charge.cost, cur)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
			{#if run.egress.warnings && run.egress.warnings.length > 0}
				<ul class="list-disc pl-5 text-yellow-800 dark:text-yellow-200">
					{#each run.egress.warnings as warning}
						<li>{warning}</li>
					{/each}
				</ul>
			{/if}
		{:else if run.egress_status === 'error'}
			<p class="text-red-700 dark:text-red-300">Egress measurement failed (retried): {run.egress_error}</p>
		{:else if run.egress_status === 'unsupported'}
			<p class="text-gray-500 dark:text-gray-400">The provider can't measure egress.</p>
		{:else if run.started_at}
			<p class="text-gray-500 dark:text-gray-400">Not measured yet. Deleted instances are measured a few minutes after they end.</p>
		{:else}
			<p class="text-gray-500 dark:text-gray-400">The instance never ran.</p>
		{/if}
	</div>
{/snippet}
