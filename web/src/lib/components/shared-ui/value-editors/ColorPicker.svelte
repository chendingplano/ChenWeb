<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { colorFamilies } from './commonColors';
	import { colorMode, colorPart, withColorMode, withColorPart, type ColorMode } from './values';
	import { rgbToHsv, hsvToRgb, rgbaHex } from './color-math';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';
	type Variant = 'single' | 'light' | 'dark';
	let { id, label, value, options, check, onChange }: ValueEditorProps = $props();
	const mode = $derived(colorMode(value));
	const variants = $derived(
		mode === 'theme' ? (['light', 'dark'] as const) : (['single'] as const)
	);
	let opened = $state<Variant | null>(null),
		palette = $state('common'),
		familyKey = $state(colorFamilies[0].key);
	let h = $state(0),
		s = $state(0),
		v = $state(0),
		a = $state(1),
		x = $state(0),
		y = $state(0),
		width = $state(350),
		height = $state(560);
	let opener: HTMLElement | null = null;
	const family = $derived(colorFamilies.find((f) => f.key === familyKey) ?? colorFamilies[0]);
	const rgb = $derived(hsvToRgb(h, s, v));
	const common = colorFamilies.flatMap((f) => f.colors.slice(0, 3));
	function variantLabel(v: Variant) {
		return v === 'light'
			? m.wpi_color_light()
			: v === 'dark'
				? m.wpi_color_dark()
				: m.wpi_color_single();
	}
	function setPart(variant: Variant, css: string) {
		const next = withColorPart(value, variant, css);
		if (next !== undefined && check?.(next)) return;
		onChange(next);
	}
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
	}
	function read(css: string): [number, number, number, number] {
		if (typeof document === 'undefined') return [0, 0, 0, 1];
		const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true });
		if (!ctx) return [0, 0, 0, 1];
		ctx.fillStyle = '#000';
		ctx.fillStyle = css || '#000';
		ctx.fillRect(0, 0, 1, 1);
		const p = ctx.getImageData(0, 0, 1, 1).data;
		return [p[0], p[1], p[2], p[3] / 255];
	}
	function open(variant: Variant, target: HTMLElement) {
		if (opened === variant) {
			close();
			return;
		}
		opener = target;
		const [r, g, b, alpha] = read(colorPart(value, variant));
		[h, s, v] = rgbToHsv(r, g, b);
		a = alpha;
		const rect = target.getBoundingClientRect();
		x = Math.max(0, Math.min(rect.left, window.innerWidth - Math.min(width, window.innerWidth)));
		y = Math.max(
			0,
			Math.min(rect.bottom + 5, window.innerHeight - Math.min(height, window.innerHeight))
		);
		opened = variant;
	}
	function close() {
		opened = null;
		opener?.focus();
	}
	function apply() {
		if (opened) setPart(opened, rgbaHex(...rgb, a));
	}
	function choose(hex: string) {
		const [r, g, b, alpha] = read('#' + hex);
		[h, s, v] = rgbToHsv(r, g, b);
		a = alpha;
		apply();
	}
	function field(e: PointerEvent) {
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		const update = (ev: PointerEvent) => {
			const r = el.getBoundingClientRect();
			s = Math.max(0, Math.min(1, (ev.clientX - r.left) / r.width));
			v = 1 - Math.max(0, Math.min(1, (ev.clientY - r.top) / r.height));
			apply();
		};
		update(e);
		const end = () => {
			el.removeEventListener('pointermove', update);
			el.removeEventListener('pointerup', end);
			el.removeEventListener('pointercancel', end);
		};
		el.addEventListener('pointermove', update);
		el.addEventListener('pointerup', end);
		el.addEventListener('pointercancel', end);
	}
	function drag(e: PointerEvent) {
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		const sx = e.clientX,
			sy = e.clientY,
			ox = x,
			oy = y;
		const move = (ev: PointerEvent) => {
			x = Math.max(0, Math.min(window.innerWidth - 80, ox + ev.clientX - sx));
			y = Math.max(0, Math.min(window.innerHeight - 40, oy + ev.clientY - sy));
		};
		const end = () => {
			el.removeEventListener('pointermove', move);
			el.removeEventListener('pointerup', end);
			el.removeEventListener('pointercancel', end);
		};
		el.addEventListener('pointermove', move);
		el.addEventListener('pointerup', end);
		el.addEventListener('pointercancel', end);
	}
	function resize(e: PointerEvent) {
		const el = e.currentTarget as HTMLElement;
		const box = el.parentElement!;
		el.setPointerCapture(e.pointerId);
		const sx = e.clientX,
			sy = e.clientY,
			ow = box.offsetWidth,
			oh = box.offsetHeight;
		const move = (ev: PointerEvent) => {
			width = Math.max(280, Math.min(window.innerWidth, ow + ev.clientX - sx));
			height = Math.max(300, Math.min(window.innerHeight, oh + ev.clientY - sy));
		};
		const end = () => {
			el.removeEventListener('pointermove', move);
			el.removeEventListener('pointerup', end);
			el.removeEventListener('pointercancel', end);
		};
		el.addEventListener('pointermove', move);
		el.addEventListener('pointerup', end);
		el.addEventListener('pointercancel', end);
	}
	function arrows(e: KeyboardEvent, resize = false) {
		const d = e.shiftKey ? 20 : 5;
		const dx = e.key === 'ArrowLeft' ? -d : e.key === 'ArrowRight' ? d : 0,
			dy = e.key === 'ArrowUp' ? -d : e.key === 'ArrowDown' ? d : 0;
		if (!dx && !dy) return;
		e.preventDefault();
		if (resize) {
			width = Math.max(280, Math.min(window.innerWidth, width + dx));
			height = Math.max(300, Math.min(window.innerHeight, height + dy));
		} else {
			x = Math.max(0, Math.min(window.innerWidth - 80, x + dx));
			y = Math.max(0, Math.min(window.innerHeight - 40, y + dy));
		}
	}
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape' && opened) close();
	}}
