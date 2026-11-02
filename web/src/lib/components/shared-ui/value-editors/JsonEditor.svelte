<!-- Any value as JSON text: text that parses as JSON is stored as JSON,
     anything else as the text itself; empty unsets the value. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { jsonText, parseJsonText } from './values';
	import type { ValueEditorProps } from './types';

	let { id, value, options, check, onChange }: ValueEditorProps = $props();
</script>

<DraftField
	{id}
	{value}
	multiline
	rows={options.rows ?? 1}
	format={jsonText}
	parse={parseJsonText}
	invalidMessage={() => m.wpi_text_invalid()}
	{check}
	commitOnBlur={options.commitOnBlur}
	{onChange}
/>
