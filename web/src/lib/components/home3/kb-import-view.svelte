<script lang="ts">
	import { onMount } from 'svelte';
	import type { KbInputRecord, ParseState } from '$lib/services/kbService';
	import { listKbInputs } from '$lib/services/kbService';

	let { darkMode = true }: { darkMode: boolean } = $props();

	const docTypeOptions = [
		'all',
		'pdf',
		'doc',
		'excel',
		'ppt',
		'text',
		'json',
		'xml',
		'markdown',
		'typst'
	];
	const parseStateOptions: Array<{ value: ParseState; label: string }> = [
		{ value: 'all', label: 'All' },
		{ value: 'pending', label: 'Pending' },
		{ value: 'parsed_success', label: 'Parsed Success' },
		{ value: 'parsed_failed', label: 'Parsed Failed' }
	];

	let docType = $state('all');
	let parseState = $state<ParseState>('all');
	let fileName = $state('');
	let startTime = $state('');
	let endTime = $state('');
	let page = $state(1);
	let pageSize = $state(50);
	let total = $state(0);
	let records = $state<KbInputRecord[]>([]);
	let loading = $state(false);
	let error = $state('');
	let statusDialogOpen = $state(false);
	let statusDialogTitle = $state('');
	let statusDialogItems = $state<KbInputRecord['status']>([]);
	let statusDialogRawJson = $state('[]');

	let totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');

	type StatusItem = KbInputRecord['status'][number];

	function findStatusItem(record: KbInputRecord, operation: string): StatusItem | null {
		return (
			(record.status ?? []).find((item) => (item?.operation ?? '').toLowerCase() === operation.toLowerCase()) ??
			null
		);
	}

	function formatTime(value?: string): string {
		if (!value) return '-';
		const d = new Date(value);
		if (Number.isNaN(d.getTime())) return value;
		return d.toLocaleString();
	}

	function formatOptionalTime(value?: string): string {
		if (!value) return '';
		const d = new Date(value);
		if (Number.isNaN(d.getTime())) return value;
		return d.toLocaleString();
	}

	function parsingItem(record: KbInputRecord): StatusItem | null {
		return findStatusItem(record, 'parsing') ?? findStatusItem(record, 'parsed');
	}

	function parsingLabel(record: KbInputRecord): string {
		const item = parsingItem(record);
		if (!item) return '';
		const operation = (item.operation ?? '').toLowerCase();
		if (operation === 'parsing') return 'parsing';
		if (operation === 'parsed') return item.proc_status ?? item['proc-status'] ?? '';
		return '';
	}

	function parsingTime(record: KbInputRecord): string {
		return formatOptionalTime(parsingItem(record)?.start_time);
	}

	function convertedItem(record: KbInputRecord): StatusItem | null {
		return findStatusItem(record, 'converted');
	}

	function convertLabel(record: KbInputRecord): string {
		const item = convertedItem(record);
		return item?.proc_status ?? item?.['proc-status'] ?? '';
	}

	function convertTime(record: KbInputRecord): string {
		return formatOptionalTime(convertedItem(record)?.start_time);
	}

	function openStatusDialog(record: KbInputRecord) {
		const items = record.status ?? [];
		statusDialogItems = items;
		statusDialogTitle = `Record ID: ${record.id}, Field 'Status'`;
		statusDialogRawJson = JSON.stringify(items, null, 2);
		statusDialogOpen = true;
	}

	function closeStatusDialog() {
		statusDialogOpen = false;
	}

	async function loadRecords() {
		loading = true;
		error = '';
		try {
			const result = await listKbInputs({
				docType,
				parseState,
				fileName,
				startTime,
				endTime,
				page,
				pageSize
			});
			records = result.results ?? [];
			total = result.total ?? 0;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load records';
		} finally {
			loading = false;
		}
	}

	function doSearch() {
		page = 1;
		loadRecords();
	}

	function resetSearch() {
		docType = 'all';
		parseState = 'all';
		fileName = '';
		startTime = '';
		endTime = '';
		page = 1;
		pageSize = 50;
		loadRecords();
	}

	function prevPage() {
		if (page <= 1) return;
		page -= 1;
		loadRecords();
	}

	function nextPage() {
		if (page >= totalPages) return;
		page += 1;
		loadRecords();
	}

	onMount(() => {
		loadRecords();
	});
</script>

