<!-- The columns a list is sorted by, each ascending or descending, in order. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Json, JsonObject } from '$lib/wpi/types';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, onChange }: ValueEditorProps = $props();
	const order = $derived(
		(Array.isArray(value) ? value : []).filter((v): v is JsonObject => !!v && typeof v === 'object' && !Array.isArray(v))
	);
	let adding = $state(false);
	let draft = $state({ column: '', direction: 'asc' });
	const columns = 'grid-template-columns:minmax(0,1fr) minmax(0,1fr) 25px';

	function set(next: Json[]) {
		onChange(next.length ? next : undefined);
	}
	function add() {
		set([...order, { column: draft.column.trim(), direction: draft.direction }]);
		draft = { column: '', direction: 'asc' };
		adding = false;
	}
</script>

<div {id}>
	<div class="ve-row" style="justify-content:flex-end; margin-bottom:4px">
		<button type="button" class="ve-button" onclick={() => (adding = true)}>{m.propertiespanel_add_order()}</button>
		{#if adding}<button type="button" class="ve-button" onclick={() => { adding = false; draft = { column: '', direction: 'asc' }; }}>{m.wpi_create_element_cancel()}</button>{/if}
	</div>
	{#if adding}
		<div class="ve-grid" style={columns}>
			<input class="ve-input" aria-label={m.propertiespanel_source_column()} placeholder={m.propertiespanel_source_column()} bind:value={draft.column} />
			<select class="ve-input" aria-label={m.propertiespanel_direction()} bind:value={draft.direction}>
				<option value="asc">{m.propertiespanel_direction_asc()}</option>
				<option value="desc">{m.propertiespanel_direction_desc()}</option>
			</select>
			<button type="button" class="ve-button" aria-label={m.wpi_json_apply()} disabled={!draft.column.trim()} onclick={add}>+</button>
		</div>
	{/if}
	{#each order as item, index (index)}
		<div class="ve-grid" style={columns}>
			<input class="ve-input" aria-label={m.propertiespanel_source_column()} value={String(item.column ?? '')}
				oninput={(e) => set(order.map((v, i) => (i === index ? { ...item, column: e.currentTarget.value } : v)))} />
			<select class="ve-input" aria-label={m.propertiespanel_direction()} value={String(item.direction ?? 'asc')}
				onchange={(e) => set(order.map((v, i) => (i === index ? { ...item, direction: e.currentTarget.value } : v)))}>
				<option value="asc">{m.propertiespanel_direction_asc()}</option>
				<option value="desc">{m.propertiespanel_direction_desc()}</option>
			</select>
			<button type="button" class="ve-button" aria-label={m.propertiespanel_remove_order()} onclick={() => set(order.filter((_, i) => i !== index))}>×</button>
		</div>
	{/each}
</div>
