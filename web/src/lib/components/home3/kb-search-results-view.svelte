<script lang="ts">
	import { goto } from '$app/navigation';
	import SearchIcon from '@lucide/svelte/icons/search';
	import XIcon from '@lucide/svelte/icons/x';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import {
		searchKbArtifacts,
		type KbSearchArtifactType,
		type KbSearchResponse,
		type KbSearchResult
	} from '$lib/services/kbArtifactSearch';
	import { kbSearchArtifactOptions } from '$lib/components/home3/kb-search-lab-state';
	import {
		buildKbSearchPaginationItems,
		clampKbSearchPage
	} from '$lib/components/home3/kb-search-pagination';

	let {
		darkMode = true,
		initialQuery = '',
		initialPage = 1
	}: { darkMode: boolean; initialQuery?: string; initialPage?: number } = $props();

	function normalizeRequestedPage(page: number): number {
		return Number.isFinite(page) ? Math.max(1, Math.trunc(page)) : 1;
	}

	const PAGE_SIZE = 20;
	const artifactOptions = kbSearchArtifactOptions.map((option) => ({
		...option,
		label: option.value === 'all' ? 'All' : option.label
	}));

	let query = $state(initialQuery.trim());
	let submittedQuery = $state(initialQuery.trim());
	let artifactType = $state<KbSearchArtifactType>('all');
	let pageNumber = $state(normalizeRequestedPage(initialPage));
	let loading = $state(false);
	let error = $state('');
	let payload = $state<KbSearchResponse | null>(null);
	let lastRequestKey = '';
	let didInitialSearch = false;

	let total = $derived(payload?.total ?? 0);
	let results = $derived(payload?.results ?? []);
	let totalPages = $derived(Math.max(1, Math.ceil(total / PAGE_SIZE)));
	let firstResult = $derived(total === 0 ? 0 : (pageNumber - 1) * PAGE_SIZE + 1);
	let lastResult = $derived(Math.min(pageNumber * PAGE_SIZE, total));
	let hasQuery = $derived(submittedQuery.trim().length > 0);
	let paginationItems = $derived(buildKbSearchPaginationItems(pageNumber, totalPages));

	let pageBg = $derived(darkMode ? 'oklch(16% 0.013 250)' : 'oklch(96% 0.012 84)');
	let surface = $derived(darkMode ? 'oklch(20% 0.018 250)' : 'oklch(99% 0.006 84)');
	let surfaceSoft = $derived(darkMode ? 'oklch(24% 0.02 250)' : 'oklch(94% 0.012 84)');
	let border = $derived(darkMode ? 'oklch(31% 0.026 250)' : 'oklch(84% 0.018 84)');
	let borderStrong = $derived(darkMode ? 'oklch(42% 0.04 250)' : 'oklch(74% 0.024 84)');
	let textPrimary = $derived(darkMode ? 'oklch(92% 0.01 250)' : 'oklch(20% 0.018 250)');
	let textSecondary = $derived(darkMode ? 'oklch(72% 0.022 250)' : 'oklch(45% 0.026 250)');
	let textMuted = $derived(darkMode ? 'oklch(56% 0.027 250)' : 'oklch(57% 0.025 250)');
	let accent = $derived(darkMode ? 'oklch(74% 0.16 190)' : 'oklch(58% 0.15 190)');
	let accentSoft = $derived(darkMode ? 'oklch(31% 0.055 190)' : 'oklch(92% 0.045 190)');
	let danger = $derived(darkMode ? 'oklch(72% 0.15 25)' : 'oklch(55% 0.16 25)');

	function resultTitle(result: KbSearchResult, index: number): string {
		const candidate =
			result.primary_label ??
			result.metric_name ??
			result.topicText ??
			result.summaryText ??
			result.provisionText ??
			result.productName ??
			result.artifact_id ??
			`Result ${firstResult + index}`;
		return String(candidate).trim() || `Result ${firstResult + index}`;
	}

	function resultType(result: KbSearchResult): string {
		const raw = String(result.artifact_type ?? payload?.artifact_type ?? artifactType);
		return raw
			.replace(/-/g, ' ')
			.replace(/_/g, ' ')
			.replace(/\b\w/g, (letter) => letter.toUpperCase());
	}

	function resultSource(result: KbSearchResult): string {
		const source = result.source_title ?? result.source_filename ?? result.secondary_label;
		return source ? String(source) : 'SemOS KB';
	}

	function resultRecord(result: KbSearchResult): string {
		const recordId = result.input_record_id ?? result.inputId;
		if (!recordId) return '';
		return `record ${recordId}`;
	}

	function resultSnippet(result: KbSearchResult): string {
		const snippet =
			result.snippet ??
			result.search_document ??
			result.metric_desc ??
			result.metric_context ??
			result.summaryText ??
			result.topicText;
		return String(snippet ?? '').trim();
	}

	function formatScore(result: KbSearchResult): string {
		return typeof result.score === 'number' ? result.score.toFixed(3) : '';
	}

	function buildSearchUrl(q: string, nextPage = 1): string {
		const params = new URLSearchParams({ section: 'kb-search' });
		if (q.trim()) params.set('q', q.trim());
		if (nextPage > 1) params.set('page', String(nextPage));
		if (!darkMode) params.set('dark', '0');
		return `/home3/knowledge?${params.toString()}`;
	}

	async function fetchResults(nextQuery = submittedQuery, nextPage = pageNumber) {
		const trimmed = nextQuery.trim();
		if (!trimmed) {
			payload = null;
			error = '';
			return;
		}
		const requestKey = `${artifactType}:${trimmed}:${nextPage}`;
		lastRequestKey = requestKey;
		loading = true;
		error = '';
		try {
			const nextPayload = await searchKbArtifacts(artifactType, {
				q: trimmed,
				page: nextPage,
				pageSize: PAGE_SIZE
			});
			if (lastRequestKey === requestKey) {
				const resolvedTotalPages = Math.max(
					1,
					Math.ceil((nextPayload.total ?? nextPayload.results?.length ?? 0) / PAGE_SIZE)
				);
				const resolvedPage = clampKbSearchPage(nextPage, resolvedTotalPages);
				if (resolvedPage !== nextPage && (nextPayload.total ?? 0) > 0) {
					pageNumber = resolvedPage;
					goto(buildSearchUrl(trimmed, resolvedPage));
					void fetchResults(trimmed, resolvedPage);
					return;
				}
				pageNumber = resolvedPage;
				payload = nextPayload;
			}
		} catch (err) {
			if (lastRequestKey === requestKey) {
				error = err instanceof Error ? err.message : 'Search failed';
				payload = null;
			}
		} finally {
			if (lastRequestKey === requestKey) {
				loading = false;
			}
		}
	}

	function submitSearch(event: SubmitEvent) {
		event.preventDefault();
		const trimmed = query.trim();
		pageNumber = 1;
		submittedQuery = trimmed;
		goto(buildSearchUrl(trimmed, 1));
		void fetchResults(trimmed, 1);
	}

	function clearSearch() {
		query = '';
		submittedQuery = '';
		pageNumber = 1;
		payload = null;
		error = '';
		goto(buildSearchUrl('', 1));
	}

	function selectArtifactType(nextType: KbSearchArtifactType) {
		if (artifactType === nextType) return;
		artifactType = nextType;
		pageNumber = 1;
		goto(buildSearchUrl(submittedQuery, 1));
		void fetchResults(submittedQuery, 1);
	}

	function goToPage(nextPage: number) {
		if (nextPage < 1 || nextPage > totalPages || loading) return;
		pageNumber = nextPage;
		goto(buildSearchUrl(submittedQuery, nextPage));
		void fetchResults(submittedQuery, nextPage);
	}

	$effect(() => {
		const next = initialQuery.trim();
		const nextPage = normalizeRequestedPage(initialPage);
		if (!didInitialSearch) {
			didInitialSearch = true;
			pageNumber = nextPage;
			if (next) void fetchResults(next, nextPage);
			return;
		}
		if (next !== submittedQuery || nextPage !== pageNumber) {
			query = next;
			submittedQuery = next;
			pageNumber = nextPage;
			void fetchResults(next, nextPage);
		}
	});
