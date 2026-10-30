<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ElementView from '../elements/Element.svelte';
	import { resolveEntry } from '../entries';
	import ErrorBox from '../ErrorBox.svelte';
	import { flowItemStyle, flowStyle, panelStyle } from '../layout';
	import type { LayouterProps } from './index';

	let { instanceId, page, paneId, panel }: LayouterProps = $props();

	// The panel's own parameters replace its class copy's whole (design D2).
	const config = $derived(panel.elem_layouter.config ?? panel.elem_layouter.class?.config ?? {});
	const entries = $derived((panel.elem_list ?? []).map((entry, i) => resolveEntry(page, paneId, entry, i)));
	const sized = $derived(entries.map((e) => ('error' in e ? {} : e.desc)));
</script>

<div class="flex flex-wrap select-text" style="{flowStyle(config)}; {panelStyle(sized)}" data-flow-layout>
	{#each entries as entry, i (i)}
		{#if 'error' in entry}
			<ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={entry.error} />
		{:else}
			<div
				class="min-w-0"
				class:stretch={config.vertical_alignment === 'stretch' && (entry.desc.height === undefined || entry.desc.height === 'auto' || entry.desc.height === 'elastic')}
				style={flowItemStyle(entry.desc)}
				data-flow-item
			>
				<ElementView {instanceId} element={entry} />
			</div>
		{/if}
	{/each}
</div>

<style>
	.stretch {
		display: flex;
		flex-direction: column;
	}

	.stretch > :global(*) {
		flex: 1 1 auto;
	}
</style>
