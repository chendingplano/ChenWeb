<script lang="ts">
	// Overview of document groups as cards (Development › System Admin › Resources › Documents):
	// Sync (rescan + classify new, as `mise build-docs <group>`), Build Page (publish the group to
	// Resources › Documents) and New group. Editing members and documents stays on the full admin page.
	// See KnowledgeStore/doc-repo/modules/module-document-builder-frontend.md.
	import { onMount } from 'svelte';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import FilePlusIcon from '@lucide/svelte/icons/file-plus';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import {
		ApiError,
		MemberRefusedError,
		buildPage,
		createGroup,
		groupLabel,
		hasPage,
		listGroups,
		nextPosition,
		parseRoles,
		syncGroup,
		type DocGroup
	} from './doc-groups-client';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	const c = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#fff',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#64748B',
		border: darkMode ? '#2D3348' : '#DCE2EA',
		accent: darkMode ? '#818CF8' : '#6366F1',
		danger: '#EF4444'
	});

	let groups = $state<DocGroup[]>([]);
	let loading = $state(true);
	let loadError = $state('');
	// The card that is working, and what it is doing; one action at a time.
	let working = $state<{ key: string; action: 'sync' | 'build' } | null>(null);
	let notes = $state<Record<string, { text: string; error: boolean }>>({});
	const locale = $derived(getLocale());

	async function load() {
		loading = true;
		try {
			groups = await listGroups();
			loadError = '';
		} catch {
			loadError = m.docgroups_failed();
		} finally {
			loading = false;
		}
	}

	async function act(g: DocGroup, action: 'sync' | 'build') {
		working = { key: g.group_key, action };
		try {
			if (action === 'sync') {
				const r = await syncGroup(g.group_key);
				notes[g.group_key] =
					r.classified === null
						? { text: m.docgroups_synced_no_model(), error: true }
						: { text: m.docgroups_synced({ count: r.classified }), error: false };
			} else {
				await buildPage(g);
				notes[g.group_key] = { text: m.docgroups_page_built(), error: false };
			}
			await load();
		} catch {
			notes[g.group_key] = { text: m.docgroups_failed(), error: true };
		} finally {
			working = null;
		}
	}

	// New group form.
	let creating = $state(false);
	let saving = $state(false);
	let formError = $state('');
	let fKey = $state('');
	let fEn = $state('');
	let fZh = $state('');
	let fOrder = $state(10);
	let fRoles = $state('admin');
	let fMembers = $state<{ member_type: 'dir' | 'file'; path: string }[]>([]);
	const filledMembers = $derived(fMembers.filter((x) => x.path.trim()));
	const canCreate = $derived(!saving && fKey.trim() !== '' && filledMembers.length > 0);

	function openForm() {
		fKey = '';
		fEn = '';
		fZh = '';
		fOrder = nextPosition(groups);
		fRoles = 'admin';
		fMembers = [{ member_type: 'dir', path: '' }];
		formError = '';
		creating = true;
	}

	async function create() {
		saving = true;
		formError = '';
		const key = fKey.trim();
		try {
			await createGroup({
				group_key: key,
				label: { en: fEn.trim() || key, 'zh-cn': fZh.trim() },
				sort_order: Number(fOrder) || 0,
				access_role: parseRoles(fRoles),
				members: filledMembers.map((x) => ({ member_type: x.member_type, path: x.path.trim() }))
			});
			creating = false;
			notes[key] = { text: m.docgroups_created(), error: false };
			await load();
		} catch (e) {
			formError =
				e instanceof MemberRefusedError
					? m.docgroups_member_refused({ path: e.path })
					: e instanceof ApiError && e.status === 409
						? m.docgroups_key_taken()
						: e instanceof ApiError && e.status === 400
							? m.docgroups_key_invalid()
							: m.docgroups_failed();
		} finally {
			saving = false;
		}
	}

	onMount(load);

	const input = 'w-full rounded-md px-2 py-1';
	const button =
		'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm disabled:opacity-50';
</script>

