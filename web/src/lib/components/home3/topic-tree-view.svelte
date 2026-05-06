<script lang="ts">
	import PanelsTopLeftIcon from '@lucide/svelte/icons/panels-top-left';
	import Rows3Icon from '@lucide/svelte/icons/rows-3';
	import {
		getRecordTopics,
		type KbInputRecord,
		type TopicCardApi
	} from '$lib/services/kbService';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import {
		createTopicTreeState,
		selectRecordTopicTarget,
		selectTopicTreeRecord,
		toggleTopicTreeListMode
	} from './topic-tree-state.js';
	import type { TopicRecordTarget, TopicTreeRecord } from './topic-types';

	let {
		darkMode = true,
		heroEyebrow = 'Semantic Web',
		heroTitle = 'Document Semantic Tree',
		heroDescription = 'Document-centric browser over topics extracted from chunks.',
		sidebarTitle = 'Selected Topic',
		loadErrorTitle = 'Topic Tree',
		itemSingular = 'topic',
		itemPlural = 'topics',
		getRecordItems = getRecordTopics,
		scopeToActiveStore = false,
		browserInstanceKey = 'topic-tree'
	}: {
		darkMode?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
		sidebarTitle?: string;
		loadErrorTitle?: string;
		itemSingular?: string;
		itemPlural?: string;
		getRecordItems?: (recordId: number) => Promise<{ topics: TopicCardApi[] }>;
		scopeToActiveStore?: boolean;
		browserInstanceKey?: string;
	} = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#22c55e' : '#16a34a');

	let loadError = $state('');
	let errorDialogOpen = $state(false);
	let topicLoadingByRecordId = $state<Record<number, boolean>>({});
	let recordCache = $state<Record<number, TopicTreeRecord>>({});
	let treeState = $state(createTopicTreeState());
	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);

	let activeRecord = $derived(
		treeState.selectedRecordId != null ? (recordCache[treeState.selectedRecordId] ?? null) : null
	);
	let selectedTopic = $derived(
		activeRecord?.topics.find((topic) => topic.id === treeState.selectedTopicId) ?? null
	);
	let viewerInputId = $derived(treeState.selectedPdfTarget?.inputId ?? activeRecord?.id ?? null);
	let viewerFileUrl = $derived(viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : '');
	let viewerIsPdf = $derived((activeRecord?.fileName ?? '').trim().toLowerCase().endsWith('.pdf'));

	type TopicPdfViewport = {
		convertToViewportRectangle: (rect: number[]) => number[];
	};

	function renderTopicHighlight(pageNo: number, viewport: TopicPdfViewport, overlay: HTMLDivElement) {
		if (!selectedTopic) return;
		const targets = selectedTopic.targets?.filter(
			(target: TopicRecordTarget) =>
				target.page === pageNo && Array.isArray(target.coords) && target.coords.length >= 4
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

	function topicTreeStatusText(record: KbInputRecord): string {
		const items = record.status ?? [];
		const matched = [...items].reverse().find((item) => item != null);
		if (!matched) return 'pending';
		return (
			matched.proc_status?.trim() ||
			matched['proc-status']?.trim() ||
			matched.status?.trim() ||
			'pending'
		);
	}

	function mapKbInputToTopicTreeRecord(record: KbInputRecord): TopicTreeRecord {
		return {
			id: record.id,
			title:
				record.title?.trim() ||
				record.name?.trim() ||
				record.file_name?.trim() ||
				`Record #${record.id}`,
			fileName: record.file_name?.trim() || record.name?.trim() || '—',
			docType: record.type?.trim() || '—',
			docNo: record.doc_no?.trim() || '—',
			parserName: record.parser_name?.trim() || '—',
			procStatus: topicTreeStatusText(record),
			createTime: record.create_time ? new Date(record.create_time).toLocaleString() : '—',
			modifyTime: record.modify_time ? new Date(record.modify_time).toLocaleString() : '—',
			topics: []
		};
	}

	$effect(() => {
		const selectedPage = treeState.selectedPdfTarget?.page;
		if (selectedPage) {
			docPage = selectedPage;
		}
	});

	$effect(() => {
		if (activeRecord) {
			void ensureRecordTopics(activeRecord.id);
		}
	});

	async function ensureRecordTopics(recordId: number) {
		const current = recordCache[recordId];
		if (!current || current.topics.length > 0 || topicLoadingByRecordId[recordId]) return;

		topicLoadingByRecordId = { ...topicLoadingByRecordId, [recordId]: true };
		try {
			const response = await getRecordItems(recordId);
			recordCache = {
				...recordCache,
				[recordId]: {
					...current,
					topics: response.topics.map((topic) => ({ ...topic, recordId }))
				}
			};
			if (
				treeState.selectedRecordId === recordId &&
				response.topics[0] &&
				!treeState.selectedPdfTarget
			) {
				treeState = selectRecordTopicTarget(treeState, {
					recordId,
					topicId: response.topics[0].id,
					inputId: response.topics[0].inputId,
					page: response.topics[0].page
				});
			}
		} catch (error) {
			loadError =
				error instanceof Error ? error.message : `Failed to load ${itemPlural} for record ${recordId}`;
			errorDialogOpen = true;
		} finally {
			topicLoadingByRecordId = { ...topicLoadingByRecordId, [recordId]: false };
		}
	}

	function handleRecordSelect(record: KbInputRecord) {
		const mapped = mapKbInputToTopicTreeRecord(record);
		recordCache = {
			...recordCache,
			[record.id]: {
				...(recordCache[record.id] ?? mapped),
				...mapped,
				topics: recordCache[record.id]?.topics ?? []
			}
		};
		treeState = selectTopicTreeRecord(treeState, record.id);
	}

	function mapBrowserRecord(record: KbInputRecord) {
		const mapped = mapKbInputToTopicTreeRecord(record);
		return {
			id: record.id,
			title: mapped.title,
			subtitle: mapped.fileName,
			meta: [mapped.docNo, mapped.parserName],
			status: mapped.procStatus,
			description: mapped.createTime,
			badges: [mapped.docType]
		};
	}
</script>

<div
	class="tree-shell"
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --panel-bg:${panelBg}; --panel-bg-alt:${panelAlt}; --ink-line:${border}; --ink-line-soft:rgba(148,163,184,0.16); --text-primary:${textMain}; --text-secondary:${textMuted}; --text-muted:${textMuted};`}
>
	{#if errorDialogOpen && loadError}
		<div
			class="error-overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (errorDialogOpen = false)}
			onkeydown={(event) => {
				if (event.key === 'Escape') errorDialogOpen = false;
			}}
		>
			<div
				class="error-dialog"
				role="dialog"
				aria-modal="true"
				aria-label="Topic Tree Load Error"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				<div class="eyebrow">Load Error</div>
				<h3>Could not load {loadErrorTitle} data</h3>
				<p class="dialog-copy">{loadError}</p>
				<div class="dialog-actions">
					<button type="button" class="ghost" onclick={() => (errorDialogOpen = false)}>Close</button>
				</div>
			</div>
		</div>
	{/if}

	<div class="hero">
		<div>
			<div class="eyebrow">{heroEyebrow}</div>
			<h2>{heroTitle}</h2>
			<p>{heroDescription}</p>
		</div>
		<div class="hero-actions">
			<button type="button" class="ghost" onclick={() => (treeState = toggleTopicTreeListMode(treeState))}>
				{#if treeState.listMode === 'compact'}
					<PanelsTopLeftIcon class="h-4 w-4" />
					Expanded View
				{:else}
					<Rows3Icon class="h-4 w-4" />
					Compact View
				{/if}
			</button>
		</div>
	</div>

	<div class="workspace">
		<KbInputRecordBrowser
			{darkMode}
			instanceKey={browserInstanceKey}
			title="kb.inputs"
			subtitle={`Search, filter, and select records before inspecting their ${itemPlural}.`}
			emptyTitle="No records found."
			emptySubtitle={`Use Search or Retrieve to browse kb.inputs for ${itemPlural}.`}
			{scopeToActiveStore}
			selectedRecordId={treeState.selectedRecordId}
			renderMode={treeState.listMode === 'cards' ? 'cards' : 'compact'}
			mapRecord={mapBrowserRecord}
			onSelect={handleRecordSelect}
		/>

		<div class="right-panel">
			<div class="right-tabs">
				<div class="tab active">PDF Display</div>
				{#if activeRecord}
					<div class="tab passive" title={activeRecord.fileName}>{activeRecord.fileName}</div>
				{/if}
			</div>

			{#if activeRecord}
				<div class="detail-grid">
					<div class="detail-card">
						<div class="eyebrow">Selected Record</div>
						<h3>{activeRecord.title}</h3>
						<div class="detail-meta">
							<span>{activeRecord.docNo}</span>
							<span>{activeRecord.docType}</span>
							<span>{activeRecord.parserName}</span>
						</div>
						<div class="topic-snippets">
							{#each activeRecord.topics as topic}
								<button
									type="button"
									class:selected={treeState.selectedTopicId === topic.id}
									class="snippet"
									onclick={() =>
										(treeState = selectRecordTopicTarget(treeState, {
											recordId: activeRecord.id,
											topicId: topic.id,
											inputId: topic.inputId,
											page: topic.page
										}))}
								>
									<div class="snippet-head">
										<div class="keyword-row">
											{#each topic.topicKeywords.slice(0, 4) as kw}
												<span class="keyword">{kw}</span>
											{/each}
										</div>
										<span>p.{topic.page}</span>
									</div>
									<p>{topic.topicText}</p>
								</button>
							{/each}
						</div>
					</div>

					<div class="pdf-card">
						<div class="eyebrow">PDF Display</div>
						{#if viewerInputId && treeState.selectedPdfTarget && viewerIsPdf}
							<PdfViewWindow
								inputId={viewerInputId}
								fileUrl={viewerFileUrl}
								bind:page={docPage}
								bind:zoom={pdfZoom}
								bind:numPages={pdfNumPages}
								highlightVersion={selectedTopic
									? `${selectedTopic.id}:${selectedTopic.targets.map((target) => `${target.page}:${target.coords.join(',')}`).join('|')}`
									: 'topic-tree'}
								renderHighlights={renderTopicHighlight}
								sidebarMinWidth={240}
								sidebarMaxWidth={520}
								sidebarDefaultWidth={320}
							>
								{#snippet sidebar()}
									<aside class="topic-sidebar">
										<div class="topic-sidebar-title">{sidebarTitle}</div>
										{#if selectedTopic}
											<div class="topic-sidebar-block">
												<div class="topic-sidebar-row">
													<span>{itemSingular} ID</span>
													<strong>{selectedTopic.id}</strong>
												</div>
												<div class="topic-sidebar-row">
													<span>Record ID</span>
													<strong>{selectedTopic.recordId}</strong>
												</div>
												<div class="topic-sidebar-row">
													<span>Page</span>
													<strong>{selectedTopic.page}</strong>
												</div>
												<div class="topic-sidebar-row">
													<span>Type</span>
													<strong>{selectedTopic.topicType || '—'}</strong>
												</div>
											</div>
											<div class="topic-sidebar-block">
												<div class="topic-sidebar-label">{itemSingular}</div>
												<p class="topic-sidebar-copy">{selectedTopic.topicText}</p>
											</div>
											<div class="topic-sidebar-block">
												<div class="topic-sidebar-label">Keywords</div>
												{#if selectedTopic.topicKeywords.length > 0}
													<div class="keyword-list">
														{#each selectedTopic.topicKeywords as kw (kw)}
															<span class="keyword-chip">{kw}</span>
														{/each}
													</div>
												{:else}
													<p class="topic-sidebar-copy muted">No keywords extracted.</p>
												{/if}
											</div>
										{:else}
											<div class="topic-sidebar-empty">Select a {itemSingular} to inspect it alongside the source PDF.</div>
										{/if}
									</aside>
								{/snippet}
							</PdfViewWindow>
						{:else if viewerInputId && treeState.selectedPdfTarget}
							<iframe class="pdf-fallback-frame" title={activeRecord.fileName} src={viewerFileUrl}></iframe>
						{:else if topicLoadingByRecordId[activeRecord.id]}
							<div class="empty-state pdf-empty">Loading {itemPlural} for the selected record…</div>
						{:else if activeRecord.topics.length === 0}
							<div class="empty-state pdf-empty">No {itemPlural} are available for this record yet.</div>
						{:else}
							<div class="empty-state pdf-empty">Select a {itemSingular} to move the PDF display to the relevant page.</div>
						{/if}
					</div>
				</div>
			{:else}
				<div class="empty-state">Select a record from the left panel to inspect its {itemPlural}.</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.tree-shell {
		position: relative;
		display: flex;
		height: 100%;
		flex-direction: column;
		color: var(--text);
	}

	.error-overlay {
		position: absolute;
		inset: 0;
		z-index: 24;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1.5rem;
		background: rgba(2, 6, 23, 0.62);
		backdrop-filter: blur(10px);
	}

	.error-dialog {
		width: min(540px, 100%);
		border-radius: 22px;
		border: 1px solid rgba(248, 113, 113, 0.28);
		background: #111827;
		padding: 1.25rem;
		box-shadow: 0 30px 80px rgba(15, 23, 42, 0.5);
	}

	.dialog-copy { margin-top: 0.55rem; color: var(--muted); }

	.dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.hero {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
		padding: 1.1rem 1.2rem;
		border-radius: 24px;
		background:
			radial-gradient(circle at top right, rgba(34, 197, 94, 0.14), transparent 42%),
			linear-gradient(180deg, rgba(15, 23, 42, 0.86), rgba(15, 23, 42, 0.66));
		border: 1px solid var(--border);
	}

	.eyebrow { font-size: 0.72rem; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); }
	h2, h3, p { margin: 0; }
	h2 { margin-top: 0.3rem; font-size: 1.5rem; }
	.hero p { margin-top: 0.45rem; max-width: 52rem; color: var(--muted); }

	.hero-actions { display: flex; gap: 0.75rem; }

	.ghost {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		border-radius: 14px;
		padding: 0.8rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.16);
		cursor: pointer;
	}

	.ghost { background: rgba(15, 23, 42, 0.36); color: var(--text); }

	.workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 1rem;
	}

	.right-panel {
		min-height: 0;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
		display: flex;
		flex-direction: column;
		padding: 1rem;
	}

	.snippet-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 0.75rem; }

	.detail-meta { display: flex; flex-wrap: wrap; gap: 0.45rem; margin: 0.75rem 0; }
	.detail-meta span { border-radius: 999px; padding: 0.25rem 0.55rem; background: rgba(148, 163, 184, 0.12); font-size: 0.72rem; color: var(--muted); }

	.right-tabs { display: flex; flex-wrap: wrap; gap: 0.55rem; margin-bottom: 1rem; }

	.tab { border-radius: 14px; padding: 0.72rem 0.9rem; border: 1px solid rgba(148, 163, 184, 0.14); background: rgba(15, 23, 42, 0.3); font-weight: 700; }
	.tab.active { border-color: rgba(34, 197, 94, 0.36); background: rgba(13, 148, 136, 0.16); color: #a7f3d0; }
	.tab.passive { color: var(--muted); }

	.detail-grid { display: grid; min-height: 0; flex: 1; grid-template-columns: 340px minmax(0, 1fr); gap: 1rem; }
	.detail-card, .pdf-card { display: flex; flex-direction: column; min-height: 0; border-radius: 20px; border: 1px solid rgba(148, 163, 184, 0.14); background: rgba(15, 23, 42, 0.26); padding: 1rem; }

	.topic-snippets { display: flex; min-height: 0; flex: 1; flex-direction: column; gap: 0.75rem; margin-top: 1rem; overflow: auto; padding-right: 0.25rem; }

	.snippet { position: relative; border-radius: 16px; border: 1px solid rgba(148, 163, 184, 0.14); background: rgba(15, 23, 42, 0.34); padding: 0.85rem; text-align: left; color: inherit; transition: border-color 150ms ease, background 150ms ease, box-shadow 150ms ease, transform 150ms ease; }
	.snippet:hover { border-color: rgba(34, 197, 94, 0.34); }
	.snippet.selected { border-color: rgba(74, 222, 128, 0.58); background: linear-gradient(180deg, rgba(13, 148, 136, 0.14), rgba(13, 148, 136, 0.06)), rgba(15, 23, 42, 0.9); box-shadow: 0 0 0 1px rgba(74, 222, 128, 0.18) inset, 0 16px 36px rgba(15, 23, 42, 0.24); transform: translateY(-1px); }
	.snippet p { margin-top: 0.5rem; font-size: 0.84rem; color: var(--muted); line-height: 1.5; }
	.snippet.selected p { color: #d1fae5; }

	.keyword-row { display: flex; flex-wrap: wrap; gap: 0.3rem; }
	.keyword { border-radius: 999px; padding: 0.18rem 0.48rem; background: rgba(34, 197, 94, 0.14); font-size: 0.7rem; font-weight: 600; color: #4ade80; }

	.empty-state { display: flex; flex: 1; align-items: center; justify-content: center; border-radius: 18px; border: 1px dashed rgba(148, 163, 184, 0.2); background: rgba(2, 6, 23, 0.28); padding: 1rem; text-align: center; color: var(--muted); }

	.pdf-card :global(.doc-page-bar) { margin-top: 0.85rem; border: 1px solid rgba(148, 163, 184, 0.14); border-radius: 16px 16px 0 0; }
	.pdf-card :global(.pdf-stage) { margin-top: -1px; border: 1px solid rgba(148, 163, 184, 0.14); border-radius: 0 0 16px 16px; background: rgba(2, 6, 23, 0.28); }

	.topic-sidebar { width: 100%; height: 100%; min-width: 0; padding: 1rem 1rem 1rem 0; background: rgba(15, 23, 42, 0.82); overflow: auto; }
	.topic-sidebar-title, .topic-sidebar-label { font-size: 0.72rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
	.topic-sidebar-title { margin-bottom: 0.85rem; }
	.topic-sidebar-block + .topic-sidebar-block { margin-top: 1rem; padding-top: 1rem; border-top: 1px solid rgba(148, 163, 184, 0.1); }
	.topic-sidebar-row { display: grid; grid-template-columns: 76px minmax(0, 1fr); gap: 0.6rem; margin-bottom: 0.6rem; align-items: start; }
	.topic-sidebar-row span { font-size: 0.76rem; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); }
	.topic-sidebar-row strong, .topic-sidebar-copy { min-width: 0; overflow-wrap: anywhere; word-break: break-word; }

	.topic-sidebar-copy { margin-top: 0.55rem; font-size: 0.88rem; line-height: 1.6; color: var(--text); }
	.topic-sidebar-copy.muted, .topic-sidebar-empty { color: var(--muted); }
	.topic-sidebar-empty { font-size: 0.9rem; line-height: 1.6; }

	.keyword-list { display: flex; flex-wrap: wrap; gap: 0.45rem; margin-top: 0.6rem; }
	.keyword-chip { display: inline-flex; align-items: center; padding: 0.32rem 0.58rem; border-radius: 999px; border: 1px solid rgba(74, 222, 128, 0.22); background: rgba(13, 148, 136, 0.12); font-size: 0.78rem; color: #6ee7b7; }

	.pdf-fallback-frame { margin-top: 0.85rem; width: 100%; min-height: 540px; flex: 1; border: 1px solid rgba(148, 163, 184, 0.14); border-radius: 16px; background: white; }

	:global(.pdf-highlight) { position: absolute; background: rgba(34, 197, 94, 0.25); outline: 1px solid rgba(34, 197, 94, 0.8); border-radius: 2px; }

	@media (max-width: 980px) {
		.workspace, .detail-grid { grid-template-columns: minmax(0, 1fr); }
	}
</style>
