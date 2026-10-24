<script lang="ts">
	import { onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { createQuestionRun, listQuestionRuns, getQuestionRun, type QuestionRun, type GeneratedQuestion } from '$lib/services/questionGeneratorService';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let recordID = $state<number | undefined>();
	let purpose = $state<'search_metrics'>('search_metrics');
	let description = $state('');
	let metricsPerDocument = $state(10);
	let totalMetrics = $state(10);
	let runs = $state<QuestionRun[]>([]);
	let selectedID = $state<number | null>(null);
	let selected = $state<QuestionRun | null>(null);
	let questions = $state<GeneratedQuestion[]>([]);
	let saving = $state(false);
	let loading = $state(false);
	let detailLoading = $state(false);
	let listed = $state(false);
	let error = $state<'load' | 'create' | ''>('');
	let detailError = $state(false);
	let retry = $state(0);
	let alive = true;
	let listController: AbortController | null = null;
	let createController: AbortController | null = null;
	let generation = 0;
	let normalizedRecordID = $derived(recordID ?? 0);
	let validRecord = $derived(Number.isSafeInteger(normalizedRecordID) && normalizedRecordID >= 0);
	let singleDocument = $derived(normalizedRecordID > 0);
	let validCounts = $derived(Number.isInteger(metricsPerDocument) && metricsPerDocument >= 1 && metricsPerDocument <= 50);
	let validTotals = $derived(Number.isInteger(totalMetrics) && totalMetrics >= 1 && totalMetrics <= 1000);
	let valid = $derived(validRecord && purpose === 'search_metrics' && validCounts && (singleDocument || validTotals));
	const purposeLabel = (value: string) => value === 'search_metrics' ? m.qg_search_metrics() : value;
	const languageLabel = (lang: string | undefined) => lang === 'en' ? m.qg_english() : lang === 'zh-cn' ? m.qg_chinese() : m.qg_mixed();
	const progress = (run: QuestionRun, hop: GeneratedQuestion['hop_type']) => {
		const count = run[`generated_${hop}_count`] ?? questions.filter(q => q.hop_type === hop).length;
		const target = run.input_record_id > 0 ? run[`${hop}_count`] : run[`total_${hop}_count`];
		return m.qg_progress_count({ count, total: target ?? 0 });
	};
	const statusLabel = (status: QuestionRun['status']) => status === 'running' ? m.qg_running() : status === 'done' ? m.qg_done() : m.qg_failed();
	const hopLabel = (hop: GeneratedQuestion['hop_type']) => hop === 'one_hop' ? m.qg_one_hop() : hop === 'two_hop' ? m.qg_two_hop() : m.qg_multi_hop();

	$effect(() => {
		recordID;
		generation++;
		listController?.abort();
		runs = [];
		selectedID = null;
		detailLoading = false;
		detailError = false;
		selected = null;
		questions = [];
		listed = false;
		loading = false;
		error = '';
	});

	$effect(() => {
		const id = selectedID;
		retry;
		if (id === null) return;
		const controller = new AbortController();
		let timer: ReturnType<typeof setTimeout> | undefined;
		detailLoading = true;
		detailError = false;
		selected = null;
		questions = [];
		async function poll() {
			try {
				const result = await getQuestionRun(id!, controller.signal);
				if (controller.signal.aborted) return;
				selected = result.run;
				questions = result.questions ?? [];
				runs = runs.map(run => run.id === id ? result.run : run);
				detailLoading = false;
				if (result.run.status === 'running') timer = setTimeout(poll, 2500);
			} catch {
				if (!controller.signal.aborted) { detailError = true; detailLoading = false; }
			}
		}
		void poll();
		return () => { controller.abort(); clearTimeout(timer); };
	});

	onDestroy(() => { alive = false; listController?.abort(); createController?.abort(); });

	async function loadRuns() {
		if (!validRecord || saving) return;
		listController?.abort();
		const controller = new AbortController();
		listController = controller;
		const version = generation;
		loading = true;
		error = '';
		try {
			const result = await listQuestionRuns(recordID, controller.signal);
			if (!alive || controller.signal.aborted || version !== generation) return;
			runs = result.runs ?? [];
			listed = true;
		} catch { if (alive && !controller.signal.aborted && version === generation) error = 'load'; }
		finally { if (alive && !controller.signal.aborted && version === generation) loading = false; }
	}

	async function generate(event: SubmitEvent) {
		event.preventDefault();
		if (!valid || saving) return;
		listController?.abort();
		loading = false;
		const version = generation;
		createController = new AbortController();
		saving = true;
		error = '';
		try {
			const { run } = await createQuestionRun({ input_record_id: normalizedRecordID, purpose, description: description.trim(), metrics_per_document: metricsPerDocument, total_metrics: singleDocument ? 0 : totalMetrics }, createController.signal);
			if (!alive || version !== generation) return;
			runs = [run, ...runs.filter(r => r.id !== run.id)];
			listed = true;
			selectedID = run.id;
		} catch { if (alive && version === generation) error = 'create'; }
		finally { if (alive) saving = false; }
	}
</script>

<div class="question-generator select-text" class:dark={darkMode}>
	<header><p class="eyebrow">{m.qg_benchmark()}</p><h1>{m.qg_title()}</h1><p>{m.qg_intro()}</p></header>
	<div class="workspace">
		<section class="panel form-panel" aria-labelledby="qg-config-title">
			<h2 id="qg-config-title">{m.qg_configure()}</h2>
			<form onsubmit={generate}>
				<label for="qg-record">{m.qg_record_id()}</label>
				<div class="record-row"><input id="qg-record" type="number" min="0" step="1" bind:value={recordID} disabled={saving} /><button type="button" disabled={!validRecord || loading || saving} onclick={loadRuns}>{loading ? m.qg_loading() : m.qg_load_runs()}</button></div>
				<p class="hint">{m.qg_record_hint()}</p>
				<label for="qg-purpose">{m.qg_purpose()}</label>
				<select id="qg-purpose" bind:value={purpose}><option value="search_metrics">{m.qg_search_metrics()}</option></select>
				<label for="qg-description">{m.qg_description()}</label>
				<textarea id="qg-description" rows="3" maxlength="10000" bind:value={description} placeholder={m.qg_description_placeholder()}></textarea>
				<div class="counts">
					<div class="metric-count"><label for="qg-metrics-per-document">{m.qg_metrics_per_document()}</label><input id="qg-metrics-per-document" type="number" min="1" max="50" step="1" required bind:value={metricsPerDocument} /><p class="hint">{m.qg_metrics_per_document_hint()}</p></div>
					<div class="metric-count"><label for="qg-total-metrics">{m.qg_total_metrics()}</label><input id="qg-total-metrics" type="number" min="1" max="1000" step="1" required disabled={singleDocument} bind:value={totalMetrics} /><p class="hint">{singleDocument ? m.qg_totals_ignored() : m.qg_total_metrics_hint()}</p></div>
				</div>
				<p class="hint">{m.qg_metric_triplet_hint()}</p>
				<p class="hint">{m.qg_language_hint()}</p>
				<button class="primary" type="submit" disabled={!valid || saving}>{saving ? m.qg_starting() : m.qg_generate()}</button>
			</form>
			{#if error}<p role="alert" class="error">{error === 'load' ? m.qg_load_error() : m.qg_create_error()}</p>{/if}
		</section>
		<section class="panel runs-panel" aria-labelledby="qg-runs-title">
			<h2 id="qg-runs-title">{m.qg_saved_runs()}</h2>
			{#if runs.length === 0}<p class="empty">{listed ? m.qg_no_runs() : m.qg_load_hint()}</p>{/if}
			<div class="run-list">{#each runs as run (run.id)}<button type="button" class="run" class:active={selectedID === run.id} aria-pressed={selectedID === run.id} onclick={() => selectedID = run.id}>
				<span class="run-top"><strong>{m.qg_run_number({ id: run.id })}</strong><span class="badge" class:failed={run.status === 'failed'}>{statusLabel(run.status)}</span></span>
				<span class="run-purpose">{purposeLabel(run.purpose)}</span><small>{run.input_record_id > 0 ? m.qg_source_record({ id: run.input_record_id }) : m.qg_batch_run()}</small><small>{new Date(run.created_at).toLocaleString(getLocale())}</small>
			</button>{/each}</div>
		</section>
	</div>
	<section class="panel results" aria-labelledby="qg-results-title" aria-busy={detailLoading}>
		<h2 id="qg-results-title">{m.qg_results()}</h2>
		<p class="hint">{m.qg_review_hint()}</p>
		{#if detailLoading}<p role="status">{m.qg_loading()}</p>
		{:else if detailError}<p role="alert" class="error">{m.qg_load_error()}</p><button onclick={() => retry++}>{m.qg_retry()}</button>
		{:else if selected}
			{#each selected ? [selected] : [] as run (run.id)}
			<div class="result-meta"><strong>{m.qg_run_number({ id: run.id })}</strong><span class="badge">{statusLabel(run.status)}</span><span>{run.model_name}</span><span>{run.prompt_name}</span><span>{languageLabel(run.lang)}</span></div>
			<div class="progress" role="status"><p>{m.qg_documents_processed({ count: run.documents_processed ?? (run.input_record_id > 0 && questions.length > 0 ? 1 : 0) })}</p><div class="progress-counts"><span>{m.qg_one_hop()}: {progress(run, 'one_hop')}</span><span>{m.qg_two_hop()}: {progress(run, 'two_hop')}</span><span>{m.qg_multi_hop()}: {progress(run, 'multi_hop')}</span></div></div>
			<p class="purpose">{purposeLabel(run.purpose)}</p>
			{#if run.description}<p>{run.description}</p>{/if}
			{#if run.status === 'running'}<p role="status">{m.qg_running_hint()}</p>{/if}
			{#if run.status === 'failed'}<p role="alert" class="error">{m.qg_generation_failed()}</p>{#if run.error_msg}<details><summary>{m.qg_error_detail()}</summary><pre>{run.error_msg}</pre></details>{/if}{/if}
			{#each questions as question, index (question.id)}
				<article class="question"><div class="question-heading"><span class="index">{index + 1}</span><span class="badge">{hopLabel(question.hop_type)}</span><span class="badge">{languageLabel(question.lang ?? (run.lang === 'mixed' ? undefined : run.lang))}</span><small>{m.qg_source_record({ id: question.input_record_id })}</small>{#if question.metric_id}<small>{m.qg_metric_id({ id: question.metric_id })}</small>{/if}</div><h3>{question.question}</h3>
					<h4>{m.qg_answer()}</h4><p class="answer">{question.answer}</p>
					<details><summary>{m.qg_reasoning_evidence()}</summary><h4>{m.qg_reasoning()}</h4><ol>{#each question.reasoning_steps ?? [] as step, stepIndex}<li>{step}{#if question.step_line_ids?.[stepIndex]?.length}<small class="step-lines">{m.qg_step_lines({ lines: question.step_line_ids[stepIndex].join(', ') })}</small>{/if}</li>{/each}</ol><h4>{m.qg_evidence()}</h4>{#each question.evidence ?? [] as evidence}<blockquote><span>{m.qg_source_line({ line: evidence.line_id })}</span><p>{evidence.text}</p></blockquote>{/each}</details>
				</article>
			{/each}
			{/each}
		{:else}<p class="empty">{m.qg_select_run()}</p>{/if}
	</section>
</div>

<style>
	.question-generator { --bg:#f2f4f7; --panel:#fff; --line:#e4e6eb; --text:#111827; --muted:#6b7280; --accent:#4f46e5; background:var(--bg); color:var(--text); padding:32px; min-height:100%; user-select:text; }
	.question-generator.dark { --bg:#171b26; --panel:#1f2333; --line:#2d3348; --text:#e2e8f0; --muted:#94a3b8; --accent:#a5b4fc; }
	header { margin-bottom:28px; max-width:850px; } h1 { font-size:28px; font-weight:650; letter-spacing:-.6px; margin:4px 0 10px; } header p, .hint, .empty, small { color:var(--muted); } .eyebrow { font-size:11px; letter-spacing:1.6px; text-transform:uppercase; } h2 { font-size:16px; font-weight:600; margin:0 0 20px; } p { line-height:1.6; } .workspace { display:grid; grid-template-columns:minmax(0,1.4fr) minmax(280px,1fr); gap:20px; align-items:start; } .panel { border:1px solid var(--line); border-radius:12px; padding:24px; background:var(--panel); min-width:0; } form { display:flex; flex-direction:column; gap:10px; } label { font-size:13px; font-weight:600; } textarea, input, select { background:var(--bg); color:var(--text); border:1px solid var(--line); border-radius:6px; padding:10px 12px; width:100%; min-width:0; font:inherit; } textarea { resize:vertical; margin-bottom:8px; } input:focus, textarea:focus, select:focus, button:focus-visible, summary:focus-visible { outline:2px solid var(--accent); outline-offset:2px; } button { color:var(--text); border:1px solid var(--line); border-radius:6px; padding:10px 14px; font-size:13px; cursor:pointer; background:var(--bg); } button:disabled { opacity:.5; cursor:default; } .primary { background:#4f46e5; color:white; border-color:#4f46e5; padding:12px; font-weight:600; } .record-row { display:flex; gap:8px; margin-bottom:12px; } .record-row button { white-space:nowrap; } input:disabled { opacity:.6; } .progress { margin:16px 0; font-size:12px; color:var(--muted); } .progress-counts { display:flex; flex-wrap:wrap; gap:16px; } .counts { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; margin-top:8px; } .metric-count { display:flex; flex-direction:column; gap:8px; } .metric-count .hint { margin:0; } .counts label { font-size:12px; } .hint { font-size:12px; margin:8px 0; } .run-list { display:grid; gap:10px; max-height:610px; overflow-y:auto; } .run { display:flex; flex-direction:column; gap:8px; text-align:left; width:100%; padding:14px; } .run.active { border-color:var(--accent); } .run-top { display:flex; justify-content:space-between; align-items:center; gap:8px; } .run-purpose { overflow-wrap:anywhere; } .badge { font-size:11px; border:1px solid var(--line); border-radius:4px; padding:3px 7px; color:var(--accent); white-space:nowrap; } .failed, .error { color:#ef7373; } .results { margin-top:20px; } .result-meta { display:flex; flex-wrap:wrap; gap:12px; align-items:center; font-size:12px; color:var(--muted); } .purpose { font-weight:600; } .question { border-top:1px solid var(--line); margin-top:24px; padding-top:24px; } .question-heading { display:flex; flex-wrap:wrap; align-items:center; gap:12px; } .index { font-size:12px; color:var(--muted); font-variant-numeric:tabular-nums; } h3 { font-size:17px; font-weight:600; line-height:1.6; margin:10px 0 20px; } h4 { color:var(--muted); font-size:12px; font-weight:600; margin:16px 0 8px; } .answer { white-space:pre-wrap; } summary { cursor:pointer; font-size:13px; color:var(--accent); margin:16px 0; } .step-lines { display:block; } ol { padding-left:24px; } li { padding:4px 0; line-height:1.6; } blockquote { border-left:2px solid var(--accent); padding:4px 16px; margin:12px 0; font-size:13px; } blockquote span { color:var(--muted); font-size:11px; } blockquote p { white-space:pre-wrap; } pre { white-space:pre-wrap; overflow-wrap:anywhere; font-size:12px; } @media(max-width:900px) { .workspace { grid-template-columns:1fr; } .question-generator { padding:20px; } .panel { padding:18px; } }
</style>
