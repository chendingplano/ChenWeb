<script lang="ts">
	import { BY_ID, CHAIN_NODE_BY_ID } from './model';
	import type { ExplorerTokens } from './theme';
	import { LOADERS, type Cell } from '$lib/services/metricOntologyExplorerService';
	import MetricSearchPane from './metric-search-pane.svelte';

	let {
		tokens,
		darkMode = true,
		focusId,
		metricId = '',
		openTabs,
		activeTab,
		onselecttab,
		onclosetab,
		onpickmetric
	}: {
		tokens: ExplorerTokens;
		darkMode?: boolean;
		focusId: string;
		metricId?: string;
		openTabs: string[];
		activeTab: string; // 'entry' | 'search' | chain node id
		onselecttab: (id: string) => void;
		onclosetab: (id: string) => void;
		onpickmetric: (metricId: string) => void;
	} = $props();

	const SECTIONS = ['Definition', 'Connection', 'Processing', 'Sources'] as const;
	let activeSection = $state<(typeof SECTIONS)[number]>('Definition');

	const focusNode = $derived(BY_ID[focusId] ?? BY_ID.metric);

	type Loaded = { state: 'loading' | 'ok' | 'err'; rows?: Cell[][]; error?: string };
	let cache = $state<Record<string, Loaded>>({});

	$effect(() => {
		const id = activeTab;
		if (id === 'entry') return;
		const node = CHAIN_NODE_BY_ID[id];
		if (!node?.loaderKey) return;
		if (cache[id]) return;
		cache[id] = { state: 'loading' };
		LOADERS[node.loaderKey]()
			.then((r) => (cache[id] = { state: 'ok', rows: r.rows }))
			.catch((e) => (cache[id] = { state: 'err', error: e instanceof Error ? e.message : String(e) }));
	});

	const activeNode = $derived(activeTab === 'entry' ? null : CHAIN_NODE_BY_ID[activeTab]);
</script>

<div
	class="cv"
	style="--panel:{tokens.panelBg}; --card:{tokens.cardBg}; --border:{tokens.border}; --border-strong:{tokens.borderStrong}; --text:{tokens.textPrimary}; --text-2:{tokens.textSecondary}; --text-3:{tokens.textMuted}; --accent:{tokens.accent}; --hover:{tokens.hoverBg};"
