<script lang="ts">
	import { onDestroy, onMount, type Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { searchAllInputs, type InputRecordSummary } from './metric-review-client.js';
	import { ScoreAPIError } from './metric-score-client';
	import { getRawLines, type RawLine } from '$lib/services/kbService';
	import BenchmarkReportViewer from './benchmark-report-viewer.svelte';
	import type { TableReference } from './pdf-table-geometry';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';
	import {
		summaryGoldRuns, summaryScoreAlgorithms, summaryScoreArtifact, summaryScoreDetail,
		summaryScoreHistory, summaryScoreModels, startSummaryScore,
		type SummaryGoldRun, type SummaryScoreRun
	} from './summary-score-client';

	let { darkMode = true, onSummaryDetailsChange }: { darkMode: boolean; onSummaryDetailsChange?: (details: Snippet | null) => void } = $props();
	const lang = getLocale();
	const comparisonFields = [
		['summary_seq_no', () => m.ssb_summary_sequence()],
		['summary_level', () => m.ssb_summary_level()],
		['summary_text', () => m.sgv_summary_text()],
		['summary_text_en', () => m.sgv_english_summary()],
		['language', () => m.sgv_language()],
		['keywords', () => m.sgv_keywords()],
		['keywords_en', () => m.sgv_keywords_en()],
		['source_line_spans', () => m.ssb_source_lines()],
		['empty_reason', () => m.sgv_empty_reason()]
	] as const;
	function comparisonValue(item: Record<string, any> | null, field: string): string {
		// Gold payloads describe leaf chunks; their level is implicit in the scoring contract.
		const value = field === 'summary_level' && item ? item[field] ?? 0 : item?.[field];
		if (value == null || value === '' || (Array.isArray(value) && value.length === 0)) return '—';
		return typeof value === 'string' ? value : JSON.stringify(value);
	}
	let query = $state('');
	let searching = $state(false);
	let searched = $state(false);
	let error = $state('');
	let documents = $state<InputRecordSummary[]>([]);
	let selected = $state<InputRecordSummary | null>(null);
	let algorithms = $state<string[]>([]);
	let algorithm = $state('');
	let models = $state<string[]>([]);
	let model = $state('');
	let gold = $state<SummaryGoldRun[]>([]);
	let goldChoice = $state('');
	let runs = $state<SummaryScoreRun[]>([]);
	let detail = $state<SummaryScoreRun | null>(null);
	let selectedSummaryPair = $state<{ production: Record<string, any> | null; gold: Record<string, any> | null } | null>(null);
	let rawLines = $state<RawLine[]>([]);
	let sourceLines = $state<RawLine[]>([]);
	let sourcePage = $state(1);
	let sourcePageVersion = $state(0);
	let rawLineSequence = 0;
	let rawLinesRequestKey = '';
	type ReportBodyProps = { selectSource(lines: number[], tableReferences?: TableReference[]): void; selectedSourceLines: number[]; selectedTableReferences: TableReference[] };
	type SourceIdentity = { runId: number; inputId: number; status: SummaryScoreRun['status'] };
	let rawLinesIdentity: SourceIdentity | null = null;
	let pendingSource: { identity: SourceIdentity; lines: number[]; select: ReportBodyProps['selectSource'] } | null = null;
	const sameSourceIdentity = (left: SourceIdentity | null | undefined, right: SourceIdentity) =>
		left?.runId === right.runId && left.inputId === right.inputId && left.status === right.status;
	let starting = $state(false);
	let loading = $state(false);
	let alive = true;
	let sequence = 0;
	let detailSequence = 0;
	let historySequence = 0;
	let timer: ReturnType<typeof setInterval> | undefined;
	function parseSourceLines(value: unknown): number[] {
		const found: number[] = [];
		const add = (candidate: unknown) => {
			if (typeof candidate === 'number' && Number.isInteger(candidate) && candidate > 0) { found.push(candidate); return; }
			if (typeof candidate !== 'string') return;
			const text = candidate.trim();
			const range = text.match(/^(\d+)\s*[:-]\s*(\d+)$/);
			if (range) { const start = Number(range[1]), end = Number(range[2]); if (start > 0 && end >= start) for (let n = start; n <= end; n++) found.push(n); return; }
			if (/^\d+$/.test(text) && Number(text) > 0) found.push(Number(text));
		};
		const visit = (item: unknown) => {
			if (Array.isArray(item)) item.forEach(visit);
			else if (item && typeof item === 'object') { const record = item as Record<string, unknown>; if (record.line_number !== undefined) add(record.line_number); }
			else add(item);
		};
		visit(value);
		return [...new Set(found)].sort((a, b) => a - b);
	}
	function renderSourceHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement, resolvedTableLines: ReadonlySet<number>) {
		for (const line of sourceLines) {
			if (resolvedTableLines.has(line.line_number) || line.page_number !== pageNo || !Array.isArray(line.coords) || line.coords.length < 4) continue;
			const coords = line.coords.slice(0, 4);
			if (!coords.every(Number.isFinite)) continue;
			const [x1, y1, x2, y2] = coords;
			const left = Math.min(x1, x2) * viewport.width / 1000, top = Math.min(y1, y2) * viewport.height / 1000;
			const width = Math.abs(x2 - x1) * viewport.width / 1000, height = Math.abs(y2 - y1) * viewport.height / 1000;
			if (width < 1 || height < 1) continue;
			const mark = document.createElement('div'); mark.className = 'pdf-highlight';
			mark.style.cssText = 'position:absolute;background:rgba(251,191,36,.22);border:1px solid rgba(245,158,11,.8);box-sizing:border-box;pointer-events:none;';
			mark.style.left = `${left}px`; mark.style.top = `${top}px`; mark.style.width = `${width}px`; mark.style.height = `${height}px`; overlay.appendChild(mark);
		}
	}
	function selectSourceLines(lines: number[], select: ReportBodyProps['selectSource']) {
		const byNumber = new Map(rawLines.map((line) => [line.line_number, line]));
		sourceLines = lines.flatMap((number) => { const line = byNumber.get(number); return line ? [line] : []; });
		if (!sourceLines.length) { select([]); return; }
		sourcePage = sourceLines[0].page_number;
		sourcePageVersion++;
		select(sourceLines.map((line) => line.line_number), []);
	}
	function selectSummary(predictionId: unknown, select: ReportBodyProps['selectSource']) {
		const prediction = detail?.input?.predictions?.find((item: any) => String(item.id) === String(predictionId)) ?? null;
		const goldSummary = prediction
			? detail?.input?.gold?.summaries?.find((item: any) => Number(item.summary_seq_no) === Number(prediction.summary_seq_no)) ?? null
			: null;
		selectedSummaryPair = { production: prediction, gold: goldSummary };
		const lines = [...new Set([
			...parseSourceLines(prediction?.source_line_spans),
			...parseSourceLines(goldSummary?.source_line_spans)
		])].sort((a, b) => a - b);
		const identity: SourceIdentity | null = detail ? { runId: detail.id, inputId: detail.input_record_id, status: detail.status } : null;
		if (!lines.length || !identity) { pendingSource = null; sourceLines = []; select([]); return; }
		if (sameSourceIdentity(rawLinesIdentity, identity)) selectSourceLines(lines, select);
		else { pendingSource = { identity, lines, select }; sourceLines = []; select([]); }
	}
	$effect(() => {
		onSummaryDetailsChange?.(selectedSummaryPair ? (summaryDetailsContent as unknown as Snippet) : null);
	});
	$effect(() => {
		const inputId = detail?.input_record_id ?? null;
		const runId = detail?.id ?? null;
		const status = detail?.status ?? null;
		const requestKey = runId == null || inputId == null || status == null ? '' : `${runId}:${inputId}:${status}`;
		if (requestKey === rawLinesRequestKey) return;
		rawLinesRequestKey = requestKey;
		const sequence = ++rawLineSequence;
		rawLines = []; sourceLines = []; rawLinesIdentity = null; pendingSource = null;
		if (inputId == null || runId == null) return;
		const identity: SourceIdentity = { runId, inputId, status: detail!.status };
		void getRawLines(inputId).then((result) => {
			if (!alive || sequence !== rawLineSequence || !sameSourceIdentity(detail ? { runId: detail.id, inputId: detail.input_record_id, status: detail.status } : null, identity)) return;
			rawLines = result.lines ?? [];
			rawLinesIdentity = identity;
			const pending = pendingSource;
			if (pending && sameSourceIdentity(pending.identity, identity)) {
				pendingSource = null;
				if (detail?.status === pending.identity.status) selectSourceLines(pending.lines, pending.select);
				else pending.select([]);
			}
		}).catch(() => {
			if (!alive || sequence !== rawLineSequence || !sameSourceIdentity(detail ? { runId: detail.id, inputId: detail.input_record_id, status: detail.status } : null, identity)) return;
			rawLines = []; rawLinesIdentity = identity;
			const pending = pendingSource;
			if (pending && sameSourceIdentity(pending.identity, identity)) {
				pendingSource = null; pending.select([]);
			}
		});
	});

	const date = (value?: string) => value ? new Date(value).toLocaleString(lang) : '—';
	const status = (value: string) => value === 'done' ? m.ssb_done() : value === 'failed' ? m.ssb_failed() : m.ssb_running();
	const errorText = (value: unknown) => {
		const code = (value instanceof ScoreAPIError ? value.code : '').toUpperCase();
		if (code === 'NO_GOLD') return m.ssb_error_no_gold();
		if (code === 'NO_PREDICTIONS') return m.ssb_error_no_predictions();
		if (code === 'RUN_CONFLICT') return m.ssb_error_running();
		if (code === 'SCORER_UNAVAILABLE') return m.ssb_error_scorer();
		if (code === 'INVALID_JUDGMENTS') return m.ssb_error_judgments();
		if (code === 'STALE_GOLD' || code === 'STALE_PREDICTIONS') return m.ssb_error_stale();
		if (code === 'FORBIDDEN' || code === 'UNAUTHORIZED') return m.ssb_error_forbidden();
		return m.ssb_error();
	};

	async function search() {
		searching = true;
		error = '';
		try {
			documents = await searchAllInputs(query, true, 'summary_rows');
			searched = true;
		} catch (err) { error = errorText(err); }
		finally { searching = false; }
	}

	async function choose(record: InputRecordSummary) {
		detailSequence++;
		historySequence++;
		selectedSummaryPair = null;
		selected = record;
		detail = null;
		runs = [];
		loading = false;
		algorithm = '';
		goldChoice = '';
		gold = [];
		algorithms = [];
		const current = ++sequence;
		error = '';
		try {
			const [foundAlgorithms, foundGold] = await Promise.all([
				summaryScoreAlgorithms(record.id), summaryGoldRuns(record.id)
			]);
			if (!alive || current !== sequence) return;
			algorithms = foundAlgorithms;
			gold = foundGold;
			if (foundAlgorithms.length === 1) algorithm = foundAlgorithms[0];
			void history();
		} catch (err) { if (current === sequence) error = errorText(err); }
	}

	async function history(quiet = false) {
		const current = ++historySequence;
		const recordId = selected?.id;
		if (!quiet) loading = true;
		try {
			const result = await summaryScoreHistory(recordId);
			if (alive && current === historySequence && selected?.id === recordId) runs = result.runs ?? [];
		} catch (err) { if (alive && current === historySequence && selected?.id === recordId) error = errorText(err); }
		finally { if (current === historySequence) loading = false; }
	}

	async function open(id: number, quiet = false) {
		const current = ++detailSequence;
		const selectedId = selected?.id;
		if (!quiet) selectedSummaryPair = null;
		try {
			const result = await summaryScoreDetail(id);
			if (alive && current === detailSequence && selected?.id === selectedId && (selectedId == null || result.input_record_id === selectedId)) detail = result;
		} catch (err) { if (alive && current === detailSequence && selected?.id === selectedId && !quiet) error = errorText(err); }
	}

	async function run() {
		if (!selected || !algorithm || !model || starting) return;
		const recordId = selected.id;
		selectedSummaryPair = null;
		const current = ++detailSequence;
		starting = true;
		error = '';
		try {
			const result = await startSummaryScore({
				record_id: recordId, algorithm, model, lang,
				gold: gold.find((item) => JSON.stringify([item.skill_version, item.model_name, item.benchmark_run_id]) === goldChoice)
			});
			if (alive && current === detailSequence && selected?.id === recordId && result.input_record_id === recordId) {
				detail = result;
				await history();
			}
		} catch (err) { if (alive && current === detailSequence && selected?.id === recordId) error = errorText(err); }
		finally { starting = false; }
	}

	onMount(() => {
		void search();
		void Promise.all([summaryScoreModels().then((result) => { models = result; model = result[0] ?? ''; }), history()]).catch((err) => error = errorText(err));
		timer = setInterval(() => {
			if (runs.some((item) => item.status === 'running')) void history(true);
			if (detail?.status === 'running') void open(detail.id, true);
		}, 4000);
	});
	onDestroy(() => { alive = false; if (timer) clearInterval(timer); onSummaryDetailsChange?.(null); });
