<!-- A colour: none, one CSS colour, or (with options.theme) separate light and
     dark colours. Each colour is chosen with the picker or typed as CSS. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { colorMode, colorPart, withColorMode, withColorPart, type ColorMode } from './values';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, label, value, options, check, onChange }: ValueEditorProps = $props();
	const mode = $derived(colorMode(value));
	const variants = $derived(mode === 'theme' ? (['light', 'dark'] as const) : (['single'] as const));

	// The native picker takes opaque #rrggbb; the typed CSS text is kept as it is.
	function pickerValue(css: string): string {
		if (typeof document === 'undefined' || !css) return '#000000';
		const ctx = document.createElement('canvas').getContext('2d');
		if (!ctx) return '#000000';
		ctx.fillStyle = '#000000';
		ctx.fillStyle = css;
		const color = ctx.fillStyle;
		if (/^#[0-9a-f]{6}$/i.test(color)) return color;
		const rgb = color.match(/^rgba?\(\s*(\d+),\s*(\d+),\s*(\d+)/);
		return rgb ? '#' + rgb.slice(1, 4).map((n) => Number(n).toString(16).padStart(2, '0')).join('') : '#000000';
	}
	function variantLabel(variant: 'single' | 'light' | 'dark'): string {
		return variant === 'light' ? m.wpi_color_light() : variant === 'dark' ? m.wpi_color_dark() : m.wpi_color_single();
	}
	function setPart(variant: 'single' | 'light' | 'dark', css: string) {
		const next = withColorPart(value, variant, css);
		if (next !== undefined && check?.(next)) return;
		onChange(next);
	}
</script>

<select {id} class="ve-input" aria-label={label} value={mode} onchange={(e) => onChange(withColorMode(value, e.currentTarget.value as ColorMode))}>
	<option value="none">{m.wpi_color_none()}</option>
	<option value="single">{m.wpi_color_single()}</option>
	{#if options.theme || mode === 'theme'}<option value="theme">{m.wpi_color_theme()}</option>{/if}
</select>
{#if mode !== 'none'}
	{#each variants as variant (variant)}
		<div class="ve-row" style="margin-top:4px">
			{#if variant !== 'single'}<span class="ve-variant">{variantLabel(variant)}</span>{/if}
			<input
				id={`${id}-${variant}-picker`}
				class="ve-color"
				type="color"
				aria-label={m.wpi_color_picker_label({ field: label, variant: variantLabel(variant) })}
				value={pickerValue(colorPart(value, variant))}
				oninput={(e) => setPart(variant, e.currentTarget.value)}
			/>
			<div style="flex:1; min-width:0">
				<DraftField
					id={`${id}-${variant}`}
					value={colorPart(value, variant)}
					ariaLabel={m.wpi_color_value_label({ field: label, variant: variantLabel(variant) })}
					format={(v) => (typeof v === 'string' ? v : '')}
					parse={(text) => ({ ok: true, value: text.trim() === '' ? undefined : text.trim() })}
					invalidMessage={() => m.wpi_text_invalid()}
					check={(css) => {
						const next = withColorPart(value, variant, String(css));
						return check && next !== undefined ? check(next) : null;
					}}
					onChange={(css) => onChange(withColorPart(value, variant, String(css ?? '')))}
				/>
			</div>
		</div>
	{/each}
{/if}
