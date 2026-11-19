<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import TagIcon from '@lucide/svelte/icons/tag';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import FileDownIcon from '@lucide/svelte/icons/file-down';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import {
		addItem,
		createLine,
		deleteItem,
		getNotesText,
		getReleases,
		notesPdfUrl,
		previousBuild,
		updateItem,
		type ItemInput,
		type ItemType,
		type Release,
		type ReleaseItem,
		type ReleasesState
	} from './releases-client';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	const STATE_KEY = 'releases-view-state';
	const ITEM_TYPES: ItemType[] = ['new feature', 'improvement', 'bug fix'];
	const emptyItem = (): ItemInput => ({ item_type: 'bug fix', component: 'ChenWeb', description: '', issue: '', pull_request: '', notes: '' });

	let data = $state<ReleasesState | null>(null);
	let loadError = $state('');
	let loading = $state(true);
	// Open "new major / new minor" panel and its description.
	let linePanel = $state<'major' | 'minor' | null>(null);
	let lineDescription = $state('');
	let lineError = $state('');
	let lineBusy = $state(false);
	// Item form: null = closed, 0 = new item, otherwise the id being edited.
	let editingItem = $state<number | null>(null);
	let itemDraft = $state<ItemInput>(emptyItem());
	let itemError = $state('');
	let itemBusy = $state(false);
	let confirmDeleteId = $state<number | null>(null);
	let expanded = $state<Record<string, boolean>>({});
	let notesShown = $state<Record<string, string>>({});
	let notesError = $state<Record<string, string>>({});

	const colors = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#FFFFFF',
		raised: darkMode ? '#252A3A' : '#F8FAFC',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#64748B',
		border: darkMode ? '#2D3348' : '#DCE2EA',
		accent: darkMode ? '#67E8C2' : '#087F68',
		accentTint: darkMode ? 'rgba(103,232,194,.12)' : 'rgba(8,127,104,.09)',
		danger: darkMode ? '#F87171' : '#B42318',
		dangerTint: darkMode ? 'rgba(248,113,113,.12)' : 'rgba(180,35,24,.07)',
		warning: darkMode ? '#FBBF24' : '#A16207',
		warningTint: darkMode ? 'rgba(251,191,36,.12)' : 'rgba(161,98,7,.08)'
	});
	const cssVars = $derived(
		Object.entries(colors)
			.map(([key, value]) => `--rel-${key}:${value}`)
			.join(';')
	);
	const lang = $derived(getLocale() === 'zh-cn' ? 'zh-cn' : 'en');
	const locale = $derived(getLocale() === 'zh-cn' ? 'zh-CN' : 'en-US');
	const editable = $derived(!!data?.editable);
	const lineVersion = $derived(data && linePanel ? nextLine(data.current, linePanel) : '');

	function nextLine(current: string, kind: 'major' | 'minor'): string {
		const [major, minor] = current.split('.').map(Number);
		const pad = (n: number) => String(n).padStart(2, '0');
		return kind === 'major' ? `${pad(major + 1)}.00.00` : `${pad(major)}.${pad(minor + 1)}.00`;
	}

	function typeLabel(t: string): string {
		return ({ 'new feature': m.releases_type_new_feature, improvement: m.releases_type_improvement, 'bug fix': m.releases_type_bug_fix }[t] ?? (() => t))();
	}

	function kindLabel(r: Release): string {
		return r.kind === 'major' ? m.releases_kind_major() : r.kind === 'minor' ? m.releases_kind_minor() : m.releases_kind_build();
	}

	function issueLabel(it: ReleaseItem): string {
		if (!it.issue) return '';
		return it.component === 'shared' ? `shared#${it.issue}` : `#${it.issue}`;
	}

	// A commit id shortened for display; * marks uncommitted changes in the build.
	function shortRev(rev: string, dirty: boolean): string {
		return rev ? rev.slice(0, 10) + (dirty ? '*' : '') : '—';
	}

	function errorText(err: unknown, fallback: string): string {
		return err instanceof Error && err.message ? err.message : fallback;
	}

	function formatTime(value?: string): string {
		if (!value) return '';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
	}

	function saveState() {
		if (typeof sessionStorage === 'undefined') return;
		try {
			sessionStorage.setItem(
				STATE_KEY,
				JSON.stringify({ linePanel, lineDescription, editingItem, itemDraft, expanded, scroll: pageEl?.scrollTop ?? 0 })
			);
		} catch {
			// storage unavailable: state is simply not kept
		}
	}

	function restoreState() {
		if (typeof sessionStorage === 'undefined') return;
		try {
			const saved = JSON.parse(sessionStorage.getItem(STATE_KEY) || 'null');
			if (!saved) return;
			linePanel = saved.linePanel ?? null;
			lineDescription = saved.lineDescription ?? '';
			editingItem = saved.editingItem ?? null;
			itemDraft = { ...emptyItem(), ...(saved.itemDraft ?? {}) };
			expanded = saved.expanded ?? {};
			restoreScroll = saved.scroll ?? 0;
		} catch {
			sessionStorage.removeItem(STATE_KEY);
		}
	}

	let pageEl = $state<HTMLElement>();
	let restoreScroll = 0;
	// Restore before the save effect below first runs, or it would overwrite the saved state.
	restoreState();

	async function load() {
		loading = true;
		loadError = '';
		try {
			data = await getReleases();
		} catch (err) {
			loadError = errorText(err, m.releases_load_error());
		} finally {
			loading = false;
		}
	}

	function openLine(kind: 'major' | 'minor') {
		linePanel = linePanel === kind ? null : kind;
		lineDescription = '';
		lineError = '';
	}

	async function submitLine() {
		if (!linePanel) return;
		lineBusy = true;
		lineError = '';
		try {
			await createLine(linePanel, lineDescription.trim());
			linePanel = null;
			lineDescription = '';
			await load();
		} catch (err) {
			lineError = errorText(err, m.releases_save_error());
		} finally {
			lineBusy = false;
		}
	}

	function startItem(item?: ReleaseItem) {
		itemError = '';
		confirmDeleteId = null;
		if (item) {
			editingItem = item.id;
			itemDraft = { item_type: item.item_type, component: item.component, description: item.description, issue: item.issue, pull_request: item.pull_request, notes: item.notes };
		} else {
			editingItem = 0;
			itemDraft = emptyItem();
		}
	}

	function closeItem() {
		editingItem = null;
		itemDraft = emptyItem();
		itemError = '';
	}

	async function submitItem() {
		if (!itemDraft.description.trim()) {
			itemError = m.releases_item_description_required();
			return;
		}
		if (itemDraft.issue.trim() && !/^#?\d+$/.test(itemDraft.issue.trim())) {
			itemError = m.releases_item_issue_invalid();
			return;
		}
		itemBusy = true;
		itemError = '';
		try {
			if (editingItem) await updateItem(editingItem, itemDraft);
			else await addItem(itemDraft);
			closeItem();
			await load();
		} catch (err) {
			itemError = errorText(err, m.releases_save_error());
		} finally {
			itemBusy = false;
		}
	}

	async function removeItem(id: number) {
		itemError = '';
		try {
			await deleteItem(id);
			confirmDeleteId = null;
			if (editingItem === id) closeItem();
			await load();
		} catch (err) {
			itemError = errorText(err, m.releases_save_error());
		}
	}

	async function toggleNotes(r: Release) {
		if (notesShown[r.version] !== undefined) {
			const { [r.version]: _, ...rest } = notesShown;
			notesShown = rest;
			return;
		}
		try {
			const text = await getNotesText(previousBuild(data?.releases ?? [], r.version), r.version, lang);
			notesShown = { ...notesShown, [r.version]: text };
			notesError = { ...notesError, [r.version]: '' };
		} catch (err) {
			notesError = { ...notesError, [r.version]: errorText(err, m.releases_notes_error()) };
		}
	}

	function pdfUrl(r: Release): string {
		return notesPdfUrl(previousBuild(data?.releases ?? [], r.version), r.version, lang);
	}

	$effect(() => {
		// Keep page state when the reader leaves through the menu and comes back.
		void [linePanel, lineDescription, editingItem, itemDraft.item_type, itemDraft.component, itemDraft.description,
			itemDraft.issue, itemDraft.pull_request, itemDraft.notes, expanded];
		saveState();
	});

	onMount(() => {
		void load().then(() => {
			if (restoreScroll && pageEl) requestAnimationFrame(() => pageEl && (pageEl.scrollTop = restoreScroll));
		});
	});
