<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		listJobTypes,
		listSchedules,
		createSchedule,
		updateSchedule,
		deleteSchedule,
		listScheduleRuns,
		formatInterval,
		progressToNextRun,
		type JobType,
		type Schedule,
		type ScheduleRun
	} from './schedules-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let jobTypes = $state<JobType[]>([]);
	let schedules = $state<Schedule[]>([]);
	let loading = $state(false);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let showCreate = $state(false);
	let now = $state(new Date());

	let historyForID = $state<number | null>(null);
	let runs = $state<ScheduleRun[]>([]);
	let runsLoading = $state(false);

	const intervalUnits = [
		{ label: 'minutes', seconds: 60 },
		{ label: 'hours', seconds: 3600 },
		{ label: 'days', seconds: 86400 }
	];

	let draft = $state({
		name: '',
		job_type: '',
		intervalValue: 1,
		intervalUnit: 3600, // hours
		limit: 200,
		enabled: true
	});

	let ticker: ReturnType<typeof setInterval> | undefined;

	onMount(() => {
		loadAll();
		ticker = setInterval(() => {
			now = new Date();
		}, 5000);
	});

	onDestroy(() => {
		if (ticker) clearInterval(ticker);
	});

	async function loadAll() {
		loading = true;
		error = null;
		try {
			const [jt, sch] = await Promise.all([listJobTypes(), listSchedules()]);
			jobTypes = jt;
			schedules = sch;
			if (!draft.job_type && jt.length > 0) {
				draft.job_type = jt[0].job_type;
			}
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	async function submitCreate() {
		error = null;
		info = null;
		if (!draft.name.trim()) {
			error = 'Name is required';
			return;
		}
		if (!draft.job_type) {
			error = 'Job type is required';
			return;
		}
		submitting = true;
		try {
			await createSchedule({
				name: draft.name.trim(),
				job_type: draft.job_type,
				interval_seconds: Math.max(1, Math.round(draft.intervalValue * draft.intervalUnit)),
				params: { limit: draft.limit },
				enabled: draft.enabled
			});
			draft.name = '';
			showCreate = false;
			info = 'Schedule created.';
			await loadAll();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	async function toggleEnabled(sched: Schedule) {
		error = null;
		try {
			await updateSchedule(sched.id, {
				name: sched.name,
				interval_seconds: sched.interval_seconds,
				params: sched.params,
				enabled: !sched.enabled
			});
			await loadAll();
		} catch (err) {
			error = String((err as Error).message ?? err);
		}
	}

	async function removeSchedule(id: number) {
		if (!confirm('Delete this schedule? Its run history will be deleted too.')) return;
		error = null;
		try {
			await deleteSchedule(id);
			if (historyForID === id) historyForID = null;
			await loadAll();
		} catch (err) {
			error = String((err as Error).message ?? err);
		}
	}

	async function viewHistory(id: number) {
		historyForID = id;
		runsLoading = true;
		error = null;
		try {
			runs = await listScheduleRuns(id);
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			runsLoading = false;
		}
	}

	function jobLabel(jobType: string): string {
		return jobTypes.find((j) => j.job_type === jobType)?.label ?? jobType;
	}

	function fmtDate(raw?: string): string {
		return raw ? new Date(raw).toLocaleString() : '—';
	}

	function durationLabel(run: ScheduleRun): string {
		if (!run.finished_at) return run.status === 'running' ? 'running…' : '—';
		const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime();
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function resultSummary(run: ScheduleRun): string {
		const entries = Object.entries(run.result ?? {});
		if (entries.length === 0) return '—';
		return entries.map(([k, v]) => `${k}: ${v}`).join(', ');
	}

	function statusColor(status?: string): string {
		switch (status) {
			case 'success':
				return darkMode ? '#5eead4' : '#0F766E';
			case 'failed':
				return '#f87171';
			case 'running':
				return darkMode ? '#93c5fd' : '#2563eb';
			default:
				return darkMode ? '#94A3B8' : '#6B7280';
		}
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived('#0F766E');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const panelBg = $derived(darkMode ? '#151A29' : '#FDFDFD');
	const trackBg = $derived(darkMode ? '#0F1320' : '#EEF0F3');
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
	style:--panel-bg={panelBg}
	style:--track-bg={trackBg}
>
	<header class="toolbar">
		<div>
			<h2>Schedules</h2>
			<p class="muted">
				Recurring backlog-drain jobs (entity object resolution, ambiguous object resolution, search
				embedding backfill). Runs in-process on the server every 30s tick — nothing here requires an
				external cron.
			</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadAll} disabled={loading}>
				{loading ? 'Refreshing…' : 'Refresh'}
			</button>
			<button class="primary" onclick={() => (showCreate = !showCreate)}>
				{showCreate ? 'Cancel' : '+ New Schedule'}
			</button>
		</div>
	</header>

	{#if error}<div class="error">{error}</div>{/if}
	{#if info}<div class="info">{info}</div>{/if}

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Schedules</div>
			<div class="summary-value">{schedules.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Enabled</div>
			<div class="summary-value">{schedules.filter((s) => s.enabled).length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Last Run Failed</div>
			<div class="summary-value">{schedules.filter((s) => s.last_run_status === 'failed').length}</div>
		</div>
	</div>

	{#if showCreate}
		<form
			class="create-form"
			onsubmit={(e) => {
				e.preventDefault();
				submitCreate();
			}}
		>
			<div class="row two">
				<label>
					<span>Name</span>
					<input bind:value={draft.name} required placeholder="Nightly Entity Resolve" />
				</label>
				<label>
					<span>Job</span>
					<select bind:value={draft.job_type}>
						{#each jobTypes as jt (jt.job_type)}
							<option value={jt.job_type}>{jt.label}</option>
						{/each}
					</select>
				</label>
			</div>
			<div class="row three">
				<label>
					<span>Run every</span>
					<input type="number" bind:value={draft.intervalValue} min="1" />
				</label>
				<label>
					<span>Unit</span>
					<select bind:value={draft.intervalUnit}>
						{#each intervalUnits as u (u.seconds)}
							<option value={u.seconds}>{u.label}</option>
						{/each}
					</select>
				</label>
				<label>
					<span>Limit per run</span>
					<input type="number" bind:value={draft.limit} min="1" />
				</label>
			</div>
			<label class="toggle-row">
				<span>Enabled</span>
				<input type="checkbox" bind:checked={draft.enabled} />
			</label>
			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting}>
					{submitting ? 'Creating…' : 'Create schedule'}
				</button>
			</div>
		</form>
	{/if}

	<h3>Active Schedules</h3>
	{#if schedules.length === 0}
		<p class="empty">No schedules yet. Create one above to start draining a backlog job automatically.</p>
	{:else}
		<div class="schedule-grid">
			{#each schedules as sched (sched.id)}
				{@const progress = progressToNextRun(sched, now)}
				<div class="schedule-card" class:disabled-card={!sched.enabled}>
					<div class="schedule-head">
						<div>
							<div class="schedule-name">{sched.name}</div>
							<div class="cell-secondary">{jobLabel(sched.job_type)}</div>
						</div>
						<label class="switch">
							<input type="checkbox" checked={sched.enabled} onchange={() => toggleEnabled(sched)} />
							<span class="slider"></span>
						</label>
					</div>
					<div class="badge-row">
						<span class="badge">{formatInterval(sched.interval_seconds)}</span>
						{#if sched.last_run_status}
							<span class="badge" style:color={statusColor(sched.last_run_status)}>
								last: {sched.last_run_status}
							</span>
						{/if}
					</div>
					<div class="progress-label">
						<span>next run</span>
						<span>{sched.enabled ? fmtDate(sched.next_run_at) : 'paused'}</span>
					</div>
					<div class="progress-track">
						<div
							class="progress-fill"
							style:width="{Math.round(progress * 100)}%"
							style:background={sched.enabled ? btn : sub}
						></div>
					</div>
					<div class="card-foot">
						<button class="ghost compact-btn" onclick={() => viewHistory(sched.id)}>History</button>
						<button class="ghost compact-btn danger" onclick={() => removeSchedule(sched.id)}>Delete</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	{#if historyForID !== null}
		{@const activeSchedule = schedules.find((s) => s.id === historyForID)}
		<div class="panel">
			<div class="panel-head">
				<h3>Run History{activeSchedule ? ` — ${activeSchedule.name}` : ''}</h3>
				<button class="ghost compact-btn" onclick={() => (historyForID = null)}>Close</button>
			</div>
			{#if runsLoading}
				<p class="empty">Loading…</p>
			{:else if runs.length === 0}
				<p class="empty">No runs yet.</p>
			{:else}
				<div class="table-wrap">
					<table>
						<thead>
							<tr>
								<th>Started</th>
								<th>Status</th>
								<th>Duration</th>
								<th>Result</th>
							</tr>
						</thead>
						<tbody>
							{#each runs as run (run.id)}
								<tr>
									<td>{fmtDate(run.started_at)}</td>
									<td><span style:color={statusColor(run.status)}>{run.status}</span></td>
									<td>{durationLabel(run)}</td>
									<td>
										{#if run.error}
											<span style:color={statusColor('failed')}>{run.error}</span>
										{:else}
											{resultSummary(run)}
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>
	{/if}
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
	.toolbar, .panel-head, .form-foot, .toolbar-actions, .toggle-row, .schedule-head, .badge-row, .progress-label, .card-foot {
		display: flex;
	}
	.toolbar, .panel-head, .schedule-head {
		justify-content: space-between;
		align-items: flex-start;
		gap: 12px;
	}
	.toolbar-actions { gap: 10px; flex-wrap: wrap; }
	.summary-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
		gap: 10px;
	}
	h2, h3 { margin: 0; color: var(--heading); }
	h2 { font-size: 20px; }
	h3 { font-size: 16px; margin-bottom: 4px; }
	.muted { color: var(--sub); font-size: 12px; margin: 4px 0 0; max-width: 640px; }
	.primary, .ghost {
		border-radius: 8px;
		padding: 8px 14px;
		font-size: 13px;
		cursor: pointer;
	}
	.primary { background: var(--btn); color: white; border: none; }
	.ghost { background: transparent; color: var(--heading); border: 1px solid var(--border); }
	.ghost.danger { color: #f87171; border-color: rgba(248, 113, 113, 0.4); }
	.compact-btn { padding: 6px 10px; font-size: 12px; }
	.primary:disabled, .ghost:disabled { opacity: 0.5; cursor: not-allowed; }
	.summary-card, .panel, .create-form, .schedule-card {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	.summary-card { padding: 14px 16px; }
	.summary-label { font-size: 12px; color: var(--sub); text-transform: uppercase; letter-spacing: 0.04em; }
	.summary-value { margin-top: 6px; font-size: 22px; font-weight: 600; color: var(--heading); }
	.create-form, .panel { padding: 16px; }
	.create-form { display: flex; flex-direction: column; gap: 10px; }
	.row { display: grid; gap: 10px; }
	.row.two { grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); }
	.row.three { grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); }
	label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--sub); }
	input, select {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	.toggle-row {
		align-items: center;
		justify-content: space-between;
		background: var(--panel-bg);
		border: 1px solid var(--border);
		padding: 10px 12px;
		border-radius: 8px;
	}
	.form-foot { justify-content: flex-end; }
	.error, .info { padding: 10px 12px; border-radius: 8px; font-size: 13px; }
	.error { background: rgba(248, 113, 113, 0.12); color: #f87171; }
	.info { background: rgba(15, 118, 110, 0.16); color: #5eead4; }
	.empty { color: var(--sub); font-style: italic; padding: 12px 8px; }
	.cell-secondary { font-size: 12px; color: var(--sub); margin-top: 2px; }

	.schedule-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
		gap: 12px;
	}
	.schedule-card { padding: 14px 16px; display: flex; flex-direction: column; gap: 10px; }
	.schedule-card.disabled-card { opacity: 0.55; }
	.schedule-name { font-weight: 600; color: var(--heading); font-size: 14px; }
	.badge-row { gap: 8px; flex-wrap: wrap; }
	.badge {
		font-size: 11px;
		padding: 3px 8px;
		border-radius: 999px;
		background: var(--panel-bg);
		border: 1px solid var(--border);
		color: var(--sub);
	}
	.progress-label {
		justify-content: space-between;
		font-size: 11px;
		color: var(--sub);
	}
	.progress-track {
		height: 6px;
		border-radius: 999px;
		background: var(--track-bg);
		overflow: hidden;
	}
	.progress-fill { height: 100%; transition: width 0.4s ease; }
	.card-foot { justify-content: flex-end; gap: 8px; }

	/* Toggle switch */
	.switch { position: relative; display: inline-block; width: 34px; height: 20px; flex-shrink: 0; }
	.switch input { opacity: 0; width: 0; height: 0; }
	.slider {
		position: absolute;
		cursor: pointer;
		inset: 0;
		background: var(--border);
		border-radius: 999px;
		transition: background 0.2s ease;
	}
	.slider::before {
		content: '';
		position: absolute;
		height: 14px;
		width: 14px;
		left: 3px;
		top: 3px;
		background: white;
		border-radius: 50%;
		transition: transform 0.2s ease;
	}
	.switch input:checked + .slider { background: var(--btn); }
	.switch input:checked + .slider::before { transform: translateX(14px); }

	.table-wrap { overflow-x: auto; margin-top: 8px; }
	table { width: 100%; border-collapse: collapse; }
	th, td {
		padding: 10px 10px;
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
</style>
