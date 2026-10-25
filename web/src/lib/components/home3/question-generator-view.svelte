<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import QuestionRecord from './question-record.svelte';
	import QuestionGenerationJobs from './question-generation-jobs.svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		archiveQuestions,
		createQuestionRun,
		QuestionCapacityError,
		getQuestionRun,
		listQuestions,
		getQuestionFilters,
		type QuestionRun,
		type QuestionRequest,
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
	let view = $state<'list' | 'cards'>('cards');
	let contentTab = $state<'questions' | 'jobs'>('questions');
	let jobsPanel = $state<ReturnType<typeof QuestionGenerationJobs>>();
	let statusPanel = $state<HTMLElement>();
	let selected = $state<number[]>([]);
	let allSelected = $derived(
		questions.length > 0 && questions.every((q) => selected.includes(q.id))
	);
	let partiallySelected = $derived(selected.length > 0 && !allSelected);
	let deleting = $state(false);
	let deleteError = $state(false);
	let archivedCount = $state<number | null>(null);
	let archiveController: AbortController | undefined;
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
	let numQuestionsToGenerate = $state(30);
	let totalMetrics = $state(10);
	let saving = $state(false);
	let createError = $state(false);
	let capacityError = $state(false);
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
			(singleDocument
				? Number.isInteger(numQuestionsToGenerate) &&
					numQuestionsToGenerate >= 1 &&
					numQuestionsToGenerate <= 3000
				: Number.isInteger(metricsPerDocument) &&
					metricsPerDocument >= 1 &&
					metricsPerDocument <= 50 &&
					Number.isInteger(totalMetrics) &&
					totalMetrics >= 1 &&
					totalMetrics <= 1000)
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
		archiveController?.abort();
		filterController?.abort();
		createController?.abort();
		pollController?.abort();
		clearTimeout(timer);
	});
	function selectAll(checked: boolean) {
		selected = checked ? questions.map((q) => q.id) : [];
	}
	function selectQuestion(id: number, checked: boolean) {
		selected = checked ? [...new Set([...selected, id])] : selected.filter((value) => value !== id);
	}
	function changeTab(event: KeyboardEvent) {
		if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
		event.preventDefault();
		view =
			event.key === 'Home'
				? 'list'
				: event.key === 'End'
					? 'cards'
					: view === 'list'
						? 'cards'
						: 'list';
		document.getElementById(`qg-tab-${view}`)?.focus();
	}
	function changeContentTab(event: KeyboardEvent) {
		if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
		event.preventDefault();
		contentTab =
			event.key === 'Home'
				? 'questions'
				: event.key === 'End'
					? 'jobs'
					: contentTab === 'questions'
						? 'jobs'
						: 'questions';
		document.getElementById(`qg-content-tab-${contentTab}`)?.focus();
	}
	async function deleteSelected() {
		if (deleting || loading || selected.length === 0) return;
		if (!confirm(m.qg_delete_confirm({ count: selected.length }))) return;
		archiveController = new AbortController();
		deleting = true;
		deleteError = false;
		archivedCount = null;
		listController?.abort();
		try {
			const result = await archiveQuestions([...selected], archiveController.signal);
			if (!alive) return;
			selected = [];
			archivedCount = result.archived_count;
			await loadQuestions(
				Math.min(page, Math.max(1, Math.ceil((total - result.archived_count) / pageSize)))
			);
			void loadOptions();
		} catch {
			if (alive && !archiveController.signal.aborted) deleteError = true;
		} finally {
			if (alive) deleting = false;
		}
	}
	async function loadQuestions(nextPage: number) {
		listController?.abort();
		const controller = new AbortController();
		listController = controller;
		const current = ++version;
		loading = true;
		selected = [];
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
		if (!validDates || deleting) return;
		applied = { ...draft };
		void loadQuestions(1);
	}
	function reset() {
		if (deleting) return;
		draft = emptyFilters();
		applied = emptyFilters();
		void loadQuestions(1);
		void loadOptions();
	}
	async function viewJob(job: QuestionRun) {
		activeRun = job;
		runQuestions = [];
		void poll(job.id);
		await tick();
		statusPanel?.scrollIntoView({ block: 'start' });
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
				if (!deleting) void loadQuestions(page);
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
		capacityError = false;
		try {
			let payload: QuestionRequest = {
				input_record_id: normalizedRecordID,
				purpose,
				description: description.trim(),
				...(singleDocument
					? { num_questions_to_generate: numQuestionsToGenerate }
					: { metrics_per_document: metricsPerDocument, total_metrics: totalMetrics })
			};
			let run: QuestionRun;
			while (true) {
				try {
					({ run } = await createQuestionRun(payload, createController.signal));
					break;
				} catch (error) {
					if (!alive || createController.signal.aborted) return;
					if (!(error instanceof QuestionCapacityError)) throw error;
					if (!error.confirmationRequired) {
						capacityError = true;
						return;
					}
					if (
						!confirm(
							m.qg_capacity_confirm({
								requested: payload.num_questions_to_generate ?? 0,
								metrics: error.availableMetrics,
								questions: error.availableQuestions
							})
						)
					)
						return;
					payload = { ...payload, confirmed_available_questions: error.availableQuestions };
				}
			}
			if (!alive) return;
			activeRun = run;
			runQuestions = [];
			jobsPanel?.refresh();
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
	<form
		class="panel filters"
		hidden={contentTab !== 'questions'}
		onsubmit={search}
		aria-label={m.qg_filter_questions()}
	>
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
			<button class="primary" type="submit" disabled={!validDates || deleting}
				>{m.qg_search()}</button
			><button type="button" disabled={deleting} onclick={reset}>{m.qg_reset()}</button>
		</div>
		{#if !validDates}<p class="error" role="alert">{m.qg_invalid_dates()}</p>{/if}
		{#if filtersError}<p class="error" role="alert">
				{m.qg_filter_error()} <button type="button" onclick={loadOptions}>{m.qg_retry()}</button>
			</p>{/if}
	</form>
	{#if activeRun}
		<section
			class="panel generation-status"
			aria-label={m.qg_generation_status()}
			bind:this={statusPanel}
		>
			<div class="result-meta">
				<strong>{m.qg_run_number({ id: activeRun.id })}</strong><span
					class="badge"
					class:running={activeRun.status === 'running'}>{statusLabel(activeRun.status)}</span
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
	<section class="panel results" aria-label={m.qg_content_tabs()}>
		<div class="content-tabs" role="tablist" aria-label={m.qg_content_tabs()}>
			<button
				id="qg-content-tab-questions"
				type="button"
				role="tab"
				aria-selected={contentTab === 'questions'}
				aria-controls="qg-content-panel-questions"
				tabindex={contentTab === 'questions' ? 0 : -1}
				onkeydown={changeContentTab}
				onclick={() => (contentTab = 'questions')}>{m.qg_results()}</button
			>
			<button
				id="qg-content-tab-jobs"
				type="button"
				role="tab"
				aria-selected={contentTab === 'jobs'}
				aria-controls="qg-content-panel-jobs"
				tabindex={contentTab === 'jobs' ? 0 : -1}
				onkeydown={changeContentTab}
				onclick={() => (contentTab = 'jobs')}>{m.qg_jobs()}</button
			>
		</div>
		<div
			id="qg-content-panel-questions"
			role="tabpanel"
			aria-labelledby="qg-content-tab-questions"
			hidden={contentTab !== 'questions'}
			aria-busy={loading}
			tabindex="0"
		>
			<div class="results-heading">
				<h2 id="qg-results-title">{m.qg_results()}</h2>
				<span>{m.qg_result_count({ count: total })}</span>
			</div>
			<p class="hint">{m.qg_review_hint()}</p>
			<div class="results-toolbar">
				<div class="view-tabs" role="tablist" aria-label={m.qg_results()}>
					<button
						id="qg-tab-list"
						role="tab"
						aria-selected={view === 'list'}
						aria-controls="qg-panel-list"
						tabindex={view === 'list' ? 0 : -1}
						onkeydown={changeTab}
						onclick={() => (view = 'list')}>{m.qg_list()}</button
					>
					<button
						id="qg-tab-cards"
						role="tab"
						aria-selected={view === 'cards'}
						aria-controls="qg-panel-cards"
						tabindex={view === 'cards' ? 0 : -1}
						onkeydown={changeTab}
						onclick={() => (view = 'cards')}>{m.qg_cards()}</button
					>
				</div>
				<div class="selection-actions">
					{#if view === 'cards'}{@render selectAllControl()}{/if}
					<span>{m.qg_selected_count({ count: selected.length })}</span>
					<button
						class="delete"
						disabled={deleting || loading || selected.length === 0}
						title={m.qg_delete_hint()}
						onclick={deleteSelected}>{deleting ? m.qg_deleting() : m.qg_delete()}</button
					>
				</div>
			</div>
			{#if deleteError}<p class="error" role="alert">{m.qg_delete_error()}</p>{/if}
			{#if archivedCount !== null}<p class="hint" role="status">
					{m.qg_archived_count({ count: archivedCount })}
				</p>{/if}
			<div
				id="qg-panel-list"
				role="tabpanel"
				aria-labelledby="qg-tab-list"
				hidden={view !== 'list'}
				tabindex="0"
			>
				{#if view === 'list'}{@render results()}{/if}
			</div>
			<div
				id="qg-panel-cards"
				role="tabpanel"
				aria-labelledby="qg-tab-cards"
				hidden={view !== 'cards'}
				tabindex="0"
			>
				{#if view === 'cards'}{@render results()}{/if}
			</div>
			{#snippet selectAllControl()}
				<label class="select-all" title={m.qg_select_all_hint()}>
					<input
						type="checkbox"
						checked={allSelected}
						indeterminate={partiallySelected}
						disabled={loading || deleting || loadError || questions.length === 0}
						onchange={(event) => selectAll(event.currentTarget.checked)}
					/>
					{m.qg_select_all()}
				</label>
			{/snippet}
			{#snippet results()}
				{#if loading}<p role="status">{m.qg_loading()}</p>
				{:else if loadError}<p class="error" role="alert">{m.qg_load_error()}</p>
					<button onclick={() => void loadQuestions(page)}>{m.qg_retry()}</button>
				{:else if questions.length === 0}<p class="empty">{m.qg_no_questions()}</p>
				{:else if view === 'cards'}
					<div class="question-grid">
						{#each questions as question, index (question.id)}
							<QuestionRecord
								{question}
								index={(page - 1) * pageSize + index + 1}
								selected={selected.includes(question.id)}
								disabled={deleting}
								onselect={(checked) => selectQuestion(question.id, checked)}
							/>
						{/each}
					</div>
				{:else}
					<div class="question-list">
						<table>
							<thead
								><tr>
									<th>{@render selectAllControl()}</th><th>{m.qg_field_question()}</th><th
										>{m.qg_answer()}</th
									>
									<th>{m.qg_filter_run()}</th><th>{m.qg_filter_record()}</th><th
										>{m.qg_filter_language()}</th
									><th>{m.qg_reasoning_evidence()}</th>
								</tr></thead
							>
							<tbody
								>{#each questions as question, index (question.id)}<tr>
										<td
											><input
												type="checkbox"
												checked={selected.includes(question.id)}
												disabled={deleting}
												aria-label={m.qg_select_question({
													number: (page - 1) * pageSize + index + 1
												})}
												onchange={(event) =>
													selectQuestion(question.id, event.currentTarget.checked)}
											/></td
										>
										<td lang={question.lang}>{question.question}</td><td lang={question.lang}
											>{question.answer}</td
										>
										<td>{question.run_id}</td><td>{question.input_record_id}</td><td
											>{languageLabel(question.lang)}</td
										>
										<td
											><details>
												<summary>{m.qg_details()}</summary>
												<QuestionRecord {question} index={(page - 1) * pageSize + index + 1} />
											</details></td
										>
									</tr>{/each}</tbody
							>
						</table>
					</div>
				{/if}
			{/snippet}
			<nav class="pagination" aria-label={m.qg_pagination()}>
				<button
					disabled={loading || deleting || page <= 1}
					onclick={() => void loadQuestions(page - 1)}>{m.qg_previous()}</button
				><span>{m.qg_page_status({ page, pages: Math.max(1, Math.ceil(total / pageSize)) })}</span
				><button
					disabled={loading || deleting || page * pageSize >= total}
					onclick={() => void loadQuestions(page + 1)}>{m.qg_next()}</button
				>
			</nav>
		</div>
		<div
			id="qg-content-panel-jobs"
			role="tabpanel"
			aria-labelledby="qg-content-tab-jobs"
			hidden={contentTab !== 'jobs'}
			tabindex="0"
		>
			{#if contentTab === 'jobs'}<QuestionGenerationJobs
					bind:this={jobsPanel}
					{darkMode}
					onview={viewJob}
				/>{/if}
		</div>
	</section>
	<dialog
		bind:this={dialog}
		aria-labelledby="qg-config-title"
		onpointerdown={(event) => {
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
			{#if singleDocument}
				<label for="qg-num-questions">{m.qg_num_questions()}</label>
				<input
					id="qg-num-questions"
					type="number"
					min="1"
					max="3000"
					step="1"
					required
					bind:value={numQuestionsToGenerate}
					disabled={saving}
				/>
				<p class="hint">{m.qg_num_questions_hint()}</p>
				{#if valid}
					<p class="hint">
						{m.qg_question_plan({
							metrics: Math.ceil(numQuestionsToGenerate / 3),
							questions: Math.ceil(numQuestionsToGenerate / 3) * 3
						})}
					</p>
				{/if}
			{:else}
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
			{/if}
			<p class="hint">{m.qg_metric_triplet_hint()}</p>
			<p class="hint">{m.qg_language_hint()}</p>
			<button class="primary" type="submit" disabled={!valid || saving}
				>{saving ? m.qg_starting() : m.qg_generate()}</button
			>
		</form>
		{#if createError}<p role="alert" class="error">{m.qg_create_error()}</p>{/if}
		{#if capacityError}<p role="alert" class="error">{m.qg_no_usable_metrics()}</p>{/if}
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
	.empty {
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
	.badge.running {
		--running-background: #fef3c7;
		--running-highlight: #fcd34d;
		color: #78350f;
		background: var(--running-background);
		border-color: #d97706;
		font-weight: 700;
		animation: running-flash 1.4s ease-in-out infinite;
	}
	.dark .badge.running {
		--running-background: #78350f;
		--running-highlight: #92400e;
		color: #fef3c7;
		border-color: #f59e0b;
	}
	@keyframes running-flash {
		50% {
			background: var(--running-highlight);
			box-shadow: 0 0 0 3px #f59e0b40;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.badge.running {
			animation: none;
		}
	}
	.error {
		color: #ef7373;
	}
	.results {
		margin-top: 20px;
		container-type: inline-size;
	}
	.results-toolbar,
	.content-tabs,
	.selection-actions,
	.view-tabs {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.results-toolbar {
		justify-content: space-between;
		flex-wrap: wrap;
		margin-top: 20px;
	}
	.select-all {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		white-space: nowrap;
		cursor: pointer;
	}
	.selection-actions {
		font-size: 13px;
	}
	.view-tabs {
		gap: 4px;
	}
	.view-tabs [aria-selected='true'] {
		color: var(--accent);
		border-color: var(--accent);
	}
	.filters[hidden] {
		display: none;
	}
	.content-tabs {
		gap: 24px;
		border-bottom: 1px solid var(--line);
		margin-bottom: 24px;
		overflow-x: auto;
	}
	.content-tabs button {
		border: none;
		border-bottom: 2px solid transparent;
		border-radius: 0;
		padding: 12px 0;
		background: none;
		color: var(--muted);
		white-space: nowrap;
		font-weight: 600;
	}
	.content-tabs button[aria-selected='true'] {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}
	.delete {
		color: #dc2626;
	}
	.dark .delete {
		color: #fca5a5;
	}
	input[type='checkbox'] {
		width: 16px;
		height: 16px;
		padding: 0;
		accent-color: #4f46e5;
	}
	.question-list {
		overflow-x: auto;
		margin-top: 24px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	th,
	td {
		padding: 12px;
		text-align: start;
		vertical-align: top;
		border-bottom: 1px solid var(--line);
	}
	th {
		color: var(--muted);
		white-space: nowrap;
	}
	td {
		min-width: 70px;
		overflow-wrap: anywhere;
	}
	td:nth-child(2),
	td:nth-child(3) {
		min-width: 220px;
	}
	td details[open] {
		min-width: 360px;
	}
	.question-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 16px;
		align-items: start;
		margin-top: 24px;
	}
	@container (max-width: 1100px) {
		.question-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@container (max-width: 720px) {
		.question-grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.result-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		align-items: center;
		font-size: 12px;
		color: var(--muted);
	}
	summary {
		cursor: pointer;
		font-size: 13px;
		color: var(--accent);
		margin: 16px 0;
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
