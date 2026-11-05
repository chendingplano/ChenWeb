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
	let drag: { x: number; y: number } | undefined;
	// Dragging moves the dialog by its header; the buttons in it keep their own clicks.
	function startDrag(e: PointerEvent) {
		if (e.button !== 0 || (e.target as Element).closest('button')) return;
		const rect = dialog.getBoundingClientRect();
		dialog.style.margin = '0';
		dialog.style.left = `${rect.left}px`;
		dialog.style.top = `${rect.top}px`;
		dialog.style.right = 'auto';
		dialog.style.bottom = 'auto';
		drag = { x: e.clientX - rect.left, y: e.clientY - rect.top };
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}
	function moveDrag(e: PointerEvent) {
		if (!drag) return;
		const left = Math.min(Math.max(0, e.clientX - drag.x), window.innerWidth - dialog.offsetWidth);
		const top = Math.min(Math.max(0, e.clientY - drag.y), window.innerHeight - dialog.offsetHeight);
		dialog.style.left = `${left}px`;
		dialog.style.top = `${top}px`;
	}
	function endDrag() {
		drag = undefined;
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
	<header
		onpointerdown={startDrag}
		onpointermove={moveDrag}
		onpointerup={endDrag}
		onpointercancel={endDrag}
	>
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
			<div class="cards">
				{#each pages as page (page.descriptor_id)}
					{@const description = localized(page.description ?? undefined) ?? ''}
					<div
						class="card"
						role="button"
						tabindex="0"
						aria-disabled={!!loadingId}
						ondblclick={() => !loadingId && load(page.descriptor_id)}
						onkeydown={(e) => {
							if (e.key === 'Enter' && !loadingId) load(page.descriptor_id);
						}}
					>
						<div class="thumb">
							{#if page.page_image}
								<img
									src={`/api/v1/wpi/page-images/${encodeURIComponent(page.page_image)}`}
									alt={localized(page.page_name ?? undefined)}
									loading="lazy"
								/>
							{/if}
						</div>
						<div class="body">
							<strong>{m.wpi_load_card_name({ name: localized(page.page_name ?? undefined) ?? '' })}</strong>
							<span class="id">{m.wpi_load_card_id({ id: page.descriptor_id })}</span>
							<p class="desc" title={description}>
								{m.wpi_load_card_description({ description })}
							</p>
							<dl>
								<dt>{m.wpi_load_created()}</dt>
								<dd>{new Date(page.created_at).toLocaleString(getLocale())}</dd>
								<dt>{m.wpi_load_updated()}</dt>
								<dd>{new Date(page.updated_at).toLocaleString(getLocale())}</dd>
							</dl>
							{#if loadingId === page.descriptor_id}<span class="loading">{m.wpi_load_loading()}</span>{/if}
						</div>
					</div>
				{/each}
			</div>
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
		display: flex;
		flex-direction: column;
		width: 1000px;
		height: 85vh;
		min-width: min(480px, calc(100vw - 32px));
		min-height: 360px;
		max-width: calc(100vw - 32px);
		max-height: calc(100vh - 32px);
		overflow: hidden;
		resize: both;
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
		flex-shrink: 0;
		display: flex;
		cursor: move;
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
		flex-shrink: 0;
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
	button:disabled {
		opacity: 0.45;
		cursor: default;
	}
	button:hover:not(:disabled) {
		background: var(--cv-hover);
	}
	.results {
		flex: 1;
		overflow: auto;
		min-height: 120px;
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
		gap: 16px;
		padding: 0 20px 20px;
	}
	.card {
		display: flex;
		flex-direction: column;
		overflow: hidden;
		border: 1px solid var(--cv-divider);
		border-radius: 12px;
		background: var(--cv-raised);
		cursor: pointer;
	}
	.card:hover,
	.card:focus-visible {
		background: var(--cv-hover);
		outline: none;
		border-color: var(--cv-text-muted);
	}
	.thumb {
		box-sizing: border-box;
		padding: 15px 10px 0;
		aspect-ratio: 16 / 10;
		/* Mixing in the text colour gives a tint that stands out from the card in both themes. */
		background: color-mix(in srgb, var(--cv-text) 10%, var(--cv-panel));
	}
	.thumb img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
		object-position: top;
	}
	.body {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 12px;
		min-width: 0;

		background: color-mix(in srgb, var(--cv-text) 10%, var(--cv-panel));
	}
	.body strong {
		font-size: 13px;
		overflow-wrap: anywhere;
	}
	.body .id {
		color: var(--cv-text-muted);
		overflow-wrap: anywhere;
	}
	.desc {
		margin: 0;
		padding: 0;
		line-height: 1.4;
		min-height: 2.8em;
		color: var(--cv-text-muted);
		display: -webkit-box;
		line-clamp: 2;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 2px 8px;
		margin: 0;
		color: var(--cv-text-muted);
	}
	dt,
	dd {
		margin: 0;
	}
	.loading {
		color: var(--cv-text-muted);
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
