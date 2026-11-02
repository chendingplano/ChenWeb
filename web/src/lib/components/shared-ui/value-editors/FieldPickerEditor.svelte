<!-- field-picker: fields of the table in options.table, chosen with Pick Fields (module
     20261029-02); the value is the list of fields, as in a data source. -->
<script lang="ts">
	import FieldPicker from '../database-picker/FieldPicker.svelte';
	import type { ValueEditorProps } from './types';

	let { id, value, options, onChange }: ValueEditorProps = $props();
</script>

<div {id}>
	<FieldPicker
		table={options.table ?? ''}
		fields={Array.isArray(value) ? value : []}
		onChange={(fields) => onChange(fields.length ? fields : undefined)}
	/>
</div>
