<!-- A text field whose text stays as typed while the author edits it. Valid
     text is applied at once (or on leaving the field with commitOnBlur).
     Invalid text is not applied; a half-typed value is not flagged while the
     author types, but on leaving the field it shows the stored value again
     and says why the text was not applied (OpenSpec wpi-property-editors, D4). -->
<script lang="ts">
	import { untrack } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import type { Json } from '$lib/wpi/types';
	import type { Parsed } from './values';
	import './value-editors.css';

	let {
		id,
		value,
		format,
		parse,
		invalidMessage,
		check,
		ariaLabel,
		multiline = false,
		rows = 1,
		commitOnBlur = false,
		readonly = false,
		inputmode,
		onChange
	}: {
		id: string;
		value: Json | undefined;
		format: (value: Json | undefined) => string;
		parse: (text: string) => Parsed;
		invalidMessage: () => string;
		check?: (value: Json) => string | null;
		ariaLabel?: string;
		multiline?: boolean;
		rows?: number;
		commitOnBlur?: boolean;
		readonly?: boolean;
		inputmode?: 'text' | 'numeric' | 'decimal';
		onChange: (value: Json | undefined) => void;
	} = $props();

	let draft = $state<string | null>(null);
	// Why the typed text is invalid, and whether the author left the field with it.
	let error = $state<string | null>(null);
	let rejected = $state(false);
	let emitted = '';

	// A change from elsewhere (undo, the canvas) replaces the text being typed.
	$effect(() => {
		const key = JSON.stringify(value ?? null);
		untrack(() => {
			if (draft !== null && key !== emitted) {
				draft = null;
				error = null;
			}
		});
	});

	function read(text: string): { value: Json | undefined } | { error: string } {
		const p = parse(text);
		if (!p.ok) return { error: invalidMessage() };
		const problem = p.value !== undefined && check ? check(p.value) : null;
		return problem ? { error: problem } : { value: p.value };
	}
	function emit(next: Json | undefined) {
		emitted = JSON.stringify(next ?? null);
		onChange(next);
	}
	function input(text: string) {
		draft = text;
		rejected = false;
		const r = read(text);
		error = 'error' in r ? r.error : null;
		if ('value' in r && !commitOnBlur) emit(r.value);
	}
	function leave() {
		if (draft === null) return;
		const r = read(draft);
		if ('error' in r) rejected = true;
		else if (commitOnBlur) emit(r.value);
		draft = null;
	}
</script>

{#if multiline}
	<textarea
		{id}
		class="ve-input"
		{rows}
		{readonly}
		aria-label={ariaLabel}
		aria-invalid={rejected}
		aria-describedby={rejected ? `${id}-error` : undefined}
		value={draft ?? format(value)}
		oninput={(e) => input(e.currentTarget.value)}
		onblur={leave}
	></textarea>
{:else}
	<input
		{id}
		class="ve-input"
		type="text"
		{inputmode}
		{readonly}
		aria-label={ariaLabel}
		aria-invalid={rejected}
		aria-describedby={rejected ? `${id}-error` : undefined}
		value={draft ?? format(value)}
		oninput={(e) => input(e.currentTarget.value)}
		onblur={leave}
		onkeydown={(e) => {
			if (e.key === 'Enter') {
				e.preventDefault();
				leave();
			}
		}}
	/>
{/if}
{#if rejected && error}<p id={`${id}-error`} class="ve-error" role="alert">{error}&nbsp;{m.wpi_flow_value_not_applied()}</p>{/if}
