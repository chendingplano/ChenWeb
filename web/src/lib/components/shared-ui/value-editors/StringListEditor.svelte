<!-- A list of words typed as comma-separated text; empty unsets the value. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { parseStringList, stringListText } from './values';
	import type { ValueEditorProps } from './types';

	let { id, value, check, onChange }: ValueEditorProps = $props();
</script>

<DraftField
	{id}
	{value}
	format={stringListText}
	parse={parseStringList}
	invalidMessage={() => m.wpi_text_invalid()}
	{check}
	{onChange}
/>
