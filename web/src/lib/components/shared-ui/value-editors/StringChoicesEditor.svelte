<!-- The List mode's choices: a list of strings, each removable with ×, and a new
     one added with + (or Enter). Empty unsets the list. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Json } from '$lib/wpi/types';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, check, onChange }: Pick<ValueEditorProps, 'id' | 'value' | 'check' | 'onChange'> = $props();
	const choices = $derived(
		(Array.isArray(value) ? value : []).filter((v): v is string => typeof v === 'string')
	);
	let draft = $state('');
	const next = $derived(draft.trim());
	const canAdd = $derived(next !== '' && !choices.includes(next));

	function set(updated: string[]) {
		const json: Json | undefined = updated.length ? updated : undefined;
		if (!check?.(json ?? null)) onChange(json);
	}
	// An empty or repeated value adds nothing; the draft stays for correction.
	function add() {
		if (!canAdd) return;
		set([...choices, next]);
		draft = '';
	}
</script>

<div {id}>
	<div class="ve-row">
		<input
			class="ve-input"
			aria-label={m.wpi_string_choice_new()}
			placeholder={m.wpi_string_choice_new()}
			bind:value={draft}
			onblur={add}
			onkeydown={(e) => {
				if (e.key === 'Enter') {
					e.preventDefault();
					add();
				}
			}}
		/>
		<button type="button" class="ve-button" aria-label={m.wpi_string_choice_add()} disabled={!canAdd} onclick={add}>+</button>
	</div>
	{#each choices as choice, index (index)}
		<div class="ve-row">
			<span class="ve-input choice">{choice}</span>
			<button type="button" class="ve-button" aria-label={m.wpi_string_choice_remove()} onclick={() => set(choices.filter((_, i) => i !== index))}>×</button>
		</div>
	{/each}
</div>

<style>
	.choice { flex: 1; min-width: 0; overflow-wrap: anywhere; }
</style>
