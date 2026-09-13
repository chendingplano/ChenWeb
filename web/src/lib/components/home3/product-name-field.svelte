<script module lang="ts">
	import { listProductNames, type ProductNameEntry } from '$lib/services/productNamesService';

	// kb.product_names is a few thousand rows and changes rarely, so every
	// mount of this field across the app shares one fetch (and one in-memory
	// search) for the page session instead of querying per keystroke or
	// refetching per instance.
	let cached: ProductNameEntry[] | null = null;
	let inflight: Promise<ProductNameEntry[]> | null = null;

	function loadCatalog(): Promise<ProductNameEntry[]> {
		if (cached) return Promise.resolve(cached);
		if (!inflight) {
			inflight = listProductNames()
				.then((out) => {
					cached = out.product_names;
					return cached;
				})
				.finally(() => {
					inflight = null;
				});
		}
		return inflight;
	}
</script>

<script lang="ts">
	import { onMount } from 'svelte';

	// Theming: reads --pnf-* custom properties (with neutral fallbacks) rather
	// than fixed colors, so callers on differently themed pages can match
	// their own palette via an inline `style` prop instead of this component
	// hard-coding one look.
	let {
		value = $bindable(''),
		label = 'Product Name',
		placeholder = '',
		disabled = false,
		style = ''
	}: {
		value?: string;
		label?: string;
		placeholder?: string;
		disabled?: boolean;
		style?: string;
	} = $props();

	let productNames = $state<ProductNameEntry[]>([]);
	let suggestionsOpen = $state(false);

	onMount(() => {
		loadCatalog()
			.then((names) => (productNames = names))
			.catch(() => {
				// Typeahead is a convenience — leave it empty rather than
				// surfacing an error on a field the user can still type into.
			});
	});

	const suggestions = $derived.by(() => {
		const q = value.trim().toLowerCase();
		if (q.length < 1) return [];
		const hits: ProductNameEntry[] = [];
		for (const p of productNames) {
			const matches =
				p.product_name.toLowerCase().includes(q) ||
				p.product_name_en.toLowerCase().includes(q) ||
				p.aliases.some((a) => a.toLowerCase().includes(q));
			if (matches) {
				hits.push(p);
				if (hits.length >= 20) break;
			}
		}
		return hits;
	});

	function suggestionLabel(p: ProductNameEntry): string {
		return p.product_name_en ? `${p.product_name} / ${p.product_name_en}` : p.product_name;
	}

	function select(p: ProductNameEntry) {
		value = p.product_name;
		suggestionsOpen = false;
	}
</script>

<label class="pnf-label" {style}>
	<span>{label}</span>
	<div class="pnf-wrap">
		<input
			type="text"
			bind:value
			{placeholder}
			{disabled}
			autocomplete="off"
			onfocus={() => (suggestionsOpen = true)}
			oninput={() => (suggestionsOpen = true)}
			onblur={() => (suggestionsOpen = false)}
			onkeydown={(e) => {
				if (e.key === 'Escape') suggestionsOpen = false;
			}}
		/>
		{#if suggestionsOpen && suggestions.length > 0}
			<ul class="pnf-suggestions">
				{#each suggestions as p (p.id)}
					<li>
						<button
							type="button"
							title={suggestionLabel(p)}
							onmousedown={(e) => {
								e.preventDefault();
								select(p);
							}}
						>
							<span>{suggestionLabel(p)}</span>
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</label>

<style>
	.pnf-label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font: inherit;
		font-size: 12px;
		color: var(--pnf-subtle, #667085);
	}
	.pnf-wrap {
		position: relative;
	}
	.pnf-wrap input {
		box-sizing: border-box;
		width: 100%;
		padding: 9px 11px;
		border: 1px solid var(--pnf-border, #d0d5dd);
		border-radius: var(--pnf-radius, 0px);
		background: var(--pnf-bg, #fff);
		color: var(--pnf-text, inherit);
		font: inherit;
		font-size: 13px;
	}
	.pnf-suggestions {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		z-index: 20;
		margin: 0;
		padding: 4px;
		list-style: none;
		border: 1px solid var(--pnf-border, #d0d5dd);
		border-radius: var(--pnf-radius, 0px);
		background: var(--pnf-bg, #fff);
		box-shadow: 0 8px 20px rgba(0, 0, 0, 0.16);
		max-height: 260px;
		overflow-y: auto;
	}
	.pnf-suggestions button {
		display: block;
		width: 100%;
		padding: 7px 9px;
		border: 0;
		background: transparent;
		color: var(--pnf-text, inherit);
		text-align: left;
		font: inherit;
		font-size: 12px;
		cursor: pointer;
	}
	.pnf-suggestions button:hover {
		background: var(--pnf-hover, rgba(99, 102, 241, 0.12));
	}
	.pnf-suggestions span {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
