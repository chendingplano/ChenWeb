<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { locales } from '$lib/paraglide/runtime';
	import {
		fetchAnnouncementsAdmin,
		createAnnouncement,
		updateAnnouncement,
		deleteAnnouncement,
		type AnnouncementAdmin
	} from '$lib/services/workspaceListsService';

	let items = $state<AnnouncementAdmin[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	let editingGroupId = $state<number | null>(null);
	let formOccurredAt = $state('');
	let formImportance = $state('normal');
	let formTranslations = $state<Record<string, string>>(emptyTranslations());
	let saving = $state(false);
	let formError = $state<string | null>(null);

	function emptyTranslations(): Record<string, string> {
		return Object.fromEntries(locales.map((l) => [l, '']));
	}

	async function load() {
		loading = true;
		loadError = null;
		try {
			items = await fetchAnnouncementsAdmin();
		} catch (e) {
			loadError = String(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function resetForm() {
		editingGroupId = null;
		formOccurredAt = '';
		formImportance = 'normal';
		formTranslations = emptyTranslations();
		formError = null;
	}

	function startEdit(item: AnnouncementAdmin) {
		editingGroupId = item.group_id;
		formOccurredAt = item.occurred_at.slice(0, 16);
		formImportance = item.importance;
		formTranslations = { ...emptyTranslations(), ...item.translations };
		formError = null;
	}

	async function submitForm() {
		saving = true;
		formError = null;
		try {
			const input = {
				occurred_at: formOccurredAt ? new Date(formOccurredAt).toISOString() : undefined,
				importance: formImportance,
				translations: formTranslations
			};
			if (editingGroupId == null) {
				await createAnnouncement(input);
			} else {
				await updateAnnouncement(editingGroupId, input);
			}
			resetForm();
			await load();
		} catch (e) {
			formError = String(e);
		} finally {
			saving = false;
		}
	}

	async function remove(groupId: number) {
		if (!confirm(m.semos_admin_confirm_delete())) return;
		try {
			await deleteAnnouncement(groupId);
			await load();
		} catch (e) {
			loadError = String(e);
		}
	}
</script>

<svelte:head>
	<title>{m.semos_admin_announcements_title()}</title>
</svelte:head>

<div class="mx-auto max-w-4xl px-6 py-12">
	<h1 class="text-xl font-bold tracking-tight">{m.semos_admin_announcements_title()}</h1>

	<form
		onsubmit={(e) => {
			e.preventDefault();
			submitForm();
		}}
		class="mt-8 rounded-lg border border-[#17181c]/10 p-5 dark:border-white/10"
	>
		<div class="grid gap-4 sm:grid-cols-2">
			<label class="text-sm">
				<span class="mb-1 block text-xs font-bold uppercase tracking-wide text-[#6f6c66] dark:text-[#a5a29b]">
					{m.semos_admin_column_time()}
				</span>
				<input
					type="datetime-local"
					bind:value={formOccurredAt}
					class="w-full rounded border border-[#17181c]/15 bg-transparent px-3 py-2 text-sm dark:border-white/15"
				/>
			</label>
			<label class="text-sm">
				<span class="mb-1 block text-xs font-bold uppercase tracking-wide text-[#6f6c66] dark:text-[#a5a29b]">
					{m.semos_admin_column_importance()}
				</span>
				<input
					type="text"
					bind:value={formImportance}
					placeholder="normal"
					class="w-full rounded border border-[#17181c]/15 bg-transparent px-3 py-2 text-sm dark:border-white/15"
				/>
			</label>
		</div>

		{#each locales as lang (lang)}
			<label class="mt-4 block text-sm">
				<span class="mb-1 block text-xs font-bold uppercase tracking-wide text-[#6f6c66] dark:text-[#a5a29b]">
					{m.semos_admin_column_announcement()} ({lang})
				</span>
				<textarea
					bind:value={formTranslations[lang]}
					rows="2"
					required
					class="w-full rounded border border-[#17181c]/15 bg-transparent px-3 py-2 text-sm dark:border-white/15"
				></textarea>
			</label>
		{/each}

		{#if formError}
			<p class="mt-3 text-sm text-[#b4462f] dark:text-[#e08a76]">{formError}</p>
		{/if}

		<div class="mt-4 flex gap-3">
			<button
				type="submit"
				disabled={saving}
				class="rounded bg-[#b08d57] px-4 py-2 text-sm font-bold text-white disabled:opacity-50"
			>
				{editingGroupId == null ? m.semos_admin_button_add() : m.semos_admin_button_save()}
			</button>
			{#if editingGroupId != null}
				<button
					type="button"
					onclick={resetForm}
					class="rounded border border-[#17181c]/15 px-4 py-2 text-sm dark:border-white/15"
				>
					{m.semos_admin_button_cancel()}
				</button>
			{/if}
		</div>
	</form>

	{#if loadError}
		<p class="mt-6 text-sm text-[#b4462f] dark:text-[#e08a76]">{loadError}</p>
	{:else if loading}
		<p class="mt-6 text-sm text-[#6f6c66] dark:text-[#a5a29b]">…</p>
	{:else if items.length === 0}
		<p class="mt-6 text-sm text-[#6f6c66] dark:text-[#a5a29b]">{m.semos_admin_empty()}</p>
	{:else}
		<table class="mt-8 w-full border-t border-[#17181c]/10 text-sm dark:border-white/10">
			<thead>
				<tr class="text-left text-xs font-bold uppercase tracking-wide text-[#6f6c66] dark:text-[#a5a29b]">
					<th class="py-2 pr-3">{m.semos_admin_column_time()}</th>
					<th class="py-2 pr-3">{m.semos_admin_column_importance()}</th>
					<th class="py-2 pr-3">{m.semos_admin_column_announcement()}</th>
					<th class="py-2"></th>
				</tr>
			</thead>
			<tbody>
				{#each items as item (item.group_id)}
					<tr class="border-t border-[#17181c]/10 align-top dark:border-white/10">
						<td class="py-3 pr-3 tabular-nums whitespace-nowrap">{item.occurred_at.slice(0, 16).replace('T', ' ')}</td>
						<td class="py-3 pr-3">{item.importance}</td>
						<td class="py-3 pr-3">
							{#each locales as lang (lang)}
								<div><span class="text-xs text-[#6f6c66] dark:text-[#a5a29b]">{lang}:</span> {item.translations[lang] ?? ''}</div>
							{/each}
						</td>
						<td class="py-3 whitespace-nowrap">
							<button onclick={() => startEdit(item)} class="text-[#b08d57] hover:underline">
								{m.semos_admin_button_edit()}
							</button>
							<button
								onclick={() => remove(item.group_id)}
								class="ml-3 text-[#b4462f] hover:underline dark:text-[#e08a76]"
							>
								{m.semos_admin_button_delete()}
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
