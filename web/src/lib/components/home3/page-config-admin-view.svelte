<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { locales } from '$lib/paraglide/runtime';
	import {
		listPages,
		listEntries,
		upsertEntry,
		deleteEntry,
		type PageDef,
		type AdminEntry
	} from '$lib/services/pageConfigService';
	import { listManagedRoles, type ManagedRole } from '$lib/services/userManagementService';

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

	// Canonical access roles (same source as User Management). Used by the inline
	// ACCESS_ROLE chip editor. Fails soft: an empty list just means "no add menu".
	let roleOptions = $state<ManagedRole[]>([]);

	// Inline-edit state (quick edits straight from the table; the modal remains
	// for creating entries and full edits).
	let savingKey = $state<string | null>(null); // entry_key mid-save (for subtle UI)
	let editingRolesKey = $state<string | null>(null); // row whose roles cell is open
	let roleMenuOpenKey = $state<string | null>(null); // row whose "add role" menu is open
	let editingLabel = $state<{ key: string; lang: string } | null>(null);
	let labelDraft = $state('');

	function roleLabel(key: string): string {
		return roleOptions.find((r) => r.key === key)?.label ?? key;
	}
	function availableRolesFor(entry: AdminEntry): ManagedRole[] {
		const have = new Set(entry.access_role ?? []);
		return roleOptions.filter((r) => !have.has(r.key));
	}

	// Persist a patched entry with an optimistic update; revert via reload on error.
	async function saveEntryPatch(entry: AdminEntry, patch: Partial<AdminEntry>) {
		const updated: AdminEntry = { ...entry, ...patch };
		entries = entries.map((e) => (e.entry_key === entry.entry_key ? updated : e));
		savingKey = entry.entry_key;
		loadError = null;
		try {
			await upsertEntry(selectedPageKey, updated);
		} catch (e) {
			loadError = String(e);
			await loadEntries(); // revert to server truth
		} finally {
			savingKey = null;
		}
	}

	// ── Status column (dropdown, auto-save) ───────────────────────────────────
	// The stored state is two booleans; the dropdown surfaces the three mutually
	// exclusive states. "no roles (hidden)" is orthogonal (shown by the roles cell).
	function statusValue(entry: AdminEntry): 'active' | 'disabled' | 'suspended' {
		if (!entry.enabled) return 'disabled';
		if (!entry.accessible) return 'suspended';
		return 'active';
	}
	function onStatusChange(entry: AdminEntry, value: string) {
		const patch =
			value === 'disabled'
				? { enabled: false, accessible: true }
				: value === 'suspended'
					? { enabled: true, accessible: false }
					: { enabled: true, accessible: true };
		saveEntryPatch(entry, patch);
	}

	// ── ACCESS_ROLE column (chip editor, auto-save) ───────────────────────────
	function startRolesEdit(entry: AdminEntry) {
		editingRolesKey = entry.entry_key;
		roleMenuOpenKey = null;
	}
	function stopRolesEdit() {
		editingRolesKey = null;
		roleMenuOpenKey = null;
	}
	function addRoleTo(entry: AdminEntry, key: string) {
		if ((entry.access_role ?? []).includes(key)) return;
		saveEntryPatch(entry, { access_role: [...(entry.access_role ?? []), key] });
		roleMenuOpenKey = null;
	}
	function removeRoleFrom(entry: AdminEntry, key: string) {
		saveEntryPatch(entry, { access_role: (entry.access_role ?? []).filter((r) => r !== key) });
	}

	// ── Label columns (inline edit, auto-save) ────────────────────────────────
	function startLabelEdit(entry: AdminEntry, lang: string) {
		editingLabel = { key: entry.entry_key, lang };
		labelDraft = entry.content?.[lang]?.label ?? '';
	}
	function commitLabelEdit(entry: AdminEntry, lang: string) {
		if (!editingLabel || editingLabel.key !== entry.entry_key || editingLabel.lang !== lang) return;
		const value = labelDraft.trim();
		const prev = entry.content?.[lang]?.label ?? '';
		editingLabel = null;
		if (value === prev) return;
		const langContent = { ...(entry.content?.[lang] ?? {}) };
		if (value) langContent.label = value;
		else delete langContent.label; // empty → revert to built-in default
		saveEntryPatch(entry, { content: { ...entry.content, [lang]: langContent } });
	}
	function cancelLabelEdit() {
		editingLabel = null;
	}

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

	// ── Live inspect (page-config-live-inspect) ───────────────────────────────
	// The right pane embeds a same-origin <iframe> preview of the selected page.
	// Any element on a configurable page whose text comes from getPageConfig
	// carries data-entry-key="<entry_key>" — that single attribute is the whole
	// page-side contract, so onboarding a new page needs NO change here. All the
	// hover/highlight logic lives in this component and drives the preview
	// through iframe.contentDocument (same-origin only; degrades silently
	// otherwise). See openspec/changes/page-config-live-inspect.
	let previewEl = $state<HTMLIFrameElement | null>(null);
	let previewOpen = $state(true);
	// entry_key currently hovered inside the preview → highlights its config row.
	let hoveredKey = $state<string | null>(null);

	// Draggable divider: left pane width as a % of the split container.
	let splitEl = $state<HTMLDivElement | null>(null);
	let leftPct = $state(50);
	let dragging = $state(false);

	function startDrag(e: PointerEvent) {
		dragging = true;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		e.preventDefault();
	}
	function onDrag(e: PointerEvent) {
		if (!dragging || !splitEl) return;
		const rect = splitEl.getBoundingClientRect();
		const pct = ((e.clientX - rect.left) / rect.width) * 100;
		leftPct = Math.min(80, Math.max(20, pct));
	}
	function endDrag(e: PointerEvent) {
		if (!dragging) return;
		dragging = false;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {
			/* pointer already released */
		}
	}

	// Whether the preview document ever became readable (same-origin + loaded).
	// Refused frames (auth 401, frame headers) leave this false → show a hint.
	let previewReadable = $state(false);

	const INSPECT_STYLE_ID = 'kb-inspect-style';
	const INSPECT_CLASS = 'kb-inspect-hl';
	const INSPECT_CSS = `.${INSPECT_CLASS}{outline:2px solid #6366F1;outline-offset:2px;border-radius:8px;background:rgba(99,102,241,0.14);transition:outline-color .12s,background .12s;}`;

	// Teardown for whatever we attached to the current preview document.
	let detachInspect: (() => void) | null = null;

	// Same-origin guard: reading contentDocument throws cross-origin, and is
	// null before load. Return null (disable highlighting) instead of throwing.
	function previewDoc(): Document | null {
		try {
			return previewEl?.contentDocument ?? null;
		} catch {
			return null;
		}
	}

	// Attribute-only contract: find the element a config row points at.
	function previewElementFor(entryKey: string): HTMLElement | null {
		const doc = previewDoc();
		if (!doc) return null;
		return doc.querySelector<HTMLElement>(
			`[data-entry-key="${(window.CSS?.escape ?? ((s: string) => s))(entryKey)}"]`
		);
	}

	// req 2: hovering a config row highlights + reveals the matching element.
	function highlightInPreview(entryKey: string) {
		previewElementFor(entryKey)?.classList.add(INSPECT_CLASS);
		previewElementFor(entryKey)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
	}
	function clearPreviewHighlight(entryKey: string) {
		previewElementFor(entryKey)?.classList.remove(INSPECT_CLASS);
	}

	// (Re)wire the preview on every load. Delegation on the document survives the
	// previewed page's internal SvelteKit re-renders; a hard navigation re-fires
	// load and we re-inject. Always tear down first to avoid double-binding.
	function onPreviewLoad() {
		detachInspect?.();
		detachInspect = null;
		hoveredKey = null;

		const doc = previewDoc();
		if (!doc) {
			previewReadable = false; // refused frame (auth/frame-header) or cross-origin
			return;
		}
		// A refused frame (X-Frame-Options, or a 401 that renders nothing) leaves an
		// empty document; require real body content so the fallback hint shows.
		previewReadable = !!doc.body && doc.body.children.length > 0;

		const style = doc.createElement('style');
		style.id = INSPECT_STYLE_ID;
		style.textContent = INSPECT_CSS;
		doc.head?.appendChild(style);

		// req 3 (primary): hovering a keyed element highlights its config row.
		const onOver = (e: Event) => {
			const el = (e.target as HTMLElement | null)?.closest<HTMLElement>('[data-entry-key]');
			hoveredKey = el?.dataset.entryKey ?? null;
		};
		const onOut = (e: Event) => {
			const to = (e as MouseEvent).relatedTarget as HTMLElement | null;
			if (!to?.closest?.('[data-entry-key]')) hoveredKey = null;
		};
		doc.addEventListener('mouseover', onOver);
		doc.addEventListener('mouseout', onOut);

		detachInspect = () => {
			doc.removeEventListener('mouseover', onOver);
			doc.removeEventListener('mouseout', onOut);
			doc.getElementById(INSPECT_STYLE_ID)?.remove();
		};
	}

	onDestroy(() => {
		detachInspect?.();
		detachInspect = null;
	});

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

	onMount(() => {
		loadPages();
		listManagedRoles()
			.then((r) => (roleOptions = r))
			.catch(() => (roleOptions = [])); // roles are optional for the editor
	});

	// Reload the preview iframe (same-origin reload; falls back to re-setting src).
	function refreshPreview() {
		if (!previewEl || !selectedPage) return;
		try {
			previewEl.contentWindow?.location.reload();
		} catch {
			previewEl.src = selectedPage.route;
		}
	}

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
		<button
			type="button"
			class="toggle"
			aria-pressed={previewOpen}
			onclick={() => (previewOpen = !previewOpen)}
		>
			{previewOpen ? 'Hide preview' : 'Show preview'}
		</button>
	</div>

	<div class="split" class:preview-open={previewOpen} class:dragging bind:this={splitEl}>
		<div class="left-pane" style:flex={previewOpen ? `0 0 ${leftPct}%` : '1 1 auto'}>
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
						<tr
							class:inactive={status(entry) !== 'active'}
							class:row-hl={hoveredKey === entry.entry_key}
							onmouseenter={() => highlightInPreview(entry.entry_key)}
							onmouseleave={() => clearPreviewHighlight(entry.entry_key)}
						>
							<td><code>{entry.entry_key}</code></td>
							<td>{entry.entry_desc || '—'}</td>
							<td>
								<select
									class="status-select"
									value={statusValue(entry)}
									disabled={savingKey === entry.entry_key}
									onchange={(e) => onStatusChange(entry, (e.currentTarget as HTMLSelectElement).value)}
								>
									<option value="active">active</option>
									<option value="disabled">disabled</option>
									<option value="suspended">suspended</option>
								</select>
								{#if (entry.access_role ?? []).length === 0}
									<span class="hint-noroles">hidden: no roles</span>
								{/if}
							</td>
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<td
								class="roles-cell"
								ondblclick={() => startRolesEdit(entry)}
								title="Double-click to edit roles"
							>
								{#if editingRolesKey === entry.entry_key}
									<div class="role-chip-list">
										{#each entry.access_role ?? [] as roleKey (roleKey)}
											<button
												type="button"
												class="role-chip"
												onclick={() => removeRoleFrom(entry, roleKey)}
											>
												{roleLabel(roleKey)}<span aria-hidden="true">×</span>
											</button>
										{/each}
										{#if (entry.access_role ?? []).length === 0}
											<span class="role-placeholder">No roles</span>
										{/if}
									</div>
									<div class="role-menu-wrap">
										<button
											type="button"
											class="mini"
											onclick={() =>
												(roleMenuOpenKey = roleMenuOpenKey === entry.entry_key ? null : entry.entry_key)}
										>
											{roleMenuOpenKey === entry.entry_key ? 'Close' : '+ Add role'}
										</button>
										<button type="button" class="mini" onclick={stopRolesEdit}>Done</button>
										{#if roleMenuOpenKey === entry.entry_key}
											<div class="role-menu">
												{#each availableRolesFor(entry) as role (role.key)}
													<button
														type="button"
														class="role-menu-item"
														onclick={() => addRoleTo(entry, role.key)}
													>
														<strong>{role.label}</strong><span>{role.key}</span>
													</button>
												{:else}
													<div class="role-menu-empty">No additional roles.</div>
												{/each}
											</div>
										{/if}
									</div>
								{:else if (entry.access_role ?? []).length > 0}
									<div class="role-chip-list readonly">
										{#each entry.access_role ?? [] as roleKey (roleKey)}
											<span class="role-chip">{roleLabel(roleKey)}</span>
										{/each}
									</div>
								{:else}
									<span class="muted">—</span>
								{/if}
							</td>
							{#each locales as lang (lang)}
								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<td class="label-cell" ondblclick={() => startLabelEdit(entry, lang)} title="Double-click to edit">
									{#if editingLabel?.key === entry.entry_key && editingLabel?.lang === lang}
										<!-- svelte-ignore a11y_autofocus -->
										<input
											class="label-input"
											bind:value={labelDraft}
											autofocus
											onblur={() => commitLabelEdit(entry, lang)}
											onkeydown={(e) => {
												if (e.key === 'Enter') commitLabelEdit(entry, lang);
												else if (e.key === 'Escape') cancelLabelEdit();
											}}
										/>
									{:else}
										{entry.content?.[lang]?.label ?? '—'}
									{/if}
								</td>
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

		{#if previewOpen}
			<!-- Draggable divider between the entry list and the preview. -->
			<div
				class="divider"
				role="separator"
				aria-orientation="vertical"
				aria-label="Resize preview"
				onpointerdown={startDrag}
				onpointermove={onDrag}
				onpointerup={endDrag}
				onpointercancel={endDrag}
			>
				<span class="grip"></span>
			</div>

			<div class="right-pane" style:flex={`1 1 ${100 - leftPct}%`}>
				{#if selectedPage}
					<div class="preview-bar">
						<code>{selectedPage.route}</code>
						<div class="preview-bar-actions">
							<button type="button" class="mini" onclick={refreshPreview}>↻ Refresh</button>
							<a href={selectedPage.route} target="_blank" rel="noopener" class="open-link">
								Open ↗
							</a>
						</div>
					</div>
					<div class="preview-body">
						<iframe
							bind:this={previewEl}
							title="Live preview of {selectedPage.route}"
							src={selectedPage.route}
							onload={onPreviewLoad}
						></iframe>
						{#if !previewReadable}
							<div class="preview-hint">
								<p>Preview couldn't be displayed inline.</p>
								<p class="muted">
									The page likely refused to load in a frame (auth or frame policy). Use
									<a href={selectedPage.route} target="_blank" rel="noopener">Open ↗</a> to view it in a
									new tab. Highlighting works only when the preview renders inline.
								</p>
							</div>
						{/if}
					</div>
				{:else}
					<p class="muted preview-empty">Select a page to preview.</p>
				{/if}
			</div>
		{/if}
	</div>
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
		flex: 1;
		min-height: 0;
		overflow: auto;
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	table {
		width: 100%;
		border-collapse: separate;
		border-spacing: 0;
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
		/* Item 1: header stays put while the list body scrolls. */
		position: sticky;
		top: 0;
		z-index: 3;
		background: var(--card);
	}
	tbody tr:hover {
		background: var(--row-hover);
	}
	tbody tr.row-hl {
		background: var(--row-hover);
		box-shadow: inset 3px 0 0 var(--btn);
	}
	tr.inactive {
		opacity: 0.55;
	}

	/* Split layout: config table (left) + live preview iframe (right). */
	.split {
		display: flex;
		gap: 1rem;
		align-items: stretch;
	}
	.split {
		height: calc(100vh - 240px);
		min-height: 480px;
	}
	.left-pane {
		flex: 1 1 0;
		min-width: 0;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.right-pane {
		flex: 1 1 50%;
		min-width: 0;
		display: flex;
		flex-direction: column;
		border: 1px solid var(--border);
		border-radius: 10px;
		overflow: hidden;
		background: var(--card);
	}
	.preview-bar {
		flex: 0 0 auto;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.4rem 0.6rem;
		border-bottom: 1px solid var(--border);
		background: var(--input-bg);
	}
	.open-link {
		color: var(--btn);
		text-decoration: none;
		font-size: 0.8rem;
		white-space: nowrap;
	}
	.open-link:hover {
		text-decoration: underline;
	}
	.preview-body {
		position: relative;
		flex: 1;
		min-height: 0;
	}
	.preview-body iframe {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		border: 0;
		background: #fff;
	}
	.preview-hint {
		position: absolute;
		inset: 0;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		text-align: center;
		padding: 1.5rem;
		background: var(--card);
		color: var(--heading);
	}
	.preview-hint .muted {
		max-width: 42ch;
		font-size: 0.82rem;
	}
	.preview-empty {
		margin: auto;
	}

	/* Draggable divider between list and preview. */
	.divider {
		flex: 0 0 10px;
		align-self: stretch;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: col-resize;
		touch-action: none;
	}
	.divider .grip {
		width: 2px;
		height: 100%;
		background: var(--border);
		border-radius: 2px;
		transition: background 0.12s;
	}
	.divider:hover .grip,
	.split.dragging .divider .grip {
		background: var(--btn);
		width: 3px;
	}
	.split.dragging {
		cursor: col-resize;
		user-select: none;
	}
	.split.dragging .preview-body iframe {
		pointer-events: none;
	}

	/* Preview toolbar actions (Refresh + Open). */
	.preview-bar-actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	button.mini {
		padding: 0.2rem 0.55rem;
		font-size: 0.78rem;
		border-radius: 6px;
		border: 1px solid var(--border);
		background: var(--card);
		color: var(--heading);
		cursor: pointer;
	}
	button.mini:hover {
		border-color: var(--btn);
	}

	/* Status dropdown + no-roles hint. */
	.status-select {
		padding: 0.2rem 0.4rem;
		font-size: 0.82rem;
	}
	.hint-noroles {
		display: block;
		margin-top: 2px;
		font-size: 0.68rem;
		color: var(--danger);
	}

	/* Access-role chip editor. */
	.roles-cell {
		white-space: normal;
		min-width: 180px;
		cursor: default;
	}
	.role-chip-list {
		display: flex;
		flex-wrap: wrap;
		gap: 5px;
		align-items: center;
	}
	.role-chip-list.readonly {
		cursor: pointer;
	}
	.role-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 2px 8px;
		background: var(--card);
		color: var(--heading);
		font-size: 0.75rem;
		cursor: pointer;
	}
	span.role-chip {
		cursor: pointer;
	}
	.role-chip span {
		color: var(--sub);
		font-weight: 700;
	}
	.role-placeholder {
		color: var(--sub);
		font-size: 0.75rem;
	}
	.role-menu-wrap {
		position: relative;
		display: flex;
		gap: 0.4rem;
		margin-top: 6px;
	}
	.role-menu {
		position: absolute;
		top: calc(100% + 6px);
		left: 0;
		z-index: 6;
		width: min(260px, 60vw);
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 12px;
		box-shadow: 0 18px 40px rgba(0, 0, 0, 0.28);
		padding: 6px;
		max-height: 240px;
		overflow: auto;
	}
	.role-menu-item {
		width: 100%;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 1px;
		border: none;
		background: transparent;
		color: var(--heading);
		padding: 7px 10px;
		border-radius: 8px;
		cursor: pointer;
	}
	.role-menu-item:hover {
		background: var(--row-hover);
	}
	.role-menu-item span {
		color: var(--sub);
		font-size: 0.72rem;
	}
	.role-menu-empty {
		padding: 10px;
		color: var(--sub);
		font-size: 0.75rem;
	}

	/* Inline label editing. */
	.label-cell {
		cursor: text;
		min-width: 90px;
	}
	.label-input {
		width: 100%;
		min-width: 80px;
		padding: 0.2rem 0.4rem;
		font-size: 0.82rem;
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
