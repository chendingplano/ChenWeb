<!-- icon-picker: an icon from the shared icon library, found by keyword or description in
     English or Chinese. The value is the icon's name, such as refresh-cw. -->
<script lang="ts">
	import { Icon, IconSearchPicker } from '@chendingplano/shared';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, value, onChange }: ValueEditorProps = $props();
	const name = $derived(typeof value === 'string' ? value : '');
	const locale = $derived(getLocale());
	let open = $state(false);
</script>

<div {id}>
	<div class="ve-row">
		<button type="button" class="ve-button ve-icon" aria-expanded={open} onclick={() => (open = !open)}>
			{#if name}<Icon {name} size={16} /><span>{name}</span>{:else}<span>{m.wpi_icon_picker_choose()}</span>{/if}
		</button>
		{#if name}<button type="button" class="ve-button" onclick={() => onChange(undefined)}>{m.wpi_icon_picker_clear()}</button>{/if}
	</div>
	{#if open}
		<div class="ve-icon-search">
			<IconSearchPicker
				value={name}
				onSelect={(picked: string) => {
					onChange(picked);
					open = false;
				}}
				labels={{ placeholder: m.icon_search_placeholder(), empty: m.icon_search_empty(), error: m.icon_search_error() }}
				{locale}
				limit={24}
				columns={4}
			/>
		</div>
	{/if}
</div>

<style>
	.ve-icon {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
	}
	.ve-icon-search {
		margin-top: 6px;
		font-size: 11px;
	}
</style>
