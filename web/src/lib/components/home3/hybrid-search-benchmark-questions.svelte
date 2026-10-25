<script lang="ts">
	// "Tests to run": a button plus a dialog that searches the questions of one purpose and lets the
	// user tick the ones a benchmark run should use. An empty selection means every question.
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		listBenchmarkQuestions,
		type BenchmarkQuestion,
		type BenchmarkQuestionFilters
	} from '$lib/services/hybridSearchBenchmarkService';
	import {
		getQuestionFilters,
		type QuestionFilterOptions
	} from '$lib/services/questionGeneratorService';

	let {
		purpose,
		selected = $bindable([]),
		disabled = false
	}: { purpose: string; selected?: number[]; disabled?: boolean } = $props();

	const emptyFilters = (): BenchmarkQuestionFilters => ({
		purpose,
		created_from: '',
		created_to: '',
		run_id: '',
		hop_type: '',
		lang: '',
		input_record_id: '',
		question: '',
		answer: '',
		metric_id: ''
	});
	let filters = $state(emptyFilters());
	let options = $state<QuestionFilterOptions>({
		purposes: [],
		run_ids: [],
		record_ids: [],
		languages: []
	});
	let questions = $state<BenchmarkQuestion[]>([]);
	let total = $state(0);
	let limit = $state(0);
	let loading = $state(false);
	let loadError = $state(false);
	let dialog: HTMLDialogElement;
	let controller: AbortController | undefined;

	let selectedSet = $derived(new Set(selected));
	let shownSelected = $derived(questions.filter((q) => selectedSet.has(q.id)).length);
	let allShownSelected = $derived(questions.length > 0 && shownSelected === questions.length);
	let validDates = $derived(
		!filters.created_from || !filters.created_to || filters.created_from <= filters.created_to
	);

	const hopLabel = (hop: string) =>
		hop === 'one_hop' ? m.qg_one_hop() : hop === 'two_hop' ? m.qg_two_hop() : m.qg_multi_hop();
	const langLabel = (lang: string) => (lang === 'en' ? m.qg_english() : m.qg_chinese());
	const when = (iso: string) => new Date(iso).toLocaleString(getLocale());

	function open() {
		filters = { ...filters, purpose };
		dialog.showModal();
		void search();
		void getQuestionFilters()
			.then((o) => (options = o))
			.catch(() => {});
	}

	async function search(event?: SubmitEvent) {
		event?.preventDefault();
		if (!validDates) return;
		controller?.abort();
		const current = new AbortController();
		controller = current;
		loading = true;
		loadError = false;
		try {
			const result = await listBenchmarkQuestions(filters, current.signal);
			if (current.signal.aborted) return;
			questions = result.questions ?? [];
			total = result.total;
			limit = result.limit;
		} catch {
			if (!current.signal.aborted) loadError = true;
		} finally {
			if (!current.signal.aborted) loading = false;
		}
	}

	function reset() {
		filters = emptyFilters();
		void search();
	}

	function toggle(id: number) {
		selected = selectedSet.has(id) ? selected.filter((v) => v !== id) : [...selected, id];
	}

	// Ticks (or unticks) every question of the current search; selections outside it are kept.
	function toggleShown() {
		const shown = new Set(questions.map((q) => q.id));
		selected = allShownSelected
			? selected.filter((id) => !shown.has(id))
			: [...selected, ...questions.filter((q) => !selectedSet.has(q.id)).map((q) => q.id)];
	}
</script>

<!-- A form field like its neighbours in the options row: the selection summary is the label on top. -->
<div class="tests-field">
	<label for="hsbq-open"
		>{selected.length === 0
			? m.hsbq_summary_all()
			: m.hsbq_summary_selected({ count: selected.length })}</label
	>
	<button id="hsbq-open" type="button" class="primary tests-button" {disabled} onclick={open}
		>{m.hsbq_button()}</button
	>
</div>

<dialog
	bind:this={dialog}
	aria-labelledby="hsbq-title"
	onclose={() => controller?.abort()}
	onclick={(event) => {
		// Close on a backdrop click only (the dialog's own padding is also the event target).
		if (event.target !== dialog) return;
		const rect = dialog.getBoundingClientRect();
		if (
			event.clientX < rect.left ||
			event.clientX > rect.right ||
			event.clientY < rect.top ||
			event.clientY > rect.bottom
		)
			dialog.close();
	}}
