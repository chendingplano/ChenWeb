<!-- A length: an allowed keyword (auto, elastic) or a number with an allowed
     unit; a number typed without a unit means px. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { parseLength } from './values';
	import type { ValueEditorProps } from './types';

	let { id, value, options, check, onChange }: ValueEditorProps = $props();
	const keywords = $derived(options.keywords ?? []);
	const units = $derived(options.units ?? ['px', 'rem', 'em', '%']);
</script>

<DraftField
	{id}
	{value}
	format={(v) => (v === undefined || v === null ? '' : String(v))}
	parse={(text) => parseLength(text, keywords, units)}
	invalidMessage={() => m.wpi_prop_invalid_length({ allowed: [...units, ...keywords].join(', ') })}
	{check}
	commitOnBlur={options.commitOnBlur}
	{onChange}
/>
