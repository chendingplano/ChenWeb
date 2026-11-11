<script lang="ts">
	// Content panel of the Analyze tab: one benchmark question's log in search order — the
	// question's own search, then (Medium/High) the effort analysis and each attempt with its
	// queries and the judge's verdict. Each query shows its four candidate lists (first rows plus
	// the target's row). Every metric ID is a button that selects the metric.
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import type {
		HybridBenchmarkResult,
		TraceCandidate,
		TraceQuery,
		TraceStage
	} from '$lib/services/hybridSearchBenchmarkService';

	let {
		result,
		queries,
		limit,
		backend = '',
		selectedMetricId = null,
		onSelectMetric
	}: {
		result: HybridBenchmarkResult;
		queries: TraceQuery[];
		limit: number;
		backend?: string;
		selectedMetricId?: string | null;
		onSelectMetric: (metricId: string) => void;
	} = $props();

	let effort = $derived(result.effort_trace);
	let questionQueries = $derived(queries.filter((q) => q.source === 'question'));
	let fallbackQueries = $derived(
		queries.filter((q) => q.source === 'effort' && q.attempt === null)
	);
	const attemptQueries = (attempt: number) =>
		queries.filter((q) => q.source === 'effort' && q.attempt === attempt);

	const hopLabel = (hop: string) =>
		hop === 'one_hop' ? m.qg_one_hop() : hop === 'two_hop' ? m.qg_two_hop() : m.qg_multi_hop();
	const langLabel = (lang: string) => (lang === 'en' ? m.qg_english() : m.qg_chinese());
	const rankText = (rank: number | null) => (rank === null ? m.hsb_miss() : String(rank));
	const fixed = (v: number, digits: number) =>
		v.toLocaleString(getLocale(), { minimumFractionDigits: digits, maximumFractionDigits: digits });
	const queryTitle = (q: TraceQuery) =>
		q.source === 'question'
			? m.hsba_query_question({ seq: q.seq })
			: q.attempt === null
				? m.hsba_query_fallback({ seq: q.seq })
				: m.hsba_query_effort({ seq: q.seq, attempt: q.attempt });
	const isTarget = (c: TraceCandidate) =>
		c.artifact_id === result.target_artifact_id && c.artifact_type === result.target_artifact_type;
	// The effort judge names candidates as "<type>/<id>".
	const metricIdOf = (key: string) => key.slice(key.indexOf('/') + 1);
	const typeOf = (key: string) => key.slice(0, key.indexOf('/'));
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

	// The four lists start two per row, each half the width of their area, and keep that width when
	// the panel is resized; once two no longer fit side by side they stack and fill the width.
	// (A list the user drags keeps its own size: the resize grip sets inline width and height.)
	const LIST_GAP = 18; // matches the .lists column gap
	const LIST_MIN_WIDTH = 220;
	function listsLayout(node: HTMLElement) {
		let listWidth = 0;
		const observer = new ResizeObserver(([entry]) => {
			const width = entry.contentRect.width;
			if (width <= 0) return; // a collapsed query: measure once it is opened
			if (!listWidth) {
				listWidth = Math.max(LIST_MIN_WIDTH, Math.floor((width - LIST_GAP) / 2));
				node.style.setProperty('--list-w', `${listWidth}px`);
			}
			node.classList.toggle('stacked', width < 2 * listWidth + LIST_GAP);
		});
		observer.observe(node);
		return { destroy: () => observer.disconnect() };
	}
</script>

