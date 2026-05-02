<script lang="ts">
	import SummaryCard from './summary-card.svelte';
	import type { SummaryPdfTarget, SummaryRecordCard } from './summary-types';

	let {
		categoryPath,
		summaries = [],
		selectedSummaryId = null,
		selectedTarget = null,
		onSelectSummary = () => {}
	}: {
		categoryPath: string;
		summaries?: SummaryRecordCard[];
		selectedSummaryId?: string | null;
		selectedTarget?: SummaryPdfTarget | null;
		onSelectSummary?: (summary: SummaryRecordCard) => void;
	} = $props();

	let leftWidth = $state(360);
	let resizing = $state(false);
	let startX = 0;
	let startWidth = 360;

	function beginResize(event: MouseEvent) {
		resizing = true;
		startX = event.clientX;
		startWidth = leftWidth;
	}

	function handleMove(event: MouseEvent) {
		if (!resizing) return;
		leftWidth = Math.max(280, Math.min(520, startWidth + (event.clientX - startX)));
	}

	function endResize() {
		resizing = false;
	}
</script>

<svelte:window onmousemove={handleMove} onmouseup={endResize} />

<div class="category-shell">
	<div class="category-head">
		<div>
			<div class="eyebrow">Category Path</div>
			<h3 title={categoryPath}>{categoryPath}</h3>
		</div>
		<div class="meta-pill">{summaries.length} summaries</div>
	</div>

	<div class="category-body">
		<div class="summary-list" style={`width:${leftWidth}px;`}>
			{#if summaries.length === 0}
				<div class="empty-state">No summaries in this category yet.</div>
			{:else}
				{#each summaries as summary}
					<SummaryCard
						{summary}
						selected={selectedSummaryId === summary.id}
						onSelect={() => onSelectSummary(summary)}
					/>
				{/each}
			{/if}
		</div>

		<button
			type="button"
			class:active={resizing}
			class="resizer"
			aria-label="Resize summary list"
			onmousedown={beginResize}
		></button>

		<div class="pdf-panel">
			<div class="pdf-head">
				<div>
					<div class="eyebrow">PDF Display</div>
					<h4>{selectedTarget ? `Input #${selectedTarget.inputId}` : 'Select a summary'}</h4>
				</div>
				{#if selectedTarget}
					<div class="meta-pill">page {selectedTarget.page}</div>
				{/if}
			</div>

			{#if selectedTarget}
				<div class="pdf-mock">
					<div class="paper">
						<div class="paper-title">Mock PDF Jump Target</div>
						<div class="paper-grid">
							<div><span>Input ID</span><strong>{selectedTarget.inputId}</strong></div>
							<div><span>Page</span><strong>{selectedTarget.page}</strong></div>
							<div><span>Summary ID</span><strong>{selectedTarget.summaryId}</strong></div>
							<div><span>Status</span><strong>Phase 1 Mock</strong></div>
						</div>
					</div>
				</div>
			{:else}
				<div class="pdf-empty">
					Select a summary card on the left to preview its mocked PDF target.
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.category-shell {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.category-head,
	.pdf-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}

	.category-head {
		margin-bottom: 1rem;
	}

	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: #94a3b8;
	}

	h3,
	h4 {
		margin: 0.25rem 0 0;
		font-size: 1.05rem;
	}

	.meta-pill {
		border-radius: 999px;
		padding: 0.32rem 0.72rem;
		background: rgba(99, 102, 241, 0.14);
		color: #c7d2fe;
		font-size: 0.76rem;
		font-weight: 700;
	}

	.category-body {
		display: flex;
		min-height: 0;
		flex: 1;
		border-radius: 22px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.4);
		overflow: hidden;
	}

	.summary-list {
		display: flex;
		min-width: 0;
		flex-direction: column;
		gap: 0.85rem;
		padding: 1rem;
		overflow: auto;
	}

	.resizer {
		width: 10px;
		border: none;
		border-inline: 1px solid rgba(148, 163, 184, 0.1);
		background:
			linear-gradient(180deg, rgba(129, 140, 248, 0.05), rgba(129, 140, 248, 0.12));
		cursor: col-resize;
	}

	.resizer.active {
		background: rgba(129, 140, 248, 0.35);
	}

	.pdf-panel {
		display: flex;
		min-width: 0;
		flex: 1;
		flex-direction: column;
		padding: 1rem;
	}

	.pdf-mock,
	.pdf-empty,
	.empty-state {
		display: flex;
		flex: 1;
		align-items: center;
		justify-content: center;
		border-radius: 18px;
		border: 1px dashed rgba(148, 163, 184, 0.2);
		background: rgba(2, 6, 23, 0.28);
		padding: 1.2rem;
		color: #94a3b8;
		text-align: center;
	}

	.paper {
		width: min(420px, 100%);
		border-radius: 20px;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(241, 245, 249, 0.92));
		padding: 1.4rem;
		color: #0f172a;
		box-shadow: 0 22px 50px rgba(15, 23, 42, 0.22);
	}

	.paper-title {
		margin-bottom: 1rem;
		font-size: 1rem;
		font-weight: 800;
	}

	.paper-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
		text-align: left;
	}

	.paper-grid span {
		display: block;
		margin-bottom: 0.2rem;
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: #64748b;
	}

	.paper-grid strong {
		font-size: 0.95rem;
	}

	@media (max-width: 900px) {
		.category-body {
			flex-direction: column;
		}

		.summary-list {
			width: 100% !important;
			max-height: 40vh;
		}

		.resizer {
			width: auto;
			height: 10px;
			cursor: row-resize;
		}
	}
</style>
