<script lang="ts">
	// A page instance (devdoc 2026102601 §3.3.2; OpenSpec wpi-pages-own-panes,
	// design D7). In the app's page area the page's own alignment and scroll bars
	// place its box, and its root pane sizes itself within it. Opened in a host pane, that
	// pane takes the place of the root pane: it already applies its margins,
	// alignment and scroll bars, and the box draws only the root's content.
	import { m } from '$lib/paraglide/messages.js';
	import ErrorBox from './ErrorBox.svelte';
	import PaneView from './PaneView.svelte';
	import { effectiveRoot, pageBoxStyle, placeStyle } from './layout';
	import { getRuntime } from './runtime.svelte';
	import type { PaneSpec } from './types';

	let { instanceId, host = undefined }: { instanceId: string; host?: PaneSpec } = $props();

	const rt = getRuntime();
	const inst = $derived(rt.pages[instanceId]);
	const page = $derived(inst ? rt.bundle.pages[inst.pageId] : undefined);
	const root = $derived(page ? effectiveRoot(page) : undefined);
	// The page layouter code its copy names; page-layouter is the only one.
	const module = $derived(page?.page_layouter?.class?.code_module_id ?? 'page-layouter');
</script>

{#snippet box()}
	{#if inst && page && root}
		<div style={pageBoxStyle(page, host)} data-page={page.descriptor_id} data-page-box>
			{#if inst.error}
				<div class="mb-3 w-full">
					<ErrorBox title={m.wpi_action_failed({ message: inst.error })} />
				</div>
			{/if}
			{#if module !== 'page-layouter'}
				<ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={m.wpi_unknown_layouter({ name: module })} />
			{:else if !inst.ready}
				<p class="text-muted-foreground text-sm">{m.wpi_panel_loading()}</p>
			{:else}
				<PaneView {instanceId} pane={root} bare={!!host} {host} />
			{/if}
		</div>
	{/if}
{/snippet}

{#if host}
	{@render box()}
{:else if page}
	<div style={placeStyle({ hori_align: page.hori_align, vert_align: page.vert_align, hori_scrollbar: page.hori_scrollbar, vert_scrollbar: page.vert_scrollbar })}>
		{@render box()}
	</div>
{/if}
