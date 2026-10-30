<script lang="ts">
	import { tick } from 'svelte';
	import { Info, CircleHelp, CircleCheck, TriangleAlert, FileText, User } from '@lucide/svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { getRuntime } from '../runtime.svelte';
	import { localized } from '../text';
	import { displayedText } from '../text-attributes';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();
	const rt = getRuntime();
	const id = $props.id();
	const icons = {
		info: Info,
		'circle-help': CircleHelp,
		'circle-check': CircleCheck,
		'triangle-alert': TriangleAlert,
		'file-text': FileText,
		user: User
	};
	const d = $derived(element.desc);
	const body = $derived(displayedText(d, rt.read(instanceId, d.data_binding), getLocale()));
	const caption = $derived(localized(d.label));
	const tip = $derived(localized(d.help_tip));
	const Icon = $derived(d.icon ? icons[d.icon as keyof typeof icons] : undefined);
	const placement = $derived(d.label_placement ?? 'top');
	const horizontal = $derived(placement === 'left' || placement === 'right');
	let root = $state<HTMLDivElement>();
	let field = $state<HTMLDivElement>();
	let trigger = $state<HTMLButtonElement>();
	let popup = $state<HTMLDivElement>();
	let narrow = $state(false);
	let overflowing = $state(false);
	let open = $state(false);
	let tipScrollable = $state(false);
	let closeTimer: ReturnType<typeof setTimeout> | undefined;
	const fontSize = $derived(d.font_size ?? '0.875rem');
	const lineHeight = $derived(
		d.line_height && d.line_height !== 'normal'
			? String(d.line_height)
			: fontSize === '0.875rem'
				? '1.25rem'
				: '1.5'
	);
	const fixedHeight = $derived(
		d.height !== undefined && d.height !== 'auto' && d.height !== 'elastic'
	);

	function cancelClose() {
		clearTimeout(closeTimer);
		closeTimer = undefined;
	}
	function show() {
		cancelClose();
		open = true;
	}
	function close() {
		cancelClose();
		open = false;
	}
	function leave() {
		cancelClose();
		closeTimer = setTimeout(() => {
			if (
				document.activeElement !== trigger &&
				!popup?.contains(document.activeElement) &&
				!popup?.matches(':hover')
			)
				open = false;
		}, 150);
	}
	function outside(event: PointerEvent) {
		if (!trigger?.contains(event.target as Node) && !popup?.contains(event.target as Node)) close();
	}
	function keydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && open) {
			event.stopPropagation();
			if (popup?.contains(document.activeElement)) trigger?.focus();
			close();
		}
	}
	function position() {
		if (!trigger || !popup) return;
		const r = trigger.getBoundingClientRect();
		popup.style.maxWidth = `${Math.max(0, Math.min(280, window.innerWidth - 16))}px`;
		popup.style.maxHeight = `${Math.max(0, window.innerHeight - 16)}px`;
		const p = popup.getBoundingClientRect();
		tipScrollable = popup.scrollHeight > popup.clientHeight + 1;
		popup.style.left = `${Math.max(8, Math.min(r.left, window.innerWidth - p.width - 8))}px`;
		popup.style.top = `${Math.max(8, Math.min(r.bottom + 4, window.innerHeight - p.height - 8))}px`;
	}
	$effect(() => {
		const box = root,
			contents = field;
		if (!box || !contents) return;
		const measure = () => {
			narrow = box.clientWidth < 250;
			overflowing =
				box.scrollHeight > box.clientHeight + 1 || box.scrollWidth > box.clientWidth + 1;
			if (box.getClientRects().length === 0) close();
		};
		const observer = new ResizeObserver(measure);
		observer.observe(box);
		observer.observe(contents);
		measure();
		return () => {
			observer.disconnect();
			cancelClose();
		};
	});
	$effect(() => {
		body;
		caption;
		tip;
		d.padding;
		fixedHeight;
		let disposed = false;
		void tick().then(() => {
			if (!disposed && root)
				overflowing =
					root.scrollHeight > root.clientHeight + 1 || root.scrollWidth > root.clientWidth + 1;
		});
		return () => {
			disposed = true;
		};
	});
	$effect(() => {
		if (!tip) close();
	});
	$effect(() => {
		if (!open || !popup) return;
		const node = popup;
		node.showPopover?.();
		position();
		const observer = new ResizeObserver(position);
		observer.observe(node);
		window.addEventListener('resize', position);
		window.addEventListener('scroll', position, true);
		return () => {
			observer.disconnect();
			node.hidePopover?.();
			window.removeEventListener('resize', position);
			window.removeEventListener('scroll', position, true);
		};
	});
</script>

<svelte:window onpointerdown={outside} onkeydown={keydown} />

