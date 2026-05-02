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
		background: #fbbf24;
		box-shadow: none;
		overflow: hidden;
		transform: translateY(10px);
		z-index: 1;
	}

	.tab.active {
		border-color: rgba(15, 23, 42, 0.96);
		background: #2f75c8;
		transform: translateY(0);
		z-index: 3;
		box-shadow: none;
	}

	.tab.active::after {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		bottom: -2px;
		height: 4px;
		background: rgba(15, 23, 42, 0.96);
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
		color: #111827;
	}

	.close-button {
		padding: 0.88rem 0.85rem 0.8rem 0.2rem;
		color: rgba(17, 24, 39, 0.76);
		font-size: 1rem;
	}

	.tab.active .tab-button,
	.tab.active .close-button {
		color: #ffffff;
	}

	@media (max-width: 760px) {
		.tab-button {
			padding: 0.82rem 1rem 0.74rem;
			font-size: 0.9rem;
		}
	}
</style>
