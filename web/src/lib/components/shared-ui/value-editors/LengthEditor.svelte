<script lang="ts">
	import { tick } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import DraftField from './DraftField.svelte';
	import { lengthParts, lengthWithType, parseLength } from './values';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, label, value, options, check, onChange }: ValueEditorProps = $props();
	const keywords = $derived(options.keywords ?? []);
	const units = $derived(options.units ?? ['px', 'rem', 'em', '%']);
	const parts = $derived(lengthParts(value));
	const type = $derived(options.useDefault && options.defaultValue !== undefined ? 'default' : (parts.type || 'value'));
	const numeric = $derived((type === 'value' || units.includes(type)));
	let editing = $state(false);
	let wrapper: HTMLDivElement;
	async function edit() { editing = true; await tick(); wrapper?.querySelector('input')?.focus(); }
	function changeType(event: Event) {
		const select = event.currentTarget as HTMLSelectElement;
		const result = lengthWithType(value, select.value, units);
		if (result === undefined || (parseLength(result, keywords, units, select.value === 'value').ok && !check?.(result))) onChange(result);
		else select.value = type;
	}
	function parseAmount(text: string) {
		return parseLength(text.trim() ? `${text.trim()}${type === 'value' ? '' : type}` : '', [], units, type === 'value');
	}
	function leaveLabel(event: FocusEvent) {
		const target = event.target as HTMLInputElement;
		if (target.tagName !== 'INPUT') return;
		const parsed = options.displayType === false ? parseLength(target.value, keywords, units, true) : parseAmount(target.value);
		if (parsed.ok && (parsed.value === undefined || !check?.(parsed.value))) editing = false;
	}
</script>

<div class="length-row" bind:this={wrapper} onfocusout={leaveLabel}>
	{#if options.displayType === false}
		{#if options.readOnly}
			<span id={id} class="value-label">{value ?? options.defaultValue ?? ''}</span>
		{:else if options.valueDisplay === 'label' && !editing}
			<button type="button" {id} class="value-label editable" ondblclick={edit} onclick={(e) => { if (e.detail === 0) edit(); }}>{value ?? options.defaultValue ?? ''}</button>
		{:else if options.valueDisplay === 'select' && !options.allowCustomValue}
			{@const current = String(value ?? options.defaultValue ?? '')}
			{@const selectedParts = lengthParts(current)}
			{@const suffix = units.includes(selectedParts.type) ? selectedParts.type : ''}
			<select {id} class="ve-input" value={current} onchange={(e) => { const parsed = parseLength(e.currentTarget.value, keywords, units, true); if (parsed.ok && (parsed.value === undefined || !check?.(parsed.value))) onChange(parsed.value); }}>
				<option value={current}>{current}</option>
				{#each options.choices ?? [] as choice}{#if `${choice}${suffix}` !== current}<option value={`${choice}${suffix}`}>{`${choice}${suffix}`}</option>{/if}{/each}
			</select>
		{:else}
			<DraftField deferEmpty {id} {value} format={(v) => String(v ?? options.defaultValue ?? '')} parse={(text) => parseLength(text, keywords, units, true)} invalidMessage={() => m.wpi_prop_invalid_length({ allowed: [...units, ...keywords].join(', ') })} {check} commitOnBlur={options.commitOnBlur} {onChange} list={options.valueDisplay === 'select' ? `${id}-choices` : undefined} />
			{#if options.valueDisplay === 'select'}<datalist id={`${id}-choices`}>{#each options.choices ?? [] as choice}<option value={choice}></option>{/each}</datalist>{/if}
		{/if}
	{:else if options.readOnly}
		<span id={id} class="value-label">{type === 'default' ? options.defaultValue : (parts.amount || value || '')}</span>
		<span class="type-label">{type === 'default' ? m.wpi_length_default() : type}</span>
	{:else if numeric}
		{#if options.valueDisplay === 'label' && !editing}
			<button type="button" {id} class="value-label editable" ondblclick={edit} onclick={(e) => { if (e.detail === 0) edit(); }}>{parts.amount}</button>
		{:else if options.valueDisplay === 'select' && !options.allowCustomValue}
			<select {id} class="ve-input" value={parts.amount} onchange={(e) => { const p = parseAmount(e.currentTarget.value); if (p.ok && (p.value === undefined || !check?.(p.value))) onChange(p.value); }}>
				{#if !options.choices?.includes(parts.amount)}<option value={parts.amount}>{parts.amount}</option>{/if}
				{#each options.choices ?? [] as choice}<option value={choice}>{choice}</option>{/each}
			</select>
		{:else}
			<DraftField deferEmpty {id} {value} format={(v) => lengthParts(v).amount} parse={parseAmount} invalidMessage={() => m.wpi_prop_invalid_length({ allowed: [...units, ...keywords].join(', ') })} {check} commitOnBlur={options.commitOnBlur} {onChange} list={options.valueDisplay === 'select' ? `${id}-choices` : undefined} />
			{#if options.valueDisplay === 'select'}<datalist id={`${id}-choices`}>{#each options.choices ?? [] as choice}<option value={choice}></option>{/each}</datalist>{/if}
		{/if}
	{:else}
		<span id={id} class="value-label">{type === 'default' ? options.defaultValue : ''}</span>
	{/if}
	{#if options.displayType !== false && !options.readOnly}
		<select class="ve-input type" aria-label={m.wpi_length_type({ name: label })} value={type} onchange={changeType}>
			{#if type === ''}<option value=""></option>{/if}
			{#if type && type !== 'default' && type !== 'value' && !keywords.includes(type) && !units.includes(type)}<option value={type}>{type}</option>{/if}
			{#if options.defaultValue !== undefined}<option value="default">{m.wpi_length_default()}</option>{/if}
			<option value="value">{m.wpi_length_value()}</option>
			{#each keywords as keyword}<option value={keyword}>{keyword}</option>{/each}
			{#each units as unit}<option value={unit}>{unit}</option>{/each}
		</select>
	{/if}
</div>
<style>
	.length-row { display: flex; align-items: flex-start; gap: 4px; min-width: 0; width: 100%; }
	.type { width: auto; flex: none; }
	.type-label { padding: 4px; font-size: 11px; }
	.value-label { display: block; box-sizing: border-box; min-width: 0; flex: 1; min-height: 25px; padding: 4px 8px; border-radius: 4px; background: var(--ve-length-label-bg, var(--cv-group-header, #f3f4f6)); color: inherit; font-size: 11px; }
	.editable { border: 0; text-align: left; cursor: text; }
</style>
