<script lang="ts">
	import { onMount, tick } from 'svelte';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
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
	import { listDocProcLogs, listKbInputs, getKbFrontendConfig, stopKbInput, type KbInputRecord, type ParseState } from '$lib/services/kbService';
	import KbInputSearchDialog from '$lib/components/home3/kb-input-search-dialog.svelte';
	import {
		ALL_CONFIGURABLE_PROCESSOR_IDS,
		ALL_PROCESSOR_IDS,
		MANDATORY_DISPLAY_STAGES,
		MANDATORY_PROCESSOR_IDS,
		PIPELINE_STAGES,
		buildManualLaunchOperations,
		computeStages,
		entityExtractionSucceeded,
		isActiveRecord,
		visibleStages,
		type StageInfo,
		type StageStatus,
		type StatusEntry
	} from './doc-processor-dashboard-state';

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

	// ── Config ────────────────────────────────────────────────────────────

	// requiredProcessors: the configurable processors that are enabled in config.toml.
	// Populated once on mount from GET /api/v1/kb/config.
	let requiredProcessors = $state<string[]>(ALL_CONFIGURABLE_PROCESSOR_IDS);

	let selectableProcessorIds = $derived([...MANDATORY_PROCESSOR_IDS, ...requiredProcessors]);

	// Show Phase A processors plus processors present in requiredProcessors (from config).
	let CONFIGURABLE_PROCESSORS = $derived(selectableProcessorIds
		.map(id => PIPELINE_STAGES.find(s => s.id === id))
		.filter(s => s !== undefined));

	// configuredProcessorIds: mandatory + required — the set isActiveRecord checks.
	let configuredProcessorIds = $derived([...MANDATORY_PROCESSOR_IDS, ...requiredProcessors]);
	let activePipelineLimit = $state(10);

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
		progress?: string;
		progressLoading?: boolean;
		x: number;
		y: number;
	} | null>(null);
	let stageProgressCache = $state<Record<string, string | null>>({});

	// ── Sync control state ────────────────────────────────────────────────
	let autoSync = $state(true);
	let syncInterval: ReturnType<typeof setInterval> | null = null;
	let pdfPollInterval: ReturnType<typeof setInterval> | null = null;
	let emptyPollCount = 0;
	let prevActivePipelinesCount = 0;

	// ── PDF Parsing monitor state ──────────────────────────────────────────

	const PDF_ACTIVE_LIMIT = 20;
	let pdfActiveParsing = $state<KbInputRecord[]>([]);
	let pdfParsingLoading = $state(true);
	let pdfParsingError = $state('');
	let pdfStats = $state({ total: 0, success: 0, failed: 0, active: 0, waiting: 0 });

	// Pie chart segments (parsed-success / parsed-failed / active / waiting)
	let pieData = $derived([
		{ key: 'success', label: 'Parsed Success', value: pdfStats.success, color: colorSuccess },
		{ key: 'failed', label: 'Parsed Failed', value: pdfStats.failed, color: colorError },
		{ key: 'active', label: 'Active', value: pdfStats.active, color: accent },
		{ key: 'waiting', label: 'Waiting', value: pdfStats.waiting, color: textMuted }
	]);
	let pieTotal = $derived(pieData.reduce((sum, d) => sum + d.value, 0));

	// Donut geometry — circumference of an r=42 circle.
	const DONUT_C = 2 * Math.PI * 42;
	let pieSegments = $derived.by(() => {
		const total = pieTotal;
		let acc = 0;
		return pieData.map((d) => {
			const frac = total > 0 ? d.value / total : 0;
			const seg = { ...d, frac, dash: frac * DONUT_C, offset: -acc * DONUT_C };
			acc += frac;
			return seg;
		});
	});

	// Bar chart series (new records / parsed-success / parsed-failed / wait)
	let barData = $derived([
		{ label: 'New Records', value: pdfStats.total, color: accent },
		{ label: 'Parsed Success', value: pdfStats.success, color: colorSuccess },
		{ label: 'Parsed Failed', value: pdfStats.failed, color: colorError },
		{ label: 'Wait', value: pdfStats.waiting, color: textMuted }
	]);
	let barMax = $derived(Math.max(1, ...barData.map((b) => b.value)));

	// Read a 0–100 percentage from a parsing entry's free-form progress string
	// ("45%", "3/10", …). Returns null when no numeric progress is available.
	// The PDF parser writes operation="parsed" (proc_status: active → success/fail).
	// Match any of the canonical parse operation names so progress is found.
	function findParseEntry(record: KbInputRecord) {
		const PARSE_OPS = new Set(['parsed', 'parsing', 'parse']);
		return (record.status ?? []).find((e) => PARSE_OPS.has((e.operation ?? '').toLowerCase()));
	}

	function parseProgressPercent(record: KbInputRecord): number | null {
		const entry = findParseEntry(record);
		const raw = entry?.progress?.trim();
		if (!raw) return null;
		const pct = raw.match(/(\d+(?:\.\d+)?)\s*%/);
		if (pct) {
			const value = Math.max(0, Math.min(100, parseFloat(pct[1])));
			// 0% while actively parsing means no real page progress yet —
			// return null so the indeterminate animation plays instead of a flat bar.
			const isActive = (entry?.proc_status ?? (entry as Record<string, unknown>)?.['proc-status'] ?? '').toString().toLowerCase() === 'active';
			if (value === 0 && isActive) return null;
			return value;
		}
		const ratio = raw.match(/(\d+)\s*\/\s*(\d+)/);
		if (ratio) {
			const a = parseFloat(ratio[1]);
			const b = parseFloat(ratio[2]);
			if (b > 0) return Math.max(0, Math.min(100, (a / b) * 100));
		}
		return null;
	}

	function parsingProgressText(record: KbInputRecord): string {
		const entry = findParseEntry(record);
		const raw = entry?.progress?.trim() ?? '';
		const isActive = (entry?.proc_status ?? (entry as Record<string, unknown>)?.['proc-status'] ?? '').toString().toLowerCase() === 'active';
		if (raw === '0%' && isActive) {
			// Show server-side elapsed time (ms_used advances via heartbeat every 15 s).
			const msUsed = (entry as Record<string, unknown>)?.ms_used;
			if (typeof msUsed === 'number' && msUsed > 0) {
				const secs = Math.floor(msUsed / 1000);
				if (secs < 60) return `${secs}s`;
				const mins = Math.floor(secs / 60);
				const remSecs = secs % 60;
				return remSecs > 0 ? `${mins}m ${remSecs}s` : `${mins}m`;
			}
			return ''; // template falls through to 'parsing…'
		}
		return raw;
	}

	async function pollPdfParsing() {
		const base = {
			docType: 'all',
			parseState: 'all' as ParseState,
			fileName: '',
			startTime: '',
			endTime: '',
			page: 1,
			pageSize: 1
		};
		try {
			const [activeRes, successRes, failedRes, waitingRes, totalRes] = await Promise.all([
				listKbInputs({ ...base, pageSize: PDF_ACTIVE_LIMIT, parseState: 'parsing' }),
				listKbInputs({ ...base, parseState: 'parsed_success' }),
				listKbInputs({ ...base, parseState: 'parsed_failed' }),
				listKbInputs({ ...base, parseState: 'pending' }),
				listKbInputs({ ...base })
			]);
			pdfActiveParsing = (activeRes.results ?? []).filter(
				(r) => !r.file_name?.toLowerCase().endsWith('.zip')
			);
			pdfStats = {
				total: totalRes.total ?? 0,
				success: successRes.total ?? 0,
				failed: failedRes.total ?? 0,
				active: activeRes.total ?? 0,
				waiting: waitingRes.total ?? 0
			};
			pdfParsingError = '';
		} catch (err) {
			pdfParsingError = err instanceof Error ? err.message : 'Failed to load PDF parsing status';
		} finally {
			pdfParsingLoading = false;
		}
		// If active parses detected while pipeline sync is paused, wake it back up.
		if (pdfStats.active > 0 && !autoSync) {
			startAutoSync();
		}
		// Stop the dedicated PDF keepalive once there's nothing left to watch.
		if (pdfStats.active === 0 && pdfPollInterval !== null) {
			clearInterval(pdfPollInterval);
			pdfPollInterval = null;
		}
	}

	// ── Manual launch state ────────────────────────────────────────────────

	let searchDialogOpen = $state(false);
	let selectedRecords = $state<KbInputRecord[]>([]);
	let processors = $state<Record<string, boolean>>(
		Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, true]))
	);
	let parseFileChecked = $state(false);
	let convertChecked = $state(false);
	let showConfirm = $state(false);
	let launching = $state(false);
	let launchError = $state('');
	let launchToast = $state<{ kind: 'success' | 'error'; msg: string } | null>(null);
	type RunMode = 'unfinished' | 'failed' | 'unfinished_failed' | 'force';
	let runMode = $state<RunMode>('failed');
	let maxRecords = $state<number>(5);
	let noEligibleDialog = $state<{ recordCount: number; modeLabel: string; fromSearch?: boolean } | null>(null);
	let searchingRecords = $state(false);

	$effect(() => {
		const len = selectedRecords.length;
		if (len > 0) {
			maxRecords = len;
		} else if (runMode === 'force') {
			runMode = 'failed';
		}
	});

	// ── Stop state ─────────────────────────────────────────────────────────

	let stoppingIds = $state<Set<number>>(new Set());

	async function doStop(record: KbInputRecord) {
		if (stoppingIds.has(record.id)) return;
		stoppingIds = new Set([...stoppingIds, record.id]);
		try {
			await stopKbInput(record.id);
			launchToast = { kind: 'success', msg: `Stop requested for record #${record.id}` };
			setTimeout(() => { launchToast = null; }, 4000);
		} catch (err) {
			launchToast = { kind: 'error', msg: err instanceof Error ? err.message : 'Stop failed' };
			setTimeout(() => { launchToast = null; }, 4000);
		} finally {
			stoppingIds = new Set([...stoppingIds].filter((id) => id !== record.id));
		}
	}

	// ── Detail dialog state ────────────────────────────────────────────────

	let detailRecord = $state<KbInputRecord | null>(null);
	let detailRawJsonEl = $state<HTMLElement | null>(null);
	let detailHasOverflow = $state(false);

	async function openDetail(record: KbInputRecord) {
		detailRecord = record;
		detailHasOverflow = false;
		await tick();
		if (detailRawJsonEl) {
			detailHasOverflow = detailRawJsonEl.scrollHeight > detailRawJsonEl.clientHeight;
		}
	}

	function closeDetail() {
		detailRecord = null;
	}

	// ── Restart dialog state ───────────────────────────────────────────────

	let restartTarget = $state<KbInputRecord | null>(null);
	let restartProcessors = $state<Record<string, boolean>>(
		Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, true]))
	);
	let restartParseFile = $state(false);
	let restartConvert = $state(false);
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
		// Cancel the PDF-only keepalive — full sync covers it now.
		if (pdfPollInterval !== null) {
			clearInterval(pdfPollInterval);
			pdfPollInterval = null;
		}
	}

	function stopAutoSync() {
		if (syncInterval !== null) {
			clearInterval(syncInterval);
			syncInterval = null;
		}
		autoSync = false;
		// Keep the PDF section live with a dedicated interval so active parses
		// continue to update (and can wake full sync back up) even while paused.
		if (pdfPollInterval === null) {
			pdfPollInterval = setInterval(pollPdfParsing, 5000);
		}
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
				pageSize: activePipelineLimit,
				operation: 'doc_processing',
				procStatus: 'running'
			});
			const ids = configuredProcessorIds;
			const newPipelines = (res.results ?? [])
				.filter(r => !r.file_name?.toLowerCase().endsWith('.zip'))
				.filter(r => isActiveRecord(r, ids))
				.slice(0, activePipelineLimit);
			const newCount = newPipelines.length;
			activePipelines = newPipelines;
			prevActivePipelinesCount = newCount;
			lastPoll = new Date().toLocaleTimeString();
			pipelinesError = '';

			// Turn sync back on when pipelines or active parses appear while sync was off
			if (!autoSync && (newCount > 0 || pdfStats.active > 0)) {
				startAutoSync();
			}
			// Track consecutive empty polls; disable sync after 5 with no active tasks.
			// Keep syncing as long as PDF parsing is active (pdfStats from previous poll).
			if (newCount === 0 && pdfStats.active === 0) {
				emptyPollCount++;
				if (emptyPollCount >= 5 && autoSync) stopAutoSync();
			} else {
				emptyPollCount = 0;
			}
		} catch (err) {
			pipelinesError = err instanceof Error ? err.message : 'Failed to load pipelines';
		} finally {
			pipelinesLoading = false;
		}
		void pollPdfParsing();
	}

	async function publishEvent(subject: string, payload: Record<string, unknown>) {
		const res = await fetch('/api/v1/jetstream/events', {
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ subject, payload: JSON.stringify(payload) })
		});
		if (!res.ok) {
			const body = await res.json().catch(() => null);
			throw new Error(body?.error_msg ?? body?.message ?? `Request failed (${res.status})`);
		}
	}

	async function doLaunch(
		record: KbInputRecord,
		procs: Record<string, boolean>,
		reParse: boolean,
		reConvert: boolean
	) {
		// Re-parse triggers the whole downstream chain automatically (in auto mode).
		// Re-convert triggers convert + downstream doc processing.
		// Both supersede a standalone doc-processor publish to avoid double-processing.
		if (reParse) {
			await publishEvent('kb.pdf.staged', { record_id: String(record.id), type: record.type ?? 'pdf', status: 'success', force: true });
			return;
		}
		if (reConvert) {
			await publishEvent('kb.pdf.parsed', { record_id: String(record.id), type: 'pdf', status: 'success', force: true });
			return;
		}
		// extract_relation links its endpoints against extract_entity's entities. If
		// entities don't already exist for this record, force extract_entity to run too
		// (ADR 2026061702). Done per record so a record that already has entities is not
		// re-run unnecessarily.
		const chosen = buildManualLaunchOperations(selectableProcessorIds, procs, entityExtractionSucceeded(record));
		const payload: Record<string, unknown> = { record_id: String(record.id), force: runMode === 'force' };
		payload.operation = chosen;
		await publishEvent('kb.line-file-generated', payload);
	}

	function filterByRunMode(records: KbInputRecord[], mode: RunMode, selectedStageIds: Set<string>): KbInputRecord[] {
		if (mode === 'force') return records;
		return records.filter((rec) => {
			const relevantStages = computeStages(rec).filter((s) => selectedStageIds.has(s.id));
			return relevantStages.some((s) => {
				if (mode === 'unfinished') return s.status === 'pending' || s.status === 'in-progress';
				if (mode === 'failed') return s.status === 'failed';
				return s.status === 'pending' || s.status === 'in-progress' || s.status === 'failed';
			});
		});
	}

	async function searchAndLaunch() {
		searchingRecords = true;
		launchError = '';
		try {
			const base = {
				docType: 'all',
				parseState: 'all' as ParseState,
				fileName: '',
				startTime: '',
				endTime: '',
				page: 1
			};
			const fetchSize = Math.max(maxRecords * 3, 30);
			let fetchedRecords: KbInputRecord[] = [];

			if (runMode === 'failed') {
				const res = await listKbInputs({ ...base, pageSize: fetchSize, procStatus: 'failed' });
				fetchedRecords = (res.results ?? []).filter(r => !r.file_name?.toLowerCase().endsWith('.zip'));
			} else {
				// 'unfinished' and 'unfinished_failed': use with_unfinished_procs which
				// finds all records where any processor lacks a success entry (pending,
				// in-progress, or failed). filterByRunMode then narrows to the exact
				// statuses the mode requires. procStatus:'running' was wrong — it missed
				// records that are parsed+converted but never had doc processors run.
				const res = await listKbInputs({ ...base, pageSize: fetchSize, pipelineFilter: 'with_unfinished_procs' });
				fetchedRecords = (res.results ?? []).filter(r => !r.file_name?.toLowerCase().endsWith('.zip'));
			}

			const selectedStageIds = new Set<string>([
				...(parseFileChecked ? ['parsing'] : []),
				...(convertChecked ? ['converting'] : []),
				...selectableProcessorIds.filter(p => processors[p])
			]);

			const eligible = filterByRunMode(fetchedRecords, runMode, selectedStageIds).slice(0, maxRecords);

			if (eligible.length === 0) {
				const modeLabel = runMode === 'unfinished' ? 'unfinished'
					: runMode === 'failed' ? 'failed'
					: 'unfinished or failed';
				noEligibleDialog = { recordCount: fetchedRecords.length, modeLabel, fromSearch: true };
				return;
			}

			selectedRecords = eligible;
			showConfirm = true;
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'Search failed';
		} finally {
			searchingRecords = false;
		}
	}

	async function handleLaunchClick() {
		if (selectedRecords.length === 0) {
			await searchAndLaunch();
		} else {
			launchError = '';
			showConfirm = true;
		}
	}

	async function confirmLaunch() {
		if (!selectedRecords.length) return;
		launching = true;
		launchError = '';
		try {
			const selectedStageIds = new Set<string>([
				...(parseFileChecked ? ['parsing'] : []),
				...(convertChecked ? ['converting'] : []),
				...selectableProcessorIds.filter((p) => processors[p])
			]);
			const eligible = filterByRunMode(selectedRecords, runMode, selectedStageIds).slice(0, maxRecords);
			showConfirm = false;
			if (eligible.length === 0) {
				const modeLabel = runMode === 'unfinished' ? 'unfinished'
					: runMode === 'failed' ? 'failed'
					: 'unfinished or failed';
				noEligibleDialog = { recordCount: selectedRecords.length, modeLabel };
				return;
			}
			const results = await Promise.allSettled(
				eligible.map((rec) => doLaunch(rec, processors, parseFileChecked, convertChecked))
			);
			const failures = results.filter((r) => r.status === 'rejected') as PromiseRejectedResult[];
			if (failures.length === 0) {
				const n = eligible.length;
				if (!autoSync) startAutoSync();
				launchToast = { kind: 'success', msg: `Launched ${n} record${n !== 1 ? 's' : ''}` };
			} else {
				const firstMsg = failures[0].reason instanceof Error ? failures[0].reason.message : 'unknown error';
				launchToast = { kind: 'error', msg: `${failures.length}/${eligible.length} failed: ${firstMsg}` };
			}
			setTimeout(() => { launchToast = null; }, 4000);
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
			await doLaunch(restartTarget, restartProcessors, restartParseFile, restartConvert);
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
		restartParseFile = false;
		restartConvert = false;
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
			return Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, selectableProcessorIds.includes(p)]));
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
		return selectableProcessorIds.every((p) => processors[p]);
	}

	function someProcessorsSelected(): boolean {
		return parseFileChecked || convertChecked || selectableProcessorIds.some((p) => processors[p]);
	}

	function toggleAll() {
		const next = !allProcessorsSelected();
		if (!next) {
			// Deselect all: also clear the optional pre-processors.
			parseFileChecked = false;
			convertChecked = false;
		}
		processors = Object.fromEntries(selectableProcessorIds.map((p) => [p, next]));
	}

	function allRestartSelected(): boolean {
		return selectableProcessorIds.every((p) => restartProcessors[p]);
	}

	function selectFailedProcessors() {
		const failedStageIds = new Set<string>();
		for (const rec of selectedRecords) {
			for (const stage of computeStages(rec)) {
				if (stage.status === 'failed') failedStageIds.add(stage.id);
			}
		}
		if (failedStageIds.size === 0) return;
		// 'parsing' → parseFileChecked; 'converting' → convertChecked (parse takes priority).
		// If parse failed, launching parse_file re-chains the full downstream automatically.
		const hasParseFailed = failedStageIds.has('parsing');
		const hasConvertFailed = failedStageIds.has('converting');
		parseFileChecked = hasParseFailed;
		convertChecked = !hasParseFailed && hasConvertFailed;
		processors = Object.fromEntries(
			ALL_PROCESSOR_IDS.map((p) => [p, !hasParseFailed && !hasConvertFailed && failedStageIds.has(p)])
		);
	}

	function selectIncompletedProcessors() {
		const incompleteStageIds = new Set<string>();
		for (const rec of selectedRecords) {
			for (const stage of computeStages(rec)) {
				if (stage.status !== 'success') incompleteStageIds.add(stage.id);
			}
		}
		if (incompleteStageIds.size === 0) return;
		const hasParseIncomplete = incompleteStageIds.has('parsing');
		const hasConvertIncomplete = incompleteStageIds.has('converting');
		parseFileChecked = hasParseIncomplete;
		convertChecked = !hasParseIncomplete && hasConvertIncomplete;
		processors = Object.fromEntries(
			ALL_PROCESSOR_IDS.map((p) => [p, !hasParseIncomplete && !hasConvertIncomplete && incompleteStageIds.has(p)])
		);
	}

	function toggleAllRestart() {
		const next = !allRestartSelected();
		restartProcessors = Object.fromEntries(selectableProcessorIds.map((p) => [p, next]));
	}

	function showTooltip(
		e: MouseEvent,
		recordId: number,
		stage: StageInfo
	) {
		const el = e.currentTarget as HTMLElement;
		const rect = el.getBoundingClientRect();
		const cacheKey = `${recordId}:${stage.id}`;
		const cachedProgress = stageProgressCache[cacheKey];
		tooltipState = {
			recordId,
			stageId: stage.id,
			label: stage.label,
			status: stage.status,
			entry: stage.entry,
			progress: stage.entry?.progress ?? cachedProgress ?? undefined,
			progressLoading: stage.status === 'in-progress' && !stage.entry?.progress && cachedProgress === undefined,
			x: rect.left + rect.width / 2,
			y: rect.top
		};
		if (stage.status === 'in-progress' && !stage.entry?.progress && cachedProgress === undefined) {
			void hydrateTooltipProgress(recordId, stage.id);
		}
	}

	function hideTooltip() {
		tooltipState = null;
	}

	async function hydrateTooltipProgress(recordId: number, stageId: string) {
		const cacheKey = `${recordId}:${stageId}`;
		try {
			const res = await listDocProcLogs({
				docProcName: stageId,
				recordId,
				page: 1,
				pageSize: 10,
				orderBy: 'create_time',
				orderDir: 'desc'
			});
			const progress = res.results?.find((row) => row.proc_progress?.trim())?.proc_progress?.trim() || null;
			stageProgressCache = { ...stageProgressCache, [cacheKey]: progress };
			if (tooltipState && tooltipState.recordId === recordId && tooltipState.stageId === stageId) {
				tooltipState = {
					...tooltipState,
					progress: progress ?? undefined,
					progressLoading: false
				};
			}
		} catch {
			stageProgressCache = { ...stageProgressCache, [cacheKey]: null };
			if (tooltipState && tooltipState.recordId === recordId && tooltipState.stageId === stageId) {
				tooltipState = {
					...tooltipState,
					progressLoading: false
				};
			}
		}
	}

	onMount(() => {
		getKbFrontendConfig().then((cfg) => {
			const req = cfg.required_processors ?? [];
			requiredProcessors = req;
			activePipelineLimit = Math.max(1, cfg.max_doc_process_pipelines ?? 10);
			processors = Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, MANDATORY_PROCESSOR_IDS.includes(p) || req.includes(p)]));
		}).catch(() => {
			// Keep defaults on failure
		});

		pollPipelines();
		syncInterval = setInterval(pollPipelines, 5000);
		return () => {
			if (syncInterval !== null) clearInterval(syncInterval);
			if (pdfPollInterval !== null) clearInterval(pdfPollInterval);
		};
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
				{#if tooltipState.progress}
					<div style="font-size:12px; color:{textSecondary};">Progress: {tooltipState.progress}</div>
				{:else if tooltipState.progressLoading}
					<div style="font-size:12px; color:{textMuted};">Loading progress…</div>
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
					{@const stages = visibleStages(computeStages(record), requiredProcessors)}
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
								<!-- Detail button -->
								<button
									onclick={() => openDetail(record)}
									title="View status details"
									class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
									style="background:{surface2}; border:1px solid {borderColor}; color:{textMuted}; font-size:12px; cursor:pointer;"
									onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accentTint; (e.currentTarget as HTMLElement).style.color = accent; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; (e.currentTarget as HTMLElement).style.color = textMuted; }}
								>
									<FileTextIcon class="h-3 w-3" />
									Detail
								</button>
								<!-- Stop button -->
								<button
									onclick={() => doStop(record)}
									disabled={stoppingIds.has(record.id)}
									title="Request pipeline stop"
									class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
									style="background:{surface2}; border:1px solid {borderColor}; color:{textMuted}; font-size:12px; cursor:{stoppingIds.has(record.id) ? 'not-allowed' : 'pointer'}; opacity:{stoppingIds.has(record.id) ? 0.5 : 1};"
									onmouseenter={(e) => { if (!stoppingIds.has(record.id)) (e.currentTarget as HTMLElement).style.background = colorErrorTint; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; }}
								>
									<SquareIcon class="h-3 w-3" />
									{stoppingIds.has(record.id) ? 'Stopping…' : 'Stop'}
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
	     Section 1b — PDF Parsing Status
	══════════════════════════════════════════════════════════ -->
	<section class="p-6">

		<!-- Section header -->
		<div class="mb-4 flex items-start justify-between">
			<div>
				<h2 style="font-size:15px; font-weight:600; color:{textPrimary}; margin:0 0 3px;">PDF Parsing</h2>
				<p style="font-size:12px; color:{textMuted}; margin:0;">Live PDF parse status, throughput and queue health</p>
			</div>
			<button
				onclick={pollPdfParsing}
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

		{#if pdfParsingError}
			<div
				class="mb-4 flex items-center gap-3 rounded-xl p-4"
				style="background:{colorErrorTint}; border:1px solid {colorError}30; color:{colorError}; font-size:13px;"
			>
				<AlertCircleIcon class="h-4 w-4 flex-shrink-0" />
				{pdfParsingError}
			</div>
		{/if}

		<!-- Charts -->
		<div class="mb-5 grid gap-4" style="grid-template-columns: minmax(0,1fr) minmax(0,1fr);">

			<!-- Pie / donut chart -->
			<div class="rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
				<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em; margin-bottom:12px;">Parse State Distribution</div>
				<div class="flex items-center gap-5">
					<div class="relative flex-shrink-0" style="width:120px; height:120px;">
						<svg viewBox="0 0 120 120" style="width:120px; height:120px; transform:rotate(-90deg);">
							<circle cx="60" cy="60" r="42" fill="none" stroke={surface2} stroke-width="18" />
							{#if pieTotal > 0}
								{#each pieSegments as seg (seg.key)}
									<circle
										cx="60" cy="60" r="42" fill="none"
										stroke={seg.color} stroke-width="18"
										stroke-dasharray="{seg.dash} {DONUT_C - seg.dash}"
										stroke-dashoffset={seg.offset}
									/>
								{/each}
							{/if}
						</svg>
						<div class="absolute inset-0 flex flex-col items-center justify-center">
							<span style="font-size:20px; font-weight:700; color:{textPrimary}; line-height:1;">{pieTotal}</span>
							<span style="font-size:10px; color:{textMuted}; text-transform:uppercase; letter-spacing:0.06em;">total</span>
						</div>
					</div>
					<div class="flex-1 space-y-1.5">
						{#each pieData as d (d.key)}
							<div class="flex items-center justify-between gap-2">
								<div class="flex items-center gap-2 min-w-0">
									<span style="width:9px; height:9px; border-radius:2px; background:{d.color}; flex-shrink:0;"></span>
									<span style="font-size:12px; color:{textSecondary};" class="truncate">{d.label}</span>
								</div>
								<div class="flex items-center gap-2 flex-shrink-0">
									<span style="font-size:12px; font-weight:600; color:{textPrimary}; font-variant-numeric:tabular-nums;">{d.value}</span>
									<span style="font-size:11px; color:{textMuted}; font-variant-numeric:tabular-nums; min-width:34px; text-align:right;">
										{pieTotal > 0 ? Math.round((d.value / pieTotal) * 100) : 0}%
									</span>
								</div>
							</div>
						{/each}
					</div>
				</div>
			</div>

			<!-- Bar chart -->
			<div class="rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
				<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em; margin-bottom:12px;">Throughput</div>
				<div class="flex items-end justify-around gap-3" style="height:120px;">
					{#each barData as b (b.label)}
						<div class="flex h-full flex-1 flex-col items-center justify-end gap-1.5">
							<span style="font-size:12px; font-weight:600; color:{textPrimary}; font-variant-numeric:tabular-nums;">{b.value}</span>
							<div
								class="w-full rounded-t-md"
								style="height:{Math.max(2, (b.value / barMax) * 84)}px; max-width:48px; background:{b.color}; transition:height 0.3s ease;"
							></div>
							<span style="font-size:10px; color:{textMuted}; text-align:center; line-height:1.2; height:24px;">{b.label}</span>
						</div>
					{/each}
				</div>
			</div>
		</div>

		<!-- Active PDF parsing list -->
		<div class="mb-2" style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em;">
			Active PDF Parsing
			<span style="color:{accent}; margin-left:4px;">({pdfStats.active})</span>
		</div>

		{#if pdfParsingLoading}
			<div class="space-y-2">
				{#each Array(2) as _, i}
					<div class="rounded-xl" style="height:64px; background:{surface2}; border:1px solid {borderColor}; opacity:{1 - i * 0.3};"></div>
				{/each}
			</div>
		{:else if pdfActiveParsing.length === 0}
			<div
				class="flex flex-col items-center rounded-xl py-10"
				style="background:{surface2}; border:1px solid {borderColor};"
			>
				<FileTextIcon class="mb-3 h-8 w-8" style="color:{textMuted}; opacity:0.4;" />
				<p style="font-size:13px; font-weight:500; color:{textSecondary}; margin:0 0 4px;">No PDFs parsing right now</p>
				<p style="font-size:12px; color:{textMuted}; margin:0;">Records currently in the PDF parser appear here.</p>
			</div>
		{:else}
			<div class="space-y-2">
				{#each pdfActiveParsing as record (record.id)}
					{@const pct = parseProgressPercent(record)}
					{@const progressText = parsingProgressText(record)}
					<div
						class="rounded-xl p-3.5"
						style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 1px 3px rgba(0,0,0,0.20);"
					>
						<div class="flex items-center justify-between gap-3">
							<div class="flex items-center gap-2 min-w-0">
								<span
									style="font-family:monospace; font-size:10px; font-weight:600; color:{accent};
									       background:{accentTint}; border:1px solid {accent}30; border-radius:6px; padding:1px 6px; flex-shrink:0;"
								>#{record.id}</span>
								<span
									class="truncate"
									style="font-size:13px; font-weight:500; color:{textPrimary}; max-width:360px;"
									title={record.file_name ?? recordTitle(record)}
								>{record.file_name?.trim() || recordTitle(record)}</span>
							</div>
							<div class="flex flex-shrink-0 items-center gap-2">
								<!-- Detail -->
								<button
									onclick={() => openDetail(record)}
									title="View status details"
									class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
									style="background:{surface2}; border:1px solid {borderColor}; color:{textMuted}; font-size:12px; cursor:pointer;"
									onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accentTint; (e.currentTarget as HTMLElement).style.color = accent; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; (e.currentTarget as HTMLElement).style.color = textMuted; }}
								>
									<FileTextIcon class="h-3 w-3" />
									Detail
								</button>
								<!-- Restart -->
								<button
									onclick={() => openRestart(record)}
									title="Restart parsing"
									class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
									style="background:{accentTint}; border:1px solid {accent}30; color:{accent}; font-size:12px; font-weight:500; cursor:pointer;"
									onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accent + '25'; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = accentTint; }}
								>
									<PlayIcon class="h-3 w-3" />
									Restart
								</button>
								<!-- Abort -->
								<button
									onclick={() => doStop(record)}
									disabled={stoppingIds.has(record.id)}
									title="Abort parsing"
									class="flex items-center gap-1 rounded-lg px-2.5 py-1.5"
									style="background:{surface2}; border:1px solid {borderColor}; color:{textMuted}; font-size:12px; cursor:{stoppingIds.has(record.id) ? 'not-allowed' : 'pointer'}; opacity:{stoppingIds.has(record.id) ? 0.5 : 1};"
									onmouseenter={(e) => { if (!stoppingIds.has(record.id)) { (e.currentTarget as HTMLElement).style.background = colorErrorTint; (e.currentTarget as HTMLElement).style.color = colorError; } }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; (e.currentTarget as HTMLElement).style.color = textMuted; }}
								>
									<XCircleIcon class="h-3 w-3" />
									{stoppingIds.has(record.id) ? 'Aborting…' : 'Abort'}
								</button>
							</div>
						</div>

						<!-- Progress bar -->
						<div class="mt-3 flex items-center gap-3">
							<div
								class="relative flex-1 overflow-hidden rounded-full"
								style="height:7px; background:{surface2};"
							>
								{#if pct !== null}
									<div
										class="h-full rounded-full"
										style="width:{pct}%; background:{accent}; transition:width 0.3s ease;"
									></div>
								{:else}
									<!-- Indeterminate stripe while parsing with no numeric progress -->
									<div
										class="absolute inset-y-0 rounded-full"
										style="width:35%; background:{accent}; animation:pdf-parse-indeterminate 1.4s ease-in-out infinite;"
									></div>
								{/if}
							</div>
							<span style="font-size:11px; color:{textSecondary}; font-family:monospace; min-width:48px; text-align:right;">
								{pct !== null ? `${Math.round(pct)}%` : (progressText || 'parsing…')}
							</span>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<!-- ── Section divider ────────────────────────────────────── -->
	<div style="height:1px; background:{borderColor}; margin:0 24px;"></div>

	<!-- Search dialog -->
	<KbInputSearchDialog
		bind:open={searchDialogOpen}
		onSelect={(records) => { selectedRecords = records; }}
	/>

	<!-- ════════════════════════════════════════════════════════
	     Section 2 — Manual Launch
	══════════════════════════════════════════════════════════ -->
	<section class="p-6">

		<div class="mb-4 flex items-start justify-between gap-4">
			<div class="flex-shrink-0">
				<h2 style="font-size:15px; font-weight:600; color:{textPrimary}; margin:0 0 3px;">Manual Launch</h2>
				<p style="font-size:12px; color:{textMuted}; margin:0;">Select run mode and click Launch, or Search to pick specific records</p>
			</div>
			<div class="flex flex-col items-end gap-2">
				<!-- Run mode radio group + max records -->
				<div class="flex flex-wrap items-center gap-x-3 gap-y-1.5">
					{#each ([
						{ value: 'unfinished', label: 'Run Unfinished Only' },
						{ value: 'failed', label: 'Run Failed Only' },
						{ value: 'unfinished_failed', label: 'Run Unfinished & Failed' },
						{ value: 'force', label: 'Force Run' }
					] as const) as opt (opt.value)}
						{@const isForceDisabled = opt.value === 'force' && selectedRecords.length === 0}
						<label
							class="flex items-center gap-1.5"
							style="cursor:{isForceDisabled ? 'not-allowed' : 'pointer'}; font-size:12px; color:{runMode === opt.value ? accent : textSecondary}; user-select:none; opacity:{isForceDisabled ? 0.4 : 1};"
						>
							<input
								type="radio"
								name="runMode"
								value={opt.value}
								checked={runMode === opt.value}
								disabled={isForceDisabled}
								onchange={() => { runMode = opt.value; }}
								style="accent-color:{accent}; cursor:{isForceDisabled ? 'not-allowed' : 'pointer'};"
							/>
							{opt.label}
						</label>
					{/each}
					<div class="flex items-center gap-1.5" style="margin-left:8px;">
						<span style="font-size:12px; color:{textSecondary}; white-space:nowrap;">Max</span>
						<input
							type="number"
							min="1"
							bind:value={maxRecords}
							readonly={selectedRecords.length > 0}
							class="rounded-md px-2 py-1"
							style="width:52px; font-size:12px; background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; text-align:center;{selectedRecords.length > 0 ? ' opacity:0.7; cursor:default;' : ''}"
						/>
					</div>
				</div>
				<!-- Action buttons -->
				<div class="flex items-center gap-2">
					<button
						onclick={handleLaunchClick}
						disabled={!someProcessorsSelected() || maxRecords < 1 || isNaN(maxRecords) || searchingRecords}
						class="flex items-center gap-2 rounded-lg px-4 py-2"
						style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none; cursor:{someProcessorsSelected() && maxRecords >= 1 && !isNaN(maxRecords) && !searchingRecords ? 'pointer' : 'not-allowed'}; opacity:{someProcessorsSelected() && maxRecords >= 1 && !isNaN(maxRecords) && !searchingRecords ? '1' : '0.5'}; flex-shrink:0;"
						onmouseenter={(e) => { if (someProcessorsSelected() && maxRecords >= 1 && !isNaN(maxRecords) && !searchingRecords) (e.currentTarget as HTMLElement).style.opacity = '0.88'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = someProcessorsSelected() && maxRecords >= 1 && !isNaN(maxRecords) && !searchingRecords ? '1' : '0.5'; }}
					>
						<PlayIcon class="h-4 w-4" />
						{searchingRecords ? 'Searching…' : 'Launch'}
					</button>
					<button
						onclick={() => { searchDialogOpen = true; }}
						class="flex items-center gap-1.5 rounded-lg px-4 py-2"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:13px; font-weight:600; cursor:pointer; flex-shrink:0;"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; (e.currentTarget as HTMLElement).style.borderColor = accent + '60'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
					>
						<SearchIcon class="h-4 w-4" />
						Search
					</button>
				</div>
				{#if launchError}
					<div style="font-size:12px; color:{colorError}; text-align:right;">{launchError}</div>
				{/if}
			</div>
		</div>

		<!-- Selected records chip list -->
		{#if selectedRecords.length > 0}
			<div class="mb-4 rounded-xl p-3" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="mb-2 flex items-center justify-between">
					<span style="font-size:10px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em;">
						{selectedRecords.length} record{selectedRecords.length !== 1 ? 's' : ''} selected
					</span>
					<button
						onclick={() => { selectedRecords = []; }}
						style="font-size:11px; color:{textMuted}; background:none; border:none; cursor:pointer; padding:0;"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = colorError; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
					>Clear all</button>
				</div>
				<div class="flex flex-wrap gap-1.5">
					{#each selectedRecords as rec (rec.id)}
						<span
							class="flex items-center gap-1.5 rounded-lg px-2.5 py-1"
							style="background:{cardBg}; border:1px solid {borderColor}; font-size:12px; color:{textPrimary};"
						>
							<span style="font-family:monospace; color:{accent}; font-size:11px;">#{rec.id}</span>
							<span class="max-w-32 truncate" title={recordTitle(rec)}>{recordTitle(rec)}</span>
							<button
								onclick={() => { selectedRecords = selectedRecords.filter(r => r.id !== rec.id); }}
								style="background:none; border:none; cursor:pointer; color:{textMuted}; padding:0; line-height:1; font-size:14px; margin-left:2px;"
								onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = colorError; }}
								onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
								title="Remove"
							>×</button>
						</span>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Processor selection + launch (shown once records are selected) -->
		{#if selectedRecords.length > 0}
			<div
				class="rounded-xl p-4"
				style="background:{cardBg}; border:1px solid {borderColor};"
			>
				<div style="font-size:12px; color:{textMuted}; margin-bottom:10px; text-transform:uppercase; letter-spacing:0.08em; font-weight:600;">Processors to run</div>

				<div class="mb-4 space-y-1.5">
					<!-- Optional pre-processor: Parse File -->
					<label
						class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
						style="background:{parseFileChecked ? accentTint : surface2}; border:1px solid {parseFileChecked ? accent + '40' : borderColor};"
						onmouseenter={(e) => { if (!parseFileChecked) (e.currentTarget as HTMLElement).style.background = surface3; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = parseFileChecked ? accentTint : surface2; }}
					>
						<input type="checkbox" bind:checked={parseFileChecked} class="sr-only" />
						{#if parseFileChecked}
							<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
						{:else}
							<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
						{/if}
						<span style="font-size:13px; color:{parseFileChecked ? textPrimary : textSecondary}; font-family:monospace;">parse_file</span>
						<span style="font-size:12px; color:{textMuted}; margin-left:6px;">Parse File</span>
					</label>

					<!-- Optional pre-processor: Convert Parse Result -->
					<label
						class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
						style="background:{convertChecked ? accentTint : surface2}; border:1px solid {convertChecked ? accent + '40' : borderColor};"
						onmouseenter={(e) => { if (!convertChecked) (e.currentTarget as HTMLElement).style.background = surface3; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = convertChecked ? accentTint : surface2; }}
					>
						<input type="checkbox" bind:checked={convertChecked} class="sr-only" />
						{#if convertChecked}
							<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
						{:else}
							<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
						{/if}
						<span style="font-size:13px; color:{convertChecked ? textPrimary : textSecondary}; font-family:monospace;">convert_parse_result</span>
						<span style="font-size:12px; color:{textMuted}; margin-left:6px;">Convert Parse Result</span>
					</label>

					<!-- Mandatory (always-on) processors -->
					{#each MANDATORY_DISPLAY_STAGES as proc}
						<label
							class="flex cursor-not-allowed items-center gap-2.5 rounded-lg px-3 py-2"
							style="background:{surface2}; border:1px solid {borderColor}; opacity:0.6;"
						>
							<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{colorSuccess};" />
							<span style="font-size:13px; color:{textSecondary}; font-family:monospace;">{proc.id}</span>
							<span style="font-size:12px; color:{textMuted}; margin-left:6px;">{proc.label}</span>
							<span style="font-size:10px; color:{textMuted}; margin-left:auto; font-family:monospace;">mandatory</span>
						</label>
					{/each}

					<!-- Configurable processors -->
					{#each CONFIGURABLE_PROCESSORS as proc}
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

				<!-- Toggle all + action buttons -->
				<div class="flex items-center gap-2">
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
						onclick={selectFailedProcessors}
						class="rounded-lg px-3 py-1.5"
						style="background:{colorErrorTint}; border:1px solid {colorError}30; color:{colorError}; font-size:12px; cursor:pointer;"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = colorError + '60'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = colorError + '30'; }}
					>
						Select Failed
					</button>
					<button
						onclick={selectIncompletedProcessors}
						class="rounded-lg px-3 py-1.5"
						style="background:{accentTint}; border:1px solid {accent}30; color:{accent}; font-size:12px; cursor:pointer;"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent + '60'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent + '30'; }}
					>
						Select Incompleted
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
{#if showConfirm && selectedRecords.length > 0}
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
			<p style="font-size:13px; color:{textSecondary}; margin:0 0 12px; line-height:1.5;">
				Launch processing for {selectedRecords.length} record{selectedRecords.length !== 1 ? 's' : ''}:
			</p>
			<div class="mb-4 flex flex-wrap gap-1.5">
				{#each selectedRecords as rec (rec.id)}
					<span
						style="font-family:monospace; font-size:11px; padding:2px 8px; border-radius:999px;
						       background:{accentTint}; color:{accent}; border:1px solid {accent}30;"
						title={recordTitle(rec)}
					>#{rec.id}</span>
				{/each}
			</div>

			<!-- Processors summary -->
			<div
				class="mb-4 rounded-lg px-3 py-2.5"
				style="background:{surface2}; border:1px solid {borderColor}; font-size:12px; color:{textSecondary};"
			>
				<div style="font-size:10px; color:{textMuted}; text-transform:uppercase; letter-spacing:0.08em; font-weight:600; margin-bottom:6px;">Processors</div>
				<div class="flex flex-wrap gap-1.5">
					{#if parseFileChecked}
						<span style="font-family:monospace; background:{accentTint}; border:1px solid {accent}30; padding:1px 8px; border-radius:999px; font-size:11px; color:{accent};">parse_file</span>
					{/if}
					{#if convertChecked}
						<span style="font-family:monospace; background:{accentTint}; border:1px solid {accent}30; padding:1px 8px; border-radius:999px; font-size:11px; color:{accent};">convert_parse_result</span>
					{/if}
					{#each MANDATORY_DISPLAY_STAGES as stage}
						<span style="font-family:monospace; background:{surface3}; border:1px solid {borderColor}; padding:1px 8px; border-radius:999px; font-size:11px; color:{colorSuccess};">{stage.id}</span>
					{/each}
					{#each selectableProcessorIds.filter(p => processors[p]) as p}
						<span style="font-family:monospace; background:{accentTint}; border:1px solid {accent}30; padding:1px 8px; border-radius:999px; font-size:11px; color:{accent};">{p}</span>
					{/each}
				</div>
				{#if parseFileChecked}
					<div style="margin-top:6px; font-size:11px; color:{textMuted};">Triggers parse → convert → all doc processors (auto chain).</div>
				{:else if convertChecked}
					<div style="margin-top:6px; font-size:11px; color:{textMuted};">Triggers convert → all doc processors (auto chain).</div>
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
     No Eligible Records Dialog
══════════════════════════════════════════════════════════════ -->
{#if noEligibleDialog}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-40 flex items-center justify-center"
		style="background:rgba(0,0,0,0.6); backdrop-filter:blur(4px);"
		onmousedown={(e) => { if (e.target === e.currentTarget) noEligibleDialog = null; }}
	>
		<div
			class="mx-4 w-full max-w-sm rounded-2xl p-6"
			style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 24px 64px rgba(0,0,0,0.4);"
		>
			<div class="mb-3 flex items-center gap-2">
				<XCircleIcon class="h-5 w-5 flex-shrink-0" style="color:{colorError};" />
				<h3 style="font-size:16px; font-weight:600; color:{textPrimary}; margin:0;">Nothing to launch</h3>
			</div>
			<p style="font-size:13px; color:{textSecondary}; margin:0 0 20px; line-height:1.6;">
				{#if noEligibleDialog.fromSearch}
					No <strong style="color:{textPrimary};">{noEligibleDialog.modeLabel}</strong> records found to launch.
					Try a different run mode or use Search to pick records manually.
				{:else}
					{noEligibleDialog.recordCount === 1
						? 'The selected record has'
						: `All ${noEligibleDialog.recordCount} selected records have`}
					no <strong style="color:{textPrimary};">{noEligibleDialog.modeLabel}</strong> processors.
					Switch to <em>Force Run</em> to re-run regardless of status, or select a different run mode.
				{/if}
			</p>
			<div class="flex justify-end">
				<button
					onclick={() => { noEligibleDialog = null; }}
					class="rounded-lg px-5 py-2"
					style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none; cursor:pointer;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.88'; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
				>OK</button>
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
				<!-- Optional pre-processor: Parse File -->
				<label
					class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
					style="background:{restartParseFile ? accentTint : surface2}; border:1px solid {restartParseFile ? accent + '40' : borderColor};"
					onmouseenter={(e) => { if (!restartParseFile) (e.currentTarget as HTMLElement).style.background = surface3; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = restartParseFile ? accentTint : surface2; }}
				>
					<input type="checkbox" bind:checked={restartParseFile} class="sr-only" />
					{#if restartParseFile}
						<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
					{:else}
						<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
					{/if}
					<span style="font-size:13px; color:{restartParseFile ? textPrimary : textSecondary}; font-family:monospace;">parse_file</span>
					<span style="font-size:12px; color:{textMuted}; margin-left:6px;">Parse File</span>
				</label>

				<!-- Optional pre-processor: Convert Parse Result -->
				<label
					class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
					style="background:{restartConvert ? accentTint : surface2}; border:1px solid {restartConvert ? accent + '40' : borderColor};"
					onmouseenter={(e) => { if (!restartConvert) (e.currentTarget as HTMLElement).style.background = surface3; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = restartConvert ? accentTint : surface2; }}
				>
					<input type="checkbox" bind:checked={restartConvert} class="sr-only" />
					{#if restartConvert}
						<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
					{:else}
						<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
					{/if}
					<span style="font-size:13px; color:{restartConvert ? textPrimary : textSecondary}; font-family:monospace;">convert_parse_result</span>
					<span style="font-size:12px; color:{textMuted}; margin-left:6px;">Convert Parse Result</span>
				</label>

				<!-- Mandatory (always-on) processors -->
				{#each MANDATORY_DISPLAY_STAGES as proc}
					<label class="flex cursor-not-allowed items-center gap-2.5 rounded-lg px-3 py-2" style="background:{surface2}; border:1px solid {borderColor}; opacity:0.6;">
						<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{colorSuccess};" />
						<span style="font-size:13px; color:{textSecondary}; font-family:monospace;">{proc.id}</span>
						<span style="font-size:12px; color:{textMuted}; margin-left:6px;">{proc.label}</span>
						<span style="font-size:10px; color:{textMuted}; margin-left:auto; font-family:monospace;">mandatory</span>
					</label>
				{/each}

				<!-- Configurable processors -->
				{#each CONFIGURABLE_PROCESSORS as proc}
					<label
						class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
						style="background:{restartProcessors[proc.id] ? accentTint : surface2}; border:1px solid {restartProcessors[proc.id] ? accent + '40' : borderColor};"
						onmouseenter={(e) => { if (!restartProcessors[proc.id]) (e.currentTarget as HTMLElement).style.background = surface3; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = restartProcessors[proc.id] ? accentTint : surface2; }}
					>
						<input type="checkbox" bind:checked={restartProcessors[proc.id]} class="sr-only" />
						{#if restartProcessors[proc.id]}
							<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
						{:else}
							<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
						{/if}
						<span style="font-size:13px; color:{restartProcessors[proc.id] ? textPrimary : textSecondary}; font-family:monospace;">{proc.id}</span>
						<span style="font-size:12px; color:{textMuted}; margin-left:6px;">{proc.label}</span>
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
					disabled={restarting || !(restartParseFile || restartConvert || selectableProcessorIds.some(p => restartProcessors[p]))}
					class="flex items-center gap-2 rounded-lg px-4 py-2"
					style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none;
					       cursor:{restarting ? 'not-allowed' : 'pointer'};
					       opacity:{restarting || !(restartParseFile || restartConvert || selectableProcessorIds.some(p => restartProcessors[p])) ? '0.6' : '1'};"
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

<!-- ════════════════════════════════════════════════════════════
     Detail Dialog
══════════════════════════════════════════════════════════════ -->
{#if detailRecord}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center"
		style="background:rgba(0,0,0,0.65); backdrop-filter:blur(6px);"
		onmousedown={(e) => { if (e.target === e.currentTarget) closeDetail(); }}
	>
		<div
			class="mx-4 w-full max-w-4xl rounded-2xl overflow-hidden"
			style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 24px 64px rgba(0,0,0,0.45); max-height:90vh; display:flex; flex-direction:column;"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			tabindex="0"
		>
			<!-- Header -->
			<div class="flex items-center justify-between px-5 py-3.5 flex-shrink-0" style="border-bottom:1px solid {borderColor};">
				<div class="flex items-center gap-2">
					<span style="font-family:monospace; font-size:11px; font-weight:600; color:{accent}; background:{accentTint}; border:1px solid {accent}30; border-radius:6px; padding:1px 7px;">#{detailRecord.id}</span>
					<h3 style="font-size:14px; font-weight:600; color:{textPrimary}; margin:0;">{recordTitle(detailRecord)} — Status</h3>
				</div>
				<button
					onclick={closeDetail}
					style="height:30px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; }}
				>Close</button>
			</div>

			<!-- Body -->
			<div class="grid gap-4 p-5 overflow-auto flex-1" style="grid-template-columns: minmax(0,1fr) minmax(0,1fr); align-items:start;">
				<!-- Readable -->
				<div class="rounded-xl p-3" style="border:1px solid {borderColor}; background:{surface2};">
					<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.1em; margin-bottom:10px;">Readable</div>
					{#if !detailRecord.status?.length}
						<div style="font-size:12px; color:{textMuted};">No status entries.</div>
					{:else}
						<div class="space-y-2">
							{#each detailRecord.status as item, idx}
								{@const ps = (item.proc_status ?? item['proc-status'] ?? item.status ?? '').toLowerCase()}
								<div class="rounded-lg p-2.5" style="border:1px solid {borderColor}; background:{cardBg};">
									<div style="font-size:11px; font-weight:600; color:{textSecondary}; margin-bottom:7px; text-transform:uppercase; letter-spacing:0.08em;">
										Entry #{idx + 1}
									</div>
									<div class="grid gap-y-1 gap-x-3" style="grid-template-columns: 110px 1fr;">
										<span style="font-size:11px; color:{textMuted};">operation</span>
										<span style="font-size:12px; color:{textPrimary}; font-family:monospace;">{item.operation ?? '—'}</span>
										<span style="font-size:11px; color:{textMuted};">proc_status</span>
										<span style="font-size:12px; font-family:monospace; font-weight:600;
											color:{ps === 'success' ? colorSuccess : ps === 'failed' || ps === 'fail' ? colorError : textPrimary};">
											{item.proc_status ?? item['proc-status'] ?? '—'}
										</span>
										<span style="font-size:11px; color:{textMuted};">start_time</span>
										<span style="font-size:12px; color:{textPrimary}; font-family:monospace;">{item.start_time ?? '—'}</span>
										{#if item.doc_processor_name}
											<span style="font-size:11px; color:{textMuted};">processor</span>
											<span style="font-size:12px; color:{textPrimary}; font-family:monospace;">{item.doc_processor_name}</span>
										{/if}
										{#if item.error}
											<span style="font-size:11px; color:{textMuted};">error</span>
											<span style="font-size:12px; color:{colorError}; word-break:break-word;">{item.error}</span>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Raw JSON -->
				<div class="rounded-xl p-3 flex flex-col" style="border:1px solid {borderColor}; background:{surface2}; min-height:0;">
					<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.1em; margin-bottom:10px;">Raw JSON</div>
					<pre
						bind:this={detailRawJsonEl}
						style="margin:0; white-space:pre-wrap; word-break:break-word; font-size:12px; color:{textPrimary}; overflow-y:{detailHasOverflow ? 'auto' : 'visible'}; max-height:420px;"
					>{JSON.stringify(detailRecord.status ?? [], null, 2)}</pre>
				</div>
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
	@keyframes pdf-parse-indeterminate {
		0%   { left: -35%; }
		100% { left: 100%; }
	}
</style>