</script>

{#snippet paginationControls(position: 'top' | 'bottom')}
	{#if totalPages > 1}
		<nav class="pagination {position}" aria-label="Search result pages">
			<button type="button" onclick={() => goToPage(pageNumber - 1)} disabled={pageNumber <= 1}>
				<ChevronLeftIcon size={18} />
				Previous
			</button>
			<div class="pagination-pages" aria-label="Page numbers">
				{#each paginationItems as item}
					{#if item === 'ellipsis'}
						<span class="pagination-ellipsis" aria-hidden="true">...</span>
					{:else}
						<button
							type="button"
							class:current={item === pageNumber}
							aria-current={item === pageNumber ? 'page' : undefined}
							aria-label={`Go to page ${item}`}
							onclick={() => goToPage(item)}
						>
							{item}
						</button>
					{/if}
				{/each}
			</div>
			<span class="pagination-summary">Page {pageNumber} of {totalPages}</span>
			<button
				type="button"
				onclick={() => goToPage(pageNumber + 1)}
				disabled={pageNumber >= totalPages}
			>
				Next
				<ChevronRightIcon size={18} />
			</button>
		</nav>
	{/if}
{/snippet}

<section
	class="search-page"
	style="--page-bg:{pageBg}; --surface:{surface}; --surface-soft:{surfaceSoft}; --border:{border}; --border-strong:{borderStrong}; --text-primary:{textPrimary}; --text-secondary:{textSecondary}; --text-muted:{textMuted}; --accent:{accent}; --accent-soft:{accentSoft}; --danger:{danger};"
>
	<div class="search-shell">
		<header class="search-header">
			<div>
				<p class="eyebrow">SemOS knowledge index</p>
				<h1>Search results</h1>
			</div>
			{#if payload && hasQuery}
				<p class="result-range">Results {firstResult} - {lastResult} of {total}</p>
			{/if}
		</header>

		<form class="search-form" role="search" onsubmit={submitSearch}>
			<SearchIcon class="search-icon" size={20} strokeWidth={1.8} />
			<input
				type="search"
				bind:value={query}
				placeholder="Search the knowledge base"
				aria-label="Search the knowledge base"
				autocomplete="off"
			/>
			{#if query}
				<button type="button" class="icon-button" onclick={clearSearch} aria-label="Clear search">
					<XIcon size={16} />
				</button>
			{/if}
			<button class="submit-button" type="submit" disabled={loading || !query.trim()}>
				{loading ? 'Searching' : 'Search'}
			</button>
		</form>

		<div class="scope-row" aria-label="Search scope">
			<span>Search in</span>
			<div class="scope-tabs">
				{#each artifactOptions as option}
					<button
						type="button"
						class:active={artifactType === option.value}
						onclick={() => selectArtifactType(option.value)}
					>
						{option.label}
					</button>
				{/each}
			</div>
		</div>

		{#if error}
			<div class="status error">{error}</div>
		{:else if loading}
			<div class="result-list" aria-label="Loading search results">
				{#each Array(5) as _, index}
					<div class="skeleton-result" aria-hidden="true">
						<span class="skeleton-title" style="width:{index === 0 ? '44%' : '32%'};"></span>
						<span class="skeleton-line"></span>
						<span class="skeleton-line short"></span>
					</div>
				{/each}
			</div>
		{:else if hasQuery && results.length > 0}
			<div class="suggestion">
				Showing results for <strong>{submittedQuery}</strong>. Refine by artifact type when you need a
				narrower slice of the knowledge base.
			</div>

			{@render paginationControls('top')}

			<div class="result-list" aria-label="Search results">
				{#each results as result, index}
					<article class="result-item">
						<div class="thumb" aria-hidden="true">{resultType(result).slice(0, 1)}</div>
						<div class="result-body">
							<a class="result-title" href={`/home3/knowledge?section=kb-search&q=${encodeURIComponent(submittedQuery)}`}>
								{resultTitle(result, index)}
							</a>
							<div class="meta-line">
								<span>{resultType(result)}</span>
								{#if resultRecord(result)}
									<span>{resultRecord(result)}</span>
								{/if}
								<span>{resultSource(result)}</span>
								{#if formatScore(result)}
									<span>score {formatScore(result)}</span>
								{/if}
							</div>
							{#if resultSnippet(result)}
								<p class="snippet">{resultSnippet(result)}</p>
							{/if}
						</div>
					</article>
				{/each}
			</div>

			{@render paginationControls('bottom')}
		{:else if hasQuery}
			<div class="empty-state">
				<h2>No results found</h2>
				<p>
					No indexed artifact matched <strong>{submittedQuery}</strong>. Try a shorter phrase,
					another artifact type, or a source document term.
				</p>
			</div>
		{:else}
			<div class="empty-state">
				<h2>Enter a query to search SemOS</h2>
				<p>Search across documents, topics, metrics, scenes, provisions, products, and summaries.</p>
			</div>
		{/if}
	</div>
</section>

<style>
	.search-page {
		min-height: 100%;
		overflow: auto;
		background:
			radial-gradient(circle at 52% 0%, color-mix(in oklch, var(--accent) 10%, transparent), transparent 34rem),
			var(--page-bg);
		color: var(--text-primary);
	}

	.search-shell {
		width: min(1120px, calc(100% - 48px));
		margin: 0 auto;
		padding: 48px 0 72px;
	}

	.search-header {
		display: flex;
		align-items: end;
		justify-content: space-between;
		gap: 24px;
		padding-bottom: 16px;
		border-bottom: 1px solid var(--border-strong);
	}

	.eyebrow {
		margin: 0 0 8px;
		color: var(--text-muted);
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.14em;
		text-transform: uppercase;
	}

	h1 {
		margin: 0;
		font-family: Georgia, 'Times New Roman', serif;
		font-size: 2.1rem;
		font-weight: 650;
		letter-spacing: 0;
	}

	.result-range {
		margin: 0 0 5px;
		color: var(--text-secondary);
		font-size: 0.9rem;
		white-space: nowrap;
	}

	.search-form {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto auto;
		align-items: center;
		margin-top: 22px;
		background: var(--surface);
		border: 1px solid var(--border-strong);
		border-radius: 8px;
		overflow: hidden;
	}

	.search-form :global(.search-icon) {
		margin-left: 14px;
		color: var(--text-muted);
	}

	.search-form input {
		min-width: 0;
		height: 46px;
		padding: 0 12px;
		background: transparent;
		border: 0;
		color: var(--text-primary);
		font-size: 0.95rem;
		outline: none;
	}

	.search-form input::placeholder {
		color: var(--text-muted);
	}

	.icon-button {
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		margin-right: 6px;
		border: 0;
		border-radius: 6px;
		background: transparent;
		color: var(--text-muted);
		cursor: pointer;
	}

	.icon-button:hover {
		background: var(--surface-soft);
		color: var(--text-primary);
	}

	.submit-button {
		height: 46px;
		padding: 0 22px;
		border: 0;
		background: var(--accent);
		color: oklch(16% 0.012 250);
		font-size: 0.9rem;
		font-weight: 800;
		cursor: pointer;
	}

	.submit-button:disabled {
		cursor: not-allowed;
		opacity: 0.55;
	}

	.scope-row {
		display: flex;
		align-items: center;
		gap: 12px;
		margin-top: 10px;
		padding: 8px 0 16px;
		border-bottom: 1px solid var(--border);
		color: var(--text-secondary);
		font-size: 0.86rem;
	}

	.scope-tabs {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.scope-tabs button {
		height: 30px;
		padding: 0 11px;
		border: 1px solid var(--border);
		border-radius: 999px;
		background: transparent;
		color: var(--text-secondary);
		font-size: 0.82rem;
		cursor: pointer;
	}

	.scope-tabs button:hover,
	.scope-tabs button.active {
		border-color: color-mix(in oklch, var(--accent) 65%, var(--border));
		background: var(--accent-soft);
		color: var(--text-primary);
	}

	.suggestion,
	.status {
		max-width: 75ch;
		margin-top: 22px;
		color: var(--text-secondary);
		font-size: 0.95rem;
		line-height: 1.55;
	}

	.status.error {
		padding: 16px 18px;
		background: color-mix(in oklch, var(--danger) 10%, transparent);
		border: 1px solid color-mix(in oklch, var(--danger) 45%, var(--border));
		border-radius: 8px;
		color: var(--danger);
	}

	.result-list {
		margin-top: 24px;
	}

	.result-item {
		display: grid;
		grid-template-columns: 72px minmax(0, 1fr);
		gap: 16px;
		padding: 14px 0 18px;
		border-bottom: 1px solid var(--border);
	}

	.thumb {
		display: grid;
		place-items: center;
		width: 72px;
		height: 72px;
		background: var(--surface-soft);
		border: 1px solid var(--border);
		border-radius: 6px;
		color: var(--accent);
		font-size: 1.4rem;
		font-weight: 800;
	}

	.result-title {
		color: var(--accent);
		font-size: 1.05rem;
		font-weight: 750;
		line-height: 1.35;
		text-decoration: none;
	}

	.result-title:hover {
		text-decoration: underline;
	}

	.meta-line {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin-top: 5px;
		color: var(--text-muted);
		font-size: 0.78rem;
	}

	.meta-line span:not(:last-child)::after {
		content: '·';
		margin-left: 8px;
		color: var(--border-strong);
	}

	.snippet {
		max-width: 76ch;
		margin: 8px 0 0;
		color: var(--text-secondary);
		font-size: 0.92rem;
		line-height: 1.55;
	}

	.pagination {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		flex-wrap: wrap;
		gap: 14px;
		margin-top: 28px;
		color: var(--text-secondary);
		font-size: 0.86rem;
	}

	.pagination.top {
		justify-content: space-between;
		margin-top: 16px;
		padding: 12px 0 2px;
		border-top: 1px solid var(--border);
	}

	.pagination.bottom {
		margin-top: 28px;
	}

	.pagination button {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 34px;
		padding: 0 12px;
		border: 1px solid var(--border);
		border-radius: 7px;
		background: var(--surface);
		color: var(--text-primary);
		cursor: pointer;
	}

	.pagination-pages {
		display: inline-flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 8px;
	}

	.pagination-pages button {
		min-width: 34px;
		justify-content: center;
		padding: 0 10px;
	}

	.pagination-pages button.current {
		border-color: color-mix(in oklch, var(--accent) 70%, var(--border));
		background: var(--accent-soft);
		color: var(--text-primary);
	}

	.pagination-ellipsis {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 18px;
		color: var(--text-muted);
	}

	.pagination-summary {
		white-space: nowrap;
	}

	.pagination button:disabled {
		cursor: not-allowed;
		opacity: 0.45;
	}

	.empty-state {
		margin-top: 36px;
		padding: 28px;
		background: var(--surface);
		border: 1px dashed var(--border-strong);
		border-radius: 8px;
	}

	.empty-state h2 {
		margin: 0 0 8px;
		font-size: 1rem;
		font-weight: 750;
	}

	.empty-state p {
		max-width: 64ch;
		margin: 0;
		color: var(--text-secondary);
		font-size: 0.92rem;
		line-height: 1.55;
	}

	.skeleton-result {
		padding: 16px 0 20px;
		border-bottom: 1px solid var(--border);
	}

	.skeleton-title,
	.skeleton-line {
		display: block;
		height: 14px;
		border-radius: 999px;
		background: linear-gradient(90deg, var(--surface-soft), var(--border), var(--surface-soft));
		background-size: 220% 100%;
		animation: shimmer 1.2s ease-in-out infinite;
	}

	.skeleton-title {
		height: 18px;
		margin-bottom: 12px;
	}

	.skeleton-line {
		width: min(100%, 720px);
		margin-top: 8px;
	}

	.skeleton-line.short {
		width: min(64%, 460px);
	}

	@keyframes shimmer {
		from {
			background-position: 120% 0;
		}
		to {
			background-position: -120% 0;
		}
	}

	@media (max-width: 760px) {
		.search-shell {
			width: min(100% - 28px, 1120px);
			padding-top: 28px;
		}

		.search-header {
			display: block;
		}

		h1 {
			font-size: 1.7rem;
		}

		.result-range {
			margin-top: 10px;
			white-space: normal;
		}

		.search-form {
			grid-template-columns: auto minmax(0, 1fr) auto;
		}

		.submit-button {
			grid-column: 1 / -1;
			width: 100%;
		}

		.scope-row {
			align-items: flex-start;
			flex-direction: column;
		}

		.result-item {
			grid-template-columns: 48px minmax(0, 1fr);
			gap: 12px;
		}

		.thumb {
			width: 48px;
			height: 48px;
			font-size: 1rem;
		}

		.pagination {
			justify-content: space-between;
		}
	}
</style>
