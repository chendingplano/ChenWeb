<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ElementView from '../elements/Element.svelte';
	import { resolveEntry } from '../entries';
	import ErrorBox from '../ErrorBox.svelte';
	import { flowItemStyle, flowStyle } from '../layout';
	import { getRuntime } from '../runtime.svelte';
	import type { LayouterProps } from './index';

	let { panelId, page }: LayouterProps = $props();

	const rt = getRuntime();
	const entries = $derived(page.elements.map((entry, i) => resolveEntry(rt.bundle, page, entry, i)));
</script>

<div class="flex flex-wrap select-text" style={flowStyle(page)} data-flow-layout>
	{#each entries as entry, i (i)}
		{#if 'error' in entry}
			<ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={entry.error} />
		{:else}
			<div
				class="min-w-0"
				class:stretch={page.layouter_config?.vertical_alignment === 'stretch' && (!entry.desc.height || entry.desc.height === 'auto' || entry.desc.height === 'elastic')}
				style={flowItemStyle(entry.desc)}
				data-flow-item
			>
				<ElementView {panelId} element={entry} />
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
