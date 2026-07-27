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
	import { setContext } from 'svelte';
	import type { Block, BlockType } from './types.js';
	import { BLOCK_TYPES } from './types.js';
	import BlockView from './BlockView.svelte';
	import InsertControl from './InsertControl.svelte';
	import {
		createIdAllocator,
		collectBlockIds,
		ALLOCATE_ID_CONTEXT_KEY,
		type BlockIdHint
	} from './block-id.js';
	import { createDefaultBlock } from './block-defaults.js';
	import {
		insertBlockAt,
		deleteBlockById,
		moveBlock,
		changeContentBlockType,
		isContentBearingType,
		type ContentBearingType
	} from './block-ops.js';

	// editable false is the frozen/published read-only state (design D4/D8,
	// task group 7): the caller (DocumentEditor) sets this once a save or
	// publish attempt reveals the document is published, hiding every
	// mutating control while still showing the block content underneath.
	//
	// blockErrors attributes a save-time validation violation or block-slug
	// conflict (task 7.4) to the block that caused it, keyed by block id;
	// DocumentEditor computes it from document-editor-ops.ts's
	// attributeToBlocks. Read-only here -- this component only displays it,
	// it does not clear it (DocumentEditor clears it on the next save
	// attempt).
	let {
		blocks = $bindable(),
		editable = true,
		blockErrors = new Map<string, string[]>(),
		emptyMessage = 'This document has no blocks yet.'
	}: {
		blocks: Block[];
		editable?: boolean;
		blockErrors?: Map<string, string[]>;
		emptyMessage?: string;
	} = $props();

	// Provided to every nested BlockView so a block created anywhere in the
	// tree (a new list item, say) gets an id unique against the *whole*
	// document, not just its own subtree -- recomputed from the current
	// `blocks` on every call, never a stale snapshot.
	setContext(ALLOCATE_ID_CONTEXT_KEY, (hint: BlockIdHint) =>
		createIdAllocator(collectBlockIds(blocks))(hint)
	);

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
	{#if editable}
		<div class="cdm-insert-row">
			<InsertControl
				bind:choice={insertTypeChoice}
				onInsert={() => insertAt(0)}
				label="Insert at top"
			/>
		</div>
	{/if}

	{#each blocks as block, index (block.id)}
		<div
			class="cdm-block-row"
			class:cdm-block-row--selected={editable && selectedId === block.id}
			class:cdm-block-row--error={(blockErrors.get(block.id)?.length ?? 0) > 0}
		>
			<!--
				A plain div, not a button: this hosts InlineEditor for
				content-bearing blocks, which mounts a real contenteditable
				ProseMirror view (task group 5). A <button> ancestor breaks
				focus and cursor placement for editable descendant content,
				so selection is a bubbling click handler here instead --
				clicking into the editable text both places the cursor
				(native browser behavior, unaffected by this handler) and
				reveals the block's toolbar.
			-->
			<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
			<div class="cdm-block-select" onclick={() => editable && selectBlock(block.id)}>
				<BlockView bind:block={blocks[index]} {editable} />
			</div>

			{#if blockErrors.get(block.id)?.length}
				<div class="cdm-block-errors">
					{#each blockErrors.get(block.id) ?? [] as msg (msg)}
						<p>{msg}</p>
					{/each}
				</div>
			{/if}

			{#if editable && selectedId === block.id}
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

		{#if editable}
			<div class="cdm-insert-row">
				<InsertControl
					bind:choice={insertTypeChoice}
					onInsert={() => insertAt(index + 1)}
					label="Insert here"
				/>
			</div>
		{/if}
	{/each}

	{#if blocks.length === 0}
		<p class="cdm-empty">{emptyMessage}</p>
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
		/*
		 * position+z-index, not just a higher stacking context by luck:
		 * confirmed live (task group 8's full-loop test) that a focused
		 * block's floating InlineEditor toolbar (position: absolute,
		 * z-index: 10, floats above its own editor) can render directly on
		 * top of the "Insert at top" row immediately preceding the first
		 * block, hiding it completely and swallowing clicks meant for it.
		 * Outranking that toolbar's z-index here keeps every insert row
		 * visible and clickable regardless of which block currently has
		 * focus.
		 */
		position: relative;
		z-index: 11;
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
	.cdm-block-row--error {
		border-color: rgba(220, 38, 38, 0.5);
		background: rgba(220, 38, 38, 0.04);
	}
	.cdm-block-errors {
		color: #b91c1c;
		font-size: 0.8em;
		padding: 0 8px 6px;
	}
	.cdm-block-errors p {
		margin: 0;
	}
	.cdm-block-select {
		display: block;
		width: 100%;
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
