<script lang="ts">
	import { onMount } from 'svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import PanelsTopLeftIcon from '@lucide/svelte/icons/panels-top-left';
	import Rows3Icon from '@lucide/svelte/icons/rows-3';
	import {
		getKbInput,
		getRecordTopics,
		searchSummaryTree,
		type KbInputRecord
	} from '$lib/services/kbService';
	import KbInputSearchDialog from './kb-input-search-dialog.svelte';
	import SharedPdfViewer from './shared-pdf-viewer.svelte';
	import {
		createTopicTreeState,
		selectRecordTopicTarget,
		selectTopicTreeRecord,
		toggleTopicTreeListMode
	} from './topic-tree-state.js';
	import type { TopicRecordTarget, TopicTreeRecord } from './topic-types';
	import { knowledgeStoreState } from './knowledge-store-state.svelte';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#22c55e' : '#16a34a');

	let searchOpen = $state(false);
	let recordIdInput = $state('');
	let results = $state<TopicTreeRecord[]>([]);
	let loading = $state(true);
	let loadError = $state('');
	let errorDialogOpen = $state(false);
	let topicLoadingByRecordId = $state<Record<number, boolean>>({});
	let treeState = $state(createTopicTreeState());
	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);
	const TOPIC_SIDEBAR_MIN_WIDTH = 240;
	const TOPIC_SIDEBAR_MAX_WIDTH = 520;
	let topicSidebarWidth = $state(320);
	let topicSidebarResizing = $state(false);

	onMount(async () => {
		await runSearch();
	});

	let activeRecord = $derived(results.find((r) => r.id === treeState.selectedRecordId) ?? results[0] ?? null);
	let selectedTopic = $derived(
		activeRecord?.topics.find((t) => t.id === treeState.selectedTopicId) ?? null
	);
	let viewerInputId = $derived(treeState.selectedPdfTarget?.inputId ?? activeRecord?.id ?? null);
	let viewerFileUrl = $derived(viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : '');
	let viewerIsPdf = $derived(
		(activeRecord?.fileName ?? '').trim().toLowerCase().endsWith('.pdf')
	);

	type TopicPdfViewport = {
		convertToViewportRectangle: (rect: number[]) => number[];
	};

	function recordDisplayName(record: TopicTreeRecord) {
		return record.title?.trim() || `Record #${record.id}`;
	}

	function recordDisplayDocNo(record: TopicTreeRecord) {
		return record.docNo?.trim() || '—';
	}

	function formatRecordTime(value: string) {
		return value?.trim() || '—';
	}

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

	function clampTopicSidebarWidth(width: number) {
		return Math.min(TOPIC_SIDEBAR_MAX_WIDTH, Math.max(TOPIC_SIDEBAR_MIN_WIDTH, Math.round(width)));
	}

	function adjustTopicSidebarWidth(delta: number) {
		topicSidebarWidth = clampTopicSidebarWidth(topicSidebarWidth + delta);
	}

	function startTopicSidebarResize(event: PointerEvent) {
		event.preventDefault();
		const handle = event.currentTarget as HTMLElement | null;
		const startX = event.clientX;
		const startWidth = topicSidebarWidth;
		topicSidebarResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		handle?.setPointerCapture?.(event.pointerId);

		const handleMove = (moveEvent: PointerEvent) => {
			topicSidebarWidth = clampTopicSidebarWidth(startWidth + (moveEvent.clientX - startX));
		};
		const handleUp = (upEvent: PointerEvent) => {
			topicSidebarResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			handle?.releasePointerCapture?.(upEvent.pointerId);
			window.removeEventListener('pointermove', handleMove);
			window.removeEventListener('pointerup', handleUp);
			window.removeEventListener('pointercancel', handleUp);
		};

		window.addEventListener('pointermove', handleMove);
		window.addEventListener('pointerup', handleUp, { once: true });
		window.addEventListener('pointercancel', handleUp, { once: true });
	}

	function onTopicSidebarResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') { event.preventDefault(); adjustTopicSidebarWidth(-16); }
		else if (event.key === 'ArrowRight') { event.preventDefault(); adjustTopicSidebarWidth(16); }
		else if (event.key === 'Home') { event.preventDefault(); topicSidebarWidth = TOPIC_SIDEBAR_MIN_WIDTH; }
		else if (event.key === 'End') { event.preventDefault(); topicSidebarWidth = TOPIC_SIDEBAR_MAX_WIDTH; }
	}

	function topicTreeStatusText(record: KbInputRecord): string {
		const items = record.status ?? [];
		const matched = [...items].reverse().find((item) => item != null);
		if (!matched) return 'pending';
		return matched.proc_status?.trim() || matched['proc-status']?.trim() || matched.status?.trim() || 'pending';
	}

	function mapKbInputToTopicTreeRecord(record: KbInputRecord): TopicTreeRecord {
		return {
			id: record.id,
			title: record.title?.trim() || record.name?.trim() || record.file_name?.trim() || `Record #${record.id}`,
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
		if (!activeRecord && results[0]) {
			treeState = selectTopicTreeRecord(treeState, results[0].id);
		}
	});

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

	async function runSearch() {
		loading = true;
		loadError = '';
		try {
			const directRecordId = Number(recordIdInput.trim());
			if (recordIdInput.trim() !== '') {
				if (!Number.isFinite(directRecordId) || directRecordId <= 0) {
					throw new Error('Enter a valid Record ID');
				}
				const response = await getKbInput(directRecordId);
				results = [mapKbInputToTopicTreeRecord(response.record)];
				treeState = createTopicTreeState();
				treeState = selectTopicTreeRecord(treeState, response.record.id);
				return;
			}

			const response = await searchSummaryTree({
				recordId: '',
				title: '',
				docNo: '',
				fileName: '',
				docType: 'all',
				parserName: '',
				operation: '',
				procStatus: 'all',
				createStart: '',
				createEnd: '',
				modifyStart: '',
				modifyEnd: '',
				ksStoreId: knowledgeStoreState.activeStore?.id ?? null
			});
			results = response.results.map((r) => ({
				...r,
				topics: []
			}));
			treeState = createTopicTreeState();
			if (results[0]) {
				treeState = selectTopicTreeRecord(treeState, results[0].id);
			}
		} catch (error) {
			results = [];
			treeState = createTopicTreeState();
			loadError = error instanceof Error ? error.message : 'Failed to load topic tree records';
			errorDialogOpen = true;
		} finally {
			loading = false;
		}
	}

	async function ensureRecordTopics(recordId: number) {
		const current = results.find((r) => r.id === recordId);
		if (!current || current.topics.length > 0 || topicLoadingByRecordId[recordId]) return;

		topicLoadingByRecordId = { ...topicLoadingByRecordId, [recordId]: true };
		try {
			const response = await getRecordTopics(recordId);
			results = results.map((r) =>
				r.id === recordId
					? { ...r, topics: response.topics.map((t) => ({ ...t, recordId })) }
					: r
			);
			if (treeState.selectedRecordId === recordId && response.topics[0] && !treeState.selectedPdfTarget) {
				treeState = selectRecordTopicTarget(treeState, {
					recordId,
					topicId: response.topics[0].id,
					inputId: response.topics[0].inputId,
					page: response.topics[0].page
				});
			}
		} catch (error) {
			loadError = error instanceof Error ? error.message : `Failed to load topics for record ${recordId}`;
			errorDialogOpen = true;
		} finally {
			topicLoadingByRecordId = { ...topicLoadingByRecordId, [recordId]: false };
		}
	}
