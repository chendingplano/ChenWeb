<script lang="ts">
	import { onDestroy, type Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import GoldRunFilters from '$lib/components/home3/gold-run-filters.svelte';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import { GoldRunFilter, goldRunBadges } from './gold-run-filter.svelte';
	import { kbInputRecordKey, metricBenchmarkSelection } from './metric-benchmark-record';
	import { getRawLines, listGoldSummaries, type GoldSummaryRecord, type KbInputRecord, type RawLine } from '$lib/services/kbService';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import PanelLeftCloseIcon from '@lucide/svelte/icons/panel-left-close';
	import PanelLeftOpenIcon from '@lucide/svelte/icons/panel-left-open';

	let { darkMode = true, onSummaryDetailsChange }: { darkMode: boolean; onSummaryDetailsChange?: (details: Snippet | null) => void } = $props();
	const gold = new GoldRunFilter('summaries');
	let browserFolded = $state(false);
	let searchPanelWidth = $state(330);
	let searchPanelResizing = $state(false);
	let currentInput = $state<KbInputRecord | null>(null);
	let summaries = $state<GoldSummaryRecord[]>([]);
	let rawLines = $state<RawLine[]>([]);
	let selectedSummaryId = $state<number | null>(null);
	let loading = $state(false);
	let error = $state('');
	let requestVersion = 0;
	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);
	const selectedSummary = $derived(summaries.find((item) => item.id === selectedSummaryId) ?? null);
	const selectedSourceLineNumbers = $derived(parseSummaryLineSpans(selectedSummary?.source_line_spans));
	const selectedLinesByPage = $derived.by(() => {
		const wanted = new Set(selectedSourceLineNumbers);
		const byPage = new Map<number, RawLine[]>();
		for (const line of rawLines) {
			if (!wanted.has(line.line_number) || !Array.isArray(line.coords) || line.coords.length < 4) continue;
			const pageLines = byPage.get(line.page_number) ?? [];
			pageLines.push(line);
			byPage.set(line.page_number, pageLines);
		}
		return byPage;
	});
	const selectedSummaryPages = $derived([...selectedLinesByPage.keys()].sort((a, b) => a - b));
	const fileUrl = $derived(currentInput ? `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width` : '');
	const isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	const bg = $derived(darkMode ? '#0E1116' : '#F5F1E8');
	const panel = $derived(darkMode ? '#161A22' : '#FBF8F0');
	const panelAlt = $derived(darkMode ? '#1C212C' : '#F0EADB');
	const line = $derived(darkMode ? '#2A3140' : '#D7CFB8');
	const text = $derived(darkMode ? '#EDE7D3' : '#1A1410');
	const muted = $derived(darkMode ? '#8F8975' : '#786F5F');
	const brass = $derived(darkMode ? '#D4A24C' : '#B8801E');
	const red = $derived(darkMode ? '#C8553D' : '#A23E26');
	const bodyGridStyle = $derived(browserFolded
		? 'grid-template-columns:42px 360px minmax(0,1fr)'
		: `grid-template-columns:${searchPanelWidth}px 14px 360px minmax(0,1fr)`);

	$effect(() => {
		void gold.load();
	});

	$effect(() => {
		const hasSelectedSummary = selectedSummary != null;
		onSummaryDetailsChange?.(hasSelectedSummary ? (summaryDetailsContent as unknown as Snippet) : null);
	});

	onDestroy(() => onSummaryDetailsChange?.(null));

	function mapBrowserRecord(record: KbInputRecord) {
		return {
			id: record.id,
			title: record.title?.trim() || record.name?.trim() || record.file_name?.trim() || m.metric_mgmt_input({ id: record.id }),
			subtitle: record.file_name?.trim() || record.name?.trim() || '—',
			meta: [record.doc_no?.trim() || '—', record.parser_name?.trim() || '—'],
			status: record.type?.trim() || '—',
			description: record.create_time ? new Date(record.create_time).toLocaleString() : '—',
			badges: [record.type?.trim() || '—'],
			benchmarkRuns: goldRunBadges(record)
		};
	}

	function startSearchPanelResize(event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = searchPanelWidth;
		searchPanelResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (moveEvent: PointerEvent) => {
			searchPanelWidth = Math.max(250, Math.min(560, startWidth + moveEvent.clientX - startX));
		};
		const finish = () => {
			searchPanelResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', finish);
			window.removeEventListener('pointercancel', finish);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', finish, { once: true });
		window.addEventListener('pointercancel', finish, { once: true });
	}

	function resizeSearchPanelByKeyboard(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			searchPanelWidth = Math.max(250, searchPanelWidth - 16);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			searchPanelWidth = Math.min(560, searchPanelWidth + 16);
		}
	}

	function clearBenchmarkRecord() {
		requestVersion++;
		currentInput = null;
		summaries = [];
		rawLines = [];
		selectedSummaryId = null;
		docPage = 1;
		pdfNumPages = 0;
		loading = false;
		error = '';
	}

	async function handleRecordSelect(record: KbInputRecord) {
		if (currentInput && kbInputRecordKey(currentInput) === kbInputRecordKey(record)) return;
		const version = ++requestVersion;
		currentInput = record;
		summaries = [];
		rawLines = [];
		selectedSummaryId = null;
		docPage = 1;
		pdfNumPages = 0;
		loading = true;
		error = '';
		try {
			const filters = metricBenchmarkSelection(record, gold.skillVersion, gold.modelName);
			const [result, lineResult] = await Promise.all([
				listGoldSummaries(record.id, filters),
				getRawLines(record.id).catch(() => null)
			]);
			if (version !== requestVersion) return;
			summaries = result.summaries ?? [];
			rawLines = lineResult?.lines ?? [];
			selectedSummaryId = summaries[0]?.id ?? null;
		} catch (err) {
			if (version === requestVersion) error = err instanceof Error ? err.message : m.sgv_load_failed();
		} finally {
			if (version === requestVersion) loading = false;
		}
	}

	function parseSummaryLineSpans(value: unknown): number[] {
		const found = new Set<number>();
		const addRange = (start: unknown, end?: unknown) => {
			const first = Number(start);
			const last = end == null ? first : Number(end);
			if (!Number.isInteger(first) || !Number.isInteger(last) || first < 1 || last < first || last - first > 200) return;
			for (let line = first; line <= last; line++) found.add(line);
		};
		const visit = (entry: unknown) => {
			if (typeof entry === 'number') { addRange(entry); return; }
			if (typeof entry === 'string') {
				const match = entry.trim().match(/^(\d+)\s*(?::|-|,)\s*(\d+)$/);
				if (match) addRange(match[1], match[2]);
				else if (/^\d+$/.test(entry.trim())) addRange(entry.trim());
				return;
			}
			if (Array.isArray(entry)) { for (const part of entry) visit(part); return; }
			if (entry && typeof entry === 'object') {
				const span = entry as Record<string, unknown>;
				const line = span.line_number ?? span.line ?? span.line_no ?? span.lineNo;
				if (line != null) addRange(line);
				else if (span.start_line != null || span.start != null) addRange(span.start_line ?? span.start, span.end_line ?? span.end);
			}
		};
		visit(value);
		return [...found].sort((a, b) => a - b);
	}

	$effect(() => {
		const firstPage = selectedSummaryPages[0];
		if (firstPage != null) docPage = firstPage;
	});

	function renderSummaryHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		for (const line of selectedLinesByPage.get(pageNo) ?? []) {
			const [x1, y1, x2, y2] = line.coords;
			const left = Math.min(x1, x2) * viewport.width / 1000;
			const top = Math.max(0, Math.min(y1, y2) * viewport.height / 1000 - 2);
			const width = Math.abs(x2 - x1) * viewport.width / 1000 + 20;
			const height = Math.max(1, Math.abs(y2 - y1) * viewport.height / 1000 + 4);
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

	function jsonText(value: unknown): string {
		if (value == null) return '—';
		return typeof value === 'string' ? value : JSON.stringify(value, null, 2) ?? '—';
	}
</script>

<svelte:head>
	<title>{m.sgv_title()}</title>
</svelte:head>

<!-- Bound the PDF scroll pane to the content slot; min-height alone lets it grow to fit every page. -->
<div
	class="benchmark-page flex h-full min-h-0 min-w-0 flex-col overflow-hidden select-text"
	style="--page-bg:{bg};--panel-bg:{panel};--panel-bg-alt:{panelAlt};--ink-line:{line};--ink-line-soft:{line};--text-primary:{text};--text-secondary:{muted};--text-muted:{muted};--brass:{brass};--brass-faint:color-mix(in srgb, {brass} 12%, transparent);--crimson:{red};--crimson-faint:color-mix(in srgb, {red} 14%, transparent);--teal:#5DAFA8;--font-serif:'Cormorant Garamond','Playfair Display',Georgia,serif;--font-mono:'JetBrains Mono','IBM Plex Mono',monospace;--font-sans:'Inter Tight',system-ui,sans-serif;"
>
	<header class="benchmark-header">
		<div class="header-left"><h1>{m.sgv_title()}</h1></div>
		<div class="header-meta">
			<span>{m.metric_mgmt_record()}</span><strong>{currentInput?.id ?? '—'}</strong>
			<span>{m.metric_mgmt_version_label()}</span><strong>{gold.skillVersion || '—'}</strong>
			<span>{m.metric_mgmt_model_label()}</span><strong>{gold.modelName || '—'}</strong>
			<span>{m.sgv_summary_records()}</span><strong>{summaries.length.toString().padStart(3, '0')}</strong>
		</div>
	</header>

	<div class="body" class:browser-folded={browserFolded} style={bodyGridStyle}>
		{#if browserFolded}
			<div class="browser-rail">
				<button type="button" class="fold-toggle" onclick={() => (browserFolded = false)} title={m.metric_mgmt_unfold_record_list()} aria-label={m.metric_mgmt_unfold_record_list()}><PanelLeftOpenIcon size={16} /></button>
				<span>{m.metric_mgmt_record_2()}</span>
			</div>
		{:else}
			<div class="record-browser-slot">
				<button type="button" class="fold-toggle" onclick={() => (browserFolded = true)} title={m.metric_mgmt_fold_record_list()} aria-label={m.metric_mgmt_fold_record_list()}><PanelLeftCloseIcon size={16} /></button>
				<KbInputRecordBrowser
					{darkMode}
					instanceKey="gold-summary-record-browser"
					title={m.metric_mgmt_kb_inputs()}
					subtitle={m.metric_mgmt_search_filter_and_select_input()}
					emptyTitle={m.metric_mgmt_no_records_yet()}
					emptySubtitle={m.metric_mgmt_use_search_or_retrieve_to()}
					autoSelectFirstRecord={false}
					selectedRecordId={currentInput?.id ?? null}
					selectedRecordKey={currentInput ? kbInputRecordKey(currentInput) : null}
					listPausedMessage={gold.pausedMessage}
					mapRecord={mapBrowserRecord}
					onSelect={handleRecordSelect}
					onError={(e) => (error = e.message)}
					listParams={gold.listParams}
				>
					{#snippet extraControls()}
						<GoldRunFilters filter={gold} onChange={clearBenchmarkRecord} />
					{/snippet}
				</KbInputRecordBrowser>
			</div>
		{/if}
		{#if !browserFolded}
			<button type="button" class="search-resize-handle" class:active={searchPanelResizing} aria-label={m.kb_input_record_browser_resize_record_browser()} onpointerdown={startSearchPanelResize} onkeydown={resizeSearchPanelByKeyboard}>
				<span class="search-resize-grip" aria-hidden="true"></span>
			</button>
		{/if}

		<aside class="summary-sidebar">
			<div class="sidebar-heading"><h2>{m.sgv_summary_records()}</h2><span>{summaries.length} {m.sgv_found()}</span></div>
			{#if error}<div class="error">{error}</div>{/if}
			{#if loading}
				<div class="empty-state">{m.sgv_loading_summaries()}</div>
			{:else if !currentInput}
				<div class="empty-state">{m.sgv_select_document()}</div>
			{:else if summaries.length === 0}
				<div class="empty-state">{m.sgv_no_summaries()}</div>
			{:else}
				<div class="summary-list">
					{#each summaries as item (item.id)}
						<button type="button" class="summary-card" class:selected={selectedSummaryId === item.id} onclick={() => (selectedSummaryId = item.id)}>
							<div class="card-top"><span class="sequence">#{String(item.summary_seq_no).padStart(3, '0')} · {item.summary_id}</span><span class="language">{item.language}</span></div>
							<div class="card-text">{item.summary_text || item.empty_reason || '—'}</div>
							<div class="card-footer"><span>{item.skill_version}</span><span>{item.model_name}</span></div>
						</button>
					{/each}
				</div>
			{/if}
		</aside>

		<section class="detail-panel">
			{#if currentInput}
				<div class="doc-frame-wrap">
					{#if isPdf}
						<PdfViewWindow inputId={currentInput.id} {fileUrl} bind:page={docPage} bind:zoom={pdfZoom} bind:numPages={pdfNumPages} {darkMode} enableSelectionDialog={false} highlightPages={selectedSummaryPages} highlightVersion={selectedSummaryId ?? 0} renderHighlights={renderSummaryHighlights} />
					{:else}
						<iframe class="doc-frame" title={currentInput.file_name ?? m.metric_mgmt_record_4({ id: currentInput.id })} src={fileUrl}></iframe>
					{/if}
				</div>
			{:else}
				<div class="detail-empty">{m.sgv_select_document()}</div>
			{/if}
		</section>
	</div>
</div>

{#snippet summaryDetailsContent()}
	{#if selectedSummary}
		<div class="context-summary-details select-text">
			<div class="context-summary-heading">
				<div class="detail-eyebrow">{m.sgv_summary_detail()}</div>
				<h2>{selectedSummary.summary_id}</h2>
				<div class="context-summary-badges"><span>{selectedSummary.language}</span><span>{selectedSummary.skill_version}</span><span>{selectedSummary.model_name}</span></div>
			</div>
			<section class="summary-text-block"><h3>{m.sgv_summary_text()}</h3><p>{selectedSummary.summary_text || selectedSummary.empty_reason || '—'}</p></section>
			{#if selectedSummary.summary_text_en && selectedSummary.summary_text_en !== selectedSummary.summary_text}
				<section class="summary-text-block"><h3>{m.sgv_english_summary()}</h3><p>{selectedSummary.summary_text_en}</p></section>
			{/if}
			<div class="metadata-grid">
				<div><span>{m.sgv_level()}</span><strong>{selectedSummary.summary_level}</strong></div>
				<div><span>{m.sgv_sequence()}</span><strong>{selectedSummary.summary_seq_no}</strong></div>
				<div><span>{m.sgv_empty_reason()}</span><strong>{selectedSummary.empty_reason || '—'}</strong></div>
				<div><span>{m.sgv_created_at()}</span><strong>{new Date(selectedSummary.created_at).toLocaleString()}</strong></div>
				<div><span>{m.metric_mgmt_record()}</span><strong>{selectedSummary.input_record_id}</strong></div>
				<div><span>{m.metric_mgmt_model_label()}</span><strong>{selectedSummary.model_name}</strong></div>
				<div class="run-meta"><span>{m.sgv_run_id()}</span><strong>{selectedSummary.run_id}</strong></div>
			</div>
			<div class="detail-grid">
				<section class="data-card"><h3>{m.sgv_keywords()}</h3><pre>{jsonText(selectedSummary.keywords)}</pre></section>
				<section class="data-card"><h3>{m.sgv_keywords_en()}</h3><pre>{jsonText(selectedSummary.keywords_en)}</pre></section>
				<section class="data-card"><h3>{m.sgv_source_line_spans()}</h3><pre>{jsonText(selectedSummary.source_line_spans)}</pre></section>
				<section class="data-card"><h3>{m.sgv_overlap_lines()}</h3><pre>{jsonText(selectedSummary.overlap_lines)}</pre></section>
				<section class="data-card"><h3>{m.sgv_regular_lines()}</h3><pre>{jsonText(selectedSummary.regular_lines)}</pre></section>
				<section class="data-card"><h3>{m.sgv_information_units()}</h3><pre>{jsonText(selectedSummary.units)}</pre></section>
				<section class="data-card"><h3>{m.sgv_completeness_review()}</h3><pre>{jsonText(selectedSummary.completeness_review)}</pre></section>
				<section class="data-card"><h3>{m.sgv_correctness_review()}</h3><pre>{jsonText(selectedSummary.correctness_review)}</pre></section>
			</div>
		</div>
	{/if}
{/snippet}

<style>
	.benchmark-page { background: var(--page-bg); color: var(--text-primary); }
	.benchmark-header { display:flex; align-items:center; gap:20px; min-height:58px; padding:8px 18px; border-bottom:1px solid var(--ink-line); background:var(--panel-bg); }
	.header-left { flex:0 0 auto; }
	.header-left h1 { margin:0; font:500 28px/1 var(--font-serif); white-space:nowrap; }
	.header-meta { display:flex; flex:1; justify-content:flex-end; align-items:baseline; flex-wrap:wrap; gap:4px 10px; padding:7px 12px; border:1px solid var(--ink-line); background:var(--panel-bg-alt); white-space:nowrap; }
	.header-meta span,.detail-eyebrow { color:var(--text-muted); font:10px var(--font-mono); letter-spacing:.14em; text-transform:uppercase; }
	.header-meta strong { margin-right:8px; color:var(--text-primary); font:12px var(--font-mono); }
	.body { display:grid; grid-template-columns:330px 14px 360px minmax(0,1fr); grid-template-rows:minmax(0,1fr); flex:1; min-width:0; min-height:0; overflow:hidden; }
	.body.browser-folded { grid-template-columns:42px 360px minmax(0,1fr) !important; }
	.record-browser-slot { position:relative; min-width:0; min-height:0; overflow:hidden; border-right:1px solid var(--ink-line); }
	.search-resize-handle { grid-column:2; grid-row:1; position:relative; min-width:14px; padding:0; border:0; background:transparent; cursor:col-resize; outline:none; touch-action:none; z-index:2; }
	.search-resize-handle::before { content:''; position:absolute; top:0; bottom:0; left:6px; width:2px; background:var(--ink-line); opacity:.6; transition:background 140ms,opacity 140ms; }
	.search-resize-handle:hover::before,.search-resize-handle.active::before,.search-resize-handle:focus-visible::before { background:var(--brass); opacity:1; }
	.search-resize-grip { position:absolute; top:50%; left:50%; width:7px; height:48px; transform:translate(-50%,-50%); border-radius:999px; background:radial-gradient(circle,var(--text-muted) 22%,transparent 24%) center 6px / 5px 10px repeat-y,var(--panel-bg); border:1px solid var(--ink-line); }
	.fold-toggle { all:unset; position:absolute; top:10px; right:10px; z-index:6; display:inline-flex; align-items:center; justify-content:center; width:28px; height:28px; border:1px solid var(--ink-line); background:var(--panel-bg-alt); color:var(--text-secondary); cursor:pointer; }
	.fold-toggle:hover,.fold-toggle:focus-visible { border-color:var(--brass); color:var(--brass); }
	.browser-rail { position:relative; display:flex; align-items:center; justify-content:center; border-right:1px solid var(--ink-line); background:var(--panel-bg); }
	.browser-rail .fold-toggle { position:static; }
	.browser-rail span { position:absolute; bottom:18px; left:50%; transform:translateX(-50%) rotate(-90deg); color:var(--text-muted); font:10px var(--font-mono); letter-spacing:.16em; text-transform:uppercase; white-space:nowrap; }
	.summary-sidebar { display:flex; flex-direction:column; min-width:0; min-height:0; overflow:hidden; border-right:1px solid var(--ink-line); background:var(--panel-bg); }
	.sidebar-heading { display:flex; align-items:baseline; gap:8px; padding:16px 18px 8px; }
	.sidebar-heading h2 { margin:0; color:var(--text-primary); font:500 22px var(--font-serif); }
	.sidebar-heading span { color:var(--text-muted); font:10px var(--font-mono); text-transform:uppercase; }
	.summary-list { display:flex; flex-direction:column; gap:8px; min-height:0; overflow:auto; padding:8px 10px 14px; }
	.summary-card { display:block; width:100%; padding:10px; border:1px solid var(--ink-line); background:var(--panel-bg-alt); color:var(--text-primary); text-align:left; cursor:pointer; transition:border-color 120ms,background 120ms; }
	.summary-card:hover,.summary-card.selected { border-color:var(--brass); }
	.summary-card.selected { background:color-mix(in srgb,var(--brass) 10%,var(--panel-bg-alt)); }
	.card-top,.card-footer { display:flex; justify-content:space-between; gap:8px; color:var(--text-muted); font:9px var(--font-mono); }
	.sequence { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
	.language { color:var(--brass); text-transform:uppercase; }
	.card-text { display:-webkit-box; overflow:hidden; margin:8px 0; color:var(--text-primary); font:12px/1.45 var(--font-sans); line-clamp:3; -webkit-box-orient:vertical; -webkit-line-clamp:3; }
	.card-footer { justify-content:flex-start; flex-wrap:wrap; color:var(--text-secondary); }
	.error { margin:8px 10px; padding:8px 10px; border-left:3px solid var(--crimson); background:var(--crimson-faint); color:var(--crimson); font:11px var(--font-mono); overflow-wrap:anywhere; }
	.empty-state,.detail-empty { display:grid; flex:1; place-items:center; padding:24px; color:var(--text-muted); font:12px/1.6 var(--font-mono); text-align:center; }
	.detail-panel { display:flex; flex-direction:column; min-width:0; min-height:0; overflow:hidden; background:var(--page-bg); }
	.doc-frame-wrap { display:flex; flex:1; flex-direction:column; min-height:0; padding:12px 16px; overflow:hidden; }
	.doc-frame { flex:1; width:100%; min-width:0; min-height:0; border:1px solid var(--ink-line); background:var(--panel-bg); }
	.summary-text-block,.data-card { min-width:0; border:1px solid var(--ink-line); background:var(--panel-bg); }
	.summary-text-block { margin-bottom:12px; padding:12px 14px; border-left:3px solid var(--brass); }
	.summary-text-block h3,.data-card h3 { margin:0; color:var(--text-muted); font:600 10px var(--font-mono); letter-spacing:.11em; text-transform:uppercase; }
	.summary-text-block p { margin:9px 0 0; color:var(--text-primary); font:14px/1.65 var(--font-sans); white-space:pre-wrap; }
	.metadata-grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:8px; margin-bottom:12px; }
	.metadata-grid div { min-width:0; padding:9px 11px; border:1px solid var(--ink-line); background:var(--panel-bg-alt); }
	.metadata-grid span { display:block; color:var(--text-muted); font:9px var(--font-mono); letter-spacing:.08em; text-transform:uppercase; }
	.metadata-grid strong { display:block; margin-top:5px; color:var(--text-primary); font:11px/1.4 var(--font-mono); overflow-wrap:anywhere; }
	.detail-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:9px; }
	.data-card { overflow:hidden; }
	.data-card h3 { padding:9px 11px; border-bottom:1px solid var(--ink-line); }
	.data-card pre { max-height:260px; overflow:auto; margin:0; padding:10px 11px; color:var(--text-secondary); font:10px/1.5 var(--font-mono); white-space:pre-wrap; overflow-wrap:anywhere; user-select:text; }
	.context-summary-details { display:flex; flex:1; flex-direction:column; gap:10px; min-height:0; overflow:auto; padding:0 2px 16px; }
	.context-summary-heading { padding:12px; border:1px solid var(--ink-line); background:var(--panel-bg-alt); }
	.context-summary-heading h2 { margin:5px 0 8px; color:var(--text-primary); font:500 18px/1.2 var(--font-serif); overflow-wrap:anywhere; }
	.context-summary-badges { display:flex; flex-wrap:wrap; gap:4px; }
	.context-summary-badges span { padding:3px 5px; border:1px solid var(--ink-line); color:var(--text-secondary); font:8px var(--font-mono); overflow-wrap:anywhere; }
	.context-summary-details .summary-text-block { margin:0; padding:10px; }
	.context-summary-details .summary-text-block p { font-size:12px; }
	.context-summary-details .metadata-grid { grid-template-columns:repeat(2,minmax(0,1fr)); gap:5px; margin:0; }
	.context-summary-details .metadata-grid div { padding:7px; }
	.context-summary-details .metadata-grid span { font-size:8px; }
	.context-summary-details .metadata-grid strong { font-size:9px; }
	.context-summary-details .run-meta { grid-column:1 / -1; }
	.context-summary-details .detail-grid { grid-template-columns:minmax(0,1fr); gap:7px; }
	.context-summary-details .data-card pre { max-height:190px; font-size:9px; }
	@media (max-width:1480px) { .body:not(.browser-folded) { grid-template-columns: minmax(250px, 330px) 14px 320px minmax(0,1fr); } .body.browser-folded { grid-template-columns:42px 320px minmax(0,1fr) !important; } }
	@media (max-width:1250px) { .metadata-grid { grid-template-columns:repeat(2,minmax(0,1fr)); } }
	@media (max-width:900px) { .benchmark-header { align-items:flex-start; flex-direction:column; } .header-meta { justify-content:flex-start; width:100%; } .body,.body.browser-folded { grid-template-columns:minmax(0,1fr) !important; grid-template-rows:minmax(260px,38vh) minmax(240px,32vh) minmax(400px,1fr); overflow:auto; } .search-resize-handle { display:none; } .record-browser-slot,.browser-rail,.summary-sidebar { border-right:0; border-bottom:1px solid var(--ink-line); } .browser-rail { display:none; } .record-browser-slot { min-height:280px; } .detail-grid { grid-template-columns:minmax(0,1fr); } }
</style>
