<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { localized } from '$lib/wpi/text';
	import { getPage, getPageRecovery, searchPages, type PageSummary } from '$lib/wpi/api';
	import type { JsonObject } from '$lib/wpi/types';
	import { canvasThemeVars } from './canvas-theme';

	let {
		darkMode = true,
		onClose,
		onLoad
	}: {
		darkMode?: boolean;
		onClose: () => void;
		onLoad: (page: JsonObject, version: number, recovery: JsonObject | null) => void;
	} = $props();
	let dialog: HTMLDialogElement;
	let descriptor = $state('');
	let description = $state('');
	let createdFrom = $state('');
	let createdTo = $state('');
	let updatedFrom = $state('');
	let updatedTo = $state('');
	let pages = $state<PageSummary[]>([]);
	let offset = $state(0);
	let hasMore = $state(false);
	let busy = $state(false);
	let loadingId = $state('');
	let error = $state('');
	let request: AbortController | undefined;
	const limit = 20;
	let filters = new URLSearchParams();

	// Date fields represent local calendar days; an end date includes the whole day.
	function dateBound(value: string, end: boolean): string {
		const date = new Date(`${value}T00:00:00`);
		if (end) date.setDate(date.getDate() + 1);
		return date.toISOString();
	}
	async function list(nextOffset: number, search = false) {
		request?.abort();
		const controller = new AbortController();
		request = controller;
		error = '';
		busy = true;
		try {
			if (search) {
				filters = new URLSearchParams({
					descriptor_id: descriptor.trim(),
					description: description.trim()
				});
				for (const [key, value, end] of [
					['created_from', createdFrom, false],
					['created_to', createdTo, true],
					['updated_from', updatedFrom, false],
					['updated_to', updatedTo, true]
				] as const) {
					if (value) filters.set(key, dateBound(value, end));
				}
			}
			const params = new URLSearchParams(filters);
			params.set('limit', String(limit));
			params.set('offset', String(nextOffset));
			const result = await searchPages(params, controller.signal);
			if (controller.signal.aborted) return;
			pages = result.pages;
			hasMore = result.has_more;
			offset = nextOffset;
		} catch {
			if (!controller.signal.aborted) {
				error = m.wpi_load_search_failed();
				pages = [];
				hasMore = false;
			}
		} finally {
			if (request === controller) busy = false;
		}
	}
	async function load(id: string) {
		request?.abort();
		const controller = new AbortController();
		request = controller;
		loadingId = id;
		error = '';
		try {
			const result = await getPage(id, controller.signal);
			const recovery = await getPageRecovery(id, controller.signal);
			if (!controller.signal.aborted) onLoad(result.page, result.version, recovery);
		} catch {
			if (!controller.signal.aborted) error = m.wpi_load_page_failed();
		} finally {
			if (request === controller) loadingId = '';
		}
	}
	onMount(() => {
		dialog.showModal();
		void list(0, true);
		return () => request?.abort();
	});
</script>

<dialog
	bind:this={dialog}
	onclose={onClose}
	class="select-text"
	style={canvasThemeVars(darkMode)}
	aria-labelledby="wpi-load-title"
