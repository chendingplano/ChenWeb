<script lang="ts">
	// Categories — home3/knowledge → Ontology → Categories (openspec ontology-categories-page).
	// Lists every category tree, draws the selected one in the chosen layout, and opens the
	// artifacts bound to a clicked leaf in a right-hand panel.
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { onMount } from 'svelte';
	import CategoryTreeChart from './category-tree-chart.svelte';
	import {
		buildTreeData,
		DEFAULT_EDGE_STYLE,
		DEFAULT_TREE_LAYOUT,
		EDGE_STYLES,
		labelLang,
		nodeLabel,
		TREE_LAYOUTS,
		type CategoryTreeDatum,
		type EdgeStyle,
		type TreeLayout
	} from './category-tree-model.js';
	import {
		CATEGORY_MEMBERS_LIMIT,
		getCategoryTree,
		listCategoryMembers,
		listCategoryTrees,
		type CategoryMember,
		type CategoryTree,
		type CategoryTreeNode
	} from '$lib/services/categoryTreeService';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	const lang = $derived(labelLang(getLocale()));

	let trees = $state<CategoryTree[]>([]);
	let treesLoading = $state(false);
	let treesError = $state(false);

	let selectedName = $state<string | null>(null);
	let treeNodes = $state<CategoryTreeNode[]>([]);
	let treeProblems = $state<string[]>([]);
	let treeLoading = $state(false);
	let treeError = $state(false);
	let layout = $state<TreeLayout>(DEFAULT_TREE_LAYOUT);
	let edgeStyle = $state<EdgeStyle>(DEFAULT_EDGE_STYLE);
	const treeData = $derived<CategoryTreeDatum | null>(buildTreeData(treeNodes, lang));

	let leaf = $state<CategoryTreeDatum | null>(null);
	const leafLabel = $derived.by(() => {
		const node = treeNodes.find((node) => node.id === leaf?.id);
		return node ? nodeLabel(node, lang) : leaf?.name;
	});
	let members = $state<CategoryMember[]>([]);
	let membersLoading = $state(false);
	let membersError = $state(false);

	// Responses for a superseded selection are dropped, so a slow request cannot overwrite a newer one.
	let treeSeq = 0;
	let membersSeq = 0;

	const memberListTruncated = $derived(members.length >= CATEGORY_MEMBERS_LIMIT);

	function layoutLabel(l: TreeLayout): string {
		switch (l) {
			case 'LR':
				return m.home3_categories_layout_lr();
			case 'RL':
				return m.home3_categories_layout_rl();
			case 'TB':
				return m.home3_categories_layout_tb();
			case 'BT':
				return m.home3_categories_layout_bt();
		}
	}

	function edgeStyleLabel(s: EdgeStyle): string {
		return s === 'curve'
			? m.home3_categories_line_style_curve()
			: m.home3_categories_line_style_broken();
	}

	function treeLabel(t: CategoryTree): string {
		return t.pref_labels[lang] || t.pref_labels.en || t.canonical_name;
	}

	async function loadTrees() {
		treesLoading = true;
		treesError = false;
		try {
			trees = await listCategoryTrees();
		} catch {
			treesError = true;
		} finally {
			treesLoading = false;
		}
	}

	onMount(loadTrees);

	async function loadTree(name: string) {
		const seq = ++treeSeq;
		treeLoading = true;
		treeError = false;
		treeNodes = [];
		treeProblems = [];
		leaf = null;
		members = [];
		membersError = false;
		membersLoading = false;
		membersSeq++;
		try {
			const res = await getCategoryTree(name);
			if (seq !== treeSeq) return;
			treeNodes = res.nodes ?? [];
			treeProblems = res.valid ? [] : (res.problems ?? []);
		} catch {
			if (seq !== treeSeq) return;
			treeError = true;
		} finally {
			if (seq === treeSeq) treeLoading = false;
		}
	}

	function selectTree(name: string) {
		if (name === selectedName) return;
		selectedName = name;
		loadTree(name);
	}

	async function openLeaf(datum: CategoryTreeDatum) {
		if (!selectedName) return;
		const name = selectedName;
		const seq = ++membersSeq;
		leaf = datum;
		members = [];
		membersError = false;
		membersLoading = true;
		try {
			const res = await listCategoryMembers(name, datum.id);
			if (seq !== membersSeq) return;
			members = res.members ?? [];
		} catch {
			if (seq !== membersSeq) return;
			membersError = true;
		} finally {
			if (seq === membersSeq) membersLoading = false;
		}
	}

	function closeLeaf() {
		membersSeq++;
		leaf = null;
		members = [];
		membersLoading = false;
		membersError = false;
	}
