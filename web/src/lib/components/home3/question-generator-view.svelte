<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		createQuestionRun,
		getQuestionRun,
		listQuestions,
		getQuestionFilters,
		type QuestionRun,
		type GeneratedQuestion,
		type QuestionFilters,
		type QuestionFilterOptions
	} from '$lib/services/questionGeneratorService';
	let { darkMode = true }: { darkMode?: boolean } = $props();
	const emptyFilters = (): QuestionFilters => ({
		purpose: '',
		run_id: '',
		input_record_id: '',
		lang: '',
		created_from: '',
		created_to: ''
	});
	let draft = $state(emptyFilters());
	let applied = $state(emptyFilters());
	let options = $state<QuestionFilterOptions>({
		purposes: [],
		run_ids: [],
		record_ids: [],
		languages: []
	});
	let questions = $state<GeneratedQuestion[]>([]);
	let total = $state(0);
	let page = $state(1);
	const pageSize = 50;
	let loading = $state(false);
	let loadError = $state(false);
	let filtersError = $state(false);
	let recordID = $state<number | undefined>();
	let purpose = $state<'search_metrics'>('search_metrics');
	let description = $state('');
	let metricsPerDocument = $state(10);
	let totalMetrics = $state(10);
	let saving = $state(false);
	let createError = $state(false);
	let activeRun = $state<QuestionRun | null>(null);
	let runQuestions = $state<GeneratedQuestion[]>([]);
	let pollError = $state(false);
	let dialog: HTMLDialogElement;
	let alive = true;
	let listController: AbortController | undefined;
	let filterController: AbortController | undefined;
	let createController: AbortController | undefined;
	let pollController: AbortController | undefined;
	let timer: ReturnType<typeof setTimeout> | undefined;
	let version = 0;
	let normalizedRecordID = $derived(recordID ?? 0);
	let singleDocument = $derived(normalizedRecordID > 0);
	let validRecord = $derived(Number.isSafeInteger(normalizedRecordID) && normalizedRecordID >= 0);
	let valid = $derived(
		validRecord &&
			Number.isInteger(metricsPerDocument) &&
			metricsPerDocument >= 1 &&
			metricsPerDocument <= 50 &&
			(singleDocument ||
				(Number.isInteger(totalMetrics) && totalMetrics >= 1 && totalMetrics <= 1000))
	);
	let validDates = $derived(
		!draft.created_from || !draft.created_to || draft.created_from <= draft.created_to
	);
	const purposeLabel = (value: string) =>
		value === 'search_metrics' ? m.qg_search_metrics() : value;
	const languageLabel = (lang: string | undefined) =>
		lang === 'en' ? m.qg_english() : lang === 'zh-cn' ? m.qg_chinese() : lang || m.qg_mixed();
	const statusLabel = (status: QuestionRun['status']) =>
		status === 'running' ? m.qg_running() : status === 'done' ? m.qg_done() : m.qg_failed();
	const hopLabel = (hop: GeneratedQuestion['hop_type']) =>
		hop === 'one_hop' ? m.qg_one_hop() : hop === 'two_hop' ? m.qg_two_hop() : m.qg_multi_hop();
	const progress = (run: QuestionRun, hop: GeneratedQuestion['hop_type']) =>
		m.qg_progress_count({
			count: run[`generated_${hop}_count`] ?? runQuestions.filter((q) => q.hop_type === hop).length,
			total: (run.input_record_id > 0 ? run[`${hop}_count`] : run[`total_${hop}_count`]) ?? 0
		});
	onMount(() => {
		void loadQuestions(1);
		void loadOptions();
	});
	onDestroy(() => {
		alive = false;
		listController?.abort();
		filterController?.abort();
		createController?.abort();
		pollController?.abort();
		clearTimeout(timer);
	});
	async function loadQuestions(nextPage: number) {
		listController?.abort();
		const controller = new AbortController();
		listController = controller;
		const current = ++version;
		loading = true;
		loadError = false;
		try {
			const result = await listQuestions(applied, nextPage, pageSize, controller.signal);
			if (!alive || controller.signal.aborted || current !== version) return;
			questions = result.questions ?? [];
			total = result.total;
			page = result.page;
		} catch {
			if (alive && !controller.signal.aborted && current === version) loadError = true;
		} finally {
			if (alive && !controller.signal.aborted && current === version) loading = false;
		}
	}
	async function loadOptions() {
		filterController?.abort();
		const controller = new AbortController();
		filterController = controller;
		filtersError = false;
		try {
			const result = await getQuestionFilters(controller.signal);
			if (alive && !controller.signal.aborted) options = result;
		} catch {
			if (alive && !controller.signal.aborted) filtersError = true;
		}
	}
	function search(event: SubmitEvent) {
		event.preventDefault();
		if (!validDates) return;
		applied = { ...draft };
		void loadQuestions(1);
	}
	function reset() {
		draft = emptyFilters();
		applied = emptyFilters();
		void loadQuestions(1);
		void loadOptions();
	}
	async function poll(id: number) {
		pollController?.abort();
		clearTimeout(timer);
		const controller = new AbortController();
		pollController = controller;
		pollError = false;
		try {
			const result = await getQuestionRun(id, controller.signal);
			if (!alive || controller.signal.aborted) return;
			activeRun = result.run;
			runQuestions = result.questions ?? [];
			if (result.run.status === 'running') timer = setTimeout(() => void poll(id), 2500);
			else {
				void loadQuestions(page);
				void loadOptions();
			}
		} catch {
			if (alive && !controller.signal.aborted) pollError = true;
		}
	}
	async function generate(event: SubmitEvent) {
		event.preventDefault();
		if (!valid || saving || activeRun?.status === 'running') return;
		createController = new AbortController();
		saving = true;
		createError = false;
		try {
			const { run } = await createQuestionRun(
				{
					input_record_id: normalizedRecordID,
					purpose,
					description: description.trim(),
					metrics_per_document: metricsPerDocument,
					total_metrics: singleDocument ? 0 : totalMetrics
				},
				createController.signal
			);
			if (!alive) return;
			activeRun = run;
			runQuestions = [];
			dialog.close();
			void poll(run.id);
		} catch {
			if (alive && !createController.signal.aborted) createError = true;
		} finally {
			if (alive) saving = false;
		}
	}
