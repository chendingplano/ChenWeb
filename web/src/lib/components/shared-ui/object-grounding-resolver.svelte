<script lang="ts">
	import { untrack } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { movableDialog } from './movable-dialog';
	import { searchObjects, type ObjectNodeSummary } from '$lib/services/objectManagerService';
	import type { GroundingItem, GroundingInput } from './object-grounding-types';

	let { items, resolve, onclose, anchor, darkMode = false, afterResolveHint = '' }: {
		items: GroundingItem[];
		resolve: (item: GroundingItem, input: GroundingInput) => Promise<void>;
		onclose: () => void;
		anchor?: DOMRect;
		darkMode?: boolean;
		afterResolveHint?: string;
	} = $props();
	const titleId = $props.id();
	let dialog = $state<HTMLDialogElement>();
	let selectedId = $state('');
	const selected = $derived(items.find((item) => item.id === selectedId) ?? items[0]);
	let query = $state('');
	let candidates = $state<ObjectNodeSummary[]>([]);
	let picked = $state('');
	let searching = $state(false);
	let saving = $state(false);
	let error = $state('');
	let notice = $state('');
	let name = $state('');
	let nameEN = $state('');
	let nameZH = $state('');
	let aliases = $state('');
	let description = $state('');
	let searchVersion = 0;
	const candidate = $derived(candidates.find((item) => item.object_id === picked));
	const usable = (item: ObjectNodeSummary) => !['rejected', 'merged', 'ambiguous', 'pending_review'].includes(item.reconcile_status);

	$effect(() => { if (dialog && !dialog.open) dialog.show(); });
	$effect(() => {
		const id = selected?.id;
		untrack(() => {
			const item = items.find((item) => item.id === id);
			picked = ''; error = ''; candidates = [];
			name = item?.name ?? ''; nameEN = item?.nameEN ?? ''; nameZH = item?.nameZH ?? '';
			aliases = item?.aliases?.join('\n') ?? ''; description = item?.description ?? '';
			query = item?.label ?? '';
			if (item) void search();
			else { searchVersion++; searching = false; }
		});
	});

	async function search() {
		const version = ++searchVersion;
		searching = true; error = ''; picked = '';
		try {
			const result = await searchObjects({ table: 'object_nodes', query: query.trim(), page: 1, page_size: 50 });
			if (!result.status) throw new Error();
			if (version === searchVersion) candidates = result.rows as ObjectNodeSummary[];
		} catch {
			if (version === searchVersion) error = m.grounding_search_failed();
		} finally { if (version === searchVersion) searching = false; }
	}

	async function save(create: boolean) {
		const item = selected;
		if (!item || saving || (create ? !name.trim() : !candidate || !usable(candidate))) return;
		saving = true; error = ''; notice = '';
		try {
			await resolve(item, create ? {
				create: true, name: name.trim(), name_en: nameEN.trim(), name_zh: nameZH.trim(),
				aliases: aliases.split(/\n|,/).map((value) => value.trim()).filter(Boolean), description: description.trim()
			} : { object_id: picked });
			notice = m.grounding_resolved({ name: item.label });
		} catch (e) {
			error = e instanceof Error ? e.message : m.grounding_save_failed();
		} finally { saving = false; }
	}
</script>

