<script lang="ts">
	// Administration of document groups: menu location and access, members, documents,
	// classification. See KnowledgeStore/doc-repo/modules/module-document-builder-backend.md.
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import {
		ApiError,
		addMember,
		classify,
		deleteGroup,
		listDocs,
		listGroups,
		parseRoles,
		removeDoc,
		removeMember,
		rescan,
		saveGroup,
		type DocGroup,
		type DocGroupDoc
	} from './doc-groups-client';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	const c = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#fff',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#64748B',
		border: darkMode ? '#2D3348' : '#DCE2EA',
		accent: darkMode ? '#818CF8' : '#6366F1'
	});

	let groups = $state<DocGroup[]>([]);
	let selectedKey = $state<string | null>(null);
	let docs = $state<DocGroupDoc[]>([]);
	let loading = $state(true);
	let busy = $state(false);
	let message = $state('');
	let error = $state('');

	// Form state for the selected (or new) group.
	let isNew = $state(false);
	let fKey = $state('');
	let fEn = $state('');
	let fZh = $state('');
	let fParent = $state('documents');
	let fOrder = $state(100);
	let fRoles = $state('');
	let fEnabled = $state(true);
	let mType = $state<'dir' | 'file'>('dir');
	let mPath = $state('');

	const selected = $derived(groups.find((g) => g.group_key === selectedKey) ?? null);

	function fill(g: DocGroup | null) {
		fKey = g?.group_key ?? '';
		fEn = g?.label?.en ?? '';
		fZh = g?.label?.['zh-cn'] ?? '';
		fParent = g?.parent_menu_id ?? 'documents';
		fOrder = g?.sort_order ?? 100;
		fRoles = (g?.access_role ?? ['admin']).join(', ');
		fEnabled = g?.enabled ?? true;
	}

	function say(text: string) {
		message = text;
		error = '';
	}
	function fail(e: unknown) {
		message = '';
		error =
			e instanceof ApiError && e.status === 409 && e.groups.length
				? m.docgroups_delete_blocked({ groups: e.groups.join(', ') })
				: m.docgroups_failed();
	}

	async function load(keep?: string | null) {
		loading = true;
		try {
			groups = await listGroups();
			if (keep !== undefined) await select(keep);
		} catch (e) {
			fail(e);
		} finally {
			loading = false;
		}
	}

	async function select(key: string | null) {
		selectedKey = key;
		isNew = false;
		docs = [];
		fill(groups.find((g) => g.group_key === key) ?? null);
		if (key) {
			try {
				docs = await listDocs(key);
			} catch (e) {
				fail(e);
			}
		}
	}

	function startNew() {
		selectedKey = null;
		isNew = true;
		docs = [];
		fill(null);
	}

	async function run(action: () => Promise<void>, ok?: string) {
		busy = true;
		try {
			await action();
			if (ok) say(ok);
		} catch (e) {
			fail(e);
		} finally {
			busy = false;
		}
	}

	const save = () =>
		run(async () => {
			const saved = await saveGroup(
				{
					group_key: fKey.trim(),
					label: { en: fEn.trim(), 'zh-cn': fZh.trim() },
					parent_menu_id: fParent.trim() || 'documents',
					sort_order: Number(fOrder) || 0,
					access_role: parseRoles(fRoles),
					enabled: fEnabled
				},
				isNew
			);
			await load(saved.group_key);
		}, m.docgroups_saved());

	const add = () =>
		run(async () => {
			if (!selected) return;
			await addMember(selected.group_key, mType, mPath.trim());
			mPath = '';
			await load(selected.group_key);
		});

	const dropMember = (id: number) =>
		run(async () => {
			if (!selected) return;
			await removeMember(selected.group_key, id);
			await load(selected.group_key);
		});

	const dropDoc = (d: DocGroupDoc, del: boolean) =>
		run(async () => {
			if (!selected) return;
			const text = del
				? m.docgroups_confirm_delete({ title: d.title })
				: m.docgroups_confirm_remove({ title: d.title });
			if (!confirm(text)) return;
			await removeDoc(selected.group_key, d.doc_id, del);
			await load(selected.group_key);
		});

	const doRescan = () =>
		run(async () => {
			if (!selected) return;
			await rescan(selected.group_key);
			await load(selected.group_key);
		});

	const doClassify = (force: boolean) =>
		run(async () => {
			if (!selected) return;
			const r = await classify(selected.group_key, force);
			say(m.docgroups_classified({ count: r.classified }));
			await load(selected.group_key);
		});

	const dropGroup = () =>
		run(async () => {
			if (!selected || !confirm(m.docgroups_confirm_delete_group({ key: selected.group_key })))
				return;
			await deleteGroup(selected.group_key);
			await load(null);
		});

	onMount(() => load(null));

	const input = 'w-full rounded-md px-2 py-1';
