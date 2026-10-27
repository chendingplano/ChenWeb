<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ErrorBox from './ErrorBox.svelte';
	import { pageStyle } from './layout';
	import { layouters } from './layouters';
	import { getRuntime } from './runtime.svelte';

	let { panelId }: { panelId: string } = $props();

	const rt = getRuntime();
	const panel = $derived(rt.panels[panelId]);
	const page = $derived(panel ? rt.bundle.pages[panel.pageId] : undefined);
	const Layouter = $derived(page ? layouters[page.layouter] : undefined);
</script>

{#if panel && page}
	<div class="h-full p-3" data-panel={panelId} data-page={page.descriptor_id}>
		<div style={pageStyle(page)} data-page-box>
			{#if panel.error}
				<div class="mb-3">
					<ErrorBox title={m.wpi_action_failed({ message: panel.error })} />
				</div>
			{/if}
			{#if !panel.ready}
				<p class="text-muted-foreground text-sm">{m.wpi_panel_loading()}</p>
			{:else if Layouter}
				<Layouter {panelId} {page} />
			{:else}
				<ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={m.wpi_unknown_layouter({ name: page.layouter })} />
			{/if}
		</div>
	</div>
{/if}