</script>

<div
	class="tree-shell"
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --panel-bg:${panelBg}; --panel-bg-alt:${panelAlt}; --ink-line:${border}; --ink-line-soft:rgba(148,163,184,0.16); --text-primary:${textMain}; --text-secondary:${textMuted}; --text-muted:${textMuted};`}
>
	<KbInputSearchDialog
		bind:open={searchOpen}
		onSelect={async (record) => {
			recordIdInput = String(record.id);
			await runSearch();
		}}
	/>

	{#if errorDialogOpen && loadError}
		<div
			class="error-overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (errorDialogOpen = false)}
			onkeydown={(event) => { if (event.key === 'Escape') errorDialogOpen = false; }}
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
				<h3>Could not load Topic Tree data</h3>
				<p class="dialog-copy">{loadError}</p>
				<div class="dialog-actions">
					<button type="button" class="ghost" onclick={() => (errorDialogOpen = false)}>Close</button>
					<button type="button" class="primary" onclick={async () => { errorDialogOpen = false; await runSearch(); }}>Try Again</button>
				</div>
			</div>
		</div>
	{/if}

	<div class="hero">
		<div>
			<div class="eyebrow">Semantic Web</div>
			<h2>Document Semantic Tree</h2>
			<p>Document-centric browser over topics extracted from chunks.</p>
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
			<button type="button" class="primary" onclick={() => (searchOpen = true)}>
				<SearchIcon class="h-4 w-4" />
				Search
			</button>
		</div>
	</div>

	<div class="workspace">
		<div class="left-panel">
			<div class="left-controls">
				<label class="field">
					<span class="field-label">Record ID</span>
					<div class="field-row">
						<input
							type="text"
							inputmode="numeric"
							bind:value={recordIdInput}
							placeholder="e.g. 1042"
							onkeydown={(e) => { if (e.key === 'Enter') void runSearch(); }}
						/>
						<button type="button" class="ghost search-btn" onclick={() => (searchOpen = true)} title="Search records">
							<SearchIcon class="h-4 w-4" />
							Search
						</button>
					</div>
				</label>
				<button type="button" class="retrieve-btn" onclick={runSearch} disabled={loading}>
					{#if loading}Loading…{:else}<span class="retrieve-arrow">→</span>Retrieve{/if}
				</button>
			</div>

			<div class="left-meta">
				<div class="left-meta-title">kb.inputs</div>
				<div class="left-meta-count">{results.length} found</div>
			</div>

			<div class="result-list topic-tree-results">
				{#if loading}
					<div class="empty-state">Loading records…</div>
				{:else if loadError}
					<div class="empty-state">Topic Tree could not be loaded. Open the error dialog for details or try again.</div>
				{:else if results.length === 0}
					<div class="empty-state">No records found.</div>
				{:else if treeState.listMode === 'compact'}
					{#each results as record, idx (record.id)}
						<button
							type="button"
							class="record-browser-card"
							class:selected={treeState.selectedRecordId === record.id}
							onclick={() => (treeState = selectTopicTreeRecord(treeState, record.id))}
						>
							<div class="card-rule" aria-hidden="true"></div>
							<div class="card-body">
								<div class="card-row-top">
									<div class="card-index">
										№ {String(idx + 1).padStart(3, '0')}
										<span class="card-id">id {record.id}</span>
									</div>
									<div class="card-conf mono" title="Type">{record.docType || '—'}</div>
								</div>
								<div class="card-name">{recordDisplayName(record)}</div>
								<div class="card-desc">Doc No: {recordDisplayDocNo(record)}</div>
								<div class="card-foot">
									<span class="chip chip-quiet">{formatRecordTime(record.createTime)}</span>
								</div>
							</div>
						</button>
					{/each}
				{:else}
					{#each results as record}
						<button
							type="button"
							class:selected={treeState.selectedRecordId === record.id}
							class="record-card"
							onclick={() => (treeState = selectTopicTreeRecord(treeState, record.id))}
						>
							<div class="record-head">
								<div>
									<div class="record-title">{record.title}</div>
									<div class="record-subtitle">{record.fileName}</div>
								</div>
								<div class="record-status">{record.procStatus}</div>
							</div>
							<div class="record-meta">
								<span>{record.docNo}</span>
								<span>{record.parserName}</span>
							</div>
							<p>
								{record.topics[0]?.topicText ??
									(treeState.selectedRecordId === record.id && topicLoadingByRecordId[record.id]
										? 'Loading topic preview…'
										: 'Select this record to load its topics.')}
							</p>
						</button>
					{/each}
				{/if}
			</div>
		</div>

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
									onclick={() => (treeState = selectRecordTopicTarget(treeState, {
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
							<SharedPdfViewer
								inputId={viewerInputId}
								fileUrl={viewerFileUrl}
								bind:page={docPage}
								bind:zoom={pdfZoom}
								bind:numPages={pdfNumPages}
								highlightVersion={selectedTopic
									? `${selectedTopic.id}:${selectedTopic.targets.map((t) => `${t.page}:${t.coords.join(',')}`).join('|')}`
									: 'topic-tree'}
								renderHighlights={renderTopicHighlight}
							>
								<div slot="sidebar">
									<div class="topic-sidebar-shell" style={`width:${topicSidebarWidth}px;`}>
										<aside class="topic-sidebar">
											<div class="topic-sidebar-title">Selected Topic</div>
											{#if selectedTopic}
												<div class="topic-sidebar-block">
													<div class="topic-sidebar-row">
														<span>Topic ID</span>
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
													<div class="topic-sidebar-label">Topic</div>
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
												<div class="topic-sidebar-empty">Select a topic to inspect it alongside the source PDF.</div>
											{/if}
										</aside>
										<button
											type="button"
											class="topic-sidebar-resizer"
											class:active={topicSidebarResizing}
											aria-label="Resize topic sidebar"
											onpointerdown={startTopicSidebarResize}
											onkeydown={onTopicSidebarResizerKeydown}
										>
											<span class="topic-sidebar-resizer-grip" aria-hidden="true"></span>
										</button>
									</div>
								</div>
							</SharedPdfViewer>
						{:else if viewerInputId && treeState.selectedPdfTarget}
							<iframe class="pdf-fallback-frame" title={activeRecord.fileName} src={viewerFileUrl}></iframe>
						{:else if topicLoadingByRecordId[activeRecord.id]}
							<div class="empty-state pdf-empty">Loading topics for the selected record…</div>
						{:else if activeRecord.topics.length === 0}
							<div class="empty-state pdf-empty">No topics are available for this record yet.</div>
						{:else}
							<div class="empty-state pdf-empty">Select a topic to move the PDF display to the relevant page.</div>
						{/if}
					</div>
				</div>
			{:else}
				<div class="empty-state">Select a record from the left panel to inspect its topics.</div>
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

	.ghost, .primary {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		border-radius: 14px;
		padding: 0.8rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.16);
		cursor: pointer;
	}

	.ghost { background: rgba(15, 23, 42, 0.36); color: var(--text); }
	.primary { background: linear-gradient(135deg, #16a34a, #0d9488); border-color: transparent; color: white; font-weight: 700; }

	.workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: 360px minmax(0, 1fr);
		gap: 1rem;
	}

	.left-panel, .right-panel {
		min-height: 0;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
	}

	.left-panel { display: flex; flex-direction: column; padding: 0; overflow: hidden; }
	.left-controls { padding: 1rem; border-bottom: 1px solid rgba(148, 163, 184, 0.08); }

	.field { display: block; }
	.field-label { display: block; margin-bottom: 0.6rem; font-size: 0.72rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
	.field-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 0.75rem; }
	.field-row input { width: 100%; min-width: 0; border-radius: 0; border: 1px solid rgba(148, 163, 184, 0.14); background: rgba(31, 41, 55, 0.9); padding: 0.95rem 1rem; font-size: 0.95rem; color: #f3f4f6; }
	.search-btn { padding-inline: 1rem; }

	.retrieve-btn {
		margin-top: 1rem;
		width: 100%;
		border: none;
		background: #22c55e;
		color: #111827;
		padding: 0.95rem 1rem;
		font-size: 1rem;
		font-weight: 700;
		cursor: pointer;
	}

	.retrieve-btn:disabled { opacity: 0.7; cursor: default; }
	.retrieve-arrow { margin-right: 0.45rem; }

	.result-list { display: flex; min-height: 0; flex: 1; flex-direction: column; overflow: auto; padding: 0 1rem 1rem; }
	.topic-tree-results { gap: 0.8rem; }

	.left-meta { display: flex; align-items: baseline; justify-content: space-between; padding: 1rem; border-bottom: 1px solid rgba(148, 163, 184, 0.08); }
	.left-meta-title { font-size: 0.95rem; font-weight: 700; color: #bbf7d0; }
	.left-meta-count { font-size: 0.78rem; letter-spacing: 0.14em; text-transform: uppercase; color: #4ade80; }

	.record-card { margin-bottom: 0.75rem; border-radius: 16px; border: 1px solid rgba(148, 163, 184, 0.14); background: rgba(15, 23, 42, 0.28); padding: 0.9rem; text-align: left; color: inherit; }
	.record-card.selected, .record-card:hover { border-color: rgba(34, 197, 94, 0.34); background: rgba(13, 148, 136, 0.1); }

	.record-head, .snippet-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 0.75rem; }
	.record-title { font-size: 0.94rem; font-weight: 700; }
	.record-subtitle { margin-top: 0.15rem; font-size: 0.8rem; color: var(--muted); }

	.record-meta, .detail-meta { display: flex; flex-wrap: wrap; gap: 0.45rem; margin: 0.75rem 0; }
	.record-meta span, .record-status, .detail-meta span { border-radius: 999px; padding: 0.25rem 0.55rem; background: rgba(148, 163, 184, 0.12); font-size: 0.72rem; color: var(--muted); }

	.record-browser-card { position: relative; width: 100%; border: 1px solid rgba(148, 163, 184, 0.12); background: rgba(31, 41, 55, 0.72); color: inherit; text-align: left; cursor: pointer; overflow: hidden; }
	.record-browser-card:hover { background: rgba(39, 51, 67, 0.84); }
	.record-browser-card.selected { border-color: rgba(34, 197, 94, 0.38); background: rgba(30, 60, 40, 0.9); }

	.card-rule { position: absolute; left: 0; top: 0; bottom: 0; width: 6px; background: rgba(34, 197, 94, 0.18); }
	.record-browser-card.selected .card-rule { background: #22c55e; }

	.card-body { padding: 1rem 1rem 1rem 1.25rem; }
	.card-row-top { display: flex; align-items: flex-start; justify-content: space-between; gap: 0.75rem; }
	.card-index { font-size: 0.74rem; letter-spacing: 0.12em; text-transform: uppercase; color: #4ade80; }
	.card-id { margin-left: 0.35rem; color: #86efac; }
	.card-conf { font-size: 0.82rem; font-weight: 700; color: #34d399; text-transform: lowercase; }
	.card-name { margin-top: 0.65rem; font-size: 1rem; font-weight: 700; line-height: 1.35; color: #f8fafc; }
	.record-browser-card.selected .card-name { color: #ffffff; }
	.card-desc { margin-top: 0.45rem; font-size: 0.9rem; color: #d1fae5; }
	.card-foot { margin-top: 0.85rem; }

	.chip { display: inline-flex; align-items: center; padding: 0.38rem 0.68rem; border: 1px solid rgba(74, 222, 128, 0.22); background: rgba(21, 28, 40, 0.9); font-size: 0.78rem; letter-spacing: 0.06em; color: #4ade80; }
	.chip-quiet { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }

	.right-panel { display: flex; min-height: 0; flex-direction: column; padding: 1rem; }
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

	.topic-sidebar-shell { position: relative; flex: 0 0 auto; height: 100%; }
	.topic-sidebar { width: 100%; height: 100%; min-width: 0; padding: 1rem 1.25rem 1rem 1rem; border-right: 1px solid rgba(148, 163, 184, 0.14); background: rgba(15, 23, 42, 0.82); overflow: auto; }
	.topic-sidebar-title, .topic-sidebar-label { font-size: 0.72rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
	.topic-sidebar-title { margin-bottom: 0.85rem; }
	.topic-sidebar-block + .topic-sidebar-block { margin-top: 1rem; padding-top: 1rem; border-top: 1px solid rgba(148, 163, 184, 0.1); }
	.topic-sidebar-row { display: grid; grid-template-columns: 76px minmax(0, 1fr); gap: 0.6rem; margin-bottom: 0.6rem; align-items: start; }
	.topic-sidebar-row span { font-size: 0.76rem; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); }
	.topic-sidebar-row strong, .topic-sidebar-copy { min-width: 0; overflow-wrap: anywhere; word-break: break-word; }

	.topic-sidebar-resizer { position: absolute; top: 0; right: 0; bottom: 0; display: flex; align-items: center; justify-content: center; width: 16px; min-height: 120px; padding: 0; border: 0; background: transparent; cursor: col-resize; user-select: none; touch-action: none; outline: none; z-index: 4; }
	.topic-sidebar-resizer::before { content: ''; width: 1px; height: 100%; background: var(--ink-line); opacity: 0.8; transition: background 150ms ease; }
	.topic-sidebar-resizer:hover::before, .topic-sidebar-resizer.active::before, .topic-sidebar-resizer:focus-visible::before { background: #22c55e; }
	.topic-sidebar-resizer-grip { position: absolute; width: 8px; height: 52px; border-radius: 999px; background: radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y, var(--panel-bg); border: 1px solid var(--ink-line-soft); box-shadow: 0 0 0 2px rgba(0,0,0,0.14); transition: border-color 150ms ease, background-color 150ms ease; }
	.topic-sidebar-resizer:hover .topic-sidebar-resizer-grip, .topic-sidebar-resizer.active .topic-sidebar-resizer-grip, .topic-sidebar-resizer:focus-visible .topic-sidebar-resizer-grip { border-color: #22c55e; background: radial-gradient(circle, #22c55e 22%, transparent 24%) center 6px / 6px 12px repeat-y, var(--panel-bg); }

	.topic-sidebar-copy { margin-top: 0.55rem; font-size: 0.88rem; line-height: 1.6; color: var(--text); }
	.topic-sidebar-copy.muted, .topic-sidebar-empty { color: var(--muted); }
	.topic-sidebar-empty { font-size: 0.9rem; line-height: 1.6; }

	.keyword-list { display: flex; flex-wrap: wrap; gap: 0.45rem; margin-top: 0.6rem; }
	.keyword-chip { display: inline-flex; align-items: center; padding: 0.32rem 0.58rem; border-radius: 999px; border: 1px solid rgba(74, 222, 128, 0.22); background: rgba(13, 148, 136, 0.12); font-size: 0.78rem; color: #6ee7b7; }

	.pdf-fallback-frame { margin-top: 0.85rem; width: 100%; min-height: 540px; flex: 1; border: 1px solid rgba(148, 163, 184, 0.14); border-radius: 16px; background: white; }

	:global(.pdf-highlight) { position: absolute; background: rgba(34, 197, 94, 0.25); outline: 1px solid rgba(34, 197, 94, 0.8); border-radius: 2px; }

	@media (max-width: 980px) {
		.workspace, .detail-grid { grid-template-columns: minmax(0, 1fr); }
		.topic-sidebar-resizer { display: none; }
	}
</style>