/>
<div class="color-line">
	<select
		{id}
		class="ve-input color-mode"
		aria-label={label}
		value={mode}
		onchange={(e) => {
			if (opened) close();
			onChange(withColorMode(value, e.currentTarget.value as ColorMode));
		}}
		><option value="none">{m.wpi_color_none()}</option><option value="single"
			>{m.wpi_color_single()}</option
		>{#if options.theme || mode === 'theme'}<option value="theme">{m.wpi_color_theme()}</option
			>{/if}</select
	>
	{#if mode !== 'none'}{#each variants as variant (variant)}<div class="color-value">
				{#if variant !== 'single'}<span class="ve-variant">{variantLabel(variant)}</span
					>{/if}<button
					type="button"
					class="color-swatch"
					aria-label={m.wpi_color_picker_label({ field: label, variant: variantLabel(variant) })}
					aria-expanded={opened === variant}
					style:background={colorPart(value, variant) || 'transparent'}
					onclick={(e) => open(variant, e.currentTarget)}
				></button><DraftField
					id={`${id}-${variant}`}
					value={colorPart(value, variant)}
					ariaLabel={m.wpi_color_value_label({ field: label, variant: variantLabel(variant) })}
					format={(v) => (typeof v === 'string' ? v : '')}
					parse={(text) => ({ ok: true, value: text.trim() === '' ? undefined : text.trim() })}
					invalidMessage={() => m.wpi_text_invalid()}
					check={(css) => {
						if (typeof CSS !== 'undefined' && !CSS.supports('color', String(css)))
							return m.wpi_text_invalid();
						const next = withColorPart(value, variant, String(css));
						return check && next !== undefined ? check(next) : null;
					}}
					onChange={(css) => onChange(withColorPart(value, variant, String(css ?? '')))}
				/>
				<span class="color-percent">{Math.round(read(colorPart(value, variant))[3] * 100)}%</span>
			</div>{/each}{/if}
</div>
{#if opened}<div
		class="color-dialog"
		use:portal
		role="dialog"
		aria-label={m.wpi_color_picker_label({ field: label, variant: variantLabel(opened) })}
		style:left={`${x}px`}
		style:top={`${y}px`}
		style:width={`${width}px`}
		style:height={`${height}px`}
	>
		<div class="color-head">
			<button
				type="button"
				class="color-drag"
				aria-label={m.wpi_color_move()}
				onpointerdown={drag}
				onkeydown={(e) => arrows(e)}>{m.wpi_color_custom()}</button
			><button type="button" class="color-close" aria-label={m.wpi_color_close()} onclick={close}
				>×</button
			>
		</div>
		<div class="color-body">
			<div
				class="color-field"
				role="slider"
				tabindex="0"
				aria-label={m.wpi_color_saturation()}
				aria-valuemin="0"
				aria-valuemax="100"
				aria-valuenow={Math.round(s * 100)}
				style:background-color={`hsl(${h} 100% 50%)`}
				onpointerdown={field}
				onkeydown={(e) => {
					if (e.key === 'ArrowLeft') s = Math.max(0, s - 0.01);
					else if (e.key === 'ArrowRight') s = Math.min(1, s + 0.01);
					else if (e.key === 'ArrowUp') v = Math.min(1, v + 0.01);
					else if (e.key === 'ArrowDown') v = Math.max(0, v - 0.01);
					else return;
					e.preventDefault();
					apply();
				}}
			>
				<span class="color-marker" style:left={`${s * 100}%`} style:top={`${(1 - v) * 100}%`}
				></span>
			</div>
			<input
				class="color-hue"
				type="range"
				min="0"
				max="359"
				step="1"
				aria-label={m.wpi_color_hue()}
				value={h}
				oninput={(e) => {
					h = Number(e.currentTarget.value);
					apply();
				}}
			/><input
				class="color-alpha"
				type="range"
				min="0"
				max="1"
				step="0.01"
				aria-label={m.wpi_color_opacity()}
				value={a}
				oninput={(e) => {
					a = Number(e.currentTarget.value);
					apply();
				}}
				style:--color-end={rgbaHex(...rgb, 1)}
			/>
			<div class="color-channels">
				<span>{m.wpi_color_rgb()}</span>{#each rgb as channel, i}<input
						type="number"
						min="0"
						max="255"
						value={channel}
						aria-label={m.wpi_color_channel({ channel: ['R', 'G', 'B'][i] })}
						onchange={(e) => {
							const next = [...rgb] as [number, number, number];
							next[i] = Math.max(0, Math.min(255, Number(e.currentTarget.value)));
							[h, s, v] = rgbToHsv(...next);
							apply();
						}}
					/>{/each}<span>{Math.round(a * 100)}%</span>
			</div>
			<select class="ve-input" aria-label={m.wpi_color_palette()} bind:value={palette}
				><option value="common">{m.wpi_color_common()}</option><option value="family"
					>{m.wpi_color_families()}</option
				></select
			>{#if palette === 'family'}<select
					class="ve-input"
					aria-label={m.wpi_color_family_select({ field: label })}
					bind:value={familyKey}
					>{#each colorFamilies as f}<option value={f.key}>{f.title()}</option>{/each}</select
				>{/if}
			<div class="color-palette">
				{#each palette === 'common' ? common : family.colors as [name, hex], i (hex + name + i)}<button
						type="button"
						title={`${name} #${hex}`}
						aria-label={`${name} #${hex}`}
						style:background={`#${hex}`}
						onclick={() => choose(hex)}
					></button>{/each}
			</div>
		</div>
		<button
			type="button"
			class="color-resize"
			aria-label={m.wpi_color_resize()}
			onpointerdown={resize}
			onkeydown={(e) => arrows(e, true)}
		></button>
	</div>{/if}

<style>
	.color-line {
		display: flex;
		align-items: center;
		gap: 4px;
		overflow-x: auto;
	}
	.color-percent {
		flex: none;
		padding: 0 4px;
		color: var(--cv-text-muted, #777);
		font-size: 11px;
	}
	.color-mode {
		width: auto;
		flex: none;
		max-width: 120px;
	}
	.color-value {
		display: flex;
		align-items: center;
		gap: 4px;
		flex: 1;
		min-width: 125px;
		background: var(--cv-surface, #f3f3f3);
		border-radius: 6px;
		padding: 3px;
	}
	.color-value :global(.ve-input) {
		min-width: 75px;
		border: 0;
		background: transparent;
	}
	.color-swatch {
		width: 25px;
		height: 25px;
		border: 1px solid #aaa;
		border-radius: 4px;
		flex: none;
		cursor: pointer;
	}
	.color-dialog {
		position: fixed;
		z-index: 9999;
		min-width: 280px;
		min-height: 300px;
		max-width: 100vw;
		max-height: 100vh;
		resize: both;
		overflow: auto;
		box-sizing: border-box;
		background: var(--cv-surface, #fff);
		color: var(--cv-text, #222);
		border: 1px solid var(--cv-border, #ddd);
		border-radius: 14px;
		box-shadow: 0 8px 30px #0003;
	}
	.color-head {
		display: flex;
		align-items: center;
		border-bottom: 1px solid #ddd;
		padding: 9px 14px;
	}
	.color-drag {
		flex: 1;
		text-align: left;
		cursor: move;
		touch-action: none;
		border: 0;
		background: transparent;
		color: inherit;
		font-weight: 600;
		padding: 5px;
	}
	.color-close {
		border: 0;
		background: transparent;
		color: inherit;
		font-size: 24px;
		cursor: pointer;
	}
	.color-body {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.color-field {
		position: relative;
		min-height: 140px;
		aspect-ratio: 1.35;
		border-radius: 8px;
		touch-action: none;
		background-image:
			linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, transparent);
		cursor: crosshair;
	}
	.color-marker {
		position: absolute;
		width: 15px;
		height: 15px;
		transform: translate(-50%, -50%);
		border: 3px solid white;
		border-radius: 50%;
		box-shadow: 0 1px 4px #0008;
		pointer-events: none;
	}
	.color-hue,
	.color-alpha {
		width: 100%;
		height: 18px;
		appearance: none;
		border-radius: 20px;
		cursor: pointer;
	}
	.color-hue {
		background: linear-gradient(to right, red, #ff0, #0f0, #0ff, blue, #f0f, red);
	}
	.color-alpha {
		background:
			linear-gradient(to right, transparent, var(--color-end)),
			repeating-conic-gradient(#ddd 0 25%, #fff 0 50%) 0/12px 12px;
	}
	.color-hue::-webkit-slider-thumb,
	.color-alpha::-webkit-slider-thumb {
		appearance: none;
		width: 20px;
		height: 20px;
		background: white;
		border: 2px solid #777;
		border-radius: 50%;
	}
	.color-channels {
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.color-channels input {
		width: 48px;
		min-width: 0;
		padding: 5px;
		border: 1px solid #ddd;
		border-radius: 4px;
		background: transparent;
		color: inherit;
	}
	.color-palette {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(25px, 1fr));
		gap: 8px;
	}
	.color-palette button {
		height: 25px;
		border: 1px solid #0002;
		border-radius: 4px;
		cursor: pointer;
	}
	.color-resize {
		position: absolute;
		right: 0;
		bottom: 0;
		width: 20px;
		height: 20px;
		opacity: 0.5;
		cursor: nwse-resize;
		touch-action: none;
		background: linear-gradient(
			135deg,
			transparent 50%,
			#888 51%,
			transparent 57%,
			#888 62%,
			transparent 68%
		);
		border: 0;
	}
</style>
