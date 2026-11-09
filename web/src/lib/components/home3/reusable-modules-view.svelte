<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { SearchIcon, BoxesIcon, ArrowUpRightIcon, GripHorizontalIcon, XIcon } from '@lucide/svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { filterModules, type ReusableModule } from './reusable-modules';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let modules = $state<ReusableModule[]>([]);
	let query = $state('');
	let loading = $state(true);
	let failed = $state(false);
	let selected = $state<ReusableModule | null>(null);
	let dialog: HTMLDialogElement;
	let drag: { pointerId: number; offsetX: number; offsetY: number } | null = null;

	async function openDetails(module: ReusableModule, button: HTMLButtonElement) {
		const anchor = button.getBoundingClientRect();
		selected = module;
		await tick();
		dialog.removeAttribute('style');
		dialog.showModal();
		const rect = dialog.getBoundingClientRect();
		const gap = 12;
		if (anchor.right + gap + rect.width <= window.innerWidth - 8) {
			positionDialog(anchor.right + gap, anchor.top);
		} else if (anchor.left - gap - rect.width >= 8) {
			positionDialog(anchor.left - gap - rect.width, anchor.top);
		} else {
			const top = anchor.bottom + gap + rect.height <= window.innerHeight - 8
				? anchor.bottom + gap : anchor.top - gap - rect.height;
			positionDialog(anchor.left, top);
		}
	}

	function positionDialog(left: number, top: number) {
		const rect = dialog.getBoundingClientRect();
		dialog.style.margin = '0';
		dialog.style.inset = 'auto';
		dialog.style.left = `${Math.max(8, Math.min(left, window.innerWidth - rect.width - 8))}px`;
		dialog.style.top = `${Math.max(8, Math.min(top, window.innerHeight - rect.height - 8))}px`;
	}

	function startDrag(event: PointerEvent) {
		if (event.button !== 0) return;
		if ((event.target as Element).closest('button:not(.dialog-drag-handle)')) return;
		const rect = dialog.getBoundingClientRect();
		drag = { pointerId: event.pointerId, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top };
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault();
	}

	function moveDrag(event: PointerEvent) {
		if (drag?.pointerId !== event.pointerId) return;
		positionDialog(event.clientX - drag.offsetX, event.clientY - drag.offsetY);
	}

	function endDrag(event: PointerEvent) {
		if (drag?.pointerId !== event.pointerId) return;
		drag = null;
		const handle = event.currentTarget as HTMLElement;
		if (handle.hasPointerCapture(event.pointerId)) handle.releasePointerCapture(event.pointerId);
	}

	function moveWithKeyboard(event: KeyboardEvent) {
		const offsets: Record<string, [number, number]> = { ArrowLeft: [-10, 0], ArrowRight: [10, 0], ArrowUp: [0, -10], ArrowDown: [0, 10] };
		const delta = offsets[event.key];
		if (!delta) return;
		event.preventDefault();
		const rect = dialog.getBoundingClientRect();
		positionDialog(rect.left + delta[0], rect.top + delta[1]);
	}

	function keepDialogInView() {
		if (!dialog?.open || !dialog.style.left) return;
		const rect = dialog.getBoundingClientRect();
		positionDialog(rect.left, rect.top);
	}
	const visible = $derived(filterModules(modules, query));
	const statusLabel = (status: ReusableModule['status']) => ({ active: m.modules_active, 'under-dev': m.modules_under_dev, retired: m.modules_retired })[status]();
	const typeLabel = (type: ReusableModule['module_type']) => ({ frontend: m.modules_frontend, backend: m.modules_backend, script: m.modules_script })[type]();
	const formatDate = (date: string) => new Date(date).toLocaleString(getLocale());

	async function load() {
		loading = true;
		failed = false;
		try {
			const response = await fetch('/api/v1/reusable-modules', { credentials: 'same-origin' });
			if (!response.ok) throw new Error(String(response.status));
			modules = (await response.json()).modules;
		} catch {
			failed = true;
		} finally {
			loading = false;
		}
	}
	onMount(load);
</script>

<svelte:window onresize={keepDialogInView} />

