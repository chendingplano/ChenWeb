<script lang="ts">
	import { onMount, onDestroy, tick, type Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { getRawLines, listKbMetrics, type RawLine } from '$lib/services/kbService';
	import BenchmarkReportViewer from './benchmark-report-viewer.svelte';
	import type { TableReference } from './pdf-table-geometry';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';
	import { searchInputs, type InputRecordSummary } from './metric-review-client.js';
	import {
		scoreModels,
		goldRuns,
		goldRunKey,
		scoreHistory,
		scoreDetail,
		startScore,
		scoreArtifact,
		ScoreAPIError,
		type ScoreRun,
		type GoldRun,
		type ScoreSummary
	} from './metric-score-client.js';
	let {
		darkMode = true,
		onSelectMetrics = (_metrics: { production: Record<string, unknown> | null; gold: Record<string, unknown> | null } | null) => {}
	}: {
		darkMode: boolean;
		onSelectMetrics?: (metrics: { production: Record<string, unknown> | null; gold: Record<string, unknown> | null } | null) => void;
	} = $props();
	let query = $state(''),
		searchScope = $state('gold'),
		searching = $state(false),
		searched = $state(false),
		starting = $state(false),
		loading = $state(false),
		error = $state('');
	let documents = $state<InputRecordSummary[]>([]),
		selected = $state<InputRecordSummary | null>(null);
	let models = $state<string[]>([]),
		model = $state(''),
		gold = $state<GoldRun[]>([]),
		goldChoice = $state('');
	let runs = $state<ScoreRun[]>([]),
		total = $state(0),
		offset = $state(0),
		filter = $state('all'),
		detail = $state<ScoreRun | null>(null);
	let pairFilter = $state('all');
	let evidenceKind = $state('');
	let activeInputId = $derived(detail?.input_record_id ?? selected?.id ?? null);
	let rawLines = $state<RawLine[]>([]);
	let sourceLines = $state<RawLine[]>([]);
	let tableReferences = $state<TableReference[]>([]);
	let sourcePage = $state(1);
	let sourcePageVersion = $state(0);
	// source_table_rows are not in the scorer export; read them from the metric tables, keyed by metric_id.
	let tableRowsById = $state(new Map<string, TableReference[]>());
	let tableRowsKey = '';
	let previousInputId: number | null | undefined;
	let previousRunId: number | null | undefined;
	type ReportBodyProps = {
		selectSource(lines: number[], tableReferences?: TableReference[]): void;
		selectedSourceLines: number[];
		selectedTableReferences: TableReference[];
	};
	let frameSelectSource: (lines: number[], refs?: TableReference[]) => void = () => {};
	let splitPanelsEl = $state<HTMLDivElement | null>(null);
	let rawLinesReady = $state(false);
	let pendingSourceEntry = $state<Array<{ id: string; prediction: boolean }> | null>(null);
	let selectedEntryKey = $state('');
	let rawLineSequence = 0;
	const evidenceLabels: Record<string, () => string> = {
		input: m.msc_input_download,
		matches: m.msc_matches_download,
		score: m.msc_score_download,
		report: m.msc_report_download
	};
	const json = (value: unknown) => JSON.stringify(value, null, 2);
	const failureLabels: Record<string, () => string> = {
		kind: m.msc_kind_incorrect,
		unit: m.msc_unit_incorrect,
		lines: m.msc_lines_incorrect,
		value: m.msc_value_incorrect,
		range_type: m.msc_range_type_incorrect
	};
	const filteredPairs = $derived(
		(detail?.score?.pairs ?? []).filter(
			(pair) =>
				pairFilter === 'all' ||
				(pairFilter === 'full'
					? pair.credit === 1
					: pair.credit < 1 && (pairFilter === 'partial' || pair.checks[pairFilter] === false))
		)
	);
	const evidenceText = $derived(
		evidenceKind === 'report'
			? (detail?.report ?? '')
			: json(detail?.[evidenceKind as 'input' | 'matches' | 'score'])
	);
	async function downloadEvidence(kind: string) {
		if (detail) {
			const id = detail.id;
			try {
				const response = await fetch(scoreArtifact(id, kind), { credentials: 'same-origin' });
				if (!response.ok) throw new Error();
				const url = URL.createObjectURL(await response.blob());
				const link = document.createElement('a');
				link.href = url;
				link.download = `benchmark-${id}-${kind}.${kind === 'report' ? 'md' : 'json'}`;
				link.click();
				setTimeout(() => URL.revokeObjectURL(url), 1000);
			} catch (e) {
				error = errorText(e);
			}
		}
	}
	const evidenceActions = $derived(Object.entries(evidenceLabels).flatMap(([kind, label]) => detail?.[kind as 'input' | 'matches' | 'score' | 'report'] ? [{ id: kind, label: label(), view: () => { evidenceKind = kind; }, download: () => { void downloadEvidence(kind); } }] : []));

	let timer: ReturnType<typeof setInterval> | undefined;
	let alive = true,
		historySequence = 0,
		detailSequence = 0,
		documentSequence = 0;
	let historyInFlight = 0,
		detailInFlight = 0;
	const lang = getLocale();
	const pct = (value: number | null | undefined) =>
		value == null
			? m.msc_na()
			: new Intl.NumberFormat(lang, { style: 'percent', maximumFractionDigits: 1 }).format(value);
	const date = (value?: string) => (value ? new Date(value).toLocaleString(lang) : m.msc_na());
	const status = (value: string) =>
		value === 'done' ? m.msc_done() : value === 'failed' ? m.msc_failed() : m.msc_running();
	const fields: Record<string, () => string> = {
		value: m.msc_field_value,
		range_type: m.msc_field_range_type,
		kind: m.msc_field_kind,
		unit: m.msc_field_unit,
		lines: m.msc_field_lines
	};
	const comparedFields: Record<string, string[]> = {
		value: ['metric_value', 'value'],
		range_type: ['value_range_type', 'range_type'],
		kind: ['value_class', 'statement_kind', 'kind'],
		unit: ['metric_unit', 'unit'],
		lines: ['source_line_spans', 'source_lines', 'lines']
	};
	function comparedValue(id: string, prediction: boolean, field: string) {
		const row = (prediction ? detail?.input?.predictions : detail?.input?.gold)?.find(
			(row) => row.metric_id === id
		);
		const keys = comparedFields[field] ?? [];
		const value = keys.map((key) => row?.[key]).find((candidate) => candidate != null && candidate !== '');
		if (value == null) return '—';
		return typeof value === 'string' ? value : JSON.stringify(value);
	}
	const causes: Record<string, () => string> = {
		gold_excluded: m.msc_cause_gold_excluded,
		duplicate: m.msc_cause_duplicate,
		not_in_gold: m.msc_cause_not_in_gold,
		invalid: m.msc_cause_invalid
	};
	function errorText(value: unknown) {
		const code = (
			value instanceof ScoreAPIError ? value.code : typeof value === 'string' ? value : ''
		).toUpperCase();
		if (code === 'NO_GOLD') return m.msc_error_no_gold();
		if (code === 'NO_PREDICTIONS') return m.msc_error_no_predictions();
		if (code === 'RUN_CONFLICT') return m.msc_error_running();
		if (code === 'FORBIDDEN' || code === 'UNAUTHORIZED') return m.msc_error_forbidden();
		if (code === 'RUN_NOT_FOUND' || code === 'DOCUMENT_NOT_FOUND') return m.msc_error_not_found();
		if (code === 'UNKNOWN_MODEL' || code === 'MODEL_UNAVAILABLE') return m.msc_error_model();
		return m.msc_error();
	}
	async function search() {
		searching = true;
		error = '';
		try {
			const found = await searchInputs(query, searchScope === 'gold');
			if (alive) {
				documents = found;
				searched = true;
			}
		} catch (e) {
			error = errorText(e);
		} finally {
			searching = false;
		}
	}
	async function choose(document: InputRecordSummary) {
		selected = document;
		goldChoice = '';
		gold = [];
		error = '';
		const seq = ++documentSequence;
		if (filter === 'selected') {
			offset = 0;
			void history();
		}
		try {
			const found = await goldRuns(document.id);
			if (alive && seq === documentSequence) gold = found;
		} catch (e) {
			if (seq === documentSequence) error = errorText(e);
		}
	}
	async function history(quiet = false) {
		if (quiet && historyInFlight > 0) return;
		historyInFlight++;
		const seq = ++historySequence;
		if (!quiet) loading = true;
		try {
			const result = await scoreHistory(filter === 'selected' ? selected?.id : undefined, offset);
			if (alive && seq === historySequence) {
				runs = result.runs ?? [];
				total = result.total;
			}
		} catch (e) {
			if (alive && seq === historySequence) error = errorText(e);
		} finally {
			historyInFlight--;
			if (seq === historySequence) loading = false;
		}
	}
	// The results panel sits below the history table, so bring it into view when a run is opened.
	async function showResults() {
		await tick();
		splitPanelsEl?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}
	async function open(id: number, quiet = false) {
		if (quiet && (detailInFlight > 0 || starting)) return;
		detailInFlight++;
		const seq = ++detailSequence;
		if (!quiet) {
			error = '';
			pairFilter = 'all';
			evidenceKind = '';
		}
		try {
			const result = await scoreDetail(id);
			if (alive && seq === detailSequence) {
				detail = result;
				if (!quiet) void showResults();
			}
		} catch (e) {
			if (alive && seq === detailSequence) error = errorText(e);
		} finally {
			detailInFlight--;
		}
	}
	async function run() {
		if (!selected || !model || starting) return;
		starting = true;
		error = '';
		try {
			detailSequence++;
			pairFilter = 'all';
			evidenceKind = '';
			detail = await startScore(
				selected.id,
				model,
				lang,
				goldChoice ? gold.find((g) => goldRunKey(g) === goldChoice) : undefined
			);
			void showResults();
			offset = 0;
			await history();
		} catch (e) {
			error = errorText(e);
		} finally {
			starting = false;
		}
	}
	function rowName(id: string, prediction = false) {
		const row = (prediction ? detail?.input?.predictions : detail?.input?.gold)?.find(
			(row) => row.metric_id === id
		);
		return row
			? [
					id,
					row.name ?? row.metric_name,
					row.value ?? row.metric_value,
					row.unit ?? row.metric_unit
				]
					.filter(Boolean)
					.join(' · ')
			: id;
	}
	function parseSourceLines(value: unknown): number[] {
		const found: number[] = [];
		const add = (candidate: unknown) => {
			if (typeof candidate === 'number' && Number.isInteger(candidate) && candidate > 0) {
				found.push(candidate);
				return;
			}
			if (typeof candidate !== 'string') return;
			const text = candidate.trim();
			const range = text.match(/^(\d+)\s*[:,-]\s*(\d+)$/);
			if (range) {
				const start = Number(range[1]);
				const end = Number(range[2]);
				if (start > 0 && end >= start) {
					for (let line = start; line <= Math.min(end, start + 199); line++) found.push(line);
				}
				return;
			}
			if (/^\d+$/.test(text) && Number(text) > 0) found.push(Number(text));
		};
		const visit = (item: unknown) => {
			if (Array.isArray(item)) {
				for (const nested of item) visit(nested);
			} else if (item && typeof item === 'object') {
				const record = item as Record<string, unknown>;
				for (const key of ['line_number', 'line', 'line_no', 'lineNo']) {
					if (record[key] !== undefined) add(record[key]);
				}
			} else add(item);
		};
		visit(value);
		return [...new Set(found)].sort((a, b) => a - b);
	}
	function recordSourceLines(id: string, prediction: boolean): number[] {
		const rows = prediction ? detail?.input?.predictions : detail?.input?.gold;
		const record = rows?.find((item) => item.metric_id === id);
		return parseSourceLines(record?.source_line_spans);
	}
	function selectSourceEntry(entryKey: string, ids: Array<{ id: string; prediction: boolean }>, select = frameSelectSource) {
		frameSelectSource = select;
		selectedEntryKey = entryKey;
		const productionId = ids.find((item) => item.prediction)?.id;
		const goldId = ids.find((item) => !item.prediction)?.id;
		const production = detail?.input?.predictions.find((item) => item.metric_id === productionId) ?? null;
		const goldMetric = detail?.input?.gold.find((item) => item.metric_id === goldId) ?? null;
		onSelectMetrics(production || goldMetric ? { production, gold: goldMetric } : null);
		pendingSourceEntry = ids;
		resolveSourceEntry(ids, frameSelectSource);
	}
	function resolveSourceEntry(ids: Array<{ id: string; prediction: boolean }>, select = frameSelectSource) {
		sourceLines = [];
		tableReferences = [];
		select([], []);
		if (!rawLinesReady) return;
		const refs = ids.flatMap(({ id }) => tableRowsById.get(id) ?? []);
		const numbers = [...new Set([...ids.flatMap(({ id, prediction }) => recordSourceLines(id, prediction)), ...refs.map((ref) => ref.line)])].sort(
			(a, b) => a - b
		);
		if (!numbers.length) { select([], refs); return; }
		const byNumber = new Map(rawLines.map((line) => [line.line_number, line]));
		const resolved = numbers.flatMap((number) => {
			const line = byNumber.get(number);
			return line ? [line] : [];
		});
		if (!resolved.length) { select([], []); return; }
		sourceLines = resolved;
		tableReferences = refs;
		sourcePage = resolved[0].page_number;
		sourcePageVersion++;
		select(resolved.map((line) => line.line_number), refs);
	}
	function renderSourceHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement, resolvedTableLines: ReadonlySet<number>) {
		for (const line of sourceLines) {
			// Table lines are highlighted row by row by the PDF viewer; whole line only when no row was located.
			if (resolvedTableLines.has(line.line_number)) continue;
			if (line.page_number !== pageNo || !Array.isArray(line.coords) || line.coords.length < 4) continue;
			const coords = line.coords.slice(0, 4);
			if (!coords.every(Number.isFinite)) continue;
			const [x1, y1, x2, y2] = coords;
			const left = Math.min(x1, x2) * viewport.width / 1000;
			const top = Math.min(y1, y2) * viewport.height / 1000;
			const width = Math.abs(x2 - x1) * viewport.width / 1000;
			const height = Math.abs(y2 - y1) * viewport.height / 1000;
			if (width < 1 || height < 1) continue;
			const mark = document.createElement('div');
			mark.className = 'pdf-highlight';
			mark.style.left = `${left}px`;
			mark.style.top = `${top}px`;
			mark.style.width = `${width}px`;
			mark.style.height = `${height}px`;
			overlay.appendChild(mark);
		}
	}
	$effect(() => {
		const inputId = activeInputId;
		const runId = detail?.id ?? null;
		if (inputId !== previousInputId) {
			previousInputId = inputId;
			sourcePage = 1;
			sourceLines = [];
			pendingSourceEntry = null;
			selectedEntryKey = '';
			rawLinesReady = false;
			rawLines = [];
			const sequence = ++rawLineSequence;
			if (inputId != null) {
				void getRawLines(inputId).then((result) => {
					if (alive && sequence === rawLineSequence && inputId === activeInputId) {
						rawLines = result.lines ?? [];
						rawLinesReady = true;
						if (pendingSourceEntry) resolveSourceEntry(pendingSourceEntry);
					}
				}).catch(() => {
					if (alive && sequence === rawLineSequence && inputId === activeInputId) {
						rawLines = [];
						rawLinesReady = true;
						pendingSourceEntry = null;
						sourceLines = [];
						frameSelectSource([], []);
								}
				});
			}
		}
		const goldRun = detail?.input?.gold_run;
		const tableKey = `${inputId}|${runId}`;
		if (inputId != null && runId != null && goldRun && tableKey !== tableRowsKey) {
			tableRowsKey = tableKey;
			tableRowsById = new Map();
			void Promise.all([
				listKbMetrics(inputId),
				listKbMetrics(inputId, 'testbed', {
					skillVersion: String(goldRun.skill_version ?? ''),
					modelName: String(goldRun.model_name ?? ''),
					runId: String(goldRun.benchmark_run_id ?? '')
				})
			]).then((results) => {
				if (!alive || tableKey !== tableRowsKey) return;
				const byId = new Map<string, TableReference[]>();
				for (const res of results)
					for (const row of res.results ?? [])
						if (row.metric_id && row.source_table_rows?.length) byId.set(row.metric_id, row.source_table_rows);
				tableRowsById = byId;
				if (pendingSourceEntry) resolveSourceEntry(pendingSourceEntry);
			}).catch(() => { /* highlight falls back to whole lines */ });
		}
		if (runId !== previousRunId) {
			previousRunId = runId;
			tableReferences = [];
			sourceLines = [];
			pendingSourceEntry = null;
			selectedEntryKey = '';
		}
	});

	onMount(() => {
		void search();
		void history();
		void scoreModels()
			.then((result) => {
				if (alive) {
					models = result;
					model = result[0] ?? '';
				}
			})
			.catch((e) => {
				error = errorText(e);
			});
		timer = setInterval(() => {
			if (detail?.status === 'running') void open(detail.id, true);
			if (runs.some((run) => run.status === 'running') || detail?.status === 'running')
				void history(true);
		}, 4000);
	});
	onDestroy(() => {
		alive = false;
		if (timer) clearInterval(timer);
	});
