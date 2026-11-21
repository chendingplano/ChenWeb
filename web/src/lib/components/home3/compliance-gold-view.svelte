<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import PanelLeftCloseIcon from '@lucide/svelte/icons/panel-left-close';
	import PanelLeftOpenIcon from '@lucide/svelte/icons/panel-left-open';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import GoldRunFilters from './gold-run-filters.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import { GoldRunFilter, goldRunBadges } from './gold-run-filter.svelte';
	import { kbInputRecordKey, metricBenchmarkSelection } from './metric-benchmark-record';
	import { getGoldCompliance, getRawLines, type ComplianceGoldData, type ComplianceGoldJudgement, type ComplianceGoldLabel, type KbInputRecord, type RawLine } from '$lib/services/kbService';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	const gold = new GoldRunFilter('compliance', { allowAllVersions: true });
	let currentInput = $state<KbInputRecord | null>(null);
	let data = $state<ComplianceGoldData | null>(null);
	let lines = $state<RawLine[]>([]);
	let selectedId = $state<number | null>(null);
	let label = $state<ComplianceGoldLabel | ''>('');
	let query = $state('');
	let folded = $state(false);
	let loading = $state(false);
	let error = $state('');
	let sourceUnavailable = $state(false);
	let page = $state(1);
	let zoom = $state(0.5);
	let numPages = $state(0);
	let version = 0;
	let candId = $state<number | null>(null);
	let candLines = $state<RawLine[]>([]);
	let candUnavailable = $state(false);
	let candReady = $state(false);
	let candPage = $state(1);
	let candZoom = $state(0.5);
	let candNumPages = $state(0);
	let candRequest = 0;
	const rawLinesCache = new Map<number, RawLine[] | null>();
	const labels: ComplianceGoldLabel[] = ['governs', 'topical', 'unrelated', 'ambiguous'];
	const selected = $derived(data?.activities.find((a) => a.id === selectedId));
	const judgements = $derived((selected?.judgements ?? []).filter((j) =>
		(!label || j.final_label === label) && (!query.trim() ||
			[j.artifact_id, j.source_title, j.reason_1, j.reason_2, j.rationale, j.found_by, String(j.candidate_record)].join(' ').toLowerCase().includes(query.trim().toLowerCase()))));
	const governing = $derived(data?.activities.filter((a) => a.judgements.some((j) => j.final_label === 'governs')).length ?? 0);
	const sourceLines = $derived(selected ? lines.filter((l) => l.line_number >= selected.line_start && l.line_number <= selected.line_end) : []);
	const candJudgement = $derived(judgements.find((j) => j.id === candId));
	const candNumbers = $derived(candJudgement ? spanLineNumbers(candJudgement.source_line_spans) : new Set<number>());
	const candSourceLines = $derived(candLines.filter((l) => candNumbers.has(l.line_number)));
	const candHighlightPages = $derived([...new Set(candSourceLines.map((l) => l.page_number))]);
	const candFileUrl = $derived(candJudgement ? `/api/v1/kb/inputs/${candJudgement.candidate_record}/file` : '');
	const highlightPages = $derived([...new Set(sourceLines.map((l) => l.page_number))]);
	const fileUrl = $derived(currentInput ? `/api/v1/kb/inputs/${currentInput.id}/file` : '');
	const isPdf = $derived(currentInput?.type?.toLowerCase() === 'pdf');
	const MIN_PANEL = 280;
	const MIN_EVIDENCE = 360;
	let panelWidth = $state(460);
	const MIN_ACTIVITIES = 200;
	let activitiesWidth = $state(270);
	let resizingActivities = false;
	function clampActivities(width: number) {
		const total = workspace?.clientWidth ?? 0;
		const fixed = (folded ? 42 : 380) + 12 + panelWidth;
		const max = Math.max(MIN_ACTIVITIES, total - fixed - MIN_EVIDENCE);
		return Math.min(max, Math.max(MIN_ACTIVITIES, width));
	}
	function startResizeActivities(e: PointerEvent) { resizingActivities = true; (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId); e.preventDefault(); }
	function moveResizeActivities(e: PointerEvent) {
		const left = activitiesEl?.getBoundingClientRect().left;
		if (!resizingActivities || left === undefined) return;
		activitiesWidth = clampActivities(e.clientX - left);
	}
	function endResizeActivities(e: PointerEvent) { resizingActivities = false; (e.currentTarget as HTMLElement).releasePointerCapture?.(e.pointerId); }
	function keyResizeActivities(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft') { activitiesWidth = clampActivities(activitiesWidth - 24); e.preventDefault(); }
		else if (e.key === 'ArrowRight') { activitiesWidth = clampActivities(activitiesWidth + 24); e.preventDefault(); }
	}
	let activitiesEl = $state<HTMLElement | null>(null);
	let workspace = $state<HTMLDivElement | null>(null);
	let resizing = false;
	function clampPanel(width: number) {
		const total = workspace?.clientWidth ?? 0;
		const fixed = (folded ? 42 : 380) + activitiesWidth + 12 + 6;
		const max = Math.max(MIN_PANEL, total - fixed - MIN_EVIDENCE);
		return Math.min(max, Math.max(MIN_PANEL, width));
	}
	function startResize(e: PointerEvent) { resizing = true; (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId); e.preventDefault(); }
	function moveResize(e: PointerEvent) {
		if (!resizing || !workspace) return;
		panelWidth = clampPanel(workspace.getBoundingClientRect().right - e.clientX);
	}
	function endResize(e: PointerEvent) { resizing = false; (e.currentTarget as HTMLElement).releasePointerCapture?.(e.pointerId); }
	function keyResize(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft') { panelWidth = clampPanel(panelWidth + 24); e.preventDefault(); }
		else if (e.key === 'ArrowRight') { panelWidth = clampPanel(panelWidth - 24); e.preventDefault(); }
	}
	onMount(() => { void gold.load(); });
	$effect(() => { if (highlightPages.length) page = highlightPages[0]; });
	$effect(() => { if (candHighlightPages.length) candPage = candHighlightPages[0]; });
	// source_line_spans are strings such as "1112" or "1221-1222" (line numbers of the candidate record).
	function spanLineNumbers(spans: unknown): Set<number> {
		const out = new Set<number>();
		if (!Array.isArray(spans)) return out;
		for (const span of spans) {
			const [a, rawEnd] = String(span).split('-').map((x) => parseInt(x, 10));
			if (Number.isNaN(a)) continue;
			const end = rawEnd === undefined || Number.isNaN(rawEnd) ? a : Math.min(rawEnd, a + 2000);
			for (let n = a; n <= end; n++) out.add(n);
		}
		return out;
	}
	async function selectCandidate(j: ComplianceGoldJudgement) {
		if (candId === j.id) return;
		candId = j.id; candLines = []; candUnavailable = false; candReady = false; candPage = 1; candNumPages = 0;
		const request = ++candRequest;
		if (!rawLinesCache.has(j.candidate_record)) {
			rawLinesCache.set(j.candidate_record, (await getRawLines(j.candidate_record).catch(() => null))?.lines ?? null);
		}
		if (request !== candRequest) return;
		const found = rawLinesCache.get(j.candidate_record);
		candUnavailable = found == null; candLines = found ?? [];
		const first = candLines.find((l) => candNumbers.has(l.line_number));
		candPage = first?.page_number ?? 1; candReady = true;
	}
	function clearCandidate() { candRequest++; candReady = false; candId = null; candLines = []; candUnavailable = false; candPage = 1; candNumPages = 0; }
	$effect(() => { void selectedId; clearCandidate(); });
	// Activity names are stored as slugs ("document_specifies_heat_pump_water_heaters"); show them as a sentence.
	function activityText(slug: string) {
		const text = slug.replace(/_+/g, ' ').trim();
		return text.charAt(0).toUpperCase() + text.slice(1);
	}
	// Why the candidate relates to the activity: the adjudicated rationale, else the annotators' reasons.
	function whyRelated(j: ComplianceGoldJudgement) {
		return j.rationale || j.reason_1 || j.reason_2 || '—';
	}
	function labelText(value: string) {
		switch (value) {
			case 'governs': return m.cgv_governs();
				case 'topical': return m.cgv_topical();
				case 'unrelated': return m.cgv_unrelated();
				case 'ambiguous': return m.cgv_ambiguous();
				default: return value;
		}
	}
	function clear() {
		version++; rawLinesCache.clear(); currentInput = null; data = null; lines = []; selectedId = null;
		label = ''; query = ''; error = ''; sourceUnavailable = false; loading = false; page = 1; numPages = 0;
	}
	async function selectRecord(record: KbInputRecord) {
		if (currentInput && kbInputRecordKey(record) === kbInputRecordKey(currentInput)) return;
		clear();
		currentInput = record;
		const request = ++version;
		loading = true;
		try {
			const run = metricBenchmarkSelection(record, gold.skillVersion, gold.modelName).runId;
			if (!run) throw new Error(m.cgv_select_document());
			const [result, raw] = await Promise.all([getGoldCompliance(record.id, run), getRawLines(record.id).catch(() => null)]);
			if (request !== version) return;
			data = result; lines = raw?.lines ?? []; sourceUnavailable = raw == null; selectedId = result.activities[0]?.id ?? null;
		} catch {
			if (request === version) error = m.cgv_load_failed();
		} finally { if (request === version) loading = false; }
	}
	function mapRecord(record: KbInputRecord) {
		return { id: record.id, title: record.title || record.name || record.file_name || String(record.id),
			subtitle: record.doc_no || record.file_name || '', benchmarkRuns: goldRunBadges(record) };
	}
	const highlights = (pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) => drawHighlights(sourceLines, pageNo, viewport, overlay);
	const candHighlights = (pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) => drawHighlights(candSourceLines, pageNo, viewport, overlay);
	function drawHighlights(source: RawLine[], pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		for (const line of source.filter((l) => l.page_number === pageNo && l.coords?.length >= 4)) {
			const [x1, y1, x2, y2] = line.coords;
			const mark = document.createElement('div'); mark.className = 'pdf-highlight';
			Object.assign(mark.style, { left: `${Math.min(x1,x2)*viewport.width/1000}px`, top: `${Math.min(y1,y2)*viewport.height/1000}px`,
				width: `${Math.abs(x2-x1)*viewport.width/1000}px`, height: `${Math.max(2,Math.abs(y2-y1)*viewport.height/1000)}px` });
			overlay.appendChild(mark);
		}
	}
