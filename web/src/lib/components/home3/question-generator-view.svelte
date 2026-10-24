<script lang="ts">
	import { onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { createQuestionRun, listQuestionRuns, getQuestionRun, type QuestionRun, type GeneratedQuestion } from '$lib/services/questionGeneratorService';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let recordID = $state<number>(0);
	let purpose = $state('');
	let description = $state('');
	let oneHop = $state(10);
	let twoHop = $state(10);
	let multiHop = $state(10);
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
	let validRecord = $derived(Number.isSafeInteger(recordID) && recordID > 0);
	let validCounts = $derived([oneHop, twoHop, multiHop].every(n => Number.isInteger(n) && n >= 0 && n <= 50) && oneHop + twoHop + multiHop > 0);
	let valid = $derived(validRecord && purpose.trim().length > 0 && validCounts);
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
			const { run } = await createQuestionRun({ input_record_id: recordID, purpose: purpose.trim(), description: description.trim(), one_hop_count: oneHop, two_hop_count: twoHop, multi_hop_count: multiHop, lang: getLocale() }, createController.signal);
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
				<div class="record-row"><input id="qg-record" type="number" min="1" step="1" required bind:value={recordID} disabled={saving} /><button type="button" disabled={!validRecord || loading || saving} onclick={loadRuns}>{loading ? m.qg_loading() : m.qg_load_runs()}</button></div>
				<label for="qg-purpose">{m.qg_purpose()}</label>
				<textarea id="qg-purpose" rows="3" maxlength="2000" required bind:value={purpose} placeholder={m.qg_purpose_placeholder()}></textarea>
				<label for="qg-description">{m.qg_description()}</label>
				<textarea id="qg-description" rows="3" maxlength="10000" bind:value={description} placeholder={m.qg_description_placeholder()}></textarea>
				<fieldset><legend>{m.qg_counts()}</legend><div class="counts">
					<label>{m.qg_one_hop()}<input type="number" min="0" max="50" step="1" required bind:value={oneHop} /></label>
					<label>{m.qg_two_hop()}<input type="number" min="0" max="50" step="1" required bind:value={twoHop} /></label>
					<label>{m.qg_multi_hop()}<input type="number" min="0" max="50" step="1" required bind:value={multiHop} /></label>
				</div><p class="hint">{m.qg_count_hint()}</p></fieldset>
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
				<span class="run-purpose">{run.purpose}</span><small>{new Date(run.created_at).toLocaleString(getLocale())}</small>
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
			<div class="result-meta"><strong>{m.qg_run_number({ id: run.id })}</strong><span class="badge">{statusLabel(run.status)}</span><span>{run.model_name}</span><span>{run.prompt_name}</span></div>
			<p class="purpose">{run.purpose}</p>
			{#if run.description}<p>{run.description}</p>{/if}
			{#if run.status === 'running'}<p role="status">{m.qg_running_hint()}</p>{/if}
			{#if run.status === 'failed'}<p role="alert" class="error">{m.qg_generation_failed()}</p>{#if run.error_msg}<details><summary>{m.qg_error_detail()}</summary><pre>{run.error_msg}</pre></details>{/if}{/if}
			{#each questions as question, index (question.id)}
				<article class="question"><div class="question-heading"><span class="index">{index + 1}</span><span class="badge">{hopLabel(question.hop_type)}</span></div><h3>{question.question}</h3>
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
	header { margin-bottom:28px; max-width:850px; } h1 { font-size:28px; font-weight:650; letter-spacing:-.6px; margin:4px 0 10px; } header p, .hint, .empty, small { color:var(--muted); } .eyebrow { font-size:11px; letter-spacing:1.6px; text-transform:uppercase; } h2 { font-size:16px; font-weight:600; margin:0 0 20px; } p { line-height:1.6; } .workspace { display:grid; grid-template-columns:minmax(0,1.4fr) minmax(280px,1fr); gap:20px; align-items:start; } .panel { border:1px solid var(--line); border-radius:12px; padding:24px; background:var(--panel); min-width:0; } form { display:flex; flex-direction:column; gap:10px; } label, legend { font-size:13px; font-weight:600; } textarea, input { background:var(--bg); color:var(--text); border:1px solid var(--line); border-radius:6px; padding:10px 12px; width:100%; min-width:0; font:inherit; } textarea { resize:vertical; margin-bottom:8px; } input:focus, textarea:focus, button:focus-visible, summary:focus-visible { outline:2px solid var(--accent); outline-offset:2px; } button { color:var(--text); border:1px solid var(--line); border-radius:6px; padding:10px 14px; font-size:13px; cursor:pointer; background:var(--bg); } button:disabled { opacity:.5; cursor:default; } .primary { background:#4f46e5; color:white; border-color:#4f46e5; padding:12px; font-weight:600; } .record-row { display:flex; gap:8px; margin-bottom:12px; } .record-row button { white-space:nowrap; } fieldset { border:0; padding:0; margin:8px 0 0; } legend { margin-bottom:10px; } .counts { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:12px; } .counts label { display:flex; flex-direction:column; gap:8px; font-size:12px; } .hint { font-size:12px; margin:8px 0; } .run-list { display:grid; gap:10px; max-height:610px; overflow-y:auto; } .run { display:flex; flex-direction:column; gap:8px; text-align:left; width:100%; padding:14px; } .run.active { border-color:var(--accent); } .run-top { display:flex; justify-content:space-between; align-items:center; gap:8px; } .run-purpose { overflow-wrap:anywhere; } .badge { font-size:11px; border:1px solid var(--line); border-radius:4px; padding:3px 7px; color:var(--accent); white-space:nowrap; } .failed, .error { color:#ef7373; } .results { margin-top:20px; } .result-meta { display:flex; flex-wrap:wrap; gap:12px; align-items:center; font-size:12px; color:var(--muted); } .purpose { font-weight:600; } .question { border-top:1px solid var(--line); margin-top:24px; padding-top:24px; } .question-heading { display:flex; align-items:center; gap:12px; } .index { font-size:12px; color:var(--muted); font-variant-numeric:tabular-nums; } h3 { font-size:17px; font-weight:600; line-height:1.6; margin:10px 0 20px; } h4 { color:var(--muted); font-size:12px; font-weight:600; margin:16px 0 8px; } .answer { white-space:pre-wrap; } summary { cursor:pointer; font-size:13px; color:var(--accent); margin:16px 0; } .step-lines { display:block; } ol { padding-left:24px; } li { padding:4px 0; line-height:1.6; } blockquote { border-left:2px solid var(--accent); padding:4px 16px; margin:12px 0; font-size:13px; } blockquote span { color:var(--muted); font-size:11px; } blockquote p { white-space:pre-wrap; } pre { white-space:pre-wrap; overflow-wrap:anywhere; font-size:12px; } @media(max-width:900px) { .workspace { grid-template-columns:1fr; } .question-generator { padding:20px; } .panel { padding:18px; } }
</style>
