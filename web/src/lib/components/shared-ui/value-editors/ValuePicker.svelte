<!-- One value from a fixed list, in a pulldown. A stored value the list does
     not offer is kept and shown as it is. With emptyLabel, a first choice
     unsets the value. -->
<script lang="ts">
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, options, onChange }: ValueEditorProps = $props();
	const choices = $derived(options.choices ?? []);
	const current = $derived(value === undefined || value === null ? '' : String(value));
	const label = (choice: string) => options.choiceLabel?.(choice) ?? choice;
</script>

<select {id} class="ve-input" value={current} onchange={(e) => onChange(e.currentTarget.value === '' ? undefined : e.currentTarget.value)}>
	{#if options.emptyLabel}<option value="">{options.emptyLabel()}</option>{/if}
	{#if current !== '' && !choices.includes(current)}<option value={current}>{current}</option>{/if}
	{#each choices as choice (choice)}<option value={choice}>{label(choice)}</option>{/each}
</select>
