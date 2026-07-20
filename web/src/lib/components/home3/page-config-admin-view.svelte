<script lang="ts">
	import { onMount } from 'svelte';
	import { locales } from '$lib/paraglide/runtime';
	import {
		listPages,
		listEntries,
		upsertEntry,
		deleteEntry,
		type PageDef,
		type AdminEntry
	} from '$lib/services/pageConfigService';

	// Admin tooling for DB-backed page content (kb.page_def / kb.page_config;
	// spec 2026072001 §9). Overlay model: an entry can override text, hide an
	// item (enabled=off), suspend it (accessible=off), or scope it by role.
	// Deleting an entry removes the overlay entirely, so the page reverts to its
	// own built-in default text (and the item becomes visible to all users).
	// Entries are referenced by page_key + entry_key, never by display text.

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let pages = $state<PageDef[]>([]);
	let selectedPageKey = $state<string>('');
	let entries = $state<AdminEntry[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	// Modal editor state (editorOpen drives the dialog; null entryKey = create).
	let editorOpen = $state(false);
	let editingEntryKey = $state<string | null>(null);
	let formEntryKey = $state('');
	let formEntryDesc = $state('');
	let formLabels = $state<Record<string, string>>({});
	let formDescriptions = $state<Record<string, string>>({});
	let formAccessRoles = $state('');
	let formAccessible = $state(true);
	let formEnabled = $state(true);
	let saving = $state(false);
	let formError = $state<string | null>(null);

	const selectedPage = $derived(pages.find((p) => p.page_key === selectedPageKey) ?? null);

	async function loadPages() {
		loading = true;
		loadError = null;
		try {
			pages = await listPages();
			if (pages.length > 0 && !selectedPageKey) selectedPageKey = pages[0].page_key;
			if (selectedPageKey) await loadEntries();
		} catch (e) {
			loadError = String(e);
		} finally {
			loading = false;
		}
	}

	async function loadEntries() {
		if (!selectedPageKey) return;
		loadError = null;
		try {
			entries = (await listEntries(selectedPageKey)).sort((a, b) =>
				a.entry_key.localeCompare(b.entry_key)
			);
		} catch (e) {
			loadError = String(e);
		}
	}

	onMount(loadPages);

	function onSelectPage() {
		loadEntries();
	}

	function openNew() {
		editingEntryKey = null;
		formEntryKey = '';
		formEntryDesc = '';
		formLabels = {};
		formDescriptions = {};
		formAccessRoles = '';
		formAccessible = true;
		formEnabled = true;
		formError = null;
		editorOpen = true;
	}

	function startEdit(entry: AdminEntry) {
		editingEntryKey = entry.entry_key;
		formEntryKey = entry.entry_key;
		formEntryDesc = entry.entry_desc ?? '';
		formLabels = {};
		formDescriptions = {};
		for (const lang of locales) {
			formLabels[lang] = entry.content?.[lang]?.label ?? '';
			formDescriptions[lang] = entry.content?.[lang]?.description ?? '';
		}
		formAccessRoles = (entry.access_role ?? []).join(', ');
		formAccessible = entry.accessible;
		formEnabled = entry.enabled;
		formError = null;
		editorOpen = true;
	}

	function closeEditor() {
		if (saving) return;
		editorOpen = false;
	}

	async function submitForm() {
		const entryKey = formEntryKey.trim();
		if (!entryKey) {
			formError = 'entry_key is required';
			return;
		}
		saving = true;
		formError = null;
		try {
			const content: AdminEntry['content'] = {};
			for (const lang of locales) {
				const label = (formLabels[lang] ?? '').trim();
				const description = (formDescriptions[lang] ?? '').trim();
				const c: { label?: string; description?: string } = {};
				if (label) c.label = label;
				if (description) c.description = description;
				content[lang] = c;
			}
			const accessRole = formAccessRoles
				.split(',')
				.map((r) => r.trim())
				.filter(Boolean);
			await upsertEntry(selectedPageKey, {
				entry_key: entryKey,
				entry_desc: formEntryDesc.trim(),
				content,
				access_role: accessRole,
				accessible: formAccessible,
				enabled: formEnabled
			});
			editorOpen = false;
			await loadEntries();
		} catch (e) {
			formError = String(e);
		} finally {
			saving = false;
		}
	}

	async function remove(entry: AdminEntry) {
		const msg =
			`Delete configuration for "${entry.entry_key}"?\n\n` +
			`This removes the entry for all languages. The item will revert to the ` +
			`page's built-in default text and become visible to all users (its label/` +
			`description overrides and role restrictions are removed).\n\n` +
			`To hide the item instead, edit it and turn off "Enabled".`;
		if (!confirm(msg)) return;
		try {
			await deleteEntry(selectedPageKey, entry.entry_key);
			await loadEntries();
		} catch (e) {
			loadError = String(e);
		}
	}

	function status(entry: AdminEntry): string {
		if (!entry.enabled) return 'disabled (hidden)';
		if (!entry.accessible) return 'suspended (hidden)';
		if (!entry.access_role || entry.access_role.length === 0) return 'no roles (hidden)';
		return 'active';
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#6366F1' : '#4F46E5');
	const danger = $derived(darkMode ? '#F87171' : '#DC2626');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const rowHover = $derived(darkMode ? '#252A3C' : '#F3F4F6');
	const overlay = $derived(darkMode ? 'rgba(4,10,22,0.72)' : 'rgba(15,23,42,0.32)');
</script>

<div
	class="pcadmin"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--danger={danger}
	style:--input-bg={inputBg}
	style:--row-hover={rowHover}
	style:--overlay={overlay}
>
	<header class="head">
		<div>
			<h1>Page Content Configuration</h1>
			<p class="sub">
				DB-backed, language-aware content for configurable pages. Entries are keyed by
				<code>page_key + entry_key</code>. Disable, suspend, or clear roles to hide an entry;
				delete to revert it to the page's built-in default.
			</p>
		</div>
	</header>

	<div class="controls">
		<label class="field">
			<span>Page</span>
			<select bind:value={selectedPageKey} onchange={onSelectPage}>
				{#each pages as p (p.page_key)}
					<option value={p.page_key}>{p.title || p.page_key} ({p.route})</option>
				{/each}
			</select>
		</label>
		{#if selectedPage}
			<span class="route-hint"><code>{selectedPage.route}</code></span>
		{/if}
		<button type="button" class="primary" onclick={openNew}>+ New entry</button>
	</div>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if loadError}
		<p class="error">{loadError}</p>
	{:else}
		<div class="table-wrap">
			<table>
				<thead>
					<tr>
						<th>entry_key</th>
						<th>description</th>
						<th>status</th>
						<th>access_role</th>
						{#each locales as lang (lang)}
							<th>{lang} label</th>
						{/each}
						<th></th>
					</tr>
				</thead>
				<tbody>
					{#each entries as entry (entry.entry_key)}
						<tr class:inactive={status(entry) !== 'active'}>
							<td><code>{entry.entry_key}</code></td>
							<td>{entry.entry_desc || '—'}</td>
							<td>{status(entry)}</td>
							<td>{(entry.access_role ?? []).join(', ') || '—'}</td>
							{#each locales as lang (lang)}
								<td>{entry.content?.[lang]?.label ?? '—'}</td>
							{/each}
							<td class="row-actions">
								<button type="button" onclick={() => startEdit(entry)}>Edit</button>
								<button type="button" class="link-danger" onclick={() => remove(entry)}>Delete</button>
							</td>
						</tr>
					{/each}
					{#if entries.length === 0}
						<tr><td colspan={locales.length + 5} class="muted">No entries for this page.</td></tr>
					{/if}
				</tbody>
			</table>
		</div>
	{/if}
</div>

{#if editorOpen}
	<div
		class="backdrop"
		role="button"
		tabindex="-1"
		onclick={closeEditor}
		onkeydown={(e) => e.key === 'Escape' && closeEditor()}
		style:--card={card}
		style:--border={border}
		style:--heading={heading}
		style:--sub={sub}
		style:--btn={btn}
		style:--input-bg={inputBg}
		style:--overlay={overlay}
	>
		<div
			class="dialog"
			role="dialog"
			aria-modal="true"
			aria-label="Edit page content entry"
			tabindex="-1"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<div class="dialog-head">
				<div class="dialog-title">{editingEntryKey ? `Edit "${editingEntryKey}"` : 'New entry'}</div>
				<button type="button" class="ghost" onclick={closeEditor} disabled={saving}>Close</button>
			</div>
			<form class="dialog-body" onsubmit={(e) => (e.preventDefault(), submitForm())}>
				<label class="wide">
					<span>entry_key</span>
					<input bind:value={formEntryKey} disabled={!!editingEntryKey} placeholder="stable id" />
				</label>
				<label class="wide">
					<span>Description (what this entry is)</span>
					<input bind:value={formEntryDesc} placeholder="e.g. Wiki sidebar menu item" />
				</label>

				{#each locales as lang (lang)}
					<label>
						<span>{lang} label</span>
						<input bind:value={formLabels[lang]} placeholder="(built-in default)" />
					</label>
					<label>
						<span>{lang} description</span>
						<input bind:value={formDescriptions[lang]} placeholder="(built-in default)" />
					</label>
				{/each}

				<label class="wide">
					<span>access_role (comma-separated role keys)</span>
					<input bind:value={formAccessRoles} placeholder="admin, dev, guest" />
				</label>
				<label class="check"><input type="checkbox" bind:checked={formEnabled} /> Enabled</label>
				<label class="check"
					><input type="checkbox" bind:checked={formAccessible} /> Accessible</label
				>

				{#if formError}<p class="error wide">{formError}</p>{/if}

				<div class="dialog-foot wide">
					<button type="button" class="ghost" onclick={closeEditor} disabled={saving}>Cancel</button>
					<button type="submit" class="primary" disabled={saving}>{saving ? 'Saving…' : 'Save'}</button>
				</div>
			</form>
		</div>
	</div>
{/if}

<style>
	.pcadmin {
		padding: 1.75rem;
		background: var(--page);
		min-height: 100%;
		font-size: 0.9rem;
		color: var(--heading);
	}
	h1 {
		font-size: 1.35rem;
		margin: 0 0 0.25rem;
		color: var(--heading);
	}
	.sub {
		color: var(--sub);
		margin: 0 0 1.25rem;
		max-width: 68ch;
	}
	code {
		font-family: ui-monospace, monospace;
		font-size: 0.85em;
	}
	.controls {
		display: flex;
		align-items: end;
		gap: 1rem;
		margin-bottom: 1rem;
		flex-wrap: wrap;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.field span {
		color: var(--sub);
		font-size: 0.8rem;
	}
	.route-hint {
		color: var(--sub);
		padding-bottom: 0.4rem;
	}
	.table-wrap {
		overflow-x: auto;
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		text-align: left;
		padding: 0.5rem 0.7rem;
		border-bottom: 1px solid var(--border);
		vertical-align: top;
		white-space: nowrap;
	}
	th {
		color: var(--sub);
		font-weight: 600;
		font-size: 0.78rem;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}
	tbody tr:hover {
		background: var(--row-hover);
	}
	tr.inactive {
		opacity: 0.55;
	}
	.row-actions {
		white-space: nowrap;
		display: flex;
		gap: 0.5rem;
	}
	.muted {
		color: var(--sub);
	}
	.error {
		color: var(--danger);
	}
	button {
		padding: 0.35rem 0.8rem;
		border-radius: 7px;
		border: 1px solid var(--border);
		background: var(--card);
		color: var(--heading);
		cursor: pointer;
		font-size: 0.85rem;
	}
	button.primary {
		background: var(--btn);
		border-color: var(--btn);
		color: #fff;
	}
	button.ghost {
		background: transparent;
	}
	button.link-danger {
		border: none;
		background: transparent;
		color: var(--danger);
		padding: 0.35rem 0.4rem;
	}
	button:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.backdrop {
		position: fixed;
		inset: 0;
		z-index: 70;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 24px;
		background: var(--overlay);
	}
	.dialog {
		width: min(680px, 100%);
		max-height: 90vh;
		overflow-y: auto;
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 16px;
		box-shadow: 0 24px 90px rgba(0, 0, 0, 0.34);
	}
	.dialog-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 18px;
		border-bottom: 1px solid var(--border);
	}
	.dialog-title {
		color: var(--heading);
		font-size: 1.05rem;
		font-weight: 700;
	}
	.dialog-body {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.9rem;
		padding: 18px;
	}
	.dialog-body label {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.dialog-body label span {
		color: var(--sub);
		font-size: 0.8rem;
	}
	.dialog-body .wide {
		grid-column: 1 / -1;
	}
	.dialog-body .check {
		flex-direction: row;
		align-items: center;
		gap: 0.5rem;
		color: var(--heading);
	}
	.dialog-foot {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
	}
	input,
	select {
		padding: 0.4rem 0.55rem;
		border: 1px solid var(--border);
		border-radius: 7px;
		background: var(--input-bg);
		color: var(--heading);
		font-size: 0.85rem;
	}
	input:disabled {
		opacity: 0.6;
	}
</style>
