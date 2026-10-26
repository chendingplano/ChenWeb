<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ErrorBox from '../ErrorBox.svelte';
	import { elementTypes, type ElementProps } from './index';

	let { panelId, element }: ElementProps = $props();

	const View = $derived(elementTypes[element.desc.element_type]);
</script>

{#if View}
	<View {panelId} {element} />
{:else}
	<ErrorBox
		title={m.wpi_error_title({ descriptor: element.desc.descriptor_id })}
		message={m.wpi_unknown_element({ type: element.desc.element_type })}
	/>
{/if}
