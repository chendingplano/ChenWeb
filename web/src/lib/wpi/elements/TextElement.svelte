<script lang="ts">
	import { getRuntime } from '../runtime.svelte';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();

	const rt = getRuntime();
	const d = $derived(element.desc);
	const value = $derived(d.data_binding ? rt.read(instanceId, d.data_binding) : d.text);
</script>

<p class="py-1 text-sm break-words">{value == null ? '' : String(value)}</p>
