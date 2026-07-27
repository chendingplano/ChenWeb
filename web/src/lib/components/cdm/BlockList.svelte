<script lang="ts">
	// Owns a document's top-level block structure: selection, insertion,
	// deletion, reordering, and (for the three content-bearing types) type
	// change. This is the "Svelte-owned block list" half of ADR 2026072603
	// DR1 -- everything here operates directly on the block array; no
	// rich-text engine is involved. The inline editor that will eventually
	// replace read-only text inside paragraph/heading/quote blocks is task
	// group 5, out of scope here.
	//
	// blocks is $bindable so the caller (the eventual editor page) holds the
	// single canonical copy of the document's blocks; this component never
	// keeps its own duplicate (design D8 -- no view model, no mapping layer).
	//
	// Reordering is up/down buttons, not drag-and-drop, for this iteration.
	// @dnd-kit is already a project dependency and used elsewhere
	// (data-table.svelte), so richer reordering can be layered in later
	// without a new dependency; up/down is simpler and fully satisfies
	// "operating directly on the block array" for now.
	import type { Block, BlockType } from './types.js';
	import { BLOCK_TYPES } from './types.js';
	import BlockView from './BlockView.svelte';
	import InsertControl from './InsertControl.svelte';
	import { createIdAllocator, collectBlockIds } from './block-id.js';
	import { createDefaultBlock } from './block-defaults.js';
	import {
		insertBlockAt,
		deleteBlockById,
		moveBlock,
		changeContentBlockType,
		isContentBearingType,
		type ContentBearingType
	} from './block-ops.js';

	let { blocks = $bindable() }: { blocks: Block[] } = $props();

	let selectedId = $state<string | null>(null);
	let insertTypeChoice = $state<BlockType>('paragraph');

	const CONTENT_TYPE_CHOICES: ContentBearingType[] = ['paragraph', 'heading', 'quote'];

	function selectBlock(id: string) {
		selectedId = selectedId === id ? null : id;
	}

	function insertAt(index: number) {
		const allocate = createIdAllocator(collectBlockIds(blocks));
		const block = createDefaultBlock(insertTypeChoice, allocate);
		blocks = insertBlockAt(blocks, index, block);
		selectedId = block.id;
	}

	function remove(id: string) {
		blocks = deleteBlockById(blocks, id);
		if (selectedId === id) selectedId = null;
	}

	function move(id: string, direction: 'up' | 'down') {
		blocks = moveBlock(blocks, id, direction);
	}

	function changeType(id: string, newType: ContentBearingType) {
		blocks = blocks.map((b) => (b.id === id ? changeContentBlockType(b, newType) : b));
	}
</script>

<div class="cdm-block-list">
	<div class="cdm-insert-row">
		<InsertControl
			bind:choice={insertTypeChoice}
			onInsert={() => insertAt(0)}
			label="Insert at top"
		/>
	</div>

	{#each blocks as block, index (block.id)}
		<div class="cdm-block-row" class:cdm-block-row--selected={selectedId === block.id}>
			<button
				type="button"
				class="cdm-block-select"
				onclick={() => selectBlock(block.id)}
				aria-pressed={selectedId === block.id}
			>
				<BlockView {block} />
			</button>

			{#if selectedId === block.id}
				<div class="cdm-block-toolbar">
					<span class="cdm-block-id" title="block id">{block.id}</span>
					<button type="button" onclick={() => move(block.id, 'up')} disabled={index === 0}
						>↑ Up</button
					>
					<button
						type="button"
						onclick={() => move(block.id, 'down')}
						disabled={index === blocks.length - 1}>↓ Down</button
					>
					{#if isContentBearingType(block.type)}
						<select
							value={block.type}
							onchange={(e) => changeType(block.id, e.currentTarget.value as ContentBearingType)}
						>
							{#each CONTENT_TYPE_CHOICES as choice (choice)}
								<option value={choice}>{choice}</option>
							{/each}
						</select>
					{/if}
					<button type="button" class="cdm-block-delete" onclick={() => remove(block.id)}
						>Delete</button
					>
				</div>
			{/if}
		</div>

		<div class="cdm-insert-row">
			<InsertControl
				bind:choice={insertTypeChoice}
				onInsert={() => insertAt(index + 1)}
				label="Insert here"
			/>
		</div>
	{/each}

	{#if blocks.length === 0}
		<p class="cdm-empty">This document has no blocks yet.</p>
	{/if}
</div>

<style>
	.cdm-block-list {
		display: flex;
		flex-direction: column;
	}
	.cdm-insert-row {
		height: 1.25rem;
		display: flex;
		align-items: center;
		opacity: 0.35;
		transition: opacity 0.1s;
	}
	.cdm-insert-row:hover {
		opacity: 1;
	}
	.cdm-block-row {
		border: 1px solid transparent;
		border-radius: 6px;
		position: relative;
	}
	.cdm-block-row--selected {
		border-color: rgba(37, 99, 235, 0.5);
		background: rgba(37, 99, 235, 0.04);
	}
	.cdm-block-select {
		all: unset;
		display: block;
		width: 100%;
		cursor: pointer;
		padding: 4px 8px;
	}
	.cdm-block-toolbar {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 4px 8px 8px;
		font-size: 0.8em;
	}
	.cdm-block-id {
		font-family: ui-monospace, monospace;
		color: var(--cdm-muted, #6b7280);
		margin-right: auto;
	}
	.cdm-block-delete {
		color: #b91c1c;
	}
	.cdm-empty {
		color: var(--cdm-muted, #6b7280);
		font-style: italic;
		padding: 12px 8px;
	}
</style>
