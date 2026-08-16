<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		listMetricRangeTypeErrors,
		isInvalidMapEntry,
		type MetricRangeTypeError
	} from './resolve-metric-range-types-client.js';
	import { rangeTypeMapShelf, loadRangeTypeMapEntries } from './resolve-metric-range-types-shelf-store.svelte';
	import { getKbInput, getRawLines, type KbInputRecord, type RawLine } from '$lib/services/kbService';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import AlertTriangleIcon from '@lucide/svelte/icons/alert-triangle';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (matches db-consistency-view.svelte / resolve-ambiguous-objects-view.svelte) ---
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let inputBg       = $derived(darkMode ? '#141824' : '#FFFFFF');
	let colorError    = $derived(darkMode ? '#F87171' : '#DC2626');

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

	// The row's source_line_spans verbatim (line-number spans like "90" /
	// "98:99"), for the Information Block -- normalizeSpans above resolves the
	// same field to pages for highlighting, but an operator triaging a bad
	// value_range_type also wants the stored grounding as kb.metrics holds it.
	let spansDisplay = $derived.by(() => {
		const raw = selected?.source_line_spans;
		if (!Array.isArray(raw)) return '';
		return raw
			.map((item) =>
				typeof item === 'string' ? item.trim() : typeof item === 'number' ? String(item) : ''
			)
			.filter((s) => s !== '')
			.join(', ');
	});

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

		if (currentInput?.id === row.input_record_id) {
			// Source document already loaded -- just move the PDF display to the
			// target page and repaint the highlight, without refetching or
			// remounting PdfViewWindow (matches metric-mgmt-view.svelte's
			// selectMetric, which avoids the same remount/reload trap).
			const spans = normalizeSpans(row);
			if (spans.length > 0) docPage = spans[0].page_number;
			highlightSelectionVersion += 1;
			return;
		}

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
			docPage = spans.length > 0 ? spans[0].page_number : 1;
			highlightSelectionVersion += 1;
		} catch (e) {
			pdfError = e instanceof Error ? e.message : String(e);
			currentInput = null;
			rawLines = [];
		} finally {
			pdfLoading = false;
		}
	}

	// --- Map Block: kb.metric_value_range_type_map governance, rendered in the
	// page-level context shelf (see resolve-metric-range-types-shelf-store) ---
	let errorTypeOptions = $derived.by(() =>
		[...new Set(rangeTypeMapShelf.entries.filter(isInvalidMapEntry).map((e) => e.raw_value))].sort()
	);

	onMount(() => {
		loadList();
		rangeTypeMapShelf.active = true;
		loadRangeTypeMapEntries();
	});

	onDestroy(() => {
		rangeTypeMapShelf.active = false;
		rangeTypeMapShelf.onCorrected = null;
	});

	$effect(() => {
		rangeTypeMapShelf.onCorrected = () => loadList();
	});
</script>

<div class="p-6 flex flex-col" style="background:{pageBg}; height:100%; overflow:hidden;">
	<div class="mb-5 flex-shrink-0">
		<h1 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">
			Database Maintenance — Resolve Metric Range Types
		</h1>
		<p style="font-size:13px; color:{textSecondary};">
			Triage kb.metrics rows whose value_range_type has no approved governed mapping, and correct
			kb.metric_value_range_type_map entries (see the Value Range Type Map in the context panel on
			the right). Approving a mapping clears the error on every metric row that shares the same raw
			value; "Apply" on an approved entry additionally rewrites those rows' value_range_type to the
			canonical bucket.
		</p>
	</div>

	<!-- Left / Right split -->
	<div class="flex-1 flex gap-5" style="min-height:0;">
		<!-- Left panel: search + results -->
		<div class="flex flex-col gap-4" style="width:380px; flex-shrink:0; min-height:0;">
			<div class="rounded-xl p-4 flex-shrink-0" style="background:{cardBg}; border:1px solid {borderColor};">
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
				<div class="rounded-xl p-3 flex-shrink-0" style="background:rgba(248,113,113,0.08); border:1px solid rgba(248,113,113,0.25); color:{colorError}; font-size:13px;">
					{listError}
				</div>
			{/if}

			<div class="rounded-xl flex-1" style="background:{cardBg}; border:1px solid {borderColor}; min-height:0; overflow-y:auto;">
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
		<div class="flex-1 flex gap-4" style="min-width:0; min-height:0;">
			<div class="rounded-xl p-4 overflow-y-auto" style="width:360px; flex-shrink:0; min-height:0; background:{cardBg}; border:1px solid {borderColor};">
				{#if !selected}
					<div style="font-size:13px; color:{textMuted};">Select a metric on the left to see its details.</div>
				{:else}
					<div class="flex flex-col gap-3">
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Metric ID</div>
							<div style="font-size:13px; color:{textPrimary}; font-family:ui-monospace,SFMono-Regular,Menlo,monospace; word-break:break-all;">{selected.metric_id || '—'}</div>
						</div>
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
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">Source Line Spans</div>
							<div style="font-size:13px; color:{textPrimary}; font-family:ui-monospace,SFMono-Regular,Menlo,monospace; word-break:break-word;">{spansDisplay || '—'}</div>
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

			<!-- Must be a flex COLUMN: PdfViewWindow's .pvw-container relies on `flex:1`
			     to inherit a definite height, which then flows down to
			     .pdf-stage -> .pdf-layout(height:100%) -> .pdf-canvas-host(height:100%;
			     overflow:auto). In a plain block parent that `flex:1` is inert, the host
			     grows to fit every page, and scrollToPage()/scrollToFirstHighlight()
			     become no-ops (host.scrollHeight === host.clientHeight). Matches
			     metric-mgmt-view.svelte's .doc-frame-wrap. -->
			<div class="flex-1 flex flex-col rounded-xl overflow-hidden" style="min-width:0; min-height:0; background:{cardBg}; border:1px solid {borderColor};">
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
</div>

<style>
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(200, 85, 61, 0.18);
		border: 1px solid rgba(200, 85, 61, 0.9);
		box-shadow: inset 0 0 0 1px rgba(255, 210, 179, 0.22);
	}
</style>
