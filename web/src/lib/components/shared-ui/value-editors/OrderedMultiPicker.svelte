<!-- Several values from a fixed list, in order: tick to include, arrows to
     reorder. The chosen ones come first, in their order. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { moved } from './values';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, options, onChange }: ValueEditorProps = $props();
	const all = $derived(options.choices ?? []);
	const chosen = $derived(Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : all);
	const label = (choice: string) => options.choiceLabel?.(choice) ?? choice;
	function set(next: string[]) {
		if (next.length >= (options.minItems ?? 0)) onChange(next);
	}
</script>

<ul {id} class="ve-list">
	{#each [...chosen, ...all.filter((c) => !chosen.includes(c))] as choice (choice)}
		{@const at = chosen.indexOf(choice)}
		<li>
			<input
				id={`${id}-${choice}`}
				type="checkbox"
				checked={at >= 0}
				disabled={at >= 0 && chosen.length <= (options.minItems ?? 0)}
				onchange={(e) => set(e.currentTarget.checked ? [...chosen, choice] : chosen.filter((c) => c !== choice))}
			/>
			<label for={`${id}-${choice}`}>{label(choice)}</label>
			{#if at >= 0}
				<button type="button" class="ve-button" aria-label={m.wpi_chat_move_up({ name: label(choice) })} disabled={at === 0} onclick={() => set(moved(chosen, at, -1))}>↑</button>
				<button type="button" class="ve-button" aria-label={m.wpi_chat_move_down({ name: label(choice) })} disabled={at === chosen.length - 1} onclick={() => set(moved(chosen, at, 1))}>↓</button>
			{/if}
		</li>
	{/each}
</ul>
