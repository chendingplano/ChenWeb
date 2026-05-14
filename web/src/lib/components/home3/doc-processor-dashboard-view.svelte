<script lang="ts">
	import { onMount } from 'svelte';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import XCircleIcon from '@lucide/svelte/icons/x-circle';
	import ClockIcon from '@lucide/svelte/icons/clock';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import SearchIcon from '@lucide/svelte/icons/search';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SquareIcon from '@lucide/svelte/icons/square';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CheckSquareIcon from '@lucide/svelte/icons/check-square';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import { listKbInputs, type KbInputRecord } from '$lib/services/kbService';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// Design tokens
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let surface3 = $derived(darkMode ? '#1A1E2C' : '#F8F9FB');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let colorSuccess = $derived(darkMode ? '#34D399' : '#10B981');
	let colorSuccessTint = $derived(darkMode ? 'rgba(52,211,153,0.12)' : 'rgba(16,185,129,0.10)');
	let colorError = $derived(darkMode ? '#F87171' : '#EF4444');
	let colorErrorTint = $derived(darkMode ? 'rgba(248,113,113,0.12)' : 'rgba(239,68,68,0.10)');

	// ── Types ──────────────────────────────────────────────────────────────

	type StageStatus = 'pending' | 'in-progress' | 'success' | 'failed';

	type StatusEntry = {
		operation?: string;
		time?: string;
		start_time?: string;
		status?: string;
		proc_status?: string;
		'proc-status'?: string;
		error?: string;
		progress?: string;
	};

	type StageInfo = {
		id: string;
		label: string;
		status: StageStatus;
		entry?: StatusEntry;
	};

	// ── Pipeline definition ────────────────────────────────────────────────

	// Single ordered pipeline definition. 'operations' lists all operation names
	// this stage may write into kb.inputs.status, including legacy aliases.
	const PIPELINE_STAGES = [
		{ id: 'staged',               label: 'Staged',             operations: [] as string[] },
		{ id: 'parsing',              label: 'PDF Parser',          operations: ['parsing', 'parsed'] },
		{ id: 'converting',           label: 'Result Convert',      operations: ['converting', 'converted', 'line-file-generated'] },
		{ id: 'structure_analyzer',   label: 'Structure Analyzer',  operations: ['structure_analyzer', 'static_analyzer', 'static_analzyer'] },
		{ id: 'chunking',             label: 'Chunking',            operations: ['chunking', 'chunked'] },
		{ id: 'extract_doc_metadata', label: 'Extract Metadata',    operations: ['extract_doc_metadata', 'extract_metadata'] },
		{ id: 'extract_metrics',      label: 'Extract Metrics',     operations: ['extract_metrics'] },
		{ id: 'extract_provisions',   label: 'Extract Provisions',  operations: ['extract_provisions'] },
		{ id: 'generate_summaries',   label: 'Generate Summaries',  operations: ['generate_summaries'] },
		{ id: 'generate_topics',      label: 'Generate Topics',     operations: ['generate_topics'] }
	];

	// Stages managed by the doc-processor service — all must finish for the pipeline to be done.
	const DOC_PROCESSOR_STAGES = PIPELINE_STAGES.slice(3);

	// Processors that can be explicitly requested in launch/restart payloads.
	const ALL_PROCESSOR_IDS = [
		'structure_analyzer', 'chunking', 'extract_doc_metadata', 'extract_metrics', 'extract_provisions',
		'generate_summaries', 'generate_topics'
	];

	// Ordered subset of PIPELINE_STAGES shown in the launch/restart UI.
	const MANUAL_PROCESSORS = PIPELINE_STAGES.filter(s => ALL_PROCESSOR_IDS.includes(s.id));

	// ── Active pipelines state ─────────────────────────────────────────────

	let activePipelines = $state<KbInputRecord[]>([]);
	let pipelinesLoading = $state(true);
	let pipelinesError = $state('');
	let lastPoll = $state('');
	let tooltipState = $state<{
		recordId: number;
		stageId: string;
		label: string;
		status: StageStatus;
		entry?: StatusEntry;
		x: number;
		y: number;
	} | null>(null);

	// ── Sync control state ────────────────────────────────────────────────
	let autoSync = $state(true);
	let syncInterval: ReturnType<typeof setInterval> | null = null;
	let emptyPollCount = 0;
	let prevActivePipelinesCount = 0;

	// ── Manual launch state ────────────────────────────────────────────────

	let searchQuery = $state('');
	let searchLoading = $state(false);
	let searchResults = $state<KbInputRecord[]>([]);
	let searchError = $state('');
	let selectedRecord = $state<KbInputRecord | null>(null);
	let processors = $state<Record<string, boolean>>(
		Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, true]))
	);
	let showConfirm = $state(false);
	let launching = $state(false);
	let launchError = $state('');
	let launchToast = $state<{ kind: 'success' | 'error'; msg: string } | null>(null);

	// ── Restart dialog state ───────────────────────────────────────────────

	let restartTarget = $state<KbInputRecord | null>(null);
	let restartProcessors = $state<Record<string, boolean>>(
		Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, true]))
	);
	let showRestartDialog = $state(false);
	let restarting = $state(false);
	let restartError = $state('');

	// ── Failed Pipelines state ─────────────────────────────────────────────

	const FAILED_PAGE_SIZE = 30;
	let failedExpanded = $state(false);
	let failedRecords = $state<KbInputRecord[]>([]);
	let failedLoading = $state(false);
	let failedError = $state('');
	let failedPage = $state(1);
	let failedTotal = $state(0);
	let failedLoaded = $state(false);

	let failedTotalPages = $derived(Math.max(1, Math.ceil(failedTotal / FAILED_PAGE_SIZE)));

	// ── Helpers ────────────────────────────────────────────────────────────

	function resolveEntryStatus(entry: StatusEntry): StageStatus {
		const ps = (entry.proc_status ?? entry['proc-status'] ?? entry.status ?? '').toLowerCase();
		if (ps === 'success') return 'success';
		if (ps === 'fail' || ps === 'failed' || ps === 'error') return 'failed';
		if (entry.progress !== undefined) return 'in-progress';
		if (!ps) return 'in-progress';
		return 'in-progress';
	}

	function computeStages(record: KbInputRecord): StageInfo[] {
		const statusMap = new Map<string, StatusEntry>();
		for (const e of record.status ?? []) {
			if (e.operation) statusMap.set(e.operation, e as StatusEntry);
		}

		function stageFor(operations: string[]): { status: StageStatus; entry?: StatusEntry } {
			for (const op of operations) {
				const e = statusMap.get(op);
				if (e) return { status: resolveEntryStatus(e), entry: e };
			}
			return { status: 'pending' };
		}

		return PIPELINE_STAGES.map((s) => {
			if (s.id === 'staged') return { id: 'staged', label: s.label, status: 'success' as StageStatus };
			const { status, entry } = stageFor(s.operations);
			return { id: s.id, label: s.label, status, entry };
		});
	}

	const FINAL_STATUSES = new Set(['success', 'fail', 'failed']);

	function isActiveRecord(record: KbInputRecord): boolean {
		const s = record.status ?? [];
		if (!s.length) return true;

		// Build a map of the latest proc_status for each operation.
		const statusMap = new Map<string, string>();
		for (const e of s as StatusEntry[]) {
			const op = e.operation ?? '';
			const ps = (e.proc_status ?? e['proc-status'] ?? e.status ?? '').toLowerCase();
			if (op) statusMap.set(op, ps);
		}

		// Any entry with a non-final status means something is still running.
		for (const ps of statusMap.values()) {
			if (!FINAL_STATUSES.has(ps)) return true;
		}

		// All present entries are final. The pipeline is done only when every
		// doc-processor stage has reported a final status. Until then (e.g. after
		// 'converted' is done but before any leaf processor has reported) the record
		// is active. Blocking is internal and writes no status entry.
		for (const stage of DOC_PROCESSOR_STAGES) {
			const hasFinal = stage.operations.some((op) => FINAL_STATUSES.has(statusMap.get(op) ?? ''));
			if (!hasFinal) return true;
		}

		return false;
	}

	function stageStatusColor(status: StageStatus): string {
		if (status === 'success') return colorSuccess;
		if (status === 'failed') return colorError;
		if (status === 'in-progress') return accent;
		return textMuted;
	}

	function stageStatusBg(status: StageStatus): string {
		if (status === 'success') return colorSuccessTint;
		if (status === 'failed') return colorErrorTint;
		if (status === 'in-progress') return accentTint;
		return 'transparent';
	}

	function recordTitle(record: KbInputRecord): string {
		return record.title?.trim() || record.name?.trim() || record.file_name?.trim() || `Record #${record.id}`;
	}

	function formatTime(s?: string): string {
		if (!s) return '—';
		return s.slice(0, 19).replace('T', ' ');
	}

	function lastStatusText(record: KbInputRecord): string {
		const items = record.status ?? [];
		if (!items.length) return 'staged';
		const last = items[items.length - 1];
		const ps = last.proc_status ?? last['proc-status'] ?? last.status ?? '';
		return `${last.operation ?? '?'} · ${ps || 'running'}`;
	}

	// ── Sync helpers ─────────────────────────────────────────────────────

	function startAutoSync() {
		if (syncInterval !== null) return;
		autoSync = true;
		emptyPollCount = 0;
		syncInterval = setInterval(pollPipelines, 5000);
	}

	function stopAutoSync() {
		if (syncInterval !== null) {
			clearInterval(syncInterval);
			syncInterval = null;
		}
		autoSync = false;
	}

	function toggleSync() {
		if (autoSync) stopAutoSync(); else startAutoSync();
	}

	// ── API calls ──────────────────────────────────────────────────────────

	async function pollPipelines() {
		const prevCount = prevActivePipelinesCount;
		try {
			const res = await listKbInputs({
				docType: 'all',
				parseState: 'all',
				fileName: '',
				startTime: '',
				endTime: '',
				page: 1,
				pageSize: 20
			});
			const newPipelines = (res.results ?? [])
				.filter(r => !r.file_name?.toLowerCase().endsWith('.zip'))
				.filter(isActiveRecord)
				.slice(0, 10);
			const newCount = newPipelines.length;
			activePipelines = newPipelines;
			prevActivePipelinesCount = newCount;
			lastPoll = new Date().toLocaleTimeString();
			pipelinesError = '';

			// Turn sync back on when pipelines appear while sync was off
			if (prevCount === 0 && newCount > 0 && !autoSync) {
				startAutoSync();
			}
			// Track consecutive empty polls; disable sync after 5 following a non-empty state
			if (newCount === 0) {
				if (prevCount > 0) {
					emptyPollCount = 1;
				} else if (emptyPollCount > 0) {
					emptyPollCount++;
					if (emptyPollCount >= 5 && autoSync) stopAutoSync();
				}
			} else {
				emptyPollCount = 0;
			}
		} catch (err) {
			pipelinesError = err instanceof Error ? err.message : 'Failed to load pipelines';
		} finally {
			pipelinesLoading = false;
		}
	}

	async function runSearch() {
		if (!searchQuery.trim()) return;
		searchLoading = true;
		searchError = '';
		selectedRecord = null;
		try {
			const isNumeric = /^\d+$/.test(searchQuery.trim());
			const res = await listKbInputs({
				docType: 'all',
				parseState: 'all',
				fileName: '',
				startTime: '',
				endTime: '',
				page: 1,
				pageSize: 30,
				recordId: isNumeric ? searchQuery.trim() : undefined,
				title: isNumeric ? undefined : searchQuery.trim()
			});
			searchResults = res.results ?? [];
			if (!searchResults.length) searchError = 'No records found.';
		} catch (err) {
			searchError = err instanceof Error ? err.message : 'Search failed';
		} finally {
			searchLoading = false;
		}
	}

	async function doLaunch(record: KbInputRecord, procs: Record<string, boolean>) {
		const chosen = ALL_PROCESSOR_IDS.filter((p) => procs[p]);
		const allChosen = chosen.length === ALL_PROCESSOR_IDS.length;
		const payload: Record<string, unknown> = { record_id: String(record.id), force: true };
		if (!allChosen) payload.operation = chosen;

		const res = await fetch('/api/v1/jetstream/events', {
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ subject: 'kb.line-file-generated', payload: JSON.stringify(payload) })
		});
		if (!res.ok) {
			const body = await res.json().catch(() => null);
			throw new Error(body?.error_msg ?? body?.message ?? `Request failed (${res.status})`);
		}
	}

	async function confirmLaunch() {
		if (!selectedRecord) return;
		launching = true;
		launchError = '';
		try {
			await doLaunch(selectedRecord, processors);
			showConfirm = false;
			launchToast = { kind: 'success', msg: `Launched processing for record #${selectedRecord.id}` };
			setTimeout(() => {
				launchToast = null;
			}, 4000);
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'Launch failed';
		} finally {
			launching = false;
		}
	}

	async function confirmRestart() {
		if (!restartTarget) return;
		restarting = true;
		restartError = '';
		try {
			await doLaunch(restartTarget, restartProcessors);
			showRestartDialog = false;
			restartTarget = null;
			launchToast = { kind: 'success', msg: `Restart triggered` };
			setTimeout(() => {
				launchToast = null;
			}, 4000);
		} catch (err) {
			restartError = err instanceof Error ? err.message : 'Restart failed';
		} finally {
			restarting = false;
		}
	}

	function openRestart(record: KbInputRecord) {
		restartTarget = record;
		restartProcessors = getDefaultRestartProcessors(record);
		restartError = '';
		showRestartDialog = true;
	}

	function getDefaultRestartProcessors(record: KbInputRecord): Record<string, boolean> {
		const unfinishedStageIds = new Set(
			computeStages(record)
				.filter((stage) => stage.status !== 'success' && ALL_PROCESSOR_IDS.includes(stage.id))
				.map((stage) => stage.id)
		);

		if (!unfinishedStageIds.size) {
			return Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, true]));
		}

		return Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, unfinishedStageIds.has(p)]));
	}

	function getFailedSteps(record: KbInputRecord): string[] {
		return (record.status ?? [])
			.filter((e) => {
				const ps = (
					(e as StatusEntry).proc_status ??
					(e as StatusEntry)['proc-status'] ??
					e.status ??
					''
				).toLowerCase();
				return ps === 'failed' || ps === 'fail';
			})
			.map((e) => e.operation ?? '?');
	}

	async function loadFailedPipelines() {
		failedLoading = true;
		failedError = '';
		try {
			const res = await listKbInputs({
				docType: 'all',
				parseState: 'all',
				fileName: '',
				startTime: '',
				endTime: '',
				page: failedPage,
				pageSize: FAILED_PAGE_SIZE,
				procStatus: 'failed'
			});
			failedRecords = res.results ?? [];
			failedTotal = res.total ?? 0;
			failedLoaded = true;
		} catch (err) {
			failedError = err instanceof Error ? err.message : 'Failed to load failed pipelines';
		} finally {
			failedLoading = false;
		}
	}

	function toggleFailedExpanded() {
		failedExpanded = !failedExpanded;
		if (failedExpanded && !failedLoaded && !failedLoading) {
			void loadFailedPipelines();
		}
	}

	function failedGoToPage(page: number) {
		if (page < 1 || page > failedTotalPages) return;
		failedPage = page;
		void loadFailedPipelines();
	}

	function allProcessorsSelected(): boolean {
		return ALL_PROCESSOR_IDS.every((p) => processors[p]);
	}

	function someProcessorsSelected(): boolean {
		return ALL_PROCESSOR_IDS.some((p) => processors[p]);
	}

	function toggleAll() {
		const next = !allProcessorsSelected();
		processors = Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, next]));
	}

	function allRestartSelected(): boolean {
		return ALL_PROCESSOR_IDS.every((p) => restartProcessors[p]);
	}

	function toggleAllRestart() {
		const next = !allRestartSelected();
		restartProcessors = Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, next]));
	}

	function showTooltip(
		e: MouseEvent,
		recordId: number,
		stage: StageInfo
	) {
		const el = e.currentTarget as HTMLElement;
		const rect = el.getBoundingClientRect();
		tooltipState = {
			recordId,
			stageId: stage.id,
			label: stage.label,
			status: stage.status,
			entry: stage.entry,
			x: rect.left + rect.width / 2,
			y: rect.top
		};
	}

	function hideTooltip() {
		tooltipState = null;
	}

	onMount(() => {
		pollPipelines();
		syncInterval = setInterval(pollPipelines, 5000);
		return () => { if (syncInterval !== null) clearInterval(syncInterval); };
	});
