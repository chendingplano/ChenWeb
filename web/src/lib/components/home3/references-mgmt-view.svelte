<script lang="ts">
	import { browser } from '$app/environment';
	import { m } from '$lib/paraglide/messages.js';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import { getRawLines, listKbReferences, type KbDroppedReference, type KbMetricSource, type KbReferenceMention, type KbInputRecord, type RawLine } from '$lib/services/kbService';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';
	import { GoldRunFilter, goldRunBadges } from './gold-run-filter.svelte';
	import GoldRunFilters from './gold-run-filters.svelte';
	import { kbInputRecordKey } from './metric-benchmark-record';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';

	// 'testbed' is the Reference Benchmark: read-only gold references (testbed.ref_mentions)
	// written by the extract-references-benchmark skill, one record-list entry per run,
	// plus each run's dropped candidates. Gold rows have no holding resolution.
	let { darkMode = true, source = 'kb' }: { darkMode?: boolean; source?: KbMetricSource } = $props();
	const gold = new GoldRunFilter('references');
	$effect(() => {
		if (source === 'testbed') void gold.load();
	});
	let record = $state<KbInputRecord | null>(null);
	let mentions = $state<KbReferenceMention[]>([]);
	let selected = $state<KbReferenceMention | null>(null);
	let referenceView = $state<'gold' | 'dropped'>('gold');
	let dropped = $state<KbDroppedReference[]>([]);
	let selectedDropped = $state<KbDroppedReference | null>(null);
	let loading = $state(false);
	let error = $state('');
	let rawLines = $state<RawLine[]>([]);
	let referenceFilter = $state('');
	let normativityFilter = $state('');
	let resolutionFilter = $state('');
	let highlightVersion = $state(0);
	let selectionRequest = 0;
	let pageNo = $state(1);
	let zoom = $state(0.7);
	let numPages = $state(0);
	const RECORD_LIST_WIDTH_KEY = 'references:records-width';
	const RECORD_WIDTH_MIN = 220;
	const RECORD_WIDTH_MAX = 640;
	let recordListWidth = $state(320);
	const REFERENCE_LIST_WIDTH_KEY = 'references:list-width';
	const REFERENCE_DETAILS_WIDTH_KEY = 'references:details-width';
	const LIST_WIDTH_MIN = 280;
	const LIST_WIDTH_MAX = 640;
	const DETAILS_WIDTH_MIN = 280;
	const DETAILS_WIDTH_MAX = 640;
	let referenceListWidth = $state(360);
	let referenceDetailsWidth = $state(360);
	let resizing = $state<'records' | 'list' | 'details' | null>(null);
	let widthsHydrated = $state(false);
	let palette = $derived(darkMode ? {
		bg: '#0E1116', panel: '#161A22', panelAlt: '#1C212C', line: '#2A3140',
		text: '#EDE7D3', secondary: '#B5AE94', muted: '#7C7560', accent: '#D4A24C', lineSoft: '#1F2530', crimson: '#C8553D', crimsonFaint: 'rgba(200,85,61,.18)', teal: '#5DAFA8'
	} : {
		bg: '#F5F1E8', panel: '#FBF8F0', panelAlt: '#F0EADB', line: '#D7CFB8',
		text: '#1A1410', secondary: '#5C5345', muted: '#8F8472', accent: '#B8801E', lineSoft: '#E5DEC8', crimson: '#A23E26', crimsonFaint: 'rgba(162,62,38,.12)', teal: '#2D7B73'
	});

	$effect(() => {
		if (!browser || widthsHydrated) return;
		const recordsWidth = Number(localStorage.getItem(RECORD_LIST_WIDTH_KEY));
		if (Number.isFinite(recordsWidth) && recordsWidth >= RECORD_WIDTH_MIN && recordsWidth <= RECORD_WIDTH_MAX) {
			recordListWidth = recordsWidth;
		}
		const listWidth = Number(localStorage.getItem(REFERENCE_LIST_WIDTH_KEY));
		const detailsWidth = Number(localStorage.getItem(REFERENCE_DETAILS_WIDTH_KEY));
		if (Number.isFinite(listWidth) && listWidth >= LIST_WIDTH_MIN && listWidth <= LIST_WIDTH_MAX) {
			referenceListWidth = listWidth;
		}
		if (Number.isFinite(detailsWidth) && detailsWidth >= DETAILS_WIDTH_MIN && detailsWidth <= DETAILS_WIDTH_MAX) {
			referenceDetailsWidth = detailsWidth;
		}
		widthsHydrated = true;
	});

	let workspaceStyle = $derived(
		`grid-template-columns:${recordListWidth}px 14px ${referenceListWidth}px 14px minmax(320px,1fr) 14px ${referenceDetailsWidth}px`
	);
	let filterControlBg = $derived(darkMode ? '#303A4B' : '#FFFFFF');
	let visibleMentions = $derived.by(() => {
		const query = referenceFilter.trim().toLocaleLowerCase();
		return mentions.filter((mention) => {
			const matchesText = !query || [mention.target_title, mention.cited_raw, mention.resource_title, mention.evidence, mention.identity_key]
				.some((value) => value?.toLocaleLowerCase().includes(query));
			const matchesNormativity = !normativityFilter || mention.normativity === normativityFilter;
			const matchesResolution = !resolutionFilter || mention.r2_state === resolutionFilter;
			return matchesText && matchesNormativity && matchesResolution;
		});
	});
	let visibleDropped = $derived.by(() => {
		const query = referenceFilter.trim().toLocaleLowerCase();
		return dropped.filter((item) => !query || [item.candidate_id, item.cited_raw, item.reason, item.drop_reason]
			.some((value) => value?.toLocaleLowerCase().includes(query)));
	});
	let lineNumToPage = $derived.by(() => {
		const map = new Map<number, number>();
		for (const line of rawLines) if (!map.has(line.line_number)) map.set(line.line_number, line.page_number);
		return map;
	});
	let selectedSourceLines = $derived.by(() => {
		const lines: RawLine[] = [];
		const seen = new Set<string>();
		const spans = referenceView === 'dropped' ? selectedDropped?.source_line_spans : selected?.source_line_spans;
		for (const span of spans ?? []) {
			const match = span.trim().match(/^(\d+)\s*(?:[:,\-]\s*(\d+))?$/);
			if (!match) continue;
			const start = Number(match[1]);
			const end = match[2] ? Math.min(Number(match[2]), start + 200) : start;
			for (let lineNumber = start; lineNumber <= end; lineNumber += 1) {
				const pageNumber = lineNumToPage.get(lineNumber);
				if (!pageNumber) continue;
				const key = `${pageNumber}:${lineNumber}`;
				const line = rawLines.find((item) => item.page_number === pageNumber && item.line_number === lineNumber);
				if (line && !seen.has(key)) {
					seen.add(key);
					lines.push(line);
				}
			}
		}
		return lines.sort((a, b) => a.page_number - b.page_number || a.line_number - b.line_number);
	});

	function persistPanelWidth(panel: 'records' | 'list' | 'details', width: number) {
		if (panel === 'records') {
			// Move this divider within the two lists, preserving their combined width.
			const combined = recordListWidth + referenceListWidth;
			recordListWidth = Math.max(RECORD_WIDTH_MIN, combined - LIST_WIDTH_MAX,
				Math.min(RECORD_WIDTH_MAX, combined - LIST_WIDTH_MIN, Math.round(width)));
			referenceListWidth = combined - recordListWidth;
			if (browser) {
				localStorage.setItem(RECORD_LIST_WIDTH_KEY, String(recordListWidth));
				localStorage.setItem(REFERENCE_LIST_WIDTH_KEY, String(referenceListWidth));
			}
		} else if (panel === 'list') {
			referenceListWidth = Math.max(LIST_WIDTH_MIN, Math.min(LIST_WIDTH_MAX, Math.round(width)));
			if (browser) localStorage.setItem(REFERENCE_LIST_WIDTH_KEY, String(referenceListWidth));
		} else {
			referenceDetailsWidth = Math.max(DETAILS_WIDTH_MIN, Math.min(DETAILS_WIDTH_MAX, Math.round(width)));
			if (browser) localStorage.setItem(REFERENCE_DETAILS_WIDTH_KEY, String(referenceDetailsWidth));
		}
	}

	function startResize(event: PointerEvent, panel: 'records' | 'list' | 'details') {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = panel === 'records' ? recordListWidth : panel === 'list' ? referenceListWidth : referenceDetailsWidth;
		resizing = panel;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (e: PointerEvent) => {
			const delta = e.clientX - startX;
			persistPanelWidth(panel, panel === 'details' ? startWidth - delta : startWidth + delta);
		};
		const end = () => {
			resizing = null;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', end);
			window.removeEventListener('pointercancel', end);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', end, { once: true });
		window.addEventListener('pointercancel', end, { once: true });
	}

	function resizeWithKeyboard(event: KeyboardEvent, panel: 'records' | 'list' | 'details') {
		if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
		event.preventDefault();
		const current = panel === 'records' ? recordListWidth : panel === 'list' ? referenceListWidth : referenceDetailsWidth;
		const direction = event.key === 'ArrowRight' ? 1 : -1;
		persistPanelWidth(panel, current + direction * (panel === 'details' ? -16 : 16));
	}

	async function selectRecord(value: KbInputRecord) {
		const request = ++selectionRequest;
		record = value;
		selected = null;
		mentions = [];
		selectedDropped = null;
		dropped = [];
		rawLines = [];
		referenceFilter = '';
		normativityFilter = '';
		resolutionFilter = '';
		pageNo = 1;
		loading = true;
		error = '';
		try {
			// Benchmark entries carry exactly one run; the gold rows are that run's.
			const run = source === 'testbed' ? value.gold_metric_runs?.[0] : undefined;
			if (source === 'testbed' && !run) throw new Error(m.references_mgmt_load_failed());
			const [response, lines] = await Promise.all([
				listKbReferences(value.id, run),
				getRawLines(value.id).catch((error) => {
					throw new Error(m.references_mgmt_source_lines_load_failed({
						error: error instanceof Error ? error.message : String(error)
					}));
				})
			]);
			if (request !== selectionRequest) return;
			mentions = response.results ?? [];
			dropped = response.dropped ?? [];
			rawLines = lines.lines ?? [];
			if (referenceView === 'dropped') {
				if (dropped[0]) selectDropped(dropped[0]);
			} else if (mentions[0]) selectReference(mentions[0]);
		} catch (e) {
			if (request !== selectionRequest) return;
			error = e instanceof Error ? e.message : m.references_mgmt_load_failed();
		} finally {
			if (request === selectionRequest) loading = false;
		}
	}

	function selectReference(mention: KbReferenceMention) {
		selected = mention;
		highlightVersion += 1;
		const firstLine = selectedSourceLines[0];
		if (firstLine) pageNo = firstLine.page_number;
	}

	function selectDropped(item: KbDroppedReference) {
		selectedDropped = item;
		highlightVersion += 1;
		const firstLine = selectedSourceLines[0];
		if (firstLine) pageNo = firstLine.page_number;
	}

	function clearRecord() {
		selectionRequest++;
		record = null;
		selected = null;
		mentions = [];
		selectedDropped = null;
		dropped = [];
		rawLines = [];
		loading = false;
		error = '';
	}

	function changeReferenceView(view: 'gold' | 'dropped') {
		referenceView = view;
		referenceFilter = '';
		normativityFilter = '';
		selected = null;
		selectedDropped = null;
		if (view === 'dropped' && dropped[0]) selectDropped(dropped[0]);
		else if (view === 'gold' && mentions[0]) selectReference(mentions[0]);
	}

	let selectedVisibleIndex = $derived(referenceView === 'dropped'
		? visibleDropped.findIndex((item) => item.id === selectedDropped?.id)
		: visibleMentions.findIndex((mention) => mention.id === selected?.id));
	let visibleCount = $derived(referenceView === 'dropped' ? visibleDropped.length : visibleMentions.length);

	function stepReference(direction: -1 | 1) {
		if (referenceView === 'dropped') {
			const item = visibleDropped[selectedVisibleIndex + direction];
			if (item) selectDropped(item);
			return;
		}
		const mention = visibleMentions[selectedVisibleIndex + direction];
		if (mention) selectReference(mention);
	}

	function yesNo(value: boolean | undefined) {
		return value === undefined ? '—' : value ? m.references_mgmt_yes() : m.references_mgmt_no();
	}

	function renderReferenceHighlights(page: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		const pageLines = selectedSourceLines.filter((line) =>
			line.page_number === page && !selected?.source_table_rows?.some((ref) => ref.line === line.line_number)
		);
		if (pageLines.length === 0) return;
		const lines = pageLines.filter((line) => Array.isArray(line.coords) && line.coords.length >= 4);
		for (let i = 0; i < lines.length; i += 1) {
			const line = lines[i];
			const next = lines[i + 1];
			const left = Math.min(line.coords[0], line.coords[2]) * viewport.width / 1000;
			const top = Math.max(0, Math.min(line.coords[1], line.coords[3]) * viewport.height / 1000 - 2);
			const nextTop = next && next.line_number === line.line_number + 1
				? Math.min(next.coords[1], next.coords[3]) * viewport.height / 1000 - 2
				: Math.max(line.coords[1], line.coords[3]) * viewport.height / 1000;
			const width = Math.abs(line.coords[2] - line.coords[0]) * viewport.width / 1000 + 20;
			const height = Math.max(0, nextTop - top);
			if (width < 1 || height < 1) continue;
			const mark = document.createElement('div');
			mark.className = 'pdf-highlight';
			mark.style.left = `${left}px`;
			mark.style.top = `${top}px`;
			mark.style.width = `${width}px`;
			mark.style.height = `${height}px`;
			mark.title = `line ${line.line_number}`;
			overlay.appendChild(mark);
		}
	}

	function mapRecord(input: KbInputRecord) {
		return {
			id: input.id,
			title: input.title?.trim() || input.name?.trim() || input.file_name?.trim() || `Input #${input.id}`,
			subtitle: input.file_name || input.name || '',
			meta: [input.doc_no || '', input.type || ''].filter(Boolean),
			status: input.type || '',
			description: m.references_mgmt_select_document_to_review(),
			badges: [input.type || ''],
			benchmarkRuns: source === 'testbed' ? goldRunBadges(input) : []
		};
	}
</script>

<div class="references-page select-text" style={`--ref-bg:${palette.bg};--ref-panel:${palette.panel};--ref-alt:${palette.panelAlt};--ref-card:${filterControlBg};--ref-line:${palette.line};--ref-text:${palette.text};--ref-secondary:${palette.secondary};--ref-muted:${palette.muted};--ref-accent:${palette.accent};--ref-line-soft:${palette.lineSoft};--ref-crimson:${palette.crimson};--ref-crimson-faint:${palette.crimsonFaint};--ref-teal:${palette.teal}`}>
	<header class="heading">
		<div><div class="eyebrow">{m.references_mgmt_knowledge_system_vol_iii()}</div><h1>{source === 'testbed' ? m.references_mgmt_benchmark_title() : m.references_mgmt_title()}</h1><p>{source === 'testbed' ? m.references_mgmt_benchmark_subtitle() : m.references_mgmt_subtitle()}</p></div>
		<div class="stats"><small>{m.references_mgmt_record()}</small><strong>{record?.id ?? '—'}</strong><small>{m.references_mgmt_references()}</small><strong>{String(mentions.length).padStart(3, '0')}</strong>{#if source === 'testbed'}<small>{m.references_mgmt_dropped()}</small><strong>{String(dropped.length).padStart(3, '0')}</strong>{/if}</div>
	</header>
	<div class="workspace" style={workspaceStyle}>
		<aside class="records">
			<KbInputRecordBrowser {darkMode} fitPanel instanceKey={source === 'testbed' ? 'gold-references-record-browser' : 'references-record-browser'} title={m.references_mgmt_kb_inputs()} subtitle={m.references_mgmt_search_and_select()} emptyTitle={m.references_mgmt_no_records()} emptySubtitle={m.references_mgmt_use_search_to_browse()} autoSelectFirstRecord={false} selectedRecordId={record?.id ?? null} selectedRecordKey={record ? kbInputRecordKey(record) : null} listPausedMessage={source === 'testbed' ? gold.pausedMessage : ''} listParams={source === 'testbed' ? gold.listParams : undefined} mapRecord={mapRecord} onSelect={selectRecord} onError={(e) => (error = e.message)}>
				{#snippet extraControls()}
					{#if source === 'testbed'}<GoldRunFilters filter={gold} onChange={clearRecord} />{/if}
				{/snippet}
			</KbInputRecordBrowser>
		</aside>
		<button type="button" class="resize-handle" class:active={resizing === 'records'} aria-label={m.references_mgmt_resize_records_panel()} onpointerdown={(event) => startResize(event, 'records')} onkeydown={(event) => resizeWithKeyboard(event, 'records')}>
			<span class="resize-grip" aria-hidden="true"></span>
		</button>
		<section class="mention-column" aria-label={m.references_mgmt_references()}>
			<div class="column-head"><strong>{referenceView === 'dropped' ? m.references_mgmt_view_dropped() : m.references_mgmt_references()}</strong><span>{visibleCount} {m.references_mgmt_found()}</span></div>
			<details class="reference-filters">
				<summary><ChevronRightIcon size={14} />{m.references_mgmt_filters()}</summary>
				<div class="filter-controls">
					{#if source === 'testbed'}
						<select aria-label={m.references_mgmt_view()} value={referenceView} onchange={(event) => changeReferenceView(event.currentTarget.value as 'gold' | 'dropped')}>
							<option value="gold">{m.references_mgmt_view_gold()}</option>
							<option value="dropped">{m.references_mgmt_view_dropped()}</option>
						</select>
					{/if}
					<input aria-label={m.references_mgmt_filter_by_text()} placeholder={m.references_mgmt_filter_by_text()} bind:value={referenceFilter} />
					{#if referenceView === 'gold'}
						<select aria-label={m.references_mgmt_filter_by_normativity()} bind:value={normativityFilter}>
							<option value="">{m.references_mgmt_all_normativities()}</option>
							{#each [...new Set(mentions.map((item) => item.normativity).filter(Boolean))] as value}<option value={value}>{value}</option>{/each}
						</select>
					{/if}
					{#if source !== 'testbed'}
						<select aria-label={m.references_mgmt_filter_by_resolution()} bind:value={resolutionFilter}>
							<option value="">{m.references_mgmt_all_resolutions()}</option>
							{#each [...new Set(mentions.map((item) => item.r2_state).filter(Boolean))] as value}<option value={value}>{value}</option>{/each}
						</select>
					{/if}
				</div>
			</details>
			<div class="reference-navigation">
				<button type="button" disabled={loading || !!error || selectedVisibleIndex <= 0} onclick={() => stepReference(-1)}>{m.references_mgmt_prev()}</button>
				<button type="button" disabled={loading || !!error || visibleCount === 0 || selectedVisibleIndex >= visibleCount - 1} onclick={() => stepReference(1)}>{m.references_mgmt_next()}</button>
			</div>
			{#if loading}<p class="notice">{m.references_mgmt_loading()}</p>
			{:else if error}<p class="notice error">{error}</p>
			{:else if !record}<p class="notice">{m.references_mgmt_select_document()}</p>
			{:else if referenceView === 'dropped'}
				{#if visibleDropped.length === 0}<p class="notice">{m.references_mgmt_no_dropped_for_run()}</p>
				{:else}
					<div class="mention-list">
						{#each visibleDropped as item, i (item.id)}
							<button type="button" class="mention-card" class:active={selectedDropped?.id === item.id} aria-pressed={selectedDropped?.id === item.id} onclick={() => selectDropped(item)}>
								<div class="card-rule" aria-hidden="true"></div>
								<div class="card-body">
									<div class="card-row-top"><span class="card-index">№ {String(i + 1).padStart(3, '0')} <span class="card-reference-id">· {item.candidate_id}</span></span><span class="card-normativity">{item.drop_reason}</span></div>
									<div class="card-name">{item.cited_raw || m.references_mgmt_untitled_reference()}</div>
									<div class="card-desc">{item.reason || m.references_mgmt_no_excerpt()}</div>
									<div class="card-foot"><span class="chip"><span class="chip-dot" aria-hidden="true"></span>{item.decision}</span>{#each item.ref_ids as refId}<span class="chip chip-quiet">{refId}</span>{/each}</div>
								</div>
							</button>
						{/each}
					</div>
				{/if}
			{:else if mentions.length === 0}<p class="notice">{m.references_mgmt_no_references_for_document()}</p>
			{:else if visibleMentions.length === 0}<p class="notice">{m.references_mgmt_no_references_for_document()}</p>
			{:else}
				<div class="mention-list">
					{#each visibleMentions as mention, i (mention.id)}
						<button type="button" class="mention-card" class:active={selected?.id === mention.id} aria-pressed={selected?.id === mention.id} onclick={() => selectReference(mention)}>
							<div class="card-rule" aria-hidden="true"></div>
							<div class="card-body">
								<div class="card-row-top"><span class="card-index">№ {String(i + 1).padStart(3, '0')} <span class="card-reference-id">· {mention.ref_id}</span></span><span class="card-normativity">{mention.normativity}</span></div>
								<div class="card-name">{mention.target_title || mention.cited_raw || mention.resource_title || m.references_mgmt_untitled_reference()}</div>
								<div class="card-desc">{mention.evidence || mention.cited_raw || m.references_mgmt_no_excerpt()}</div>
								<div class="card-foot"><span class="chip"><span class="chip-dot" aria-hidden="true"></span>{mention.target_type} · {mention.target_content}</span>{#if mention.r2_state}<span class="chip chip-mono">{mention.r2_state}</span>{/if}{#if mention.metric_state}<span class="chip chip-quiet">{mention.metric_state}</span>{/if}</div>
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</section>
		<button type="button" class="resize-handle" class:active={resizing === 'list'} aria-label={m.references_mgmt_resize_reference_list()} onpointerdown={(event) => startResize(event, 'list')} onkeydown={(event) => resizeWithKeyboard(event, 'list')}>
			<span class="resize-grip" aria-hidden="true"></span>
		</button>
		<section class="document" aria-label={m.references_mgmt_source_document()}>
			{#if record}
				<PdfViewWindow {darkMode} inputId={record.id} fileUrl={`/api/v1/kb/inputs/${record.id}/file`} bind:page={pageNo} bind:zoom bind:numPages showSidebar={false} sidebarSettingsKey="references-pdf" {highlightVersion} renderHighlights={renderReferenceHighlights} tableReferences={selected?.source_table_rows ?? []}>
					{#snippet floatingOverlay()}
						{#if selected}
							<div class="reference-info select-text">
								<div class="reference-info-heading">{m.references_mgmt_reference_info()} <span>{selected.ref_id}</span></div>
								<div class="reference-info-content" onpointerdown={(event) => event.stopPropagation()} onselectstart={(event) => event.stopPropagation()}>
									<strong>{selected.target_title || selected.cited_raw || selected.resource_title || m.references_mgmt_untitled_reference()}</strong>
									{#if selected.evidence}<p>{selected.evidence}</p>{/if}
									<div class="reference-info-tags"><span>{selected.target_type} · {selected.target_content}</span><span>{selected.normativity}</span>{#if source !== 'testbed'}<span>{selected.r2_state || m.references_mgmt_unresolved()}</span>{/if}</div>
								</div>
							</div>
						{/if}
					{/snippet}
				</PdfViewWindow>
			{:else}
				<div class="empty-document">{m.references_mgmt_select_document()}</div>
			{/if}
		</section>
		<button type="button" class="resize-handle" class:active={resizing === 'details'} aria-label={m.references_mgmt_resize_details_panel()} onpointerdown={(event) => startResize(event, 'details')} onkeydown={(event) => resizeWithKeyboard(event, 'details')}>
			<span class="resize-grip" aria-hidden="true"></span>
		</button>
		<aside class="details-panel" aria-label={m.references_mgmt_reference_details()}>
			<div class="details-heading">{m.references_mgmt_reference_details()}</div>
			{#if referenceView === 'dropped'}
				{#if selectedDropped}
					<div class="detail"><div class="detail-label">{m.references_mgmt_dropped_details()}</div><h2>{selectedDropped.cited_raw || m.references_mgmt_untitled_reference()}</h2>
						<dl>
							<dt>{m.references_mgmt_decision()}</dt><dd>{selectedDropped.decision}</dd>
							<dt>{m.references_mgmt_rule()}</dt><dd>{selectedDropped.drop_reason}</dd>
							<dt>{m.references_mgmt_reason()}</dt><dd>{selectedDropped.reason || '—'}</dd>
							<dt>{m.references_mgmt_merged_into()}</dt><dd>{selectedDropped.ref_ids.join(', ') || '—'}</dd>
						</dl>
					</div>
				{:else}<p class="notice">{m.references_mgmt_select_dropped()}</p>{/if}
			{:else if selected}
				<div class="detail"><div class="detail-label">{m.references_mgmt_selected_reference()}</div><h2>{selected.target_title || selected.cited_raw || selected.resource_title || m.references_mgmt_untitled_reference()}</h2>
					<dl>
						<dt>{m.references_mgmt_type()}</dt><dd>{selected.target_type} / {selected.target_subtype}</dd>
						<dt>{m.references_mgmt_content()}</dt><dd>{selected.target_content}</dd>
						<dt>{m.references_mgmt_normativity()}</dt><dd>{selected.normativity}{selected.list_kind ? ` · ${selected.list_kind}` : ''}</dd>
						{#if source === 'testbed'}
							<dt>{m.references_mgmt_ref_form()}</dt><dd>{selected.ref_form || '—'}</dd>
							<dt>{m.references_mgmt_mechanism()}</dt><dd>{selected.mechanism || '—'}</dd>
							<dt>{m.references_mgmt_in_requirement()}</dt><dd>{yesNo(selected.in_requirement)}</dd>
							<dt>{m.references_mgmt_internal()}</dt><dd>{yesNo(selected.internal)}</dd>
							<dt>{m.references_mgmt_confidence()}</dt><dd>{selected.confidence ?? '—'}</dd>
						{:else}
							<dt>{m.references_mgmt_holding_resolution()}</dt><dd>{selected.r2_state || m.references_mgmt_unresolved()}{selected.target_input_title ? ` · ${selected.target_input_title}` : ''}</dd>
							<dt>{m.references_mgmt_resource_identity()}</dt><dd>{selected.identity_key || '—'}</dd>
						{/if}
						<dt>{m.references_mgmt_identifiers()}</dt><dd>{selected.identifiers?.join(', ') || '—'}</dd>
						<dt>{m.references_mgmt_source_excerpt()}</dt><dd>{selected.evidence || selected.cited_raw || '—'}</dd>
					</dl>
				</div>
			{:else}<p class="notice">{m.references_mgmt_select_reference()}</p>{/if}
		</aside>
	</div>
</div>

<style>
	.references-page{height:100%;min-height:0;display:flex;flex-direction:column;background:var(--ref-bg);color:var(--ref-text);font-family:'Inter Tight',system-ui,sans-serif;user-select:text}
	.heading{flex:none;min-height:126px;padding:22px 28px;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--ref-line);background:var(--ref-bg)}
	.eyebrow,.stats small,.detail-label{font:600 10px 'JetBrains Mono',monospace;letter-spacing:.16em;text-transform:uppercase;color:var(--ref-accent)}
	h1{font:600 34px 'Cormorant Garamond',Georgia,serif;margin:6px 0;color:var(--ref-text)}
	.heading p{margin:0;color:var(--ref-secondary);font:italic 13px Georgia,serif}
	.stats{display:grid;grid-template-columns:auto auto;gap:5px 18px;text-align:right;padding:12px 16px;background:var(--ref-panel);border:1px solid var(--ref-line)}
	.stats strong{font:600 14px 'JetBrains Mono',monospace}
	.workspace{flex:1;min-height:0;min-width:0;display:grid;grid-template-rows:minmax(0,1fr);overflow:hidden}
	.records :global(.action-row){grid-template-columns:repeat(2,minmax(0,1fr))}
	.records :global(.retrieve-btn){grid-column:1 / -1;white-space:nowrap}
	.records :global(.reset-btn),.records :global(.settings-btn){min-width:0;min-height:40px;padding:.6rem .5rem;font-size:12px;white-space:nowrap}
	.records,.mention-column,.document,.details-panel{min-width:0;min-height:0;border-right:1px solid var(--ref-line);background:var(--ref-panel)}
	.records{position:relative;overflow:hidden;--panel-bg:var(--ref-card);--ink-line:var(--ref-line);--text-primary:var(--ref-text);--text-muted:var(--ref-muted);--brass:var(--ref-accent);--font-mono:'JetBrains Mono',monospace}.mention-column{display:flex;flex-direction:column;overflow:hidden;background:var(--ref-bg)}.document{display:flex;flex-direction:column;overflow:hidden;background:var(--ref-bg)}.details-panel{overflow-y:auto;overflow-x:hidden;border-right:0}
	.column-head{height:58px;flex:none;display:flex;align-items:center;justify-content:space-between;padding:0 18px;border-bottom:1px solid var(--ref-line);color:var(--ref-text)}
	.column-head span{font:10px 'JetBrains Mono',monospace;color:var(--ref-accent);text-transform:uppercase;letter-spacing:.1em}
	.reference-navigation{flex:none;display:flex;gap:8px;margin:0 8px 12px}
	.reference-navigation button{flex:1;height:32px;border:1px solid var(--ref-line);background:var(--ref-card);color:var(--ref-text);font:11px 'JetBrains Mono',monospace;cursor:pointer}
	.reference-navigation button:hover:not(:disabled){border-color:var(--ref-accent);background:var(--ref-alt)}.reference-navigation button:disabled{opacity:.4;cursor:default}.reference-navigation button:focus-visible{outline:2px solid var(--ref-accent);outline-offset:2px}
	.reference-filters{flex:none;margin:0 8px 8px;background:var(--ref-card);color:var(--ref-text)}
	.reference-filters summary{height:34px;display:flex;align-items:center;gap:8px;padding:0 10px;cursor:pointer;list-style:none;font:10px 'JetBrains Mono',monospace;color:var(--ref-secondary)}
	.reference-filters summary::-webkit-details-marker{display:none}.reference-filters[open] summary :global(svg){transform:rotate(90deg)}
	.filter-controls{display:grid;gap:6px;padding:0 10px 10px}.filter-controls input,.filter-controls select{width:100%;min-width:0;height:30px;padding:0 8px;background:var(--ref-panel);border:1px solid var(--ref-line);color:var(--ref-secondary);font:10px 'JetBrains Mono',monospace}
	.mention-list{flex:1;min-width:0;min-height:0;overflow-y:auto;overflow-x:hidden;padding:0 16px 24px;display:flex;flex-direction:column;gap:10px;scrollbar-width:thin;scrollbar-color:var(--ref-line) transparent}
	.mention-list::-webkit-scrollbar,.details-panel::-webkit-scrollbar{width:8px}.mention-list::-webkit-scrollbar-thumb,.details-panel::-webkit-scrollbar-thumb{background:var(--ref-line)}
	/* Cards keep their content height while the list scrolls. */
	.mention-card{all:unset;box-sizing:border-box;flex-shrink:0;min-width:0;display:flex;position:relative;text-align:left;background:var(--ref-card);border:1px solid var(--ref-line-soft);color:var(--ref-text);cursor:pointer;transition:border-color 150ms,background 150ms}
	.mention-card:hover{border-color:var(--ref-accent);background:var(--ref-panel)}.mention-card.active{border-color:var(--ref-crimson);background:var(--ref-crimson-faint)}
	.mention-card:focus-visible{outline:2px solid var(--ref-accent);outline-offset:2px}
	.card-rule{width:4px;flex-shrink:0;background:var(--ref-line);transition:background 150ms}.mention-card:hover .card-rule{background:var(--ref-accent)}.mention-card.active .card-rule{background:var(--ref-crimson)}
	.card-body{padding:12px 14px 12px 12px;flex:1;min-width:0;display:flex;flex-direction:column;gap:6px}
	.card-row-top{display:flex;justify-content:space-between;align-items:baseline;gap:8px;overflow-wrap:anywhere}
	.card-index{min-width:0;font:10px 'JetBrains Mono',monospace;letter-spacing:.1em;color:var(--ref-muted);text-transform:uppercase}.card-reference-id{text-transform:none;letter-spacing:.02em}
	.card-normativity{font:600 11px 'JetBrains Mono',monospace;color:var(--ref-teal)}
	.card-name{font:500 17px/1.2 'Cormorant Garamond','Playfair Display',Georgia,serif;color:var(--ref-text);overflow-wrap:anywhere}.mention-card.active .card-name{color:var(--ref-crimson)}
	.card-desc{font-size:12px;line-height:1.45;color:var(--ref-secondary);overflow-wrap:anywhere;display:-webkit-box;line-clamp:2;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
	.card-foot{display:flex;flex-wrap:wrap;gap:6px;margin-top:2px}.chip{display:inline-flex;align-items:center;gap:5px;font:10px 'JetBrains Mono',monospace;text-transform:uppercase;letter-spacing:.06em;padding:3px 8px;border:1px solid var(--ref-line);color:var(--ref-secondary);overflow-wrap:anywhere;min-width:0}
	.chip-dot{width:4px;height:4px;flex-shrink:0;background:var(--ref-accent);border-radius:50%}.chip-mono{color:var(--ref-accent);border-color:var(--ref-accent)}.chip-quiet{color:var(--ref-muted)}
	:global(.pdf-highlight){position:absolute;background:rgba(200,85,61,.18);border:1px solid rgba(200,85,61,.9);box-shadow:inset 0 0 0 1px rgba(255,210,179,.22)}
	.notice,.empty-document{padding:24px;color:var(--ref-muted);font-size:13px}.error{color:#c8553d}.empty-document{height:100%;display:grid;place-items:center}
	.details-heading{height:58px;display:flex;align-items:center;padding:0 16px;border-bottom:1px solid var(--ref-line);font-weight:600;color:var(--ref-text)}
	.detail{padding:16px;color:var(--ref-text)}.detail h2{font:600 19px Georgia,serif;margin:10px 0 16px;overflow-wrap:anywhere}.detail dl{display:grid;grid-template-columns:115px minmax(0,1fr);gap:9px 10px;margin:0}.detail dt{font:9px 'JetBrains Mono',monospace;color:var(--ref-muted);text-transform:uppercase;letter-spacing:.08em}.detail dd{margin:0;font-size:12px;overflow-wrap:anywhere;line-height:1.45}
	.document :global(.pdf-floating-overlay-anchor){max-width:calc(100% - 16px)}
	.reference-info{width:300px;max-width:100%;border:1px solid #414957;border-radius:12px;background:linear-gradient(145deg,#252B35,#191D24);color:#EDE7D3;box-shadow:0 12px 28px rgba(0,0,0,.35);overflow:hidden;user-select:text}
	.reference-info-heading{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:6px;padding:10px 12px 0;font:10px 'JetBrains Mono',monospace;color:#D4A24C}
	.reference-info-heading span{color:#B5AE94;border:1px solid #414957;border-radius:999px;padding:2px 6px}
	.reference-info-content{padding:8px 12px 12px;cursor:text;user-select:text}.reference-info-content strong{display:block;font:600 15px/1.4 Georgia,serif;overflow-wrap:anywhere}
	.reference-info-content p{margin:7px 0;font-size:11px;line-height:1.5;color:#B5AE94;white-space:pre-line;display:-webkit-box;line-clamp:4;-webkit-line-clamp:4;-webkit-box-orient:vertical;overflow:hidden}
	.reference-info-tags{display:flex;flex-wrap:wrap;gap:5px;margin-top:8px;font:9px 'JetBrains Mono',monospace;color:#B5AE94}.reference-info-tags span{padding:3px 6px;border:1px solid #414957;border-radius:999px}
	.resize-handle{width:14px;position:relative;padding:0;border:0;background:transparent;cursor:col-resize;touch-action:none;z-index:2}.resize-handle::before{content:'';position:absolute;top:0;bottom:0;left:6px;width:2px;background:var(--ref-line);opacity:.65}.resize-handle:hover::before,.resize-handle.active::before,.resize-handle:focus-visible::before{background:var(--ref-accent);opacity:1}.resize-grip{position:absolute;top:50%;left:50%;width:7px;height:48px;transform:translate(-50%,-50%);border-radius:999px;background:radial-gradient(circle,var(--ref-muted) 22%,transparent 24%) center 6px/5px 10px repeat-y,var(--ref-panel);border:1px solid var(--ref-line)}
	@media(max-width:1250px){.workspace{grid-template-columns:minmax(220px,20%) 14px minmax(260px,1fr) 14px minmax(300px,1.2fr) 14px minmax(260px,0.9fr)}}
	@media(max-width:1000px){.workspace{grid-template-columns:minmax(200px,28%) 14px minmax(240px,1fr) 14px minmax(300px,1.2fr) 14px minmax(240px,.9fr)}.heading{min-height:110px}}
</style>