</script>

{#snippet itemList(items: ReleaseItem[], actions: boolean)}
	<ul class="items">
		{#each items as it (it.id)}
			<li class="item">
				<span class="pill type-{it.item_type.replace(' ', '-')}">{typeLabel(it.item_type)}</span>
				{#if it.component === 'shared'}<span class="pill">{m.releases_component_shared()}</span>{/if}
				<div class="item-text">
					<span>{it.description}{#if it.issue}<span class="mono accent issue">({issueLabel(it)})</span>{/if}</span>
					{#if it.notes}<small>{it.notes}</small>{/if}
					{#if it.pull_request}<small class="mono">{m.releases_item_pr({ value: it.pull_request })}</small>{/if}
				</div>
				{#if actions}
					<div class="item-actions">
						{#if confirmDeleteId === it.id}
							<span class="muted">{m.releases_item_delete_confirm()}</span>
							<button type="button" class="ghost small danger-text" onclick={() => removeItem(it.id)}>{m.releases_yes()}</button>
							<button type="button" class="ghost small" onclick={() => (confirmDeleteId = null)}>{m.releases_no()}</button>
						{:else}
							<button type="button" class="ghost small" onclick={() => startItem(it)}>{m.releases_edit()}</button>
							<button type="button" class="ghost small" onclick={() => (confirmDeleteId = it.id)}>{m.releases_delete()}</button>
						{/if}
					</div>
				{/if}
			</li>
			{#if actions && editingItem === it.id}<li>{@render itemForm()}</li>{/if}
		{:else}
			<li class="empty-row">{m.releases_no_items()}</li>
		{/each}
	</ul>
{/snippet}

{#snippet itemForm()}
	<form class="item-form" onsubmit={(e) => { e.preventDefault(); void submitItem(); }}>
		<div class="form-grid">
			<label class="field">
				<span>{m.releases_item_type()}</span>
				<select bind:value={itemDraft.item_type}>
					{#each ITEM_TYPES as t (t)}<option value={t}>{typeLabel(t)}</option>{/each}
				</select>
			</label>
			<label class="field">
				<span>{m.releases_item_component()}</span>
				<select bind:value={itemDraft.component}>
					<option value="ChenWeb">{m.releases_component_chenweb()}</option>
					<option value="shared">{m.releases_component_shared()}</option>
				</select>
			</label>
			<label class="field">
				<span>{m.releases_item_issue()}</span>
				<input type="text" bind:value={itemDraft.issue} placeholder={m.releases_item_issue_placeholder()} inputmode="numeric" />
			</label>
			<label class="field">
				<span>{m.releases_item_pull_request()}</span>
				<input type="text" bind:value={itemDraft.pull_request} />
			</label>
		</div>
		<label class="field">
			<span>{m.releases_item_description()}</span>
			<input type="text" bind:value={itemDraft.description} placeholder={m.releases_item_description_placeholder()} maxlength="500" />
		</label>
		<label class="field">
			<span>{m.releases_item_notes()}</span>
			<textarea rows="2" bind:value={itemDraft.notes}></textarea>
		</label>
		{#if itemError}<p class="form-error" role="alert">{itemError}</p>{/if}
		<div class="form-actions">
			<button type="button" class="ghost" onclick={closeItem}>{m.releases_cancel()}</button>
			<button type="submit" class="primary" disabled={itemBusy}>
				{#if itemBusy}<LoaderCircleIcon size={14} class="rel-spin" />{/if}
				{editingItem ? m.releases_item_save() : m.releases_item_add()}
			</button>
		</div>
	</form>
{/snippet}

<div class="rel-page select-text" style={cssVars} bind:this={pageEl} onscroll={saveState}>
	<header class="page-head">
		<div class="title-lockup">
			<div class="title-icon" aria-hidden="true"><TagIcon size={19} /></div>
			<div>
				<p class="eyebrow">{m.nav_sysadmin_system()}</p>
				<h1>{m.releases_title()}</h1>
				<p class="lede">{editable ? m.releases_lede_build() : m.releases_lede_reference()}</p>
			</div>
		</div>
		<button type="button" class="ghost" onclick={load} disabled={loading}>
			<RefreshCwIcon size={15} />
			{m.releases_refresh()}
		</button>
	</header>

	{#if loadError}
		<div class="banner danger" role="alert"><TriangleAlertIcon size={17} /><div><strong>{m.releases_load_error()}</strong><code>{loadError}</code></div></div>
	{/if}

	{#if data}
		<section class="card">
			<div class="card-head">
				<div class="versions">
					<span class="pill project">{data.project}</span>
					{#if editable}
						<div><span class="label">{m.releases_current()}</span><strong class="mono big">{data.current}</strong></div>
						<div><span class="label">{m.releases_next_build()}</span><strong class="mono big accent">{data.next_build || '—'}</strong></div>
					{:else if data.current_build_number || data.current !== '00.00.00'}
						<div>
							<span class="label">{m.releases_running()}</span>
							<strong class="mono big accent">{data.current}</strong>
							{#if data.current_build_number}<small class="mono muted">{m.releases_build_number({ number: data.current_build_number })}</small>{/if}
						</div>
					{:else}
						<div><span class="label">{m.releases_running()}</span><span class="muted">{m.releases_running_unknown()}</span></div>
					{/if}
				</div>
				{#if editable}
					<div class="line-buttons">
						<button type="button" class="ghost" class:active={linePanel === 'major'} onclick={() => openLine('major')} disabled={!!data.building}>{m.releases_new_major()}</button>
						<button type="button" class="ghost" class:active={linePanel === 'minor'} onclick={() => openLine('minor')} disabled={!!data.building}>{m.releases_new_minor()}</button>
					</div>
				{/if}
			</div>
			{#if editable && data.revisions_left < 10}
				<div class="banner warning inset" role="status">
					<TriangleAlertIcon size={16} />
					<span>{data.revisions_left === 0 ? m.releases_revisions_none() : m.releases_revisions_left({ count: data.revisions_left })}</span>
				</div>
			{/if}
			{#if !editable}
				<p class="note">{m.releases_read_only_note()}</p>
			{/if}
			{#if linePanel}
				<form class="line-panel" onsubmit={(e) => { e.preventDefault(); void submitLine(); }}>
					<p>{linePanel === 'major' ? m.releases_new_major_body({ version: lineVersion }) : m.releases_new_minor_body({ version: lineVersion })}</p>
					<label class="field">
						<span>{m.releases_line_description()}</span>
						<textarea rows="2" bind:value={lineDescription} placeholder={m.releases_line_description_placeholder()}></textarea>
					</label>
					{#if lineError}<p class="form-error" role="alert">{lineError}</p>{/if}
					<div class="form-actions">
						<button type="button" class="ghost" onclick={() => (linePanel = null)}>{m.releases_cancel()}</button>
						<button type="submit" class="primary" disabled={lineBusy}>
							{#if lineBusy}<LoaderCircleIcon size={14} class="rel-spin" />{/if}
							{m.releases_create_version({ version: lineVersion })}
						</button>
					</div>
				</form>
			{/if}
		</section>

		{#if data.building}
			<section class="card">
				<div class="card-head">
					<div>
						<p class="eyebrow">{m.releases_building_title({ version: data.building.version })}</p>
						<p class="busy-note">{m.releases_building_since({ time: formatTime(data.building.created_at) })}</p>
					</div>
					<LoaderCircleIcon size={17} class="rel-spin accent" />
				</div>
				{@render itemList(data.building.items, false)}
			</section>
		{/if}

		{#if editable}
			<section class="card">
				<div class="card-head">
					<div>
						<p class="eyebrow">{m.releases_pending_title({ count: data.pending.length })}</p>
						<p class="busy-note">{m.releases_pending_note({ version: data.next_build || '—' })}</p>
					</div>
					<button type="button" class="primary" onclick={() => startItem()} disabled={editingItem === 0}>
						<PlusIcon size={15} />{m.releases_item_add()}
					</button>
				</div>
				{#if editingItem === 0}{@render itemForm()}{/if}
				{@render itemList(data.pending, true)}
			</section>
		{/if}

		<section class="card">
			<div class="card-head"><p class="eyebrow">{m.releases_history()}</p></div>
			<ul class="history">
				{#each data.releases as r (r.version)}
					{@const open = r.kind === 'build' && !!expanded[r.version]}
					<li class="release kind-{r.kind}">
						{#if r.kind === 'build'}
							<button type="button" class="release-row" aria-expanded={open} onclick={() => (expanded = { ...expanded, [r.version]: !open })}>
								<span class="chevron" class:open aria-hidden="true"><ChevronRightIcon size={15} /></span>
								<strong class="mono">{r.version}</strong>
								<span class="pill">{kindLabel(r)}</span>
								<span class="desc">{r.description}</span>
								<span class="muted small-text">{m.releases_item_count({ count: r.items.length })}</span>
								<span class="muted small-text">{r.release_date}</span>
							</button>
						{:else}
							<div class="release-row">
								<span class="chevron" aria-hidden="true"></span>
								<strong class="mono">{r.version}</strong>
								<span class="pill">{kindLabel(r)}</span>
								<span class="desc">{r.description}</span>
								<span class="muted small-text">{r.release_date}</span>
							</div>
						{/if}
						{#if open}
							<div class="release-body">
								{#if r.build_number || r.git_rev}
									<p class="meta mono muted">
										{#if r.build_number}{m.releases_build_number({ number: r.build_number })}{/if}
										{#if r.git_rev} · {m.releases_commits({ chenweb: shortRev(r.git_rev, r.git_dirty), shared: shortRev(r.shared_git_rev, r.shared_git_dirty) })}{/if}
									</p>
								{/if}
								{@render itemList(r.items, false)}
								<div class="notes-actions">
									<button type="button" class="ghost small" onclick={() => toggleNotes(r)}>
										<FileTextIcon size={14} />{notesShown[r.version] !== undefined ? m.releases_notes_hide() : m.releases_notes_text()}
									</button>
									<a class="ghost small" href={pdfUrl(r)} target="_blank" rel="noopener"><FileDownIcon size={14} />{m.releases_notes_pdf()}</a>
								</div>
								{#if notesError[r.version]}<p class="form-error" role="alert">{notesError[r.version]}</p>{/if}
								{#if notesShown[r.version] !== undefined}<pre class="notes">{notesShown[r.version]}</pre>{/if}
							</div>
						{/if}
					</li>
				{:else}
					<li class="empty-row">{m.releases_no_releases()}</li>
				{/each}
			</ul>
		</section>
	{:else if loading}
		<p class="muted"><LoaderCircleIcon size={15} class="rel-spin" /> {m.releases_loading()}</p>
	{/if}
</div>

<style>
	.rel-page{height:100%;overflow:auto;padding:clamp(16px,3vw,34px);background:var(--rel-bg);color:var(--rel-text);font-family:'Inter Tight',system-ui,sans-serif;user-select:text;-webkit-user-select:text}
	.page-head,.card,.banner{max-width:1200px;margin-left:auto;margin-right:auto}
	.page-head{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:22px}
	.title-lockup{display:flex;align-items:center;gap:14px;min-width:0}
	.title-icon{display:grid;place-items:center;width:44px;height:44px;flex:none;border:1px solid color-mix(in srgb,var(--rel-accent) 32%,transparent);border-radius:13px;background:var(--rel-accentTint);color:var(--rel-accent)}
	.eyebrow{margin:0;color:var(--rel-accent);font-size:10px;font-weight:750;letter-spacing:.14em;text-transform:uppercase}
	h1{margin:4px 0 0;font-size:clamp(22px,2.5vw,30px);font-weight:680;letter-spacing:-.035em}
	.lede{margin:5px 0 0;color:var(--rel-muted);font-size:13px}
	.mono{font-family:'SFMono-Regular',Consolas,monospace}
	.muted{color:var(--rel-muted)}
	.accent,:global(.accent){color:var(--rel-accent)}
	.small-text{font-size:11.5px}
	button,a.ghost{font-family:inherit;cursor:pointer}
	button:disabled{opacity:.55;cursor:default}
	.primary,.ghost{display:inline-flex;align-items:center;gap:7px;padding:8px 13px;border-radius:9px;font-size:12.5px;font-weight:650;white-space:nowrap;text-decoration:none}
	.primary{border:1px solid var(--rel-accent);background:var(--rel-accent);color:var(--rel-bg)}
	.ghost{border:1px solid var(--rel-border);background:var(--rel-card);color:var(--rel-text)}
	.ghost.active{border-color:var(--rel-accent);color:var(--rel-accent)}
	.ghost.small{padding:4px 9px;font-size:11.5px}
	.danger-text{color:var(--rel-danger)}
	.card{margin-bottom:18px;border:1px solid var(--rel-border);border-radius:14px;background:var(--rel-card);overflow:hidden}
	.card-head{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap;padding:16px 20px;border-bottom:1px solid var(--rel-border)}
	.busy-note{margin:5px 0 0;color:var(--rel-muted);font-size:12px}
	.versions{display:flex;align-items:center;gap:26px;flex-wrap:wrap}
	.versions .label{display:block;color:var(--rel-muted);font-size:10px;font-weight:700;letter-spacing:.1em;text-transform:uppercase}
	.big{font-size:22px;font-weight:650}
	.versions small{display:block;margin-top:2px;font-size:11px}
	.line-buttons{display:flex;gap:8px}
	.note{margin:0;padding:12px 20px;color:var(--rel-muted);font-size:12.5px}
	.line-panel,.item-form{display:grid;gap:10px;padding:14px 20px;border-bottom:1px solid var(--rel-border);background:var(--rel-raised)}
	.line-panel p{margin:0;font-size:13px}
	.form-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:10px}
	.field{display:grid;gap:4px;color:var(--rel-muted);font-size:11px}
	.field input,.field select,.field textarea{padding:7px 9px;border:1px solid var(--rel-border);border-radius:8px;background:var(--rel-card);color:var(--rel-text);font:13px inherit}
	.form-actions{display:flex;justify-content:flex-end;gap:8px}
	.form-error{margin:0;color:var(--rel-danger);font-size:12.5px}
	.banner{display:flex;align-items:flex-start;gap:10px;margin-bottom:16px;padding:12px 16px;border:1px solid var(--rel-border);border-radius:11px;font-size:13px}
	.banner div{display:grid;gap:6px;min-width:0}
	.banner code{font:11px/1.5 'SFMono-Regular',Consolas,monospace;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--rel-text)}
	.banner.inset{margin:12px 20px}
	.banner.warning{background:var(--rel-warningTint);color:var(--rel-warning)}
	.banner.danger{background:var(--rel-dangerTint);color:var(--rel-danger)}
	.pill{display:inline-flex;flex:none;padding:2px 8px;border:1px solid var(--rel-border);border-radius:999px;background:var(--rel-raised);color:var(--rel-muted);font-size:10px;font-weight:700;letter-spacing:.04em;text-transform:uppercase}
	.pill.project{color:var(--rel-accent);border-color:color-mix(in srgb,var(--rel-accent) 35%,transparent)}
	.pill.type-bug-fix{color:var(--rel-danger)}
	.pill.type-new-feature{color:var(--rel-accent)}
	.items,.history{list-style:none;margin:0;padding:0}
	.item{display:flex;align-items:flex-start;gap:8px;padding:10px 20px;border-bottom:1px solid var(--rel-border);font-size:13px}
	.item:last-child{border-bottom:0}
	.item-text{display:grid;gap:3px;flex:1;min-width:0;overflow-wrap:anywhere}
	.item-text small{color:var(--rel-muted);font-size:11.5px}
	.issue{margin-left:5px}
	.item-actions{display:flex;align-items:center;gap:6px;font-size:12px}
	.empty-row{padding:16px 20px;color:var(--rel-muted);font-size:13px}
	.release{border-bottom:1px solid var(--rel-border)}
	.release:last-child{border-bottom:0}
	.release.kind-major,.release.kind-minor{background:var(--rel-raised)}
	.release-row{display:flex;align-items:center;gap:10px;width:100%;padding:10px 20px;border:0;background:transparent;color:var(--rel-text);text-align:left;font-size:13px}
	button.release-row:hover{background:var(--rel-raised)}
	.release-row .desc{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
	.chevron{display:grid;place-items:center;width:15px;color:var(--rel-muted);transition:transform .15s}
	.chevron.open{transform:rotate(90deg)}
	.release-body{padding:0 20px 14px 45px}
	.release-body .items{border:1px solid var(--rel-border);border-radius:10px}
	.release-body .item{padding:8px 12px}
	.meta{margin:0 0 8px;font-size:11px}
	.notes-actions{display:flex;gap:8px;margin-top:10px}
	.notes{margin:10px 0 0;max-height:420px;overflow:auto;padding:10px 12px;border:1px solid var(--rel-border);border-radius:8px;background:var(--rel-bg);font:12px/1.55 'SFMono-Regular',Consolas,monospace;white-space:pre-wrap}
	:global(.rel-spin){animation:rel-spin 1s linear infinite}
	@keyframes rel-spin{to{transform:rotate(360deg)}}
	@media(max-width:640px){.page-head{flex-direction:column;align-items:flex-start}.item{flex-wrap:wrap}.release-body{padding-left:20px}}
</style>