<div class="h-full overflow-y-auto p-6 select-text" style="background:{c.bg}; color:{c.text};">
	<div class="mb-1 flex items-center gap-3">
		<h1 class="flex-1 text-xl font-semibold">{m.docgroups_title()}</h1>
		<button
			class={button}
			style="background:{c.accent}; color:#fff;"
			disabled={creating}
			onclick={openForm}><PlusIcon size={15} />{m.docgroups_new_group()}</button
		>
	</div>
	<p class="mb-5 text-sm" style="color:{c.muted};">{m.docgroups_cards_intro()}</p>
	{#if loadError}<p class="mb-3" style="color:{c.danger};">{loadError}</p>{/if}

	{#if creating}
		<section
			class="mb-6 max-w-3xl rounded-lg p-4"
			style="background:{c.card}; border:1px solid {c.accent};"
		>
			<h2 class="mb-1 font-semibold">{m.docgroups_new_group()}</h2>
			<p class="mb-3 text-sm" style="color:{c.muted};">{m.docgroups_new_group_hint()}</p>
			<div class="grid grid-cols-2 gap-3">
				<label>
					<span style="font-size:12px; color:{c.muted};">{m.docgroups_key()}</span>
					<input
						class={input}
						style="border:1px solid {c.border}; background:transparent;"
						bind:value={fKey}
					/>
				</label>
				<label>
					<span style="font-size:12px; color:{c.muted};">{m.docgroups_position()}</span>
					<input
						type="number"
						class={input}
						style="border:1px solid {c.border}; background:transparent;"
						bind:value={fOrder}
					/>
				</label>
				<label>
					<span style="font-size:12px; color:{c.muted};">{m.docgroups_label_en()}</span>
					<input
						class={input}
						style="border:1px solid {c.border}; background:transparent;"
						bind:value={fEn}
					/>
				</label>
				<label>
					<span style="font-size:12px; color:{c.muted};">{m.docgroups_label_zh()}</span>
					<input
						class={input}
						style="border:1px solid {c.border}; background:transparent;"
						bind:value={fZh}
					/>
				</label>
				<label class="col-span-2">
					<span style="font-size:12px; color:{c.muted};">{m.docgroups_roles()}</span>
					<input
						class={input}
						style="border:1px solid {c.border}; background:transparent;"
						bind:value={fRoles}
					/>
				</label>
			</div>

			<h3 class="mt-4 mb-1 text-sm font-semibold">{m.docgroups_members()}</h3>
			{#each fMembers as mem, i (i)}
				<div class="mb-2 flex gap-2">
					<select
						class="rounded-md px-2 py-1"
						style="border:1px solid {c.border}; background:transparent;"
						bind:value={mem.member_type}
					>
						<option value="dir">{m.docgroups_dir()}</option>
						<option value="file">{m.docgroups_file()}</option>
					</select>
					<input
						class={input}
						style="border:1px solid {c.border}; background:transparent;"
						placeholder={m.docgroups_path()}
						bind:value={mem.path}
					/>
					{#if fMembers.length > 1}
						<button class="text-sm" style="color:{c.danger};" onclick={() => fMembers.splice(i, 1)}
							>{m.docgroups_remove()}</button
						>
					{/if}
				</div>
			{/each}
			<button
				class="text-sm"
				style="color:{c.accent};"
				onclick={() => fMembers.push({ member_type: 'dir', path: '' })}
				>{m.docgroups_add_member()}</button
			>

			{#if formError}<p class="mt-3 text-sm" style="color:{c.danger};">{formError}</p>{/if}
			<div class="mt-4 flex gap-2">
				<button
					class={button}
					style="background:{c.accent}; color:#fff;"
					disabled={!canCreate}
					onclick={create}>{m.docgroups_create()}</button
				>
				<button
					class={button}
					style="border:1px solid {c.border};"
					disabled={saving}
					onclick={() => (creating = false)}>{m.docgroups_cancel()}</button
				>
			</div>
		</section>
	{/if}

	{#if loading && !groups.length}
		<p style="color:{c.muted};">{m.docgroups_loading()}</p>
	{:else if !groups.length && !loadError}
		<p style="color:{c.muted};">{m.docgroups_no_groups()}</p>
	{/if}

	<div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));">
		{#each groups as g (g.group_id)}
			{@const label = groupLabel(g, locale)}
			{@const busy = working?.key === g.group_key}
			{@const note = notes[g.group_key]}
			<section
				class="flex flex-col rounded-lg p-4"
				style="background:{c.card}; border:1px solid {c.border};"
			>
				<div class="flex items-baseline gap-2">
					<h2 class="flex-1 truncate font-semibold">{label}</h2>
					<span
						class="rounded px-1.5 font-mono"
						style="font-size:11px; border:1px solid {c.border}; color:{c.muted};"
						>{g.group_key}</span
					>
				</div>
				<p class="mt-1 text-sm" style="color:{hasPage(g) ? c.accent : c.muted};">
					{hasPage(g) ? m.docgroups_page_at({ label }) : m.docgroups_no_page()}
				</p>
				<p class="mt-1 text-sm" style="color:{c.muted};">
					{m.docgroups_counts({ documents: g.documents, uncategorized: g.uncategorized })}
				</p>
				<p class="text-sm" style="color:{c.muted};">
					{m.docgroups_visible_to({
						roles: g.access_role.length ? g.access_role.join(', ') : m.docgroups_nobody()
					})}
				</p>

				<ul class="mt-3 flex-1 space-y-1 text-sm">
					{#each g.members as mem (mem.member_id)}
						<li class="flex items-center gap-1.5" title={mem.path}>
							{#if mem.member_type === 'dir'}
								<FolderIcon size={14} style="color:{c.muted}; flex-shrink:0;" />
							{:else}
								<FileTextIcon size={14} style="color:{c.muted}; flex-shrink:0;" />
							{/if}
							<span class="min-w-0 flex-1 truncate">{mem.path}</span>
							{#if !mem.exists}<span style="color:{c.danger}; font-size:12px;"
									>{m.docgroups_not_found()}</span
								>{/if}
						</li>
					{:else}
						<li style="color:{c.muted};">{m.docgroups_no_members()}</li>
					{/each}
				</ul>

				{#if note}
					<p class="mt-3 text-sm" style="color:{note.error ? c.danger : c.accent};">{note.text}</p>
				{/if}
				<div class="mt-3 flex gap-2">
					<button
						class={button}
						style="border:1px solid {c.border};"
						disabled={working !== null}
						onclick={() => act(g, 'sync')}
					>
						<RefreshCwIcon
							size={14}
							class={busy && working?.action === 'sync' ? 'animate-spin' : ''}
						/>
						{busy && working?.action === 'sync' ? m.docgroups_syncing() : m.docgroups_sync()}
					</button>
					<button
						class={button}
						style="border:1px solid {c.border}; color:{hasPage(g) ? c.muted : c.accent};"
						disabled={working !== null || hasPage(g)}
						title={hasPage(g) ? m.docgroups_page_exists() : undefined}
						onclick={() => act(g, 'build')}
					>
						<FilePlusIcon size={14} />{m.docgroups_build_page()}
					</button>
				</div>
			</section>
		{/each}
	</div>
</div>