<!-- svelte-ignore a11y_no_noninteractive_tabindex (Only overflowing text becomes a named, keyboard-scrollable region.) -->
<div
	bind:this={root}
	class="text-element select-text"
	data-text-element
	role={overflowing ? 'region' : undefined}
	aria-label={overflowing ? caption || m.wpi_text_scrollable() : undefined}
	tabindex={overflowing ? 0 : undefined}
	style:height={fixedHeight ? '100%' : undefined}
	style:font-family={d.font_family ?? 'inherit'}
	style:font-size={fontSize}
	style:font-weight={d.font_weight ?? 'inherit'}
	style:font-style={d.font_style ?? 'normal'}
	style:line-height={lineHeight}
	style:color={d.color ?? 'inherit'}
	style:background-color={d.background_color ?? 'transparent'}
	style:padding={d.padding ?? '0.25rem 0px'}
>
	<div
		bind:this={field}
		class="field"
		class:horizontal={horizontal && !narrow}
		class:reverse={(placement === 'right' && !narrow) || placement === 'bottom'}
		style:gap={caption
			? horizontal && !narrow
				? (d.label_horizontal_gap ?? '8px')
				: (d.label_vertical_gap ?? '4px')
			: '0px'}
	>
		{#if caption}
			<div
				id="{id}-caption"
				class="caption"
				style:max-width={d.label_max_width ?? '160px'}
				style:text-align={d.label_horizontal_align ?? 'start'}
				style:align-self={horizontal && !narrow
					? { top: 'flex-start', center: 'center', bottom: 'flex-end' }[
							d.label_vertical_align ?? 'top'
						]
					: undefined}
				style:font-weight={d.label_font_weight ?? 500}
			>
				{caption}
			</div>
		{/if}
		<div
			class="content"
			role={caption ? 'group' : undefined}
			aria-labelledby={caption ? `${id}-caption` : undefined}
			aria-describedby={open && tip ? `${id}-tip` : undefined}
		>
			<div
				class="body-row"
				class:icon-end={d.icon_placement === 'end'}
				style:gap={Icon ? (d.icon_gap ?? '8px') : '0px'}
			>
				{#if Icon}<span class="icon" aria-hidden="true" style:color={d.icon_color ?? 'inherit'}
						><Icon size={d.icon_size ?? '16px'} /></span
					>{/if}
				<p style:text-align={d.text_align ?? 'start'} style:white-space={d.white_space ?? 'normal'}>
					{body}
				</p>
			</div>
			{#if tip}
				<button
					bind:this={trigger}
					type="button"
					class="help"
					aria-label={m.wpi_text_more_info()}
					aria-expanded={open}
					aria-describedby={open ? `${id}-tip` : undefined}
					onclick={() => {
						if (!open) show();
					}}
					onpointerenter={show}
					onpointerleave={leave}
					onfocus={show}
					onblur={leave}
				>
					<CircleHelp size={16} aria-hidden="true" />
				</button>
			{/if}
		</div>
	</div>
	{#if tip && open}
		<!-- svelte-ignore a11y_no_noninteractive_tabindex (An overflowing tooltip is keyboard scrollable.) -->
		<div
			bind:this={popup}
			id="{id}-tip"
			class="tooltip"
			role="tooltip"
			popover="manual"
			tabindex={tipScrollable ? 0 : undefined}
			onfocusin={cancelClose}
			onfocusout={leave}
			onpointerenter={cancelClose}
			onpointerleave={leave}
		>
			{tip}
		</div>
	{/if}
</div>

<style>
	.text-element {
		box-sizing: border-box;
		min-width: 0;
		max-width: 100%;
		overflow: auto;
		user-select: text;
	}
	.text-element:focus-visible,
	.help:focus-visible {
		outline: 2px solid var(--ring, currentColor);
		outline-offset: 2px;
	}
	.field {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.field.horizontal {
		flex-direction: row;
	}
	.field.reverse {
		flex-direction: column-reverse;
	}
	.field.horizontal.reverse {
		flex-direction: row-reverse;
	}
	.caption {
		min-width: 0;
		overflow-wrap: anywhere;
		flex: 0 1 auto;
	}
	.content {
		display: flex;
		align-items: flex-start;
		min-width: 0;
		flex: 1 1 auto;
		gap: 4px;
	}
	.body-row {
		display: flex;
		align-items: flex-start;
		min-width: 0;
		flex: 1 1 auto;
	}
	.body-row.icon-end {
		flex-direction: row-reverse;
	}
	p {
		margin: 0;
		min-width: 0;
		flex: 1 1 auto;
		overflow-wrap: anywhere;
	}
	.icon {
		flex: none;
		display: inline-flex;
	}
	.help {
		flex: none;
		display: inline-flex;
		justify-content: center;
		align-items: center;
		width: 44px;
		height: 44px;
		border-radius: 4px;
		cursor: pointer;
		color: inherit;
	}
	.help:hover {
		background: var(--muted, transparent);
	}
	.tooltip {
		position: fixed;
		inset: auto;
		margin: 0;
		box-sizing: border-box;
		overflow: auto;
		overflow-wrap: anywhere;
		padding: 8px 12px;
		border: 1px solid var(--border, currentColor);
		border-radius: 6px;
		background: var(--popover, var(--background, white));
		color: var(--popover-foreground, var(--foreground, black));
		font-size: 0.875rem;
		line-height: 1.5;
		user-select: text;
	}
</style>
