<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import SystemMonitorView from './system-monitor-view.svelte';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import HammerIcon from '@lucide/svelte/icons/hammer';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import RotateCwIcon from '@lucide/svelte/icons/rotate-cw';
	import CircleIcon from '@lucide/svelte/icons/circle';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import CircleMinusIcon from '@lucide/svelte/icons/circle-minus';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import XIcon from '@lucide/svelte/icons/x';
	import {
		DeploymentError,
		getDeploymentStatus,
		getNextArchive,
		getRun,
		listProcesses,
		restartProcess,
		startBuild,
		startDeployment,
		type DeploymentProcess,
		type DeploymentRole,
		type DeploymentRun,
		type DeploymentStep,
		type NextArchive,
		type RunKind
	} from '$lib/services/deploymentService';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// Service names are product names, the same as on the System Monitor page.
	const processNames: Record<string, string> = {
		chenweb: 'ChenWeb',
		'doc-processor': 'Doc Processor',
		'doc-service': 'Doc Service',
		'parser-result-converter': 'parser-result-converter',
		kratos: 'Kratos',
		nats: 'NATS',
		'pdf-parser': 'pdf-parser',
		caddy: 'Caddy',
		'deepseek-watcher': 'deepseek-watcher'
	};
	const RUNNER_EXITED = 'runner exited without reporting';
	// Binaries deploy-server-china.sh installs without restarting a service.
	const INSTALL_ONLY = new Set(['create-admin', 'deploy-runner']);
	const SELF_RESTART_TIMEOUT_MS = 120_000;

	type Problem = { text: string; detail: string };
	type Dialog =
		| { type: 'deploy'; archive: NextArchive }
		| { type: 'restart'; process: DeploymentProcess };

	let role = $state<DeploymentRole>('');
	let kind = $state<RunKind | ''>('');
	let loaded = $state(false);
	let active = $state<DeploymentRun | null>(null);
	let recent = $state<DeploymentRun[]>([]);
	let selectedId = $state<number | null>(null);
	let shown = $state<DeploymentRun | null>(null);
	let expandedOverride = $state<Record<number, boolean>>({});
	let processes = $state<DeploymentProcess[]>([]);
	let description = $state('');
	let starting = $state(false);
	let checkingRelay = $state(false);
	let actionProblem = $state<Problem | null>(null);
	let loadProblem = $state<Problem | null>(null);
	let unreachable = $state(false);
	let restartingId = $state<string | null>(null);
	let processProblems = $state<Record<string, Problem>>({});
	let selfRestart = $state<{ pid?: number; sawDown: boolean; since: number } | null>(null);
	let selfRestartTimedOut = $state(false);
	let dialog = $state<Dialog | null>(null);
	let dialogLeft = $state(0);
	let dialogTop = $state(0);
	let now = $state(Date.now());

	const colors = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#FFFFFF',
		raised: darkMode ? '#252A3A' : '#F8FAFC',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#64748B',
		border: darkMode ? '#2D3348' : '#DCE2EA',
		accent: darkMode ? '#67E8C2' : '#087F68',
		accentTint: darkMode ? 'rgba(103,232,194,.12)' : 'rgba(8,127,104,.09)',
		danger: darkMode ? '#F87171' : '#B42318',
		dangerTint: darkMode ? 'rgba(248,113,113,.12)' : 'rgba(180,35,24,.07)',
		warning: darkMode ? '#FBBF24' : '#A16207',
		warningTint: darkMode ? 'rgba(251,191,36,.12)' : 'rgba(161,98,7,.08)',
		running: darkMode ? '#93C5FD' : '#1D4ED8'
	});
	const cssVars = $derived(
		Object.entries(colors)
			.map(([key, value]) => `--dep-${key}:${value}`)
			.join(';')
	);

	const locale = $derived(getLocale() === 'zh-cn' ? 'zh-CN' : 'en-US');
	const busy = $derived(!!active || starting);
	const chenweb = $derived(processes.find((p) => p.id === 'chenweb'));

	function problemOf(err: unknown): Problem {
		if (err instanceof DeploymentError) {
			const texts: Record<string, () => string> = {
				wrong_role: () => m.dep_err_wrong_role(),
				active_run: () => m.dep_err_active_run(),
				invalid_archive: () => m.dep_err_invalid_archive(),
				unknown_process: () => m.dep_err_unknown_process(),
				relay_failed: () => m.dep_err_relay_failed(),
				start_failed: () => m.dep_err_start_failed(),
				restart_failed: () => m.dep_err_restart_failed(),
				unreachable: () => m.dep_err_unreachable()
			};
			return { text: (texts[err.code] ?? (() => m.dep_err_generic()))(), detail: err.message };
		}
		return { text: m.dep_err_generic(), detail: err instanceof Error ? err.message : String(err) };
	}

	function isUnreachable(err: unknown) {
		return err instanceof DeploymentError && err.status === 0;
	}

	function setShown(run: DeploymentRun) {
		if (run.id !== shown?.id || run.kind !== shown?.kind) expandedOverride = {};
		shown = run;
	}

	async function updateShown() {
		if (!kind) {
			shown = null;
			return;
		}
		const targetId = selectedId ?? active?.id ?? recent[0]?.id ?? null;
		if (targetId == null) {
			shown = null;
			return;
		}
		if (active && active.id === targetId) {
			setShown(active);
			return;
		}
		const finished = shown?.status === 'succeeded' || shown?.status === 'failed';
		if (shown && shown.id === targetId && finished) return;
		setShown(await getRun(kind, targetId));
	}

	async function refresh() {
		try {
			const status = await getDeploymentStatus();
			role = status.role;
			kind = status.kind;
			active = status.active;
			recent = status.recent;
			unreachable = false;
			loadProblem = null;
			loaded = true;
			await updateShown();
		} catch (err) {
			if (isUnreachable(err)) unreachable = true;
			else loadProblem = problemOf(err);
		}
	}

	async function loadProcesses() {
		try {
			const list = await listProcesses();
			if (selfRestart) {
				const me = list.find((p) => p.id === 'chenweb');
				if (selfRestart.sawDown || (me?.pid && selfRestart.pid && me.pid !== selfRestart.pid)) {
					location.reload();
					return;
				}
			}
			processes = list;
		} catch (err) {
			if (selfRestart && isUnreachable(err)) selfRestart.sawDown = true;
		}
		if (selfRestart && Date.now() - selfRestart.since > SELF_RESTART_TIMEOUT_MS) {
			selfRestart = null;
			restartingId = null;
			selfRestartTimedOut = true;
		}
	}

	async function build() {
		starting = true;
		actionProblem = null;
		try {
			await startBuild(description.trim());
			description = '';
			selectedId = null;
		} catch (err) {
			actionProblem = problemOf(err);
		} finally {
			starting = false;
		}
		await refresh();
	}

	function placeDialog(trigger: HTMLElement) {
		const rect = trigger.getBoundingClientRect();
		const width = Math.min(420, innerWidth - 16);
		const height = 280;
		dialogLeft = Math.max(8, Math.min(rect.left, innerWidth - width - 8));
		dialogTop = rect.bottom + 6 + height > innerHeight ? Math.max(8, rect.top - height - 6) : rect.bottom + 6;
	}

	async function openDeploy(event: MouseEvent) {
		const trigger = event.currentTarget as HTMLElement;
		checkingRelay = true;
		actionProblem = null;
		try {
			const archive = await getNextArchive();
			placeDialog(trigger);
			dialog = { type: 'deploy', archive };
		} catch (err) {
			actionProblem = problemOf(err);
		} finally {
			checkingRelay = false;
		}
	}

	async function confirmDeploy(archive: NextArchive) {
		dialog = null;
		starting = true;
		actionProblem = null;
		try {
			await startDeployment(archive.archive_name);
			selectedId = null;
		} catch (err) {
			actionProblem = problemOf(err);
		} finally {
			starting = false;
		}
		await refresh();
	}

	function openRestart(event: MouseEvent, process: DeploymentProcess) {
		placeDialog(event.currentTarget as HTMLElement);
		dialog = { type: 'restart', process };
	}

	async function confirmRestart(process: DeploymentProcess) {
		dialog = null;
		restartingId = process.id;
		selfRestartTimedOut = false;
		processProblems = Object.fromEntries(Object.entries(processProblems).filter(([id]) => id !== process.id));
		try {
			const result = await restartProcess(process.id);
			if (result.self) {
				selfRestart = { pid: process.pid, sawDown: false, since: Date.now() };
				return;
			}
			await loadProcesses();
		} catch (err) {
			processProblems = { ...processProblems, [process.id]: problemOf(err) };
		}
		if (!selfRestart) restartingId = null;
	}

	// Dialog: dragged by its title bar, resized with the native corner handle.
	let drag: { x: number; y: number; left: number; top: number } | null = null;
	let dialogEl = $state<HTMLElement>();
	function startDrag(event: PointerEvent) {
		if ((event.target as HTMLElement).closest('button')) return;
		drag = { x: event.clientX, y: event.clientY, left: dialogLeft, top: dialogTop };
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
	}
	function moveDrag(event: PointerEvent) {
		if (!drag) return;
		const width = dialogEl?.offsetWidth ?? 0;
		const height = dialogEl?.offsetHeight ?? 0;
		dialogLeft = Math.max(0, Math.min(innerWidth - width, drag.left + event.clientX - drag.x));
		dialogTop = Math.max(0, Math.min(innerHeight - height, drag.top + event.clientY - drag.y));
	}

	function stepLabel(runKind: RunKind, step: DeploymentStep): string {
		const name = step.arg;
		const build: Record<string, () => string> = {
			check: () => m.dep_step_build_check(),
			web: () => m.dep_step_build_web(),
			'prompt-defaults': () => m.dep_step_build_prompt_defaults(),
			binary: () => m.dep_step_build_binary({ name }),
			stage: () => m.dep_step_build_stage(),
			documents: () => m.dep_step_build_documents(),
			'pdf-parser': () => m.dep_step_build_pdf_parser(),
			archive: () => m.dep_step_build_archive(),
			upload: () => m.dep_step_build_upload()
		};
		const deploy: Record<string, () => string> = {
			check: () => m.dep_step_deploy_check(),
			fetch: () => m.dep_step_deploy_fetch(),
			extract: () => m.dep_step_deploy_extract(),
			documents: () => m.dep_step_deploy_documents(),
			migrations: () => m.dep_step_deploy_migrations(),
			prompts: () => m.dep_step_deploy_prompts(),
			configs: () => m.dep_step_deploy_configs(),
			binary: () => (INSTALL_ONLY.has(name) ? m.dep_step_deploy_install({ name }) : m.dep_step_deploy_binary({ name })),
			'pdf-parser': () => m.dep_step_deploy_pdf_parser()
		};
		const label = (runKind === 'build' ? build : deploy)[step.key];
		return label ? label() : m.dep_step_unknown({ key: name ? `${step.key} ${name}` : step.key });
	}

	function statusLabel(status: string): string {
		const labels: Record<string, () => string> = {
			pending: () => m.dep_status_pending(),
			queued: () => m.dep_status_queued(),
			running: () => m.dep_status_running(),
			succeeded: () => m.dep_status_succeeded(),
			failed: () => m.dep_status_failed(),
			skipped: () => m.dep_status_skipped()
		};
		return (labels[status] ?? (() => status))();
	}

	function stateLabel(state?: string): string {
		const labels: Record<string, () => string> = {
			active: () => m.dep_state_active(),
			inactive: () => m.dep_state_inactive(),
			failed: () => m.dep_state_failed(),
			activating: () => m.dep_state_activating(),
			deactivating: () => m.dep_state_deactivating(),
			reloading: () => m.dep_state_reloading()
		};
		return (labels[state ?? ''] ?? (() => m.dep_state_unknown()))();
	}

	function runTitle(run: DeploymentRun): string {
		const number = run.build_number || m.dep_build_number_unknown();
		return run.kind === 'build' ? m.dep_run_build({ number }) : m.dep_run_deploy({ number });
	}

	function formatDate(value?: string): string {
		if (!value) return '';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'medium' }).format(date);
	}

	function duration(start?: string, end?: string): string {
		if (!start) return '';
		const seconds = Math.max(0, Math.round(((end ? Date.parse(end) : now) - Date.parse(start)) / 1000));
		const pad = (n: number) => String(n).padStart(2, '0');
		const h = Math.floor(seconds / 3600);
		const min = Math.floor((seconds % 3600) / 60);
		return h ? `${h}:${pad(min)}:${pad(seconds % 60)}` : `${min}:${pad(seconds % 60)}`;
	}

	function startedAgo(seconds: number): string {
		const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
		if (seconds < 60) return rtf.format(-seconds, 'second');
		if (seconds < 3600) return rtf.format(-Math.floor(seconds / 60), 'minute');
		if (seconds < 86400) return rtf.format(-Math.floor(seconds / 3600), 'hour');
		return rtf.format(-Math.floor(seconds / 86400), 'day');
	}

	function megabytes(bytes: number): string {
		return new Intl.NumberFormat(locale, { style: 'unit', unit: 'megabyte', maximumFractionDigits: 1 }).format(bytes / 1e6);
	}

	function isExpanded(step: DeploymentStep): boolean {
		return expandedOverride[step.seq] ?? (step.status === 'running' || step.status === 'failed');
	}

	function toggle(step: DeploymentStep) {
		expandedOverride = { ...expandedOverride, [step.seq]: !isExpanded(step) };
	}

	function selectRun(run: DeploymentRun) {
		selectedId = run.id;
		void updateShown().catch((err) => (loadProblem = problemOf(err)));
	}

	// Keeps a growing log scrolled to the end unless the reader scrolled up.
	function follow(node: HTMLElement, _text: string) {
		let atEnd = true;
		const onScroll = () => (atEnd = node.scrollHeight - node.scrollTop - node.clientHeight < 24);
		node.addEventListener('scroll', onScroll);
		node.scrollTop = node.scrollHeight;
		return {
			update() {
				if (atEnd) node.scrollTop = node.scrollHeight;
			},
			destroy() {
				node.removeEventListener('scroll', onScroll);
			}
		};
	}

	onMount(() => {
		let stopped = false;
		let statusTimer: ReturnType<typeof setTimeout>;
		let processTimer: ReturnType<typeof setTimeout>;
		const statusLoop = async () => {
			await refresh();
			if (!stopped) statusTimer = setTimeout(statusLoop, active || unreachable ? 2000 : 10000);
		};
		const processLoop = async () => {
			await loadProcesses();
			if (!stopped) processTimer = setTimeout(processLoop, selfRestart ? 1000 : 5000);
		};
		void statusLoop();
		void processLoop();
		const clock = setInterval(() => (now = Date.now()), 1000);
		return () => {
			stopped = true;
			clearTimeout(statusTimer);
			clearTimeout(processTimer);
			clearInterval(clock);
		};
	});
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && dialog && (dialog = null)} />

