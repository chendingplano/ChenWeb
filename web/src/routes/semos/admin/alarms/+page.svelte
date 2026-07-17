<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { fetchAlarms, updateAlarm, type Alarm } from '$lib/services/workspaceListsService';

	let items = $state<Alarm[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let unsolvedOnly = $state(true);
	let noteDrafts = $state<Record<number, string>>({});
	let busyIds = $state<Set<number>>(new Set());

	async function load() {
		loading = true;
		loadError = null;
		try {
			items = await fetchAlarms({ unsolvedOnly, limit: 100 });
		} catch (e) {
			loadError = String(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);
	$effect(() => {
		unsolvedOnly;
		load();
	});

	async function toggleStatus(item: Alarm) {
		const nextStatus = item.status === 'unsolved' ? 'solved' : 'unsolved';
		busyIds = new Set(busyIds).add(item.id);
		try {
			await updateAlarm(item.id, { status: nextStatus });
			await load();
		} catch (e) {
			loadError = String(e);
		} finally {
			const next = new Set(busyIds);
			next.delete(item.id);
			busyIds = next;
		}
	}

	async function addNote(item: Alarm) {
		const note = (noteDrafts[item.id] ?? '').trim();
		if (!note) return;
		busyIds = new Set(busyIds).add(item.id);
		try {
			await updateAlarm(item.id, { note });
			noteDrafts = { ...noteDrafts, [item.id]: '' };
			await load();
		} catch (e) {
			loadError = String(e);
		} finally {
			const next = new Set(busyIds);
			next.delete(item.id);
			busyIds = next;
		}
	}
</script>

<svelte:head>
	<title>{m.semos_admin_alarms_title()}</title>
</svelte:head>

<div class="mx-auto max-w-5xl px-6 py-12">
	<div class="flex items-center justify-between gap-4">
		<h1 class="text-xl font-bold tracking-tight">{m.semos_admin_alarms_title()}</h1>
		<label class="flex items-center gap-2 text-sm">
			<input type="checkbox" bind:checked={unsolvedOnly} />
			{m.semos_admin_toggle_unsolved_only()}
		</label>
	</div>

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
					<th class="py-2 pr-3">{m.semos_admin_column_severity()}</th>
					<th class="py-2 pr-3">{m.semos_admin_column_message()}</th>
					<th class="py-2 pr-3">{m.semos_admin_column_status()}</th>
					<th class="py-2">{m.semos_admin_column_notes()}</th>
				</tr>
			</thead>
			<tbody>
				{#each items as item (item.id)}
					<tr class="border-t border-[#17181c]/10 align-top dark:border-white/10">
						<td class="py-3 pr-3 tabular-nums whitespace-nowrap">{item.occurred_at.slice(0, 16).replace('T', ' ')}</td>
						<td class="py-3 pr-3">{item.severity}</td>
						<td class="max-w-[24ch] py-3 pr-3">{item.message}</td>
						<td class="py-3 pr-3">
							<button
								onclick={() => toggleStatus(item)}
								disabled={busyIds.has(item.id)}
								class="rounded-full px-2 py-0.5 text-xs font-bold disabled:opacity-50 {item.status === 'unsolved'
									? 'bg-[#b4462f]/12 text-[#b4462f] dark:bg-[#e08a76]/15 dark:text-[#e08a76]'
									: 'bg-[#3a7d5c]/12 text-[#3a7d5c] dark:bg-[#7fc9a3]/15 dark:text-[#7fc9a3]'}"
							>
								{item.status === 'unsolved' ? m.semos_admin_status_unsolved() : m.semos_admin_status_solved()}
							</button>
						</td>
						<td class="py-3">
							{#each item.notes as note, i (i)}
								<div class="text-xs text-[#6f6c66] dark:text-[#a5a29b]">
									<span class="font-bold">{note.user || '—'}</span>
									<span class="tabular-nums">{note.time.slice(0, 16).replace('T', ' ')}</span>: {note.note}
								</div>
							{/each}
							<div class="mt-1 flex gap-2">
								<input
									type="text"
									bind:value={noteDrafts[item.id]}
									placeholder={m.semos_admin_note_placeholder()}
									class="w-full rounded border border-[#17181c]/15 bg-transparent px-2 py-1 text-xs dark:border-white/15"
								/>
								<button
									onclick={() => addNote(item)}
									disabled={busyIds.has(item.id)}
									class="shrink-0 text-xs font-bold text-[#b08d57] hover:underline disabled:opacity-50"
								>
									{m.semos_admin_button_add_note()}
								</button>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
