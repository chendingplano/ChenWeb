<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { colorMode, withColorMode, type ColorMode } from './values';
	import type { Json } from '$lib/wpi/types';
	import './value-editors.css';
	let {
		id,
		label,
		value,
		theme,
		onChange
	}: {
		id: string;
		label: string;
		value: Json | undefined;
		theme?: boolean;
		onChange: (value: Json | undefined) => void;
	} = $props();
	const mode = $derived(colorMode(value));
</script>

<select
	{id}
	class="ve-input color-mode"
	aria-label={label}
	value={mode}
	onchange={(e) => onChange(withColorMode(value, e.currentTarget.value as ColorMode))}
>
	<option value="none">{m.wpi_color_none()}</option>
	<option value="single">{m.wpi_color_single()}</option>
	{#if theme || mode === 'theme'}<option value="theme">{m.wpi_color_theme()}</option>{/if}
</select>

<style>
	.color-mode {
		width: auto;
		max-width: 150px;
		flex: none;
	}
</style>
