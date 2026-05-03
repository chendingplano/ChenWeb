<script lang="ts">
	import TopicCard from './topic-card.svelte';
	import SharedPdfViewer from './shared-pdf-viewer.svelte';
	import type { TopicCard as TopicCardType, TopicPdfTarget, TopicRecordTarget } from './topic-types';

	let {
		categoryPath,
		topics = [],
		selectedTopicId = null,
		selectedTarget = null,
		onSelectTopic = () => {}
	}: {
		categoryPath: string;
		topics?: TopicCardType[];
		selectedTopicId?: string | null;
		selectedTarget?: TopicPdfTarget | null;
		onSelectTopic?: (topic: TopicCardType) => void;
	} = $props();

	let leftWidth = $state(360);
	let resizing = $state(false);
	let startX = 0;
	let startWidth = 360;

	let selectedTopic = $derived(topics.find((t) => t.id === selectedTopicId) ?? null);
	let viewerInputId = $derived(selectedTarget?.inputId ?? null);
	let viewerFileUrl = $derived(viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : '');
	let viewerPage = $state(1);
	let viewerZoom = $state(0.5);
	let viewerNumPages = $state(0);

	type TopicPdfViewport = {
		convertToViewportRectangle: (rect: number[]) => number[];
	};

	$effect(() => {
		if (selectedTarget?.page) {
			viewerPage = selectedTarget.page;
		}
	});

	function renderTopicHighlight(pageNo: number, viewport: TopicPdfViewport, overlay: HTMLDivElement) {
		if (!selectedTopic) return;
		const targets = selectedTopic.targets?.filter(
			(t: TopicRecordTarget) => t.page === pageNo && Array.isArray(t.coords) && t.coords.length >= 4
		);
		if (!targets || targets.length === 0) return;
		for (const target of targets) {
			const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(target.coords.slice(0, 4));
			const left = Math.max(0, Math.min(vx1, vx2) - 5);
			const top = Math.max(0, Math.min(vy1, vy2) - 4);
			const width = Math.abs(vx2 - vx1) + 10;
			const height = Math.abs(vy2 - vy1) + 8;
			if (width < 1 || height < 1) continue;
			const box = document.createElement('div');
			box.className = 'pdf-highlight';
			box.style.left = `${left}px`;
			box.style.top = `${top}px`;
			box.style.width = `${width}px`;
			box.style.height = `${height}px`;
			overlay.appendChild(box);
		}
	}

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
		<div class="meta-pill">{topics.length} topics</div>
	</div>

	<div class="category-body">
		<div class="topic-list" style={`width:${leftWidth}px;`}>
			{#if topics.length === 0}
				<div class="empty-state">
					No topics are available for this category yet.
				</div>
			{:else}
				{#each topics as topic}
					<TopicCard
						{topic}
						selected={selectedTopicId === topic.id}
						onSelect={() => onSelectTopic(topic)}
					/>
				{/each}
			{/if}
		</div>

		<button
			type="button"
			class:active={resizing}
			class="resizer"
			aria-label="Resize topic list"
			onmousedown={beginResize}
		></button>

		<div class="pdf-panel">
			{#if viewerInputId && selectedTarget}
				<SharedPdfViewer
					inputId={viewerInputId}
					fileUrl={viewerFileUrl}
					bind:page={viewerPage}
					bind:zoom={viewerZoom}
					bind:numPages={viewerNumPages}
					highlightVersion={selectedTopic
						? `${selectedTopic.id}:${selectedTopic.targets.map((t) => `${t.page}:${t.coords.join(',')}`).join('|')}`
						: 'topic-category'}
					renderHighlights={renderTopicHighlight}
				/>
			{:else}
				<div class="pdf-empty">
					Select a topic card on the left to display its source PDF.
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

	.category-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: #94a3b8;
	}

	h3 {
		margin: 0.25rem 0 0;
		font-size: 1.05rem;
	}

	.meta-pill {
		border-radius: 999px;
		padding: 0.32rem 0.72rem;
		background: rgba(34, 197, 94, 0.14);
		color: #4ade80;
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

	.topic-list {
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
			linear-gradient(180deg, rgba(34, 197, 94, 0.05), rgba(34, 197, 94, 0.12));
		cursor: col-resize;
	}

	.resizer.active {
		background: rgba(34, 197, 94, 0.35);
	}

	.pdf-panel {
		display: flex;
		min-width: 0;
		flex: 1;
		flex-direction: column;
	}

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

	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(34, 197, 94, 0.25);
		outline: 1px solid rgba(34, 197, 94, 0.8);
		border-radius: 2px;
	}

	@media (max-width: 900px) {
		.category-body {
			flex-direction: column;
		}

		.topic-list {
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
