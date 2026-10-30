<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { getRuntime } from '../runtime.svelte';
	import { fieldErrorText, localized } from '../text';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();

	const rt = getRuntime();
	const id = $props.id();
	const d = $derived(element.desc);
	const options = $derived(d.options ?? []);
	const value = $derived(rt.read(instanceId, d.data_binding));
	const errors = $derived(rt.errorsFor(instanceId, d.data_binding));

	// Option values may be numbers; write back the option's own value.
	function choose(raw: string) {
		const option = options.find((o) => String(o.value) === raw);
		rt.write(instanceId, d.data_binding, option ? option.value : null);
	}
</script>

<div class="flex flex-col gap-1 text-sm">
	<label for={id} class="text-muted-foreground">{localized(d.label)}</label>
	<select
		{id}
		class="border-input dark:bg-input/30 h-9 w-full rounded-md border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
		disabled={d.binding_mode !== 'read-write'}
		value={value == null ? '' : String(value)}
		aria-invalid={errors.length > 0}
		onchange={(e) => choose(e.currentTarget.value)}
	>
		<option value="">{m.wpi_select_placeholder()}</option>
		{#each options as option (String(option.value))}
			<option value={String(option.value)}>{option.label}</option>
		{/each}
	</select>
	{#each errors as code (code)}<p class="text-destructive text-xs">{fieldErrorText(code)}</p>{/each}
</div>
