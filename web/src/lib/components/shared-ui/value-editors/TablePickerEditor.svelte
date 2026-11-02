<!-- table-picker: a table, chosen with Pick Table (module 20261029-01); the value is its
     database.schema.table reference. -->
<script lang="ts">
	import TablePicker from '../database-picker/TablePicker.svelte';
	import type { ValueEditorProps } from './types';

	let { id, value, onChange }: ValueEditorProps = $props();
</script>

<div {id}>
	<TablePicker value={typeof value === 'string' ? value : ''} onSelect={(table) => onChange(table || undefined)} />
</div>
