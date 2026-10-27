<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ElementView from '../elements/Element.svelte';
	import { resolveEntry } from '../entries';
	import ErrorBox from '../ErrorBox.svelte';
	import { flowItemStyle } from '../layout';
	import { getRuntime } from '../runtime.svelte';
	import type { LayouterProps } from './index';

	let { panelId, page }: LayouterProps = $props();

	const rt = getRuntime();
	const entries = $derived(page.elements.map((entry, i) => resolveEntry(rt.bundle, page, entry, i)));
</script>

<div class="flex flex-wrap items-start gap-3">
	{#each entries as entry, i (i)}
		{#if 'error' in entry}
			<ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={entry.error} />
		{:else}
			<div class="min-w-0" style={flowItemStyle(entry.desc)}>
				<ElementView {panelId} element={entry} />
			</div>
		{/if}
	{/each}
</div>
