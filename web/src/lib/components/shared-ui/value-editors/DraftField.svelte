<!-- A text field whose text stays as typed while the author edits it. Valid
     text is applied at once (or on leaving the field with commitOnBlur).
     Invalid text is never applied. A half-typed value is not flagged while the
     author types; on leaving the field (or Enter) the text stays, and under it
     the reason with Abandon Edit (show the stored value again) and Stay Edit
     (back into the field) (OpenSpec wpi-property-editors, D4). -->
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
		deferEmpty = false,
		readonly = false,
		inputmode,
		list,
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
		deferEmpty?: boolean;
		readonly?: boolean;
		inputmode?: 'text' | 'numeric' | 'decimal';
		list?: string;
		onChange: (value: Json | undefined) => void;
	} = $props();

	let draft = $state<string | null>(null);
	// Why the typed text is invalid, and whether the author left the field with it.
	let error = $state<string | null>(null);
	let rejected = $state(false);
	let emitted = '';
	let field = $state<HTMLInputElement | HTMLTextAreaElement>();

	// A change from elsewhere (undo, the canvas) replaces the text being typed.
	$effect(() => {
		const key = JSON.stringify(value ?? null);
		untrack(() => {
			if (draft !== null && key !== emitted) {
				draft = null;
				error = null;
				rejected = false;
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
		if ('value' in r && !commitOnBlur && !(deferEmpty && r.value === undefined)) emit(r.value);
	}
	// Leaving with invalid text keeps it, unapplied, until the author abandons or fixes it.
	function leave() {
		if (draft === null) return;
		const r = read(draft);
		if ('error' in r) {
			rejected = true;
			return;
		}
		if (commitOnBlur || (deferEmpty && r.value === undefined)) emit(r.value);
		draft = null;
	}
	function abandon() {
		draft = null;
		error = null;
		rejected = false;
	}
	function stay() {
		rejected = false;
		field?.focus();
	}
</script>

{#if multiline}
	<textarea
		bind:this={field}
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
		bind:this={field}
		{id}
		class="ve-input"
		type="text"
		{inputmode}
		{list}
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
{#if rejected && error}
	<div id={`${id}-error`} role="alert">
		<p class="ve-error">{error}&nbsp;{m.wpi_prop_not_applied()}</p>
		<div class="ve-row" style="margin-top:4px">
			<button type="button" class="ve-button" onclick={abandon}>{m.wpi_prop_abandon_edit()}</button>
			<button type="button" class="ve-button" onclick={stay}>{m.wpi_prop_stay_edit()}</button>
		</div>
	</div>
{/if}
