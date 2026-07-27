<script lang="ts">
	// A block-type picker plus an "insert" button, used by BlockList between
	// (and before/after) every block. Kept as its own component only because
	// BlockList repeats it once per gap; there is no state here beyond the
	// bound type choice, which the caller owns.
	import { BLOCK_TYPES, type BlockType } from './types.js';

	let {
		choice = $bindable(),
		onInsert,
		label
	}: { choice: BlockType; onInsert: () => void; label: string } = $props();
</script>

<div class="cdm-insert-control">
	<select aria-label={label} bind:value={choice}>
		{#each BLOCK_TYPES as type (type)}
			<option value={type}>{type}</option>
		{/each}
	</select>
	<button type="button" onclick={onInsert} title={label}>+ Insert</button>
</div>

<style>
	.cdm-insert-control {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 0.75em;
	}
</style>
