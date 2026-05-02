<script lang="ts">
	import type { SummaryCategoryTab } from './summary-types';

	let {
		tabs,
		activeTabId,
		onSelect = () => {},
		onClose = () => {}
	}: {
		tabs: SummaryCategoryTab[];
		activeTabId: string;
		onSelect?: (tabId: string) => void;
		onClose?: (tabId: string) => void;
	} = $props();

	function formatLabel(tab: SummaryCategoryTab) {
		if (!tab.categoryPath || tab.categoryPath.length <= 26) return tab.label;
		return `...${tab.categoryPath.slice(-23)}`;
	}
</script>

<div class="tab-strip">
	{#each tabs as tab}
		<div class:active={activeTabId === tab.id} class="tab">
			<button type="button" class="tab-button" title={tab.categoryPath ?? tab.label} onclick={() => onSelect(tab.id)}>
				{formatLabel(tab)}
			</button>
			{#if tab.closable}
				<button type="button" class="close-button" onclick={() => onClose(tab.id)}>×</button>
			{/if}
		</div>
	{/each}
</div>

<style>
	.tab-strip {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
		margin-bottom: 1rem;
	}

	.tab {
		display: inline-flex;
		align-items: center;
		border-radius: 14px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.34);
		overflow: hidden;
	}

	.tab.active {
		border-color: rgba(99, 102, 241, 0.38);
		background: rgba(99, 102, 241, 0.14);
	}

	.tab-button,
	.close-button {
		border: none;
		background: transparent;
		color: inherit;
		cursor: pointer;
	}

	.tab-button {
		padding: 0.72rem 0.95rem;
		font-weight: 700;
	}

	.close-button {
		padding: 0.72rem 0.75rem;
		color: #94a3b8;
	}
</style>