<div class="modules-page select-text" class:light={!darkMode}>
	<header>
		<div class="heading">
			<div class="eyebrow"><BoxesIcon size={16} />{m.modules_eyebrow()}</div>
			<h1>{m.modules_title()}</h1>
			<p>{m.modules_description()}</p>
		</div>
		<div class="search-field">
			<label for="module-search">{m.modules_search_label()}</label>
			<div class="search-input"><SearchIcon size={18} /><input id="module-search" type="search" bind:value={query} placeholder={m.modules_search_placeholder()} /></div>
		</div>
	</header>
	{#if loading}
		<div class="state" role="status">{m.modules_loading()}</div>
	{:else if failed}
		<div class="state" role="alert"><p>{m.modules_load_error()}</p><button onclick={load}>{m.modules_retry()}</button></div>
	{:else}
		<p class="result-count" role="status">{m.modules_result_count({ count: visible.length, total: modules.length })}</p>
		{#if modules.length === 0}
			<div class="state"><BoxesIcon size={32} /><h2>{m.modules_empty()}</h2><p>{m.modules_empty_hint()}</p></div>
		{:else if visible.length === 0}
			<div class="state"><h2>{m.modules_no_matches()}</h2><p>{m.modules_no_matches_hint()}</p><button onclick={() => query = ''}>{m.modules_clear_search()}</button></div>
		{:else}
			<div class="module-grid">
				{#each visible as module (module.module_id)}
					<article class="module-card">
						<div class="card-top"><span class="module-id">{module.module_id}</span><span class="status" class:active={module.status === 'active'} class:retired={module.status === 'retired'}>{statusLabel(module.status)}</span></div>
						<h2 title={module.module_name}>{module.module_name}</h2>
						<div class="tags"><span>{typeLabel(module.module_type)}</span><span>{module.prog_language}</span><span>{m.modules_version({ version: module.version })}</span></div>
						<p class="description">{module.description || m.modules_no_description()}</p>
						<footer><span>{m.modules_file_count({ count: module.files.length })}</span><button aria-label={m.modules_details_for({ name: module.module_name })} onclick={(event) => openDetails(module, event.currentTarget)}>{m.modules_details()}<ArrowUpRightIcon size={15} /></button></footer>
					</article>
				{/each}
			</div>
		{/if}
	{/if}

	<dialog bind:this={dialog} onclose={() => { selected = null; drag = null; }} aria-labelledby="module-detail-title">
		{#if selected}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="detail-header" onpointerdown={startDrag} onpointermove={moveDrag} onpointerup={endDrag} onpointercancel={endDrag} onlostpointercapture={() => drag = null}>
				<button class="dialog-drag-handle" aria-label={m.modules_move_dialog()} title={m.modules_move_dialog()} onkeydown={moveWithKeyboard}><GripHorizontalIcon size={20} /></button>
				<p class="module-id">{selected.module_id}</p><button aria-label={m.modules_close()} onclick={() => dialog.close()}><XIcon size={20} /></button>
			</div>
			<div class="detail-body">
			<h2 id="module-detail-title">{selected.module_name}</h2>
			<p class="detail-description">{selected.description || m.modules_no_description()}</p>
			<dl>
				<dt>{m.modules_type()}</dt><dd>{typeLabel(selected.module_type)}</dd>
				<dt>{m.modules_status()}</dt><dd>{statusLabel(selected.status)}</dd>
				<dt>{m.modules_language()}</dt><dd>{selected.prog_language}</dd>
				<dt>{m.modules_version_label()}</dt><dd>{selected.version}</dd>
				<dt>{m.modules_capability()}</dt><dd>{selected.capability_label || selected.capability || m.modules_not_set()}</dd>
				<dt>{m.modules_use_when()}</dt><dd>{selected.use_when || m.modules_not_set()}</dd>
				<dt>{m.modules_documentation()}</dt><dd>{selected.doc_path || m.modules_not_set()}</dd>
				<dt>{m.modules_created()}</dt><dd>{formatDate(selected.created_at)} · {selected.created_by}</dd>
				<dt>{m.modules_updated()}</dt><dd>{formatDate(selected.updated_at)} · {selected.updated_by}</dd>
			</dl>
			<h3>{m.modules_files()}</h3>
			{#if selected.files.length}<ul>{#each selected.files as file}<li><code>{file}</code></li>{/each}</ul>{:else}<p>{m.modules_no_files()}</p>{/if}
			<h3>{m.modules_devdocs()}</h3>
			{#if selected.devdoc_paths.length}<ul>{#each selected.devdoc_paths as path}<li><code>{path}</code></li>{/each}</ul>{:else}<p>{m.modules_no_devdocs()}</p>{/if}
			<h3>{m.modules_configuration()}</h3><pre>{JSON.stringify(selected.config_json, null, 2)}</pre>
			</div>
		{/if}
	</dialog>
</div>

<style>
	.modules-page { --bg: #171b26; --surface: #1f2333; --text: #e2e8f0; --muted: #a0aec0; --border: #353e52; --accent: #5eead4; padding: 32px; height: 100%; overflow: auto; background: var(--bg); color: var(--text); user-select: text; container-type: inline-size; }
	.modules-page.light { --bg: #f4f6f8; --surface: #fff; --text: #172332; --muted: #526277; --border: #d7dfe7; --accent: #0f766e; }
	header { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; padding-bottom: 26px; border-bottom: 1px solid var(--border); }
	.eyebrow { display: flex; gap: 8px; align-items: center; color: var(--accent); font-size: 11px; font-weight: 600; letter-spacing: .12em; text-transform: uppercase; }
	h1 { margin: 8px 0; font-size: 30px; letter-spacing: -.04em; font-weight: 600; }
	header p, .result-count { color: var(--muted); font-size: 13px; }
	.search-field { width: 340px; max-width: 100%; flex-shrink: 0; }
	label { display: block; font-size: 12px; margin-bottom: 8px; color: var(--muted); }
	.search-input { display: flex; align-items: center; gap: 10px; border: 1px solid var(--border); background: var(--surface); border-radius: 8px; padding: 0 12px; color: var(--muted); }
	.search-input:focus-within { outline: 2px solid var(--accent); outline-offset: 2px; }
	input { width: 100%; min-width: 0; padding: 12px 0; background: transparent; border: 0; outline: 0; color: var(--text); font: inherit; font-size: 13px; }
	input::placeholder { color: var(--muted); }
	.result-count { margin: 20px 0 14px; font-variant-numeric: tabular-nums; }
	.module-grid { display: grid; grid-template-columns: repeat(auto-fill, 340px); gap: 18px; }
	.module-card { box-sizing: border-box; width: 340px; height: 280px; max-width: 100%; display: flex; flex-direction: column; padding: 20px; border: 1px solid var(--border); border-radius: 10px; background: #2d374b; }
	.light .module-card { background: #fff; }
	.card-top { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
	.module-id { font-family: monospace; font-size: 11px; color: var(--muted); }
	.status { font-size: 11px; color: #fbbf24; }
	.light .status { color: #92400e; }
	.status.active { color: var(--accent); }
	.status.retired { color: var(--muted); }
	.module-card h2 { font-size: 18px; font-weight: 600; line-height: 1.35; margin: 14px 0 10px; overflow: hidden; display: -webkit-box; line-clamp: 2; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; flex-shrink: 0; }
	.tags { display: flex; gap: 7px; flex-wrap: wrap; font-size: 11px; color: var(--muted); }
	.tags span { padding: 3px 7px; border: 1px solid var(--border); border-radius: 4px; }
	.description { flex: 1; min-height: 0; overflow: auto; overflow-wrap: anywhere; font-size: 13px; line-height: 1.6; color: var(--muted); margin: 12px 0; }
	footer { border-top: 1px solid var(--border); padding-top: 12px; display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 12px; color: var(--muted); }
	button { display: inline-flex; gap: 6px; align-items: center; padding: 5px 2px; background: transparent; border: 0; color: var(--accent); cursor: pointer; font: inherit; font-weight: 500; }
	button:hover { text-decoration: underline; }
	button:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: 3px; }
	.state { padding: 64px 20px; display: flex; flex-direction: column; align-items: center; gap: 12px; text-align: center; color: var(--muted); }
	.state h2 { font-size: 18px; color: var(--text); }
	dialog { position: fixed; width: min(680px, calc(100vw - 32px)); max-height: 85vh; box-sizing: border-box; padding: 0; border: 1px solid var(--border); border-radius: 12px; background: var(--surface); color: var(--text); user-select: text; }
	dialog::backdrop { background: #080e1bb3; }
	.detail-header { position: sticky; top: 0; z-index: 1; display: flex; align-items: center; gap: 16px; padding: 10px 20px; background: #34485c; border-bottom: 1px solid var(--accent); cursor: grab; touch-action: none; }
	.light .detail-header { background: #e0f2f1; }
	.detail-header:active { cursor: grabbing; }
	.detail-header .module-id { flex: 1; color: var(--text); }
	.detail-body { padding: 24px 28px; }
	.dialog-drag-handle { cursor: grab; touch-action: none; padding: 6px; user-select: none; }
	.dialog-drag-handle:active { cursor: grabbing; }
	.detail-body h2 { font-size: 23px; font-weight: 600; overflow-wrap: anywhere; }
	.detail-description { margin: 18px 0; color: var(--muted); line-height: 1.6; white-space: pre-wrap; overflow-wrap: anywhere; }
	dl { display: grid; grid-template-columns: 120px minmax(0, 1fr); gap: 12px; font-size: 13px; }
	dt { color: var(--muted); }
	dd { margin: 0; overflow-wrap: anywhere; }
	h3 { font-size: 14px; margin: 24px 0 10px; font-weight: 600; }
	ul { padding-left: 20px; }
	li { margin: 8px 0; overflow-wrap: anywhere; }
	code, pre { font-size: 12px; }
	pre { padding: 16px; background: var(--bg); border-radius: 6px; overflow: auto; }
	@media (max-width: 760px) { .modules-page { padding: 20px; } header { align-items: flex-start; flex-direction: column; } }
	@container (max-width: 339px) { .module-grid { grid-template-columns: minmax(0, 1fr); } .module-card { width: 100%; } }
	@media (max-width: 400px) { .module-grid { grid-template-columns: minmax(0, 1fr); } .module-card { width: 100%; } dl { grid-template-columns: 1fr; gap: 6px; } dd { margin-bottom: 8px; } }
</style>
