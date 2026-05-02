<script lang="ts">
	import { onMount } from 'svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import PanelsTopLeftIcon from '@lucide/svelte/icons/panels-top-left';
	import Rows3Icon from '@lucide/svelte/icons/rows-3';
	import SummaryTreeSearchDialog from './summary-tree-search-dialog.svelte';
	import type { SummaryTreeSearchFields } from './summary-tree-search-dialog.svelte';
	import { searchSummaryTreeMock } from '$lib/services/kbService';
	import {
		createSummaryTreeState,
		selectRecordSummaryTarget,
		selectSummaryTreeRecord,
		toggleSummaryTreeListMode
	} from './summary-tree-state.js';
	import type { SummaryTreeRecord } from './summary-types';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#818cf8' : '#4f46e5');

	let searchOpen = $state(false);
	let searchFields = $state<SummaryTreeSearchFields>({
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
		modifyEnd: ''
	});
	let results = $state<SummaryTreeRecord[]>([]);
	let loading = $state(true);
	let treeState = $state(createSummaryTreeState());

	onMount(async () => {
		await runSearch();
	});

	let activeRecord = $derived(results.find((record) => record.id === treeState.selectedRecordId) ?? results[0] ?? null);

	$effect(() => {
		if (!activeRecord && results[0]) {
			treeState = selectSummaryTreeRecord(treeState, results[0].id);
		}
	});

	async function runSearch() {
		loading = true;
		const response = await searchSummaryTreeMock(searchFields);
		results = response.results;
		if (response.results[0]) {
			treeState = selectSummaryTreeRecord(treeState, response.results[0].id);
		}
		loading = false;
	}

	function resetSearch() {
		searchFields = {
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
			modifyEnd: ''
		};
	}
</script>

<div class="tree-shell" style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent};`}>
	<SummaryTreeSearchDialog bind:open={searchOpen} bind:fields={searchFields} onSearch={runSearch} onReset={resetSearch} />

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
					Card View
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
			<div class="search-summary">
				<div class="eyebrow">Search Area</div>
				<div class="chips">
					<span>{searchFields.docType === 'all' ? 'All Types' : searchFields.docType}</span>
					<span>{searchFields.procStatus === 'all' ? 'All Statuses' : searchFields.procStatus}</span>
					<span>{searchFields.title || searchFields.fileName || 'No text filter'}</span>
				</div>
				<button type="button" class="ghost wide" onclick={() => (searchOpen = true)}>Refine Search</button>
			</div>

			<div class="result-list">
				<div class="list-head">
					<h3>List</h3>
					<span>{results.length} records</span>
				</div>

				{#if loading}
					<div class="empty-state">Loading mocked records…</div>
				{:else if results.length === 0}
					<div class="empty-state">No summary records found.</div>
				{:else}
					{#each results as record}
						<button
							type="button"
							class:selected={treeState.selectedRecordId === record.id}
							class:compact={treeState.listMode === 'compact'}
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
							{#if treeState.listMode === 'cards'}
								<div class="record-meta">
									<span>{record.docNo}</span>
									<span>{record.parserName}</span>
								</div>
								<p>{record.summaries[0]?.summaryText ?? 'No summary text'}</p>
							{/if}
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
										<strong>{summary.pdfFileName}</strong>
										<span>p.{summary.page}</span>
									</div>
									<p>{summary.summaryText}</p>
								</button>
							{/each}
						</div>
					</div>

					<div class="pdf-card">
						<div class="eyebrow">PDF Display</div>
						{#if treeState.selectedPdfTarget}
							<div class="mock-sheet">
								<div class="sheet-title">Mocked PDF Target</div>
								<div class="sheet-grid">
									<div><span>Input ID</span><strong>{treeState.selectedPdfTarget.inputId}</strong></div>
									<div><span>Page</span><strong>{treeState.selectedPdfTarget.page}</strong></div>
									<div><span>Summary</span><strong>{treeState.selectedPdfTarget.summaryId}</strong></div>
									<div><span>Mode</span><strong>Phase 1 Mock</strong></div>
								</div>
							</div>
						{:else}
							<div class="empty-state pdf-empty">
								Select a summary to move the mocked PDF display to the relevant page.
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

	.wide {
		width: 100%;
		justify-content: center;
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
		padding: 1rem;
		gap: 1rem;
	}

	.search-summary {
		border-radius: 18px;
		background: rgba(15, 23, 42, 0.4);
		padding: 1rem;
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin: 0.8rem 0;
	}

	.chips span,
	.record-meta span,
	.record-status,
	.detail-meta span {
		border-radius: 999px;
		padding: 0.25rem 0.55rem;
		background: rgba(148, 163, 184, 0.12);
		font-size: 0.72rem;
		color: var(--muted);
	}

	.result-list {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		overflow: hidden;
	}

	.list-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.8rem;
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

	.record-card.compact p,
	.record-card.compact .record-meta {
		display: none;
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
		min-height: 0;
		border-radius: 20px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.26);
		padding: 1rem;
	}

	.summary-snippets {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.snippet {
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.34);
		padding: 0.85rem;
		text-align: left;
		color: inherit;
	}

	.snippet.selected,
	.snippet:hover {
		border-color: rgba(59, 130, 246, 0.34);
	}

	.snippet p {
		margin-top: 0.5rem;
		font-size: 0.84rem;
		color: var(--muted);
		line-height: 1.5;
	}

	.mock-sheet,
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

	.mock-sheet {
		max-width: 420px;
		margin: 1rem auto 0;
		border-style: solid;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(241, 245, 249, 0.92));
		color: #0f172a;
		box-shadow: 0 22px 50px rgba(15, 23, 42, 0.22);
	}

	.sheet-title {
		margin-bottom: 1rem;
		font-size: 1rem;
		font-weight: 800;
	}

	.sheet-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
		text-align: left;
	}

	.sheet-grid span {
		display: block;
		margin-bottom: 0.2rem;
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: #64748b;
	}

	@media (max-width: 980px) {
		.workspace,
		.detail-grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
