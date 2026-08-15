<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listMetricRangeTypeErrors,
		listValueRangeTypeMapEntries,
		upsertValueRangeTypeMapEntry,
		isInvalidMapEntry,
		CANONICAL_BUCKET_OPTIONS,
		type MetricRangeTypeError,
		type ValueRangeTypeMapEntry
	} from './resolve-metric-range-types-client.js';
	import { getKbInput, getRawLines, type KbInputRecord, type RawLine } from '$lib/services/kbService';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CheckIcon from '@lucide/svelte/icons/check';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import AlertTriangleIcon from '@lucide/svelte/icons/alert-triangle';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (matches db-consistency-view.svelte / resolve-ambiguous-objects-view.svelte) ---
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let inputBg       = $derived(darkMode ? '#141824' : '#FFFFFF');
	let colorWarn     = $derived(darkMode ? '#FBBF24' : '#D97706');
	let colorError    = $derived(darkMode ? '#F87171' : '#DC2626');
	let colorOk       = $derived(darkMode ? '#34D399' : '#059669');

	// --- Left panel: search + results list ---
	let rows        = $state<MetricRangeTypeError[]>([]);
	let listLoading = $state(false);
	let listError   = $state('');

	let searchRecordId  = $state('');
	let searchDateFrom   = $state('');
	let searchDateTo     = $state('');
	let searchErrorType  = $state('');

	async function loadList() {
		listLoading = true;
		listError = '';
		try {
			const filters: Parameters<typeof listMetricRangeTypeErrors>[0] = {};
			if (searchRecordId.trim()) {
				const id = Number(searchRecordId.trim());
				if (Number.isFinite(id) && id > 0) filters.input_record_id = id;
			}
			if (searchDateFrom) filters.date_from = searchDateFrom;
			if (searchDateTo) filters.date_to = searchDateTo + 'T23:59:59';
			if (searchErrorType) filters.value_range_type = searchErrorType;
			const res = await listMetricRangeTypeErrors(filters);
			rows = res.results;
		} catch (e) {
			listError = e instanceof Error ? e.message : String(e);
		} finally {
			listLoading = false;
		}
	}

	function search() {
		loadList();
	}

	// --- Right panel: selection + Information Block + PDF Display ---
	let selectedId = $state<number | null>(null);
	let selected   = $derived.by(() => rows.find((r) => r.id === selectedId) ?? null);

	let currentInput = $state<KbInputRecord | null>(null);
	let rawLines      = $state<RawLine[]>([]);
	let docPage       = $state(1);
	let pdfZoom       = $state(0.6);
	let pdfNumPages   = $state(0);
	let highlightSelectionVersion = $state(0);
	let pdfLoading    = $state(false);
	let pdfError      = $state('');

	type NormalizedSpan = { page_number: number; line_number: number };

	// source_line_spans carries line numbers only ("90", "98:99"); page numbers
	// are resolved via the raw-line file, matching metric-mgmt-view.svelte's
	// PDF-highlight mechanism (design D7 -- duplicated here rather than shared,
	// see openspec/changes/resolve-metric-range-types/design.md).
	let lineNumToPage = $derived.by(() => {
		const map = new Map<number, number>();
		for (const ln of rawLines) {
			if (!map.has(ln.line_number)) map.set(ln.line_number, ln.page_number);
		}
		return map;
	});

	function normalizeSpans(row: MetricRangeTypeError | null): NormalizedSpan[] {
		if (!row) return [];
		const raw = row.source_line_spans;
		if (!Array.isArray(raw)) return [];
		const lineNums: number[] = [];
		for (const item of raw) {
			if (typeof item === 'string') {
				const s = item.trim();
				const mm = s.match(/^(\d+)\s*[:,-]\s*(\d+)$/);
				if (mm) {
					const start = parseInt(mm[1], 10);
					const end = parseInt(mm[2], 10);
					for (let n = start; n <= end && n <= start + 200; n++) lineNums.push(n);
				} else {
					const n = parseInt(s, 10);
					if (n > 0) lineNums.push(n);
				}
			} else if (typeof item === 'number' && item > 0) {
				lineNums.push(Math.trunc(item));
			}
		}
		const out: NormalizedSpan[] = [];
		for (const lineNo of lineNums) {
			const pageNo = lineNumToPage.get(lineNo);
			if (pageNo) out.push({ page_number: pageNo, line_number: lineNo });
		}
		return out;
	}

	let rawLineByKey = $derived.by(() => {
		const map = new Map<string, RawLine>();
		for (const ln of rawLines) map.set(`${ln.page_number}:${ln.line_number}`, ln);
		return map;
	});

	let selectedLinesByPage = $derived.by(() => {
		const map = new Map<number, RawLine[]>();
		for (const span of normalizeSpans(selected)) {
			const ln = rawLineByKey.get(`${span.page_number}:${span.line_number}`);
			if (ln && Array.isArray(ln.coords) && ln.coords.length >= 4) {
				const arr = map.get(span.page_number) ?? [];
				arr.push(ln);
				map.set(span.page_number, arr);
			}
		}
		return map;
	});

	function renderHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		const lines = selectedLinesByPage.get(pageNo) ?? [];
		const rects = lines.flatMap((ln) => {
			if (!Array.isArray(ln.coords) || ln.coords.length < 4) return [];
			const vx1 = (ln.coords[0] * viewport.width) / 1000;
			const vy1 = (ln.coords[1] * viewport.height) / 1000;
			const vx2 = (ln.coords[2] * viewport.width) / 1000;
			const vy2 = (ln.coords[3] * viewport.height) / 1000;
			return [
				{
					lineNumber: ln.line_number,
					left: Math.min(vx1, vx2),
					top: Math.max(0, Math.min(vy1, vy2) - 2),
					rawBottom: Math.max(vy1, vy2),
					width: Math.abs(vx2 - vx1) + 20
				}
			];
		});
		for (let i = 0; i < rects.length; i += 1) {
			const rect = rects[i];
			const nextRect = rects[i + 1];
			const isContiguous = nextRect && nextRect.lineNumber === rect.lineNumber + 1;
			const bottom = isContiguous ? nextRect.top : rect.rawBottom;
			const height = Math.max(0, bottom - rect.top);
			if (rect.width < 1 || height < 1) continue;
			const mark = document.createElement('div');
			mark.className = 'pdf-highlight';
			mark.style.left = `${rect.left}px`;
			mark.style.top = `${rect.top}px`;
			mark.style.width = `${rect.width}px`;
			mark.style.height = `${height}px`;
			mark.title = `line ${rect.lineNumber}`;
			overlay.appendChild(mark);
		}
	}

	let fileUrl = $derived.by(() =>
		currentInput ? `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width` : ''
	);
	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');

	async function selectRow(id: number) {
		selectedId = id;
		const row = rows.find((r) => r.id === id);
		if (!row) return;
		pdfLoading = true;
		pdfError = '';
		try {
			const [inputRes, linesRes] = await Promise.all([
				getKbInput(row.input_record_id),
				getRawLines(row.input_record_id)
			]);
			currentInput = inputRes.record;
			rawLines = linesRes.lines;
			const spans = normalizeSpans(row);
			if (spans.length > 0) docPage = spans[0].page_number;
			highlightSelectionVersion += 1;
		} catch (e) {
			pdfError = e instanceof Error ? e.message : String(e);
			currentInput = null;
			rawLines = [];
		} finally {
			pdfLoading = false;
		}
	}

	// --- Map Block: kb.metric_value_range_type_map governance ---
	let mapEntries  = $state<ValueRangeTypeMapEntry[]>([]);
	let mapLoading  = $state(false);
	let mapError    = $state('');

	let mapBucketDraft = $state<Record<string, string>>({});
	let mapApplying     = $state<Record<string, boolean>>({});
	let mapApplyResult  = $state<Record<string, string>>({});
	let mapApplyError   = $state<Record<string, string>>({});

	let newEntryRawValue = $state('');
	let newEntryBucket   = $state('');
	let addingEntry      = $state(false);
	let addEntryError    = $state('');
	let addEntryResult   = $state('');

	let errorTypeOptions = $derived.by(() =>
		[...new Set(mapEntries.filter(isInvalidMapEntry).map((e) => e.raw_value))].sort()
	);

	let sortedMapEntries = $derived.by(() =>
		[...mapEntries].sort((a, b) => {
			const aInvalid = isInvalidMapEntry(a) ? 0 : 1;
			const bInvalid = isInvalidMapEntry(b) ? 0 : 1;
			if (aInvalid !== bInvalid) return aInvalid - bInvalid;
			return b.occurrence_count - a.occurrence_count;
		})
	);

	async function loadMapEntries() {
		mapLoading = true;
		mapError = '';
		try {
			const res = await listValueRangeTypeMapEntries();
			mapEntries = res.results;
			const draft: Record<string, string> = {};
			for (const e of mapEntries) draft[e.raw_value] = e.canonical_bucket ?? mapBucketDraft[e.raw_value] ?? '';
			mapBucketDraft = draft;
		} catch (e) {
			mapError = e instanceof Error ? e.message : String(e);
		} finally {
			mapLoading = false;
		}
	}

	async function applyMapEntry(rawValue: string) {
		const bucket = (mapBucketDraft[rawValue] ?? '').trim();
		if (!bucket) {
			mapApplyError = { ...mapApplyError, [rawValue]: 'Select or enter a canonical bucket first.' };
			return;
		}
		mapApplying = { ...mapApplying, [rawValue]: true };
		mapApplyError = { ...mapApplyError, [rawValue]: '' };
		mapApplyResult = { ...mapApplyResult, [rawValue]: '' };
		try {
			const res = await upsertValueRangeTypeMapEntry({ raw_value: rawValue, canonical_bucket: bucket });
			mapApplyResult = {
				...mapApplyResult,
				[rawValue]: `Saved as approved. ${res.corrected_count} metric row${res.corrected_count === 1 ? '' : 's'} corrected.`
			};
			await loadMapEntries();
			await loadList();
		} catch (e) {
			mapApplyError = { ...mapApplyError, [rawValue]: e instanceof Error ? e.message : String(e) };
		} finally {
			mapApplying = { ...mapApplying, [rawValue]: false };
		}
	}

	async function addNewEntry() {
		const rawValue = newEntryRawValue.trim();
		const bucket = newEntryBucket.trim();
		if (!rawValue || !bucket) {
			addEntryError = 'raw_value and canonical_bucket are both required.';
			return;
		}
		addingEntry = true;
		addEntryError = '';
		addEntryResult = '';
		try {
			const res = await upsertValueRangeTypeMapEntry({ raw_value: rawValue, canonical_bucket: bucket });
			addEntryResult = `Added "${res.entry.raw_value}" as approved.`;
			newEntryRawValue = '';
			newEntryBucket = '';
			await loadMapEntries();
			await loadList();
		} catch (e) {
			addEntryError = e instanceof Error ? e.message : String(e);
		} finally {
			addingEntry = false;
		}
	}

	onMount(() => {
		loadList();
		loadMapEntries();
	});
