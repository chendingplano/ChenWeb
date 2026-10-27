<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ErrorBox from '../ErrorBox.svelte';
	import { elementClasses, type ElementProps } from './index';

	let { panelId, element }: ElementProps = $props();

	const View = $derived(elementClasses[element.desc.element]);
</script>

{#if View}
	<View {panelId} {element} />
{:else}
	<ErrorBox
		title={m.wpi_error_title({ descriptor: element.pageId })}
		message={m.wpi_unknown_element({ name: element.desc.element })}
	/>
{/if}