>
	<div class="dialog-heading">
		<h2 id="hsbq-title">{m.hsbq_title()}</h2>
		<button type="button" onclick={() => dialog.close()}>{m.qg_close()}</button>
	</div>
	<p class="hint">{m.hsbq_intro()}</p>

	<form class="filters" onsubmit={search} aria-label={m.hsbq_filters()}>
		<div>
			<label for="hsbq-from">{m.qg_time_from()}</label>
			<input id="hsbq-from" type="datetime-local" bind:value={filters.created_from} />
		</div>
		<div>
			<label for="hsbq-to">{m.qg_time_to()}</label>
			<input id="hsbq-to" type="datetime-local" bind:value={filters.created_to} />
		</div>
		<div>
			<label for="hsbq-run">{m.qg_filter_run()}</label>
			<select id="hsbq-run" bind:value={filters.run_id}
				><option value="">{m.qg_all()}</option>{#each options.run_ids as value}<option
						value={String(value)}>{value}</option
					>{/each}</select
			>
		</div>
		<div>
			<label for="hsbq-hop">{m.hsb_col_hop()}</label>
			<select id="hsbq-hop" bind:value={filters.hop_type}>
				<option value="">{m.qg_all()}</option>
				<option value="one_hop">{m.qg_one_hop()}</option>
				<option value="two_hop">{m.qg_two_hop()}</option>
				<option value="multi_hop">{m.qg_multi_hop()}</option>
			</select>
		</div>
		<div>
			<label for="hsbq-lang">{m.qg_filter_language()}</label>
			<select id="hsbq-lang" bind:value={filters.lang}
				><option value="">{m.qg_all()}</option>{#each options.languages as value}<option {value}
						>{langLabel(value)}</option
					>{/each}</select
			>
		</div>
		<div>
			<label for="hsbq-record">{m.qg_filter_record()}</label>
			<select id="hsbq-record" bind:value={filters.input_record_id}
				><option value="">{m.qg_all()}</option>{#each options.record_ids as value}<option
						value={String(value)}>{value}</option
					>{/each}</select
			>
		</div>
		<div>
			<label for="hsbq-question">{m.hsbq_question_words()}</label>
			<input
				id="hsbq-question"
				maxlength="200"
				bind:value={filters.question}
				placeholder={m.hsbq_words_placeholder()}
			/>
		</div>
		<div>
			<label for="hsbq-answer">{m.hsbq_answer_words()}</label>
			<input
				id="hsbq-answer"
				maxlength="200"
				bind:value={filters.answer}
				placeholder={m.hsbq_words_placeholder()}
			/>
		</div>
		<div>
			<label for="hsbq-metric">{m.hsb_col_target()}</label>
			<input
				id="hsbq-metric"
				bind:value={filters.metric_id}
				placeholder={m.hsbq_metric_placeholder()}
			/>
		</div>
		<div class="filter-actions">
			<button class="primary" type="submit" disabled={!validDates || loading}
				>{m.qg_search()}</button
			>
			<button type="button" onclick={reset}>{m.qg_reset()}</button>
		</div>
		{#if !validDates}<p class="error" role="alert">{m.qg_invalid_dates()}</p>{/if}
	</form>

	<div class="result-bar" aria-live="polite">
		<span
			>{m.hsbq_counts({ total, selected: selected.length })}{#if total > questions.length}
				· {m.hsbq_truncated({ limit })}{/if}</span
		>
		<span class="bar-actions">
			<button type="button" disabled={questions.length === 0} onclick={toggleShown}
				>{allShownSelected ? m.hsbq_unselect_matching() : m.hsbq_select_matching()}</button
			>
			<button type="button" disabled={selected.length === 0} onclick={() => (selected = [])}
				>{m.hsbq_clear()}</button
			>
		</span>
	</div>

	<div class="table-wrap" aria-busy={loading}>
		{#if loadError}<p class="error" role="alert">{m.qg_load_error()}</p>
		{:else if loading && questions.length === 0}<p class="hint" role="status">{m.qg_loading()}</p>
		{:else if questions.length === 0}<p class="hint">{m.qg_no_questions()}</p>
		{:else}
			<table>
				<thead>
					<tr>
						<th class="check"
							><input
								type="checkbox"
								checked={allShownSelected}
								indeterminate={shownSelected > 0 && !allShownSelected}
								onchange={toggleShown}
								aria-label={m.hsbq_select_matching()}
							/></th
						>
						<th>{m.hsb_col_question()}</th>
						<th>{m.hsbq_col_answer()}</th>
						<th>{m.hsb_col_target()}</th>
						<th>{m.hsb_col_hop()}</th>
						<th>{m.hsb_col_lang()}</th>
						<th class="num">{m.qg_filter_record()}</th>
						<th class="num">{m.qg_filter_run()}</th>
						<th>{m.hsbq_col_created()}</th>
					</tr>
				</thead>
				<tbody>
					{#each questions as q (q.id)}
						<tr class:picked={selectedSet.has(q.id)}>
							<td class="check"
								><input
									type="checkbox"
									checked={selectedSet.has(q.id)}
									onchange={() => toggle(q.id)}
									aria-label={m.hsbq_select_question({ id: q.id })}
								/></td
							>
							<td class="text">{q.question}</td>
							<td class="text answer">{q.answer}</td>
							<td class="mono">{q.metric_id}</td>
							<td>{hopLabel(q.hop_type)}</td>
							<td>{langLabel(q.lang)}</td>
							<td class="num">{q.input_record_id}</td>
							<td class="num">{q.run_id}</td>
							<td class="nowrap">{when(q.created_at)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>

	<div class="dialog-footer">
		<span class="hint">{m.hsbq_footer_hint()}</span>
		<button class="primary" type="button" onclick={() => dialog.close()}>{m.hsbq_done()}</button>
	</div>
</dialog>

<style>
	.tests-field label {
		display: block;
		font-size: 13px;
		font-weight: 600;
		margin-bottom: 8px;
		white-space: nowrap;
	}
	.tests-button {
		padding: 10px 16px;
		white-space: nowrap;
	}
	dialog {
		color: var(--text);
		background: var(--panel);
		border: 1px solid var(--line);
		border-radius: 12px;
		padding: 24px;
		width: min(1200px, calc(100vw - 32px));
		max-height: calc(100dvh - 48px);
		overflow: auto;
		margin: auto;
		user-select: text;
	}
	dialog::backdrop {
		background: rgb(0 0 0 / 0.65);
	}
	.dialog-heading,
	.dialog-footer,
	.result-bar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 16px;
	}
	.dialog-heading h2 {
		margin: 0;
		font-size: 16px;
	}
	.dialog-footer {
		margin-top: 16px;
		position: sticky;
		bottom: -24px;
		background: var(--panel);
		padding: 12px 0;
		border-top: 1px solid var(--line);
	}
	.filters {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px 16px;
		align-items: end;
		margin: 16px 0;
	}
	.filters label {
		display: block;
		font-size: 12px;
		font-weight: 600;
		margin-bottom: 6px;
	}
	.filters input,
	.filters select {
		width: 100%;
		min-width: 0;
		background: var(--bg);
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 8px 10px;
		font: inherit;
		font-size: 13px;
	}
	.filter-actions {
		grid-column: 1 / -1;
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}
	.filters .error {
		grid-column: 1 / -1;
	}
	.result-bar {
		font-size: 12px;
		color: var(--muted);
		padding: 8px 0;
		border-top: 1px solid var(--line);
		flex-wrap: wrap;
	}
	.bar-actions {
		display: flex;
		gap: 8px;
	}
	.table-wrap {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 12px;
	}
	th,
	td {
		text-align: left;
		padding: 6px 8px;
		border-bottom: 1px solid var(--line);
		vertical-align: top;
	}
	th {
		font-size: 11px;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.6px;
		white-space: nowrap;
	}
	.check {
		width: 28px;
	}
	.check input {
		cursor: pointer;
	}
	tr.picked td {
		background: color-mix(in srgb, var(--accent) 8%, transparent);
	}
	.text {
		min-width: 240px;
	}
	.answer {
		color: var(--muted);
	}
	.num {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		white-space: nowrap;
	}
	.nowrap {
		white-space: nowrap;
	}
	.hint {
		font-size: 12px;
		color: var(--muted);
	}
	.error {
		color: #ef7373;
		font-size: 12px;
	}
	button {
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 7px 12px;
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
		font-weight: 600;
	}
	button:focus-visible,
	input:focus,
	select:focus {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	@media (max-width: 900px) {
		.filters {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@media (max-width: 600px) {
		.filters {
			grid-template-columns: 1fr;
		}
		dialog {
			padding: 16px;
		}
	}
</style>