</script>

<div class="h-full overflow-y-auto p-6 select-text" style="background:{c.bg}; color:{c.text};">
	<h1 class="mb-4 text-xl font-semibold">{m.docgroups_title()}</h1>
	{#if error}<p class="mb-3" style="color:#EF4444;">{error}</p>{/if}
	{#if message}<p class="mb-3" style="color:{c.accent};">{message}</p>{/if}

	<div class="flex gap-6">
		<!-- Group list -->
		<div class="w-64 shrink-0">
			<button
				class="mb-3 w-full rounded-md px-3 py-2"
				style="border:1px solid {c.border}; color:{c.accent};"
				onclick={startNew}>{m.docgroups_new_group()}</button
			>
			{#if loading}<p style="color:{c.muted};">{m.docgroups_loading()}</p>{/if}
			{#each groups as g (g.group_id)}
				<button
					class="mb-1 block w-full rounded-md px-3 py-2 text-left"
					style="background:{g.group_key === selectedKey
						? c.card
						: 'transparent'}; border:1px solid {g.group_key === selectedKey ? c.accent : c.border};"
					onclick={() => select(g.group_key)}
				>
					<div class="font-medium">{g.label?.en || g.group_key}</div>
					<div style="font-size:12px; color:{c.muted};">
						{g.parent_menu_id} · {g.sort_order}{g.enabled ? '' : ' · ✕'}
					</div>
					<div style="font-size:12px; color:{c.muted};">
						{m.docgroups_counts({ documents: g.documents, uncategorized: g.uncategorized })}
					</div>
				</button>
			{/each}
		</div>

		<!-- Detail -->
		<div class="min-w-0 flex-1">
			{#if !selected && !isNew}
				<p style="color:{c.muted};">{m.docgroups_select_group()}</p>
			{:else}
				<section
					class="mb-6 rounded-lg p-4"
					style="background:{c.card}; border:1px solid {c.border};"
				>
					<div class="grid grid-cols-2 gap-3">
						<label>
							<span style="font-size:12px; color:{c.muted};">{m.docgroups_key()}</span>
							<input
								class={input}
								style="border:1px solid {c.border}; background:transparent;"
								bind:value={fKey}
								disabled={!isNew}
							/>
						</label>
						<label>
							<span style="font-size:12px; color:{c.muted};">{m.docgroups_parent()}</span>
							<input
								class={input}
								style="border:1px solid {c.border}; background:transparent;"
								bind:value={fParent}
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
							<span style="font-size:12px; color:{c.muted};">{m.docgroups_roles()}</span>
							<input
								class={input}
								style="border:1px solid {c.border}; background:transparent;"
								bind:value={fRoles}
							/>
						</label>
					</div>
					<label class="mt-3 flex items-center gap-2">
						<input type="checkbox" bind:checked={fEnabled} />
						{m.docgroups_enabled()}
					</label>
					<div class="mt-3 flex gap-2">
						<button
							class="rounded-md px-3 py-1"
							style="background:{c.accent}; color:#fff;"
							disabled={busy}
							onclick={save}>{m.docgroups_save()}</button
						>
						{#if selected}
							<button
								class="rounded-md px-3 py-1"
								style="border:1px solid #EF4444; color:#EF4444;"
								disabled={busy}
								onclick={dropGroup}>{m.docgroups_delete_group()}</button
							>
						{/if}
					</div>
				</section>

				{#if selected}
					<section
						class="mb-6 rounded-lg p-4"
						style="background:{c.card}; border:1px solid {c.border};"
					>
						<h2 class="mb-2 font-semibold">{m.docgroups_members()}</h2>
						{#each selected.members as mem (mem.member_id)}
							<div class="flex items-center gap-2 py-1">
								<span class="rounded px-2" style="font-size:12px; border:1px solid {c.border};">
									{mem.member_type === 'dir' ? m.docgroups_dir() : m.docgroups_file()}
								</span>
								<span class="flex-1 truncate">{mem.path}</span>
								{#if !mem.exists}<span style="color:#EF4444; font-size:12px;"
										>{m.docgroups_not_found()}</span
									>{/if}
								<button
									style="color:#EF4444;"
									disabled={busy}
									onclick={() => dropMember(mem.member_id)}>{m.docgroups_remove()}</button
								>
							</div>
						{:else}
							<p style="color:{c.muted};">{m.docgroups_no_members()}</p>
						{/each}
						<div class="mt-3 flex gap-2">
							<select
								class="rounded-md px-2 py-1"
								style="border:1px solid {c.border}; background:transparent;"
								bind:value={mType}
							>
								<option value="dir">{m.docgroups_dir()}</option>
								<option value="file">{m.docgroups_file()}</option>
							</select>
							<input
								class={input}
								style="border:1px solid {c.border}; background:transparent;"
								placeholder={m.docgroups_path()}
								bind:value={mPath}
							/>
							<button
								class="rounded-md px-3 py-1"
								style="border:1px solid {c.border}; color:{c.accent};"
								disabled={busy || !mPath.trim()}
								onclick={add}>{m.docgroups_add()}</button
							>
						</div>
					</section>

					<section class="rounded-lg p-4" style="background:{c.card}; border:1px solid {c.border};">
						<div class="mb-2 flex items-center gap-2">
							<h2 class="flex-1 font-semibold">{m.docgroups_documents()}</h2>
							<button
								class="rounded-md px-3 py-1"
								style="border:1px solid {c.border};"
								disabled={busy}
								onclick={doRescan}>{m.docgroups_rescan()}</button
							>
							<button
								class="rounded-md px-3 py-1"
								style="border:1px solid {c.border}; color:{c.accent};"
								disabled={busy}
								onclick={() => doClassify(false)}>{m.docgroups_classify_new()}</button
							>
							<button
								class="rounded-md px-3 py-1"
								style="border:1px solid {c.border};"
								disabled={busy}
								onclick={() => doClassify(true)}>{m.docgroups_classify_all()}</button
							>
						</div>
						<table class="w-full" style="font-size:13px;">
							<thead style="color:{c.muted}; text-align:left;">
								<tr
									><th>{m.docgroups_col_title()}</th><th>{m.docgroups_col_category()}</th><th
										>{m.docgroups_col_file()}</th
									><th></th></tr
								>
							</thead>
							<tbody>
								{#each docs as d (d.doc_id)}
									<tr style="border-top:1px solid {c.border};">
										<td class="py-1 pr-2">{d.title}</td>
										<td class="pr-2">{d.category}</td>
										<td class="max-w-xs truncate pr-2" style="color:{c.muted};">{d.path}</td>
										<td class="whitespace-nowrap">
											<button class="mr-2" disabled={busy} onclick={() => dropDoc(d, false)}
												>{m.docgroups_remove_from_group()}</button
											>
											<button
												style="color:#EF4444;"
												disabled={busy}
												onclick={() => dropDoc(d, true)}>{m.docgroups_remove_and_delete()}</button
											>
										</td>
									</tr>
								{:else}
									<tr><td colspan="4" style="color:{c.muted};">{m.docgroups_no_documents()}</td></tr
									>
								{/each}
							</tbody>
						</table>
					</section>
				{/if}
			{/if}
		</div>
	</div>
</div>
