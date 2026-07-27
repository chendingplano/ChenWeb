<script lang="ts">
	// Editable table block (task 6.1): column add/remove/retitle/align, row
	// add/remove, per-cell inline editing, and an optional caption.
	//
	// block is $bindable (see BlockView.svelte's doc comment on its own
	// block prop for why: this component also binds into block's nested
	// properties via InlineEditor, and Svelte's ownership check expects the
	// whole chain declared, not just deep $state reactivity happening to
	// make it work). Structural operations still merge table-ops.ts's pure
	// result back via Object.assign rather than reassigning block wholesale
	// -- there is no parent array index to write back into from here,
	// unlike BlockList's own insert/delete/move on the top-level array.
	import type { Block } from './types.js';
	import InlineEditor from './InlineEditor.svelte';
	import {
		addColumn,
		removeColumn,
		renameColumnTitle,
		setColumnAlign,
		addRow,
		removeRow
	} from './table-ops.js';

	let { block = $bindable() }: { block: Block } = $props();

	// Safety net for rows loaded without every declared column's key present
	// (spec §1.2 rule 5 explicitly allows this -- "a missing key renders as
	// an empty cell"). table-ops.ts's own addColumn/addRow already backfill
	// every case *they* create; this only covers data arriving from outside
	// (a loaded document) so every cell has a real array to bind to.
	$effect(() => {
		for (const row of block.rows ?? []) {
			for (const col of block.columns ?? []) {
				if (!(col.key in row.cells)) {
					row.cells[col.key] = [];
				}
			}
		}
	});

	function doAddColumn() {
		Object.assign(block, addColumn(block));
	}
	function doRemoveColumn(key: string) {
		Object.assign(block, removeColumn(block, key));
	}
	function doRenameTitle(key: string, title: string) {
		Object.assign(block, renameColumnTitle(block, key, title));
	}
	function doSetAlign(key: string, align: string) {
		Object.assign(block, setColumnAlign(block, key, align));
	}
	function doAddRow() {
		Object.assign(block, addRow(block));
	}
	function doRemoveRow(index: number) {
		Object.assign(block, removeRow(block, index));
	}
	function addCaption() {
		block.caption = [];
	}

	let columnCount = $derived((block.columns ?? []).length);
</script>

<div class="cdm-table-editor">
	{#if block.caption}
		<div class="cdm-table-caption"><InlineEditor bind:content={block.caption} as="plain" /></div>
	{:else}
		<button type="button" class="cdm-table-add-caption" onclick={addCaption}>+ Caption</button>
	{/if}

	<table class="cdm-table">
		<thead>
			<tr>
				{#each block.columns ?? [] as col (col.key)}
					<th>
						<input
							type="text"
							class="cdm-table-title-input"
							value={col.title}
							oninput={(e) => doRenameTitle(col.key, e.currentTarget.value)}
						/>
						<select
							value={col.align ?? ''}
							onchange={(e) => doSetAlign(col.key, e.currentTarget.value)}
						>
							<option value="">left</option>
							<option value="center">center</option>
							<option value="right">right</option>
						</select>
						<button
							type="button"
							title="Remove column"
							onclick={() => doRemoveColumn(col.key)}
							disabled={columnCount <= 1}>×</button
						>
					</th>
				{/each}
				<th class="cdm-table-add-cell"
					><button type="button" onclick={doAddColumn}>+ Column</button></th
				>
			</tr>
		</thead>
		<tbody>
			{#each block.rows ?? [] as row, i (i)}
				<tr>
					{#each block.columns ?? [] as col (col.key)}
						<td><InlineEditor bind:content={row.cells[col.key]} as="plain" /></td>
					{/each}
					<td class="cdm-table-add-cell"
						><button type="button" title="Remove row" onclick={() => doRemoveRow(i)}>×</button></td
					>
				</tr>
			{/each}
			<tr>
				<td colspan={columnCount + 1} class="cdm-table-add-cell">
					<button type="button" onclick={doAddRow}>+ Row</button>
				</td>
			</tr>
		</tbody>
	</table>
</div>

<style>
	.cdm-table-caption {
		font-size: 0.85em;
		color: var(--cdm-muted, #6b7280);
		margin-bottom: 4px;
	}
	.cdm-table-add-caption {
		font-size: 0.75em;
		margin-bottom: 4px;
	}
	.cdm-table-title-input {
		border: none;
		background: transparent;
		font-weight: 600;
		width: 100%;
	}
	.cdm-table-add-cell {
		text-align: center;
	}
	.cdm-table-add-cell button {
		font-size: 0.75em;
		opacity: 0.6;
	}
	.cdm-table-add-cell button:hover {
		opacity: 1;
	}
</style>
