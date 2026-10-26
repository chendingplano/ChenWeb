<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { clampPx, isResizable, paneStyle } from './layout';
	import Panel from './Panel.svelte';
	import { getRuntime } from './runtime.svelte';

	let {
		paneId,
		axis,
		handle = null,
		class: className = ''
	}: { paneId: string; axis: 'width' | 'height'; handle?: 'left' | 'right' | null; class?: string } = $props();

	const rt = getRuntime();
	const spec = $derived(rt.bundle.app.panes[paneId]);
	const pane = $derived(rt.panes[paneId]);
	const resizable = $derived(handle !== null && isResizable(spec));

	let section: HTMLElement;
	let width = $state<number | undefined>(undefined);
	let dragging: number | null = null;

	function startResize(event: PointerEvent) {
		if (event.button !== 0) return;
		dragging = event.pointerId;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault();
	}

	function resize(event: PointerEvent) {
		if (dragging !== event.pointerId) return;
		const rect = section.getBoundingClientRect();
		const next = handle === 'right' ? event.clientX - rect.left : rect.right - event.clientX;
		width = clampPx(next, spec.min_width, spec.max_width);
	}

	function endResize(event: PointerEvent) {
		if (dragging === event.pointerId) dragging = null;
	}
</script>

<section
	bind:this={section}
	data-pane={paneId}
	class="relative min-h-0 min-w-0 {className}"
	style={paneStyle(spec, axis, axis === 'width' ? width : undefined)}
>
	{#each pane.panels as id (id)}
		<div class={id === pane.visible ? 'h-full' : 'hidden'}>
			<Panel panelId={id} />
		</div>
	{/each}
	{#if resizable}
		<div
			role="separator"
			aria-orientation="vertical"
			aria-label={m.wpi_resize_pane()}
			class="hover:bg-border absolute inset-y-0 z-10 w-1.5 cursor-col-resize select-none {handle === 'right' ? 'right-0' : 'left-0'}"
			onpointerdown={startResize}
			onpointermove={resize}
			onpointerup={endResize}
			onpointercancel={endResize}
		></div>
	{/if}
</section>
