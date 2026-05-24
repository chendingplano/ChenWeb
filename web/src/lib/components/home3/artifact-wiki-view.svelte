<script lang="ts">
	import TreeGraphView from './tree-graph-view.svelte';
	import ArtifactCategoryPanel from './artifact-category-panel.svelte';
	import { knowledgeStoreState } from '$lib/components/home3/knowledge-store-state.svelte';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	type ArtifactTab = {
		id: string;
		label: string;
		categoryPath: string | null;
		closable: boolean;
	};

	let tabs = $state<ArtifactTab[]>([
		{ id: 'artifact-graph', label: 'Artifact Graph', categoryPath: null, closable: false }
	]);
	let activeTabId = $state('artifact-graph');

	let activeTab = $derived(tabs.find((t) => t.id === activeTabId) ?? tabs[0]);

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');

	function openCategoryTab(categoryPath: string) {
		const existingId = `artifact-category:${categoryPath}`;
		const existing = tabs.find((t) => t.id === existingId);
		if (existing) {
			activeTabId = existing.id;
			return;
		}
		const label =
			categoryPath.length <= 26 ? categoryPath : `...${categoryPath.slice(-23)}`;
		tabs = [
			...tabs,
			{ id: existingId, label, categoryPath, closable: true }
		];
		activeTabId = existingId;
	}

	function closeTab(tabId: string) {
		const idx = tabs.findIndex((t) => t.id === tabId);
		tabs = tabs.filter((t) => t.id !== tabId);
		if (activeTabId === tabId) {
			const fallback = tabs[Math.max(0, idx - 1)];
			activeTabId = fallback?.id ?? 'artifact-graph';
		}
	}

	function formatTabLabel(tab: ArtifactTab) {
		if (!tab.categoryPath || tab.categoryPath.length <= 26) return tab.label;
		return `...${tab.categoryPath.slice(-23)}`;
	}
</script>

<div class="artifact-wiki" style="--panel-bg:{panelBg}; --border:{border};">
	<div class="tabbed-window">
		<div class="tabbed-window-head">
			<div class="tab-strip">
				{#each tabs as tab (tab.id)}
					<div class="tab" class:active={activeTabId === tab.id}>
						<button
							type="button"
							class="tab-button"
							title={tab.categoryPath ?? tab.label}
							onclick={() => (activeTabId = tab.id)}
						>
							{formatTabLabel(tab)}
						</button>
						{#if tab.closable}
							<button type="button" class="close-button" onclick={() => closeTab(tab.id)}>×</button>
						{/if}
					</div>
				{/each}
			</div>
		</div>

		{#if activeTab.categoryPath}
			<div class="tabbed-window-body">
				<ArtifactCategoryPanel
					categoryPath={activeTab.categoryPath}
					{darkMode}
					ksStoreId={knowledgeStoreState.activeStore?.id ?? null}
				/>
			</div>
		{:else}
			<div class="graph-frame">
				<TreeGraphView
					mode="summary"
					{darkMode}
					heroEyebrow="Artifact Wiki"
					heroTitle="Artifact Graph"
					heroDescription="Browse and explore all artifacts — summaries, topics, metrics, scenes, provisions, and products — organized by category."
					rootTabLabel="Artifact Graph"
					loadErrorLabel="Artifact Graph"
					hideTabStrip={true}
					onOpenCategoryTab={openCategoryTab}
				/>
			</div>
		{/if}
	</div>
</div>

<style>
	.artifact-wiki {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
		background: var(--panel-bg);
	}

	.tabbed-window {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
	}

	.tabbed-window-head {
		position: relative;
		z-index: 4;
		margin-bottom: -1px;
		padding-inline: 0.55rem;
	}

	/* Category tab body: adds its own border + rounded corner frame */
	.tabbed-window-body {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		border: 1px solid var(--border);
		border-radius: 0 24px 24px 24px;
		background: linear-gradient(180deg, rgba(15, 23, 42, 0.72), rgba(15, 23, 42, 0.58));
		box-shadow:
			inset 0 1px 0 rgba(255, 255, 255, 0.04),
			0 18px 44px rgba(2, 6, 23, 0.18);
		overflow: hidden;
	}

	/* Graph tab body: plain container — TreeGraphView provides its own border + background */
	.graph-frame {
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}

	.tab-strip {
		display: flex;
		flex-wrap: nowrap;
		gap: 0;
		align-items: flex-end;
		overflow-x: auto;
		overflow-y: hidden;
		padding: 0 0.5rem;
		scrollbar-width: thin;
	}

	.tab {
		display: inline-flex;
		align-items: center;
		position: relative;
		margin-right: -1px;
		border: 2px solid rgba(15, 23, 42, 0.9);
		border-bottom: none;
		border-radius: 14px 14px 0 0;
		background: #2f75c8;
		box-shadow: none;
		overflow: hidden;
		min-height: 52px;
		z-index: 1;
	}

	.tab.active {
		border-color: rgba(15, 23, 42, 0.96);
		background: #fbbf24;
		min-height: 58px;
		z-index: 3;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.12);
	}

	.tab-button,
	.close-button {
		border: none;
		background: transparent;
		color: inherit;
		cursor: pointer;
	}

	.tab-button {
		padding: 0.88rem 1.25rem 0.8rem;
		font-weight: 700;
		white-space: nowrap;
		color: #ffffff;
	}

	.close-button {
		padding: 0.88rem 0.85rem 0.8rem 0.2rem;
		color: rgba(255, 255, 255, 0.82);
		font-size: 1rem;
	}

	.tab.active .tab-button,
	.tab.active .close-button {
		color: #111827;
	}

	@media (max-width: 760px) {
		.tab-button {
			padding: 0.82rem 1rem 0.74rem;
			font-size: 0.9rem;
		}
	}
</style>