</script>

<div class="p-6" style="background:{pageBg}; min-height:100%;">
	<div class="mb-5">
		<h1 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">
			Database Maintenance — Resolve Metric Range Types
		</h1>
		<p style="font-size:13px; color:{textSecondary};">
			Triage kb.metrics rows whose value_range_type has no approved governed mapping, and correct
			kb.metric_value_range_type_map entries. Applying a correction clears the error on every metric
			row that shares the same raw value.
		</p>
	</div>

	<!-- Left / Right split -->
	<div class="flex gap-5 items-start" style="min-height:520px;">
		<!-- Left panel: search + results -->
		<div class="flex flex-col gap-4" style="width:380px; flex-shrink:0;">
			<div class="rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
				<div class="flex flex-col gap-3">
					<div class="flex flex-col gap-1">
						<label for="rmrt-record-id" style="font-size:12px; font-weight:500; color:{textMuted};">
							Input Record ID
						</label>
						<input
							id="rmrt-record-id"
							type="text"
							inputmode="numeric"
							placeholder="e.g. 244"
							bind:value={searchRecordId}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
								border-radius:7px; padding:6px 10px; font-size:13px;"
						/>
					</div>
					<div class="flex gap-2">
						<div class="flex flex-col gap-1" style="flex:1;">
							<label for="rmrt-date-from" style="font-size:12px; font-weight:500; color:{textMuted};">From</label>
							<input
								id="rmrt-date-from"
								type="date"
								bind:value={searchDateFrom}
								style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
									border-radius:7px; padding:6px 10px; font-size:13px; width:100%;"
							/>
						</div>
						<div class="flex flex-col gap-1" style="flex:1;">
							<label for="rmrt-date-to" style="font-size:12px; font-weight:500; color:{textMuted};">To</label>
							<input
								id="rmrt-date-to"
								type="date"
								bind:value={searchDateTo}
								style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
									border-radius:7px; padding:6px 10px; font-size:13px; width:100%;"
							/>
						</div>
					</div>
					<div class="flex flex-col gap-1">
						<label for="rmrt-error-type" style="font-size:12px; font-weight:500; color:{textMuted};">
							Error Type (raw value_range_type)
						</label>
						<select
							id="rmrt-error-type"
							bind:value={searchErrorType}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
								border-radius:7px; padding:6px 10px; font-size:13px;"
						>
							<option value="">All error types</option>
							{#each errorTypeOptions as opt (opt)}
								<option value={opt}>{opt}</option>
							{/each}
						</select>
					</div>
					<button
						onclick={search}
						disabled={listLoading}
						style="display:flex; align-items:center; justify-content:center; gap:6px;
							padding:7px 14px; border-radius:7px; border:none; cursor:pointer;
							font-size:13px; font-weight:500; background:{accent}; color:white;
							opacity:{listLoading ? 0.6 : 1};"
					>
						<SearchIcon style="width:13px; height:13px;" />
						{listLoading ? 'Searching…' : 'Search'}
					</button>
				</div>
			</div>

			{#if listError}
				<div class="rounded-xl p-3" style="background:rgba(248,113,113,0.08); border:1px solid rgba(248,113,113,0.25); color:{colorError}; font-size:13px;">
					{listError}
				</div>
			{/if}

			<div class="rounded-xl overflow-hidden" style="background:{cardBg}; border:1px solid {borderColor}; max-height:640px; overflow-y:auto;">
				{#if rows.length === 0 && !listLoading}
					<div class="p-4" style="font-size:13px; color:{textMuted};">No errored metrics found.</div>
				{/if}
				{#each rows as row (row.id)}
					<button
						onclick={() => selectRow(row.id)}
						style="display:block; width:100%; text-align:left; padding:10px 14px; border:none; cursor:pointer;
							border-bottom:1px solid {borderColor};
							background:{selectedId === row.id ? accentTint : 'transparent'};"
					>
						<div style="font-size:13px; font-weight:600; color:{textPrimary};">
							{row.metric_name || `Metric #${row.id}`}
						</div>
						<div style="font-size:12px; color:{textSecondary}; margin-top:2px;">
							record #{row.input_record_id} · value_range_type: {row.value_range_type || '—'}
						</div>
					</button>
				{/each}
			</div>
		</div>

		<!-- Right panel: Information Block + PDF Display -->
		<div class="flex-1 flex gap-4" style="min-width:0;">
			<div class="rounded-xl p-4" style="width:360px; flex-shrink:0; background:{cardBg}; border:1px solid {borderColor};">
				{#if !selected}
					<div style="font-size:13px; color:{textMuted};">Select a metric on the left to see its details.</div>
				{:else}
					<div class="flex flex-col gap-3">
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Metric Name</div>
							<div style="font-size:13px; color:{textPrimary};">{selected.metric_name || '—'}</div>
						</div>
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Description</div>
							<div style="font-size:13px; color:{textPrimary}; white-space:pre-wrap;">{selected.metric_desc || '—'}</div>
						</div>
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Context</div>
							<div style="font-size:13px; color:{textPrimary}; white-space:pre-wrap;">{selected.metric_context || '—'}</div>
						</div>
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Metric Value</div>
							<div style="font-size:13px; color:{textPrimary};">{selected.metric_value || '—'}</div>
						</div>
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Value Data Type</div>
							<div style="font-size:13px; color:{textPrimary};">{selected.value_data_type || '—'}</div>
						</div>
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Value Range Type</div>
							<div style="font-size:13px; color:{textPrimary};">{selected.value_range_type || '—'}</div>
						</div>
						<div class="rounded-lg p-3" style="background:rgba(248,113,113,0.08); border:1px solid rgba(248,113,113,0.25);">
							<div style="display:flex; align-items:center; gap:6px; margin-bottom:4px;">
								<AlertTriangleIcon style="width:13px; height:13px; color:{colorError};" />
								<span style="font-size:11px; font-weight:600; color:{colorError}; text-transform:uppercase; letter-spacing:0.04em;">Error</span>
							</div>
							<div style="font-size:13px; color:{textPrimary};">{selected.value_range_type_error}</div>
						</div>
					</div>
				{/if}
			</div>

			<div class="flex-1 rounded-xl overflow-hidden" style="min-width:0; min-height:600px; background:{cardBg}; border:1px solid {borderColor};">
				{#if !selected}
					<div class="p-4" style="font-size:13px; color:{textMuted};">PDF source will appear here once a metric is selected.</div>
				{:else if pdfLoading}
					<div class="p-4" style="font-size:13px; color:{textMuted};">Loading source document…</div>
				{:else if pdfError}
					<div class="p-4" style="font-size:13px; color:{colorError};">{pdfError}</div>
				{:else if isPdf && currentInput}
					<PdfViewWindow
						inputId={currentInput.id}
						{fileUrl}
						bind:page={docPage}
						bind:zoom={pdfZoom}
						bind:numPages={pdfNumPages}
						highlightVersion={`${selected.id}:${highlightSelectionVersion}`}
						{renderHighlights}
						showSidebar={false}
						enableSelectionDialog={false}
						{darkMode}
					/>
				{:else}
					<div class="p-4" style="font-size:13px; color:{textMuted};">
						Source record #{selected.input_record_id} is not a PDF; no highlight preview available.
					</div>
				{/if}
			</div>
		</div>
	</div>

	<!-- Map Block: kb.metric_value_range_type_map governance -->
	<div class="rounded-xl mt-5" style="background:{cardBg}; border:1px solid {borderColor}; overflow:hidden;">
		<div class="flex items-center justify-between p-4" style="border-bottom:1px solid {borderColor};">
			<div>
				<div style="font-size:14px; font-weight:600; color:{textPrimary};">Value Range Type Map</div>
				<div style="font-size:12px; color:{textSecondary};">
					Entries with status other than "approved" are invalid and shown first.
				</div>
			</div>
			<button
				onclick={loadMapEntries}
				disabled={mapLoading}
				title="Refresh"
				style="display:flex; align-items:center; justify-content:center; width:32px; height:32px;
					border-radius:7px; border:none; cursor:pointer; background:{surface2}; color:{textSecondary};
					opacity:{mapLoading ? 0.6 : 1};"
			>
				<RefreshCwIcon style="width:14px; height:14px;{mapLoading ? ' animation:spin 1s linear infinite;' : ''}" />
			</button>
		</div>

		{#if mapError}
			<div class="p-3" style="color:{colorError}; font-size:13px;">{mapError}</div>
		{/if}

		<datalist id="rmrt-canonical-bucket-options">
			{#each CANONICAL_BUCKET_OPTIONS as opt}
				<option value={opt}></option>
			{/each}
		</datalist>

		<div
			class="grid"
			style="grid-template-columns: 1fr 220px 110px 1fr 90px; padding:8px 16px;
				background:{surface2}; border-bottom:1px solid {borderColor};
				font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;"
		>
			<span>raw_value</span>
			<span>canonical_bucket</span>
			<span>status</span>
			<span>result</span>
			<span></span>
		</div>

		{#each sortedMapEntries as entry (entry.raw_value)}
			{@const invalid = isInvalidMapEntry(entry)}
			<div
				class="grid items-center"
				style="grid-template-columns: 1fr 220px 110px 1fr 90px; padding:8px 16px; gap:8px;
					border-bottom:1px solid {borderColor};
					background:{invalid ? (darkMode ? 'rgba(251,191,36,0.05)' : 'rgba(217,119,6,0.04)') : 'transparent'};"
			>
				<span style="font-size:13px; color:{textPrimary}; word-break:break-word;">{entry.raw_value}</span>
				<input
					type="text"
					list="rmrt-canonical-bucket-options"
					bind:value={mapBucketDraft[entry.raw_value]}
					placeholder="lower_bound, upper_bound, exact, range…"
					style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:6px; padding:5px 8px; font-size:12px;"
				/>
				<span style="font-size:12px; font-weight:600; color:{invalid ? colorWarn : colorOk};">
					{entry.status}
				</span>
				<span style="font-size:12px; color:{mapApplyError[entry.raw_value] ? colorError : colorOk};">
					{mapApplyError[entry.raw_value] || mapApplyResult[entry.raw_value] || ''}
				</span>
				<button
					onclick={() => applyMapEntry(entry.raw_value)}
					disabled={mapApplying[entry.raw_value]}
					style="display:flex; align-items:center; justify-content:center; gap:4px;
						padding:5px 10px; border-radius:6px; border:none; cursor:pointer;
						font-size:12px; font-weight:500; background:{accent}; color:white;
						opacity:{mapApplying[entry.raw_value] ? 0.6 : 1};"
				>
					<CheckIcon style="width:12px; height:12px;" />
					{mapApplying[entry.raw_value] ? '…' : 'Apply'}
				</button>
			</div>
		{/each}

		<!-- Add Entry -->
		<div class="p-4" style="border-top:1px solid {borderColor};">
			<div style="font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:8px;">Add New Entry</div>
			<div class="flex gap-2 items-center flex-wrap">
				<input
					type="text"
					placeholder="raw_value"
					bind:value={newEntryRawValue}
					style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:6px; padding:6px 10px; font-size:13px; width:220px;"
				/>
				<input
					type="text"
					list="rmrt-canonical-bucket-options"
					placeholder="canonical_bucket"
					bind:value={newEntryBucket}
					style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:6px; padding:6px 10px; font-size:13px; width:220px;"
				/>
				<button
					onclick={addNewEntry}
					disabled={addingEntry}
					style="display:flex; align-items:center; gap:6px;
						padding:6px 14px; border-radius:7px; border:none; cursor:pointer;
						font-size:13px; font-weight:500; background:{accent}; color:white;
						opacity:{addingEntry ? 0.6 : 1};"
				>
					<PlusIcon style="width:13px; height:13px;" />
					{addingEntry ? 'Adding…' : 'Add'}
				</button>
				{#if addEntryError}
					<span style="font-size:12px; color:{colorError};">{addEntryError}</span>
				{:else if addEntryResult}
					<span style="font-size:12px; color:{colorOk};">{addEntryResult}</span>
				{/if}
			</div>
		</div>
	</div>
</div>

<style>
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(200, 85, 61, 0.18);
		border: 1px solid rgba(200, 85, 61, 0.9);
		box-shadow: inset 0 0 0 1px rgba(255, 210, 179, 0.22);
	}
	@keyframes spin {
		from { transform: rotate(0deg); }
		to   { transform: rotate(360deg); }
	}
</style>
