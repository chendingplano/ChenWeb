<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { getRuntime } from '../runtime.svelte';
	import { fieldErrorText, localized } from '../text';
	import type { Json } from '../types';
	import type { ElementProps } from './index';

	let { panelId, element }: ElementProps = $props();

	const rt = getRuntime();
	const id = $props.id();
	const d = $derived(element.desc);
	const editable = $derived(d.binding_mode === 'read-write');
	const value = $derived(rt.read(panelId, d.data_binding));
	const errors = $derived(rt.errorsFor(panelId, d.data_binding));

	function edit(raw: string) {
		let next: Json = raw;
		if (d.input_type === 'number') next = raw === '' || Number.isNaN(Number(raw)) ? (raw === '' ? null : raw) : Number(raw);
		rt.write(panelId, d.data_binding, next);
	}
</script>

<div class="flex flex-col gap-1 text-sm">
	<label for={id} class="text-muted-foreground">{localized(d.label)}</label>
	<Input
		{id}
		type={d.input_type ?? 'text'}
		value={value == null ? '' : String(value)}
		readonly={!editable}
		aria-invalid={errors.length > 0}
		aria-describedby={errors.length ? `${id}-errors` : undefined}
		oninput={(e) => edit(e.currentTarget.value)}
	/>
	{#if errors.length}
		<div id="{id}-errors" class="text-destructive text-xs">
			{#each errors as code (code)}<p>{fieldErrorText(code)}</p>{/each}
		</div>
	{/if}
</div>