</script>

<div class="gold-page select-text" class:light={!darkMode}>
	<header><h1>{m.nav_sysadmin_llm_compliance_gold()}</h1><span>{m.cgv_read_only()}</span></header>
	<p class="notice">{m.cgv_scope()}</p>
	<div class="workspace" class:folded bind:this={workspace} style:--panel-w="{panelWidth}px" style:--act-w="{activitiesWidth}px">
		{#if folded}
		<div class="browser-rail">
			<button type="button" class="fold-toggle" onclick={() => folded = false} title={m.metric_mgmt_unfold_record_list()} aria-label={m.metric_mgmt_unfold_record_list()}><PanelLeftOpenIcon size={16} /></button>
			<span>{m.metric_mgmt_record_2()}</span>
		</div>
		{:else}
		<div class="browser">
			<button type="button" class="fold-toggle" onclick={() => folded = true} title={m.metric_mgmt_fold_record_list()} aria-label={m.metric_mgmt_fold_record_list()}><PanelLeftCloseIcon size={16} /></button>
			<KbInputRecordBrowser {darkMode} fitPanel instanceKey="gold-compliance-record-browser"
				title={m.metric_mgmt_kb_inputs()} subtitle={m.cgv_select_document()}
				emptyTitle={m.cgv_no_runs()} emptySubtitle={m.cgv_empty_hint()}
				autoSelectFirstRecord={false} selectedRecordId={currentInput?.id ?? null}
				selectedRecordKey={currentInput ? kbInputRecordKey(currentInput) : null}
				listPausedMessage={gold.pausedMessage} mapRecord={mapRecord} onSelect={selectRecord}
				onError={() => error = m.cgv_load_failed()} listParams={gold.listParams}>
				{#snippet extraControls()}<GoldRunFilters filter={gold} onChange={clear} />{/snippet}
			</KbInputRecordBrowser>
		</div>
		{/if}
		<section class="activities" bind:this={activitiesEl}>
			<h2>{m.cgv_activities()}</h2>
			{#if error || gold.error}<p role="alert" class="error">{error || m.cgv_load_failed()}</p><button type="button" onclick={() => { const record = currentInput; clear(); if (record) void selectRecord(record); else void gold.load(); }}>{m.cgv_retry()}</button>{/if}
			{#if loading}<p role="status">{m.cgv_loading()}</p>
			{:else if !data}<p>{m.cgv_select_document()}</p>
			{:else}
				<details class="run"><summary>{data.run.run_key}</summary>
					<dl>
						<dt>{m.cgv_snapshot()}</dt><dd>{data.run.snapshot_id}</dd>
						<dt>{m.cgv_rules()}</dt><dd>{data.run.rules_version || '—'}</dd>
						<dt>{m.metric_mgmt_model_label()}</dt><dd>{data.run.annotator_1}</dd>
						<dt>{m.cgv_method()}</dt><dd>{data.run.method === 'agent' ? m.cgv_agent() : m.cgv_panel()}</dd>
						<dt>{m.cgv_annotator_2()}</dt><dd>{data.run.annotator_2 || '—'}</dd>
						<dt>{m.cgv_adjudicator()}</dt><dd>{data.run.adjudicator || '—'}</dd>
						<dt>{m.cgv_base_run()}</dt><dd>{data.run.base_run_id ?? '—'}</dd>
						<dt>{m.cgv_review_run()}</dt><dd>{data.run.review_run_id ?? '—'}</dd>
						<dt>{m.cgv_audit()}</dt><dd>{data.run.audit_notes || '—'}</dd>
					</dl>
				</details>
				<p class="counts">{m.cgv_grounding_counts({ with: governing, without: data.activities.length - governing })}</p>
				{#each data.activities as activity (activity.id)}
					<button class="activity" class:selected={selectedId === activity.id} type="button" onclick={() => selectedId = activity.id}>
						<small>{m.cgv_lines({ start: activity.line_start, end: activity.line_end })}{activity.document_level ? ` · ${m.cgv_document_level()}` : ''}</small>
						<strong>{activityText(activity.activity)}</strong><span>{activity.object}</span>
						<small>{m.cgv_candidate_count({ count: activity.judgements.length })}</small>
					</button>
				{/each}
			{/if}
		</section>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<div class="act-splitter" role="separator" aria-orientation="vertical" aria-label={m.cgv_resize_activities()} tabindex="0"
			aria-valuemin={MIN_ACTIVITIES} aria-valuenow={activitiesWidth}
			onpointerdown={startResizeActivities} onpointermove={moveResizeActivities} onpointerup={endResizeActivities} onpointercancel={endResizeActivities}
			onkeydown={keyResizeActivities}><span class="grip" aria-hidden="true"></span></div>
		<section class="evidence">
			{#if selected && currentInput}
				<h2>{activityText(selected.activity)}</h2>
				<p>{selected.rationale}</p>
				<details><summary>{m.cgv_search_context()}</summary><p>{selected.queries.join('\n') || '—'}</p><p>{selected.cited_docs.join('\n') || '—'}</p></details>
				<div class="source-pair">
				<div class="source">
					{#if sourceUnavailable}<p role="status">{m.cgv_source_unavailable()}</p>{/if}
					{#if isPdf}
						<PdfViewWindow inputId={currentInput.id} {fileUrl} bind:page bind:zoom bind:numPages {darkMode} enableSelectionDialog={false} {highlightPages} highlightVersion={selectedId ?? 0} renderHighlights={highlights} />
					{:else}<pre>{sourceLines.map((l) => `${l.line_number}\t${l.content}`).join('\n') || m.cgv_source_unavailable()}</pre>{/if}
				</div>
				{#if candJudgement}
				<div class="source">
					{#if candUnavailable}<p role="status">{m.cgv_source_unavailable()}</p>{/if}
					{#if candReady}{#key candJudgement.id}
						<PdfViewWindow inputId={candJudgement.candidate_record} fileUrl={candFileUrl} bind:page={candPage} bind:zoom={candZoom} restoreZoom={false} bind:numPages={candNumPages} {darkMode} enableSelectionDialog={false} highlightPages={candHighlightPages} highlightVersion={candJudgement.id} renderHighlights={candHighlights} />
					{/key}{/if}
				</div>
				{/if}
				</div>
			{:else}<p class="empty">{m.cgv_select_activity()}</p>{/if}
		</section>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<div class="splitter" role="separator" aria-orientation="vertical" aria-label={m.cgv_resize_panels()} tabindex="0"
			aria-valuemin={MIN_PANEL} aria-valuenow={panelWidth}
			onpointerdown={startResize} onpointermove={moveResize} onpointerup={endResize} onpointercancel={endResize}
			onkeydown={keyResize}></div>
		<section class="judgements">
			{#if selected && currentInput}
				<h2>{m.cgv_judgements()}</h2>
				<div class="filters">
					<label>{m.cgv_label()}<select bind:value={label}><option value="">{m.cgv_all_labels()}</option>{#each labels as value}<option value={value}>{labelText(value)}</option>{/each}</select></label>
					<label>{m.cgv_search()}<input bind:value={query} placeholder={m.cgv_search_hint()} /></label>
				</div>
				{#each judgements as j (j.id)}
					<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
					<article class:selected={candId === j.id} tabindex="0" onclick={() => selectCandidate(j)}
						onkeydown={(e) => { if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) { selectCandidate(j); e.preventDefault(); } }}>
						<div class="candidate-heading"><strong class="badge" class:governs={j.final_label === 'governs'}>{labelText(j.final_label)}</strong><span>{j.artifact_type} · {j.artifact_id}</span><span>{m.cgv_cosine()}: {j.cosine?.toFixed(4) ?? '—'}</span></div>
						<a href={`/api/v1/kb/inputs/${j.candidate_record}/file`} target="_blank" rel="noopener noreferrer">{j.source_title || m.metric_mgmt_record_4({ id: j.candidate_record })} ↗</a>
						<dl>
							<dt>{m.cgv_rationale()}</dt><dd>{whyRelated(j)}</dd>
							<dt>{m.cgv_found_by()}</dt><dd>{j.found_by || '—'}</dd>
						</dl>
					</article>
				{:else}<p>{m.cgv_no_candidates()}</p>{/each}
			{:else}<p class="empty">{m.cgv_select_activity()}</p>{/if}
		</section>
	</div>
</div>

<style>
	.gold-page { --page-bg:#0e1116; --panel-bg:#161a22; --ink-line:#2a3140; --text-primary:#ede7d3; --text-muted:#aaa38f; --brass:#d4a24c; --card-bg:#303A4B; --card-line:#1F2530; --font-mono:'JetBrains Mono',monospace; display:flex; flex-direction:column; height:100%; min-height:0; color:var(--text-primary); background:var(--page-bg); }
	.gold-page.light { --page-bg:#f5f1e8; --panel-bg:#fbf8f0; --ink-line:#d7cfb8; --text-primary:#1a1410; --text-muted:#786f5f; --brass:#966412; --card-bg:#FFFFFF; --card-line:#E5DEC8; }
	header { display:flex; align-items:center; gap:16px; padding:12px 18px; border-bottom:1px solid var(--ink-line); }
	h1 { margin:0; font:500 28px Georgia,serif; } h2 { font:500 20px Georgia,serif; margin:8px 0 14px; }
	header span,small,.counts { color:var(--text-muted); font:11px/1.6 var(--font-mono); } 
	button,input,select { border:1px solid var(--ink-line); background:var(--panel-bg); color:var(--text-primary); padding:7px 9px; border-radius:4px; }
	button { cursor:pointer; } button:hover,a:hover { color:var(--brass); } button:focus-visible,a:focus-visible,input:focus-visible,select:focus-visible { outline:2px solid var(--brass); outline-offset:2px; }
	.notice { margin:0; padding:9px 18px; color:var(--text-muted); font-size:12px; border-bottom:1px solid var(--ink-line); }
	.workspace { display:grid; grid-template-columns:380px var(--act-w) 12px minmax(0,1fr) 6px var(--panel-w); flex:1; min-height:0; overflow:hidden; }
	.workspace.folded { grid-template-columns:42px var(--act-w) 12px minmax(0,1fr) 6px var(--panel-w); }
	.act-splitter { position:relative; cursor:col-resize; user-select:none; touch-action:none; border-left:1px solid var(--ink-line); margin-left:5px; outline:none; }
	.act-splitter .grip { position:absolute; top:50%; left:-5px; width:8px; height:52px; transform:translateY(-50%); border-radius:999px; border:1px solid var(--card-line); background:radial-gradient(circle,var(--text-muted) 22%,transparent 24%) center 6px / 6px 12px repeat-y,var(--panel-bg); }
	.act-splitter:hover .grip,.act-splitter:focus-visible .grip { border-color:var(--brass); }
	.splitter { cursor:col-resize; user-select:none; touch-action:none; background:var(--ink-line); } .splitter:hover,.splitter:focus-visible { background:var(--brass); outline:none; }
	.judgements { overflow:auto; min-width:0; padding:16px; background:var(--panel-bg); }
	.browser { position:relative; min-height:0; overflow:hidden; border-right:1px solid var(--ink-line); }
	.fold-toggle { position:absolute; top:10px; right:10px; z-index:6; display:inline-flex; align-items:center; justify-content:center; width:28px; height:28px; padding:0; }
	.browser-rail { position:relative; display:flex; align-items:flex-start; justify-content:center; padding-top:10px; border-right:1px solid var(--ink-line); background:var(--panel-bg); }
	.browser-rail .fold-toggle { position:static; }
	.browser-rail span { position:absolute; bottom:18px; left:50%; transform:translateX(-50%) rotate(-90deg); white-space:nowrap; letter-spacing:.16em; text-transform:uppercase; }
	.activities { overflow:auto; min-width:0; padding:16px; } .evidence { display:flex; flex-direction:column; overflow:hidden; min-width:0; min-height:0; padding:16px; } .activities { border-right:1px solid var(--ink-line); background:var(--panel-bg); }
	.activity { background:var(--card-bg); border-color:var(--card-line); display:flex; flex-direction:column; gap:6px; width:100%; margin:9px 0; text-align:left; } .activity:hover { border-color:var(--brass); } .activity.selected { background:#3b82f6; border-color:#3b82f6; box-shadow:inset 4px 0 #1d4ed8; } .activity.selected,.activity.selected small,.activity.selected strong,.activity.selected span { color:#fff; }
	.activity strong,.activity span { min-width:0; max-width:100%; overflow-wrap:anywhere; white-space:normal; }
	.activity strong { font-size:14px; font-weight:500; } .activity span { font-size:12px; } .run { font:12px var(--font-mono); overflow-wrap:anywhere; }
	details { margin-bottom:12px; } summary { cursor:pointer; color:var(--brass); } details p { white-space:pre-wrap; }
	dl { display:grid; grid-template-columns:auto minmax(0,1fr); gap:6px 12px; font-size:12px; } dt { color:var(--text-muted); } dd { margin:0; overflow-wrap:anywhere; white-space:pre-wrap; }
	.source { display:flex; flex-direction:column; flex:1 1 0; min-height:300px; overflow:auto; border:1px solid var(--ink-line); margin:12px 0 0; }
	pre { margin:0; padding:12px; font:12px/1.65 var(--font-mono); white-space:pre-wrap; overflow-wrap:anywhere; }
	.filters { display:flex; flex-wrap:wrap; gap:12px; } label { display:flex; flex-direction:column; gap:5px; font-size:12px; } label:last-child { flex:1; }
	article { margin:12px 0; padding:14px; border:1px solid var(--card-line); background:var(--card-bg); cursor:pointer; }
	article:hover { border-color:var(--brass); } article.selected { background:#3b82f6; border-color:#3b82f6; box-shadow:inset 4px 0 #1d4ed8; }
	article.selected,article.selected .badge,article.selected a,article.selected dt,article.selected dd,article.selected span { color:#fff; }
	.source-pair { display:flex; flex:1 1 0; min-height:300px; gap:8px; } .source-pair .source { flex:1 1 0; min-width:0; margin-top:12px; }	.candidate-heading { display:flex; flex-wrap:wrap; align-items:center; gap:10px; margin-bottom:10px; font:11px var(--font-mono); }
	.badge { padding:4px 7px; border:1px solid var(--ink-line); } .badge.governs { color:var(--brass); border-color:var(--brass); } a { color:var(--brass); font-size:13px; }
	.empty { padding:40px; color:var(--text-muted); } .error { color:#e78a79; }
	@media(max-width:900px) { .workspace,.workspace.folded { display:flex; flex-direction:column; overflow:auto; } .browser { min-height:340px; } .browser-rail { display:none; } .activities,.judgements { overflow:visible; flex-shrink:0; } .act-splitter { display:none; } .activities { overflow:visible; flex-shrink:0; } .evidence { overflow:visible; flex-shrink:0; } .source { flex:none; height:70vh; } .splitter { display:none; } header { flex-wrap:wrap; } }
</style>
