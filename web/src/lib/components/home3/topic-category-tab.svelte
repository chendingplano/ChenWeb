<script lang="ts">
	import TopicCard from './topic-card.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import { formatTopicLineSpecs } from './topic-line-specs';
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
	let viewerIsPdf = $derived(
		(selectedTopic?.pdfFileName ?? '').trim().toLowerCase().endsWith('.pdf')
	);
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
			{#if viewerInputId && selectedTarget && viewerIsPdf}
				<PdfViewWindow
					inputId={viewerInputId}
					fileUrl={viewerFileUrl}
					bind:page={viewerPage}
					bind:zoom={viewerZoom}
					bind:numPages={viewerNumPages}
					highlightVersion={selectedTopic
						? `${selectedTopic.id}:${selectedTopic.targets.map((t) => `${t.page}:${t.coords.join(',')}`).join('|')}`
						: 'topic-category'}
					renderHighlights={renderTopicHighlight}
					sidebarMinWidth={240}
					sidebarMaxWidth={520}
					sidebarDefaultWidth={320}
					sidebarTitle="Topic Details"
					sidebarSettingsKey="topic-category-pdf-sidebar"
					sidebarWidthSettingLabel="Panel Width"
				>
					{#snippet sidebar()}
						{#if selectedTopic}
							<div class="topic-sidebar-block">
								<div class="topic-sidebar-row">
									<span>Topic ID</span>
									<strong>{selectedTopic.id}</strong>
								</div>
								<div class="topic-sidebar-row">
									<span>Record ID</span>
									<strong>{selectedTopic.inputId}</strong>
								</div>
								<div class="topic-sidebar-row">
									<span>Page</span>
									<strong>{selectedTopic.page}</strong>
								</div>
								<div class="topic-sidebar-row">
									<span>Type</span>
									<strong>{selectedTopic.topicType || '—'}</strong>
								</div>
								<div class="topic-sidebar-row">
									<span>Line Numbers</span>
									<strong>{formatTopicLineSpecs(selectedTopic.sourceLineSpecs)}</strong>
								</div>
							</div>
							<div class="topic-sidebar-block">
								<div class="topic-sidebar-label">Topic</div>
								<p class="topic-sidebar-copy">{selectedTopic.topicText}</p>
							</div>
							<div class="topic-sidebar-block">
								<div class="topic-sidebar-label">Category Paths</div>
								{#if selectedTopic.categoryPaths && selectedTopic.categoryPaths.length > 0}
									<div class="keyword-list">
										{#each selectedTopic.categoryPaths as path, idx (`${path}-${idx}`)}
											<span class="keyword-chip">{path}</span>
										{/each}
									</div>
								{:else}
									<p class="topic-sidebar-copy muted">No category paths assigned.</p>
								{/if}
							</div>
							<div class="topic-sidebar-block">
								<div class="topic-sidebar-label">Keywords</div>
								{#if selectedTopic.topicKeywords.length > 0}
									<div class="keyword-list">
										{#each selectedTopic.topicKeywords as kw, idx (`${kw}-${idx}`)}
											<span class="keyword-chip">{kw}</span>
										{/each}
									</div>
								{:else}
									<p class="topic-sidebar-copy muted">No keywords extracted.</p>
								{/if}
							</div>
						{:else}
							<div class="topic-sidebar-empty">Select a topic to inspect it alongside the source PDF.</div>
						{/if}
					{/snippet}
				</PdfViewWindow>
			{:else if viewerInputId && selectedTarget}
				<iframe class="pdf-fallback-frame" title={selectedTopic?.pdfFileName ?? categoryPath} src={viewerFileUrl}></iframe>
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

	.pdf-panel :global(.doc-page-bar) {
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 16px 16px 0 0;
	}

	.pdf-panel :global(.pdf-stage) {
		margin-top: -1px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 0 0 16px 16px;
		background: rgba(2, 6, 23, 0.28);
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

	.topic-sidebar-block + .topic-sidebar-block {
		margin-top: 1rem;
		padding-top: 1rem;
		border-top: 1px solid rgba(148, 163, 184, 0.1);
	}

	.topic-sidebar-row {
		display: grid;
		grid-template-columns: 96px minmax(0, 1fr);
		gap: 0.6rem;
		margin-bottom: 0.6rem;
		align-items: start;
	}

	.topic-sidebar-row span,
	.topic-sidebar-label {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: #94a3b8;
	}

	.topic-sidebar-row strong,
	.topic-sidebar-copy {
		min-width: 0;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.topic-sidebar-copy {
		margin-top: 0.55rem;
		font-size: 0.88rem;
		line-height: 1.6;
		color: #e2e8f0;
	}

	.topic-sidebar-copy.muted,
	.topic-sidebar-empty {
		color: #94a3b8;
	}

	.topic-sidebar-empty {
		font-size: 0.9rem;
		line-height: 1.6;
	}

	.keyword-list {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin-top: 0.6rem;
	}

	.keyword-chip {
		display: inline-flex;
		align-items: center;
		padding: 0.32rem 0.58rem;
		border-radius: 999px;
		border: 1px solid rgba(74, 222, 128, 0.22);
		background: rgba(13, 148, 136, 0.12);
		font-size: 0.78rem;
		color: #6ee7b7;
	}

	.pdf-fallback-frame {
		width: 100%;
		min-height: 540px;
		flex: 1;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 16px;
		background: white;
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