<div class="p-6">
	<div
		class="rounded-xl p-6 mb-6"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<h2 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:6px;">Import Inputs</h2>
		<p style="font-size:14px; color:{textSecondary};">
			Browse and filter Knowledge Base import records.
		</p>
	</div>

	<div
		class="rounded-xl p-5 mb-4"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<div class="grid gap-3" style="grid-template-columns: repeat(3, minmax(0, 1fr));">
			<label class="flex flex-col gap-1.5">
				<span style="font-size:12px; color:{textMuted};">Doc Type</span>
				<select
					bind:value={docType}
					style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
				>
					{#each docTypeOptions as option}
						<option value={option}>{option}</option>
					{/each}
				</select>
			</label>

			<label class="flex flex-col gap-1.5">
				<span style="font-size:12px; color:{textMuted};">Parsing State</span>
				<select
					bind:value={parseState}
					style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
				>
					{#each parseStateOptions as option}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
			</label>

			<label class="flex flex-col gap-1.5">
				<span style="font-size:12px; color:{textMuted};">File Name</span>
				<input
					bind:value={fileName}
					type="text"
					placeholder="Contains file name"
					style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
				/>
			</label>

			<label class="flex flex-col gap-1.5">
				<span style="font-size:12px; color:{textMuted};">Start Time</span>
				<input
					bind:value={startTime}
					type="datetime-local"
					style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
				/>
			</label>

			<label class="flex flex-col gap-1.5">
				<span style="font-size:12px; color:{textMuted};">End Time</span>
				<input
					bind:value={endTime}
					type="datetime-local"
					style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
				/>
			</label>

			<label class="flex flex-col gap-1.5">
				<span style="font-size:12px; color:{textMuted};">Page Size</span>
				<select
					value={String(pageSize)}
					onchange={(e) => {
						const target = e.currentTarget as HTMLSelectElement;
						const nextValue = Number(target.value ?? '50');
						pageSize = Number.isFinite(nextValue) && nextValue > 0 ? nextValue : 50;
						page = 1;
						loadRecords();
					}}
					style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
				>
					<option value={20}>20</option>
					<option value={50}>50</option>
					<option value={100}>100</option>
					<option value={200}>200</option>
				</select>
			</label>
		</div>

		<div class="mt-4 flex items-center gap-2">
			<button
				onclick={doSearch}
				style="height:34px; padding:0 14px; border:none; border-radius:8px; background:{accent}; color:white; font-size:13px; font-weight:600; cursor:pointer;"
			>
				Search
			</button>
			<button
				onclick={resetSearch}
				style="height:34px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:13px; cursor:pointer;"
			>
				Reset
			</button>
			{#if loading}
				<span style="font-size:12px; color:{textMuted};">Loading...</span>
			{/if}
			{#if error}
				<span style="font-size:12px; color:#ef4444;">{error}</span>
			{/if}
		</div>
	</div>

	<div
		class="rounded-xl overflow-hidden"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<div class="overflow-auto">
			<table style="width:100%; border-collapse:collapse; min-width:960px;">
				<thead style="background:{pageBg};">
					<tr>
						<th class="cell head">ID</th>
						<th class="cell head">Type</th>
						<th class="cell head">File Name</th>
						<th class="cell head">Parsing</th>
						<th class="cell head">Time</th>
						<th class="cell head">Convert</th>
						<th class="cell head">Time</th>
						<th class="cell head">Create Time</th>
						<th class="cell head">Modify Time</th>
						<th class="cell head">Status</th>
					</tr>
				</thead>
				<tbody>
					{#if !loading && records.length === 0}
						<tr>
							<td class="cell" colspan={10} style="text-align:center; color:{textMuted};">No records</td>
						</tr>
					{:else}
						{#each records as record (record.id)}
							<tr style="border-top:1px solid {borderColor};">
								<td class="cell" style="color:{textSecondary};">{record.id}</td>
								<td class="cell" style="color:{textPrimary};">{record.type}</td>
								<td class="cell" style="color:{textPrimary};">{record.file_name ?? '-'}</td>
								<td class="cell" style="color:{textPrimary};">{parsingLabel(record)}</td>
								<td class="cell" style="color:{textSecondary};">{parsingTime(record)}</td>
								<td class="cell" style="color:{textPrimary};">{convertLabel(record)}</td>
								<td class="cell" style="color:{textSecondary};">{convertTime(record)}</td>
								<td class="cell" style="color:{textSecondary};">{formatTime(record.create_time)}</td>
								<td class="cell" style="color:{textSecondary};">{formatTime(record.modify_time)}</td>
								<td class="cell">
									<button
										onclick={() => openStatusDialog(record)}
										style="height:28px; padding:0 10px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer;"
									>
										View
									</button>
								</td>
							</tr>
						{/each}
					{/if}
				</tbody>
			</table>
		</div>

		<div
			class="flex items-center justify-between px-4 py-3"
			style="border-top:1px solid {borderColor}; background:{pageBg};"
		>
			<div style="font-size:12px; color:{textMuted};">
				Total: {total} | Page {page} / {totalPages}
			</div>
			<div class="flex items-center gap-2">
				<button
					onclick={prevPage}
					disabled={page <= 1 || loading}
					style="height:30px; padding:0 10px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer; opacity:{page <= 1 || loading ? 0.5 : 1};"
				>
					Prev
				</button>
				<button
					onclick={nextPage}
					disabled={page >= totalPages || loading}
					style="height:30px; padding:0 10px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer; opacity:{page >= totalPages || loading ? 0.5 : 1};"
				>
					Next
				</button>
			</div>
		</div>
	</div>
</div>

{#if statusDialogOpen}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:rgba(15,23,42,0.62);"
		onclick={closeStatusDialog}
		onkeydown={(e) => {
			if (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') closeStatusDialog();
		}}
		role="button"
		tabindex="0"
	>
		<div
			class="w-full max-w-4xl rounded-xl overflow-hidden"
			style="background:{cardBg}; border:1px solid {borderColor};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-label="Status details dialog"
			tabindex="0"
		>
			<div
				class="flex items-center justify-between px-4 py-3"
				style="border-bottom:1px solid {borderColor};"
			>
				<h3 style="font-size:15px; font-weight:600; color:{textPrimary};">{statusDialogTitle}</h3>
				<button
					onclick={closeStatusDialog}
					style="height:30px; padding:0 12px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer;"
				>
					Close
				</button>
			</div>

			<div class="grid gap-4 p-4" style="grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);">
				<div class="rounded-lg p-3" style="border:1px solid {borderColor}; background:{surface2};">
					<div style="font-size:12px; font-weight:600; color:{textSecondary}; margin-bottom:8px;">Readable</div>
					{#if statusDialogItems.length === 0}
						<div style="font-size:12px; color:{textMuted};">No status entries.</div>
					{:else}
						<div class="space-y-2">
							{#each statusDialogItems as item, idx}
								<div
									class="rounded-lg p-2"
									style="border:1px solid {borderColor}; background:{cardBg};"
								>
									<div style="font-size:12px; font-weight:600; color:{textPrimary}; margin-bottom:6px;">
										Entry #{idx + 1}
									</div>
									<div class="grid gap-1" style="grid-template-columns: 120px 1fr;">
										<span style="font-size:12px; color:{textMuted};">operation</span>
										<span style="font-size:12px; color:{textPrimary};">{item.operation ?? ''}</span>
										<span style="font-size:12px; color:{textMuted};">proc_status</span>
										<span style="font-size:12px; color:{textPrimary};">{item.proc_status ?? item['proc-status'] ?? ''}</span>
										<span style="font-size:12px; color:{textMuted};">start_time</span>
										<span style="font-size:12px; color:{textPrimary};">{item.start_time ?? ''}</span>
										<span style="font-size:12px; color:{textMuted};">time</span>
										<span style="font-size:12px; color:{textPrimary};">{item.time ?? ''}</span>
										<span style="font-size:12px; color:{textMuted};">status</span>
										<span style="font-size:12px; color:{textPrimary};">{item.status ?? ''}</span>
										<span style="font-size:12px; color:{textMuted};">error</span>
										<span style="font-size:12px; color:{textPrimary};">{item.error ?? ''}</span>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<div class="rounded-lg p-3" style="border:1px solid {borderColor}; background:{surface2};">
					<div style="font-size:12px; font-weight:600; color:{textSecondary}; margin-bottom:8px;">Raw JSON</div>
					<pre
						style="margin:0; max-height:440px; overflow:auto; white-space:pre-wrap; word-break:break-word; font-size:12px; color:{textPrimary};"
					>{statusDialogRawJson}</pre>
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	.cell {
		padding: 10px 12px;
		font-size: 13px;
		vertical-align: middle;
		text-align: left;
	}

	.head {
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.02em;
	}
</style>