</script>

{#snippet summary(result: ScoreSummary)}
	<div class="stats">
		{#each [[m.msc_score(), result.score], [m.msc_soft_precision(), pct(result.soft_precision)], [m.msc_soft_recall(), pct(result.soft_recall)], [m.msc_precision(), pct(result.precision)], [m.msc_recall(), pct(result.recall)], [m.msc_gold_rows(), result.gold_rows], [m.msc_predicted_rows(), result.predicted_rows], [m.msc_matched(), result.matched]] as stat}
			<div><span>{stat[0]}</span><strong>{stat[1]}</strong></div>
		{/each}
	</div>
{/snippet}

<div class="benchmark select-text" class:light={!darkMode}>
	<header>
		<h1>{m.msc_title()}</h1>
		<p>{m.msc_subtitle()}</p>
	</header>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
	<section class="card">
		<h2>{m.msc_document()}</h2>
		<form
			onsubmit={(event) => {
				event.preventDefault();
				void search();
			}}
			class="search"
		>
			<input
				bind:value={query}
				placeholder={m.msc_search_placeholder()}
				aria-label={m.msc_document()}
			/>
			<select
				aria-label={m.msc_search()}
				bind:value={searchScope}
				disabled={searching}
				onchange={() => void search()}
				><option value="all">{m.msc_all()}</option><option value="gold"
					>{m.msc_with_gold_metrics()}</option
				></select
			>
		</form>
		{#if documents.length}<div class="documents">
				{#each documents as document (document.id)}<button
						class:chosen={selected?.id === document.id}
						onclick={() => choose(document)}
						><span class="document-id-row"
							><span>{m.msc_record({ id: document.id })}</span><span
								title={document.create_time || ''}
								>{date(document.create_time)}</span
							></span
						><span class="document-field" title={document.title || ''}
							>{document.title || '—'}</span
						><span class="document-field" title={document.doc_no || ''}
							>{document.doc_no || '—'}</span
						><span class="document-field" title={document.file_name || ''}
							>{document.file_name || '—'}</span
					></button
					>{/each}
			</div>{:else if searched}<p class="muted">{m.msc_no_documents()}</p>{/if}
		{#if selected}<p>
				<strong>{selected.title || selected.file_name || m.msc_record({ id: selected.id })}</strong>
				· {m.msc_record({ id: selected.id })}
			</p>{:else}<p class="muted">{m.msc_select_document()}</p>{/if}
		<div class="controls">
			<label
				>{m.msc_model()}<select bind:value={model}
					><option value="" disabled>{m.msc_choose_model()}</option>{#each models as name}<option
							value={name}>{name}</option
						>{/each}</select
				></label
			>
			<label class="gold"
				>{m.msc_gold()}<select bind:value={goldChoice} disabled={!selected}
					><option value="">{m.msc_auto_gold()}</option>{#each gold as run}<option
							value={goldRunKey(run)}
							>{m.msc_gold_option({
								version: run.skill_version,
								model: run.model_name,
								run: run.benchmark_run_id,
								rows: run.rows
							})}</option
						>{/each}</select
				></label
			>
			<button class="primary" disabled={!selected || !model || starting} onclick={run}
				>{starting ? m.msc_starting() : m.msc_run()}</button
			>
		</div>
	</section>
	<section class="card">
		<div class="section-head">
			<h2>{m.msc_history()}</h2>
			<div class="actions">
				<select
					aria-label={m.msc_history()}
					bind:value={filter}
					onchange={() => {
						offset = 0;
						void history();
					}}
					><option value="all">{m.msc_all_documents()}</option><option
						value="selected"
						disabled={!selected}>{m.msc_selected_document()}</option
					></select
				><button onclick={() => history()} disabled={loading}>{m.msc_refresh()}</button>
			</div>
		</div>
		<p class="muted">{m.msc_comparison_guidance()}</p>
		{#if loading}<p class="muted" role="status">{m.msc_loading()}</p>{/if}
		<div class="table-wrap">
			<table>
				<thead
					><tr
						><th>{m.msc_document()}</th><th>{m.msc_model()}</th><th>{m.msc_created()}</th><th
							>{m.msc_status()}</th
						><th>{m.msc_score()}</th><th>{m.msc_open()}</th></tr
					></thead
				><tbody
					>{#each runs as run}<tr class:chosen={detail?.id === run.id}
							><td
								>{run.title || m.msc_record({ id: run.input_record_id })}<small
									>{m.msc_record({ id: run.input_record_id })}</small
								></td
							><td>{run.model_name}</td><td>{date(run.created_at)}</td><td
								><span class:failed={run.status === 'failed'}>{status(run.status)}</span></td
							><td>{run.score?.score ?? m.msc_na()}</td><td
								><button onclick={() => open(run.id)}>{m.msc_open()}</button></td
							></tr
						>{/each}</tbody
				>
			</table>
		</div>
		{#if !loading && !runs.length}<p class="muted">{m.msc_empty()}</p>{/if}
		<div class="pagination">
			<span
				>{m.msc_page({
					start: total ? offset + 1 : 0,
					end: Math.min(offset + 20, total),
					total
				})}</span
			><button
				disabled={offset === 0 || loading}
				onclick={() => {
					offset -= 20;
					void history();
				}}>{m.msc_previous()}</button
			><button
				disabled={offset + 20 >= total || loading}
				onclick={() => {
					offset += 20;
					void history();
				}}>{m.msc_next()}</button
			>
		</div>
	</section>
	<BenchmarkReportViewer
		run={{
			id: detail?.id ?? 'none',
			title: detail ? m.msc_results({ id: detail.id }) : m.msc_title(),
			status: detail ? status(detail.status) : '',
			documentTitle: detail ? `${detail.title || m.msc_record({ id: detail.input_record_id })} · ${detail.model_name}` : '',
			createdAt: detail ? date(detail.created_at) : '',
			subtitle: undefined
		}}
		scoreCards={detail?.score ? [
			{ label: m.msc_score(), value: String(detail.score.main.score) },
			{ label: m.msc_soft_precision(), value: pct(detail.score.main.soft_precision) },
			{ label: m.msc_soft_recall(), value: pct(detail.score.main.soft_recall) },
			{ label: m.msc_precision(), value: pct(detail.score.main.precision) },
			{ label: m.msc_recall(), value: pct(detail.score.main.recall) },
			{ label: m.msc_gold_rows(), value: String(detail.score.main.gold_rows) },
			{ label: m.msc_predicted_rows(), value: String(detail.score.main.predicted_rows) },
			{ label: m.msc_matched(), value: String(detail.score.main.matched) }
		] : []}
		warnings={detail?.input?.warnings ?? []}
		provenance={detail?.input ? (provenanceBody as Snippet) : undefined}
		evidenceActions={evidenceActions}
		inputId={activeInputId}
		{sourcePage}
		{sourcePageVersion}
		onFrameElement={(element) => (splitPanelsEl = element)}
		{darkMode}
		renderSourceHighlights={renderSourceHighlights}
		reportBody={reportBody as Snippet<[ReportBodyProps]>}
	/>
	{#snippet provenanceBody()}
		<details>
			<summary>{m.msc_provenance()}</summary>
			<dl>
				<dt>{m.msc_gold()}</dt><dd><pre>{json(detail?.input?.gold_run)}</pre></dd>
				{#if detail?.input?.provenance}<dt>{m.msc_scorer_provenance()}</dt><dd><pre>{json(detail.input.provenance)}</pre></dd>{/if}
				<dt>{m.msc_extraction()}</dt><dd><pre>{json(detail?.input?.extraction)}</pre></dd>
				<dt>{m.msc_prompt()}</dt><dd>{detail?.prompt_name}</dd><dt>{m.msc_created_by()}</dt><dd>{detail?.created_by}</dd><dt>{m.msc_finished()}</dt><dd>{date(detail?.finished_at)}</dd>
			</dl>
		</details>
		{#if evidenceKind}<div class="evidence-view"><div class="section-head"><h3>{evidenceLabels[evidenceKind]()}</h3><button onclick={() => (evidenceKind = '')}>{m.msc_close_evidence()}</button></div><pre>{evidenceText}</pre></div>{/if}
	{/snippet}
	{#snippet reportBody(props: ReportBodyProps)}
		{@const selectSource = props.selectSource}
		{@const selectedSourceLines = props.selectedSourceLines}
		{@const selectedTableReferences = props.selectedTableReferences}
		{#if detail}
            {#if detail.status === 'running'}<p role="status">{m.msc_background()}</p>{/if}
            {#if detail.status === 'failed'}<p class="error" role="alert">{errorText(detail.error_code)}</p>{/if}
<section class="card results">
			<button class="close-results" onclick={() => { detailSequence++; detail = null; onSelectMetrics(null); }}>{m.msc_close()}</button>
			{#if detail.score}
				{#if detail.score.without_test_parameters}<details>
						<summary>{m.msc_without_tests()}</summary>{@render summary(
							detail.score.without_test_parameters
						)}
					</details>{/if}
				<h3>{m.msc_field_accuracy()}</h3>
				<div class="stats">
					{#each Object.entries(detail.score.field_accuracy) as [field, accuracy]}<div>
							<span>{fields[field]?.() ?? field}</span><strong>{pct(accuracy)}</strong>
						</div>{/each}
				</div>
			{/if}
			{#if detail.score}
				<label class="pair-filter">
					{m.msc_filter()}
					<select bind:value={pairFilter}>
						<option value="all">{m.msc_view_all()}</option>
						<option value="full">{m.msc_view_full()}</option>
						<option value="partial">{m.msc_view_partial()}</option>
						{#each Object.entries(failureLabels) as [field, label]}
							<option value={field}>{label()}</option>
						{/each}
					</select>
				</label>
				<details open class="matched-metrics">
					<summary>{m.msc_pairs({ count: filteredPairs.length })}</summary
					>{#each filteredPairs as pair, index}
						{@const entryKey = JSON.stringify(['pair', index, pair.gold, pair.pred])}
						<button
							type="button"
							class="result-entry"
							class:selected-entry={selectedEntryKey === entryKey}
							aria-pressed={selectedEntryKey === entryKey}
							onclick={() =>
								selectSourceEntry(entryKey, [
									{ id: pair.gold, prediction: false },
									{ id: pair.pred, prediction: true }
									], selectSource)}
						>
							<strong>{rowName(pair.gold)}</strong>
							<span class="entry-line">{rowName(pair.pred, true)}</span>
							<span class="entry-line">{pair.note}</span>
							<span class="checks">
								{#each Object.entries(pair.checks) as [field, correct]}<span
									class:failed={correct === false}
									>{fields[field]?.() ?? field}: {correct == null
										? m.msc_na()
										: correct
											? m.msc_pass()
											: m.msc_fail()}</span
								>{#if correct === false}<span class="check-values"
										>{m.msc_expected()}: {comparedValue(pair.gold, false, field)} · {m.msc_actual()}:
										{comparedValue(pair.pred, true, field)}</span
									>{/if}{/each}<span>{m.msc_credit()}: {pct(pair.credit)}</span>
							</span>
						</button>{/each}
					{#if !filteredPairs.length}<p class="muted">{m.msc_no_matching_pairs()}</p>{/if}
				</details>
				<details open>
					<summary>{m.msc_missed({ count: detail.score.missed.length })}</summary
					>{#each detail.score.missed as missed, index}
						{@const entryKey = JSON.stringify(['missed', index, missed.gold])}
						<button
							type="button"
							class="result-entry"
							class:selected-entry={selectedEntryKey === entryKey}
							aria-pressed={selectedEntryKey === entryKey}
							onclick={() => selectSourceEntry(entryKey, [{ id: missed.gold, prediction: false }], selectSource)}
						>
							<strong>{rowName(missed.gold)}</strong>
							<span class="entry-line">{missed.note}</span>
						</button>{/each}
				</details>
				<details open>
					<summary>{m.msc_false_positives({ count: detail.score.false_positives.length })}</summary
					>{#each detail.score.false_positives as fp, index}
						{@const entryKey = JSON.stringify(['false-positive', index, fp.pred])}
						<button
							type="button"
							class="result-entry"
							class:selected-entry={selectedEntryKey === entryKey}
							aria-pressed={selectedEntryKey === entryKey}
							onclick={() => selectSourceEntry(entryKey, [{ id: fp.pred, prediction: true }], selectSource)}
						>
							<strong>{rowName(fp.pred, true)}</strong>
							<span class="entry-line">{m.msc_cause()}: {causes[fp.cause]?.() ?? fp.cause}</span>
							<span class="entry-line">{fp.note}</span>
						</button>{/each}
				</details>
				{#if detail.score.overrides.length}<details>
						<summary>{m.msc_overrides({ count: detail.score.overrides.length })}</summary>
						<pre>{json(detail.score.overrides)}</pre>
					</details>{/if}
			{/if}
			{#if detail.report}<details>
					<summary>{m.msc_report()}</summary>
					<pre class="report">{detail.report}</pre>
				</details>{/if}
		</section>
		{/if}
	{/snippet}

</div>

<style>
	.benchmark {
		--bg: #171b26;
		--card: #1f2333;
		--border: #2d3348;
		--text: #e2e8f0;
		--muted: #94a3b8;
		--surface: #252a3a;
		--accent: #818cf8;
		padding: 24px;
		background: var(--bg);
		color: var(--text);
		min-height: 100%;
		user-select: text;
	}
	.light {
		--bg: #f2f4f7;
		--card: #fff;
		--border: #e4e6eb;
		--text: #111827;
		--muted: #6b7280;
		--surface: #eceef2;
		--accent: #6366f1;
	}
	header {
		margin-bottom: 22px;
	}
	h1 {
		font-size: 24px;
		font-weight: 650;
	}
	h2 {
		font-size: 18px;
		font-weight: 600;
	}
	h3 {
		font-size: 14px;
		font-weight: 600;
		margin: 20px 0 10px;
	}
	p {
		margin: 10px 0;
	}
	header p,
	.muted,
	small {
		color: var(--muted);
	}
	small {
		display: block;
		font-size: 12px;
		margin-top: 3px;
	}
	.card {
		padding: 20px;
		border: 1px solid var(--border);
		border-radius: 12px;
		background: var(--card);
		margin-bottom: 20px;
	}
	.result-entry {
		display: block;
		width: 100%;
		padding: 12px;
		margin: 8px 0;
		text-align: left;
		white-space: normal;
		border-color: var(--border);
		user-select: text;
	}
	.result-entry.selected-entry {
		border-color: var(--accent);
		background: color-mix(in srgb, var(--accent) 22%, var(--surface));
		box-shadow: inset 3px 0 var(--accent);
	}
	.result-entry:focus-visible {
		outline: 3px solid var(--accent);
		outline-offset: 2px;
	}
	.search,
	.controls,
	.actions,
	.section-head,
	.pagination {
		display: flex;
		gap: 10px;
		align-items: center;
	}
	.search {
		margin: 14px 0;
	}
	.search input {
		flex: 1;
	}
	input,
	select,
	button {
		border: 1px solid var(--border);
		border-radius: 7px;
		padding: 9px 12px;
		background: var(--surface);
		color: var(--text);
		font-size: 13px;
	}
	button {
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.primary {
		background: var(--accent);
		color: #fff;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 7px;
		font-size: 12px;
		color: var(--muted);
	}
	.gold {
		flex: 1;
		min-width: 0;
	}
	.controls {
		align-items: end;
		flex-wrap: wrap;
	}
	.gold select {
		max-width: 100%;
	}
	.documents {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 290px), 1fr));
		gap: 8px;
		max-height: 180px;
		overflow: auto;
	}
	.documents button {
		min-width: 0;
		text-align: left;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.document-id-row {
		display: flex;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		color: var(--muted);
		font-size: 11px;
	}
	.document-id-row span {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.document-field {
		display: block;
		width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.chosen {
		background: color-mix(in srgb, var(--accent) 22%, var(--surface));
		border-color: var(--accent);
	}
	.section-head {
		justify-content: space-between;
		flex-wrap: wrap;
	}
	.actions {
		flex-wrap: wrap;
	}
	.pair-filter {
		margin: 16px 0;
		max-width: 300px;
	}
	.evidence-view pre {
		max-height: 480px;
		overflow: auto;
	}
	.table-wrap {
		overflow: auto;
		margin-top: 14px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		text-align: left;
		font-size: 13px;
	}
	th {
		font-size: 12px;
		color: var(--muted);
		font-weight: 500;
	}
	td,
	th {
		padding: 12px;
		border-bottom: 1px solid var(--border);
	}
	.pagination {
		justify-content: end;
		margin-top: 14px;
		font-size: 12px;
		color: var(--muted);
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
		gap: 12px;
		margin: 16px 0;
	}
	.stats div {
		padding: 14px;
		background: var(--surface);
		border-radius: 8px;
	}
	.stats span {
		display: block;
		font-size: 12px;
		color: var(--muted);
	}
	.stats strong {
		display: block;
		font-size: 22px;
		margin-top: 6px;
	}
	details {
		border-top: 1px solid var(--border);
		padding: 15px 0;
	}
	summary {
		cursor: pointer;
		font-weight: 600;
		font-size: 14px;
	}
	.entry-line {
		display: block;
		margin: 7px 0;
	}
	.checks {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		color: var(--muted);
		font-size: 12px;
	}
	.check-values {
		flex-basis: 100%;
		color: var(--muted);
		white-space: normal;
	}
	.error,
	.failed {
		color: #f87171;
	}
	.light .error,
	.light .failed {
		color: #dc2626;
	}
	pre {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-size: 12px;
		padding: 12px;
		background: var(--surface);
		border-radius: 6px;
	}
	dl {
		margin-top: 12px;
		font-size: 13px;
	}
	dd {
		margin: 4px 0 12px;
	}
	.report {
		line-height: 1.6;
	}
	@media (max-width: 700px) {
		.benchmark {
			padding: 12px;
		}
		.card {
			padding: 14px;
		}
		.controls {
			align-items: stretch;
			flex-direction: column;
		}
		.controls label {
			width: 100%;
		}
		.section-head {
			align-items: start;
		}
		.pagination {
			flex-wrap: wrap;
		}
	}
</style>
