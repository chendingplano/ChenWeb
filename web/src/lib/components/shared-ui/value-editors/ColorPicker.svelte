<!-- A colour: none, one CSS colour, or (with options.theme) separate light and
     dark colours. The swatch opens a dialog to pick the colour by the colour
     picker or from the colour families; the colour can also be typed as CSS. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { colorFamilies } from './commonColors';
	import { colorMode, colorPart, withColorMode, withColorPart, type ColorMode } from './values';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	type Variant = 'single' | 'light' | 'dark';
	type PickMethod = 'picker' | 'family';

	let { id, label, value, options, check, onChange }: ValueEditorProps = $props();
	const mode = $derived(colorMode(value));
	const variants = $derived(mode === 'theme' ? (['light', 'dark'] as const) : (['single'] as const));
	let pickMethod = $state<PickMethod>('picker');
	let familyKey = $state(colorFamilies[0].key);
	let openVariant = $state<Variant | null>(null);
	const family = $derived(colorFamilies.find((f) => f.key === familyKey) ?? colorFamilies[0]);

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
	function variantLabel(variant: Variant): string {
		return variant === 'light' ? m.wpi_color_light() : variant === 'dark' ? m.wpi_color_dark() : m.wpi_color_single();
	}
	function setPart(variant: Variant, css: string) {
		const next = withColorPart(value, variant, css);
		if (next !== undefined && check?.(next)) return;
		onChange(next);
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && (openVariant = null)} />

<select {id} class="ve-input" aria-label={label} value={mode} onchange={(e) => onChange(withColorMode(value, e.currentTarget.value as ColorMode))}>
	<option value="none">{m.wpi_color_none()}</option>
	<option value="single">{m.wpi_color_single()}</option>
	{#if options.theme || mode === 'theme'}<option value="theme">{m.wpi_color_theme()}</option>{/if}
</select>
{#if mode !== 'none'}
	{#each variants as variant (variant)}
		<div class="ve-row" style="margin-top:4px; position:relative">
			{#if variant !== 'single'}<span class="ve-variant">{variantLabel(variant)}</span>{/if}
			<button
				type="button"
				class="ve-color"
				aria-label={m.wpi_color_picker_label({ field: label, variant: variantLabel(variant) })}
				aria-expanded={openVariant === variant}
				style={`background:${pickerValue(colorPart(value, variant))}`}
				onclick={() => (openVariant = openVariant === variant ? null : variant)}
			></button>
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
			{#if openVariant === variant}
				<div
					role="dialog"
					aria-label={m.wpi_color_picker_label({ field: label, variant: variantLabel(variant) })}
					style="position:absolute; top:100%; left:0; z-index:10; margin-top:4px; width:240px; padding:8px; display:flex; flex-direction:column; gap:6px; border:1px solid var(--cv-border, #d1d5db); border-radius:6px; background:var(--cv-surface, #fff); box-shadow:0 4px 12px rgba(0,0,0,0.2)"
				>
					<select
						class="ve-input"
						aria-label={m.wpi_color_pick_method({ field: label })}
						value={pickMethod}
						onchange={(e) => (pickMethod = e.currentTarget.value as PickMethod)}
					>
						<option value="picker">{m.wpi_color_pick_picker()}</option>
						<option value="family">{m.wpi_color_pick_family()}</option>
					</select>
					{#if pickMethod === 'picker'}
						<input
							id={`${id}-${variant}-picker`}
							class="ve-color"
							style="width:100%; height:32px"
							type="color"
							aria-label={m.wpi_color_picker_label({ field: label, variant: variantLabel(variant) })}
							value={pickerValue(colorPart(value, variant))}
							oninput={(e) => setPart(variant, e.currentTarget.value)}
						/>
					{:else}
						<select
							class="ve-input"
							aria-label={m.wpi_color_family_select({ field: label })}
							value={familyKey}
							onchange={(e) => (familyKey = e.currentTarget.value)}
						>
							{#each colorFamilies as f (f.key)}
								<option value={f.key}>{f.title()}</option>
							{/each}
						</select>
						<div style="display:flex; flex-direction:column; max-height:220px; overflow-y:auto">
							{#each family.colors as [name, hex] (hex + name)}
								<button
									type="button"
									title={`${name} #${hex}`}
									style="display:flex; align-items:center; gap:8px; padding:3px 4px; border:0; background:transparent; color:var(--cv-text, inherit); font-size:11px; text-align:left; cursor:pointer"
									onclick={() => {
										setPart(variant, '#' + hex);
										openVariant = null;
									}}
								>
									<span
										style={`width:18px; height:18px; flex:none; border-radius:3px; border:1px solid var(--cv-border, #d1d5db); background:#${hex}`}
									></span>
									<span style="flex:1">{name}</span>
									<span style="color:var(--cv-text-muted, inherit)">#{hex}</span>
								</button>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/each}
{/if}
