<script lang="ts">
	import type { JsonTreeNode } from './doc-review-json-dialog.js';

	let { nodes, dark = true }: { nodes: JsonTreeNode[]; dark?: boolean } = $props();

	let textSecondary = $derived(dark ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(dark ? '#64748B' : '#9CA3AF');
</script>

{#if !nodes.length}
	<div class="text-center" style="color:{textMuted};padding:1rem">No data.</div>
{:else}
	<ul class="json-tree-level" style="user-select:text">
		{#each nodes as node (node.label)}
			{@render treeNode(node)}
		{/each}
	</ul>
{/if}

{#snippet treeNode(node: JsonTreeNode)}
	<li class="json-tree-li">
		<span style="color:{textMuted};font-family:monospace">{node.label}:</span>
		<span style="color:{textSecondary};word-break:break-word;white-space:pre-wrap">{node.value}</span>
		{#if node.children?.length}
			<ul class="json-tree-level json-tree-nested">
				{#each node.children as child (child.label)}
					{@render treeNode(child)}
				{/each}
			</ul>
		{/if}
	</li>
{/snippet}

<style>
	/* Each .json-tree-level is its own grid, so sibling values (same nesting
	   level) align to a shared column, independent of other levels' widths. */
	.json-tree-level {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 0.2rem 0.75rem;
		list-style: none;
		margin: 0;
		padding: 0;
		font-size: 12px;
		line-height: 1.6;
	}
	.json-tree-li {
		display: contents;
	}
	.json-tree-nested {
		grid-column: 1 / -1;
		padding-left: 1.1rem;
	}
</style>
