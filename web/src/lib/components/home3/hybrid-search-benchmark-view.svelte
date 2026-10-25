<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import HybridSearchBenchmarkQuestions from './hybrid-search-benchmark-questions.svelte';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		startBenchmarkRun,
		listBenchmarkRuns,
		getBenchmarkRun,
		type BenchmarkMode,
		type HybridBenchmarkRun,
		type HybridBenchmarkScore,
		type HybridBenchmarkResult,
		type HybridBenchmarkOptions
	} from '$lib/services/hybridSearchBenchmarkService';
	let { darkMode = true }: { darkMode?: boolean } = $props();

	const modes: BenchmarkMode[] = ['lexical', 'semantic', 'hybrid'];
	let purposes = $state<string[]>(['search_metrics']);
	let purpose = $state('search_metrics');
	let description = $state('');
	// Filled from the configured live-search settings on first load.
	let options = $state<HybridBenchmarkOptions>({
		query_keywords: true,
		lexical_weight: 1,
		semantic_weight: 1,
		candidate_limit: 200,
		parallelism: 8
	});
	let optionsLoaded = false;
	// Questions picked in "Tests to run"; empty = every question of the purpose.
	let questionIds = $state<number[]>([]);
	let starting = $state(false);
	let startError = $state(false);
	let runs = $state<HybridBenchmarkRun[]>([]);
	let runsLoading = $state(false);
	let runsError = $state(false);
	let selected = $state<HybridBenchmarkRun | null>(null);
	let scores = $state<HybridBenchmarkScore[]>([]);
	let results = $state<HybridBenchmarkResult[]>([]);
	let detailError = $state(false);
	let scoreGroup = $state<HybridBenchmarkScore['group']>('all');
	let alive = true;
	let runsController: AbortController | undefined;
	let detailController: AbortController | undefined;
	let startController: AbortController | undefined;
	let timer: ReturnType<typeof setTimeout> | undefined;
	let detailPanel = $state<HTMLElement | undefined>();

	let groupScores = $derived(scores.filter((s) => s.group === scoreGroup));
	let running = $derived(runs.some((r) => r.status === 'running'));
	const validWeight = (w: number) => Number.isFinite(w) && w >= 0 && w <= 100;
	let validOptions = $derived(
		validWeight(options.lexical_weight) &&
			validWeight(options.semantic_weight) &&
			options.lexical_weight + options.semantic_weight > 0 &&
			Number.isInteger(options.candidate_limit) &&
			options.candidate_limit >= 1 &&
			options.candidate_limit <= 1000 &&
			Number.isInteger(options.parallelism) &&
			options.parallelism >= 1 &&
			options.parallelism <= 32
	);

	const purposeLabel = (value: string) =>
		value === 'search_metrics' ? m.qg_search_metrics() : value;
	const statusLabel = (status: HybridBenchmarkRun['status']) =>
		status === 'running' ? m.qg_running() : status === 'done' ? m.qg_done() : m.qg_failed();
	const modeLabel = (mode: BenchmarkMode) =>
		mode === 'lexical'
			? m.hsb_mode_lexical()
			: mode === 'semantic'
				? m.hsb_mode_semantic()
				: m.hsb_mode_hybrid();
	const valueLabel = (score: HybridBenchmarkScore) => {
		if (score.group === 'all') return m.hsb_all_questions();
		if (score.value === 'one_hop') return m.qg_one_hop();
		if (score.value === 'two_hop') return m.qg_two_hop();
		if (score.value === 'multi_hop') return m.qg_multi_hop();
		if (score.value === 'en') return m.qg_english();
		if (score.value === 'zh-cn') return m.qg_chinese();
		return score.value;
	};
	const hopLabel = (hop: string) =>
		hop === 'one_hop' ? m.qg_one_hop() : hop === 'two_hop' ? m.qg_two_hop() : m.qg_multi_hop();
	const langLabel = (lang: string) => (lang === 'en' ? m.qg_english() : m.qg_chinese());
	const ratio = (v: number | undefined) => (v === undefined ? '' : v.toFixed(2));
	const when = (iso: string) => new Date(iso).toLocaleString(getLocale());
	const ms = (v: number) => m.hsb_ms({ ms: v.toLocaleString(getLocale()) });
	// Query time is what one live hybrid search costs: embedding + keywords + the hybrid query.
	const queryTime = (r: HybridBenchmarkResult) =>
		r.hybrid_ms === null ? null : (r.embedding_ms ?? 0) + (r.keywords_ms ?? 0) + r.hybrid_ms;
	const queryTimeDetail = (r: HybridBenchmarkResult) =>
		m.hsb_query_time_detail({
			embedding: ms(r.embedding_ms ?? 0),
			keywords: r.keywords_ms === null ? m.hsb_not_used() : ms(r.keywords_ms),
			search: ms(r.hybrid_ms ?? 0),
			lexical: r.lexical_ms === null ? '' : ms(r.lexical_ms),
			semantic: r.semantic_ms === null ? '' : ms(r.semantic_ms)
		});

	onMount(() => void loadRuns());
	onDestroy(() => {
		alive = false;
		runsController?.abort();
		detailController?.abort();
		startController?.abort();
		clearTimeout(timer);
	});

	async function loadRuns() {
		runsController?.abort();
		const controller = new AbortController();
		runsController = controller;
		runsLoading = true;
		runsError = false;
		try {
			const result = await listBenchmarkRuns(controller.signal);
			if (!alive || controller.signal.aborted) return;
			runs = result.runs ?? [];
			if (result.purposes?.length) purposes = result.purposes;
			if (result.defaults && !optionsLoaded) {
				options = { ...result.defaults };
				optionsLoaded = true;
			}
			if (!selected && runs.length > 0) void loadRun(runs[0].id);
		} catch {
			if (alive && !controller.signal.aborted) runsError = true;
		} finally {
			if (alive && !controller.signal.aborted) runsLoading = false;
		}
	}

	// Loads one run; while it is running, polls it and refreshes the run list when it ends.
	async function loadRun(id: number) {
		detailController?.abort();
		clearTimeout(timer);
		const controller = new AbortController();
		detailController = controller;
		detailError = false;
		try {
			const result = await getBenchmarkRun(id, controller.signal);
			if (!alive || controller.signal.aborted) return;
			const wasRunning = selected?.id === id && selected.status === 'running';
			selected = result.run;
			scores = result.scores ?? [];
			results = result.results ?? [];
			if (result.run.status === 'running') timer = setTimeout(() => void loadRun(id), 2000);
			else if (wasRunning) void loadRuns();
		} catch {
			if (alive && !controller.signal.aborted) detailError = true;
		}
	}

	// Opens a run's details and scrolls to them.
	async function view(id: number) {
		await loadRun(id);
		await tick();
		if (alive && selected?.id === id)
			detailPanel?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	async function start(event: SubmitEvent) {
		event.preventDefault();
		if (starting || running || !validOptions) return;
		startController = new AbortController();
		starting = true;
		startError = false;
		try {
			const { run } = await startBenchmarkRun(
				{ purpose, description: description.trim(), question_ids: questionIds, ...options },
				startController.signal
			);
			if (!alive) return;
			description = '';
			runs = [run, ...runs];
			selected = run;
			scores = [];
			results = [];
			void loadRun(run.id);
		} catch {
			if (alive && !startController.signal.aborted) startError = true;
		} finally {
			if (alive) starting = false;
		}
	}
</script>

<div class="hsb select-text" class:dark={darkMode}>
	<header>
		<p class="eyebrow">{m.hsb_eyebrow()}</p>
		<h1>{m.hsb_title()}</h1>
		<p>{m.hsb_intro()}</p>
	</header>

	<form class="panel start" onsubmit={start} aria-labelledby="hsb-start-title">
		<h2 id="hsb-start-title">{m.hsb_start_title()}</h2>
		<div class="start-fields">
			<div>
				<label for="hsb-purpose">{m.qg_purpose()}</label>
				<select
					id="hsb-purpose"
					bind:value={purpose}
					disabled={starting}
					onchange={() => (questionIds = [])}
					>{#each purposes as value}<option {value}>{purposeLabel(value)}</option>{/each}</select
				>
			</div>
			<div class="grow">
				<label for="hsb-description">{m.qg_description()}</label>
				<input
					id="hsb-description"
					maxlength="500"
					bind:value={description}
					disabled={starting}
					placeholder={m.hsb_description_placeholder()}
				/>
			</div>
			<button class="primary" type="submit" disabled={starting || running || !validOptions}
				>{starting ? m.qg_starting() : m.hsb_run()}</button
			>
		</div>
		<div class="tests">
			<HybridSearchBenchmarkQuestions {purpose} bind:selected={questionIds} disabled={starting} />
		</div>
		<div class="options">
			<label class="check"
				><input
					type="checkbox"
					bind:checked={options.query_keywords}
					disabled={starting}
				/>{m.hsb_query_keywords()}</label
			>
			<div>
				<label for="hsb-lexical-weight">{m.hsb_lexical_weight()}</label>
				<input
					id="hsb-lexical-weight"
					type="number"
					min="0"
					max="100"
					step="0.1"
					bind:value={options.lexical_weight}
					disabled={starting}
				/>
			</div>
			<div>
				<label for="hsb-semantic-weight">{m.hsb_semantic_weight()}</label>
				<input
					id="hsb-semantic-weight"
					type="number"
					min="0"
					max="100"
					step="0.1"
					bind:value={options.semantic_weight}
					disabled={starting}
				/>
			</div>
			<div>
				<label for="hsb-candidates">{m.hsb_candidate_limit()}</label>
				<input
					id="hsb-candidates"
					type="number"
					min="1"
					max="1000"
					step="1"
					bind:value={options.candidate_limit}
					disabled={starting}
				/>
			</div>
			<div>
				<label for="hsb-parallelism">{m.hsb_parallelism()}</label>
				<input
					id="hsb-parallelism"
					type="number"
					min="1"
					max="32"
					step="1"
					bind:value={options.parallelism}
					disabled={starting}
				/>
			</div>
		</div>
		<p class="hint">{m.hsb_options_hint()}</p>
		{#if !validOptions}<p class="error" role="alert">{m.hsb_invalid_options()}</p>{/if}
		<p class="hint">{m.hsb_start_hint()}</p>
		{#if running}<p class="hint">{m.hsb_one_at_a_time()}</p>{/if}
		{#if startError}<p class="error" role="alert">{m.hsb_start_error()}</p>{/if}
	</form>

	<section class="panel" aria-labelledby="hsb-runs-title" aria-busy={runsLoading}>
		<div class="heading">
			<h2 id="hsb-runs-title">{m.hsb_runs()}</h2>
			<button type="button" onclick={() => void loadRuns()} disabled={runsLoading}
				>{m.hsb_refresh()}</button
			>
		</div>
		{#if runsError}<p class="error" role="alert">{m.qg_load_error()}</p>
		{:else if runs.length === 0 && !runsLoading}<p class="hint">{m.hsb_no_runs()}</p>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>{m.hsb_col_run()}</th>
							<th>{m.hsb_col_started()}</th>
							<th>{m.hsb_col_status()}</th>
							<th>{m.hsb_col_progress()}</th>
							<th>{m.hsb_col_hybrid_r10()}</th>
							<th>{m.qg_description()}</th>
							<th><span class="sr-only">{m.hsb_col_actions()}</span></th>
						</tr>
					</thead>
					<tbody>
						{#each runs as run (run.id)}
							<tr class:active={selected?.id === run.id}>
								<td
									><button type="button" class="link" onclick={() => void view(run.id)}
										>{m.qg_run_number({ id: run.id })}</button
									></td
								>
								<td>{when(run.created_at)}</td>
								<td><span class="badge">{statusLabel(run.status)}</span></td>
								<td>{m.qg_progress_count({ count: run.done_count, total: run.question_count })}</td>
								<td class="num">{ratio(run.hybrid_r10)}</td>
								<td>{run.description}</td>
								<td class="actions"
									><button
										type="button"
										aria-label={m.hsb_view_run({ id: run.id })}
										onclick={() => void view(run.id)}>{m.hsb_view()}</button
									></td
								>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>

	{#if selected}
		<section class="panel detail" aria-labelledby="hsb-detail-title" bind:this={detailPanel}>
			<div class="heading">
				<h2 id="hsb-detail-title">{m.qg_run_number({ id: selected.id })}</h2>
				<span class="badge">{statusLabel(selected.status)}</span>
			</div>
			<div class="setting-groups">
				<section aria-labelledby="hsb-run-group">
					<h4 id="hsb-run-group">{m.hsb_group_run()}</h4>
					<dl class="settings">
						<dt>{m.qg_purpose()}</dt>
						<dd>{purposeLabel(selected.purpose)}</dd>
						<dt>{m.hsb_col_progress()}</dt>
						<dd>
							{m.qg_progress_count({
								count: selected.done_count,
								total: selected.question_count
							})}
						</dd>
						<dt>{m.hsb_col_started()}</dt>
						<dd>{when(selected.created_at)}</dd>
						<dt>{m.hsb_finished()}</dt>
						<dd>{selected.finished_at ? when(selected.finished_at) : ''}</dd>
						<dt>{m.qg_description()}</dt>
						<dd>{selected.description}</dd>
					</dl>
				</section>
				<section aria-labelledby="hsb-search-group">
					<h4 id="hsb-search-group">{m.hsb_group_search()}</h4>
					<dl class="settings">
						<dt>{m.hsb_lexical_backend()}</dt>
						<dd>{selected.lexical_backend}</dd>
						<dt>{m.hsb_query_keywords()}</dt>
						<dd>{selected.query_keywords ? m.hsb_on() : m.hsb_off()}</dd>
						<dt>{m.hsb_prompt()}</dt>
						<dd class="mono">{selected.keywords_prompt || m.hsb_not_used()}</dd>
						<dt>{m.hsb_model_name()}</dt>
						<dd class="mono">{selected.keywords_model || m.hsb_not_used()}</dd>
						<dt>{m.hsb_embedding_model()}</dt>
						<dd class="mono">{selected.embedding_model}</dd>
						<dt>{m.hsb_weights()}</dt>
						<dd>
							{m.hsb_weights_value({
								lexical: selected.lexical_weight,
								semantic: selected.semantic_weight
							})}
						</dd>
						<dt>{m.hsb_candidate_limit()}</dt>
						<dd>{selected.candidate_limit}</dd>
						<dt>{m.hsb_parallelism()}</dt>
						<dd>{selected.parallelism}</dd>
					</dl>
				</section>
			</div>
			{#if selected.status === 'running'}<p class="hint" role="status">
					{m.hsb_running_hint()}
				</p>{/if}
			{#if selected.status === 'failed' && selected.error_msg}<p class="error" role="alert">
					{selected.error_msg}
				</p>{/if}
			{#if detailError}<p class="error" role="alert">{m.qg_load_error()}</p>
				<button type="button" onclick={() => void loadRun(selected!.id)}>{m.qg_retry()}</button
				>{/if}

			<div class="heading sub">
				<h3>{m.hsb_scores()}</h3>
				<select bind:value={scoreGroup} aria-label={m.hsb_group_by()}>
					<option value="all">{m.hsb_all_questions()}</option>
					<option value="hop_type">{m.hsb_by_hop_type()}</option>
					<option value="lang">{m.hsb_by_language()}</option>
				</select>
			</div>
			<p class="hint">{m.hsb_scores_hint()}</p>
			{#if groupScores.length === 0}<p class="hint">{m.hsb_no_results()}</p>
			{:else}
				<div class="table-wrap">
					<table>
						<thead>
							<tr>
								<th>{m.hsb_col_group()}</th>
								<th>{m.hsb_col_mode()}</th>
								<th class="num">{m.hsb_col_questions()}</th>
								<th class="num">R@1</th>
								<th class="num">R@5</th>
								<th class="num">R@10</th>
								<th class="num">{m.hsb_col_mrr()}</th>
							</tr>
						</thead>
						<tbody>
							{#each groupScores as score (score.group + score.value + score.mode)}
								<tr class:hybrid={score.mode === 'hybrid'}>
									<td>{valueLabel(score)}</td>
									<td>{modeLabel(score.mode)}</td>
									<td class="num">{score.count}</td>
									<td class="num">{ratio(score.r1)}</td>
									<td class="num">{ratio(score.r5)}</td>
									<td class="num">{ratio(score.r10)}</td>
									<td class="num">{score.mrr.toFixed(3)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

			<h3>{m.hsb_questions()}</h3>
			<p class="hint">{m.hsb_ranks_hint()}</p>
			<p class="hint">{m.hsb_query_time_hint()}</p>
			{#if results.length === 0}<p class="hint">{m.hsb_no_results()}</p>
			{:else}
				<div class="table-wrap">
					<table class="questions">
						<thead>
							<tr>
								<th>{m.hsb_col_question()}</th>
								<th>{m.hsb_col_target()}</th>
								<th>{m.hsb_col_hop()}</th>
								<th>{m.hsb_col_lang()}</th>
								{#each modes as mode}<th class="num">{modeLabel(mode)}</th>{/each}
								<th class="num">{m.hsb_col_query_time()}</th>
								<th>{m.hsb_col_keywords()}</th>
								<th class="num">{m.hsb_col_search_run()}</th>
							</tr>
						</thead>
						<tbody>
							{#each results as result (result.id)}
								{@const total = queryTime(result)}
								<tr>
									<td class="question"
										>{result.question}{#if result.error_msg}<span class="error row-error"
												>{result.error_msg}</span
											>{/if}</td
									>
									<td class="mono">{result.target_artifact_id}</td>
									<td>{hopLabel(result.hop_type)}</td>
									<td>{langLabel(result.lang)}</td>
									{#each [result.lexical_rank, result.semantic_rank, result.hybrid_rank] as rank}
										<td
											class="num"
											class:hit={rank !== null && rank <= 10}
											class:miss={rank === null}>{rank ?? m.hsb_miss()}</td
										>
									{/each}
									<td class="num" title={total === null ? undefined : queryTimeDetail(result)}
										>{total === null ? '' : ms(total)}</td
									>
									<td class="keywords">{result.keywords.join(', ')}</td>
									<td class="num mono">{result.search_run_id || ''}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>
	{/if}
</div>

<style>
	.hsb {
		color-scheme: light;
		--bg: #f2f4f7;
		--panel: #fff;
		--line: #e4e6eb;
		--text: #111827;
		--muted: #6b7280;
		--accent: #4f46e5;
		--hit: #15803d;
		background: var(--bg);
		color: var(--text);
		padding: 32px;
		min-height: 100%;
		user-select: text;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.hsb.dark {
		color-scheme: dark;
		--bg: #171b26;
		--panel: #1f2333;
		--line: #2d3348;
		--text: #e2e8f0;
		--muted: #94a3b8;
		--accent: #a5b4fc;
		--hit: #4ade80;
	}
	header {
		max-width: 850px;
	}
	h1 {
		font-size: 28px;
		font-weight: 650;
		letter-spacing: -0.6px;
		margin: 4px 0 10px;
	}
	h2 {
		font-size: 16px;
		font-weight: 600;
		margin: 0;
	}
	h3 {
		font-size: 14px;
		font-weight: 600;
		margin: 24px 0 4px;
	}
	header p,
	.hint {
		color: var(--muted);
	}
	p {
		line-height: 1.6;
	}
	.eyebrow {
		font-size: 11px;
		letter-spacing: 1.6px;
		text-transform: uppercase;
	}
	.hint {
		font-size: 12px;
		margin: 8px 0;
	}
	.error {
		color: #ef7373;
	}
	.panel {
		border: 1px solid var(--line);
		border-radius: 12px;
		padding: 24px;
		background: var(--panel);
		min-width: 0;
	}
	.heading {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 16px;
		margin-bottom: 12px;
	}
	.heading.sub {
		margin: 24px 0 0;
	}
	.heading.sub h3 {
		margin: 0;
	}
	.start h2 {
		margin-bottom: 16px;
	}
	.start-fields {
		display: flex;
		gap: 16px;
		align-items: end;
		flex-wrap: wrap;
	}
	.start-fields .grow {
		flex: 1 1 280px;
	}
	label {
		display: block;
		font-size: 13px;
		font-weight: 600;
		margin-bottom: 8px;
	}
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
	.heading select {
		width: auto;
		padding: 6px 10px;
	}
	button {
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 8px 14px;
		font-size: 13px;
		cursor: pointer;
		background: var(--bg);
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	input:focus,
	select:focus,
	button:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.primary {
		background: #4f46e5;
		color: white;
		border-color: #4f46e5;
		padding: 11px 20px;
		font-weight: 600;
	}
	.actions {
		text-align: right;
		white-space: nowrap;
	}
	.actions button {
		padding: 4px 12px;
	}
	.detail {
		scroll-margin-top: 16px;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
	.link {
		border: none;
		background: none;
		padding: 0;
		color: var(--accent);
		font-weight: 600;
	}
	.badge {
		font-size: 11px;
		border: 1px solid var(--line);
		border-radius: 4px;
		padding: 3px 7px;
		color: var(--accent);
		white-space: nowrap;
	}
	.table-wrap {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	th,
	td {
		text-align: left;
		padding: 8px 10px;
		border-bottom: 1px solid var(--line);
		vertical-align: top;
	}
	th {
		font-size: 11px;
		font-weight: 600;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.6px;
		white-space: nowrap;
	}
	.num {
		text-align: right;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
	tr.active td {
		background: color-mix(in srgb, var(--accent) 10%, transparent);
	}
	tr.hybrid td {
		font-weight: 600;
	}
	td.hit {
		color: var(--hit);
		font-weight: 600;
	}
	td.miss {
		color: var(--muted);
	}
	.questions .question {
		min-width: 280px;
		max-width: 520px;
	}
	.row-error {
		display: block;
		font-size: 12px;
		margin-top: 4px;
	}
	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 12px;
		white-space: nowrap;
	}
	.keywords {
		color: var(--muted);
		font-size: 12px;
		min-width: 160px;
	}
	.tests {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		align-items: center;
		margin-top: 16px;
	}
	.options {
		display: flex;
		flex-wrap: wrap;
		gap: 16px;
		align-items: end;
		margin-top: 16px;
	}
	.options > div {
		flex: 0 1 160px;
	}
	label.check {
		display: flex;
		align-items: center;
		gap: 8px;
		margin: 0 8px 10px 0;
		cursor: pointer;
	}
	label.check input {
		width: auto;
	}
	.setting-groups {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 16px 32px;
	}
	h4 {
		font-size: 12px;
		font-weight: 600;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.6px;
		margin: 0 0 8px;
	}
	.settings {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 6px 16px;
		font-size: 13px;
		margin: 0;
	}
	.settings dt {
		color: var(--muted);
	}
	.settings dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	@media (max-width: 800px) {
		.setting-groups {
			grid-template-columns: 1fr;
		}
	}
	@media (max-width: 600px) {
		.hsb {
			padding: 16px;
		}
		.panel {
			padding: 16px;
		}
		.settings {
			grid-template-columns: 1fr;
		}
		.settings dd {
			margin-bottom: 6px;
		}
	}
</style>
