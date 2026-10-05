<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import {
		getPlaygroundOptions,
		getPolicyCurrentVersion,
		parseCriteria,
		runDecision,
		type DecisionPolicy,
		type JevAnswer,
		type PlaygroundModel,
		type PlaygroundQuestion,
		type PlaygroundRunResult,
		type QuestionType
	} from './decision-playground-client';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let models = $state<PlaygroundModel[]>([]);
	let policies = $state<DecisionPolicy[]>([]);
	let policyError = $state('');
	let modelKey = $state('');
	let policyID = $state(0);
	let policyVersion = $state(0);
	let loadedPolicy = $state('');
	let policy = $state('');
	let questionType = $state<QuestionType>('noul');
	let text = $state('');
	let questionText = $state('');
	let criteriaText = $state('');
	let questions = $state<PlaygroundQuestion[]>([]);
	let nextQuestion = 1;
	let ranQuestions = $state<PlaygroundQuestion[]>([]);
	let result = $state<PlaygroundRunResult | null>(null);
	let loading = $state(false);
	let running = $state(false);
	let error = $state('');

	const typeLabels: Record<QuestionType, () => string> = {
		noul: m.dmp_type_noul,
		choice: m.dmp_type_choice,
		score: m.dmp_type_score
	};
	let policyEdited = $derived(policyID > 0 && policy !== loadedPolicy);
	let selectedModel = $derived(models.find((x) => x.key === modelKey));

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const opts = await getPlaygroundOptions();
			models = opts.models ?? [];
			policies = opts.policies ?? [];
			policyError = opts.policy_error ?? '';
			if (!models.some((x) => x.key === modelKey)) modelKey = models[0]?.key ?? '';
		} catch (err) {
			error = message(err);
		} finally {
			loading = false;
		}
	}

	async function changePolicy() {
		error = '';
		if (!policyID) {
			policyVersion = 0;
			loadedPolicy = '';
			policy = '';
			return;
		}
		try {
			const ver = await getPolicyCurrentVersion(policyID);
			policyVersion = ver.version;
			loadedPolicy = ver.content;
			policy = ver.content;
		} catch (err) {
			error = message(err);
		}
	}

	function addQuestion() {
		error = '';
		const instructions = questionText.trim();
		if (!instructions) return;
		const criteria = parseCriteria(questionType, criteriaText);
		const count = Array.isArray(criteria) ? criteria.length : Object.keys(criteria ?? {}).length;
		if (questionType !== 'noul' && count < 2) {
			error = questionType === 'choice' ? m.dmp_error_choice_options() : m.dmp_error_score_levels();
			return;
		}
		questions = [...questions, { id: `q${nextQuestion++}`, type: questionType, instructions, criteria }];
		questionText = '';
		criteriaText = '';
	}

	function removeQuestion(id: string) {
		questions = questions.filter((q) => q.id !== id);
	}

	async function run() {
		running = true;
		error = '';
		result = null;
		const snapshot = $state.snapshot(questions) as PlaygroundQuestion[];
		try {
			result = await runDecision({
				model_key: modelKey,
				policy_id: policyID,
				policy_version: policyVersion,
				policy,
				text,
				questions: snapshot
			});
			ranQuestions = snapshot;
		} catch (err) {
			error = message(err);
		} finally {
			running = false;
		}
	}

	function criteriaSummary(q: PlaygroundQuestion): string {
		if (!q.criteria) return '';
		if (Array.isArray(q.criteria)) return q.criteria.join(' · ');
		return Object.keys(q.criteria).join(' · ');
	}

	// probabilityRows lists an answer's options, most likely first; score
	// levels keep their order and show their legend text.
	function probabilityRows(a: JevAnswer): { label: string; p: number }[] {
		const rows = Object.entries(a.probabilities ?? {}).map(([k, p]) => ({
			label: a.legend?.[k] ? `${k} · ${a.legend[k]}` : k,
			key: k,
			p
		}));
		if (a.type === 'score') rows.sort((x, y) => Number(x.key) - Number(y.key));
		else rows.sort((x, y) => y.p - x.p);
		return rows;
	}

	function pct(p: number | undefined): string {
		return p === undefined ? '' : `${(p * 100).toFixed(1)}%`;
	}

	function message(err: unknown) {
		return String((err as Error)?.message ?? err);
	}
</script>

