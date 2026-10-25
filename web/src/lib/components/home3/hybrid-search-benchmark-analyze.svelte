<script lang="ts">
	// Analyze dialog: the trace of one benchmark question — every query it searched and, for each,
	// the candidate lists of its hybrid search (BM25 per partition, BM25 merged, semantic, fused),
	// each showing its first rows plus the target's row.
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		getBenchmarkTrace,
		type HybridBenchmarkResult,
		type TraceCandidate,
		type TraceQuery,
		type TraceStage
	} from '$lib/services/hybridSearchBenchmarkService';

	let { backend = '' }: { backend?: string } = $props();

	let dialog: HTMLDialogElement;
	let controller: AbortController | undefined;
	let result = $state<HybridBenchmarkResult | null>(null);
	let queries = $state<TraceQuery[]>([]);
	let limit = $state(0);
	let loading = $state(false);
	let loadError = $state(false);

	export function open(r: HybridBenchmarkResult) {
		result = r;
		queries = [];
		dialog.showModal();
		void load();
	}

	async function load() {
		if (!result) return;
		controller?.abort();
		const current = new AbortController();
		controller = current;
		loading = true;
		loadError = false;
		try {
			const trace = await getBenchmarkTrace(result.id, current.signal);
			if (current.signal.aborted) return;
			queries = trace.queries ?? [];
			limit = trace.limit;
		} catch {
			if (!current.signal.aborted) loadError = true;
		} finally {
			if (!current.signal.aborted) loading = false;
		}
	}

	const rankText = (rank: number | null) => (rank === null ? m.hsb_miss() : String(rank));
	const fixed = (v: number, digits: number) =>
		v.toLocaleString(getLocale(), { minimumFractionDigits: digits, maximumFractionDigits: digits });
	const queryTitle = (q: TraceQuery) =>
		q.source === 'question'
			? m.hsba_query_question({ seq: q.seq })
			: q.attempt === null
				? m.hsba_query_fallback({ seq: q.seq })
				: m.hsba_query_effort({ seq: q.seq, attempt: q.attempt });
	const isTarget = (c: TraceCandidate) => c.artifact_id === result?.target_artifact_id;
	// Rows are the first `limit` ranks plus the target; a jump in rank means rows were left out.
	const gapBefore = (rows: TraceCandidate[], i: number) =>
		i === 0 ? rows[0].rank !== 1 : rows[i].rank !== rows[i - 1].rank + 1;
	const stageRows = (q: TraceQuery, stage: TraceStage) => q.stages[stage] ?? [];
	// lexical_partition rows grouped by partition, each group ranked within its partition.
	const partitionGroups = (q: TraceQuery) => {
		const groups = new Map<string, TraceCandidate[]>();
		for (const c of stageRows(q, 'lexical_partition')) {
			groups.set(c.partition_name, [...(groups.get(c.partition_name) ?? []), c]);
		}
		return [...groups.entries()];
	};
</script>

