<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { searchInputs, type InputRecordSummary } from './metric-review-client.js';
	import { ScoreAPIError } from './metric-score-client';
	import {
		summaryGoldRuns, summaryScoreAlgorithms, summaryScoreArtifact, summaryScoreDetail,
		summaryScoreHistory, summaryScoreModels, startSummaryScore,
		type SummaryGoldRun, type SummaryScoreRun
	} from './summary-score-client';

	let { darkMode = true }: { darkMode: boolean } = $props();
	const lang = getLocale();
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
	let starting = $state(false);
	let loading = $state(false);
	let alive = true;
	let sequence = 0;
	let timer: ReturnType<typeof setInterval> | undefined;

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
			documents = await searchInputs(query, true, 'summary_rows');
			searched = true;
		} catch (err) { error = errorText(err); }
		finally { searching = false; }
	}

	async function choose(record: InputRecordSummary) {
		selected = record;
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
		if (!quiet) loading = true;
		try {
			const result = await summaryScoreHistory(selected?.id);
			if (alive) runs = result.runs ?? [];
		} catch (err) { if (alive) error = errorText(err); }
		finally { loading = false; }
	}

	async function open(id: number, quiet = false) {
		try {
			const result = await summaryScoreDetail(id);
			if (alive) detail = result;
		} catch (err) { if (alive && !quiet) error = errorText(err); }
	}

	async function run() {
		if (!selected || !algorithm || !model || starting) return;
		starting = true;
		error = '';
		try {
			detail = await startSummaryScore({
				record_id: selected.id, algorithm, model, lang,
				gold: gold.find((item) => JSON.stringify([item.skill_version, item.model_name, item.benchmark_run_id]) === goldChoice)
			});
			await history();
		} catch (err) { error = errorText(err); }
		finally { starting = false; }
	}

	onMount(() => {
		void Promise.all([summaryScoreModels().then((result) => { models = result; model = result[0] ?? ''; }), history()]).catch((err) => error = errorText(err));
		timer = setInterval(() => {
			if (runs.some((item) => item.status === 'running')) void history(true);
			if (detail?.status === 'running') void open(detail.id, true);
		}, 4000);
	});
	onDestroy(() => { alive = false; if (timer) clearInterval(timer); });
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
				<span>{m.ssb_record({ id: document.id })} · {date(document.create_time)}</span>
				<strong>{document.title || document.file_name || '—'}</strong>
				<small>{document.doc_no || ''} {document.file_name || ''}</small>
			</button>
		{/each}</div>{:else if searched}<p class="muted">{m.ssb_no_documents()}</p>{/if}
		{#if selected}<p class="selected">{selected.title || selected.file_name} · {m.ssb_record({ id: selected.id })}</p>{:else}<p class="muted">{m.ssb_select_document()}</p>{/if}
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

	{#if detail}<section class="card result">
		<div class="section-head"><h2>{m.ssb_results({ id: detail.id })} · {status(detail.status)}</h2><button onclick={() => detail = null}>{m.ssb_close()}</button></div>
		<p>{detail.title} · {m.ssb_record({ id: detail.input_record_id })} · {detail.algorithm} · {detail.model_name} · {date(detail.created_at)}</p>
		{#if detail.status === 'running'}<p role="status">{m.ssb_background()}</p>{/if}
		{#if detail.status === 'failed'}<p class="error" role="alert">{errorText(new ScoreAPIError(detail.error_code ?? '', 500))}</p>{/if}
		{#if detail.input?.gold}<p>{m.ssb_gold_identity()} · {detail.input.gold.skill_version} · {detail.input.gold.model_name} · {detail.input.gold.run_id}</p>{/if}
		{#if detail.score}
			<div class="score"><span>{m.ssb_score()}</span><strong>{detail.score.score}<small>{m.ssb_out_of_100()}</small></strong></div>
			<h3>{m.ssb_components()}</h3><div class="components">{#each Object.entries(detail.score.components ?? {}) as [name, value]}<div><span>{name}</span><strong>{value == null ? m.ssb_na() : `${Math.round(Number(value) * 1000) / 10}%`}</strong><small>{m.ssb_weight({ value: detail.score.weights?.[name] ?? 0 })}</small></div>{/each}</div>
			<p class="muted">{m.ssb_alignment({ matched: detail.score.matched, gold: detail.score.gold_count, candidate: detail.score.prediction_count })}</p>
		{/if}
		{#if detail.input?.warnings?.length}<h3>{m.ssb_warnings()}</h3><ul>{#each detail.input.warnings as warning}<li>{warning}</li>{/each}</ul>{/if}
		<div class="actions"><a href={summaryScoreArtifact(detail.id, 'input')}>{m.ssb_input()}</a><a href={summaryScoreArtifact(detail.id, 'judgments')}>{m.ssb_judgments()}</a><a href={summaryScoreArtifact(detail.id, 'score')}>{m.ssb_score_json()}</a><a href={summaryScoreArtifact(detail.id, 'report')}>{m.ssb_report()}</a></div>
		{#if detail.matches?.chunks}<h3>{m.ssb_chunk_reviews()}</h3>{#each detail.matches.chunks as chunk}<details><summary>{m.ssb_chunk({ id: chunk.pred })} · {chunk.diagnostics}</summary><pre>{JSON.stringify(chunk, null, 2)}</pre></details>{/each}{/if}
		{#if detail.report}<details><summary>{m.ssb_report()}</summary><pre>{detail.report}</pre></details>{/if}
	</section>{/if}
</div>

<style>
	.summary-benchmark{--bg:#11151d;--panel:#191f2a;--line:#303847;--text:#ece8dd;--muted:#a6a18f;--accent:#d3a64f;min-height:100%;padding:24px;color:var(--text);background:var(--bg);font:14px/1.5 system-ui,sans-serif;user-select:text}
	.summary-benchmark.light{--bg:#f5f2e9;--panel:#fffdf7;--line:#d9d3c5;--text:#211e18;--muted:#716b5e;--accent:#96701f}
	header h1{margin:0;font-size:24px}header p,.muted{color:var(--muted)}header p{margin:4px 0 20px}.card{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:16px;margin:14px 0}.card h2{margin:0 0 12px;font-size:16px}.search,.controls,.section-head,.actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.search input{flex:1;min-width:200px}.controls{margin-top:14px}.controls label{display:grid;gap:5px;color:var(--muted)}input,select,button,.actions a{font:inherit;color:var(--text);background:var(--bg);border:1px solid var(--line);border-radius:6px;padding:8px 10px}button,.actions a{cursor:pointer;text-decoration:none}button:hover,.actions a:hover{border-color:var(--accent)}button:disabled{opacity:.55;cursor:default}.primary{background:var(--accent);color:#17130b;border-color:var(--accent);font-weight:700}.documents{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:8px;margin-top:12px}.documents button{text-align:left;display:grid;gap:4px}.documents button span,.documents small,td small{color:var(--muted);font-size:12px}.chosen{outline:1px solid var(--accent)}.selected{margin:12px 0 0}.section-head{justify-content:space-between}.section-head h2{margin:0}.table-wrap{overflow:auto}table{border-collapse:collapse;width:100%;text-align:left}th,td{padding:9px;border-bottom:1px solid var(--line);white-space:nowrap}td:first-child{white-space:normal}td small{display:block}.result{scroll-margin-top:24px}.score{display:flex;align-items:baseline;gap:16px;padding:14px 0}.score span{color:var(--muted)}.score strong{font-size:38px;color:var(--accent)}.score small{font-size:15px;color:var(--muted)}.components{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.components div{padding:12px;border:1px solid var(--line);border-radius:7px;display:grid;gap:4px}.components span,.components small{color:var(--muted)}.components strong{font-size:20px}h3{margin:18px 0 8px}details{border-top:1px solid var(--line);padding:10px 0}summary{cursor:pointer}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:var(--bg);padding:12px;border-radius:6px;max-height:500px;overflow:auto}.error{color:#ff8f7e;background:#4b211e;padding:10px;border-radius:6px}.actions{margin-top:12px}.summary-benchmark :global(button),.summary-benchmark :global(a){user-select:none}
	@media(max-width:760px){.summary-benchmark{padding:14px}.controls{align-items:stretch}.controls label,.controls button{width:100%}}
</style>