>
	<header>
		<h2 id="wpi-load-title">{m.wpi_load_title()}</h2>
		<button type="button" onclick={onClose} aria-label={m.wpi_load_close()}>✕</button>
	</header>
	<form
		onsubmit={(e) => {
			e.preventDefault();
			void list(0, true);
		}}
	>
		<div class="filters">
			<label>{m.wpi_load_descriptor()}<input bind:value={descriptor} /></label>
			<label>{m.wpi_load_description()}<input bind:value={description} /></label>
			<label
				>{m.wpi_load_created_from()}<input
					type="date"
					bind:value={createdFrom}
					max={createdTo || undefined}
				/></label
			>
			<label
				>{m.wpi_load_created_to()}<input
					type="date"
					bind:value={createdTo}
					min={createdFrom || undefined}
				/></label
			>
			<label
				>{m.wpi_load_updated_from()}<input
					type="date"
					bind:value={updatedFrom}
					max={updatedTo || undefined}
				/></label
			>
			<label
				>{m.wpi_load_updated_to()}<input
					type="date"
					bind:value={updatedTo}
					min={updatedFrom || undefined}
				/></label
			>
		</div>
		<button type="submit" disabled={busy || !!loadingId}>{m.wpi_load_search()}</button>
	</form>
	{#if error}<p role="alert">{error}</p>{/if}
	<div class="results" aria-busy={busy}>
		{#if busy}<p role="status">{m.wpi_load_searching()}</p>
		{:else if !pages.length}<p>{m.wpi_load_empty()}</p>
		{:else}
			<table>
				<thead
					><tr
						><th>{m.wpi_load_descriptor()}</th><th>{m.wpi_load_description_label()}</th><th
							>{m.wpi_load_created()}</th
						><th>{m.wpi_load_updated()}</th><th>{m.wpi_load_title()}</th></tr
					></thead
				>
				<tbody>
					{#each pages as page (page.descriptor_id)}
						<tr>
							<td
								><strong>{page.descriptor_id}</strong>
								<div>{localized(page.page_name ?? undefined)}</div></td
							>
							<td>{localized(page.description ?? undefined)}</td>
							<td>{new Date(page.created_at).toLocaleString(getLocale())}</td>
							<td>{new Date(page.updated_at).toLocaleString(getLocale())}</td>
							<td
								><button
									type="button"
									disabled={!!loadingId}
									onclick={() => load(page.descriptor_id)}
									>{loadingId === page.descriptor_id
										? m.wpi_load_loading()
										: m.wpi_load_open()}</button
								></td
							>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
	<footer>
		<button
			type="button"
			disabled={offset === 0 || busy || !!loadingId}
			onclick={() => list(Math.max(0, offset - limit))}>{m.wpi_load_previous()}</button
		>
		<span>{m.wpi_load_page_number({ page: Math.floor(offset / limit) + 1 })}</span>
		<button
			type="button"
			disabled={!hasMore || busy || !!loadingId}
			onclick={() => list(offset + limit)}>{m.wpi_load_next()}</button
		>
	</footer>
</dialog>

<style>
	dialog {
		margin: auto;
		width: 1000px;
		max-width: calc(100vw - 32px);
		max-height: 85vh;
		padding: 0;
		border: 1px solid var(--cv-divider);
		border-radius: 12px;
		background: var(--cv-panel);
		color: var(--cv-text);
		font-size: 12px;
	}
	dialog::backdrop {
		background: #0008;
	}
	header,
	footer {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 16px 20px;
	}
	h2 {
		font-size: 16px;
		font-weight: 600;
		margin: 0;
	}
	form {
		padding: 0 20px 16px;
		border-bottom: 1px solid var(--cv-divider);
	}
	.filters {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
		margin-bottom: 12px;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 5px;
	}
	input {
		width: 100%;
		min-width: 0;
		background: var(--cv-field);
		color: var(--cv-text);
		border: 1px solid var(--cv-divider);
		border-radius: 4px;
		padding: 7px;
		color-scheme: light dark;
	}
	button {
		border: 1px solid var(--cv-divider);
		background: var(--cv-raised);
		color: var(--cv-text);
		padding: 6px 12px;
		border-radius: 4px;
		cursor: pointer;
	}
	.results button {
		white-space: nowrap;
	}
	button:disabled {
		opacity: 0.45;
		cursor: default;
	}
	button:hover:not(:disabled) {
		background: var(--cv-hover);
	}
	.results {
		overflow: auto;
		max-height: 40vh;
		min-height: 120px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		text-align: left;
	}
	th,
	td {
		padding: 12px;
		border-bottom: 1px solid var(--cv-divider);
		vertical-align: top;
		overflow-wrap: anywhere;
	}
	th {
		color: var(--cv-text-muted);
		font-weight: 500;
	}
	p {
		padding: 12px 20px;
	}
	[role='alert'] {
		color: var(--cv-warn);
	}
	@media (max-width: 600px) {
		.filters {
			grid-template-columns: 1fr;
		}
	}
</style>