</script>

<div class="question-generator select-text" class:dark={darkMode}>
	<header>
		<div>
			<p class="eyebrow">{m.qg_benchmark()}</p>
			<h1>{m.qg_title()}</h1>
			<p>{m.qg_intro()}</p>
		</div>
		<button
			class="primary open-generation"
			disabled={saving || activeRun?.status === 'running'}
			onclick={() => {
				createError = false;
				dialog.showModal();
			}}>{m.qg_generate()}</button
		>
	</header>
	<form class="panel filters" onsubmit={search} aria-label={m.qg_filter_questions()}>
		<div>
			<label for="qg-filter-purpose">{m.qg_filter_purpose()}</label><select
				id="qg-filter-purpose"
				bind:value={draft.purpose}
				><option value="">{m.qg_all()}</option>{#each options.purposes as value}<option {value}
						>{purposeLabel(value)}</option
					>{/each}</select
			>
		</div>
		<div>
			<label for="qg-filter-run">{m.qg_filter_run()}</label><select
				id="qg-filter-run"
				bind:value={draft.run_id}
				><option value="">{m.qg_all()}</option>{#each options.run_ids as value}<option
						value={String(value)}>{value}</option
					>{/each}</select
			>
		</div>
		<div>
			<label for="qg-filter-record">{m.qg_filter_record()}</label><select
				id="qg-filter-record"
				bind:value={draft.input_record_id}
				><option value="">{m.qg_all()}</option>{#each options.record_ids as value}<option
						value={String(value)}>{value}</option
					>{/each}</select
			>
		</div>
		<div>
			<label for="qg-filter-language">{m.qg_filter_language()}</label><select
				id="qg-filter-language"
				bind:value={draft.lang}
				><option value="">{m.qg_all()}</option>{#each options.languages as value}<option {value}
						>{languageLabel(value)}</option
					>{/each}</select
			>
		</div>
		<div class="date-filter">
			<label for="qg-filter-from">{m.qg_time_from()}</label><input
				id="qg-filter-from"
				type="datetime-local"
				bind:value={draft.created_from}
			/>
		</div>
		<div class="date-filter">
			<label for="qg-filter-to">{m.qg_time_to()}</label><input
				id="qg-filter-to"
				type="datetime-local"
				bind:value={draft.created_to}
			/>
		</div>
		<div class="filter-actions">
			<button class="primary" type="submit" disabled={!validDates}>{m.qg_search()}</button><button
				type="button"
				onclick={reset}>{m.qg_reset()}</button
			>
		</div>
		{#if !validDates}<p class="error" role="alert">{m.qg_invalid_dates()}</p>{/if}
		{#if filtersError}<p class="error" role="alert">
				{m.qg_filter_error()} <button type="button" onclick={loadOptions}>{m.qg_retry()}</button>
			</p>{/if}
	</form>
	{#if activeRun}
		<section class="panel generation-status" aria-label={m.qg_generation_status()}>
			<div class="result-meta">
				<strong>{m.qg_run_number({ id: activeRun.id })}</strong><span class="badge"
					>{statusLabel(activeRun.status)}</span
				><span>{activeRun.model_name}</span><span>{activeRun.prompt_name}</span>
			</div>
			<div class="progress" role="status">
				<p>
					{m.qg_documents_processed({
						count:
							activeRun.documents_processed ??
							(activeRun.input_record_id > 0 && runQuestions.length > 0 ? 1 : 0)
					})}
				</p>
				<div class="progress-counts">
					<span>{m.qg_one_hop()}: {progress(activeRun, 'one_hop')}</span><span
						>{m.qg_two_hop()}: {progress(activeRun, 'two_hop')}</span
					><span>{m.qg_multi_hop()}: {progress(activeRun, 'multi_hop')}</span>
				</div>
			</div>
			{#if activeRun.status === 'running'}<p class="hint">{m.qg_running_hint()}</p>{/if}
			{#if pollError}<p class="error" role="alert">{m.qg_load_error()}</p>
				<button onclick={() => void poll(activeRun!.id)}>{m.qg_retry()}</button>{/if}
			{#if activeRun.status === 'failed'}<p role="alert" class="error">
					{m.qg_generation_failed()}
				</p>
				{#if activeRun.error_msg}<details>
						<summary>{m.qg_error_detail()}</summary>
						<pre>{activeRun.error_msg}</pre>
					</details>{/if}{/if}
		</section>
	{/if}
	<section class="panel results" aria-labelledby="qg-results-title" aria-busy={loading}>
		<div class="results-heading">
			<h2 id="qg-results-title">{m.qg_results()}</h2>
			<span>{m.qg_result_count({ count: total })}</span>
		</div>
		<p class="hint">{m.qg_review_hint()}</p>
		{#if loading}<p role="status">{m.qg_loading()}</p>
		{:else if loadError}<p class="error" role="alert">{m.qg_load_error()}</p>
			<button onclick={() => void loadQuestions(page)}>{m.qg_retry()}</button>
		{:else if questions.length === 0}<p class="empty">{m.qg_no_questions()}</p>
		{:else}{#each questions as question, index (question.id)}
				<article class="question">
					<div class="question-heading">
						<span class="index">{(page - 1) * pageSize + index + 1}</span><span class="badge"
							>{hopLabel(question.hop_type)}</span
						><span class="badge">{languageLabel(question.lang)}</span><small
							>{m.qg_source_record({ id: question.input_record_id })}</small
						>{#if question.metric_id}<small>{m.qg_metric_id({ id: question.metric_id })}</small
							>{/if}
					</div>
					<div class="question-meta">
						<span>{m.qg_run_number({ id: question.run_id })}</span><span
							>{purposeLabel(question.purpose)}</span
						><span>{m.qg_model({ value: question.model_name })}</span><span
							>{m.qg_prompt({ value: question.prompt_name })}</span
						><time datetime={question.created_at}
							>{new Date(question.created_at).toLocaleString(getLocale())}</time
						>
					</div>
					<h3>{question.question}</h3>
					<h4>{m.qg_answer()}</h4>
					<p class="answer">{question.answer}</p>
					<details>
						<summary>{m.qg_reasoning_evidence()}</summary>
						<h4>{m.qg_reasoning()}</h4>
						<ol>
							{#each question.reasoning_steps ?? [] as step, stepIndex}<li>
									{step}{#if question.step_line_ids?.[stepIndex]?.length}<small class="step-lines"
											>{m.qg_step_lines({
												lines: question.step_line_ids[stepIndex].join(', ')
											})}</small
										>{/if}
								</li>{/each}
						</ol>
						<h4>{m.qg_evidence()}</h4>
						{#each question.evidence ?? [] as evidence}<blockquote>
								<span>{m.qg_source_line({ line: evidence.line_id })}</span>
								<p>{evidence.text}</p>
							</blockquote>{/each}
					</details>
				</article>
			{/each}{/if}
		<nav class="pagination" aria-label={m.qg_pagination()}>
			<button disabled={loading || page <= 1} onclick={() => void loadQuestions(page - 1)}
				>{m.qg_previous()}</button
			><span>{m.qg_page_status({ page, pages: Math.max(1, Math.ceil(total / pageSize)) })}</span
			><button
				disabled={loading || page * pageSize >= total}
				onclick={() => void loadQuestions(page + 1)}>{m.qg_next()}</button
			>
		</nav>
	</section>
	<dialog
		bind:this={dialog}
		aria-labelledby="qg-config-title"
		onclick={(event) => {
			if (event.target === dialog) {
				const rect = dialog.getBoundingClientRect();
				if (
					event.clientX < rect.left ||
					event.clientX > rect.right ||
					event.clientY < rect.top ||
					event.clientY > rect.bottom
				)
					dialog.close();
			}
		}}
		onkeydown={(event) => {
			if (event.key === 'Escape') dialog.close();
		}}
	>
		<div class="dialog-heading">
			<h2 id="qg-config-title">{m.qg_configure()}</h2>
			<button type="button" onclick={() => dialog.close()}>{m.qg_close()}</button>
		</div>
		<form onsubmit={generate}>
			<label for="qg-record">{m.qg_record_id()}</label>
			<div class="record-row">
				<input
					id="qg-record"
					type="number"
					min="0"
					step="1"
					bind:value={recordID}
					disabled={saving}
				/>
			</div>
			<p class="hint">{m.qg_record_hint()}</p>
			<label for="qg-purpose">{m.qg_purpose()}</label>
			<select id="qg-purpose" bind:value={purpose}
				><option value="search_metrics">{m.qg_search_metrics()}</option></select
			>
			<label for="qg-description">{m.qg_description()}</label>
			<textarea
				id="qg-description"
				rows="3"
				maxlength="10000"
				bind:value={description}
				placeholder={m.qg_description_placeholder()}
			></textarea>
			<div class="counts">
				<div class="metric-count">
					<label for="qg-metrics-per-document">{m.qg_metrics_per_document()}</label><input
						id="qg-metrics-per-document"
						type="number"
						min="1"
						max="50"
						step="1"
						required
						bind:value={metricsPerDocument}
					/>
					<p class="hint">{m.qg_metrics_per_document_hint()}</p>
				</div>
				<div class="metric-count">
					<label for="qg-total-metrics">{m.qg_total_metrics()}</label><input
						id="qg-total-metrics"
						type="number"
						min="1"
						max="1000"
						step="1"
						required
						disabled={singleDocument}
						bind:value={totalMetrics}
					/>
					<p class="hint">{singleDocument ? m.qg_totals_ignored() : m.qg_total_metrics_hint()}</p>
				</div>
			</div>
			<p class="hint">{m.qg_metric_triplet_hint()}</p>
			<p class="hint">{m.qg_language_hint()}</p>
			<button class="primary" type="submit" disabled={!valid || saving}
				>{saving ? m.qg_starting() : m.qg_generate()}</button
			>
		</form>
		{#if createError}<p role="alert" class="error">{m.qg_create_error()}</p>{/if}
	</dialog>
</div>

<style>
	.question-generator {
		color-scheme: light;
		--bg: #f2f4f7;
		--panel: #fff;
		--line: #e4e6eb;
		--text: #111827;
		--muted: #6b7280;
		--accent: #4f46e5;
		background: var(--bg);
		color: var(--text);
		padding: 32px;
		min-height: 100%;
		user-select: text;
	}
	.question-generator.dark {
		color-scheme: dark;
		--bg: #171b26;
		--panel: #1f2333;
		--line: #2d3348;
		--text: #e2e8f0;
		--muted: #94a3b8;
		--accent: #a5b4fc;
	}
	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 24px;
		margin-bottom: 28px;
	}
	header > div {
		max-width: 850px;
	}
	h1 {
		font-size: 28px;
		font-weight: 650;
		letter-spacing: -0.6px;
		margin: 4px 0 10px;
	}
	header p,
	.hint,
	.empty,
	small {
		color: var(--muted);
	}
	.eyebrow {
		font-size: 11px;
		letter-spacing: 1.6px;
		text-transform: uppercase;
	}
	h2 {
		font-size: 16px;
		font-weight: 600;
		margin: 0 0 20px;
	}
	p {
		line-height: 1.6;
	}
	.panel {
		border: 1px solid var(--line);
		border-radius: 12px;
		padding: 24px;
		background: var(--panel);
		min-width: 0;
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	label {
		font-size: 13px;
		font-weight: 600;
	}
	textarea,
	input,
	select {
		background: var(--bg);
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 10px 12px;
		width: 100%;
		min-width: 0;
		font: inherit;
	}
	textarea {
		resize: vertical;
		margin-bottom: 8px;
	}
	input:focus,
	textarea:focus,
	select:focus,
	button:focus-visible,
	summary:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	button {
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 10px 14px;
		font-size: 13px;
		cursor: pointer;
		background: var(--bg);
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.primary {
		background: #4f46e5;
		color: white;
		border-color: #4f46e5;
		padding: 12px;
		font-weight: 600;
	}
	.record-row {
		display: flex;
		gap: 8px;
		margin-bottom: 12px;
	}
	input:disabled {
		opacity: 0.6;
	}
	.progress {
		margin: 16px 0;
		font-size: 12px;
		color: var(--muted);
	}
	.progress-counts {
		display: flex;
		flex-wrap: wrap;
		gap: 16px;
	}
	.counts {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 16px;
		margin-top: 8px;
	}
	.metric-count {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.metric-count .hint {
		margin: 0;
	}
	.counts label {
		font-size: 12px;
	}
	.hint {
		font-size: 12px;
		margin: 8px 0;
	}
	.badge {
		font-size: 11px;
		border: 1px solid var(--line);
		border-radius: 4px;
		padding: 3px 7px;
		color: var(--accent);
		white-space: nowrap;
	}
	.error {
		color: #ef7373;
	}
	.results {
		margin-top: 20px;
	}
	.result-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		align-items: center;
		font-size: 12px;
		color: var(--muted);
	}
	.question {
		border-top: 1px solid var(--line);
		margin-top: 24px;
		padding-top: 24px;
	}
	.question-heading {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px;
	}
	.index {
		font-size: 12px;
		color: var(--muted);
		font-variant-numeric: tabular-nums;
	}
	h3 {
		font-size: 17px;
		font-weight: 600;
		line-height: 1.6;
		margin: 10px 0 20px;
	}
	h4 {
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
		margin: 16px 0 8px;
	}
	.answer {
		white-space: pre-wrap;
	}
	summary {
		cursor: pointer;
		font-size: 13px;
		color: var(--accent);
		margin: 16px 0;
	}
	.step-lines {
		display: block;
	}
	ol {
		padding-left: 24px;
	}
	li {
		padding: 4px 0;
		line-height: 1.6;
	}
	blockquote {
		border-left: 2px solid var(--accent);
		padding: 4px 16px;
		margin: 12px 0;
		font-size: 13px;
	}
	blockquote span {
		color: var(--muted);
		font-size: 11px;
	}
	blockquote p {
		white-space: pre-wrap;
	}
	pre {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-size: 12px;
	}
	@media (max-width: 900px) {
		.question-generator {
			padding: 20px;
		}
		.panel {
			padding: 18px;
		}
	}

	.open-generation {
		flex-shrink: 0;
	}
	.filters > div {
		min-width: 0;
	}
	.filters {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 16px;
		align-items: end;
	}
	.filters label {
		display: block;
		margin-bottom: 8px;
	}
	.date-filter {
		grid-column: span 2;
	}
	.filter-actions {
		grid-column: 1 / -1;
		justify-content: flex-end;
		display: flex;
		gap: 8px;
	}
	.filter-actions button {
		min-width: 88px;
	}
	.generation-status {
		margin-top: 20px;
		border-left: 3px solid var(--accent);
	}
	.results-heading,
	.dialog-heading {
		display: flex;
		justify-content: space-between;
		gap: 16px;
		align-items: center;
	}
	.results-heading h2,
	.dialog-heading h2 {
		margin: 0;
	}
	.results-heading > span,
	.pagination {
		color: var(--muted);
		font-size: 12px;
	}
	.question-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 18px;
		margin-top: 12px;
		font-size: 11px;
		color: var(--muted);
		overflow-wrap: anywhere;
	}
	.question {
		overflow-wrap: anywhere;
	}
	.pagination {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 16px;
		padding-top: 24px;
		margin-top: 24px;
		border-top: 1px solid var(--line);
	}
	dialog {
		color: var(--text);
		background: var(--panel);
		border: 1px solid var(--line);
		border-radius: 12px;
		padding: 28px;
		width: min(640px, calc(100vw - 32px));
		max-height: calc(100dvh - 48px);
		overflow: auto;
		margin: auto;
		user-select: text;
	}
	dialog::backdrop {
		background: rgb(0 0 0 / 0.65);
	}
	.dialog-heading {
		margin-bottom: 24px;
	}
	@media (max-width: 1100px) {
		.date-filter {
			grid-column: auto;
		}
		.filters {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@media (max-width: 600px) {
		header {
			align-items: start;
			flex-direction: column;
		}
		.filters,
		.counts {
			grid-template-columns: 1fr;
		}
		.question-generator {
			padding: 16px;
		}
		dialog {
			padding: 20px;
		}
		.pagination {
			justify-content: space-between;
			gap: 8px;
		}
	}
</style>