<div class="dep-page select-text" style={cssVars}>
	<header class="page-head">
		<div class="title-lockup">
			<div class="title-icon" aria-hidden="true"><RocketIcon size={19} /></div>
			<div>
				<p class="eyebrow">{m.nav_sysadmin_system()}</p>
				<h1>{m.dep_title()}</h1>
				<p class="lede">
					{#if role === 'build'}{m.dep_lede_build()}{:else if role === 'deploy'}{m.dep_lede_deploy()}{/if}
				</p>
			</div>
		</div>
		<button type="button" class="ghost" onclick={() => { void refresh(); void loadProcesses(); }}>
			<RefreshCwIcon size={15} />
			{m.dep_refresh()}
		</button>
	</header>

	{#if unreachable || selfRestart}
		<div class="banner warning" role="status">
			<LoaderCircleIcon size={17} class="dep-spin" />
			<span>{selfRestart ? m.dep_restart_self_wait() : m.dep_server_restarting()}</span>
		</div>
	{/if}
	{#if selfRestartTimedOut}
		<div class="banner danger" role="alert"><TriangleAlertIcon size={17} /><span>{m.dep_restart_self_timeout()}</span></div>
	{/if}
	{#if loadProblem}
		<div class="banner danger" role="alert">
			<TriangleAlertIcon size={17} />
			<div><strong>{loadProblem.text}</strong><code>{loadProblem.detail}</code></div>
		</div>
	{/if}

	{#if loaded && !role}
		<section class="card notice">
			<TriangleAlertIcon size={18} />
			<div>
				<h2>{m.dep_role_missing_title()}</h2>
				<p>{m.dep_role_missing_body()}</p>
			</div>
		</section>
	{/if}

	{#if role}
		<section class="card">
			<div class="card-head">
				<div>
					<p class="eyebrow">{role === 'build' ? m.dep_build_section() : m.dep_deploy_section()}</p>
					{#if busy}<p class="busy-note">{role === 'build' ? m.dep_busy_build() : m.dep_busy_deploy()}</p>{/if}
				</div>
				{#if role === 'build'}
					<div class="build-form">
						<label class="field">
							<span>{m.dep_description_label()}</span>
							<input type="text" bind:value={description} placeholder={m.dep_description_placeholder()} disabled={busy} maxlength="300" />
						</label>
						<button type="button" class="primary" onclick={build} disabled={busy}>
							{#if starting}<LoaderCircleIcon size={15} class="dep-spin" />{:else}<HammerIcon size={15} />{/if}
							{starting ? m.dep_starting() : m.dep_build_button()}
						</button>
					</div>
				{:else}
					<button type="button" class="primary" onclick={openDeploy} disabled={busy || checkingRelay}>
						{#if starting || checkingRelay}<LoaderCircleIcon size={15} class="dep-spin" />{:else}<RocketIcon size={15} />{/if}
						{checkingRelay ? m.dep_checking_relay() : starting ? m.dep_starting() : m.dep_deploy_button()}
					</button>
				{/if}
			</div>

			{#if actionProblem}
				<div class="banner danger inset" role="alert">
					<TriangleAlertIcon size={17} />
					<div><strong>{actionProblem.text}</strong><code>{actionProblem.detail}</code></div>
				</div>
			{/if}

			{#if shown}
				{@const run = shown}
				<div class="run-head">
					<div class="run-title">
						<h2>{runTitle(run)}</h2>
						<span class="pill {run.status}">{statusLabel(run.status)}</span>
					</div>
					<div class="run-meta">
						{#if run.requested_by}<span>{m.dep_requested_by({ name: run.requested_by })}</span>{/if}
						{#if run.started_at || run.requested_at}<span>{m.dep_started({ time: formatDate(run.started_at ?? run.requested_at) })}</span>{/if}
						{#if run.started_at}<span>{m.dep_duration({ value: duration(run.started_at, run.finished_at) })}</span>{/if}
						{#if run.archive_name}<span class="mono">{run.archive_name}</span>{/if}
						{#if run.previous_build_number}<span>{m.dep_previous_build({ number: run.previous_build_number })}</span>{/if}
					</div>
					{#if run.description}<p class="run-description">{run.description}</p>{/if}
				</div>

				{#if run.status === 'failed' && run.error_message}
					<div class="banner danger inset" role="alert">
						<CircleXIcon size={17} />
						<div>
							<strong>{run.error_message === RUNNER_EXITED ? m.dep_runner_exited() : m.dep_run_failed_title()}</strong>
							{#if run.error_message !== RUNNER_EXITED}<code>{run.error_message}</code>{/if}
						</div>
					</div>
				{/if}

				<ol class="steps">
					{#each run.steps ?? [] as step (step.seq)}
						{@const open = isExpanded(step)}
						<li class="step {step.status}">
							<button type="button" class="step-row" onclick={() => toggle(step)} aria-expanded={open}
								aria-label={m.dep_show_output({ step: stepLabel(run.kind, step) })}>
								<span class="step-icon" aria-hidden="true">
									{#if step.status === 'running'}<LoaderCircleIcon size={17} class="dep-spin" />
									{:else if step.status === 'succeeded'}<CircleCheckIcon size={17} />
									{:else if step.status === 'failed'}<CircleXIcon size={17} />
									{:else if step.status === 'skipped'}<CircleMinusIcon size={17} />
									{:else}<CircleIcon size={17} />{/if}
								</span>
								<span class="step-label">{stepLabel(run.kind, step)}</span>
								<span class="step-status">{statusLabel(step.status)}</span>
								<span class="step-time mono">{duration(step.started_at, step.finished_at)}</span>
								<span class="chevron" class:open aria-hidden="true"><ChevronRightIcon size={15} /></span>
							</button>
							{#if open}
								<div class="step-output">
									{#if step.output_truncated}<p class="truncated">{m.dep_output_truncated()}</p>{/if}
									{#if step.output}
										<pre use:follow={step.output}>{step.output}</pre>
									{:else}
										<p class="no-output">{m.dep_no_output()}</p>
									{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ol>
			{:else if loaded}
				<p class="empty">{m.dep_no_runs()}</p>
			{/if}
		</section>

		{#if recent.length}
			<section class="card">
				<div class="card-head"><p class="eyebrow">{m.dep_history()}</p></div>
				<div class="table-scroll">
					<table>
						<thead>
							<tr>
								<th>{m.dep_col_build()}</th>
								<th>{m.dep_col_status()}</th>
								<th>{m.dep_col_requested_by()}</th>
								<th>{m.dep_col_started()}</th>
								<th>{m.dep_col_duration()}</th>
								<th>{m.dep_col_archive()}</th>
							</tr>
						</thead>
						<tbody>
							{#each recent as run (run.id)}
								<tr class:selected={shown?.id === run.id}>
									<td><button type="button" class="link mono" onclick={() => selectRun(run)}>{run.build_number || m.dep_build_number_unknown()}</button></td>
									<td><span class="pill {run.status}">{statusLabel(run.status)}</span></td>
									<td>{run.requested_by}</td>
									<td class="mono">{formatDate(run.started_at ?? run.requested_at)}</td>
									<td class="mono">{duration(run.started_at, run.finished_at)}</td>
									<td class="mono">{run.archive_name}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{/if}
	{/if}

	<section class="card">
		<div class="card-head">
			<div>
				<p class="eyebrow">{m.dep_processes()}</p>
				{#if role !== 'deploy'}<p class="busy-note">{m.dep_processes_note_build()}</p>{/if}
			</div>
		</div>
		<div class="processes">
			{#each processes as process (process.id)}
				<article class="process">
					<div class="process-head">
						<div class="process-title">
							<h3>{processNames[process.id] ?? process.id}</h3>
							{#if process.active_state}
								<span class="pill state-{process.active_state}">{stateLabel(process.active_state)}</span>
								{#if process.sub_state}<span class="mono muted">{process.sub_state}</span>{/if}
							{/if}
						</div>
						<div class="process-meta">
							{#if process.pid}<span class="mono">{m.dep_pid({ pid: process.pid })}</span>{/if}
							{#if process.uptime_seconds}<span>{m.dep_proc_started({ ago: startedAgo(process.uptime_seconds) })}</span>{/if}
							{#if process.build_status === 'running' && process.build_number}
								<span class="mono accent">{process.build_number}</span>
							{:else if process.build_status}
								<span class="muted">{m.dep_build_unreported()}</span>
							{/if}
							{#if process.can_restart}
								<button type="button" class="ghost small" disabled={!!restartingId || !!active} onclick={(e) => openRestart(e, process)}>
									{#if restartingId === process.id}<LoaderCircleIcon size={14} class="dep-spin" />{:else}<RotateCwIcon size={14} />{/if}
									{restartingId === process.id ? m.dep_restarting() : m.dep_restart()}
								</button>
							{/if}
						</div>
					</div>
					{#if processProblems[process.id]}
						<div class="banner danger inset" role="alert">
							<TriangleAlertIcon size={16} />
							<div><strong>{processProblems[process.id].text}</strong><code>{processProblems[process.id].detail}</code></div>
						</div>
					{/if}
					<SystemMonitorView {darkMode} sourceId={process.log_source} />
				</article>
			{/each}
		</div>
	</section>

	{#if dialog}
		{@const d = dialog}
		<div class="dialog" bind:this={dialogEl} role="dialog" aria-modal="false" aria-labelledby="dep-dialog-title" style:left="{dialogLeft}px" style:top="{dialogTop}px">
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="dialog-head" onpointerdown={startDrag} onpointermove={moveDrag} onpointerup={() => (drag = null)} onpointercancel={() => (drag = null)} title={m.dep_move_dialog()}>
				<strong id="dep-dialog-title">
					{d.type === 'deploy' ? m.dep_confirm_deploy_title() : m.dep_confirm_restart_title({ name: processNames[d.process.id] ?? d.process.id })}
				</strong>
				<button type="button" class="icon-button" aria-label={m.dep_close()} onclick={() => (dialog = null)}><XIcon size={15} /></button>
			</div>
			<div class="dialog-body">
				{#if d.type === 'deploy'}
					<dl>
						<dt>{m.dep_col_archive()}</dt><dd class="mono">{d.archive.archive_name}</dd>
						<dt>{m.dep_col_build()}</dt><dd class="mono accent">{d.archive.build_number || m.dep_build_number_unknown()}</dd>
						<dt>{m.dep_confirm_size()}</dt><dd>{megabytes(d.archive.size)}</dd>
						<dt>{m.dep_confirm_modified()}</dt><dd>{formatDate(d.archive.modified)}</dd>
						<dt>{m.dep_confirm_current()}</dt><dd class="mono">{chenweb?.build_number || m.dep_build_number_unknown()}</dd>
					</dl>
					<p>{m.dep_confirm_deploy_body()}</p>
				{:else}
					<p>
						{d.process.id === 'chenweb' ? m.dep_confirm_restart_self() : d.process.id === 'pdf-parser' ? m.dep_confirm_restart_pdf() : m.dep_confirm_restart_body()}
					</p>
				{/if}
			</div>
			<div class="dialog-actions">
				<button type="button" class="ghost" onclick={() => (dialog = null)}>{m.dep_cancel()}</button>
				<button type="button" class="primary" onclick={() => (d.type === 'deploy' ? confirmDeploy(d.archive) : confirmRestart(d.process))}>{m.dep_confirm()}</button>
			</div>
		</div>
	{/if}
</div>

<style>
	.dep-page{min-height:100%;padding:clamp(16px,3vw,34px);background:var(--dep-bg);color:var(--dep-text);font-family:'Inter Tight',system-ui,sans-serif;user-select:text;-webkit-user-select:text}
	.page-head,.card,.banner{max-width:1440px;margin-left:auto;margin-right:auto}
	.page-head{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:22px}
	.title-lockup{display:flex;align-items:center;gap:14px;min-width:0}
	.title-icon{display:grid;place-items:center;width:44px;height:44px;flex:none;border:1px solid color-mix(in srgb,var(--dep-accent) 32%,transparent);border-radius:13px;background:var(--dep-accentTint);color:var(--dep-accent)}
	.eyebrow{margin:0;color:var(--dep-accent);font-size:10px;font-weight:750;letter-spacing:.14em;text-transform:uppercase}
	h1{margin:4px 0 0;font-size:clamp(22px,2.5vw,30px);font-weight:680;letter-spacing:-.035em}
	h2{margin:0;font-size:17px;font-weight:650;letter-spacing:-.01em}
	h3{margin:0;font-size:14px;font-weight:650}
	.lede{margin:5px 0 0;color:var(--dep-muted);font-size:13px}
	.mono{font-family:'SFMono-Regular',Consolas,monospace;font-size:11px}
	.muted{color:var(--dep-muted)}.accent{color:var(--dep-accent)}
	button{font-family:inherit;cursor:pointer}
	button:disabled{opacity:.55;cursor:default}
	.primary,.ghost{display:inline-flex;align-items:center;gap:8px;padding:9px 14px;border-radius:9px;font-size:12.5px;font-weight:650;white-space:nowrap}
	.primary{border:1px solid var(--dep-accent);background:var(--dep-accent);color:var(--dep-bg)}
	.ghost{border:1px solid var(--dep-border);background:var(--dep-card);color:var(--dep-text)}
	.ghost.small{padding:5px 10px;font-size:11.5px}
	.icon-button{display:grid;place-items:center;padding:4px;border:0;border-radius:6px;background:transparent;color:var(--dep-muted)}
	.card{margin-bottom:18px;border:1px solid var(--dep-border);border-radius:14px;background:var(--dep-card);overflow:hidden}
	.card-head{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap;padding:16px 20px;border-bottom:1px solid var(--dep-border)}
	.busy-note{margin:5px 0 0;color:var(--dep-muted);font-size:12px}
	.build-form{display:flex;align-items:flex-end;gap:10px;flex-wrap:wrap}
	.field{display:grid;gap:4px;color:var(--dep-muted);font-size:11px}
	.field input{width:min(360px,70vw);padding:8px 10px;border:1px solid var(--dep-border);border-radius:8px;background:var(--dep-raised);color:var(--dep-text);font:13px inherit}
	.notice{display:flex;gap:12px;padding:18px 20px;color:var(--dep-warning)}
	.notice p{margin:6px 0 0;color:var(--dep-text);font-size:13px;line-height:1.5}
	.banner{display:flex;align-items:flex-start;gap:10px;margin-bottom:16px;padding:12px 16px;border:1px solid var(--dep-border);border-radius:11px;font-size:13px}
	.banner div{display:grid;gap:6px;min-width:0}
	.banner code{font:11px/1.5 'SFMono-Regular',Consolas,monospace;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--dep-text);max-height:220px;overflow:auto}
	.banner.inset{margin:14px 20px 0}
	.banner.warning{background:var(--dep-warningTint);border-color:color-mix(in srgb,var(--dep-warning) 35%,var(--dep-border));color:var(--dep-warning)}
	.banner.danger{background:var(--dep-dangerTint);border-color:color-mix(in srgb,var(--dep-danger) 40%,var(--dep-border));color:var(--dep-danger)}
	.run-head{padding:16px 20px 0}
	.run-title{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
	.run-meta{display:flex;flex-wrap:wrap;gap:6px 16px;margin-top:8px;color:var(--dep-muted);font-size:12px}
	.run-description{margin:8px 0 0;font-size:13px}
	.pill{display:inline-flex;padding:3px 8px;border:1px solid var(--dep-border);border-radius:999px;background:var(--dep-raised);color:var(--dep-muted);font-size:10px;font-weight:700;letter-spacing:.04em;text-transform:uppercase}
	.pill.succeeded,.pill.state-active{border-color:color-mix(in srgb,var(--dep-accent) 30%,transparent);background:var(--dep-accentTint);color:var(--dep-accent)}
	.pill.failed,.pill.state-failed{border-color:color-mix(in srgb,var(--dep-danger) 35%,transparent);background:var(--dep-dangerTint);color:var(--dep-danger)}
	.pill.running,.pill.queued,.pill.state-activating,.pill.state-deactivating,.pill.state-reloading{border-color:color-mix(in srgb,var(--dep-running) 35%,transparent);color:var(--dep-running)}
	.steps{list-style:none;margin:14px 0 0;padding:0 0 8px;border-top:1px solid var(--dep-border)}
	.step{border-bottom:1px solid var(--dep-border)}
	.step:last-child{border-bottom:0}
	.step-row{display:grid;grid-template-columns:22px minmax(0,1fr) auto 64px 18px;align-items:center;gap:10px;width:100%;padding:10px 20px;border:0;background:transparent;color:var(--dep-text);text-align:left;font-size:13px}
	.step-row:hover{background:var(--dep-raised)}
	.step-icon{display:grid;place-items:center;color:var(--dep-muted)}
	.step.running .step-icon,.step.running .step-status{color:var(--dep-running)}
	.step.succeeded .step-icon{color:var(--dep-accent)}
	.step.failed .step-icon,.step.failed .step-status,.step.failed .step-label{color:var(--dep-danger)}
	.step.pending .step-label,.step.skipped .step-label{color:var(--dep-muted)}
	.step-status{color:var(--dep-muted);font-size:11px}
	.step-time{color:var(--dep-muted);text-align:right}
	.chevron{display:grid;place-items:center;color:var(--dep-muted);transition:transform .15s}
	.chevron.open{transform:rotate(90deg)}
	.step-output{padding:0 20px 12px 52px}
	.step-output pre{margin:0;max-height:340px;overflow:auto;padding:10px 12px;border:1px solid var(--dep-border);border-radius:8px;background:var(--dep-bg);font:11.5px/1.5 'SFMono-Regular',Consolas,monospace;white-space:pre-wrap;overflow-wrap:anywhere}
	.truncated,.no-output{margin:0 0 6px;color:var(--dep-muted);font-size:11px}
	.empty{margin:0;padding:24px 20px;color:var(--dep-muted);font-size:13px}
	.table-scroll{overflow-x:auto}
	table{width:100%;border-collapse:collapse;text-align:left;white-space:nowrap}
	th{padding:10px 14px;background:var(--dep-raised);color:var(--dep-muted);font-size:9px;font-weight:750;letter-spacing:.1em;text-transform:uppercase}
	td{padding:10px 14px;border-top:1px solid var(--dep-border);font-size:12px}
	tbody tr:hover,tbody tr.selected{background:var(--dep-raised)}
	.link{padding:0;border:0;background:transparent;color:var(--dep-accent);text-decoration:underline;text-underline-offset:3px}
	.processes{display:grid;gap:14px;padding:16px 20px 20px}
	.process{display:grid;gap:10px}
	.process .banner.inset{margin:0}
	.process-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}
	.process-title,.process-meta{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
	.process-meta{color:var(--dep-muted);font-size:12px}
	.dialog{position:fixed;z-index:120;display:flex;flex-direction:column;width:min(420px,calc(100vw - 16px));min-width:280px;min-height:200px;max-width:95vw;max-height:90vh;overflow:auto;resize:both;border:1px solid var(--dep-border);border-radius:12px;background:var(--dep-card);color:var(--dep-text);box-shadow:0 18px 48px rgba(0,0,0,.35)}
	.dialog-head{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:12px 14px;border-bottom:1px solid var(--dep-border);cursor:move;touch-action:none}
	.dialog-head strong{font-size:14px}
	.dialog-body{flex:1;padding:12px 14px;font-size:13px;line-height:1.5}
	.dialog-body p{margin:10px 0 0;color:var(--dep-muted)}
	.dialog-body dl{display:grid;grid-template-columns:auto minmax(0,1fr);gap:6px 12px;margin:0}
	.dialog-body dt{color:var(--dep-muted);font-size:12px}
	.dialog-body dd{margin:0;overflow-wrap:anywhere}
	.dialog-actions{display:flex;justify-content:flex-end;gap:8px;padding:12px 14px;border-top:1px solid var(--dep-border)}
	:global(.dep-spin){animation:dep-spin 1s linear infinite}
	@keyframes dep-spin{to{transform:rotate(360deg)}}
	@media(max-width:640px){.page-head{flex-direction:column;align-items:flex-start}.step-row{grid-template-columns:22px minmax(0,1fr) 56px 18px}.step-status{display:none}.step-output{padding-left:20px}}
</style>
