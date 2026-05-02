<script lang="ts">
	import type { SummaryRecordCard } from './summary-types';

	let {
		summary,
		selected = false,
		onSelect = () => {}
	}: {
		summary: SummaryRecordCard;
		selected?: boolean;
		onSelect?: () => void;
	} = $props();
</script>

<button class:selected={selected} class="summary-card" type="button" onclick={onSelect}>
	<div class="card-head">
		<div class="file-name" title={summary.pdfFileName}>{summary.pdfFileName}</div>
		<div class="page-badge">p.{summary.page}</div>
	</div>
	<div class="keyword-row">
		{#each summary.keywords as keyword}
			<span class="keyword">{keyword}</span>
		{/each}
	</div>
	<p>{summary.summaryText}</p>
</button>

<style>
	.summary-card {
		width: 100%;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		background: rgba(15, 23, 42, 0.38);
		padding: 14px;
		text-align: left;
		color: inherit;
		transition:
			border-color 140ms ease,
			transform 140ms ease,
			background 140ms ease;
	}

	.summary-card:hover,
	.summary-card.selected {
		border-color: rgba(129, 140, 248, 0.42);
		background: rgba(99, 102, 241, 0.12);
		transform: translateY(-1px);
	}

	.card-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 10px;
	}

	.file-name {
		font-size: 0.92rem;
		font-weight: 700;
		line-height: 1.35;
	}

	.page-badge {
		flex-shrink: 0;
		border-radius: 999px;
		padding: 0.2rem 0.55rem;
		background: rgba(148, 163, 184, 0.12);
		font-size: 0.72rem;
	}

	.keyword-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin-bottom: 10px;
	}

	.keyword {
		border-radius: 999px;
		padding: 0.22rem 0.55rem;
		background: rgba(245, 158, 11, 0.14);
		font-size: 0.72rem;
		font-weight: 600;
		color: #fbbf24;
	}

	p {
		margin: 0;
		font-size: 0.84rem;
		line-height: 1.55;
		opacity: 0.9;
	}
</style>