>
	<div class="tabs" role="tablist">
		<button
			class="tab"
			class:active={activeTab === 'entry'}
			role="tab"
			aria-selected={activeTab === 'entry'}
			onclick={() => onselecttab('entry')}
		>
			{focusNode.label}
		</button>
		<button
			class="tab"
			class:active={activeTab === 'search'}
			role="tab"
			aria-selected={activeTab === 'search'}
			onclick={() => onselecttab('search')}
		>
			Search
		</button>
		{#each openTabs as id (id)}
			{@const node = CHAIN_NODE_BY_ID[id]}
			<span class="tab-wrap" class:active={activeTab === id}>
				<button class="tab chain" role="tab" aria-selected={activeTab === id} onclick={() => onselecttab(id)}>
					{node?.label ?? id}
				</button>
				<button class="x" title="Close tab" aria-label="Close {node?.label ?? id}" onclick={() => onclosetab(id)}>×</button>
			</span>
		{/each}
	</div>

	{#if activeTab === 'search'}
	<div class="search-slot">
		<MetricSearchPane {tokens} {darkMode} activeMetricId={metricId} onpick={onpickmetric} />
	</div>
	{:else}
	<div class="body">
		{#if activeTab === 'entry'}
			<div class="kicker">
				{#if focusId === 'metric'}The centre{:else}Metric {focusNode.edge} {focusNode.label.toLowerCase()}{/if}
			</div>
			<h1 class="title">{focusNode.label}</h1>
			<nav class="secnav">
				{#each SECTIONS as s}
					<button class:on={s === activeSection} onclick={() => (activeSection = s)}>{s}</button>
				{/each}
			</nav>
			{#if activeSection === 'Definition'}
				<p class="prose">{focusNode.entry.definition}</p>
			{:else if activeSection === 'Connection'}
				<p class="prose">{focusNode.entry.connection}</p>
			{:else if activeSection === 'Processing'}
				<p class="prose">{focusNode.entry.processing}</p>
			{:else}
				{#if focusNode.entry.sources.length}
					{#each focusNode.entry.sources as q}
						<blockquote class="src">“{q}”</blockquote>
					{/each}
				{:else}
					<p class="prose muted">No source spans recorded for this node.</p>
				{/if}
			{/if}
		{:else if activeNode}
			<div class="kicker">{BY_ID[activeNode.parentId]?.label ?? activeNode.parentId} ▸ {activeNode.label}</div>
			<h1 class="title">{activeNode.label}</h1>
			<div class="rec-meta">
				<span class="pill" class:stage={!activeNode.loaderKey && activeNode.table.startsWith('pipeline')}>{activeNode.table}</span>
			</div>
			<p class="prose">{activeNode.description}</p>

			{#if activeNode.loaderKey}
				{@const loaded = cache[activeTab]}
				{#if !loaded || loaded.state === 'loading'}
					<p class="note">Loading rows…</p>
				{:else if loaded.state === 'err'}
					<p class="note err">Could not load {activeNode.table}: {loaded.error}</p>
				{:else}
					<div class="rec-wrap">
						<table class="rec">
							<thead>
								<tr>{#each activeNode.columns as c}<th>{c}</th>{/each}</tr>
							</thead>
							<tbody>
								{#each loaded.rows ?? [] as row}
									<tr>{#each row as cell}<td>{cell}</td>{/each}</tr>
								{/each}
								{#if !(loaded.rows ?? []).length}
									<tr><td class="empty" colspan={activeNode.columns.length}>No rows.</td></tr>
								{/if}
							</tbody>
						</table>
					</div>
				{/if}
			{:else}
				<div class="schema">
					<div class="schema-head">Schema · read endpoint pending</div>
					<table class="rec">
						<thead><tr><th>column</th></tr></thead>
						<tbody>
							{#each activeNode.columns as c}<tr><td>{c}</td></tr>{/each}
						</tbody>
					</table>
					<p class="note">A follow-up change wires <code>{activeNode.table}</code> to live data.</p>
				</div>
			{/if}
		{/if}
	</div>
	{/if}
</div>

<style>
	.cv {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
		background: var(--panel);
		color: var(--text);
	}
	.search-slot {
		flex: 1 1 auto;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}
	.tabs {
		display: flex;
		gap: 2px;
		padding: 6px 10px 0;
		border-bottom: 1px solid var(--border);
		flex: 0 0 auto;
		overflow-x: auto;
	}
	.tab {
		appearance: none;
		border: 1px solid transparent;
		border-bottom: 0;
		border-radius: 7px 7px 0 0;
		background: transparent;
		color: var(--text-2);
		font: 600 11px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.03em;
		padding: 8px 12px 9px;
		cursor: pointer;
		white-space: nowrap;
	}
	.tab:hover {
		color: var(--text);
	}
	.tab-wrap {
		display: inline-flex;
		align-items: stretch;
	}
	.tab-wrap.active .tab,
	.tab.active {
		color: var(--accent);
		background: var(--card);
		border-color: var(--border);
		position: relative;
		top: 1px;
	}
	.tab-wrap .x {
		appearance: none;
		border: 0;
		background: transparent;
		color: var(--text-3);
		font-size: 14px;
		cursor: pointer;
		padding: 0 8px 0 2px;
		align-self: center;
	}
	.tab-wrap .x:hover {
		color: var(--accent);
	}

	.body {
		flex: 1 1 auto;
		overflow: auto;
		padding: 18px 22px 34px;
	}
	.kicker {
		font: 600 10px/1.4 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--accent);
		margin-bottom: 6px;
	}
	.title {
		font: 600 24px/1.15 'Fraunces', Georgia, serif;
		margin: 0 0 12px;
		letter-spacing: -0.01em;
	}
	.secnav {
		display: flex;
		gap: 16px;
		margin: 0 0 14px;
		flex-wrap: wrap;
	}
	.secnav button {
		appearance: none;
		background: transparent;
		border: 0;
		border-bottom: 2px solid transparent;
		padding: 0 0 4px;
		cursor: pointer;
		font: 600 10px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-2);
	}
	.secnav button.on {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}
	.prose {
		max-width: 60ch;
		line-height: 1.62;
		margin: 0 0 12px;
		color: var(--text);
	}
	.prose.muted {
		color: var(--text-2);
		font-style: italic;
	}
	.src {
		max-width: 58ch;
		margin: 0 0 10px;
		padding: 10px 14px;
		border-left: 3px solid var(--accent);
		background: var(--card);
		font: italic 14px/1.5 'Fraunces', Georgia, serif;
	}
	.rec-meta {
		margin: 0 0 12px;
	}
	.pill {
		font: 600 10px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.06em;
		background: var(--accent);
		color: #fff;
		padding: 3px 9px;
		border-radius: 999px;
	}
	.pill.stage {
		background: var(--text-3);
	}
	.rec-wrap {
		overflow-x: auto;
		border: 1px solid var(--border);
		border-radius: 6px;
	}
	table.rec {
		border-collapse: collapse;
		width: 100%;
		font: 400 11.5px/1.4 ui-sans-serif, system-ui, sans-serif;
	}
	table.rec th {
		text-align: left;
		padding: 8px 12px;
		background: var(--card);
		border-bottom: 1px solid var(--border);
		font-size: 9px;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--text-2);
		white-space: nowrap;
	}
	table.rec td {
		padding: 8px 12px;
		border-bottom: 1px solid var(--border);
		color: var(--text-2);
		white-space: nowrap;
	}
	table.rec tbody tr:last-child td {
		border-bottom: 0;
	}
	td.empty {
		text-align: center;
		color: var(--text-3);
		font-style: italic;
	}
	.note {
		margin: 12px 0 0;
		font: 400 11px/1.5 ui-sans-serif, system-ui, sans-serif;
		color: var(--text-2);
	}
	.note.err {
		color: #dc2626;
	}
	.schema-head {
		font: 600 10px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--text-3);
		margin-bottom: 8px;
	}
	code {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.92em;
	}
</style>
