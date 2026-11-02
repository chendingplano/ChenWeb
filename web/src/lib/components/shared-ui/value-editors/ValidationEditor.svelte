<!-- An input's validation rules: minimum, maximum and step for numbers, or
     minimum length and pattern for text. Rules of the other kind are kept. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Json, JsonObject } from '$lib/wpi/types';
	import DraftField from './DraftField.svelte';
	import { parseNumber, parseText } from './values';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, options, onChange }: ValueEditorProps = $props();
	const rules = $derived((value && typeof value === 'object' && !Array.isArray(value) ? value : {}) as JsonObject);
	const names = $derived(options.numberRules ? ['min', 'max', 'step'] : ['min_length', 'pattern']);
	const labels: Record<string, () => string> = {
		min: m.wpi_input_attr_min,
		max: m.wpi_input_attr_max,
		step: m.wpi_input_attr_step,
		min_length: m.wpi_input_attr_min_length,
		pattern: m.wpi_input_attr_pattern
	};
	function setRule(name: string, next: Json | undefined) {
		const out: JsonObject = { ...rules };
		if (next === undefined) delete out[name];
		else out[name] = next;
		onChange(Object.keys(out).length ? out : undefined);
	}
</script>

<div {id}>
	{#each names as name (name)}
		<label class="ve-sub" for={`${id}-${name}`}>{labels[name]()}</label>
		<DraftField
			id={`${id}-${name}`}
			value={rules[name]}
			inputmode={name === 'pattern' ? undefined : 'decimal'}
			format={(v) => (v === undefined || v === null ? '' : String(v))}
			parse={(text) => (name === 'pattern' ? parseText(text) : parseNumber(text, { min: name === 'min_length' ? 0 : undefined, integer: name === 'min_length' }))}
			invalidMessage={() => m.wpi_prop_number()}
			onChange={(next) => setRule(name, next)}
		/>
	{/each}
</div>