{#snippet metric(c: TraceCandidate)}
	<td class="metric"
		><span class="mono">{c.artifact_id}</span>{#if isTarget(c)}<span class="marker"
				>{m.hsba_target_marker()}</span
			>{/if}{#if c.label}<span class="label">{c.label}</span>{/if}</td
	>
{/snippet}

{#snippet gap(cols: number)}
	<tr class="gap"><td colspan={cols}>{m.hsba_gap()}</td></tr>
{/snippet}

{#snippet listHeading(title: string, rows: TraceCandidate[], count: number | undefined)}
	<h4>{title} <span class="count">{m.hsba_count({ count: count ?? 0 })}</span></h4>
	{#if rows.length > 0 && !rows.some(isTarget)}<p class="hint">{m.hsba_target_absent()}</p>{/if}
{/snippet}

<dialog
	bind:this={dialog}
	aria-labelledby="hsba-title"
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
		<h2 id="hsba-title">{m.hsba_title()}</h2>
		<button type="button" onclick={() => dialog.close()}>{m.qg_close()}</button>
	</div>

	{#if result}
		<div class="summary">
			<p class="question">{result.question}</p>
			<dl>
				<dt>{m.hsb_col_target()}</dt>
				<dd class="mono">{result.target_artifact_id}</dd>
				<dt>{m.hsb_mode_lexical()}</dt>
				<dd>{rankText(result.lexical_rank)}</dd>
				<dt>{m.hsb_mode_semantic()}</dt>
				<dd>{rankText(result.semantic_rank)}</dd>
				<dt>{m.hsb_mode_hybrid()}</dt>
				<dd>{rankText(result.hybrid_rank)}</dd>
				{#if result.effort_attempts !== null}
					<dt>{m.hsb_mode_effort()}</dt>
					<dd>{rankText(result.effort_rank)}</dd>
				{/if}
			</dl>
		</div>
	{/if}

	{#if loading}<p class="hint" role="status">{m.hsba_loading()}</p>
	{:else if loadError}
		<p class="error" role="alert">
			{m.hsba_load_error()}
			<button type="button" onclick={() => void load()}>{m.qg_retry()}</button>
		</p>
	{:else if queries.length === 0}<p class="hint">{m.hsba_no_trace()}</p>
	{:else}
		<p class="hint">{m.hsba_shown_hint({ limit })}</p>
		{#if backend === 'postgres'}<p class="hint">{m.hsba_postgres_hint()}</p>{/if}
		{#each queries as q, qi (q.id)}
			<details class="query" open={qi === 0}>
				<summary>
					<span class="query-title">{queryTitle(q)}</span>
					<span class="query-text">{q.query_text}</span>
				</summary>
				<p class="hint">
					{q.keywords.length > 0
						? m.hsba_keywords({ keywords: q.keywords.join(', ') })
						: m.hsba_no_keywords()}
				</p>
				{#if q.error_msg}<p class="error">{q.error_msg}</p>{/if}
				<div class="lists">
					{#if partitionGroups(q).length > 0}
						<section>
							{@render listHeading(
								m.hsba_stage_lexical_partition(),
								stageRows(q, 'lexical_partition'),
								q.counts.lexical_partition
							)}
							{#each partitionGroups(q) as [partition, rows]}
								<p class="partition mono">{partition}</p>
								<table>
									<thead>
										<tr>
											<th class="num">{m.hsba_col_partition_rank()}</th>
											<th>{m.hsba_col_metric()}</th>
											<th class="num">{m.hsba_col_bm25()}</th>
										</tr>
									</thead>
									<tbody>
										{#each rows as c, i}
											{#if gapBefore(rows, i)}{@render gap(3)}{/if}
											<tr class:target={isTarget(c)}>
												<td class="num">{c.rank}</td>
												{@render metric(c)}
												<td class="num">{fixed(c.score, 2)}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							{/each}
						</section>
					{/if}
					{#each [stageRows(q, 'lexical')] as rows}
						<section>
							{@render listHeading(m.hsba_stage_lexical(), rows, q.counts.lexical)}
							<table>
								<thead>
									<tr>
										<th class="num">{m.hsba_col_merged_rank()}</th>
										<th class="num">{m.hsba_col_partition_rank()}</th>
										<th>{m.hsba_col_partition()}</th>
										<th>{m.hsba_col_metric()}</th>
										<th class="num">{m.hsba_col_bm25()}</th>
									</tr>
								</thead>
								<tbody>
									{#each rows as c, i}
										{#if gapBefore(rows, i)}{@render gap(5)}{/if}
										<tr class:target={isTarget(c)}>
											<td class="num">{c.rank}</td>
											<td class="num">{c.partition_rank ?? ''}</td>
											<td class="mono">{c.partition_name}</td>
											{@render metric(c)}
											<td class="num">{fixed(c.score, 2)}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</section>
					{/each}
					{#each [stageRows(q, 'semantic')] as rows}
						<section>
							{@render listHeading(m.hsba_stage_semantic(), rows, q.counts.semantic)}
							<table>
								<thead>
									<tr>
										<th class="num">{m.hsba_col_rank()}</th>
										<th>{m.hsba_col_metric()}</th>
										<th class="num">{m.hsba_col_similarity()}</th>
									</tr>
								</thead>
								<tbody>
									{#each rows as c, i}
										{#if gapBefore(rows, i)}{@render gap(3)}{/if}
										<tr class:target={isTarget(c)}>
											<td class="num">{c.rank}</td>
											{@render metric(c)}
											<td class="num">{fixed(c.score, 3)}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</section>
					{/each}
					{#each [stageRows(q, 'fused')] as rows}
						<section>
							{@render listHeading(m.hsba_stage_fused(), rows, q.counts.fused)}
							<table>
								<thead>
									<tr>
										<th class="num">{m.hsba_col_rank()}</th>
										<th>{m.hsba_col_metric()}</th>
										<th class="num">{m.hsba_col_rrf()}</th>
										<th class="num">{m.hsba_col_lexical_rank()}</th>
										<th class="num">{m.hsba_col_semantic_rank()}</th>
									</tr>
								</thead>
								<tbody>
									{#each rows as c, i}
										{#if gapBefore(rows, i)}{@render gap(5)}{/if}
										<tr class:target={isTarget(c)}>
											<td class="num">{c.rank}</td>
											{@render metric(c)}
											<td class="num">{fixed(c.score, 4)}</td>
											<td class="num">{c.lexical_rank ?? ''}</td>
											<td class="num">{c.semantic_rank ?? ''}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</section>
					{/each}
				</div>
			</details>
		{/each}
	{/if}
</dialog>

<style>
	dialog {
		color: var(--text);
		background: var(--panel);
		border: 1px solid var(--line);
		border-radius: 12px;
		padding: 24px;
		width: min(1400px, calc(100vw - 32px));
		max-height: calc(100dvh - 48px);
		overflow: auto;
		margin: auto;
		user-select: text;
	}
	dialog::backdrop {
		background: rgb(0 0 0 / 0.65);
	}
	.dialog-heading {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 16px;
	}
	.dialog-heading h2 {
		margin: 0;
		font-size: 16px;
	}
	.summary {
		margin: 12px 0;
		padding-bottom: 12px;
		border-bottom: 1px solid var(--line);
	}
	.summary .question {
		margin: 0 0 8px;
		font-size: 14px;
		font-weight: 600;
	}
	.summary dl {
		display: flex;
		flex-wrap: wrap;
		gap: 4px 8px;
		margin: 0;
		font-size: 12px;
	}
	.summary dt {
		color: var(--muted);
	}
	.summary dd {
		margin: 0 12px 0 0;
		font-weight: 600;
	}
	.query {
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 10px 12px;
		margin-top: 12px;
	}
	.query summary {
		cursor: pointer;
		display: flex;
		flex-wrap: wrap;
		gap: 4px 10px;
		align-items: baseline;
	}
	.query-title {
		font-size: 12px;
		font-weight: 600;
		color: var(--accent);
		white-space: nowrap;
	}
	.query-text {
		font-size: 13px;
	}
	.lists {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 420px), 1fr));
		gap: 16px 20px;
		margin-top: 8px;
	}
	section {
		min-width: 0;
		overflow-x: auto;
	}
	h4 {
		margin: 0 0 6px;
		font-size: 13px;
	}
	.count {
		font-weight: 400;
		color: var(--muted);
		font-size: 12px;
	}
	.partition {
		margin: 4px 0;
		font-size: 11px;
		color: var(--muted);
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 12px;
	}
	th,
	td {
		text-align: left;
		padding: 4px 6px;
		border-bottom: 1px solid var(--line);
		vertical-align: top;
	}
	th {
		font-size: 10px;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.5px;
		white-space: nowrap;
	}
	.num {
		text-align: right;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
	.metric .label {
		display: block;
		color: var(--muted);
		font-size: 11px;
	}
	.marker {
		margin-left: 6px;
		font-size: 10px;
		font-weight: 600;
		color: var(--accent);
		text-transform: uppercase;
	}
	tr.target td {
		background: color-mix(in srgb, var(--accent) 14%, transparent);
		font-weight: 600;
	}
	tr.gap td {
		color: var(--muted);
		text-align: center;
		font-size: 11px;
	}
	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		white-space: nowrap;
	}
	.hint {
		font-size: 12px;
		color: var(--muted);
		margin: 6px 0;
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
	button:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	@media (max-width: 600px) {
		dialog {
			padding: 16px;
		}
	}
</style>
