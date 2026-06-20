<script lang="ts">
	import { onMount } from 'svelte';

	import {
		getLLMTodaySummary,
		listLLMCurrentBalances,
		listLLMDailyReports,
		listLLMUsageEvents,
		runLLMReconciliationNow,
		type LLMCurrentBalance,
		type LLMDailyReport,
		type LLMTodaySummary,
		type LLMUsageEvent
	} from './llm-activities-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let reports = $state<LLMDailyReport[]>([]);
	let balances = $state<LLMCurrentBalance[]>([]);
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
			const [summaryResponse, balancesResponse, reportsResponse, eventsResponse] = await Promise.all([
				getLLMTodaySummary(),
				listLLMCurrentBalances(),
				listLLMDailyReports(reportLimit),
				listLLMUsageEvents(eventLimit)
			]);
			todaySummary = summaryResponse.summary;
			balances = balancesResponse.balances;
			reports = reportsResponse.reports;
			usageEvents = eventsResponse.usage_events;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
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
		const day = trimmed.includes('T') ? trimmed.slice(0, trimmed.indexOf('T')) : trimmed.slice(0, 10);
		return day || trimmed;
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#A16207' : '#B45309');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
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
			<p class="muted">Provider-side daily spend reconciliation plus per-call usage telemetry for debugging and optimization.</p>
			<p class="muted">`Refresh` reloads stored activity data. `Run Reconciliation` fetches fresh provider balances and updates spend rows.</p>
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
			<button class="primary" onclick={load} disabled={loading}>
				{loading ? 'Refreshing…' : 'Refresh'}
			</button>
			<button class="secondary" onclick={runReconciliation} disabled={loading || reconciling}>
				{reconciling ? 'Reconciling…' : 'Run Reconciliation'}
			</button>
		</div>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Today's Spend</div>
			<div class="summary-value">{fmtMoney(todaySummary.spend_amount, todaySummary.currency_code || 'USD')}</div>
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
				<p class="muted">Latest stored provider-side balance snapshot per account. Use `Run Reconciliation` to fetch a fresh balance.</p>
			</div>
		</div>
		{#if loading && balances.length === 0}
			<div class="empty">Loading current balances…</div>
		{:else if balances.length === 0}
			<div class="empty">No balance snapshots yet. Run reconciliation to capture the latest provider balance.</div>
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
						{#each balances as balance (balance.account_id)}
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
				<h3>Daily Spend Reports</h3>
				<p class="muted">Authoritative daily totals grouped by LLM account and workspace timezone.</p>
			</div>
		</div>
		{#if loading && reports.length === 0}
			<div class="empty">Loading daily reports…</div>
		{:else if reports.length === 0}
			<div class="empty">No daily LLM reports yet. Reconciliation will populate this after the scheduled run.</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Day</th>
							<th>Account</th>
							<th>Spend</th>
							<th>Balance</th>
							<th>Tokens</th>
							<th>Requests</th>
							<th>Status</th>
						</tr>
					</thead>
					<tbody>
						{#each reports as report, index (`${report.account_id}-${report.workspace_day}-${index}`)}
							<tr>
								<td>
									<div class="cell-primary">{fmtWorkspaceDay(report.workspace_day)}</div>
									<div class="cell-secondary">{report.timezone_name}</div>
								</td>
								<td>{report.account_name || report.account_id}</td>
								<td>{fmtMoney(report.spend_amount, report.currency_code)}</td>
								<td>
									<div>{fmtMoney(report.opening_balance, report.currency_code)} -> {fmtMoney(report.closing_balance, report.currency_code)}</div>
								</td>
								<td>{fmtNum(report.input_tokens)} in / {fmtNum(report.output_tokens)} out</td>
								<td>{fmtNum(report.request_count)}</td>
								<td>{report.reconciliation_status}</td>
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
				<h3>Recent Usage Events</h3>
				<p class="muted">Per-request capture from `shared/go/api/llm`, including prompt names, token counts, and failures.</p>
			</div>
		</div>
		{#if loading && usageEvents.length === 0}
			<div class="empty">Loading usage events…</div>
		{:else if usageEvents.length === 0}
			<div class="empty">No usage events yet. This view will fill in once call logging is persisted.</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Time</th>
							<th>Prompt</th>
							<th>Model</th>
							<th>Tokens</th>
							<th>Latency</th>
							<th>Status</th>
						</tr>
					</thead>
					<tbody>
						{#each usageEvents as event (event.id)}
							<tr>
								<td>
									<div class="cell-primary">{fmtDate(event.request_started_at)}</div>
									<div class="cell-secondary">{event.account_name || event.account_id}</div>
								</td>
								<td>{event.prompt_name || 'Unnamed prompt'}</td>
								<td>
									<div class="cell-primary">{event.model_name}</div>
									<div class="cell-secondary">{event.provider}</div>
								</td>
								<td>{fmtNum(event.input_tokens)} in / {fmtNum(event.output_tokens)} out</td>
								<td>{fmtNum(event.latency_ms)} ms</td>
								<td class:error-cell={!!event.error_message}>
									{event.error_message ? event.error_message : 'OK'}
								</td>
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
	.toolbar, .panel-head, .toolbar-actions {
		display: flex;
	}
	.toolbar, .panel-head {
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
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
	h2, h3 { margin: 0; color: var(--heading); }
	h2 { font-size: 20px; }
	h3 { font-size: 16px; }
	.muted { color: var(--sub); font-size: 12px; margin: 4px 0 0; }
	.primary {
		background: var(--btn);
		color: white;
		border: none;
		padding: 8px 14px;
		border-radius: 8px;
		font-size: 13px;
		cursor: pointer;
	}
	.primary:disabled { opacity: 0.5; cursor: not-allowed; }
	.secondary {
		background: transparent;
		color: var(--heading);
		border: 1px solid var(--border);
		padding: 8px 14px;
		border-radius: 8px;
		font-size: 13px;
		cursor: pointer;
	}
	.secondary:disabled { opacity: 0.5; cursor: not-allowed; }
	.summary-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 10px;
	}
	.summary-card, .panel {
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
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th, td {
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
