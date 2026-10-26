<script lang="ts">
	import { onMount } from 'svelte';
	import { ChevronDownIcon, ChevronRightIcon, FileIcon, FolderIcon } from '@lucide/svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getRuntime } from '../runtime.svelte';
	import type { MenuItem } from '../types';
	import { triggerOrigin, type ElementProps } from './index';

	let { panelId, element }: ElementProps = $props();

	const rt = getRuntime();
	const d = $derived(element.desc);
	// Bindings are per menu instance: this element in this panel.
	const instance = $derived(`${panelId}/${d.descriptor_id}`);
	let collapsed = $state<Record<string, boolean>>({});

	// path is the JSON Pointer of the item below the element's "items" attribute.
	function choose(item: MenuItem, path: string) {
		if (!item.on_click) return;
		void rt.runTrigger(item.on_click, triggerOrigin(element, 'items', `${path}/on_click`), {
			panelId,
			binding: `${instance}/${item.item_id}`
		});
	}

	onMount(() => {
		if (!d.initial_item || !d.items) return;
		const found = findLeaf(d.items, d.initial_item, '');
		if (found) choose(found.item, found.path);
	});

	function findLeaf(items: MenuItem[], id: string, base: string): { item: MenuItem; path: string } | null {
		for (const [i, item] of items.entries()) {
			const path = `${base}/${i}`;
			if (item.item_id === id && item.on_click) return { item, path };
			if (item.children) {
				const inner = findLeaf(item.children, id, `${path}/children`);
				if (inner) return inner;
			}
		}
		return null;
	}
</script>

{#snippet branch(items: MenuItem[], base: string, depth: number)}
	<ul class="flex flex-col gap-0.5">
		{#each items as item, i (item.item_id)}
			{@const path = `${base}/${i}`}
			<li>
				{#if item.children}
					{@const open = !collapsed[item.item_id]}
					<button
						type="button"
						class="hover:bg-muted flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-sm"
						style="padding-left: {0.5 + depth}rem"
						aria-expanded={open}
						aria-label={m.wpi_menu_toggle({ label: item.label })}
						onclick={() => (collapsed[item.item_id] = open)}
					>
						{#if open}<ChevronDownIcon class="size-4" />{:else}<ChevronRightIcon class="size-4" />{/if}
						<FolderIcon class="size-4" />
						<span class="truncate">{item.label}</span>
					</button>
					{#if open}{@render branch(item.children, `${path}/children`, depth + 1)}{/if}
				{:else}
					{@const active = rt.isShowing(`${instance}/${item.item_id}`)}
					<button
						type="button"
						class="hover:bg-muted flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-sm {active ? 'bg-muted font-medium' : ''}"
						style="padding-left: {1.75 + depth}rem"
						aria-current={active ? 'page' : undefined}
						onclick={() => choose(item, path)}
					>
						<FileIcon class="size-4" />
						<span class="truncate">{item.label}</span>
					</button>
				{/if}
			</li>
		{/each}
	</ul>
{/snippet}

<nav class="w-full">
	{@render branch(d.items ?? [], '', 0)}
</nav>
