<!-- A list's columns: the label, field and width of each, with add and remove. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Json, JsonObject } from '$lib/wpi/types';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, onChange }: ValueEditorProps = $props();
	const columns = $derived(
		(Array.isArray(value) ? value : []).filter((v): v is JsonObject => !!v && typeof v === 'object' && !Array.isArray(v))
	);
	let adding = $state(false);
	let draft = $state({ label: '', field: '', width: '' });
	const grid = 'grid-template-columns:minmax(0,1fr) minmax(0,1fr) minmax(0,0.8fr) 25px';

	function set(next: Json[]) {
		onChange(next.length ? next : undefined);
	}
	function edit(index: number, key: 'label' | 'field' | 'width', text: string) {
		set(
			columns.map((c, i) => {
				if (i !== index) return c;
				const next: JsonObject = { ...c, [key]: text };
				if (key === 'width' && text === '') delete next.width;
				return next;
			})
		);
	}
	function add() {
		const width = draft.width.trim();
		set([...columns, { label: draft.label.trim(), field: draft.field.trim(), ...(width ? { width } : {}) }]);
		draft = { label: '', field: '', width: '' };
		adding = false;
	}
</script>

<div {id}>
	<div class="ve-row" style="justify-content:flex-end; margin-bottom:4px">
		<button type="button" class="ve-button" onclick={() => (adding = true)}>{m.propertiespanel_add_column()}</button>
		{#if adding}<button type="button" class="ve-button" onclick={() => { adding = false; draft = { label: '', field: '', width: '' }; }}>{m.wpi_create_element_cancel()}</button>{/if}
	</div>
	{#if adding}
		<div class="ve-grid" style={grid}>
			<input class="ve-input" aria-label={m.propertiespanel_column_label()} placeholder={m.propertiespanel_column_label()} bind:value={draft.label} />
			<input class="ve-input" aria-label={m.propertiespanel_column_field()} placeholder={m.propertiespanel_column_field()} bind:value={draft.field} />
			<input class="ve-input" aria-label={m.propertiespanel_column_width()} placeholder={m.propertiespanel_column_width()} bind:value={draft.width} />
			<button type="button" class="ve-button" aria-label={m.wpi_json_apply()} disabled={!draft.label.trim() || !draft.field.trim()} onclick={add}>+</button>
		</div>
	{/if}
	{#each columns as column, index (index)}
		<div class="ve-grid" style={grid}>
			<input class="ve-input" aria-label={m.propertiespanel_column_label()} value={String(column.label ?? '')} oninput={(e) => edit(index, 'label', e.currentTarget.value)} />
			<input class="ve-input" aria-label={m.propertiespanel_column_field()} value={String(column.field ?? '')} oninput={(e) => edit(index, 'field', e.currentTarget.value)} />
			<input class="ve-input" aria-label={m.propertiespanel_column_width()} placeholder={m.propertiespanel_column_width()} value={String(column.width ?? '')} oninput={(e) => edit(index, 'width', e.currentTarget.value)} />
			<button type="button" class="ve-button" aria-label={m.propertiespanel_remove_column()} onclick={() => set(columns.filter((_, i) => i !== index))}>×</button>
		</div>
	{/each}
</div>