<section class:dark={darkMode} class="decision-playground">
	<header>
		<p class="eyebrow">{m.dmp_eyebrow()}</p>
		<h2>{m.dmp_title()}</h2>
		<p>{m.dmp_subtitle()}</p>
	</header>
	{#if error}<div class="message error">{error}</div>{/if}

	<section class="card">
		<h3>{m.dmp_run_heading()}</h3>
		<div class="controls">
			<label
				>{m.dmp_model()}<select bind:value={modelKey} disabled={loading}>
					{#each models as model (model.key)}
						<option value={model.key}>{model.key} ({model.provider})</option>
					{/each}
				</select></label
			>
			<label
				>{m.dmp_policy()}<select bind:value={policyID} onchange={changePolicy} disabled={loading}>
					<option value={0}>{m.dmp_no_policy()}</option>
					{#each policies as p (p.id)}
						<option value={p.id}>{m.dmp_policy_option({ name: p.name, version: p.current_version })}</option>
					{/each}
				</select></label
			>
			<label
				>{m.dmp_request_type()}<select bind:value={questionType}>
					{#each Object.keys(typeLabels) as t (t)}
						<option value={t}>{typeLabels[t as QuestionType]()}</option>
					{/each}
				</select></label
			>
		</div>
		{#if !loading && models.length === 0}<p class="muted">{m.dmp_no_models()}</p>{/if}
		{#if policyError}<p class="muted">{m.dmp_policies_unavailable({ reason: policyError })}</p>{/if}
		{#if selectedModel?.provider === 'jev_emulated'}<p class="muted">{m.dmp_emulated_hint()}</p>{/if}

		<label
			><span class="label-row"
				>{m.dmp_policy_text()}{#if policyEdited}<span class="edited">{m.dmp_policy_edited({ version: policyVersion })}</span>{/if}</span
			><textarea bind:value={policy} rows="6" placeholder={m.dmp_policy_placeholder()}></textarea></label
		>
		<label>{m.dmp_text()}<textarea bind:value={text} rows="5" placeholder={m.dmp_text_placeholder()}></textarea></label>

		<div class="question-editor">
			<label
				>{m.dmp_question({ type: typeLabels[questionType]() })}<textarea
					bind:value={questionText}
					rows="2"
					placeholder={m.dmp_question_placeholder()}
				></textarea></label
			>
			{#if questionType !== 'noul'}
				<label
					>{questionType === 'choice' ? m.dmp_choice_options() : m.dmp_score_levels()}<textarea
						bind:value={criteriaText}
						rows="4"
						placeholder={questionType === 'choice' ? m.dmp_choice_placeholder() : m.dmp_score_placeholder()}
					></textarea></label
				>
			{/if}
			<div class="actions">
				<button class="secondary" onclick={addQuestion} disabled={!questionText.trim()}>{m.dmp_add_question()}</button>
			</div>
		</div>

		<h4>{m.dmp_question_list({ count: questions.length })}</h4>
		{#if questions.length === 0}
			<p class="muted">{m.dmp_no_questions()}</p>
		{:else}
			<ol class="question-list">
				{#each questions as q (q.id)}
					<li>
						<div>
							<span class="badge">{q.id} · {typeLabels[q.type]()}</span>
							<p>{q.instructions}</p>
							{#if q.criteria}<p class="muted">{criteriaSummary(q)}</p>{/if}
						</div>
						<button class="secondary" onclick={() => removeQuestion(q.id)}>{m.dmp_remove()}</button>
					</li>
				{/each}
			</ol>
		{/if}

		<div class="actions run-actions">
			<button onclick={run} disabled={running || !modelKey || questions.length === 0}
				>{running ? m.dmp_running() : m.dmp_run()}</button
			>
		</div>
	</section>

	<section class="card">
		<h3>{m.dmp_results_heading()}</h3>
		{#if running}
			<p class="muted">{m.dmp_running()}</p>
		{:else if !result}
			<p class="muted">{m.dmp_no_results()}</p>
		{:else}
			<p class="muted">
				{m.dmp_result_meta({
					model: result.model_key,
					provider: result.provider,
					ms: result.elapsed_ms,
					input: result.usage?.input_tokens ?? 0,
					output: result.usage?.output_tokens ?? 0
				})}
			</p>
			<div class="answers">
				{#each ranQuestions as q (q.id)}
					{@const a = result.answers[q.id]}
					<article>
						<span class="badge">{q.id} · {typeLabels[q.type]()}</span>
						<p>{q.instructions}</p>
						{#if !a}
							<p class="muted">{m.dmp_no_answer()}</p>
						{:else if a.type === 'noul'}
							<p class="verdict">{m.dmp_answer_noul({ p: pct(a.noul) })}</p>
							<div class="bar"><span style:width={pct(a.noul)}></span></div>
						{:else}
							<p class="verdict">
								{a.type === 'choice'
									? m.dmp_answer_choice({ choice: a.choice ?? '' })
									: m.dmp_answer_score({ score: (a.score ?? 0).toFixed(2) })}
								<span class="muted">{m.dmp_confidence({ p: pct(a.confidence) })}</span>
							</p>
							<table>
								<tbody>
									{#each probabilityRows(a) as row (row.label)}
										<tr>
											<td>{row.label}</td>
											<td class="bar-cell"><div class="bar"><span style:width={pct(row.p)}></span></div></td>
											<td class="num">{pct(row.p)}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/if}
					</article>
				{/each}
			</div>
			<details>
				<summary>{m.dmp_raw_response()}</summary>
				<pre>{JSON.stringify(result.raw, null, 2)}</pre>
			</details>
		{/if}
	</section>
</section>

<style>
	.decision-playground {
		display: grid;
		gap: 1rem;
		color: #1f2937;
		padding: 1.25rem;
		user-select: text;
		-webkit-user-select: text;
	}
	.decision-playground * {
		user-select: text;
		-webkit-user-select: text;
	}
	.decision-playground.dark {
		color: #e5e7eb;
	}
	header h2 {
		margin: 0.15rem 0;
		font-size: 1.5rem;
	}
	header p,
	.muted {
		color: #7b8493;
		margin: 0.25rem 0;
	}
	.eyebrow {
		color: #7085df;
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}
	.card {
		background: color-mix(in srgb, currentColor 4%, transparent);
		border: 1px solid color-mix(in srgb, currentColor 14%, transparent);
		border-radius: 0.7rem;
		padding: 1rem;
	}
	.card h3 {
		margin: 0 0 0.8rem;
	}
	.card h4 {
		margin: 1rem 0 0.4rem;
	}
	.controls,
	.actions {
		display: flex;
		align-items: end;
		gap: 0.7rem;
		flex-wrap: wrap;
	}
	.controls label {
		flex: 1 1 14rem;
	}
	label {
		display: grid;
		gap: 0.35rem;
		font-size: 0.88rem;
	}
	.card > label,
	.question-editor {
		margin-top: 0.8rem;
	}
	.question-editor {
		display: grid;
		gap: 0.6rem;
		border-top: 1px solid color-mix(in srgb, currentColor 14%, transparent);
		padding-top: 0.8rem;
	}
	.label-row {
		display: flex;
		gap: 0.6rem;
		align-items: baseline;
	}
	.edited {
		color: #d29a3a;
		font-size: 0.8rem;
	}
	select,
	textarea {
		color: inherit;
		background: color-mix(in srgb, currentColor 5%, transparent);
		border: 1px solid color-mix(in srgb, currentColor 22%, transparent);
		border-radius: 0.35rem;
		padding: 0.55rem;
		font: inherit;
	}
	textarea {
		width: 100%;
		resize: vertical;
		box-sizing: border-box;
	}
	button {
		border: 0;
		border-radius: 0.35rem;
		padding: 0.55rem 0.8rem;
		color: white;
		background: #536ce0;
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	button.secondary {
		color: inherit;
		background: color-mix(in srgb, currentColor 13%, transparent);
	}
	.run-actions {
		margin-top: 1rem;
	}
	.message {
		padding: 0.65rem 0.8rem;
		border-radius: 0.4rem;
		white-space: pre-wrap;
	}
	.error {
		color: #ef8585;
		background: #b8464622;
	}
	.question-list {
		margin: 0;
		padding: 0;
		list-style: none;
		display: grid;
		gap: 0.4rem;
	}
	.question-list li {
		display: flex;
		justify-content: space-between;
		align-items: start;
		gap: 0.8rem;
		padding: 0.55rem 0.7rem;
		border: 1px solid color-mix(in srgb, currentColor 12%, transparent);
		border-radius: 0.45rem;
	}
	.question-list p,
	.answers p {
		margin: 0.25rem 0;
		white-space: pre-wrap;
	}
	.badge {
		font-size: 0.75rem;
		color: #7085df;
		font-weight: 600;
	}
	.answers {
		display: grid;
		gap: 0.6rem;
		margin: 0.6rem 0;
	}
	.answers article {
		border-top: 1px solid color-mix(in srgb, currentColor 13%, transparent);
		padding-top: 0.6rem;
	}
	.verdict {
		font-weight: 600;
	}
	.verdict .muted {
		font-weight: normal;
		margin-left: 0.6rem;
	}
	.bar {
		height: 0.5rem;
		border-radius: 0.25rem;
		background: color-mix(in srgb, currentColor 10%, transparent);
		overflow: hidden;
		max-width: 28rem;
	}
	.bar span {
		display: block;
		height: 100%;
		background: #536ce0;
	}
	table {
		border-collapse: collapse;
		width: 100%;
		max-width: 44rem;
	}
	td {
		padding: 0.25rem 0.45rem;
		font-size: 0.86rem;
		vertical-align: middle;
	}
	.bar-cell {
		width: 50%;
	}
	.num {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	pre {
		overflow-x: auto;
		font-size: 0.8rem;
		background: color-mix(in srgb, currentColor 5%, transparent);
		padding: 0.6rem;
		border-radius: 0.35rem;
	}
</style>
