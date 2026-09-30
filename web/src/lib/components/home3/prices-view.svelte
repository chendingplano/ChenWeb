<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import {
		createPrice,
		deletePrice,
		listPrices,
		updatePrice,
		type PriceDef,
		type PriceDefInput,
		type PriceItem,
		type PriceType
	} from './prices-client';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	const colors = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#fff',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#64748B',
		border: darkMode ? '#2D3348' : '#DCE2EA'
	});
	const typeLabels: Record<PriceType, string> = {
		service: m.prices_service_prices(),
		llm: m.prices_llm_prices()
	};
	const empty = (price_type: PriceType = 'llm'): PriceDefInput => ({
		price_def_name: '',
		price_type,
		description: '',
		items: [emptyItem()]
	});
	function emptyItem(): PriceItem {
		return {
			item_name: '',
			item_type: 'input',
			cache: '',
			time_span: '',
			unit: 'million-tokens',
			currency: 'CN',
			value: ''
		};
	}
	let defs = $state<PriceDef[]>([]);
	let draft = $state<PriceDefInput>(empty());
	let editingID = $state<number | null>(null);
	let filter = $state<'all' | PriceType>('all');
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let formError = $state('');
	let expandedID = $state<number | null>(null);
	const shown = $derived(filter === 'all' ? defs : defs.filter((d) => d.price_type === filter));

	async function load() {
		loading = true;
		error = '';
		try {
			defs = await listPrices();
		} catch (e) {
			error = e instanceof Error ? e.message : m.prices_unable_to_load_prices();
		} finally {
			loading = false;
		}
	}

	function newDef() {
		editingID = null;
		draft = empty(filter === 'all' ? 'llm' : filter);
		formError = '';
	}
	function edit(d: PriceDef) {
		editingID = d.id;
		draft = {
			price_def_name: d.price_def_name,
			price_type: d.price_type,
			description: d.description,
			items: d.items.map((item) => ({ ...item }))
		};
		formError = '';
		window.scrollTo({ top: 0, behavior: 'smooth' });
	}

	async function save() {
		if (!draft.price_def_name.trim()) {
			formError = m.prices_price_definition_name_is_required();
			return;
		}
		if (draft.items.length === 0) {
			formError = m.prices_add_at_least_one_price();
			return;
		}
		if (draft.items.some((i) => !i.item_name.trim() || !i.unit.trim() || !i.currency.trim())) {
			formError = m.prices_every_price_item_needs_a();
			return;
		}
		if (draft.items.some((i) => !/^[0-9]+(\.[0-9]+)?$/.test(String(i.value).trim()))) {
			formError = m.prices_every_price_item_needs_a_2();
			return;
		}
		const names = draft.items.map((i) => i.item_name.trim());
		if (new Set(names).size !== names.length) {
			formError = m.prices_item_names_must_be_unique();
			return;
		}
		saving = true;
		formError = '';
		try {
			const input = {
				...draft,
				price_def_name: draft.price_def_name.trim(),
				items: draft.items.map((i) => ({ ...i, value: String(i.value).trim() }))
			};
			if (editingID === null) await createPrice(input);
			else await updatePrice(editingID, input);
			newDef();
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : m.prices_unable_to_save_price_definition();
		} finally {
			saving = false;
		}
	}

	async function remove(d: PriceDef) {
		if (!confirm(m.prices_delete_price_definition_and_all({ price_def_name: d.price_def_name })))
			return;
		try {
			await deletePrice(d.id);
			if (editingID === d.id) newDef();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : m.prices_unable_to_delete_price_definition();
		}
	}

	onMount(load);
</script>