</script>

<!-- ══════════════════════════════════════════════════════════════
     Root
══════════════════════════════════════════════════════════════ -->
<div class="flex flex-col" style="min-height:100%;">

	<!-- ── Toast ─────────────────────────────────────────────── -->
	{#if launchToast}
		<div
			class="fixed right-6 top-6 z-50 flex items-center gap-2 rounded-xl px-4 py-3"
			style="background:{launchToast.kind === 'success' ? colorSuccessTint : colorErrorTint};
			       border:1px solid {launchToast.kind === 'success' ? colorSuccess : colorError}40;
			       color:{launchToast.kind === 'success' ? colorSuccess : colorError};
			       font-size:13px; font-weight:500; box-shadow:0 4px 16px rgba(0,0,0,0.25);"
		>
			{#if launchToast.kind === 'success'}
				<CircleCheckIcon class="h-4 w-4 flex-shrink-0" />
			{:else}
				<XCircleIcon class="h-4 w-4 flex-shrink-0" />
			{/if}
			{launchToast.msg}
		</div>
	{/if}

	<!-- ── Floating tooltip ───────────────────────────────────── -->
	{#if tooltipState}
		<div
			class="pointer-events-none fixed z-50 rounded-lg px-3 py-2"
			style="
				left:{tooltipState.x}px; top:{tooltipState.y - 8}px;
				transform:translate(-50%, -100%);
				background:{cardBg}; border:1px solid {borderColor};
				box-shadow:0 8px 24px rgba(0,0,0,0.30);
				min-width:160px; max-width:280px;
			"
		>
			<div style="font-size:11px; font-weight:600; color:{stageStatusColor(tooltipState.status)}; margin-bottom:4px; text-transform:uppercase; letter-spacing:0.08em;">
				{tooltipState.label} · {tooltipState.status}
			</div>
			{#if tooltipState.entry}
				{#if tooltipState.entry.progress}
					<div style="font-size:12px; color:{textSecondary};">Progress: {tooltipState.entry.progress}</div>
				{/if}
				{#if tooltipState.entry.time || tooltipState.entry.start_time}
					<div style="font-size:11px; color:{textMuted}; font-family:monospace; margin-top:2px;">
						{formatTime(tooltipState.entry.time ?? tooltipState.entry.start_time)}
					</div>
				{/if}
				{#if tooltipState.entry.error}
					<div style="font-size:11px; color:{colorError}; margin-top:4px; word-break:break-word;">
						{tooltipState.entry.error}
					</div>
				{/if}
			{:else}
				<div style="font-size:12px; color:{textMuted};">Not yet started</div>
			{/if}
		</div>
	{/if}

	<!-- ════════════════════════════════════════════════════════
	     Section 1 — Active Pipelines
	══════════════════════════════════════════════════════════ -->
	<section class="p-6">

		<!-- Section header -->
		<div class="mb-4 flex items-start justify-between">
			<div>
				<h2 style="font-size:15px; font-weight:600; color:{textPrimary}; margin:0 0 3px;">Active Pipelines</h2>
				<p style="font-size:12px; color:{textMuted}; margin:0;">
				{autoSync ? 'Live processing threads — refreshes every 5 s' : 'Live processing threads — auto-sync paused'}
			</p>
			</div>
			<div class="flex items-center gap-3">
				{#if lastPoll}
					<span style="font-size:11px; color:{textMuted}; font-family:monospace;">Updated {lastPoll}</span>
				{/if}
				<!-- Stop Sync / Start Sync toggle -->
				<button
					onclick={toggleSync}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 transition-none"
					style="background:{autoSync ? surface2 : colorSuccessTint}; border:1px solid {autoSync ? borderColor : colorSuccess + '50'}; color:{autoSync ? textSecondary : colorSuccess}; font-size:12px; font-weight:500; cursor:pointer;"
					onmouseenter={(e) => {
						(e.currentTarget as HTMLElement).style.color = autoSync ? textPrimary : colorSuccess;
						(e.currentTarget as HTMLElement).style.borderColor = autoSync ? accent + '60' : colorSuccess;
					}}
					onmouseleave={(e) => {
						(e.currentTarget as HTMLElement).style.color = autoSync ? textSecondary : colorSuccess;
						(e.currentTarget as HTMLElement).style.borderColor = autoSync ? borderColor : colorSuccess + '50';
					}}
				>
					{#if autoSync}
						<PauseIcon class="h-3 w-3" />
						Stop Sync
					{:else}
						<PlayIcon class="h-3 w-3" />
						Start Sync
					{/if}
				</button>
				<button
					onclick={pollPipelines}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 transition-none"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:12px; font-weight:500; cursor:pointer;"
					onmouseenter={(e) => {
						(e.currentTarget as HTMLElement).style.color = textPrimary;
						(e.currentTarget as HTMLElement).style.borderColor = accent + '60';
					}}
					onmouseleave={(e) => {
						(e.currentTarget as HTMLElement).style.color = textSecondary;
						(e.currentTarget as HTMLElement).style.borderColor = borderColor;
					}}
				>
					<RefreshCwIcon class="h-3 w-3" />
					Refresh
				</button>
			</div>
		</div>

		<!-- Loading skeleton -->
		{#if pipelinesLoading}
			<div class="space-y-3">
				{#each Array(3) as _, i}
					<div
						class="rounded-xl"
						style="height:90px; background:{surface2}; border:1px solid {borderColor}; opacity:{1 - i * 0.25};"
					></div>
				{/each}
			</div>

		<!-- Error -->
		{:else if pipelinesError}
			<div
				class="flex items-center gap-3 rounded-xl p-4"
				style="background:{colorErrorTint}; border:1px solid {colorError}30; color:{colorError}; font-size:13px;"
			>
				<AlertCircleIcon class="h-4 w-4 flex-shrink-0" />
				{pipelinesError}
			</div>

		<!-- Empty state -->
		{:else if activePipelines.length === 0}
			<div
				class="flex flex-col items-center rounded-xl py-14"
				style="background:{surface2}; border:1px solid {borderColor};"
			>
				<ActivityIcon class="mb-3 h-10 w-10" style="color:{textMuted}; opacity:0.4;" />
				<p style="font-size:14px; font-weight:500; color:{textSecondary}; margin:0 0 6px;">No active pipelines</p>
				<p style="font-size:12px; color:{textMuted}; margin:0; max-width:320px; text-align:center; line-height:1.5;">
					Processing threads appear here while documents are being processed. Use Manual Launch below to start one.
				</p>
			</div>

		<!-- Pipeline cards -->
		{:else}
			<div class="space-y-3">
				{#each activePipelines as record (record.id)}
					{@const stages = computeStages(record)}
					<div
						class="rounded-xl p-4"
						style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 1px 3px rgba(0,0,0,0.20);"
					>
						<!-- Card header -->
						<div class="mb-4 flex items-center justify-between">
							<div class="flex items-center gap-2 min-w-0">
								<span
									style="font-family:monospace; font-size:10px; font-weight:600; color:{accent};
									       background:{accentTint}; border:1px solid {accent}30; border-radius:6px; padding:1px 6px; flex-shrink:0;"
								>#{record.id}</span>
								<span
									class="truncate"
									style="font-size:13px; font-weight:600; color:{textPrimary}; max-width:340px;"
									title={recordTitle(record)}
								>{recordTitle(record)}</span>
								<span
									style="font-size:10px; padding:1px 7px; border-radius:999px;
									       background:{accentTint}; color:{accent}; font-family:monospace; flex-shrink:0;"
								>{record.type}</span>
							</div>
							<div class="flex flex-shrink-0 items-center gap-2">
								<span style="font-size:11px; color:{textMuted}; font-family:monospace;">{formatTime(record.modify_time)}</span>
								<!-- Stop button (disabled, no API) -->
								<button
									disabled
									title="Stop — not yet implemented"
									class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
									style="background:{surface2}; border:1px solid {borderColor}; color:{textMuted}; font-size:12px; cursor:not-allowed; opacity:0.5;"
								>
									<SquareIcon class="h-3 w-3" />
									Stop
								</button>
								<!-- Restart button -->
								<button
									onclick={() => openRestart(record)}
									class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
									style="background:{accentTint}; border:1px solid {accent}30; color:{accent}; font-size:12px; font-weight:500; cursor:pointer;"
									onmouseenter={(e) => {
										(e.currentTarget as HTMLElement).style.background = accent + '25';
									}}
									onmouseleave={(e) => {
										(e.currentTarget as HTMLElement).style.background = accentTint;
									}}
								>
									<PlayIcon class="h-3 w-3" />
									Restart
								</button>
							</div>
						</div>

						<!-- Pipeline visualization — single horizontal chain -->
						<div class="flex items-center gap-0 overflow-x-auto" style="padding-bottom:4px;">
							{#each stages as stage, i}
								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<div
									class="relative flex flex-shrink-0 flex-col items-center"
									style="min-width:68px;"
									onmouseenter={(e) => showTooltip(e, record.id, stage)}
									onmouseleave={hideTooltip}
								>
									<div
										class="flex items-center justify-center rounded-full"
										style="
											width:28px; height:28px;
											background:{stageStatusBg(stage.status)};
											border:2px solid {stageStatusColor(stage.status)}{stage.status === 'pending' ? '40' : ''};
											{stage.status === 'in-progress' ? 'animation:pulse-ring 1.5s ease-in-out infinite;' : ''}
										"
									>
										{#if stage.status === 'success'}
											<CircleCheckIcon class="h-3.5 w-3.5" style="color:{colorSuccess};" />
										{:else if stage.status === 'failed'}
											<XCircleIcon class="h-3.5 w-3.5" style="color:{colorError};" />
										{:else if stage.status === 'in-progress'}
											<div
												class="rounded-full"
												style="width:8px; height:8px; background:{accent}; animation:pulse 1s ease-in-out infinite;"
											></div>
										{:else}
											<ClockIcon class="h-3 w-3" style="color:{textMuted}; opacity:0.5;" />
										{/if}
									</div>
									<div
										style="font-size:9px; color:{stageStatusColor(stage.status)}{stage.status === 'pending' ? '80' : ''}; margin-top:4px; text-align:center; white-space:nowrap; max-width:68px; overflow:hidden; text-overflow:ellipsis;"
									>{stage.label}</div>
								</div>
								{#if i < stages.length - 1}
									<div style="flex:0 0 16px; height:2px; background:{borderColor}; margin-bottom:13px; flex-shrink:0;"></div>
								{/if}
							{/each}
						</div>

						<!-- Status line -->
						<div class="mt-3" style="font-size:11px; color:{textMuted}; font-family:monospace; border-top:1px solid {borderColor}; padding-top:8px;">
							Last: {lastStatusText(record)}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<!-- ── Section divider ────────────────────────────────────── -->
	<div style="height:1px; background:{borderColor}; margin:0 24px;"></div>

	<!-- ════════════════════════════════════════════════════════
	     Section 2 — Manual Launch
	══════════════════════════════════════════════════════════ -->
	<section class="p-6">

		<div class="mb-4">
			<h2 style="font-size:15px; font-weight:600; color:{textPrimary}; margin:0 0 3px;">Manual Launch</h2>
			<p style="font-size:12px; color:{textMuted}; margin:0;">Search a kb.inputs record and launch selected processors</p>
		</div>

		<!-- Search bar -->
		<div class="mb-4 flex gap-2">
			<div
				class="flex flex-1 items-center gap-2 rounded-lg px-3"
				style="background:{surface2}; border:1px solid {borderColor}; height:38px;"
			>
				<SearchIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
				<input
					type="text"
					bind:value={searchQuery}
					placeholder="Record ID or title…"
					onkeydown={(e) => { if (e.key === 'Enter') void runSearch(); }}
					class="flex-1 bg-transparent outline-none"
					style="font-size:13px; color:{textPrimary}; border:none;"
				/>
			</div>
			<button
				onclick={runSearch}
				disabled={searchLoading || !searchQuery.trim()}
				class="flex items-center gap-1.5 rounded-lg px-4 py-2"
				style="background:{accent}; color:white; font-size:13px; font-weight:600; cursor:pointer; border:none; opacity:{searchLoading || !searchQuery.trim() ? '0.55' : '1'};"
				onmouseenter={(e) => {
					if (!searchLoading && searchQuery.trim()) (e.currentTarget as HTMLElement).style.opacity = '0.88';
				}}
				onmouseleave={(e) => {
					(e.currentTarget as HTMLElement).style.opacity = searchLoading || !searchQuery.trim() ? '0.55' : '1';
				}}
			>
				{searchLoading ? 'Searching…' : 'Search'}
			</button>
		</div>

		<!-- Search error -->
		{#if searchError && !searchResults.length}
			<div style="font-size:13px; color:{colorError}; margin-bottom:12px;">{searchError}</div>
		{/if}

		<!-- Results table -->
		{#if searchResults.length > 0}
			<div
				class="mb-4 overflow-hidden rounded-xl"
				style="border:1px solid {borderColor};"
			>
				<table style="width:100%; border-collapse:collapse; font-size:13px;">
					<thead>
						<tr style="background:{surface2}; border-bottom:1px solid {borderColor};">
							{#each ['ID', 'Type', 'Title', 'Parser', 'Last Status', 'Updated'] as col}
								<th
									class="px-3 py-2 text-left"
									style="font-size:10px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em; white-space:nowrap;"
								>{col}</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each searchResults as rec (rec.id)}
							<tr
								onclick={() => (selectedRecord = selectedRecord?.id === rec.id ? null : rec)}
								style="
									border-bottom:1px solid {borderColor};
									background:{selectedRecord?.id === rec.id ? accentTint : 'transparent'};
									cursor:pointer;
								"
								onmouseenter={(e) => {
									if (selectedRecord?.id !== rec.id)
										(e.currentTarget as HTMLElement).style.background = surface2;
								}}
								onmouseleave={(e) => {
									if (selectedRecord?.id !== rec.id)
										(e.currentTarget as HTMLElement).style.background = 'transparent';
								}}
							>
								<td class="px-3 py-2.5" style="font-family:monospace; color:{accent}; font-size:12px;">{rec.id}</td>
								<td class="px-3 py-2.5" style="color:{textMuted}; font-family:monospace; font-size:11px;">{rec.type}</td>
								<td class="px-3 py-2.5 max-w-xs truncate" style="color:{textPrimary};" title={recordTitle(rec)}>{recordTitle(rec)}</td>
								<td class="px-3 py-2.5" style="color:{textMuted}; font-family:monospace; font-size:11px;">{rec.parser_name || '—'}</td>
								<td class="px-3 py-2.5" style="color:{textSecondary}; font-family:monospace; font-size:11px;">{lastStatusText(rec)}</td>
								<td class="px-3 py-2.5" style="color:{textMuted}; font-family:monospace; font-size:11px; white-space:nowrap;">{formatTime(rec.modify_time)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}

		<!-- Processor selection + launch (shown once a record is selected) -->
		{#if selectedRecord}
			<div
				class="rounded-xl p-4"
				style="background:{cardBg}; border:1px solid {borderColor};"
			>
				<div class="mb-3 flex items-center justify-between">
					<div>
						<div style="font-size:12px; color:{textMuted}; margin-bottom:2px; font-family:monospace; text-transform:uppercase; letter-spacing:0.08em;">Selected record</div>
						<div style="font-size:13px; font-weight:600; color:{textPrimary};">
							#{selectedRecord.id} — {recordTitle(selectedRecord)}
						</div>
					</div>
				</div>

				<div style="font-size:12px; color:{textMuted}; margin-bottom:10px; text-transform:uppercase; letter-spacing:0.08em; font-weight:600;">Processors to run</div>

				<div class="mb-4 space-y-1.5">
					<!-- Always-on: blocking -->
					<label
						class="flex cursor-not-allowed items-center gap-2.5 rounded-lg px-3 py-2"
						style="background:{surface2}; border:1px solid {borderColor}; opacity:0.6;"
					>
						<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{colorSuccess};" />
						<span style="font-size:13px; color:{textSecondary};">blocking</span>
						<span style="font-size:10px; color:{textMuted}; margin-left:auto; font-family:monospace;">always on</span>
					</label>

					<!-- Selectable processors -->
					{#each MANUAL_PROCESSORS as proc}
						<label
							class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
							style="background:{processors[proc.id] ? accentTint : surface2}; border:1px solid {processors[proc.id] ? accent + '40' : borderColor};"
							onmouseenter={(e) => {
								if (!processors[proc.id]) (e.currentTarget as HTMLElement).style.background = surface3;
							}}
							onmouseleave={(e) => {
								(e.currentTarget as HTMLElement).style.background = processors[proc.id] ? accentTint : surface2;
							}}
						>
							<input
								type="checkbox"
								bind:checked={processors[proc.id]}
								class="sr-only"
							/>
							{#if processors[proc.id]}
								<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
							{:else}
								<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
							{/if}
							<span style="font-size:13px; color:{processors[proc.id] ? textPrimary : textSecondary}; font-family:monospace;">{proc.id}</span>
							<span style="font-size:12px; color:{textMuted}; margin-left:6px;">{proc.label}</span>
						</label>
					{/each}
				</div>

				<!-- Toggle all + launch -->
				<div class="flex items-center justify-between">
					<button
						onclick={toggleAll}
						class="rounded-lg px-3 py-1.5"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:12px; cursor:pointer;"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; }}
					>
						{allProcessorsSelected() ? 'Deselect all' : 'Select all'}
					</button>

					<button
						onclick={() => { launchError = ''; showConfirm = true; }}
						disabled={!someProcessorsSelected()}
						class="flex items-center gap-2 rounded-lg px-4 py-2"
						style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none; cursor:{someProcessorsSelected() ? 'pointer' : 'not-allowed'}; opacity:{someProcessorsSelected() ? '1' : '0.5'};"
						onmouseenter={(e) => {
							if (someProcessorsSelected()) (e.currentTarget as HTMLElement).style.opacity = '0.88';
						}}
						onmouseleave={(e) => {
							(e.currentTarget as HTMLElement).style.opacity = someProcessorsSelected() ? '1' : '0.5';
						}}
					>
						<PlayIcon class="h-4 w-4" />
						Launch
					</button>
				</div>
			</div>
		{/if}
	</section>

	<!-- ── Section divider ────────────────────────────────────── -->
	<div style="height:1px; background:{borderColor}; margin:0 24px;"></div>

	<!-- ════════════════════════════════════════════════════════
	     Section 3 — Failed Pipelines
	══════════════════════════════════════════════════════════ -->
	<section class="p-6">

		<!-- Collapsible header -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="flex cursor-pointer items-center justify-between"
			onclick={toggleFailedExpanded}
			onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleFailedExpanded(); } }}
			role="button"
			tabindex="0"
			style="user-select:none;"
		>
			<div class="flex items-center gap-2">
				{#if failedExpanded}
					<ChevronDownIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
				{:else}
					<ChevronRightIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
				{/if}
				<div>
					<h2 style="font-size:15px; font-weight:600; color:{textPrimary}; margin:0 0 2px;">
						Failed Pipelines
						{#if failedLoaded}
							<span style="font-size:12px; font-weight:400; color:{colorError}; margin-left:6px;">({failedTotal})</span>
						{/if}
					</h2>
					<p style="font-size:12px; color:{textMuted}; margin:0;">Records with at least one failed processing step</p>
				</div>
			</div>
			{#if failedExpanded}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<button
					onclick={(e) => { e.stopPropagation(); failedPage = 1; void loadFailedPipelines(); }}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:12px; font-weight:500; cursor:pointer;"
					onmouseenter={(e) => {
						(e.currentTarget as HTMLElement).style.color = textPrimary;
						(e.currentTarget as HTMLElement).style.borderColor = accent + '60';
					}}
					onmouseleave={(e) => {
						(e.currentTarget as HTMLElement).style.color = textSecondary;
						(e.currentTarget as HTMLElement).style.borderColor = borderColor;
					}}
				>
					<RefreshCwIcon class="h-3 w-3" />
					Refresh
				</button>
			{/if}
		</div>

		{#if failedExpanded}
			<div class="mt-4">

				<!-- Loading -->
				{#if failedLoading}
					<div class="space-y-2">
						{#each Array(4) as _, i}
							<div
								class="rounded-lg"
								style="height:44px; background:{surface2}; border:1px solid {borderColor}; opacity:{1 - i * 0.2};"
							></div>
						{/each}
					</div>

				<!-- Error -->
				{:else if failedError}
					<div
						class="flex items-center gap-3 rounded-xl p-4"
						style="background:{colorErrorTint}; border:1px solid {colorError}30; color:{colorError}; font-size:13px;"
					>
						<AlertCircleIcon class="h-4 w-4 flex-shrink-0" />
						{failedError}
					</div>

				<!-- Empty -->
				{:else if failedRecords.length === 0}
					<div
						class="flex flex-col items-center rounded-xl py-10"
						style="background:{surface2}; border:1px solid {borderColor};"
					>
						<CircleCheckIcon class="mb-3 h-8 w-8" style="color:{colorSuccess}; opacity:0.5;" />
						<p style="font-size:14px; font-weight:500; color:{textSecondary}; margin:0 0 4px;">No failed pipelines</p>
						<p style="font-size:12px; color:{textMuted}; margin:0;">All processed records completed without failures.</p>
					</div>

				<!-- Table -->
				{:else}
					<div class="overflow-hidden rounded-xl" style="border:1px solid {borderColor};">
						<table style="width:100%; border-collapse:collapse; font-size:13px;">
							<thead>
								<tr style="background:{surface2}; border-bottom:1px solid {borderColor};">
									{#each ['ID', 'Title', 'Failed Steps', 'Created', ''] as col}
										<th
											class="px-3 py-2 text-left"
											style="font-size:10px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em; white-space:nowrap;"
										>{col}</th>
									{/each}
								</tr>
							</thead>
							<tbody>
								{#each failedRecords as rec (rec.id)}
									{@const failedSteps = getFailedSteps(rec)}
									<tr style="border-bottom:1px solid {borderColor}; background:transparent;">
										<td class="px-3 py-2.5" style="font-family:monospace; color:{accent}; font-size:12px; white-space:nowrap;">{rec.id}</td>
										<td class="px-3 py-2.5 max-w-xs" style="color:{textPrimary};">
											<span class="block truncate" title={recordTitle(rec)}>{recordTitle(rec)}</span>
										</td>
										<td class="px-3 py-2.5">
											<div class="flex flex-wrap gap-1">
												{#each failedSteps as step}
													<span
														style="font-family:monospace; font-size:10px; padding:1px 6px; border-radius:999px;
														       background:{colorErrorTint}; color:{colorError}; border:1px solid {colorError}30; white-space:nowrap;"
													>{step}</span>
												{/each}
											</div>
										</td>
										<td class="px-3 py-2.5" style="color:{textMuted}; font-family:monospace; font-size:11px; white-space:nowrap;">{formatTime(rec.create_time)}</td>
										<td class="px-3 py-2.5 text-right">
											<button
												onclick={() => openRestart(rec)}
												class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
												style="background:{accentTint}; border:1px solid {accent}30; color:{accent}; font-size:11px; font-weight:500; cursor:pointer; white-space:nowrap;"
												onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accent + '25'; }}
												onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = accentTint; }}
											>
												<PlayIcon class="h-3 w-3" />
												Restart
											</button>
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>

					<!-- Pagination -->
					{#if failedTotalPages > 1}
						<div class="mt-3 flex items-center justify-between">
							<span style="font-size:12px; color:{textMuted};">
								{(failedPage - 1) * FAILED_PAGE_SIZE + 1}–{Math.min(failedPage * FAILED_PAGE_SIZE, failedTotal)} of {failedTotal}
							</span>
							<div class="flex items-center gap-1.5">
								<button
									onclick={() => failedGoToPage(failedPage - 1)}
									disabled={failedPage <= 1}
									class="rounded-lg px-3 py-1.5"
									style="background:{surface2}; border:1px solid {borderColor}; color:{failedPage <= 1 ? textMuted : textSecondary}; font-size:12px; cursor:{failedPage <= 1 ? 'not-allowed' : 'pointer'}; opacity:{failedPage <= 1 ? '0.45' : '1'};"
									onmouseenter={(e) => { if (failedPage > 1) (e.currentTarget as HTMLElement).style.color = textPrimary; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = failedPage <= 1 ? textMuted : textSecondary; }}
								>← Prev</button>
								<span style="font-size:12px; color:{textSecondary}; font-family:monospace; padding:0 6px;">
									{failedPage} / {failedTotalPages}
								</span>
								<button
									onclick={() => failedGoToPage(failedPage + 1)}
									disabled={failedPage >= failedTotalPages}
									class="rounded-lg px-3 py-1.5"
									style="background:{surface2}; border:1px solid {borderColor}; color:{failedPage >= failedTotalPages ? textMuted : textSecondary}; font-size:12px; cursor:{failedPage >= failedTotalPages ? 'not-allowed' : 'pointer'}; opacity:{failedPage >= failedTotalPages ? '0.45' : '1'};"
									onmouseenter={(e) => { if (failedPage < failedTotalPages) (e.currentTarget as HTMLElement).style.color = textPrimary; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = failedPage >= failedTotalPages ? textMuted : textSecondary; }}
								>Next →</button>
							</div>
						</div>
					{/if}
				{/if}
			</div>
		{/if}
	</section>
</div>

<!-- ════════════════════════════════════════════════════════════
     Confirm Launch Dialog
══════════════════════════════════════════════════════════════ -->
{#if showConfirm && selectedRecord}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-40 flex items-center justify-center"
		style="background:rgba(0,0,0,0.6); backdrop-filter:blur(4px);"
		onmousedown={(e) => { if (e.target === e.currentTarget) { showConfirm = false; launchError = ''; } }}
	>
		<div
			class="mx-4 w-full max-w-md rounded-2xl p-6"
			style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 24px 64px rgba(0,0,0,0.4);"
		>
			<h3 style="font-size:16px; font-weight:600; color:{textPrimary}; margin:0 0 8px;">Confirm launch</h3>
			<p style="font-size:13px; color:{textSecondary}; margin:0 0 16px; line-height:1.5;">
				Launch processing for record
				<span style="color:{accent}; font-family:monospace; font-weight:600;">#{selectedRecord.id}</span>
				({recordTitle(selectedRecord)})?
			</p>

			<!-- Processors summary -->
			<div
				class="mb-4 rounded-lg px-3 py-2.5"
				style="background:{surface2}; border:1px solid {borderColor}; font-size:12px; color:{textSecondary};"
			>
				<div style="font-size:10px; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em; font-weight:600; margin-bottom:6px;">Processors</div>
				<div class="flex flex-wrap gap-1.5">
					<span style="font-family:monospace; background:{surface3}; border:1px solid {borderColor}; padding:1px 8px; border-radius:999px; font-size:11px; color:{colorSuccess};">blocking</span>
					{#each ALL_PROCESSOR_IDS.filter(p => processors[p]) as p}
						<span style="font-family:monospace; background:{accentTint}; border:1px solid {accent}30; padding:1px 8px; border-radius:999px; font-size:11px; color:{accent};">{p}</span>
					{/each}
				</div>
				{#if ALL_PROCESSOR_IDS.every(p => processors[p])}
					<div style="margin-top:6px; font-size:11px; color:{textMuted};">All processors selected — omitting operation filter.</div>
				{/if}
			</div>

			{#if launchError}
				<div style="font-size:12px; color:{colorError}; margin-bottom:12px;">{launchError}</div>
			{/if}

			<div class="flex justify-end gap-2">
				<button
					onclick={() => { showConfirm = false; launchError = ''; }}
					class="rounded-lg px-4 py-2"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:13px; cursor:pointer;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; }}
				>Cancel</button>
				<button
					onclick={confirmLaunch}
					disabled={launching}
					class="flex items-center gap-2 rounded-lg px-4 py-2"
					style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none; cursor:{launching ? 'not-allowed' : 'pointer'}; opacity:{launching ? '0.7' : '1'};"
					onmouseenter={(e) => { if (!launching) (e.currentTarget as HTMLElement).style.opacity = '0.88'; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = launching ? '0.7' : '1'; }}
				>
					<PlayIcon class="h-4 w-4" />
					{launching ? 'Launching…' : 'Confirm Launch'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- ════════════════════════════════════════════════════════════
     Restart Dialog
══════════════════════════════════════════════════════════════ -->
{#if showRestartDialog && restartTarget}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-40 flex items-center justify-center"
		style="background:rgba(0,0,0,0.6); backdrop-filter:blur(4px);"
		onmousedown={(e) => { if (e.target === e.currentTarget) { showRestartDialog = false; restartError = ''; } }}
	>
		<div
			class="mx-4 w-full max-w-md rounded-2xl p-6"
			style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 24px 64px rgba(0,0,0,0.4);"
		>
			<h3 style="font-size:16px; font-weight:600; color:{textPrimary}; margin:0 0 4px;">Restart pipeline</h3>
			<p style="font-size:13px; color:{textSecondary}; margin:0 0 16px;">
				Record <span style="color:{accent}; font-family:monospace; font-weight:600;">#{restartTarget.id}</span>
				— select processors to re-run:
			</p>

			<div class="mb-4 space-y-1.5">
				<label class="flex cursor-not-allowed items-center gap-2.5 rounded-lg px-3 py-2" style="background:{surface2}; border:1px solid {borderColor}; opacity:0.6;">
					<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{colorSuccess};" />
					<span style="font-size:13px; color:{textSecondary}; font-family:monospace;">blocking</span>
					<span style="font-size:10px; color:{textMuted}; margin-left:auto; font-family:monospace;">always on</span>
				</label>
				{#each MANUAL_PROCESSORS as proc}
					<label
						class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
						style="background:{restartProcessors[proc.id] ? accentTint : surface2}; border:1px solid {restartProcessors[proc.id] ? accent + '40' : borderColor};"
					>
						<input type="checkbox" bind:checked={restartProcessors[proc.id]} class="sr-only" />
						{#if restartProcessors[proc.id]}
							<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
						{:else}
							<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
						{/if}
						<span style="font-size:13px; color:{restartProcessors[proc.id] ? textPrimary : textSecondary}; font-family:monospace;">{proc.id}</span>
					</label>
				{/each}
			</div>

			<div class="mb-4 flex">
				<button
					onclick={toggleAllRestart}
					class="rounded-lg px-3 py-1.5"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:12px; cursor:pointer;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; }}
				>{allRestartSelected() ? 'Deselect all' : 'Select all'}</button>
			</div>

			{#if restartError}
				<div style="font-size:12px; color:{colorError}; margin-bottom:12px;">{restartError}</div>
			{/if}

			<div class="flex justify-end gap-2">
				<button
					onclick={() => { showRestartDialog = false; restartError = ''; }}
					class="rounded-lg px-4 py-2"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:13px; cursor:pointer;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; }}
				>Cancel</button>
				<button
					onclick={confirmRestart}
					disabled={restarting || !ALL_PROCESSOR_IDS.some(p => restartProcessors[p])}
					class="flex items-center gap-2 rounded-lg px-4 py-2"
					style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none;
					       cursor:{restarting ? 'not-allowed' : 'pointer'};
					       opacity:{restarting || !ALL_PROCESSOR_IDS.some(p => restartProcessors[p]) ? '0.6' : '1'};"
					onmouseenter={(e) => {
						if (!restarting) (e.currentTarget as HTMLElement).style.opacity = '0.88';
					}}
					onmouseleave={(e) => {
						(e.currentTarget as HTMLElement).style.opacity = restarting ? '0.6' : '1';
					}}
				>
					<RefreshCwIcon class="h-4 w-4" />
					{restarting ? 'Restarting…' : 'Restart'}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	@keyframes pulse {
		0%, 100% { opacity: 1; transform: scale(1); }
		50%       { opacity: 0.5; transform: scale(0.85); }
	}
	@keyframes pulse-ring {
		0%, 100% { box-shadow: 0 0 0 0 rgba(129, 140, 248, 0.4); }
		50%       { box-shadow: 0 0 0 4px rgba(129, 140, 248, 0); }
	}
	input[type='text'] {
		background: transparent;
		border: none;
		outline: none;
	}
	input[type='text']::placeholder {
		color: #64748b;
	}
</style>
