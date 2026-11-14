<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import GoldRunFilters from './gold-run-filters.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import { GoldRunFilter, goldRunBadges } from './gold-run-filter.svelte';
	import { kbInputRecordKey, metricBenchmarkSelection } from './metric-benchmark-record';
	import { getGoldCompliance, getRawLines, type ComplianceGoldData, type ComplianceGoldLabel, type KbInputRecord, type RawLine } from '$lib/services/kbService';
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
	const labels: ComplianceGoldLabel[] = ['governs', 'topical', 'unrelated', 'ambiguous'];
	const selected = $derived(data?.activities.find((a) => a.id === selectedId));
	const judgements = $derived((selected?.judgements ?? []).filter((j) =>
		(!label || j.final_label === label) && (!query.trim() ||
			[j.artifact_id, j.source_title, j.passage, j.reason_1, j.reason_2, j.rationale, j.found_by, String(j.candidate_record)].join(' ').toLowerCase().includes(query.trim().toLowerCase()))));
	const governing = $derived(data?.activities.filter((a) => a.judgements.some((j) => j.final_label === 'governs')).length ?? 0);
	const sourceLines = $derived(selected ? lines.filter((l) => l.line_number >= selected.line_start && l.line_number <= selected.line_end) : []);
	const highlightPages = $derived([...new Set(sourceLines.map((l) => l.page_number))]);
	const fileUrl = $derived(currentInput ? `/api/v1/kb/inputs/${currentInput.id}/file` : '');
	const isPdf = $derived(currentInput?.type?.toLowerCase() === 'pdf');
	onMount(() => { void gold.load(); });
	$effect(() => { if (highlightPages.length) page = highlightPages[0]; });
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
		version++; currentInput = null; data = null; lines = []; selectedId = null;
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
	function highlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		for (const line of sourceLines.filter((l) => l.page_number === pageNo && l.coords?.length >= 4)) {
			const [x1, y1, x2, y2] = line.coords;
			const mark = document.createElement('div'); mark.className = 'pdf-highlight';
			Object.assign(mark.style, { left: `${Math.min(x1,x2)*viewport.width/1000}px`, top: `${Math.min(y1,y2)*viewport.height/1000}px`,
				width: `${Math.abs(x2-x1)*viewport.width/1000}px`, height: `${Math.max(2,Math.abs(y2-y1)*viewport.height/1000)}px` });
			overlay.appendChild(mark);
		}
	}
</script>

