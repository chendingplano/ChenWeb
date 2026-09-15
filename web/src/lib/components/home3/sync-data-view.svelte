<script lang="ts">
	import { onMount } from 'svelte';

	import { applySync, listSyncItems, previewSync, type SyncItem } from './sync-data-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let items = $state<SyncItem[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let previewingID = $state<string | null>(null);
	let applyingID = $state<string | null>(null);
	let previewCounts = $state<Record<string, number>>({});

	onMount(() => {
		loadItems();
	});

	async function loadItems() {
		loading = true;
		error = null;
		try {
			const response = await listSyncItems();
			items = response.items;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	async function runPreview(itemID: string) {
		previewingID = itemID;
		error = null;
		info = null;
		try {
			const result = await previewSync(itemID);
			previewCounts = { ...previewCounts, [itemID]: result.changed_row_count };
			info = `${itemID}: ${result.changed_row_count} row(s) changed since the last sync.`;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			previewingID = null;
		}
	}

	async function runApply(itemID: string) {
		applyingID = itemID;
		error = null;
		info = null;
		try {
			const result = await applySync(itemID);
			const { [itemID]: _discard, ...rest } = previewCounts;
			previewCounts = rest;
			info = `${itemID}: synced ${result.synced_row_count} row(s).`;
			await loadItems();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			applyingID = null;
		}
	}

	function fmtDate(raw?: string): string {
		if (!raw) return 'Never';
		return new Date(raw).toLocaleString();
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#0F766E' : '#0F766E');
	const altBtn = $derived(darkMode ? '#818CF8' : '#6366F1');
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--alt-btn={altBtn}
>
	<header class="toolbar">
		<div>
			<h2>Sync Data</h2>
			<p class="muted">
				Pull registered reference data from the dev source into this deployment. Preview shows
				what would change; Sync applies it and never deletes rows.
			</p>
		</div>
		<button class="ghost" onclick={loadItems} disabled={loading}>
			{loading ? 'Refreshing…' : 'Refresh'}
		</button>
	</header>

	{#if error}
		<div class="error" role="alert">{error}</div>
	{:else if info}
		<div class="info" role="status">{info}</div>
	{/if}

	<div class="panel">
		{#if loading}
			<div class="empty">Loading sync items…</div>
		{:else if items.length === 0}
			<div class="empty">No sync items registered.</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Item</th>
							<th>Table</th>
							<th>Last Synced</th>
							<th>Last Row Count</th>
							<th>Status</th>
							<th>Action</th>
						</tr>
					</thead>
					<tbody>
						{#each items as item (item.item_id)}
							<tr>
								<td class="cell-primary">{item.item_id}</td>
								<td>{item.table}</td>
								<td>{fmtDate(item.last_synced_at)}</td>
								<td>{item.last_row_count}</td>
								<td>
									{#if item.last_error}
										<span class="status-error">{item.last_error}</span>
									{:else if item.item_id in previewCounts}
										<span class="cell-secondary"
											>{previewCounts[item.item_id]} row(s) pending</span
										>
									{:else}
										<span class="cell-secondary">OK</span>
									{/if}
								</td>
								<td>
									<div class="row-actions">
										<button
											class="ghost compact-btn"
											onclick={() => runPreview(item.item_id)}
											disabled={previewingID === item.item_id || applyingID === item.item_id}
										>
											{previewingID === item.item_id ? 'Previewing…' : 'Preview'}
										</button>
										<button
											class="alt-btn compact-btn"
											onclick={() => runApply(item.item_id)}
											disabled={previewingID === item.item_id || applyingID === item.item_id}
										>
											{applyingID === item.item_id ? 'Syncing…' : 'Sync'}
										</button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

<style>
	.wrap {
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--page);
		min-height: 100%;
		padding: 16px 20px 32px;
	}
	.toolbar {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
	}
	h2 {
		margin: 0;
		color: var(--heading);
		font-size: 20px;
	}
	.muted {
		color: var(--sub);
		font-size: 12px;
		margin: 4px 0 0;
		max-width: 560px;
	}
	.ghost,
	.alt-btn {
		border-radius: 8px;
		padding: 8px 14px;
		font-size: 13px;
		cursor: pointer;
	}
	.ghost {
		background: transparent;
		color: var(--heading);
		border: 1px solid var(--border);
	}
	.alt-btn {
		background: var(--alt-btn);
		color: white;
		border: none;
	}
	.compact-btn {
		padding: 6px 10px;
		font-size: 12px;
	}
	.ghost:disabled,
	.alt-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.panel {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 16px;
	}
	.error,
	.info {
		padding: 10px 12px;
		border-radius: 8px;
		font-size: 13px;
	}
	.error {
		background: rgba(248, 113, 113, 0.12);
		color: #f87171;
	}
	.info {
		background: rgba(15, 118, 110, 0.16);
		color: #5eead4;
	}
	.table-wrap {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		padding: 12px 10px;
		border-top: 1px solid var(--border);
		font-size: 13px;
		color: var(--heading);
		text-align: left;
		vertical-align: top;
	}
	th {
		color: var(--sub);
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		border-top: none;
		padding-top: 0;
	}
	.cell-primary {
		font-weight: 600;
	}
	.cell-secondary {
		font-size: 12px;
		color: var(--sub);
	}
	.status-error {
		font-size: 12px;
		color: #f87171;
	}
	.row-actions {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.empty {
		color: var(--sub);
		font-style: italic;
		padding: 24px 8px 8px;
	}
</style>
