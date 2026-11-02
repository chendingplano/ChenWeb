<!-- The pane editor's commands (OpenSpec wpi-property-editors, design D2): the
     page's pane tree to choose a pane, and splitting, adding, merging and
     deleting panes. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { findPane, isLeaf } from '$lib/wpi/layout';
	import type { JsonObject, PaneSpec } from '$lib/wpi/types';
	import { rootOf } from './pane-edit';

	let { page, selectedPane, onSelectPane, onSplit, onAdd, onMerge, onDelete }: {
		page: JsonObject;
		selectedPane: string;
		onSelectPane: (pane: string) => void;
		onSplit: (pane: string, split: 'rows' | 'columns') => void;
		onAdd: (pane: string) => void;
		onMerge: (pane: string) => void;
		onDelete: (pane: string) => void;
	} = $props();

	const root = $derived(rootOf(page));
	const pane = $derived(findPane(root, selectedPane));
	function flat(p: PaneSpec, depth = 0): { pane: PaneSpec; depth: number }[] {
		return [{ pane: p, depth }, ...(p.children ?? []).flatMap((c) => flat(c, depth + 1))];
	}
</script>

<div class="structure">
	<div class="label">{m.wpi_pane_tree()}</div>
	<ul class="tree" aria-label={m.wpi_pane_tree()}>
		{#each flat(root) as item (item.pane.pane_id)}
			<li>
				<button type="button" class:active={item.pane.pane_id === selectedPane} aria-pressed={item.pane.pane_id === selectedPane}
					style="padding-left:{6 + item.depth * 12}px" onclick={() => onSelectPane(item.pane.pane_id)}
					>{item.depth === 0 ? m.wpi_pane_root() : item.pane.pane_id}{#if item.depth !== 0 && item.pane.split}<span class="split"> · {item.pane.split}</span>{/if}</button>
			</li>
		{/each}
	</ul>
	{#if pane}
		<div class="actions">
			{#if isLeaf(pane)}
				<button type="button" onclick={() => onSplit(pane.pane_id, 'rows')}>{m.wpi_pane_split_rows()}</button>
				<button type="button" onclick={() => onSplit(pane.pane_id, 'columns')}>{m.wpi_pane_split_columns()}</button>
			{:else}
				<button type="button" onclick={() => onAdd(pane.pane_id)}>{m.wpi_pane_add()}</button>
				<button type="button" onclick={() => onMerge(pane.pane_id)}>{m.wpi_pane_merge()}</button>
			{/if}
			{#if pane.pane_id !== root.pane_id}<button type="button" class="danger" onclick={() => onDelete(pane.pane_id)}>{m.wpi_pane_delete()}</button>{/if}
		</div>
	{/if}
</div>

<style>
	.structure {
		margin-bottom: 10px;
	}
	.label {
		font-size: 10px;
		color: var(--cv-text-muted);
		margin: 0 0 4px;
	}
	.tree {
		list-style: none;
		margin: 0 0 8px;
		padding: 0;
		border: 1px solid var(--cv-border);
		border-radius: 4px;
	}
	.tree button {
		display: block;
		width: 100%;
		text-align: left;
		border: 0;
		background: transparent;
		color: var(--cv-text-2);
		font-size: 11px;
		padding: 4px 6px;
		cursor: pointer;
	}
	.tree button:hover,
	.tree button.active {
		background: var(--cv-hover);
		color: var(--cv-text);
	}
	.split {
		color: var(--cv-text-muted);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.actions button {
		border: 1px solid var(--cv-border);
		border-radius: 4px;
		background: var(--cv-surface);
		color: var(--cv-text-2);
		font-size: 10px;
		padding: 4px 8px;
		cursor: pointer;
	}
	.actions button:hover {
		background: var(--cv-hover);
	}
	.actions .danger {
		color: #dc2626;
	}
</style>
