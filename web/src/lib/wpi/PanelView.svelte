<script lang="ts">
	// The panel of one leaf pane: its elements, arranged by the element layouter
	// its copy names (devdoc 2026102601 §3.3). A pane without a panel is empty.
	import { m } from '$lib/paraglide/messages.js';
	import { flowMarginStyle, panelStyle, surfaceColorStyle } from './layout';
	import type { PaneSpec } from './types';
	import ErrorBox from './ErrorBox.svelte';
	import { elementLayouters } from './layouters';
	import { getRuntime } from './runtime.svelte';

	let { instanceId, paneId, pane }: { instanceId: string; paneId: string; pane?: PaneSpec } = $props();

	const rt = getRuntime();
	const inst = $derived(rt.pages[instanceId]);
	const page = $derived(inst ? rt.bundle.pages[inst.pageId] : undefined);
	const panel = $derived(page?.panels?.[paneId]);
	const module = $derived(panel ? (panel.elem_layouter.class?.code_module_id ?? panel.elem_layouter.layouter) : '');
	const config = $derived(panel?.elem_layouter.config ?? panel?.elem_layouter.class?.config ?? {});
	const stretches = $derived((panel?.elem_list ?? []).some((e) => e.width === 'elastic' || (typeof e.width === 'string' && e.width.endsWith('%'))));
	const Layouter = $derived(elementLayouters[module]);
</script>

{#if page && panel}
	{#if Layouter}
		<div style={`${panelStyle(panel.elem_list ?? [])}; ${surfaceColorStyle(panel, !!pane?.background_color)}; ${module === 'flow-layouter' ? `${flowMarginStyle(config)}; box-sizing: ${stretches ? 'border-box' : 'content-box'}` : ''}`} ><Layouter {instanceId} {page} {paneId} {panel} /></div>
	{:else}
		<ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={m.wpi_unknown_layouter({ name: module })} />
	{/if}
{/if}
