<!-- Where a list gets its rows: a table (Pick Table), its fields (Pick
     Fields), a row limit, the action and result, and the sort order. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Json, JsonObject } from '$lib/wpi/types';
	import TablePicker from '../database-picker/TablePicker.svelte';
	import FieldPicker from '../database-picker/FieldPicker.svelte';
	import { withListSourceDefaults, withSelectedTable, withSourceFields, withSourceProperty } from '../canvas/element-attributes';
	import SortOrderEditor from './SortOrderEditor.svelte';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, label, value, options, onChange }: ValueEditorProps = $props();
	const source = $derived((value && typeof value === 'object' && !Array.isArray(value) ? value : {}) as JsonObject);
	const table = $derived(String(source.table ?? ''));

	// The helpers edit a node's attributes; the source is the one attribute here.
	function apply(attrs: JsonObject) {
		onChange(attrs.source);
	}
	function setProperty(key: string, next: Json | undefined) {
		apply(withSourceProperty({ source }, undefined, key, next));
	}
</script>

<div {id} class="ve-grid" style="grid-template-columns:minmax(0,1fr) minmax(0,2fr); align-items:center">
	<label class="ve-sub" for={`${id}-limit`}>{m.propertiespanel_source_limit()}</label>
	<input id={`${id}-limit`} class="ve-input" type="number" min="1" step="1" value={source.limit ?? ''}
		oninput={(e) => setProperty('limit', e.currentTarget.value === '' ? undefined : Number(e.currentTarget.value))} />
	<span class="ve-sub">{m.propertiespanel_source_table()}</span>
	<TablePicker value={table} onSelect={(next) => apply(withSelectedTable({ source }, undefined, next))} />
	<span class="ve-sub">{m.propertiespanel_source_fields()}</span>
	<FieldPicker {table} fields={Array.isArray(source.fields) ? source.fields : []}
		onChange={(fields) => apply(withListSourceDefaults(withSourceFields({ source }, undefined, fields), undefined))} />
	<label class="ve-sub" for={`${id}-action`}>{m.propertiespanel_source_action()}</label>
	<select id={`${id}-action`} class="ve-input" value={String(source.action || (table ? 'load' : ''))} onchange={(e) => setProperty('action', e.currentTarget.value)}>
		<option value="">{m.propertiespanel_select_action()}</option>
		<option value="load">{m.propertiespanel_action_load()}</option>
		<option value="query">{m.propertiespanel_action_query()}</option>
	</select>
	<label class="ve-sub" for={`${id}-result`}>{m.propertiespanel_source_result()}</label>
	<select id={`${id}-result`} class="ve-input" value={String(source.result || (table ? 'list' : ''))} onchange={(e) => setProperty('result', e.currentTarget.value)}>
		<option value="">{m.propertiespanel_select_result()}</option>
		<option value="list">{m.propertiespanel_result_list()}</option>
	</select>
</div>
<span class="ve-sub">{m.propertiespanel_order_by()}</span>
<SortOrderEditor id={`${id}-order`} {label} value={source.order_by} {options} onChange={(order) => setProperty('order_by', order)} />
