<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { getRuntime } from '../runtime.svelte';
	import { fieldErrorText, localized } from '../text';
	import { bindingPointer, runValidators, type ValidationError } from '../validators';
	import { elementValidationTexts } from '../validators/messages';
	import type { Json } from '../types';
	import type { ElementProps } from './index';

	let { instanceId, element }: ElementProps = $props();

	const rt = getRuntime();
	const id = $props.id();
	const d = $derived(element.desc);
	const options = $derived(d.options ?? []);
	const value = $derived(rt.read(instanceId, d.data_binding));
	const errors = $derived(rt.errorsFor(instanceId, d.data_binding));
	// Validators run once a value is chosen and before a save (OpenSpec
	// wpi-primitive-validators, D8).
	let validatorErrors = $state<ValidationError[]>([]);
	const validatorTexts = $derived(
		elementValidationTexts(validatorErrors, rt.validationErrorsFor(instanceId, d.data_binding), d.validators)
	);
	function check(next: Json | undefined): boolean {
		validatorErrors = d.binding_mode === 'read-write' ? runValidators(d.validators, next, bindingPointer(d.data_binding)) : [];
		return validatorErrors.length === 0;
	}
	$effect(() =>
		rt.registerInput(instanceId, id, (focus) => {
			const valid = check(rt.read(instanceId, d.data_binding));
			if (!valid && focus) document.getElementById(id)?.focus();
			return valid;
		})
	);

	// Option values may be numbers; write back the option's own value.
	function choose(raw: string) {
		const option = options.find((o) => String(o.value) === raw);
		const next = option ? option.value : null;
		rt.write(instanceId, d.data_binding, next);
		check(next);
	}
</script>

<div class="flex flex-col gap-1 text-sm">
	<label for={id} class="text-muted-foreground">{localized(d.label)}</label>
	<select
		{id}
		class="border-input dark:bg-input/30 h-9 w-full rounded-md border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
		disabled={d.binding_mode !== 'read-write'}
		value={value == null ? '' : String(value)}
		aria-invalid={errors.length > 0 || validatorTexts.length > 0}
		onchange={(e) => choose(e.currentTarget.value)}
	>
		<option value="">{m.wpi_select_placeholder()}</option>
		{#each options as option (String(option.value))}
			<option value={String(option.value)}>{option.label}</option>
		{/each}
	</select>
	{#each errors as code (code)}<p class="text-destructive text-xs">{fieldErrorText(code)}</p>{/each}
	{#each validatorTexts as text (text)}<p class="text-destructive text-xs">{text}</p>{/each}
</div>