<!-- Only metrics have a details panel; other artifact types show their ID as plain text. -->
{#snippet metricButton(id: string, label = '', type = 'metric')}
	{#if type === 'metric'}
		<button
			type="button"
			class="metric-link"
			class:selected={id === selectedMetricId}
			onclick={() => onSelectMetric(id)}
			><span class="mono">{id}</span>{#if id === result.target_artifact_id}<span class="marker"
					>{m.hsba_target_marker()}</span
				>{/if}</button
		>
	{:else}
		<span class="mono">{id}</span>{#if id === result.target_artifact_id && type === result.target_artifact_type}<span
				class="marker">{m.hsba_target_marker()}</span
			>{/if}
	{/if}{#if label}<span class="label">{label}</span>{/if}
{/snippet}

{#snippet gap(cols: number)}
	<tr class="gap"><td colspan={cols}>{m.hsba_gap()}</td></tr>
{/snippet}

{#snippet listHeading(title: string, rows: TraceCandidate[], count: number | undefined)}
	<div class="list-head">
		<h4>{title} <span class="count">{m.hsba_count({ count: count ?? 0 })}</span></h4>
		{#if rows.length > 0 && !rows.some(isTarget)}<p class="hint">{m.hsba_target_absent()}</p>{/if}
	</div>
{/snippet}

{#snippet queryBlock(q: TraceQuery, open: boolean)}
	<details class="query" {open}>
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
		<div class="lists" use:listsLayout>
			{#if partitionGroups(q).length > 0}
				<section>
					{@render listHeading(
						m.hsba_stage_lexical_partition(),
						stageRows(q, 'lexical_partition'),
						q.counts.lexical_partition
					)}
					<div class="list-body">
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
										<tr
											class:target={isTarget(c)}
											class:picked={c.artifact_id === selectedMetricId}
										>
											<td class="num">{c.rank}</td>
											<td class="metric">{@render metricButton(c.artifact_id, c.label, c.artifact_type)}</td>
											<td class="num">{fixed(c.score, 2)}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/each}
					</div>
				</section>
			{/if}
			{#each [stageRows(q, 'lexical')] as rows}
				<section>
					{@render listHeading(m.hsba_stage_lexical(), rows, q.counts.lexical)}
					<div class="list-body">
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
									<tr class:target={isTarget(c)} class:picked={c.artifact_id === selectedMetricId}>
										<td class="num">{c.rank}</td>
										<td class="num">{c.partition_rank ?? ''}</td>
										<td class="mono">{c.partition_name}</td>
										<td class="metric">{@render metricButton(c.artifact_id, c.label, c.artifact_type)}</td>
										<td class="num">{fixed(c.score, 2)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</section>
			{/each}
			{#each [stageRows(q, 'semantic')] as rows}
				<section>
					{@render listHeading(m.hsba_stage_semantic(), rows, q.counts.semantic)}
					<div class="list-body">
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
									<tr class:target={isTarget(c)} class:picked={c.artifact_id === selectedMetricId}>
										<td class="num">{c.rank}</td>
										<td class="metric">{@render metricButton(c.artifact_id, c.label, c.artifact_type)}</td>
										<td class="num">{fixed(c.score, 3)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</section>
			{/each}
			{#each [stageRows(q, 'fused')] as rows}
				<section>
					{@render listHeading(m.hsba_stage_fused(), rows, q.counts.fused)}
					<div class="list-body">
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
									<tr class:target={isTarget(c)} class:picked={c.artifact_id === selectedMetricId}>
										<td class="num">{c.rank}</td>
										<td class="metric">{@render metricButton(c.artifact_id, c.label, c.artifact_type)}</td>
										<td class="num">{fixed(c.score, 4)}</td>
										<td class="num">{c.lexical_rank ?? ''}</td>
										<td class="num">{c.semantic_rank ?? ''}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</section>
			{/each}
		</div>
	</details>
{/snippet}

<div class="log">
	<header>
		<p class="eyebrow">{m.hsba_title()}</p>
		<h2>{result.question}</h2>
		<dl>
			<dt>{m.hsb_col_target()}</dt>
			<dd>{@render metricButton(result.target_artifact_id, '', result.target_artifact_type)}</dd>
			<dt>{m.hsb_col_hop()}</dt>
			<dd>{hopLabel(result.hop_type)}</dd>
			<dt>{m.hsb_col_lang()}</dt>
			<dd>{langLabel(result.lang)}</dd>
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
			{#if result.search_run_id}
				<dt>{m.hsb_col_search_run()}</dt>
				<dd class="mono">{result.search_run_id}</dd>
			{/if}
		</dl>
		{#if result.error_msg}<p class="error">{result.error_msg}</p>{/if}
	</header>

	{#if queries.length === 0}<p class="hint">{m.hsba_no_trace()}</p>
	{:else}
		<p class="hint">{m.hsba_shown_hint({ limit })} {m.hsba_click_hint()}</p>
		{#if backend === 'postgres'}<p class="hint">{m.hsba_postgres_hint()}</p>{/if}

		<h3>{m.hsba_log_question()}</h3>
		{#each questionQueries as q (q.id)}{@render queryBlock(q, true)}{/each}

		{#if effort}
			<h3>{m.hsba_log_effort()}</h3>
			<p>
				{m.hsba_log_strategy({ strategy: effort.strategy || '—' })}{#if effort.need}<br
					/>{m.hsb_effort_need({ need: effort.need })}{/if}
			</p>
			{#if effort.error}<p class="error">{effort.error}</p>{/if}
			{#each effort.attempts as attempt, i}
				<h3>{m.hsba_log_attempt({ attempt: i + 1, max: effort.max_attempts })}</h3>
				{#each attemptQueries(i + 1) as q (q.id)}{@render queryBlock(q, false)}{/each}
				<div class="verdict" class:found={attempt.found}>
					<strong>{attempt.found ? m.hsba_judge_found() : m.hsba_judge_not_found()}</strong>
					{#if attempt.reason}<span>{attempt.reason}</span>{/if}
					{#if attempt.matching_ids.length > 0}
						<span class="matches"
							>{m.hsba_judge_matches()}
							{#each attempt.matching_ids as key}{@render metricButton(
									metricIdOf(key),
									'',
									typeOf(key)
								)}{/each}</span
						>
					{/if}
					{#if attempt.error}<span class="error">{attempt.error}</span>{/if}
				</div>
			{/each}
			{#if fallbackQueries.length > 0}
				<h3>{m.hsba_log_fallback()}</h3>
				{#each fallbackQueries as q (q.id)}{@render queryBlock(q, false)}{/each}
			{/if}
			<p class="final">{m.hsba_log_effort_rank({ rank: rankText(result.effort_rank) })}</p>
		{/if}
	{/if}
</div>

<style>
	.log {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 13px;
	}
	header {
		padding-bottom: 12px;
		border-bottom: 1px solid var(--line);
	}
	.eyebrow {
		margin: 0;
		font-size: 11px;
		font-weight: 600;
		color: var(--accent);
		text-transform: uppercase;
		letter-spacing: 0.6px;
	}
	h2 {
		margin: 4px 0 8px;
		font-size: 15px;
		font-weight: 600;
	}
	h3 {
		margin: 16px 0 4px;
		font-size: 13px;
		font-weight: 650;
	}
	dl {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 4px 8px;
		margin: 0;
		font-size: 12px;
	}
	dt {
		color: var(--muted);
	}
	dd {
		margin: 0 12px 0 0;
		font-weight: 600;
	}
	p {
		margin: 4px 0;
	}
	.query {
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 8px 10px;
		margin-top: 8px;
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
	/* Lists wrap; a list the user resizes keeps its size and the others flow around it. */
	.lists {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		gap: 14px 18px;
		margin-top: 8px;
	}
	/* Each list: a rounded frame with a header bar (title, count) over its entries, a fixed
	   height with its own scroll bars, and the browser's resize grip (bottom-right corner) to drag
	   its width and height. */
	section {
		flex: 0 0 auto;
		box-sizing: border-box;
		width: var(--list-w, calc(50% - 9px));
		max-width: 100%;
		min-width: min(100%, 220px);
		/* Two rows of lists fill one panel height (--list-h is set by the Analyze view). */
		height: var(--list-h, 340px);
		min-height: 120px;
		display: flex;
		flex-direction: column;
		border: 1px solid var(--list-line);
		border-radius: 10px;
		overflow: hidden;
		resize: both;
		background: var(--list-body);
	}
	.lists:global(.stacked) > section {
		width: 100%;
	}
	.list-head {
		flex: 0 0 auto;
		padding: 7px 10px;
		background: var(--list-head);
		border-bottom: 1px solid var(--list-line);
	}
	.list-head .hint {
		margin: 2px 0 0;
	}
	.list-body {
		flex: 1 1 auto;
		min-height: 0;
		overflow: auto;
		padding: 0 4px 10px;
	}
	/* Column headings stay visible while the entries scroll. */
	.list-body thead th {
		position: sticky;
		top: 0;
		z-index: 1;
		background: var(--list-body);
	}
	h4 {
		margin: 0;
		font-size: 12px;
	}
	.count {
		font-weight: 400;
		color: var(--muted);
	}
	.partition {
		margin: 6px 6px 2px;
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
	.metric-link {
		border: none;
		background: none;
		padding: 0;
		color: var(--accent);
		font: inherit;
		cursor: pointer;
		text-align: left;
	}
	.metric-link:hover .mono,
	.metric-link.selected .mono {
		text-decoration: underline;
	}
	.metric-link:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.label {
		display: block;
		color: var(--muted);
		font-size: 11px;
	}
	.marker {
		margin-left: 6px;
		font-size: 10px;
		font-weight: 600;
		text-transform: uppercase;
	}
	tr.target td {
		background: color-mix(in srgb, var(--accent) 12%, transparent);
		font-weight: 600;
	}
	/* The clicked metric: a solid, lighter background (after tr.target, so it wins on the target). */
	tr.picked td {
		background: var(--picked);
	}
	tr.picked .label {
		color: inherit;
	}
	tr.gap td {
		color: var(--muted);
		text-align: center;
		font-size: 11px;
	}
	.verdict {
		display: flex;
		flex-wrap: wrap;
		gap: 4px 10px;
		align-items: baseline;
		margin-top: 8px;
		padding: 8px 10px;
		border-left: 3px solid var(--muted);
		background: color-mix(in srgb, var(--muted) 8%, transparent);
		font-size: 12px;
	}
	.verdict.found {
		border-left-color: var(--hit);
	}
	.matches {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 4px 8px;
		align-items: baseline;
	}
	.final {
		margin-top: 12px;
		font-weight: 600;
	}
	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
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
</style>