<div class="gold-page select-text" class:light={!darkMode}>
	<header><h1>{m.nav_sysadmin_llm_compliance_gold()}</h1><span>{m.cgv_read_only()}</span>
		<button type="button" onclick={() => folded = !folded}>{folded ? m.metric_mgmt_unfold_record_list() : m.metric_mgmt_fold_record_list()}</button></header>
	<p class="notice">{m.cgv_scope()}</p>
	<div class="workspace" class:folded>
		<div class="browser" hidden={folded}>
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
		<section class="activities">
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
						<strong>{activity.activity}</strong><span>{activity.object}</span>
						<small>{m.cgv_candidate_count({ count: activity.judgements.length })}</small>
					</button>
				{/each}
			{/if}
		</section>
		<section class="evidence">
			{#if selected && currentInput}
				<h2>{selected.activity}</h2>
				<p>{selected.rationale}</p>
				<details><summary>{m.cgv_search_context()}</summary><p>{selected.queries.join('\n') || '—'}</p><p>{selected.cited_docs.join('\n') || '—'}</p></details>
				<div class="source">
					{#if sourceUnavailable}<p role="status">{m.cgv_source_unavailable()}</p>{/if}
					{#if isPdf}
						<PdfViewWindow inputId={currentInput.id} {fileUrl} bind:page bind:zoom bind:numPages {darkMode} enableSelectionDialog={false} {highlightPages} highlightVersion={selectedId ?? 0} renderHighlights={highlights} />
					{:else}<pre>{sourceLines.map((l) => `${l.line_number}\t${l.content}`).join('\n') || m.cgv_source_unavailable()}</pre>{/if}
				</div>
				<h2>{m.cgv_judgements()}</h2>
				<div class="filters">
					<label>{m.cgv_label()}<select bind:value={label}><option value="">{m.cgv_all_labels()}</option>{#each labels as value}<option value={value}>{labelText(value)}</option>{/each}</select></label>
					<label>{m.cgv_search()}<input bind:value={query} placeholder={m.cgv_search_hint()} /></label>
				</div>
				<p class="notice">{m.cgv_live_text()}</p>
				{#each judgements as j (j.id)}
					<article>
						<div class="candidate-heading"><strong class="badge" class:governs={j.final_label === 'governs'}>{labelText(j.final_label)}</strong><span>{j.artifact_type} · {j.artifact_id}</span><span>{m.cgv_cosine()}: {j.cosine?.toFixed(4) ?? '—'}</span></div>
						<a href={`/api/v1/kb/inputs/${j.candidate_record}/file`} target="_blank" rel="noopener noreferrer">{j.source_title || m.metric_mgmt_record_4({ id: j.candidate_record })} ↗</a>
						<pre>{j.source_available ? j.passage : m.cgv_source_unavailable()}</pre>
						<dl>
							<dt>{m.cgv_found_by()}</dt><dd>{j.found_by || '—'}</dd>
							<dt>{m.cgv_annotator_1()}</dt><dd>{labelText(j.label_1)} · {j.reason_1 || '—'}</dd>
							{#if j.label_2}<dt>{m.cgv_annotator_2()}</dt><dd>{labelText(j.label_2)} · {j.reason_2 || '—'}</dd>{/if}
							<dt>{m.cgv_rationale()}</dt><dd>{j.rationale || '—'}</dd>
						</dl>
					</article>
				{:else}<p>{m.cgv_no_candidates()}</p>{/each}
			{:else}<p class="empty">{m.cgv_select_activity()}</p>{/if}
		</section>
	</div>
</div>

<style>
	.gold-page { --page-bg:#0e1116; --panel-bg:#161a22; --ink-line:#2a3140; --text-primary:#ede7d3; --text-muted:#aaa38f; --brass:#d4a24c; --font-mono:'JetBrains Mono',monospace; display:flex; flex-direction:column; height:100%; min-height:0; color:var(--text-primary); background:var(--page-bg); }
	.gold-page.light { --page-bg:#f5f1e8; --panel-bg:#fbf8f0; --ink-line:#d7cfb8; --text-primary:#1a1410; --text-muted:#786f5f; --brass:#966412; }
	header { display:flex; align-items:center; gap:16px; padding:12px 18px; border-bottom:1px solid var(--ink-line); }
	h1 { margin:0; font:500 28px Georgia,serif; } h2 { font:500 20px Georgia,serif; margin:8px 0 14px; }
	header span,small,.counts { color:var(--text-muted); font:11px/1.6 var(--font-mono); } header button { margin-left:auto; }
	button,input,select { border:1px solid var(--ink-line); background:var(--panel-bg); color:var(--text-primary); padding:7px 9px; border-radius:4px; }
	button { cursor:pointer; } button:hover,a:hover { color:var(--brass); } button:focus-visible,a:focus-visible,input:focus-visible,select:focus-visible { outline:2px solid var(--brass); outline-offset:2px; }
	.notice { margin:0; padding:9px 18px; color:var(--text-muted); font-size:12px; border-bottom:1px solid var(--ink-line); }
	.workspace { display:grid; grid-template-columns:380px 270px minmax(0,1fr); flex:1; min-height:0; overflow:hidden; }
	.workspace.folded { grid-template-columns:270px minmax(0,1fr); }
	.browser { min-height:0; overflow:hidden; border-right:1px solid var(--ink-line); }
	.activities,.evidence { overflow:auto; min-width:0; padding:16px; } .activities { border-right:1px solid var(--ink-line); background:var(--panel-bg); }
	.activity { display:flex; flex-direction:column; gap:6px; width:100%; margin:9px 0; text-align:left; } .activity.selected { border-color:var(--brass); box-shadow:inset 3px 0 var(--brass); }
	.activity strong { font-size:14px; font-weight:500; } .activity span { font-size:12px; } .run { font:12px var(--font-mono); overflow-wrap:anywhere; }
	details { margin-bottom:12px; } summary { cursor:pointer; color:var(--brass); } details p { white-space:pre-wrap; }
	dl { display:grid; grid-template-columns:auto minmax(0,1fr); gap:6px 12px; font-size:12px; } dt { color:var(--text-muted); } dd { margin:0; overflow-wrap:anywhere; white-space:pre-wrap; }
	.source { display:flex; flex-direction:column; height:380px; min-height:200px; resize:vertical; overflow:auto; border:1px solid var(--ink-line); margin:15px 0 22px; }
	pre { margin:0; padding:12px; font:12px/1.65 var(--font-mono); white-space:pre-wrap; overflow-wrap:anywhere; }
	.filters { display:flex; flex-wrap:wrap; gap:12px; } label { display:flex; flex-direction:column; gap:5px; font-size:12px; } label:last-child { flex:1; }
	article { margin:12px 0; padding:14px; border:1px solid var(--ink-line); background:var(--panel-bg); }
	.candidate-heading { display:flex; flex-wrap:wrap; align-items:center; gap:10px; margin-bottom:10px; font:11px var(--font-mono); }
	.badge { padding:4px 7px; border:1px solid var(--ink-line); } .badge.governs { color:var(--brass); border-color:var(--brass); } a { color:var(--brass); font-size:13px; }
	.empty { padding:40px; color:var(--text-muted); } .error { color:#e78a79; }
	@media(max-width:1100px) { .workspace { grid-template-columns:330px 220px minmax(0,1fr); } }
	@media(max-width:900px) { .workspace,.workspace.folded { display:flex; flex-direction:column; overflow:auto; } .browser { min-height:340px; } .activities,.evidence { overflow:visible; flex-shrink:0; } header { flex-wrap:wrap; } }
</style>
