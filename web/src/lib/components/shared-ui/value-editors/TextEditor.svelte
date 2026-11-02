<!-- One line of text; empty unsets the value. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { jsonText, parseText } from './values';
	import type { ValueEditorProps } from './types';

	let { id, value, options, check, onChange }: ValueEditorProps = $props();
</script>

<DraftField
	{id}
	{value}
	format={(v) => (typeof v === 'string' ? v : jsonText(v))}
	parse={parseText}
	invalidMessage={() => m.wpi_text_invalid()}
	{check}
	commitOnBlur={options.commitOnBlur}
	{onChange}
/>
