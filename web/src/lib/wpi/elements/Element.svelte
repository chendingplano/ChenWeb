<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ErrorBox from '../ErrorBox.svelte';
	import { codeModules, type ElementProps } from './index';

	let { panelId, element }: ElementProps = $props();

	const View = $derived(codeModules[element.module]);
</script>

{#if View}
	<View {panelId} {element} />
{:else}
	<ErrorBox
		title={m.wpi_error_title({ descriptor: element.pageId })}
		message={m.wpi_unknown_element({ name: element.module })}
	/>
{/if}
