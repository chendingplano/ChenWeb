<!-- An element's validators (OpenSpec wpi-primitive-validators, design D9): each
     validator in order, with move and remove, and a pulldown that adds one of the
     classes in the validatorClasses option. Each validator's constraints have a
     group of their own in the property editor. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { localized } from '$lib/wpi/text';
	import type { Json, JsonObject } from '$lib/wpi/types';
	import { newValidatorInstance } from '$lib/wpi/validators';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, options, onChange }: ValueEditorProps = $props();
	const list = $derived(
		(Array.isArray(value) ? value : []).filter((v): v is JsonObject => !!v && typeof v === 'object' && !Array.isArray(v))
	);
	const classes = $derived(options.validatorClasses ?? []);
	let adding = $state(false);

	function set(next: Json[]) {
		onChange(next.length ? next : undefined);
	}
	function move(index: number, by: number) {
		const next = [...list];
		[next[index], next[index + by]] = [next[index + by], next[index]];
		set(next);
	}
	function add(classId: string) {
		const cls = classes.find((c) => c.descriptor_id === classId);
		if (cls) set([...list, newValidatorInstance(cls) as unknown as JsonObject]);
	}
	function description(item: JsonObject): string {
		const copy = item.class as JsonObject | undefined;
		return localized(copy?.description as Parameters<typeof localized>[0]);
	}
</script>

<div {id}>
	{#if list.length === 0}<p class="ve-sub">{m.wpi_vld_list_empty()}</p>{/if}
	<ol class="validators">
		{#each list as item, index (index)}
			<li>
				<span class="name" title={description(item)}>{index + 1}. {String(item.validator ?? '')}</span>
				<button type="button" class="ve-button" aria-label={m.wpi_vld_list_up({ number: String(index + 1) })} title={m.wpi_vld_list_up({ number: String(index + 1) })} disabled={index === 0} onclick={() => move(index, -1)}>↑</button>
				<button type="button" class="ve-button" aria-label={m.wpi_vld_list_down({ number: String(index + 1) })} title={m.wpi_vld_list_down({ number: String(index + 1) })} disabled={index === list.length - 1} onclick={() => move(index, 1)}>↓</button>
				<button type="button" class="ve-button" aria-label={m.wpi_vld_list_remove({ number: String(index + 1) })} title={m.wpi_vld_list_remove({ number: String(index + 1) })} onclick={() => set(list.filter((_, i) => i !== index))}>×</button>
			</li>
		{/each}
	</ol>
	<button type="button" class="ve-button" aria-label={m.wpi_vld_list_add()} aria-expanded={adding} onclick={() => adding = !adding}>+</button>
	{#if adding}<select class="ve-input" aria-label={m.wpi_vld_list_add()} value="" onchange={(e) => { add(e.currentTarget.value); e.currentTarget.value = ''; adding = false; }}>
		<option value="">{m.wpi_vld_list_add()}</option>
		{#each classes as cls (cls.descriptor_id)}<option value={cls.descriptor_id}>{cls.descriptor_id}</option>{/each}
	</select>{/if}
</div>

<style>
	.validators {
		list-style: none;
		margin: 0 0 4px;
		padding: 0;
	}
	.validators li {
		display: flex;
		align-items: center;
		gap: 4px;
		margin-bottom: 2px;
	}
	.name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 11px;
	}
</style>
