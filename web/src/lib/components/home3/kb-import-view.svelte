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

	let totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');

	function getParsingOutcome(record: KbInputRecord): 'pending' | 'success' | 'failed' {
		const parsingItems = (record.status ?? []).filter((item) => item?.operation === 'parsing');
		if (parsingItems.length === 0) return 'pending';
		const hasSuccess = parsingItems.some((item) => (item.status ?? '').toLowerCase() === 'success');
		return hasSuccess ? 'success' : 'failed';
	}

	function formatTime(value?: string): string {
		if (!value) return '-';
		const d = new Date(value);
		if (Number.isNaN(d.getTime())) return value;
		return d.toLocaleString();
	}

	function parsingLabel(record: KbInputRecord): string {
		const parsing = getParsingOutcome(record);
		if (parsing === 'success') return 'Parsed Success';
		if (parsing === 'failed') return 'Parsed Failed';
		return 'Pending';
	}

	function parsingBg(record: KbInputRecord): string {
		const parsing = getParsingOutcome(record);
		if (parsing === 'success') return 'rgba(16,185,129,0.14)';
		if (parsing === 'failed') return 'rgba(239,68,68,0.14)';
		return 'rgba(245,158,11,0.14)';
	}

	function parsingColor(record: KbInputRecord): string {
		const parsing = getParsingOutcome(record);
		if (parsing === 'success') return '#10b981';
		if (parsing === 'failed') return '#ef4444';
		return '#f59e0b';
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
			Browse and filter `kb.inputs` records.
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
						<th class="cell head">Create Time</th>
						<th class="cell head">Modify Time</th>
						<th class="cell head">Error</th>
					</tr>
				</thead>
				<tbody>
					{#if !loading && records.length === 0}
						<tr>
							<td class="cell" colspan={7} style="text-align:center; color:{textMuted};">No records</td>
						</tr>
					{:else}
						{#each records as record (record.id)}
							<tr style="border-top:1px solid {borderColor};">
								<td class="cell" style="color:{textSecondary};">{record.id}</td>
								<td class="cell" style="color:{textPrimary};">{record.type}</td>
								<td class="cell" style="color:{textPrimary};">{record.file_name ?? '-'}</td>
								<td class="cell">
									<span
										style="
											display:inline-flex;
											align-items:center;
											height:22px;
											padding:0 8px;
											border-radius:999px;
											font-size:12px;
											font-weight:600;
											background:{parsingBg(record)};
											color:{parsingColor(record)};
										"
									>
										{parsingLabel(record)}
									</span>
								</td>
								<td class="cell" style="color:{textSecondary};">{formatTime(record.create_time)}</td>
								<td class="cell" style="color:{textSecondary};">{formatTime(record.modify_time)}</td>
								<td class="cell" style="color:#ef4444;">{record.error_msg ?? '-'}</td>
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
