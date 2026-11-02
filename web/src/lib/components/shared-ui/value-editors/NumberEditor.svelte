<!-- A number within the property's limits; empty unsets it. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { parseNumber } from './values';
	import type { ValueEditorProps } from './types';

	let { id, value, options, check, onChange }: ValueEditorProps = $props();
	const limits = $derived(
		options.min !== undefined && options.max !== undefined
			? m.wpi_prop_number_range({ min: options.min, max: options.max })
			: options.min !== undefined
				? m.wpi_prop_number_min({ min: options.min })
				: m.wpi_prop_number()
	);
</script>

<DraftField
	{id}
	{value}
	inputmode={options.integer ? 'numeric' : 'decimal'}
	format={(v) => (v === undefined || v === null ? '' : String(v))}
	parse={(text) => parseNumber(text, options)}
	invalidMessage={() => (options.integer ? m.wpi_prop_whole_number({ limits }) : limits)}
	{check}
	{onChange}
/>