<dialog bind:this={dialog} class:dark={darkMode} aria-labelledby={titleId} onclose={onclose} oncancel={(event) => { if (saving) event.preventDefault(); }}>
	<header use:movableDialog={anchor}>
		<h2 id={titleId}>{m.grounding_title()} <span>{items.length}</span></h2>
		<button type="button" onclick={() => dialog?.close()} disabled={saving} aria-label={m.grounding_close()}>×</button>
	</header>
	<p class="hint">{m.grounding_hint()}</p>
	{#if afterResolveHint}<p class="hint">{afterResolveHint}</p>{/if}
	{#if error}<p role="alert" class="error">{error}</p>{/if}
	{#if notice}<p role="status" class="notice">{notice}</p>{/if}
	{#if items.length === 0}
		<p class="empty">{m.grounding_empty()}</p>
	{:else}
		<div class="workspace">
			<nav aria-label={m.grounding_items()}>
				{#each items as item (item.id)}
					<button type="button" class:active={selected?.id === item.id} disabled={saving} onclick={() => selectedId = item.id}>
						<strong>{item.label}</strong><small>{item.kind}</small>
					</button>
				{/each}
			</nav>
			<section>
				{#if selected}
					<h3>{selected.label}</h3>
					<dl>
						<dt>{m.grounding_name()}</dt><dd>{selected.name}</dd>
						{#if selected.nameEN}<dt>{m.grounding_name_en()}</dt><dd>{selected.nameEN}</dd>{/if}
						{#if selected.description}<dt>{m.grounding_description()}</dt><dd>{selected.description}</dd>{/if}
						{#if selected.aliases?.length}<dt>{m.grounding_aliases()}</dt><dd>{selected.aliases.join(' · ')}</dd>{/if}
						{#each selected.details ?? [] as detail}<dt>{detail.label}</dt><dd>{detail.value}</dd>{/each}
					</dl>
				{/if}
				<h3>{m.grounding_existing()}</h3>
				<form class="search" onsubmit={(event) => { event.preventDefault(); void search(); }}>
					<input aria-label={m.grounding_search()} bind:value={query} disabled={saving} />
					<button disabled={searching || saving}>{searching ? m.grounding_searching() : m.grounding_search()}</button>
				</form>
				<div class="candidates" aria-busy={searching}>
					{#each candidates as item (item.object_id)}
						<label class="candidate">
							<input type="radio" name={`${titleId}-object`} value={item.object_id} bind:group={picked} disabled={saving || searching || !usable(item)} />
							<span><strong>{item.canonical_name}</strong><small>{item.canonical_name_en} · {item.object_type} · {item.reconcile_status}</small><small>{item.object_id}</small></span>
						</label>
					{:else}<p>{searching ? m.grounding_searching() : m.grounding_no_matches()}</p>{/each}
				</div>
				<button class="primary" onclick={() => save(false)} disabled={saving || searching || !candidate || !usable(candidate)}>{m.grounding_link()}</button>
				<details>
					<summary>{m.grounding_add_new()}</summary>
					<p class="hint">{m.grounding_create_hint()}</p>
					<form class="create" onsubmit={(event) => { event.preventDefault(); void save(true); }}>
						<label>{m.grounding_name()}<input required bind:value={name} disabled={saving} /></label>
						<label>{m.grounding_name_en()}<input bind:value={nameEN} disabled={saving} /></label>
						<label>{m.grounding_name_zh()}<input bind:value={nameZH} disabled={saving} /></label>
						<label>{m.grounding_aliases()}<textarea bind:value={aliases} disabled={saving}></textarea></label>
						<label>{m.grounding_description()}<textarea bind:value={description} disabled={saving}></textarea></label>
						<button class="primary" disabled={saving || !name.trim()}>{saving ? m.grounding_saving() : m.grounding_add_link()}</button>
					</form>
				</details>
			</section>
		</div>
	{/if}
</dialog>

<style>
	dialog { --bg: #fff; --panel: #f6f8fa; --line: #cbd5e1; --text: #172033; --muted: #58677d; --accent: #9a4b14; position: fixed; z-index: 1000; width: 900px; height: 680px; min-width: min(520px, calc(100vw - 16px)); min-height: 280px; max-width: calc(100vw - 16px); max-height: calc(100dvh - 16px); padding: 0; resize: both; overflow: auto; color: var(--text); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; box-shadow: 0 16px 60px #0005; user-select: text; }
	dialog.dark { --bg: #111827; --panel: #1a2737; --line: #344b65; --text: #f1f5f9; --muted: #b5c3d5; --accent: #ff9d5c; }
	header { display: flex; justify-content: space-between; align-items: center; padding: 12px 18px; background: var(--panel); border-bottom: 1px solid var(--line); cursor: move; touch-action: none; }
	h2 { margin: 0; font-size: 16px; } h2 span { margin-left: 8px; color: var(--accent); } h3 { font-size: 14px; margin: 0 0 12px; }
	button, input, textarea { font: inherit; font-size: 13px; color: var(--text); border: 1px solid var(--line); border-radius: 4px; background: var(--bg); }
	button { padding: 7px 12px; cursor: pointer; } button:disabled { opacity: .5; cursor: default; } button:focus-visible, input:focus-visible, textarea:focus-visible, summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	.primary { color: var(--accent); border-color: var(--accent); } input, textarea { padding: 7px 9px; min-width: 0; } textarea { min-height: 65px; }
	.hint { margin: 12px 18px; color: var(--muted); font-size: 12px; line-height: 1.5; } .error, .notice { margin: 12px 18px; font-size: 13px; } .error { color: #e25555; } .notice { color: var(--accent); } .empty { padding: 24px; }
	.workspace { display: grid; grid-template-columns: 220px minmax(0, 1fr); border-top: 1px solid var(--line); } nav { background: var(--panel); padding: 8px; border-right: 1px solid var(--line); } nav button { display: block; width: 100%; text-align: left; margin-bottom: 6px; } nav button.active { border-color: var(--accent); }
	small { display: block; color: var(--muted); margin-top: 4px; overflow-wrap: anywhere; } section { padding: 18px; min-width: 0; } dl { display: grid; grid-template-columns: 120px minmax(0, 1fr); gap: 8px; font-size: 12px; margin: 0 0 24px; } dt { color: var(--muted); } dd { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
	.search { display: flex; gap: 8px; } .search input { flex: 1; } .candidates { margin: 12px 0; max-height: 230px; overflow: auto; } .candidate { display: flex; align-items: start; gap: 8px; border: 1px solid var(--line); padding: 9px; margin-bottom: 6px; font-size: 13px; } .candidate input { flex-shrink: 0; margin-top: 4px; }
	details { margin-top: 22px; border-top: 1px solid var(--line); padding-top: 16px; } summary { cursor: pointer; font-size: 14px; } .create { display: grid; gap: 10px; margin-top: 12px; } .create label { display: grid; gap: 5px; font-size: 12px; } .create .primary { justify-self: start; } details .hint { margin: 10px 0; }
	@media (max-width: 650px) { .workspace { grid-template-columns: 150px minmax(0, 1fr); } dl { grid-template-columns: 1fr; gap: 4px; } dd { margin-bottom: 6px; } section { padding: 12px; } }
</style>
