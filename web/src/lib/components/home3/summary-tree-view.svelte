<script lang="ts">
	import { onMount } from 'svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import PanelsTopLeftIcon from '@lucide/svelte/icons/panels-top-left';
	import Rows3Icon from '@lucide/svelte/icons/rows-3';
	import {
		getKbInput,
		getRecordSummaries,
		searchSummaryTree,
		type KbInputRecord
	} from '$lib/services/kbService';
	import KbInputSearchDialog from './kb-input-search-dialog.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import ViewToolbar from './view-toolbar.svelte';
	import {
		createSummaryTreeState,
		selectRecordSummaryTarget,
		selectSummaryTreeRecord,
		toggleSummaryTreeListMode
	} from './summary-tree-state.js';
	import type { SummaryRecordTarget, SummaryTreeRecord } from './summary-types';
	import { knowledgeStoreState } from './knowledge-store-state.svelte';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#818cf8' : '#4f46e5');

	let searchOpen = $state(false);
	let recordIdInput = $state('');
	let results = $state<SummaryTreeRecord[]>([]);
	let loading = $state(true);
	let loadError = $state('');
	let errorDialogOpen = $state(false);
	let summaryLoadingByRecordId = $state<Record<number, boolean>>({});
	let treeState = $state(createSummaryTreeState());
	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);

	onMount(async () => {
		await runSearch();
	});

	let activeRecord = $derived(results.find((record) => record.id === treeState.selectedRecordId) ?? results[0] ?? null);
	let selectedSummary = $derived(
		activeRecord?.summaries.find((summary) => summary.id === treeState.selectedSummaryId) ?? null
	);
	let viewerInputId = $derived(treeState.selectedPdfTarget?.inputId ?? activeRecord?.id ?? null);
	let viewerFileUrl = $derived(
		viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : ''
	);
	let viewerIsPdf = $derived(
		(activeRecord?.fileName ?? '').trim().toLowerCase().endsWith('.pdf')
	);

	type SummaryPdfViewport = {
		convertToViewportRectangle: (rect: number[]) => number[];
	};

	function recordDisplayName(record: SummaryTreeRecord) {
		return record.title?.trim() || `Record #${record.id}`;
	}

	function recordDisplayDocNo(record: SummaryTreeRecord) {
		return record.docNo?.trim() || '—';
	}

	function formatRecordTime(value: string) {
		return value?.trim() || '—';
	}

	function formatCoords(coords: number[]) {
		if (!Array.isArray(coords) || coords.length < 4) return '—';
		return `[${coords.map((n) => Math.trunc(n)).join(', ')}]`;
	}

	function formatTargets(targets: SummaryRecordTarget[]) {
		if (!Array.isArray(targets) || targets.length === 0) return '—';
		return targets
			.filter((target) => Array.isArray(target.coords) && target.coords.length >= 4)
			.map((target) => `p.${target.page} ${formatCoords(target.coords)}`)
			.join('\n');
	}

	function renderSummaryHighlight(pageNo: number, viewport: SummaryPdfViewport, overlay: HTMLDivElement) {
		if (!selectedSummary) return;
		const targets = selectedSummary.targets?.filter(
			(target) => target.page === pageNo && Array.isArray(target.coords) && target.coords.length >= 4
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

	function summaryTreeStatusText(record: KbInputRecord): string {
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

	function mapKbInputToSummaryTreeRecord(record: KbInputRecord): SummaryTreeRecord {
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
			procStatus: summaryTreeStatusText(record),
			createTime: record.create_time ? new Date(record.create_time).toLocaleString() : '—',
			modifyTime: record.modify_time ? new Date(record.modify_time).toLocaleString() : '—',
			summaries: []
		};
	}

	$effect(() => {
		if (!activeRecord && results[0]) {
			treeState = selectSummaryTreeRecord(treeState, results[0].id);
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
			void ensureRecordSummaries(activeRecord.id);
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
				results = [mapKbInputToSummaryTreeRecord(response.record)];
				treeState = createSummaryTreeState();
				treeState = selectSummaryTreeRecord(treeState, response.record.id);
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
			results = response.results;
			treeState = createSummaryTreeState();
			if (response.results[0]) {
				treeState = selectSummaryTreeRecord(treeState, response.results[0].id);
			}
		} catch (error) {
			results = [];
			treeState = createSummaryTreeState();
			loadError =
				error instanceof Error ? error.message : 'Failed to load summary tree records';
			errorDialogOpen = true;
		} finally {
			loading = false;
		}
	}

	async function ensureRecordSummaries(recordId: number) {
		const current = results.find((record) => record.id === recordId);
		if (!current || current.summaries.length > 0 || summaryLoadingByRecordId[recordId]) return;

		summaryLoadingByRecordId = { ...summaryLoadingByRecordId, [recordId]: true };
		try {
			const response = await getRecordSummaries(recordId);
			results = results.map((record) =>
				record.id === recordId
					? {
							...record,
							summaries: response.summaries.map((summary) => ({ ...summary, recordId }))
						}
					: record
			);
			if (treeState.selectedRecordId === recordId && response.summaries[0] && !treeState.selectedPdfTarget) {
				treeState = selectRecordSummaryTarget(treeState, {
					recordId,
					summaryId: response.summaries[0].id,
					inputId: response.summaries[0].inputId,
					page: response.summaries[0].page
				});
			}
		} catch (error) {
			loadError =
				error instanceof Error ? error.message : `Failed to load summaries for record ${recordId}`;
			errorDialogOpen = true;
		} finally {
			summaryLoadingByRecordId = { ...summaryLoadingByRecordId, [recordId]: false };
		}
	}
</script>

<div
	class="tree-shell"
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --panel-bg:${panelBg}; --panel-bg-alt:${panelAlt}; --ink-line:${border}; --ink-line-soft:rgba(148, 163, 184, 0.16); --text-primary:${textMain}; --text-secondary:${textMuted}; --text-muted:${textMuted}; --brass:#d8a74b; --crimson:#fca5a5; --font-mono:ui-monospace, SFMono-Regular, Menlo, monospace; --font-serif:\"Iowan Old Style\", \"Palatino Linotype\", \"Book Antiqua\", Georgia, serif;`}
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
			onkeydown={(event) => {
				if (event.key === 'Escape') errorDialogOpen = false;
			}}
		>
			<div
				class="error-dialog"
				role="dialog"
				aria-modal="true"
				aria-label="Summary Tree Load Error"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				<div class="eyebrow">Load Error</div>
				<h3>Could not load Summary Tree data</h3>
				<p class="dialog-copy">{loadError}</p>
				<div class="dialog-actions">
					<button type="button" class="ghost" onclick={() => (errorDialogOpen = false)}>Close</button>
					<button
						type="button"
						class="primary"
						onclick={async () => {
							errorDialogOpen = false;
							await runSearch();
						}}
					>
						Try Again
					</button>
				</div>
			</div>
		</div>
	{/if}

	<div class="hero">
		<div>
			<div class="eyebrow">Document Summaries</div>
			<h2>Summary Tree</h2>
			<p>Document-centric browser over <code>kb.inputs</code> with a search area modeled after Document Details.</p>
		</div>
		<div class="hero-actions">
			<button type="button" class="ghost" onclick={() => (treeState = toggleSummaryTreeListMode(treeState))}>
				{#if treeState.listMode === 'compact'}
					<PanelsTopLeftIcon class="h-4 w-4" />
					Expanded View
				{:else}
					<Rows3Icon class="h-4 w-4" />
					Document Details View
				{/if}
			</button>
			<button type="button" class="primary" onclick={() => (searchOpen = true)}>
				<SearchIcon class="h-4 w-4" />
				Search
			</button>
		</div>
	</div>

	<ViewToolbar {darkMode} />

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
							onkeydown={(e) => {
								if (e.key === 'Enter') void runSearch();
							}}
						/>
						<button type="button" class="ghost search-btn" onclick={() => (searchOpen = true)} title="Search records from kb.inputs">
							<SearchIcon class="h-4 w-4" />
							Search
						</button>
					</div>
				</label>

				<button type="button" class="retrieve-btn" onclick={runSearch} disabled={loading}>
					{#if loading}
						Loading…
					{:else}
						<span class="retrieve-arrow">→</span>
						Retrieve
					{/if}
				</button>
			</div>

			<div class="left-meta">
				<div class="left-meta-title">kb.inputs</div>
				<div class="left-meta-count">{results.length} found</div>
			</div>

			<div class="result-list summary-tree-results">
				{#if loading}
					<div class="empty-state">Loading records…</div>
				{:else if loadError}
					<div class="empty-state">Summary Tree could not be loaded. Open the error dialog for details or try again.</div>
				{:else if results.length === 0}
					<div class="empty-state">No summary records found.</div>
				{:else if treeState.listMode === 'compact'}
					{#each results as record, idx (record.id)}
						<button
							type="button"
							class="record-browser-card"
							class:selected={treeState.selectedRecordId === record.id}
							onclick={() => (treeState = selectSummaryTreeRecord(treeState, record.id))}
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
							onclick={() => (treeState = selectSummaryTreeRecord(treeState, record.id))}
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
								{record.summaries[0]?.summaryText ??
									(treeState.selectedRecordId === record.id && summaryLoadingByRecordId[record.id]
										? 'Loading summary preview…'
										: 'Select this record to load its summaries.')}
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
						<div class="summary-snippets">
								{#each activeRecord.summaries as summary}
								<button
									type="button"
									class:selected={treeState.selectedSummaryId === summary.id}
									class="snippet"
									onclick={() => (treeState = selectRecordSummaryTarget(treeState, {
										recordId: activeRecord.id,
										summaryId: summary.id,
										inputId: summary.inputId,
										page: summary.page
									}))}
								>
									<div class="snippet-head">
										<strong>{summary.id}</strong>
										<span>p.{summary.page}</span>
									</div>
									<p>{summary.summaryText}</p>
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
								highlightVersion={selectedSummary
									? `${selectedSummary.id}:${selectedSummary.targets.map((target) => `${target.page}:${target.coords.join(',')}`).join('|')}`
									: 'summary-tree'}
								renderHighlights={renderSummaryHighlight}
								sidebarMinWidth={240}
								sidebarMaxWidth={520}
								sidebarDefaultWidth={320}
							>
								{#snippet sidebar()}
									<aside class="summary-sidebar">
										<div class="summary-sidebar-title">Selected Summary</div>
										{#if selectedSummary}
											<div class="summary-sidebar-block">
												<div class="summary-sidebar-row">
													<span>Summary ID</span>
													<strong>{selectedSummary.id}</strong>
												</div>
												<div class="summary-sidebar-row">
													<span>Record ID</span>
													<strong>{selectedSummary.recordId}</strong>
												</div>
												<div class="summary-sidebar-row">
													<span>Page</span>
													<strong>{selectedSummary.page}</strong>
												</div>
												<div class="summary-sidebar-row">
													<span>PDF</span>
													<strong title={selectedSummary.pdfFileName}>{selectedSummary.pdfFileName}</strong>
												</div>
												<div class="summary-sidebar-row">
													<span>Coords</span>
													<strong class="coords-block">{formatTargets(selectedSummary.targets)}</strong>
												</div>
											</div>

											<div class="summary-sidebar-block">
												<div class="summary-sidebar-label">Summary</div>
												<p class="summary-sidebar-copy">{selectedSummary.summaryText}</p>
											</div>

											<div class="summary-sidebar-block">
												<div class="summary-sidebar-label">Keywords</div>
												{#if selectedSummary.keywords.length > 0}
													<div class="keyword-list">
														{#each selectedSummary.keywords as keyword (keyword)}
															<span class="keyword-chip">{keyword}</span>
														{/each}
													</div>
												{:else}
													<p class="summary-sidebar-copy muted">No keywords were extracted for this summary.</p>
												{/if}
											</div>
										{:else}
											<div class="summary-sidebar-empty">Select a summary card to inspect it alongside the source PDF.</div>
										{/if}
									</aside>
								{/snippet}
							</PdfViewWindow>
						{:else if viewerInputId && treeState.selectedPdfTarget}
							<iframe
								class="pdf-fallback-frame"
								title={activeRecord.fileName}
								src={viewerFileUrl}
							></iframe>
						{:else if summaryLoadingByRecordId[activeRecord.id]}
							<div class="empty-state pdf-empty">Loading summaries for the selected record…</div>
						{:else if activeRecord.summaries.length === 0}
							<div class="empty-state pdf-empty">No summaries are available for this record yet.</div>
						{:else}
							<div class="empty-state pdf-empty">
								Select a summary to move the PDF display to the relevant page.
							</div>
						{/if}
					</div>
				</div>
			{:else}
				<div class="empty-state">Select a record from the left panel to inspect its summaries.</div>
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

	.dialog-copy {
		margin-top: 0.55rem;
		color: var(--muted);
	}

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
			radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 42%),
			linear-gradient(180deg, rgba(15, 23, 42, 0.86), rgba(15, 23, 42, 0.66));
		border: 1px solid var(--border);
	}

	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}

	h2,
	h3,
	p {
		margin: 0;
	}

	h2 {
		margin-top: 0.3rem;
		font-size: 1.5rem;
	}

	.hero p {
		margin-top: 0.45rem;
		max-width: 52rem;
		color: var(--muted);
	}

	.hero-actions {
		display: flex;
		gap: 0.75rem;
	}

	.ghost,
	.primary {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		border-radius: 14px;
		padding: 0.8rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.16);
		cursor: pointer;
	}

	.ghost {
		background: rgba(15, 23, 42, 0.36);
		color: var(--text);
	}

	.primary {
		background: linear-gradient(135deg, #2563eb, #4f46e5);
		border-color: transparent;
		color: white;
		font-weight: 700;
	}

	.workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: 360px minmax(0, 1fr);
		gap: 1rem;
	}

	.left-panel,
	.right-panel {
		min-height: 0;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
	}

	.left-panel {
		display: flex;
		flex-direction: column;
		padding: 0;
		overflow: hidden;
	}

	.left-controls {
		padding: 1rem;
		border-bottom: 1px solid rgba(148, 163, 184, 0.08);
	}

	.field {
		display: block;
	}

	.field-label {
		display: block;
		margin-bottom: 0.6rem;
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
	}

	.field-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 0.75rem;
	}

	.field-row input {
		width: 100%;
		min-width: 0;
		border-radius: 0;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(31, 41, 55, 0.9);
		padding: 0.95rem 1rem;
		font-size: 0.95rem;
		color: #f3f4f6;
	}

	.search-btn {
		padding-inline: 1rem;
	}

	.retrieve-btn {
		margin-top: 1rem;
		width: 100%;
		border: none;
		background: #d8a74b;
		color: #111827;
		padding: 0.95rem 1rem;
		font-size: 1rem;
		font-weight: 700;
		cursor: pointer;
	}

	.retrieve-btn:disabled {
		opacity: 0.7;
		cursor: default;
	}

	.retrieve-arrow {
		margin-right: 0.45rem;
	}

	.result-list {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		overflow: auto;
		padding: 0 1rem 1rem;
	}

	.summary-tree-results {
		gap: 0.8rem;
	}

	.left-meta {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		padding: 1rem;
		border-bottom: 1px solid rgba(148, 163, 184, 0.08);
	}

	.left-meta-title {
		font-size: 0.95rem;
		font-weight: 700;
		color: #f5e8c8;
	}

	.left-meta-count {
		font-size: 0.78rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: #b8a57a;
	}

	.record-card {
		margin-bottom: 0.75rem;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.28);
		padding: 0.9rem;
		text-align: left;
		color: inherit;
	}

	.record-card.selected,
	.record-card:hover {
		border-color: rgba(129, 140, 248, 0.34);
		background: rgba(99, 102, 241, 0.1);
	}

	.record-head,
	.snippet-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.record-title {
		font-size: 0.94rem;
		font-weight: 700;
	}

	.record-subtitle {
		margin-top: 0.15rem;
		font-size: 0.8rem;
		color: var(--muted);
	}

	.record-meta,
	.detail-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin: 0.75rem 0;
	}

	.record-meta span,
	.record-status,
	.detail-meta span {
		border-radius: 999px;
		padding: 0.25rem 0.55rem;
		background: rgba(148, 163, 184, 0.12);
		font-size: 0.72rem;
		color: var(--muted);
	}

	.record-browser-card {
		position: relative;
		width: 100%;
		border: 1px solid rgba(148, 163, 184, 0.12);
		background: rgba(31, 41, 55, 0.72);
		color: inherit;
		text-align: left;
		cursor: pointer;
		overflow: hidden;
	}

	.record-browser-card:hover {
		background: rgba(39, 51, 67, 0.84);
	}

	.record-browser-card.selected {
		border-color: rgba(129, 140, 248, 0.38);
		background: rgba(49, 58, 82, 0.9);
	}

	.card-rule {
		position: absolute;
		left: 0;
		top: 0;
		bottom: 0;
		width: 6px;
		background: rgba(129, 140, 248, 0.18);
	}

	.record-browser-card.selected .card-rule {
		background: #818cf8;
	}

	.card-body {
		padding: 1rem 1rem 1rem 1.25rem;
	}

	.card-row-top {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.card-index {
		font-size: 0.74rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: #b8a57a;
	}

	.card-id {
		margin-left: 0.35rem;
		color: #e6c06d;
	}

	.card-conf {
		font-size: 0.82rem;
		font-weight: 700;
		color: #72d5d0;
		text-transform: lowercase;
	}

	.card-name {
		margin-top: 0.65rem;
		font-size: 1rem;
		font-weight: 700;
		line-height: 1.35;
		color: #f8fafc;
	}

	.record-browser-card.selected .card-name {
		color: #ffffff;
	}

	.card-desc {
		margin-top: 0.45rem;
		font-size: 0.9rem;
		color: #e5dec9;
	}

	.card-foot {
		margin-top: 0.85rem;
	}

	.chip {
		display: inline-flex;
		align-items: center;
		padding: 0.38rem 0.68rem;
		border: 1px solid rgba(184, 165, 122, 0.22);
		background: rgba(21, 28, 40, 0.9);
		font-size: 0.78rem;
		letter-spacing: 0.06em;
		color: #b8a57a;
	}

	.chip-quiet {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	}

	.right-panel {
		display: flex;
		min-height: 0;
		flex-direction: column;
		padding: 1rem;
	}

	.right-tabs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
		margin-bottom: 1rem;
	}

	.tab {
		border-radius: 14px;
		padding: 0.72rem 0.9rem;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.3);
		font-weight: 700;
	}

	.tab.active {
		border-color: rgba(59, 130, 246, 0.36);
		background: rgba(37, 99, 235, 0.16);
		color: #bfdbfe;
	}

	.tab.passive {
		color: var(--muted);
	}

	.detail-grid {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: 340px minmax(0, 1fr);
		gap: 1rem;
	}

	.detail-card,
	.pdf-card {
		display: flex;
		flex-direction: column;
		min-height: 0;
		border-radius: 20px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.26);
		padding: 1rem;
	}

	.summary-snippets {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		gap: 0.75rem;
		margin-top: 1rem;
		overflow: auto;
		padding-right: 0.25rem;
	}

	.snippet {
		position: relative;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.34);
		padding: 0.85rem;
		text-align: left;
		color: inherit;
		transition:
			border-color 150ms ease,
			background 150ms ease,
			box-shadow 150ms ease,
			transform 150ms ease;
	}

	.snippet::before {
		content: '';
		position: absolute;
		left: 0;
		top: 0;
		bottom: 0;
		width: 4px;
		border-radius: 16px 0 0 16px;
		background: transparent;
		transition: background 150ms ease;
	}

	.snippet:hover {
		border-color: rgba(59, 130, 246, 0.34);
	}

	.snippet.selected {
		border-color: rgba(96, 165, 250, 0.58);
		background:
			linear-gradient(180deg, rgba(37, 99, 235, 0.14), rgba(37, 99, 235, 0.06)),
			rgba(15, 23, 42, 0.9);
		box-shadow:
			0 0 0 1px rgba(96, 165, 250, 0.18) inset,
			0 16px 36px rgba(15, 23, 42, 0.24);
		transform: translateY(-1px);
	}

	.snippet.selected::before {
		background: linear-gradient(180deg, #60a5fa, #818cf8);
	}

	.snippet p {
		margin-top: 0.5rem;
		font-size: 0.84rem;
		color: var(--muted);
		line-height: 1.5;
	}

	.snippet.selected p {
		color: #dbeafe;
	}

	.empty-state {
		display: flex;
		flex: 1;
		align-items: center;
		justify-content: center;
		border-radius: 18px;
		border: 1px dashed rgba(148, 163, 184, 0.2);
		background: rgba(2, 6, 23, 0.28);
		padding: 1rem;
		text-align: center;
		color: var(--muted);
	}

	.pdf-card :global(.doc-page-bar) {
		margin-top: 0.85rem;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 16px 16px 0 0;
	}

	.pdf-card :global(.pdf-stage) {
		margin-top: -1px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 0 0 16px 16px;
		background: rgba(2, 6, 23, 0.28);
	}

	.summary-sidebar {
		width: 100%;
		height: 100%;
		min-width: 0;
		padding: 1rem 1.25rem 1rem 1rem;
		border-right: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.82);
		overflow: auto;
	}

	.summary-sidebar-title,
	.summary-sidebar-label {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
	}

	.summary-sidebar-title {
		margin-bottom: 0.85rem;
	}

	.summary-sidebar-block + .summary-sidebar-block {
		margin-top: 1rem;
		padding-top: 1rem;
		border-top: 1px solid rgba(148, 163, 184, 0.1);
	}

	.summary-sidebar-row {
		display: grid;
		grid-template-columns: 76px minmax(0, 1fr);
		gap: 0.6rem;
		margin-bottom: 0.6rem;
		align-items: start;
	}

	.summary-sidebar-row span {
		font-size: 0.76rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--muted);
	}

	.summary-sidebar-row strong,
	.summary-sidebar-copy {
		min-width: 0;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.coords-block {
		white-space: pre-wrap;
		font-family: var(--font-mono);
		font-size: 0.8rem;
		line-height: 1.45;
	}

	.summary-sidebar-copy {
		margin-top: 0.55rem;
		font-size: 0.88rem;
		line-height: 1.6;
		color: var(--text);
	}

	.summary-sidebar-copy.muted,
	.summary-sidebar-empty {
		color: var(--muted);
	}

	.summary-sidebar-empty {
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
		border: 1px solid rgba(96, 165, 250, 0.22);
		background: rgba(37, 99, 235, 0.12);
		font-size: 0.78rem;
		color: #c7d2fe;
	}

	.pdf-fallback-frame {
		margin-top: 0.85rem;
		width: 100%;
		min-height: 540px;
		flex: 1;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 16px;
		background: white;
	}

	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(212, 162, 76, 0.3);
		outline: 1px solid rgba(212, 162, 76, 0.85);
		border-radius: 2px;
	}

	@media (max-width: 980px) {
		.workspace,
		.detail-grid {
			grid-template-columns: minmax(0, 1fr);
		}

	}
</style>