</script>

<div class="summary-benchmark select-text" class:light={!darkMode}>
	<header><h1>{m.ssb_title()}</h1><p>{m.ssb_subtitle()}</p></header>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
	<section class="card">
		<h2>{m.ssb_document()}</h2>
		<form class="search" onsubmit={(event) => { event.preventDefault(); void search(); }}>
			<input bind:value={query} placeholder={m.ssb_search_placeholder()} aria-label={m.ssb_document()} />
			<button disabled={searching}>{searching ? m.ssb_searching() : m.ssb_search()}</button>
		</form>
		{#if documents.length}<div class="documents">{#each documents as document (document.id)}
			<button class:chosen={selected?.id === document.id} onclick={() => void choose(document)}>
				<span class="document-id-row"><span>{m.ssb_record({ id: document.id })}</span><span title={document.create_time || ''}>{date(document.create_time)}</span></span>
				<strong class="document-field" title={document.title || ''}>{document.title || document.file_name || '—'}</strong>
				<small class="document-field" title={document.doc_no || ''}>{document.doc_no || '—'}</small>
			</button>
		{/each}</div>{:else if searched}<p class="muted">{m.ssb_no_documents()}</p>{/if}
		{#if selected}<p class="selected">{selected.title || selected.file_name} · {m.ssb_record({ id: selected.id })}</p>{:else}<p class="muted select-prompt">{m.ssb_select_document()}</p>{/if}
		<div class="controls">
			<label>{m.ssb_algorithm()}<select bind:value={algorithm} disabled={!selected || algorithms.length === 0}><option value="">{m.ssb_choose_algorithm()}</option>{#each algorithms as item}<option value={item}>{item}</option>{/each}</select></label>
			<label>{m.ssb_gold()}<select bind:value={goldChoice} disabled={!selected}><option value="">{m.ssb_auto_gold()}</option>{#each gold as item}<option value={JSON.stringify([item.skill_version, item.model_name, item.benchmark_run_id])}>{item.skill_version} · {item.model_name} · {item.benchmark_run_id} · {m.ssb_rows({ count: item.rows })}</option>{/each}</select></label>
			<label>{m.ssb_model()}<select bind:value={model}><option value="" disabled>{m.ssb_choose_model()}</option>{#each models as item}<option value={item}>{item}</option>{/each}</select></label>
			<button class="primary" disabled={!selected || !algorithm || !model || starting} onclick={() => void run()}>{starting ? m.ssb_starting() : m.ssb_run()}</button>
		</div>
	</section>

	<section class="card">
		<div class="section-head"><h2>{m.ssb_history()}</h2><button disabled={loading} onclick={() => void history()}>{m.ssb_refresh()}</button></div>
		{#if loading}<p role="status" class="muted">{m.ssb_loading()}</p>{/if}
		<div class="table-wrap"><table><thead><tr><th>{m.ssb_document()}</th><th>{m.ssb_algorithm()}</th><th>{m.ssb_model()}</th><th>{m.ssb_created()}</th><th>{m.ssb_status()}</th><th>{m.ssb_score()}</th><th></th></tr></thead>
		<tbody>{#each runs as item}<tr class:chosen={detail?.id === item.id}><td>{item.title || m.ssb_record({ id: item.input_record_id })}<small>{m.ssb_record({ id: item.input_record_id })}</small></td><td>{item.algorithm}</td><td>{item.model_name}</td><td>{date(item.created_at)}</td><td>{status(item.status)}</td><td>{item.score?.score ?? '—'}</td><td><button onclick={() => void open(item.id)}>{m.ssb_open()}</button></td></tr>{/each}</tbody></table></div>
		{#if !loading && !runs.length}<p class="muted">{m.ssb_empty_history()}</p>{/if}
	</section>

	{#if detail}<BenchmarkReportViewer
		run={{ id: detail!.id, title: m.ssb_results({ id: detail!.id }), status: status(detail!.status), documentTitle: `${detail!.title} · ${m.ssb_record({ id: detail!.input_record_id })} · ${detail!.algorithm} · ${detail!.model_name}`, createdAt: date(detail!.created_at) }}
		scoreCards={detail!.score ? [{ label: m.ssb_score(), value: String(detail!.score.score), detail: m.ssb_out_of_100() }, ...Object.entries(detail!.score.components ?? {}).map(([name, value]) => ({ label: name, value: value == null ? m.ssb_na() : `${Math.round(Number(value) * 1000) / 10}%`, detail: m.ssb_weight({ value: detail!.score!.weights?.[name] ?? 0 }) }))] : []}
		warnings={detail!.input?.warnings ?? []}
		evidenceActions={(['input', 'judgments', 'score', 'report'] as const).map((kind) => ({ id: kind, label: kind === 'input' ? m.ssb_input() : kind === 'judgments' ? m.ssb_judgments() : kind === 'score' ? m.ssb_score_json() : m.ssb_report(), view: () => { window.location.href = summaryScoreArtifact(detail!.id, kind); } }))}
		inputId={detail!.input_record_id}
		{sourcePage}
		{sourcePageVersion}
		darkMode={darkMode}
		renderSourceHighlights={renderSourceHighlights}
	>
	{#snippet reportBody(props: ReportBodyProps)}
		{@const selectSource = props.selectSource}
		<button class="close-report" onclick={() => { detailSequence++; pendingSource = null; selectedSummaryPair = null; detail = null; }}>{m.ssb_close()}</button>
		<p>{detail!.title} · {m.ssb_record({ id: detail!.input_record_id })} · {detail!.algorithm} · {detail!.model_name} · {date(detail!.created_at)}</p>
		{#if detail!.status === 'running'}<p role="status">{m.ssb_background()}</p>{/if}
		{#if detail!.status === 'failed'}<p class="error" role="alert">{errorText(new ScoreAPIError(detail!.error_code ?? '', 500))}</p>{/if}
		{#if detail!.input?.gold}<p>{m.ssb_gold_identity()} · {detail!.input.gold.skill_version} · {detail!.input.gold.model_name} · {detail!.input.gold.run_id}</p>{/if}
		{#if detail!.input?.provenance}<details><summary>{m.ssb_provenance()}</summary><pre>{JSON.stringify(detail!.input.provenance, null, 2)}</pre></details>{/if}
		{#if detail!.score}
			<h3>{m.ssb_components()}</h3>
			<p class="muted">{m.ssb_alignment({ matched: detail!.score.matched, gold: detail!.score.gold_count, candidate: detail!.score.prediction_count })}</p>
		{/if}
		{#if detail!.matches?.chunks}<h3>{m.ssb_chunk_reviews()}</h3>{#each detail!.matches.chunks as chunk}<details class="chunk-review" class:selected={String(selectedSummaryPair?.production?.id) === String(chunk.pred)}><summary onclick={() => {
			selectSummary(chunk.pred, selectSource);
		}}><span class="chunk-source">{m.ssb_chunk({ id: chunk.pred })}</span> · {chunk.diagnostics}</summary><pre>{JSON.stringify(chunk, null, 2)}</pre></details>{/each}{/if}
		{#if detail!.report}<details><summary>{m.ssb_report()}</summary><pre>{detail!.report}</pre></details>{/if}
	{/snippet}
	</BenchmarkReportViewer>
	{/if}
</div>

{#snippet summaryDetailsContent()}
	{#if selectedSummaryPair}
		<div class="summary-comparison" class:light={!darkMode}>
			{#each [
				{ label: m.ssb_production_summary(), item: selectedSummaryPair.production },
				{ label: m.ssb_gold_summary(), item: selectedSummaryPair.gold }
			] as column (column.label)}
				<div class="summary-column">
					<h2>{column.label}</h2>
					{#if !column.item}<p>{m.ssb_no_aligned_summary()}</p>{/if}
				</div>
			{/each}
			{#each comparisonFields as [field, label]}
				{@const productionValue = comparisonValue(selectedSummaryPair.production, field)}
				{@const goldValue = comparisonValue(selectedSummaryPair.gold, field)}
				<div class="comparison-row" class:differs={productionValue !== goldValue}>
					<div class="comparison-label">{label()}</div>
					<div class="comparison-value">{productionValue}</div>
					<div class="comparison-value">{goldValue}</div>
				</div>
			{/each}
		</div>
	{/if}
{/snippet}

<style>
	.summary-benchmark{--bg:#11151d;--panel:#191f2a;--line:#303847;--text:#ece8dd;--muted:#a6a18f;--surface:#252a3a;--accent:#d3a64f;--selected-accent:#818cf8;min-height:100%;padding:24px;color:var(--text);background:var(--bg);font:14px/1.5 system-ui,sans-serif;user-select:text}
	.summary-benchmark :global(button),.summary-benchmark :global(a){user-select:none}.chunk-source{font-weight:600}.chunk-review.selected{background:color-mix(in srgb,var(--selected-accent) 18%,var(--surface));border-radius:6px;box-shadow:inset 3px 0 var(--selected-accent)}.chunk-review summary{padding:8px;user-select:text}
	.summary-comparison{--line:#2d3348;--panel:#252a3a;--text:#e2e8f0;--muted:#94a3b8;--accent:#818cf8;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));column-gap:12px;padding:12px;border:1px solid var(--line);border-radius:12px;background:var(--panel);color:var(--text);user-select:text}.summary-comparison.light{--line:#e4e6eb;--panel:#fff;--text:#111827;--muted:#6b7280;--accent:#6366f1}.summary-column{min-width:0;padding:0 8px 8px}.summary-column h2{margin:0;color:var(--accent);font-size:12px}.summary-column p{font-size:12px;color:var(--muted)}.comparison-row{grid-column:span 2;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:2px 12px;padding:6px 8px;border-bottom:1px solid var(--line);border-radius:6px}.comparison-row.differs{background:rgba(251,191,36,.12);border-color:rgba(251,191,36,.35)}.comparison-label{grid-column:span 2;color:var(--muted);font-size:9px;font-weight:600;text-transform:uppercase}.comparison-value{min-width:0;white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px}
	.summary-benchmark.light{--bg:#f5f2e9;--panel:#fffdf7;--line:#d9d3c5;--text:#211e18;--muted:#716b5e;--surface:#eceef2;--accent:#96701f;--selected-accent:#6366f1}
	header h1{margin:0;font-size:24px}header p,.muted{color:var(--muted)}header p{margin:4px 0 20px}.card{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:16px;margin:14px 0}.card h2{margin:0 0 12px;font-size:16px}.search,.controls,.section-head{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.search input{flex:1;min-width:200px}.controls{margin-top:14px;align-items:end}.controls label{display:grid;gap:5px;color:var(--muted)}input,select,button{font:inherit;color:var(--text);background:var(--bg);border:1px solid var(--line);border-radius:6px;padding:8px 10px}button{cursor:pointer;text-decoration:none}button:hover{border-color:var(--accent)}button:disabled{opacity:.55;cursor:default}.primary{background:var(--accent);color:#17130b;border-color:var(--accent);font-weight:700}.documents{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(100%,290px),1fr));gap:8px;margin-top:12px;max-height:180px;overflow:auto}.documents button{min-width:0;text-align:left;display:flex;flex-direction:column;gap:4px;background:var(--surface)}.document-id-row{display:flex;justify-content:space-between;gap:8px;width:100%;color:var(--muted);font-size:11px}.document-id-row span{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.document-field{display:block;width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.documents small,td small{color:var(--muted);font-size:12px}.documents button.chosen{background:color-mix(in srgb,var(--selected-accent) 22%,var(--surface));border-color:var(--selected-accent)}.chosen{outline:1px solid var(--accent)}.selected{margin:12px 0 0}.select-prompt{margin:14px 0 0}.section-head{justify-content:space-between}.section-head h2{margin:0}.table-wrap{overflow:auto}table{border-collapse:collapse;width:100%;text-align:left}th,td{padding:9px;border-bottom:1px solid var(--line);white-space:nowrap}td:first-child{white-space:normal}td small{display:block}h3{margin:18px 0 8px}details{border-top:1px solid var(--line);padding:10px 0}summary{cursor:pointer}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:var(--bg);padding:12px;border-radius:6px;max-height:500px;overflow:auto}.error{color:#ff8f7e;background:#4b211e;padding:10px;border-radius:6px}.summary-benchmark :global(button),.summary-benchmark :global(a){user-select:none}
	@media(max-width:760px){.summary-benchmark{padding:14px}.controls{align-items:stretch}.controls label,.controls button{width:100%}}
</style>
