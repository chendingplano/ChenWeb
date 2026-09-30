<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
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
	let balanceTimePreset = $state('last_30_days');
	let balanceCustomFrom = $state('');
	let balanceCustomTo = $state('');
	let balanceSelectedAPIKey = $state('');
	let balanceLoading = $state(false);
	let balanceFilterError = $state<string | null>(null);
	const balanceReportLimit = 1000;
	const reportFrequency = $derived(
		timePreset === 'today' || timePreset === 'yesterday' ? 'hourly' : 'daily'
	);
	const balanceFrequency = $derived(
		balanceTimePreset === 'today' || balanceTimePreset === 'yesterday' ? 'hourly' : 'daily'
	);

	const timeOptions = [
		{ value: 'today', label: m.llm_activities_today() },
		{ value: 'yesterday', label: m.llm_activities_yesterday() },
		{ value: 'last_7_days', label: m.llm_activities_last_7_days() },
		{ value: 'last_30_days', label: m.llm_activities_last_30_days() },
		{ value: 'this_month', label: m.llm_activities_this_month() },
		{ value: 'last_month', label: m.llm_activities_last_month() },
		{ value: 'custom', label: m.llm_activities_custom() }
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
			// Fetch first: "today"/"yesterday"/etc. presets below are anchored on
			// this server-computed workspace day (workspace_timezone), not the
			// viewer's browser clock/timezone, which is frequently a different
			// zone than the deployment (e.g. a Chicago browser against a
			// Shanghai-workspace box) and would otherwise silently mis-bucket or
			// truncate the current workspace day.
			const summaryResponse = await getLLMTodaySummary();
			todaySummary = summaryResponse.summary;
			const [balancesResponse, hourlyResponse, reportsResponse, eventsResponse] = await Promise.all(
				[
					listLLMCurrentBalances(),
					listLLMHourlyBalanceReports(balanceReportLimit, balanceFrequency, getBalanceFilters()),
					listLLMModelActivityReports(reportLimit, getReportFilters()),
					listLLMUsageEvents(eventLimit)
				]
			);
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

	// Pure calendar-date helpers: dates are handled as UTC-midnight `Date`
	// instants purely as a day-count/formatting convenience, never read back
	// via local getters. That keeps day arithmetic (add/subtract days, start
	// of month, ...) free of any implicit timezone (browser or system) --
	// the real timezone anchor is workspaceTodayString() below.
	function parseDateOnly(dateStr: string): Date {
		const [year, month, day] = dateStr.split('-').map(Number);
		return new Date(Date.UTC(year, month - 1, day));
	}

	function dateOnly(date: Date): string {
		const year = date.getUTCFullYear();
		const month = String(date.getUTCMonth() + 1).padStart(2, '0');
		const day = String(date.getUTCDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
	}

	// "Today" per the server's configured workspace_timezone (see
	// GetLLMTodaySummary), falling back to the browser's local date only if
	// that hasn't loaded yet (e.g. the summary request itself failed).
	function workspaceTodayString(): string {
		return todaySummary.workspace_day || dateOnly(new Date());
	}

	function timeFiltersFor(preset: string, fromDate: string, toDate: string): LLMReportFilters {
		const todayDate = parseDateOnly(workspaceTodayString());
		let from = dateOnly(todayDate);
		let to = from;

		switch (preset) {
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
				from = dateOnly(new Date(Date.UTC(todayDate.getUTCFullYear(), todayDate.getUTCMonth(), 1)));
				break;
			case 'last_month':
				from = dateOnly(
					new Date(Date.UTC(todayDate.getUTCFullYear(), todayDate.getUTCMonth() - 1, 1))
				);
				to = dateOnly(new Date(Date.UTC(todayDate.getUTCFullYear(), todayDate.getUTCMonth(), 0)));
				break;
			case 'custom':
				if (!fromDate || !toDate) {
					throw new Error(m.llm_activities_choose_both_a_custom_start());
				}
				if (fromDate > toDate) {
					throw new Error(m.llm_activities_custom_start_date_must_not());
				}
				from = fromDate;
				to = toDate;
				break;
		}

		return { from, to };
	}

	function getReportFilters(): LLMReportFilters {
		return {
			...timeFiltersFor(timePreset, customFrom, customTo),
			apiKey: selectedAPIKey || undefined,
			frequency: reportFrequency
		};
	}

	function getBalanceFilters(): LLMReportFilters {
		return {
			...timeFiltersFor(balanceTimePreset, balanceCustomFrom, balanceCustomTo),
			apiKey: balanceSelectedAPIKey || undefined
		};
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
		balanceFilterError = null;
		balanceLoading = true;
		try {
			const response = await listLLMHourlyBalanceReports(
				balanceReportLimit,
				balanceFrequency,
				getBalanceFilters()
			);
			hourlyBalanceReports = response.reports;
		} catch (err) {
			balanceFilterError = String((err as Error).message ?? err);
		} finally {
			balanceLoading = false;
		}
	}

	function handleBalanceFilterChange() {
		void loadBalanceReports();
	}

	async function runReconciliation() {
		reconciling = true;
		error = null;
		notice = null;
		try {
			const response = await runLLMReconciliationNow();
			notice = response.message ?? m.llm_activities_reconciliation_finished_daily_reports_have();
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

	function fmtReportBucket(raw: string): string {
		const trimmed = raw?.trim() ?? '';
		if (!trimmed || reportFrequency !== 'hourly') {
			return fmtWorkspaceDay(trimmed);
		}

		const normalized = trimmed.replace('T', ' ');
		const date = normalized.slice(0, 10);
		const hour = normalized.slice(11, 13);
		return hour ? `${date} ${hour}:00` : date;
	}

	function isEmbeddingModel(modelName: string): boolean {
		const normalized = (modelName || '').trim().toLowerCase();
		return normalized.includes('embedding') || normalized.includes('embed');
	}

	function tokenSummary(event: LLMUsageEvent): string {
		if (isEmbeddingModel(event.model_name)) {
			return m.llm_activities_input_completion({ input_tokens: fmtNum(event.input_tokens), output_tokens: fmtNum(event.output_tokens) });
		}
		return m.llm_activities_in_out({ input_tokens: fmtNum(event.input_tokens), output_tokens: fmtNum(event.output_tokens) });
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
		const days = group.rows.map((row) => fmtReportBucket(row.workspace_day));
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
					name: m.llm_activities_tokens(),
					nameTextStyle: { color: sub },
					axisLabel: {
						color: sub,
						formatter: (value: number) => fmtNum(value)
					},
					splitLine: { lineStyle: { color: border, opacity: 0.45 } }
				},
				{
					type: 'value',
					name: m.llm_activities_local_estimate({ currencyCode: group.currencyCode || 'CNY' }),
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
					name: m.llm_activities_input_cache_hit(),
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 18,
					data: group.rows.map((row) => row.prompt_cache_hit_tokens)
				},
				{
					name: m.llm_activities_input_cache_miss(),
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 18,
					data: group.rows.map((row) => row.prompt_cache_miss_tokens)
				},
				{
					name: m.llm_activities_output(),
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 18,
					data: group.rows.map((row) => row.output_tokens)
				},
				{
					name: m.llm_activities_local_estimate_2(),
					type: 'bar',
					yAxisIndex: 1,
					barMaxWidth: 18,
					tooltip: {
						valueFormatter: (value) => Number(value).toFixed(2)
					},
					data: group.rows.map((row) => row.spend_amount)
				}
			]
		};
	}

	type BalanceChartGroup = { key: string; accountName: string; rows: LLMHourlyBalanceReport[] };
	const balanceChartGroups = $derived(
		(() => {
			const grouped = new SvelteMap<string, BalanceChartGroup>();
			for (const row of hourlyBalanceReports) {
				const key = row.account_id;
				const group = grouped.get(key) ?? { key, accountName: row.account_name, rows: [] };
				group.rows.push(row);
				grouped.set(key, group);
			}
			return Array.from(grouped.values()).map((group) => ({
				...group,
				rows: [...group.rows].sort((a, b) => a.hour_started_at.localeCompare(b.hour_started_at))
			}));
		})()
	);

	function buildBalanceChartOptions(group: BalanceChartGroup): EChartsOption {
		return {
			backgroundColor: 'transparent',
			animationDuration: 250,
			color: [inputBar, outputBar, spendBar, '#A78BFA', '#22D3EE'],
			legend: { top: 0, textStyle: { color: sub } },
			tooltip: {
				trigger: 'axis',
				axisPointer: { type: 'shadow' },
				backgroundColor: darkMode ? '#0F1320' : '#FFFFFF',
				borderColor: border,
				textStyle: { color: heading }
			},
			grid: { top: 36, right: 30, bottom: 94, left: 70 },
			xAxis: {
				type: 'category',
				data: group.rows.map((row) => fmtBalanceBucket(row.hour_started_at, row.timezone_name ?? 'UTC')),
				axisLine: { lineStyle: { color: border } },
				axisLabel: { color: sub, rotate: 40, margin: 18, hideOverlap: true }
			},
			yAxis: [
				{
					type: 'value',
					name: 'USD',
					nameTextStyle: { color: sub },
					axisLabel: { color: sub },
					splitLine: { lineStyle: { color: border, opacity: 0.45 } }
				},
				{
					type: 'value',
					name: 'CNY',
					nameTextStyle: { color: sub },
					axisLabel: { color: sub },
					splitLine: { show: false }
				}
			],
			series: [
				{
					name: m.llm_activities_current_balance_usd(),
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 20,
					data: group.rows.map((row) => row.balance_usd)
				},
				{
					name: m.llm_activities_current_balance_cny(),
					type: 'bar',
					yAxisIndex: 1,
					barMaxWidth: 20,
					data: group.rows.map((row) => row.balance_cny)
				},
				{
					name: m.llm_activities_spending_cny(),
					type: 'bar',
					yAxisIndex: 1,
					barMaxWidth: 20,
					data: group.rows.map((row) => row.spending_cny)
				},
				{
					name: m.llm_activities_total_spending_cny(),
					type: 'bar',
					yAxisIndex: 1,
					barMaxWidth: 20,
					data: group.rows.map((row) => row.total_spending_cny)
				},
				{
					name: m.llm_activities_total_spending_usd(),
					type: 'bar',
					yAxisIndex: 0,
					barMaxWidth: 20,
					data: group.rows.map((row) => row.total_spending_usd)
				}
			]
		};
	}

	function balanceChartWidth(group: BalanceChartGroup): string {
		return `${Math.max(900, group.rows.length * 72)}px`;
	}

	function fmtBalanceBucket(raw: string, timezoneName: string): string {
		const parts = new Intl.DateTimeFormat('en-US', {
			timeZone: timezoneName,
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: 'numeric',
			hour12: true
		}).formatToParts(new Date(raw));
		const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((item) => item.type === type)?.value ?? '';
		const yyyy = part('year');
		const mm = part('month');
		const dd = part('day');
		if (balanceFrequency === 'daily') return `${yyyy}/${mm}/${dd}`;
		return `${yyyy}/${mm}/${dd} ${part('hour')} ${part('dayPeriod')}`;
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
			<h2>{m.llm_activities_llm_activities()}</h2>
			<p class="muted">
				{m.llm_activities_provider_side_daily_spend_reconciliation()}
			</p>
			<p class="muted">
				{m.llm_activities_refresh_reloads_stored_activity_data()}
			</p>
		</div>
		<div class="toolbar-actions">
			<label>
				<span>{m.llm_activities_reports()}</span>
				<input type="number" min="1" bind:value={reportLimit} />
			</label>
			<label>
				<span>{m.llm_activities_events()}</span>
				<input type="number" min="1" bind:value={eventLimit} />
			</label>
			<button class="primary" onclick={() => load()} disabled={loading}>
				{loading ? m.llm_activities_refreshing() : m.llm_activities_refresh()}
			</button>
			<button class="secondary" onclick={runReconciliation} disabled={loading || reconciling}>
				{reconciling ? m.llm_activities_reconciling() : m.llm_activities_run_reconciliation()}
			</button>
		</div>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">{m.llm_activities_today_s_provider_delta()}</div>
			<div class="summary-value">
				{fmtMoney(todaySummary.spend_amount, todaySummary.currency_code || 'USD')}
			</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">{m.llm_activities_today_s_requests()}</div>
			<div class="summary-value">{fmtNum(todaySummary.request_count)}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">{m.llm_activities_today_s_tokens()}</div>
			<div class="summary-value">{fmtNum(todaySummary.total_tokens)}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">{m.llm_activities_today_s_errors()}</div>
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
				<h3>{m.llm_activities_current_balances()}</h3>
				<p class="muted">
					{m.llm_activities_latest_stored_provider_side_balance()}
				</p>
			</div>
		</div>
		{#if loading && balances.length === 0}
			<div class="empty">{m.llm_activities_loading_current_balances()}</div>
		{:else if balances.length === 0}
			<div class="empty">
				{m.llm_activities_no_balance_snapshots_yet_run()}
			</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>{m.llm_activities_account()}</th>
							<th>{m.llm_activities_provider()}</th>
							<th>{m.llm_activities_balance()}</th>
							<th>{m.llm_activities_captured()}</th>
							<th>{m.llm_activities_workspace_day()}</th>
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
				<h3>{m.llm_activities_official_account_balance_and_spending()}</h3>
				<p class="muted">
					{m.llm_activities_provider_reported_deepseek_balance_by()}
				</p>
			</div>
			<div class="report-filters">
				<label>
					<span>{m.llm_activities_time()}</span>
					<select bind:value={balanceTimePreset} onchange={handleBalanceFilterChange}>
						{#each timeOptions as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
				</label>
				<label>
					<span>{m.llm_activities_api_key()}</span>
					<select bind:value={balanceSelectedAPIKey} onchange={handleBalanceFilterChange}>
						<option value="">{m.llm_activities_all_api_keys()}</option>
						{#each apiKeyOptions as option (option.name)}
							<option value={option.name}>{option.name}</option>
						{/each}
					</select>
				</label>
				{#if balanceTimePreset === 'custom'}
					<label>
						<span>{m.llm_activities_start_date()}</span>
						<input
							type="date"
							bind:value={balanceCustomFrom}
							onchange={handleBalanceFilterChange}
						/>
					</label>
					<label>
						<span>{m.llm_activities_end_date()}</span>
						<input type="date" bind:value={balanceCustomTo} onchange={handleBalanceFilterChange} />
					</label>
				{/if}
			</div>
		</div>
		{#if balanceFilterError}
			<div class="filter-error" role="alert">{balanceFilterError}</div>
		{/if}
		{#if (loading || balanceLoading) && hourlyBalanceReports.length === 0}
			<div class="empty">{m.llm_activities_loading_official_balance_history()}</div>
		{:else if balanceChartGroups.length === 0}
			<div class="empty">
				{m.llm_activities_no_official_balance_history_yet()}
			</div>
		{:else}
			<div class="model-chart-grid">
				{#each balanceChartGroups as group (group.key)}
					<div class="model-chart-card">
						<div class="model-chart-head">
							<div>
								<div class="cell-primary">{group.accountName}</div>
								<div class="cell-secondary">
									{m.llm_activities_official_provider_balance_and_spending({ balanceFrequency })}
								</div>
							</div>
						</div>
						<div class="balance-chart-scroll">
							<div class="model-chart" style:width={balanceChartWidth(group)}>
								<Chart
									{init}
									options={buildBalanceChartOptions(group)}
									style="width: 100%; height: 100%;"
								/>
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<div class="panel">
		<div class="panel-head">
			<div>
				<h3>{m.llm_activities_spend_reports()}</h3>
				<p class="muted">
					{m.llm_activities_per_model_activity_aggregated_by()}
				</p>
			</div>
			<div class="report-filters">
				<label>
					<span>{m.llm_activities_time()}</span>
					<select bind:value={timePreset} onchange={handleReportFilterChange}>
						{#each timeOptions as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
				</label>
				<label>
					<span>{m.llm_activities_api_key()}</span>
					<select bind:value={selectedAPIKey} onchange={handleReportFilterChange}>
						<option value="">{m.llm_activities_all_api_keys()}</option>
						{#each apiKeyOptions as option (option.name)}
							<option value={option.name}>{option.name}</option>
						{/each}
					</select>
				</label>
				{#if timePreset === 'custom'}
					<label>
						<span>{m.llm_activities_start_date()}</span>
						<input type="date" bind:value={customFrom} onchange={handleReportFilterChange} />
					</label>
					<label>
						<span>{m.llm_activities_end_date()}</span>
						<input type="date" bind:value={customTo} onchange={handleReportFilterChange} />
					</label>
				{/if}
			</div>
		</div>
		{#if reportFilterError}
			<div class="filter-error" role="alert">{reportFilterError}</div>
		{/if}
		{#if (loading || reportLoading) && modelReports.length === 0}
			<div class="empty">{m.llm_activities_loading_model_activity_reports()}</div>
		{:else if modelReports.length === 0}
			<div class="empty">
				{m.llm_activities_no_model_activity_reports_yet()}
			</div>
		{:else}
			<div class="model-chart-grid">
				{#each modelChartGroups as group (group.key)}
					<div class="model-chart-card">
						<div class="model-chart-head">
							<div>
								<div class="cell-primary">{group.modelName}</div>
								<div class="cell-secondary">
									{m.llm_activities_grouped_by({ provider: group.provider, api_key_name: group.rows[0].api_key_name || m.llm_activities_api_key_unavailable(), rowsCount: fmtNum(
										group.rows.length
									), value: reportFrequency === 'hourly' ? m.llm_activities_hour_s() : m.llm_activities_day_s(), value2: reportFrequency ===
									'hourly'
										? 'hour'
										: m.llm_activities_workspace_day_2() })}
								</div>
								{#if isEmbeddingModel(group.modelName)}
									<div class="cell-secondary">
										{m.llm_activities_embedding_vectors_are_returned_data()}
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
				<h3>{m.llm_activities_recent_usage_events()}</h3>
				<p class="muted">
					{m.llm_activities_per_request_capture_from_shared()}
				</p>
			</div>
		</div>
		{#if loading && usageEvents.length === 0}
			<div class="empty">{m.llm_activities_loading_usage_events()}</div>
		{:else if usageEvents.length === 0}
			<div class="empty">
				{m.llm_activities_no_usage_events_yet_this()}
			</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>{m.llm_activities_time()}</th>
							<th>{m.llm_activities_record_id()}</th>
							<th>{m.llm_activities_call_reason()}</th>
							<th>{m.llm_activities_prompt()}</th>
							<th>{m.llm_activities_model()}</th>
							<th>{m.llm_activities_tokens()}</th>
							<th>{m.llm_activities_latency()}</th>
							<th>{m.llm_activities_status()}</th>
							<th>{m.llm_activities_call_loc()}</th>
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
											{m.llm_activities_vectors_returned_separately_not_counted()}
										</div>
									{/if}
								</td>
								<td>{m.llm_activities_ms({ latency_ms: fmtNum(event.latency_ms) })}</td>
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
		height: 410px;
	}
	.balance-chart-scroll {
		overflow-x: auto;
		width: 100%;
	}
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