<div class="page" style={`background:${colors.bg};color:${colors.text};`}>
	<section class="panel" style={`background:${colors.card};border-color:${colors.border};`}>
		<div class="heading">
			<div>
				<span class="eyebrow" style={`color:${colors.muted}`}>{m.prices_system_admin_system()}</span
				>
				<h1>{m.prices_price_management()}</h1>
				<p style={`color:${colors.muted}`}>
					{m.prices_define_service_prices_offered_to()}
				</p>
			</div>
			<button class="secondary" onclick={newDef}>{m.prices_new_price_definition()}</button>
		</div>
		<h2>
			{editingID === null ? m.prices_create_price_definition() : m.prices_modify_price_definition()}
		</h2>
		<div class="fields head-fields">
			<label
				>{m.prices_name()}<input
					bind:value={draft.price_def_name}
					placeholder={m.prices_deepseek_pricing()}
				/></label
			>
			<label
				>{m.prices_price_type()}<select bind:value={draft.price_type}
					><option value="llm">{m.prices_llm()}</option><option value="service"
						>{m.prices_service()}</option
					></select
				></label
			>
		</div>
		<label class="full"
			>{m.prices_description()}<textarea
				bind:value={draft.description}
				rows="2"
				placeholder={m.prices_optional_notes_e_g_source()}
			></textarea></label
		>
		<div class="item-heading">
			<h3>{m.prices_price_items()}</h3>
			<button class="secondary" onclick={() => (draft.items = [...draft.items, emptyItem()])}
				>{m.prices_add_item()}</button
			>
		</div>
		{#if draft.items.length > 0}
			<div class="table-wrap">
				<table class="item-table">
					<thead
						><tr
							><th>{m.prices_item_name()}</th><th>{m.prices_type()}</th><th>{m.prices_cache()}</th
							><th>{m.prices_time_span()}</th><th>{m.prices_unit()}</th><th
								>{m.prices_currency()}</th
							><th>{m.prices_value()}</th><th></th></tr
						></thead
					><tbody>
						{#each draft.items as item, i (item)}
							<tr>
								<td
									><input
										class="name-input"
										bind:value={item.item_name}
										aria-label={m.prices_item_name_2({ value: i + 1 })}
										placeholder={m.prices_input_cache_hit_peak()}
									/></td
								>
								<td
									><select
										bind:value={item.item_type}
										aria-label={m.prices_item_type({ value: i + 1 })}
										><option value="input">{m.prices_input()}</option><option value="output"
											>{m.prices_output()}</option
										></select
									></td
								>
								<td
									><select
										bind:value={item.cache}
										aria-label={m.prices_item_cache({ value: i + 1 })}
										><option value="">-</option><option value="hit">{m.prices_hit()}</option><option
											value="miss">{m.prices_miss()}</option
										></select
									></td
								>
								<td
									><select
										bind:value={item.time_span}
										aria-label={m.prices_item_time_span({ value: i + 1 })}
										><option value="">-</option><option value="peak">{m.prices_peak()}</option
										><option value="off-peak">{m.prices_off_peak()}</option></select
									></td
								>
								<td
									><input
										bind:value={item.unit}
										aria-label={m.prices_item_unit({ value: i + 1 })}
										list="price-units"
									/></td
								>
								<td
									><input
										class="short"
										bind:value={item.currency}
										aria-label={m.prices_item_currency({ value: i + 1 })}
										list="price-currencies"
									/></td
								>
								<td
									><input
										class="short num"
										bind:value={item.value}
										aria-label={m.prices_item_value({ value: i + 1 })}
										inputmode="decimal"
										placeholder="0.00"
									/></td
								>
								<td
									><button
										class="text danger"
										onclick={() => (draft.items = draft.items.filter((_, index) => index !== i))}
										>{m.prices_remove()}</button
									></td
								>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			<datalist id="price-units"><option value="million-tokens"></option></datalist>
			<datalist id="price-currencies"
				><option value="CN"></option><option value="US"></option></datalist
			>
		{:else}<p class="muted" style={`color:${colors.muted}`}>{m.prices_no_price_items_yet()}</p>{/if}
		{#if formError}<p class="error" role="alert">{formError}</p>{/if}
		<div class="form-actions">
			<button onclick={save} disabled={saving}
				>{saving
					? m.prices_saving()
					: editingID === null
						? m.prices_create_price_definition()
						: m.prices_save_changes()}</button
			>{#if editingID !== null}<button class="secondary" onclick={newDef}
					>{m.prices_cancel()}</button
				>{/if}
		</div>
	</section>

	<section class="panel" style={`background:${colors.card};border-color:${colors.border};`}>
		<div class="heading">
			<div>
				<h2>{m.prices_all_price_definitions()}</h2>
				<div class="tabs" role="tablist">
					{#each [['all', m.prices_all()], ['service', typeLabels.service], ['llm', typeLabels.llm]] as [key, label] (key)}
						<button
							role="tab"
							class="tab"
							class:active={filter === key}
							aria-selected={filter === key}
							onclick={() => (filter = key as typeof filter)}>{label}</button
						>
					{/each}
				</div>
			</div>
			<button class="secondary" onclick={load}>{m.prices_refresh()}</button>
		</div>
		{#if loading}<p class="muted">{m.prices_loading_prices()}</p>{:else if error}<p
				class="error"
				role="alert"
			>
				{error}
			</p>{:else if shown.length === 0}<p class="muted">
				{m.prices_no_price_definitions_have_been()}
			</p>{:else}
			<div class="table-wrap">
				<table>
					<thead
						><tr
							><th>{m.prices_name()}</th><th>{m.prices_type()}</th><th>{m.prices_description()}</th
							><th>{m.prices_items()}</th><th>{m.prices_actions()}</th></tr
						></thead
					><tbody>
						{#each shown as d (d.id)}
							<tr
								><td
									><button
										class="text name"
										onclick={() => (expandedID = expandedID === d.id ? null : d.id)}
										aria-expanded={expandedID === d.id}>{d.price_def_name}</button
									></td
								><td>{typeLabels[d.price_type]}</td><td class="notes-preview"
									>{d.description || '—'}</td
								><td>{d.items.length}</td><td class="actions"
									><button class="text" onclick={() => edit(d)}>{m.prices_edit()}</button><button
										class="text danger"
										onclick={() => remove(d)}>{m.prices_delete()}</button
									></td
								></tr
							>
							{#if expandedID === d.id}<tr
									><td colspan="5"
										><div class="details">
											<table class="item-table">
												<thead
													><tr
														><th>{m.prices_item_name()}</th><th>{m.prices_type()}</th><th
															>{m.prices_cache()}</th
														><th>{m.prices_time_span()}</th><th>{m.prices_unit()}</th><th
															>{m.prices_currency()}</th
														><th class="num">{m.prices_value()}</th></tr
													></thead
												><tbody>
													{#each d.items as item (item.id)}<tr
															><td class="mono">{item.item_name}</td><td>{item.item_type}</td><td
																>{item.cache || '-'}</td
															><td>{item.time_span || '-'}</td><td>{item.unit}</td><td
																>{item.currency}</td
															><td class="num mono">{item.value}</td></tr
														>{/each}
												</tbody>
											</table>
										</div></td
									></tr
								>{/if}
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>
</div>

<style>
	.page {
		min-height: 100%;
		padding: 24px;
		display: grid;
		align-content: start;
		gap: 18px;
		font: 14px system-ui;
	}
	.panel {
		border: 1px solid;
		border-radius: 12px;
		padding: 22px;
		box-shadow: 0 4px 18px #0001;
	}
	.heading,
	.item-heading,
	.form-actions,
	.actions {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
	}
	.heading {
		margin-bottom: 20px;
	}
	.heading h1 {
		font-size: 24px;
		margin: 5px 0;
	}
	.heading h2,
	h2,
	h3 {
		margin: 0;
	}
	.heading p {
		margin: 5px 0 0;
	}
	.eyebrow {
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.12em;
	}
	.fields {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
		margin: 15px 0;
	}
	.head-fields {
		max-width: 620px;
	}
	label {
		display: grid;
		gap: 6px;
		font-weight: 600;
		font-size: 12px;
	}
	.full {
		margin: 12px 0;
	}
	input,
	select,
	textarea {
		width: 100%;
		min-width: 0;
		border: 1px solid #64748b88;
		border-radius: 7px;
		padding: 9px;
		background: transparent;
		color: inherit;
		font: inherit;
	}
	select option {
		color: #111827;
	}
	textarea {
		resize: vertical;
	}
	button {
		border: 0;
		border-radius: 7px;
		padding: 8px 12px;
		background: #6366f1;
		color: white;
		cursor: pointer;
		font: inherit;
	}
	button:disabled {
		opacity: 0.55;
		cursor: wait;
	}
	.secondary {
		background: transparent;
		border: 1px solid #64748b88;
		color: inherit;
	}
	.text {
		background: transparent;
		color: #818cf8;
		padding: 4px;
	}
	.danger {
		color: #ef4444;
	}
	.item-heading {
		margin: 24px 0 12px;
	}
	.item-table td {
		padding: 6px;
		vertical-align: middle;
	}
	.item-table .name-input {
		min-width: 190px;
	}
	.item-table .short {
		min-width: 70px;
	}
	.form-actions {
		justify-content: flex-start;
		margin-top: 20px;
	}
	.tabs {
		display: flex;
		gap: 6px;
		margin-top: 10px;
	}
	.tab {
		background: transparent;
		border: 1px solid #64748b55;
		color: inherit;
		padding: 5px 10px;
		font-size: 12px;
	}
	.tab.active {
		background: #6366f1;
		border-color: #6366f1;
		color: white;
	}
	.table-wrap {
		overflow: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		text-align: left;
		padding: 11px;
		border-bottom: 1px solid #64748b44;
		vertical-align: top;
	}
	th {
		font-size: 12px;
		white-space: nowrap;
	}
	.num {
		text-align: right;
	}
	.mono,
	.name {
		font-family: ui-monospace, monospace;
	}
	.name {
		font-weight: 700;
	}
	.notes-preview {
		max-width: 420px;
		white-space: pre-wrap;
	}
	.actions {
		justify-content: flex-start;
		white-space: nowrap;
	}
	.details {
		padding: 4px 8px 12px;
	}
	.muted {
		color: #94a3b8;
	}
	.error {
		color: #ef4444;
	}
	.page :global(button:focus-visible),
	.page :global(input:focus-visible),
	.page :global(textarea:focus-visible),
	.page :global(select:focus-visible) {
		outline: 2px solid #818cf8;
		outline-offset: 2px;
	}
	@media (max-width: 760px) {
		.page {
			padding: 12px;
		}
		.panel {
			padding: 16px;
		}
		.fields {
			grid-template-columns: 1fr;
		}
		.heading {
			align-items: flex-start;
		}
		.table-wrap {
			font-size: 12px;
		}
	}
</style>
