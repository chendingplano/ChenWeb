<script lang="ts">
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { Chart } from 'svelte-echarts';
	import type { EChartsOption } from 'echarts';
	import { BarChart } from 'echarts/charts';
	import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
	import { init, use } from 'echarts/core';
	import { CanvasRenderer } from 'echarts/renderers';

	import {
		getLLMTodaySummary,
		listLLMHourlyBalanceReports,
		listLLMCurrentBalances,
		listLLMModelActivityReports,
		listLLMUsageEvents,
		runLLMReconciliationNow,
		type LLMCurrentBalance,
		type LLMHourlyBalanceReport,
		type LLMModelActivityReport,
		type LLMReportFilters,
		type LLMTodaySummary,
		type LLMUsageEvent
	} from './llm-activities-client';

	use([BarChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let modelReports = $state<LLMModelActivityReport[]>([]);
	let balances = $state<LLMCurrentBalance[]>([]);
	let hourlyBalanceReports = $state<LLMHourlyBalanceReport[]>([]);
	let usageEvents = $state<LLMUsageEvent[]>([]);
	let todaySummary = $state<LLMTodaySummary>({
		workspace_day: '',
		timezone_name: '',
		spend_amount: 0,
		currency_code: 'USD',
		request_count: 0,
		total_tokens: 0,
		error_count: 0
	});
	let loading = $state(false);
	let reconciling = $state(false);
	let error = $state<string | null>(null);
	let notice = $state<string | null>(null);
	let reportLimit = $state(30);
	let eventLimit = $state(50);
	let reportLoading = $state(false);
	let reportFilterError = $state<string | null>(null);
	let timePreset = $state('last_30_days');
	let customFrom = $state('');
	let customTo = $state('');
	let selectedAPIKey = $state('');
	let apiKeyOptions = $state<{ name: string }[]>([]);
	let balanceFrequency = $state('hourly');

	const timeOptions = [
		{ value: 'today', label: 'Today' },
		{ value: 'yesterday', label: 'Yesterday' },
		{ value: 'last_7_days', label: 'Last 7 Days' },
		{ value: 'last_30_days', label: 'Last 30 Days' },
		{ value: 'this_month', label: 'This Month' },
		{ value: 'last_month', label: 'Last Month' },
		{ value: 'custom', label: 'Custom' }
	];

	onMount(() => {
		load();
	});

	async function load(options?: { preserveNotice?: boolean }) {
		loading = true;
		error = null;
		if (!options?.preserveNotice) {
			notice = null;
		}
		try {
			const [summaryResponse, balancesResponse, hourlyResponse, reportsResponse, eventsResponse] =
				await Promise.all([
					getLLMTodaySummary(),
					listLLMCurrentBalances(),
					listLLMHourlyBalanceReports(24, balanceFrequency),
					listLLMModelActivityReports(reportLimit, getReportFilters()),
					listLLMUsageEvents(eventLimit)
				]);
			todaySummary = summaryResponse.summary;
			balances = balancesResponse.balances;
			hourlyBalanceReports = hourlyResponse.reports;
			modelReports = reportsResponse.reports;
			apiKeyOptions = reportsResponse.api_keys || [];
			usageEvents = eventsResponse.usage_events;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	function dateOnly(date: Date): string {
		const year = date.getFullYear();
		const month = String(date.getMonth() + 1).padStart(2, '0');
		const day = String(date.getDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
	}

	function getReportFilters(): LLMReportFilters {
		const today = new Date();
		const todayDate = new Date(today.getFullYear(), today.getMonth(), today.getDate());
		let from = dateOnly(todayDate);
		let to = from;

		switch (timePreset) {
			case 'yesterday':
				from = to = dateOnly(new Date(todayDate.getTime() - 24 * 60 * 60 * 1000));
				break;
			case 'last_7_days':
				from = dateOnly(new Date(todayDate.getTime() - 6 * 24 * 60 * 60 * 1000));
				break;
			case 'last_30_days':
				from = dateOnly(new Date(todayDate.getTime() - 29 * 24 * 60 * 60 * 1000));
				break;
			case 'this_month':
				from = dateOnly(new Date(todayDate.getFullYear(), todayDate.getMonth(), 1));
				break;
			case 'last_month':
				from = dateOnly(new Date(todayDate.getFullYear(), todayDate.getMonth() - 1, 1));
				to = dateOnly(new Date(todayDate.getFullYear(), todayDate.getMonth(), 0));
				break;
			case 'custom':
				if (!customFrom || !customTo) {
					throw new Error('Choose both a custom start date and end date.');
				}
				if (customFrom > customTo) {
					throw new Error('Custom start date must not be after the end date.');
				}
				from = customFrom;
				to = customTo;
				break;
		}

		return { from, to, apiKey: selectedAPIKey || undefined };
	}

	async function loadModelReports() {
		reportFilterError = null;
		reportLoading = true;
		try {
			const response = await listLLMModelActivityReports(reportLimit, getReportFilters());
			modelReports = response.reports;
			apiKeyOptions = response.api_keys || apiKeyOptions;
		} catch (err) {
			reportFilterError = String((err as Error).message ?? err);
		} finally {
			reportLoading = false;
		}
	}

	function handleReportFilterChange() {
		void loadModelReports();
	}

	async function loadBalanceReports() {
		const response = await listLLMHourlyBalanceReports(24, balanceFrequency);
		hourlyBalanceReports = response.reports;
	}

	async function runReconciliation() {
		reconciling = true;
		error = null;
		notice = null;
		try {
			const response = await runLLMReconciliationNow();
			notice = response.message ?? 'Reconciliation finished. Daily reports have been refreshed.';
			await load({ preserveNotice: true });
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			reconciling = false;
		}
	}

	function fmtMoney(value: number, currency: string): string {
		return new Intl.NumberFormat(undefined, {
			style: 'currency',
			currency: currency || 'USD'
		}).format(value);
	}

	function fmtNum(value: number): string {
		return new Intl.NumberFormat().format(value);
	}

	function fmtDate(raw: string): string {
		return new Date(raw).toLocaleString();
	}

	function fmtWorkspaceDay(raw: string): string {
		const trimmed = raw?.trim() ?? '';
		if (!trimmed) {
			return '';
		}
		const day = trimmed.includes('T')
			? trimmed.slice(0, trimmed.indexOf('T'))
			: trimmed.slice(0, 10);
		return day || trimmed;
	}

	function isEmbeddingModel(modelName: string): boolean {
		const normalized = (modelName || '').trim().toLowerCase();
		return normalized.includes('embedding') || normalized.includes('embed');
	}

	function tokenSummary(event: LLMUsageEvent): string {
		if (isEmbeddingModel(event.model_name)) {
			return `${fmtNum(event.input_tokens)} input / ${fmtNum(event.output_tokens)} completion`;
		}
		return `${fmtNum(event.input_tokens)} in / ${fmtNum(event.output_tokens)} out`;
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#A16207' : '#B45309');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const spendBar = $derived(darkMode ? '#F59E0B' : '#D97706');
	const inputBar = $derived(darkMode ? '#3B82F6' : '#2563EB');
	const outputBar = $derived(darkMode ? '#22C55E' : '#16A34A');

	type ModelChartGroup = {
		key: string;
		provider: string;
		modelName: string;
		currencyCode: string;
		rows: LLMModelActivityReport[];
	};

	const modelChartGroups = $derived(
		(() => {
			const grouped = new SvelteMap<string, ModelChartGroup>();
			for (const row of modelReports) {
				const key = `${row.provider}:${row.model_name}:${row.api_key_name}`;
				const existing = grouped.get(key);
				if (existing) {
					existing.rows.push(row);
					if (!existing.currencyCode && row.currency_code) {
						existing.currencyCode = row.currency_code;
					}
					continue;
				}
				grouped.set(key, {
					key,
					provider: row.provider,
					modelName: row.model_name,
					currencyCode: row.currency_code || 'USD',
					rows: [row]
				});
			}

			return Array.from(grouped.values())
				.map((group) => ({
					...group,
					rows: [...group.rows].sort((a, b) => a.workspace_day.localeCompare(b.workspace_day))
				}))
				.sort(
					(a, b) => a.modelName.localeCompare(b.modelName) || a.provider.localeCompare(b.provider)
				);
		})()
	);

	function buildModelChartOptions(group: ModelChartGroup): EChartsOption {
		const days = group.rows.map((row) => fmtWorkspaceDay(row.workspace_day));
		return {
			backgroundColor: 'transparent',
			animationDuration: 250,
			color: [inputBar, '#8B5CF6', outputBar, spendBar],
			legend: {
				top: 0,
				textStyle: {
					color: sub
				},
				itemWidth: 14,
				itemHeight: 10
			},
			tooltip: {
				trigger: 'axis',
				axisPointer: {
					type: 'shadow'
				},
				backgroundColor: darkMode ? '#0F1320' : '#FFFFFF',
				borderColor: border,
				textStyle: {
					color: heading
				}
			},
			grid: {
				top: 44,
				right: 96,
				bottom: 56,
				left: 64
			},
			xAxis: {
				type: 'category',
				data: days,
				axisTick: { alignWithLabel: true },
				axisLine: { lineStyle: { color: border } },
				axisLabel: {
					color: sub,
					rotate: days.length > 8 ? 35 : 0
				}
			},
			yAxis: [
				{
					type: 'value',
					name: 'Tokens',
					nameTextStyle: { color: sub },
					axisLabel: {
						color: sub,
						formatter: (value: number) => fmtNum(value)
					},
					splitLine: { lineStyle: { color: border, opacity: 0.45 } }
				},
				{
					type: 'value',
					name: `Local estimate (${group.currencyCode || 'CNY'})`,
					position: 'right',
					offset: 64,
					nameTextStyle: { color: sub },
					axisLabel: {
						color: sub,
						formatter: (value: number) => Number(value).toFixed(2)
					},
					splitLine: { show: false }
				}
			],
			series: [
				{
					name: 'Input (Cache Hit)',
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 18,
					data: group.rows.map((row) => row.prompt_cache_hit_tokens)
				},
				{
					name: 'Input (Cache Miss)',
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 18,
					data: group.rows.map((row) => row.prompt_cache_miss_tokens)
				},
				{
					name: 'Output',
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 18,
					data: group.rows.map((row) => row.output_tokens)
				},
				{
					name: 'Local estimate',
					type: 'bar',
					yAxisIndex: 1,
					barMaxWidth: 18,
					data: group.rows.map((row) => row.spend_amount)
				}
			]
		};
	}

	type BalanceChartGroup = { key: string; accountName: string; rows: LLMHourlyBalanceReport[] };
	const balanceChartGroups = $derived((() => {
		const grouped = new SvelteMap<string, BalanceChartGroup>();
		for (const row of hourlyBalanceReports) {
			const key = row.account_id;
			const group = grouped.get(key) ?? { key, accountName: row.account_name, rows: [] };
			group.rows.push(row); grouped.set(key, group);
		}
		return Array.from(grouped.values()).map((group) => ({ ...group, rows: [...group.rows].sort((a, b) => a.hour_started_at.localeCompare(b.hour_started_at)) }));
	})());

	function buildBalanceChartOptions(group: BalanceChartGroup): EChartsOption {
		return { backgroundColor: 'transparent', animationDuration: 250, color: ['#38BDF8', '#14B8A6', spendBar],
			legend: { top: 0, textStyle: { color: sub } },
			tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, backgroundColor: darkMode ? '#0F1320' : '#FFFFFF', borderColor: border, textStyle: { color: heading } },
			grid: { top: 28, right: 30, bottom: 58, left: 70 },
			xAxis: { type: 'category', data: group.rows.map((row) => fmtDate(row.hour_started_at)), axisLine: { lineStyle: { color: border } }, axisLabel: { color: sub, rotate: 35 } },
			yAxis: [{ type: 'value', name: 'USD', nameTextStyle: { color: sub }, axisLabel: { color: sub }, splitLine: { lineStyle: { color: border, opacity: 0.45 } } }, { type: 'value', name: 'CNY', nameTextStyle: { color: sub }, axisLabel: { color: sub }, splitLine: { show: false } }],
			series: [
				{ name: 'Current balance (USD)', type: 'bar', yAxisIndex: 0, barMaxWidth: 20, data: group.rows.map((row) => row.balance_usd) },
				{ name: 'Current balance (CNY)', type: 'bar', yAxisIndex: 1, barMaxWidth: 20, data: group.rows.map((row) => row.balance_cny) },
				{ name: 'Spending (CNY)', type: 'bar', yAxisIndex: 1, barMaxWidth: 20, data: group.rows.map((row) => row.spending_cny) }
			]
		};
	}

	function balanceChartWidth(group: BalanceChartGroup): string {
		return `${Math.max(900, group.rows.length * 72)}px`;
	}
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--input-bg={inputBg}
>
	<header class="toolbar">
		<div>
			<h2>LLM Activities</h2>
			<p class="muted">
				Provider-side daily spend reconciliation plus per-call usage telemetry for debugging and
				optimization.
			</p>
			<p class="muted">
				`Refresh` reloads stored activity data. `Run Reconciliation` fetches fresh provider balances
				and updates spend rows.
			</p>
		</div>
		<div class="toolbar-actions">
			<label>
				<span>Reports</span>
				<input type="number" min="1" bind:value={reportLimit} />
			</label>
			<label>
				<span>Events</span>
				<input type="number" min="1" bind:value={eventLimit} />
			</label>
			<button class="primary" onclick={() => load()} disabled={loading}>
				{loading ? 'Refreshing…' : 'Refresh'}
			</button>
			<button class="secondary" onclick={runReconciliation} disabled={loading || reconciling}>
				{reconciling ? 'Reconciling…' : 'Run Reconciliation'}
			</button>
		</div>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Today's provider delta</div>
			<div class="summary-value">
				{fmtMoney(todaySummary.spend_amount, todaySummary.currency_code || 'USD')}
			</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Today's Requests</div>
			<div class="summary-value">{fmtNum(todaySummary.request_count)}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Today's Tokens</div>
			<div class="summary-value">{fmtNum(todaySummary.total_tokens)}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Today's Errors</div>
			<div class="summary-value">{fmtNum(todaySummary.error_count)}</div>
		</div>
	</div>

	{#if error}
		<div class="error" role="alert">{error}</div>
	{/if}

	{#if notice}
		<div class="notice" role="status">{notice}</div>
	{/if}

	<div class="panel">
		<div class="panel-head">
			<div>
				<h3>Current Balances</h3>
				<p class="muted">
					Latest stored provider-side balance snapshot per account. Use `Run Reconciliation` to
					fetch a fresh balance.
				</p>
			</div>
			<label class="balance-frequency"><span>Frequency</span><select bind:value={balanceFrequency} onchange={() => void loadBalanceReports()}><option value="hourly">Hourly</option><option value="daily">Daily</option><option value="monthly">Monthly</option></select></label>
		</div>
		{#if loading && balances.length === 0}
			<div class="empty">Loading current balances…</div>
		{:else if balances.length === 0}
			<div class="empty">
				No balance snapshots yet. Run reconciliation to capture the latest provider balance.
			</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Account</th>
							<th>Provider</th>
							<th>Balance</th>
							<th>Captured</th>
							<th>Workspace Day</th>
						</tr>
					</thead>
					<tbody>
						{#each balances as balance (`${balance.account_id}:${balance.currency_code}`)}
							<tr>
								<td>{balance.account_name}</td>
								<td>{balance.provider}</td>
								<td>{fmtMoney(balance.balance_amount, balance.currency_code)}</td>
								<td>{fmtDate(balance.captured_at)}</td>
								<td>{fmtWorkspaceDay(balance.workspace_day)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<div class="panel">
		<div class="panel-head">
			<div>
				<h3>Official Account Balance and Hourly Spending</h3>
				<p class="muted">Provider-reported DeepSeek balance by API key. Spending is the CNY balance decrease from the preceding hourly snapshot; balance increases are shown as zero spending.</p>
			</div>
		</div>
		{#if loading && hourlyBalanceReports.length === 0}
			<div class="empty">Loading official balance history…</div>
		{:else if balanceChartGroups.length === 0}
			<div class="empty">No official balance history yet. Hourly snapshots will appear here after the next capture.</div>
		{:else}
			<div class="model-chart-grid">
				{#each balanceChartGroups as group (group.key)}
					<div class="model-chart-card">
						<div class="model-chart-head"><div><div class="cell-primary">{group.accountName}</div><div class="cell-secondary">Official provider balance and hourly spending</div></div></div>
						<div class="balance-chart-scroll"><div class="model-chart" style:width={balanceChartWidth(group)}><Chart {init} options={buildBalanceChartOptions(group)} style="width: 100%; height: 100%;" /></div></div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<div class="panel">
		<div class="panel-head">
			<div>
				<h3>Spend Reports</h3>
				<p class="muted">
					Per-model activity aggregated by workspace day. Local estimates use the configured DeepSeek CNY token prices and remain separate from provider balances.
				</p>
			</div>
			<div class="report-filters">
				<label>
					<span>Time</span>
					<select bind:value={timePreset} onchange={handleReportFilterChange}>
						{#each timeOptions as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
				</label>
				<label>
					<span>API Key</span>
					<select bind:value={selectedAPIKey} onchange={handleReportFilterChange}>
						<option value="">All API Keys</option>
						{#each apiKeyOptions as option (option.name)}
							<option value={option.name}>{option.name}</option>
						{/each}
					</select>
				</label>
				{#if timePreset === 'custom'}
					<label>
						<span>Start date</span>
						<input type="date" bind:value={customFrom} onchange={handleReportFilterChange} />
					</label>
					<label>
						<span>End date</span>
						<input type="date" bind:value={customTo} onchange={handleReportFilterChange} />
					</label>
				{/if}
			</div>
		</div>
		{#if reportFilterError}
			<div class="filter-error" role="alert">{reportFilterError}</div>
		{/if}
		{#if (loading || reportLoading) && modelReports.length === 0}
			<div class="empty">Loading model activity reports…</div>
		{:else if modelReports.length === 0}
			<div class="empty">
				No model activity reports yet. Reconciliation plus captured usage events will populate this
				section.
			</div>
		{:else}
			<div class="model-chart-grid">
				{#each modelChartGroups as group (group.key)}
					<div class="model-chart-card">
						<div class="model-chart-head">
							<div>
								<div class="cell-primary">{group.modelName}</div>
								<div class="cell-secondary">
									{group.provider} · {group.rows[0].api_key_name || 'API key unavailable'} · {fmtNum(
										group.rows.length
									)} day(s) · grouped by workspace day
								</div>
								{#if isEmbeddingModel(group.modelName)}
									<div class="cell-secondary">
										Embedding vectors are returned data and usually do not count as output tokens.
									</div>
								{/if}
							</div>
						</div>
						<div class="model-chart">
							<Chart
								{init}
								options={buildModelChartOptions(group)}
								style="width: 100%; height: 100%;"
							/>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<div class="panel">
		<div class="panel-head">
			<div>
				<h3>Recent Usage Events</h3>
				<p class="muted">
					Per-request capture from `shared/go/api/llm`, including prompt names, token counts, and
					failures.
				</p>
			</div>
		</div>
		{#if loading && usageEvents.length === 0}
			<div class="empty">Loading usage events…</div>
		{:else if usageEvents.length === 0}
			<div class="empty">
				No usage events yet. This view will fill in once call logging is persisted.
			</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Time</th>
							<th>Record ID</th>
							<th>Call Reason</th>
							<th>Prompt</th>
							<th>Model</th>
							<th>Tokens</th>
							<th>Latency</th>
							<th>Status</th>
							<th>Call Loc</th>
						</tr>
					</thead>
					<tbody>
						{#each usageEvents as event (event.id)}
							<tr>
								<td>
									<div class="cell-primary">{fmtDate(event.request_started_at)}</div>
									<div class="cell-secondary">{event.account_name || event.account_id}</div>
								</td>
								<td>{event.record_id ?? ''}</td>
								<td>{event.call_reason || ''}</td>
								<td>
									<div class="cell-primary">{event.prompt_name}</div>
								</td>
								<td>
									<div class="cell-primary">{event.model_name}</div>
									<div class="cell-secondary">{event.provider}</div>
								</td>
								<td>
									<div class="cell-primary">{tokenSummary(event)}</div>
									{#if isEmbeddingModel(event.model_name)}
										<div class="cell-secondary">
											Vectors returned separately; not counted as output tokens.
										</div>
									{/if}
								</td>
								<td>{fmtNum(event.latency_ms)} ms</td>
								<td class:error-cell={!!event.error_message}>
									{event.error_message ? event.error_message : 'OK'}
								</td>
								<td>{event.call_loc || ''}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

<style>
	.wrap {
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--page);
		min-height: 100%;
		padding: 16px 20px 32px;
	}
	.toolbar,
	.panel-head,
	.toolbar-actions {
		display: flex;
	}
	.toolbar,
	.panel-head {
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
		flex-wrap: wrap;
	}
	.toolbar-actions {
		align-items: flex-end;
		gap: 10px;
		flex-wrap: wrap;
	}
	.toolbar-actions label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12px;
		color: var(--sub);
	}
	.toolbar-actions input {
		width: 88px;
	}
	h2,
	h3 {
		margin: 0;
		color: var(--heading);
	}
	h2 {
		font-size: 20px;
	}
	h3 {
		font-size: 16px;
	}
	.muted {
		color: var(--sub);
		font-size: 12px;
		margin: 4px 0 0;
	}
	.primary {
		background: var(--btn);
		color: white;
		border: none;
		padding: 8px 14px;
		border-radius: 8px;
		font-size: 13px;
		cursor: pointer;
	}
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.secondary {
		background: transparent;
		color: var(--heading);
		border: 1px solid var(--border);
		padding: 8px 14px;
		border-radius: 8px;
		font-size: 13px;
		cursor: pointer;
	}
	.secondary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.summary-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 10px;
	}
	.summary-card,
	.panel {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	.summary-card {
		padding: 14px 16px;
	}
	.summary-label {
		font-size: 12px;
		color: var(--sub);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.summary-value {
		margin-top: 6px;
		font-size: 22px;
		font-weight: 600;
		color: var(--heading);
	}
	.panel {
		padding: 16px;
	}
	.report-filters {
		display: flex;
		align-items: flex-end;
		gap: 10px;
		flex-wrap: wrap;
	}
	.report-filters label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12px;
		color: var(--sub);
	}
	.report-filters select,
	.report-filters input {
		min-width: 150px;
	}
	.filter-error {
		margin-top: 12px;
		padding: 8px 10px;
		border: 1px solid rgba(248, 113, 113, 0.4);
		border-radius: 8px;
		background: rgba(248, 113, 113, 0.12);
		color: #f87171;
		font-size: 13px;
	}
	.error {
		background: rgba(248, 113, 113, 0.12);
		color: #f87171;
		padding: 10px 12px;
		border-radius: 8px;
		font-size: 13px;
	}
	.notice {
		background: rgba(16, 185, 129, 0.12);
		color: #10b981;
		padding: 10px 12px;
		border-radius: 8px;
		font-size: 13px;
	}
	.table-wrap {
		overflow-x: auto;
		margin-top: 12px;
	}
	.model-chart-grid {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: 14px;
		margin-top: 12px;
	}
	.model-chart-card {
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 14px;
		background: color-mix(in srgb, var(--card) 92%, transparent);
	}
	.model-chart {
		width: 100%;
		height: 360px;
	}
	.balance-chart-scroll { overflow-x: auto; width: 100%; }
	.balance-frequency { display: flex; flex-direction: column; gap: 4px; font-size: 11px; color: var(--sub); }
	.model-chart-head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 12px;
		margin-bottom: 12px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		padding: 12px 10px;
		border-top: 1px solid var(--border);
		font-size: 13px;
		color: var(--heading);
		text-align: left;
		vertical-align: top;
	}
	th {
		color: var(--sub);
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		border-top: none;
		padding-top: 0;
	}
	input {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	select {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	.cell-primary {
		font-weight: 600;
	}
	.cell-secondary {
		font-size: 12px;
		color: var(--sub);
		margin-top: 2px;
	}
	.error-cell {
		color: #f87171;
	}
	.empty {
		color: var(--sub);
		font-style: italic;
		padding: 24px 8px 8px;
	}
</style>