</script>

<div class="categories-page select-text" class:light={!darkMode}>
	<aside class="cat-list" aria-label={m.home3_categories_trees_heading()}>
		<h2 class="cat-heading">{m.home3_categories_trees_heading()}</h2>
		{#if treesLoading}
			<p class="cat-muted">{m.home3_categories_tree_loading()}</p>
		{:else if treesError}
			<p class="cat-error">{m.home3_categories_load_failed()}</p>
			<button type="button" class="cat-button" onclick={loadTrees}
				>{m.home3_categories_retry()}</button
			>
		{:else if trees.length === 0}
			<p class="cat-muted">{m.home3_categories_no_trees()}</p>
		{:else}
			<ul class="cat-tree-list">
				{#each trees as t (t.id)}
					<li>
						<button
							type="button"
							class="cat-tree-item"
							class:active={t.canonical_name === selectedName}
							aria-pressed={t.canonical_name === selectedName}
							onclick={() => selectTree(t.canonical_name)}
						>
							<span class="cat-tree-label">{treeLabel(t)}</span>
							<span class="cat-tree-name">{t.canonical_name}</span>
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</aside>

	<section class="cat-main">
		{#if !selectedName}
			<p class="cat-muted">{m.home3_categories_select_tree()}</p>
		{:else}
			<div class="cat-toolbar">
				<label class="cat-layout">
					<span>{m.home3_categories_layout_label()}</span>
					<select class="cat-select" bind:value={layout}>
						{#each TREE_LAYOUTS as l (l)}
							<option value={l}>{layoutLabel(l)}</option>
						{/each}
					</select>
				</label>
				<label class="cat-layout">
					<span>{m.home3_categories_line_style_label()}</span>
					<select class="cat-select" bind:value={edgeStyle}>
						{#each EDGE_STYLES as s (s)}
							<option value={s}>{edgeStyleLabel(s)}</option>
						{/each}
					</select>
				</label>
			</div>

			{#if treeLoading}
				<p class="cat-muted">{m.home3_categories_tree_loading()}</p>
			{:else if treeError}
				<p class="cat-error">{m.home3_categories_tree_load_failed()}</p>
			{:else if treeData}
				{#if treeProblems.length > 0}
					<div class="cat-problems">
						<p>{m.home3_categories_problems()}</p>
						<ul>
							{#each treeProblems as p, i (i)}
								<li>{p}</li>
							{/each}
						</ul>
					</div>
				{/if}
				<div class="cat-chart">
					<CategoryTreeChart
						data={treeData}
						{layout}
						{edgeStyle}
						{darkMode}
						onleafclick={openLeaf}
					/>
				</div>
			{/if}
		{/if}
	</section>

	<aside class="cat-panel" aria-label={m.home3_categories_panel_title()}>
		<div class="cat-panel-head">
			<h2 class="cat-heading">{leaf ? leafLabel : m.home3_categories_panel_title()}</h2>
			{#if leaf}
				<button type="button" class="cat-button" onclick={closeLeaf}
					>{m.home3_categories_close()}</button
				>
			{/if}
		</div>

		{#if !leaf}
			<p class="cat-muted">{m.home3_categories_panel_hint()}</p>
		{:else if membersLoading}
			<p class="cat-muted">{m.home3_categories_panel_loading()}</p>
		{:else if membersError}
			<p class="cat-error">{m.home3_categories_panel_load_failed()}</p>
		{:else if members.length === 0}
			<p class="cat-muted">{m.home3_categories_panel_empty()}</p>
		{:else}
			{#if memberListTruncated}
				<p class="cat-muted">
					{m.home3_categories_panel_truncated({ count: CATEGORY_MEMBERS_LIMIT })}
				</p>
			{/if}
			<table class="cat-table">
				<thead>
					<tr>
						<th scope="col">{m.home3_categories_col_type()}</th>
						<th scope="col">{m.home3_categories_col_artifact()}</th>
						<th scope="col">{m.home3_categories_col_record()}</th>
						<th scope="col">{m.home3_categories_col_status()}</th>
					</tr>
				</thead>
				<tbody>
					{#each members as row (row.artifact_type + ':' + row.artifact_id + ':' + row.node_id)}
						<tr>
							<td>{row.artifact_type}</td>
							<td class="cat-mono">{row.artifact_id}</td>
							<td>{row.record_id}</td>
							<td>
								<span class:cat-stale={row.stale}>
									{row.stale ? m.home3_categories_stale() : m.home3_categories_current()}
								</span>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</aside>
</div>

<style>
	.categories-page {
		--cat-bg: #0e1116;
		--cat-panel: rgba(255, 255, 255, 0.03);
		--cat-border: rgba(148, 163, 184, 0.2);
		--cat-text: #e2e8f0;
		--cat-muted: #94a3b8;
		--cat-accent: #818cf8;
		--cat-error: #f87171;
		--cat-stale: #fbbf24;
		display: grid;
		grid-template-columns: minmax(180px, 240px) minmax(0, 1fr) minmax(240px, 320px);
		grid-template-rows: minmax(0, 1fr);
		gap: 16px;
		height: 100%;
		min-height: 0;
		padding: 16px;
		box-sizing: border-box;
		color: var(--cat-text);
		background: var(--cat-bg);
		user-select: text;
	}
	.categories-page.light {
		--cat-bg: #f5f1e8;
		--cat-panel: rgba(255, 255, 255, 0.8);
		--cat-border: rgba(100, 116, 139, 0.3);
		--cat-text: #1f2937;
		--cat-muted: #6b7280;
		--cat-accent: #4f46e5;
		--cat-error: #b91c1c;
		--cat-stale: #b45309;
	}
	.cat-list,
	.cat-panel {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		overflow: auto;
		padding: 12px;
		border: 1px solid var(--cat-border);
		border-radius: 10px;
		background: var(--cat-panel);
	}

	.cat-main {
		overflow: hidden;
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		padding: 12px;
		border: 1px solid var(--cat-border);
		border-radius: 10px;
		background: var(--cat-panel);
	}
	.cat-heading {
		margin: 0 0 8px;
		font-size: 15px;
		font-weight: 600;
	}
	.cat-panel-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
	}
	.cat-muted {
		color: var(--cat-muted);
		font-size: 13px;
	}
	.cat-error {
		color: var(--cat-error);
		font-size: 13px;
	}
	.cat-tree-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.cat-tree-item {
		display: flex;
		flex-direction: column;
		width: 100%;
		text-align: left;
		padding: 8px 10px;
		border: 1px solid var(--cat-border);
		border-radius: 8px;
		background: transparent;
		color: inherit;
		cursor: pointer;
	}
	.cat-tree-item.active {
		border-color: var(--cat-accent);
	}
	.cat-tree-label {
		font-size: 14px;
		font-weight: 600;
	}
	.cat-tree-name {
		font-size: 12px;
		color: var(--cat-muted);
		word-break: break-all;
	}
	.cat-toolbar {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 16px;
		margin-bottom: 8px;
	}
	.cat-layout {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
	}
	.cat-select {
		padding: 4px 8px;
		border: 1px solid var(--cat-border);
		border-radius: 6px;
		background: var(--cat-bg);
		color: inherit;
	}
	.cat-chart {
		flex: 1 1 auto;
		min-height: 0;
		position: relative;
	}
	.cat-problems {
		margin-bottom: 8px;
		padding: 8px;
		border: 1px solid var(--cat-error);
		border-radius: 8px;
		font-size: 13px;
		color: var(--cat-error);
	}
	.cat-button {
		padding: 4px 10px;
		border: 1px solid var(--cat-border);
		border-radius: 6px;
		background: transparent;
		color: inherit;
		cursor: pointer;
		font-size: 13px;
	}
	.cat-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	.cat-table th,
	.cat-table td {
		padding: 6px 4px;
		border-bottom: 1px solid var(--cat-border);
		text-align: left;
		vertical-align: top;
	}
	.cat-mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		word-break: break-all;
	}
	.cat-stale {
		color: var(--cat-stale);
	}

	/* Phone width: stack the panes, with a 16px gutter and no horizontal scroll. */
	@media (max-width: 900px) {
		.categories-page {
			grid-template-columns: minmax(0, 1fr);
			padding: 16px;
		}
		.categories-page {
			height: auto;
			min-height: 100%;
		}
		.cat-chart {
			flex: none;
			height: 60vh;
		}
	}
</style>
