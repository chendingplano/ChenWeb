<script lang="ts">
	// Metric Info card: id / confidence / metric id, name, description, table
	// context rows and value / unit / span count of one kb.metrics row. Shown
	// floating over the PDF viewer (PdfViewWindow `floatingOverlay` snippets).
	import { m as i18n } from '$lib/paraglide/messages.js';
	import type { KbMetricRecord } from '$lib/services/kbService';
	import { clipTableCell, splitTableContextRows } from './metric-table-context';

	let { metric, light = false }: { metric: KbMetricRecord; light?: boolean } = $props();

	let spans = $derived.by(() => {
		const raw = (metric as { source_line_spans?: unknown })?.source_line_spans;
		if (!Array.isArray(raw)) return 0;
		let count = 0;
		for (const item of raw) {
			if (typeof item === 'string') {
				const mm = item.trim().match(/^(\d+)\s*[:,-]\s*(\d+)$/);
				count += mm ? Math.max(0, parseInt(mm[2], 10) - parseInt(mm[1], 10) + 1) : 1;
			} else {
				count += 1;
			}
		}
		return count;
	});
</script>

<div class="metric-floating-card" class:light={light}>
	<div class="metric-floating-kicker">
		<span class="metric-floating-id">#{metric.id}</span>
		{#if metric.confidence != null}
			<span class="metric-floating-confidence">{Math.round(Number(metric.confidence) * 100)}%</span>
		{/if}
		{#if metric.metric_id}
			<span class="metric-floating-metric-id">{metric.metric_id}</span>
		{/if}
	</div>
	<div class="metric-floating-name">
		{metric.metric_name || metric.metric_subject || i18n.metric_mgmt_metric({ id: metric.id })}
	</div>
	{#if metric.metric_desc}<div class="metric-floating-desc">{metric.metric_desc}</div>{/if}
	{#each metric.table_context ?? [] as win (win.line)}
		{@const parts = splitTableContextRows(win)}
		<div class="metric-floating-table-wrap">
			{#if win.caption}<div class="metric-floating-table-caption">{win.caption}</div>{/if}
			<table class="metric-floating-table">
				<thead>
					{#each parts.head as row (row.id)}
						<tr>{#each row.cells as cell, ci (ci)}<th>{cell}</th>{/each}</tr>
					{/each}
				</thead>
				<tbody>
					{#each parts.body as row (row.id)}
						<tr class:matched={row.matched}>
							{#if row.full_width}
								<td colspan={win.columns.length} title={row.cells[0]}>{clipTableCell(row.cells[0])}</td>
							{:else}
								{#each row.cells as cell, ci (ci)}<td title={cell}>{clipTableCell(cell)}</td>{/each}
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/each}
	<div class="metric-floating-meta">
		{#if metric.metric_value}<span>{metric.metric_value}</span>{/if}
		{#if metric.metric_unit}<span>{metric.metric_unit}</span>{/if}
		{#if metric.location_type}<span>{metric.location_type}</span>{/if}
		<span>{i18n.metric_mgmt_span_2({ metric: spans, plural: spans === 1 ? '' : 's' })}</span>
	</div>
</div>

<style>
.metric-floating-card {
	display: flex;
	flex-direction: column;
	gap: 8px;
	padding: 12px 14px;
	border-radius: 14px;
	border: 1px solid rgba(148, 163, 184, 0.28);
	background:
		linear-gradient(180deg, rgba(20, 27, 39, 0.96), rgba(10, 14, 22, 0.94));
	box-shadow:
		0 18px 40px rgba(2, 6, 23, 0.42),
		inset 0 1px 0 rgba(255, 255, 255, 0.06);
	color: var(--text-primary);
	backdrop-filter: blur(10px);
}
.metric-floating-card.light {
	border-color: var(--ink-line);
	background: var(--panel-bg);
	box-shadow: 0 18px 40px rgba(26, 20, 16, 0.2), inset 0 1px 0 rgba(255, 255, 255, 0.8);
	color: var(--text-primary);
}
.metric-floating-kicker,
.metric-floating-meta {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: 8px;
	font-family: var(--font-mono);
	font-size: 11px;
	letter-spacing: 0.06em;
	text-transform: uppercase;
	color: var(--text-secondary);
}
.metric-floating-id,
.metric-floating-confidence,
.metric-floating-metric-id,
.metric-floating-meta span {
	padding: 3px 7px;
	border-radius: 999px;
	background: rgba(148, 163, 184, 0.12);
	border: 1px solid rgba(148, 163, 184, 0.18);
}
.metric-floating-confidence {
	color: var(--mint);
}
.metric-floating-name {
	font-family: var(--font-serif);
	font-size: 19px;
	line-height: 1.2;
	color: var(--paper);
}
.metric-floating-card.light .metric-floating-name { color: var(--text-primary); }
.metric-floating-desc {
	font-size: 12px;
	line-height: 1.5;
	color: var(--text-secondary);
	max-width: 42ch;
}
.metric-floating-table-wrap {
	max-width: min(640px, 70vw);
	max-height: 260px;
	overflow: auto;
}
.metric-floating-table-caption {
	font-size: 11px;
	color: var(--text-secondary);
	margin-bottom: 4px;
}
.metric-floating-table {
	border-collapse: collapse;
	font-size: 11px;
	line-height: 1.4;
	color: var(--text-secondary);
}
.metric-floating-table th,
.metric-floating-table td {
	border: 1px solid rgba(148, 163, 184, 0.2);
	padding: 3px 6px;
	text-align: left;
	vertical-align: top;
}
.metric-floating-table th {
	color: var(--text-primary);
	font-weight: 600;
	white-space: nowrap;
}
.metric-floating-table tr.matched td {
	color: var(--paper);
	background: rgba(22, 163, 74, 0.16);
}
.metric-floating-card.light .metric-floating-table tr.matched td { color: var(--text-primary); }
	.metric-floating-metric-id {
		text-transform: none;
	}
</style>
