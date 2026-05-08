<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { shouldShowOverflowScrollbar } from '$lib/components/home3/kb-import-status-dialog.js';
	import { knowledgeStoreState } from '$lib/components/home3/knowledge-store-state.svelte';
	import type { KbInputRecord, ParseState } from '$lib/services/kbService';
	import { listKbInputs, uploadKbInputs } from '$lib/services/kbService';

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
	const uploadParserOptions = ['paddleocr', 'opendata', 'mineru', 'docling'] as const;
	const procStatusOptions = ['all', 'success', 'fail'];

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
	let statusDialogRawJsonEl = $state<HTMLElement | null>(null);
	let statusDialogHasOverflow = $state(false);
	let uploadDialogOpen = $state(false);
	let uploadSubmitting = $state(false);
	let uploadError = $state('');
	let uploadLaunchError = $state('');

	// Search dialog
	let searchOpen = $state(false);
	let searchRecordId = $state('');
	let searchTitle = $state('');
	let searchDocNo = $state('');
	let searchFileName = $state('');
	let searchDocType = $state('all');
	let searchParserName = $state('');
	let searchOperation = $state('');
	let searchProcStatus = $state('all');
	let searchCreateStart = $state('');
	let searchCreateEnd = $state('');
	let searchModifyStart = $state('');
	let searchModifyEnd = $state('');
	let searchResults = $state<KbInputRecord[]>([]);
	let searchLoading = $state(false);
	let searchError = $state('');
	let searchSelected = $state<number | null>(null);
	let uploadType = $state('pdf');
	let uploadTitle = $state('');
	let uploadDocNo = $state('');
	let uploadAuthors = $state('');
	let uploadPublicInfo = $state('');
	let uploadPrivateInfo = $state('');
	let uploadNotes = $state('');
	let uploadKsDesc = $state('');
	let uploadParserName = $state<(typeof uploadParserOptions)[number]>('docling');
	let selectedFiles = $state<File[]>([]);
	let filePicker = $state<HTMLInputElement | null>(null);

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

	async function openStatusDialog(record: KbInputRecord) {
		const items = record.status ?? [];
		statusDialogItems = items;
		statusDialogTitle = `Record ID: ${record.id}, Field 'Status'`;
		statusDialogRawJson = JSON.stringify(items, null, 2);
		statusDialogOpen = true;
		await tick();
		updateStatusDialogOverflow();
	}

	function closeStatusDialog() {
		statusDialogOpen = false;
		statusDialogHasOverflow = false;
	}

	function updateStatusDialogOverflow() {
		if (!statusDialogRawJsonEl) {
			statusDialogHasOverflow = false;
			return;
		}

		statusDialogHasOverflow = shouldShowOverflowScrollbar(
			statusDialogRawJsonEl.scrollHeight,
			statusDialogRawJsonEl.clientHeight
		);
	}

	function resetUploadDialog() {
		uploadError = '';
		uploadType = 'pdf';
		uploadTitle = '';
		uploadDocNo = '';
		uploadAuthors = '';
		uploadPublicInfo = '';
		uploadPrivateInfo = '';
		uploadNotes = '';
		uploadKsDesc = '';
		uploadParserName = 'opendata';		// The default PDF parser
		selectedFiles = [];
		if (filePicker) filePicker.value = '';
	}

	function openUploadDialog() {
		uploadLaunchError = '';
		if (!knowledgeStoreState.activeStore) {
			uploadLaunchError = 'Select an active knowledge store before uploading files.';
			return;
		}
		resetUploadDialog();
		uploadDialogOpen = true;
	}

	function closeUploadDialog() {
		uploadDialogOpen = false;
		uploadSubmitting = false;
		uploadError = '';
	}

	function triggerFilePicker() {
		filePicker?.click();
	}

	function onFileSelection(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		selectedFiles = Array.from(input.files ?? []);
	}

	async function submitUpload() {
		uploadError = '';
		const activeStore = knowledgeStoreState.activeStore;
		if (!activeStore) {
			uploadError = 'No active knowledge store is selected.';
			return;
		}
		if (!activeStore.tenant_id?.trim()) {
			uploadError = 'The active knowledge store is missing tenant_id.';
			return;
		}
		if (selectedFiles.length === 0) {
			uploadError = 'Pick at least one file to upload.';
			return;
		}

		uploadSubmitting = true;
		try {
			await uploadKbInputs({
				type: uploadType,
				title: uploadTitle,
				doc_no: uploadDocNo,
				authors: uploadAuthors,
				public_info: uploadPublicInfo,
				private_info: uploadPrivateInfo,
				notes: uploadNotes,
				ks_desc: uploadKsDesc,
				parser_name: uploadParserName,
				ks_store_id: activeStore.id,
				tenant_id: activeStore.tenant_id,
				files: selectedFiles
			});
			closeUploadDialog();
			resetUploadDialog();
			page = 1;
			await loadRecords();
		} catch (err) {
			uploadError = err instanceof Error ? err.message : 'Failed to upload files';
		} finally {
			uploadSubmitting = false;
		}
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
				pageSize,
				ksStoreId: knowledgeStoreState.activeStore?.id ?? null
			});
			records = result.results ?? [];
			total = result.total ?? 0;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load records';
		} finally {
			loading = false;
		}
	}

	function openSearch() {
		searchOpen = true;
		searchSelected = null;
		searchResults = [];
		searchError = '';
		searchRecordId = '';
		searchTitle = '';
		searchDocNo = '';
		searchFileName = '';
		searchDocType = 'all';
		searchParserName = '';
		searchOperation = '';
		searchProcStatus = 'all';
		searchCreateStart = '';
		searchCreateEnd = '';
		searchModifyStart = '';
		searchModifyEnd = '';
	}

	function closeSearch() {
		searchOpen = false;
	}

	async function runSearch() {
		searchLoading = true;
		searchError = '';
		try {
			const res = await listKbInputs({
				recordId: searchRecordId,
				docType: searchDocType,
				parseState: 'all',
				title: searchTitle,
				docNo: searchDocNo,
				fileName: searchFileName,
				parserName: searchParserName,
				operation: searchOperation,
				procStatus: searchProcStatus === 'all' ? '' : searchProcStatus,
				startTime: searchCreateStart,
				endTime: searchCreateEnd,
				modifyStartTime: searchModifyStart,
				modifyEndTime: searchModifyEnd,
				page: 1,
				pageSize: 50,
				ksStoreId: knowledgeStoreState.activeStoreId ?? undefined
			});
			searchResults = res.results ?? [];
		} catch (err) {
			searchError = err instanceof Error ? err.message : 'Search failed';
		} finally {
			searchLoading = false;
		}
	}

	function recordDisplayName(r: KbInputRecord): string {
		return r.title?.trim() || r.name?.trim() || r.file_name?.trim() || `Input #${r.id}`;
	}

	function recordDisplayDocNo(r: KbInputRecord): string {
		return r.doc_no?.trim() || '—';
	}

	function searchStatusText(record: KbInputRecord): { operation: string; procStatus: string } {
		const items = record.status ?? [];
		const desiredOperation = searchOperation.trim().toLowerCase();
		const matched =
			desiredOperation !== ''
				? [...items]
						.reverse()
						.find((item) => (item?.operation ?? '').trim().toLowerCase() === desiredOperation)
				: [...items].reverse().find((item) => item != null);
		if (!matched) return { operation: '—', procStatus: 'pending' };
		return {
			operation: matched.operation?.trim() || '—',
			procStatus:
				matched.proc_status?.trim() ||
				matched['proc-status']?.trim() ||
				matched.status?.trim() ||
				'pending'
		};
	}

	function pickSearchResult(r: KbInputRecord) {
		searchOpen = false;
		// Apply matching search criteria to the list filters and show that record
		docType = searchDocType !== 'all' ? searchDocType : 'all';
		parseState = 'all';
		fileName = searchFileName;
		startTime = searchCreateStart;
		endTime = searchCreateEnd;
		page = 1;
		loadRecords();
	}

	function confirmSearchSelection() {
		const r = searchResults.find((x) => x.id === searchSelected);
		if (r) pickSearchResult(r);
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

	$effect(() => {
		if (!statusDialogOpen || !statusDialogRawJsonEl) {
			return;
		}

		const node = statusDialogRawJsonEl;
		const resizeObserver =
			typeof ResizeObserver === 'undefined'
				? null
				: new ResizeObserver(() => {
						updateStatusDialogOverflow();
					});

		resizeObserver?.observe(node);
		updateStatusDialogOverflow();

		return () => {
			resizeObserver?.disconnect();
		};
	});
</script>

<div class="p-6">
	<div
		class="rounded-xl p-6 mb-6"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<div class="flex items-start justify-between gap-4">
			<div>
				<h2 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:6px;">Import Inputs</h2>
				<p style="font-size:14px; color:{textSecondary};">
					Browse and filter Knowledge System import records.
				</p>
			</div>
			<div class="flex items-center gap-2">
				<button
					onclick={openSearch}
					style="height:38px; padding:0 14px; border:1px solid {borderColor}; border-radius:10px; background:{surface2}; color:{textPrimary}; font-size:13px; font-weight:600; cursor:pointer; white-space:nowrap;"
				>
					Search
				</button>
				<button
					onclick={openUploadDialog}
					style="height:38px; padding:0 14px; border:none; border-radius:10px; background:{accent}; color:white; font-size:13px; font-weight:600; cursor:pointer; white-space:nowrap;"
				>
					Upload Files
				</button>
			</div>
		</div>
		{#if uploadLaunchError}
			<div style="margin-top:10px; font-size:12px; color:#ef4444;">{uploadLaunchError}</div>
		{/if}
		{#if loading}
			<div style="margin-top:10px; font-size:12px; color:{textMuted};">Loading...</div>
		{/if}
		{#if error}
			<div style="margin-top:10px; font-size:12px; color:#ef4444;">{error}</div>
		{/if}
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
						<th class="cell head">Title</th>
						<th class="cell head">Doc No</th>
						<th class="cell head">Type</th>
						<th class="cell head">File Name</th>
						<th class="cell head">Parser</th>
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
							<td class="cell" colspan={13} style="text-align:center; color:{textMuted};">No records</td>
						</tr>
					{:else}
						{#each records as record (record.id)}
							<tr style="border-top:1px solid {borderColor};">
								<td class="cell" style="color:{textSecondary};">{record.id}</td>
								<td class="cell" style="color:{textPrimary};">{record.title ?? '-'}</td>
								<td class="cell" style="color:{textPrimary};">{record.doc_no ?? '-'}</td>
								<td class="cell" style="color:{textPrimary};">{record.type}</td>
								<td class="cell" style="color:{textPrimary};">{record.file_name ?? '-'}</td>
								<td class="cell" style="color:{textMuted};">{record.parser_name ?? '-'}</td>
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

{#if searchOpen}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:rgba(8,10,14,0.72); backdrop-filter:blur(3px);"
		aria-hidden="true"
	>
		<div
			class="flex flex-col overflow-hidden"
			style="
				width:100%; max-width:1100px; max-height:min(90vh,940px);
				background:{cardBg}; border:1px solid {borderColor}; border-radius:20px;
				box-shadow:0 30px 80px rgba(0,0,0,0.55);
				min-width:720px; min-height:600px; resize:both;
			"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => { if (e.key === 'Escape') closeSearch(); e.stopPropagation(); }}
			role="dialog"
			aria-modal="true"
			aria-label="Find a document"
			tabindex="0"
		>
			<!-- Header -->
			<div class="flex-shrink-0 px-6 py-5" style="border-bottom:1px solid {borderColor};">
				<div style="font-size:10px; font-weight:600; letter-spacing:0.16em; text-transform:uppercase; color:{accent}; margin-bottom:4px;">kb.inputs</div>
				<div style="font-size:22px; font-weight:700; color:{textPrimary}; margin-bottom:4px;">Find a record</div>
				<div style="font-size:13px; color:{textSecondary};">
					Search by record metadata, parser pipeline state, and create or modify windows.
				</div>
				<div style="margin-top:6px; font-size:11px; font-weight:600; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted};">
					Store scope:&nbsp;
					{#if knowledgeStoreState.activeStore}
						<span style="color:{accent}; font-weight:700;">{knowledgeStoreState.activeStore.ks_name}</span>
						<span style="opacity:0.55;"> #{knowledgeStoreState.activeStore.id}</span>
					{:else}
						<span style="color:#ef4444; text-transform:none; letter-spacing:0; font-weight:400;">No active store — results will not be scoped</span>
					{/if}
				</div>
			</div>

			<!-- Scrollable controls + results -->
			<div class="flex-1 overflow-auto flex flex-col" style="min-height:0;">
				<div class="flex-shrink-0 p-5 flex flex-col gap-3">

					<!-- Identity section -->
					<div class="rounded-xl p-4" style="border:1px solid {borderColor}; background:{surface2};">
						<div style="font-size:10px; font-weight:700; letter-spacing:0.14em; text-transform:uppercase; color:{accent}; margin-bottom:8px;">Identity</div>
						<div class="grid gap-2" style="grid-template-columns:0.7fr 0.7fr 1.2fr 1fr 1.4fr;">
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Record ID</span>
								<input type="text" bind:value={searchRecordId} placeholder="84"
									onkeydown={(e) => { if (e.key === 'Enter') runSearch(); }}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Type</span>
								<select bind:value={searchDocType}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;">
									{#each docTypeOptions as o}<option value={o}>{o}</option>{/each}
								</select>
							</label>
							<label class="flex flex-col gap-1" style="grid-column:span 2;">
								<span style="font-size:11px; color:{textMuted};">Title contains</span>
								<input type="text" bind:value={searchTitle} placeholder="Input title, standard title…"
									onkeydown={(e) => { if (e.key === 'Enter') runSearch(); }}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Doc No contains</span>
								<input type="text" bind:value={searchDocNo} placeholder="GB/T 123…"
									onkeydown={(e) => { if (e.key === 'Enter') runSearch(); }}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1" style="grid-column:span 4;">
								<span style="font-size:11px; color:{textMuted};">File name contains</span>
								<input type="text" bind:value={searchFileName} placeholder="report, spec, drawing…"
									onkeydown={(e) => { if (e.key === 'Enter') runSearch(); }}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;" />
							</label>
						</div>
					</div>

					<!-- Processing Status section -->
					<div class="rounded-xl p-4" style="border:1px solid {borderColor}; background:{surface2};">
						<div style="font-size:10px; font-weight:700; letter-spacing:0.14em; text-transform:uppercase; color:{accent}; margin-bottom:8px;">Processing Status</div>
						<div class="grid gap-2" style="grid-template-columns:repeat(3,minmax(0,1fr));">
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Parser name</span>
								<input type="text" bind:value={searchParserName} placeholder="mineru, docling…"
									onkeydown={(e) => { if (e.key === 'Enter') runSearch(); }}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Operation</span>
								<input type="text" bind:value={searchOperation} placeholder="extract_metadata"
									onkeydown={(e) => { if (e.key === 'Enter') runSearch(); }}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Proc status</span>
								<select bind:value={searchProcStatus}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:13px;">
									{#each procStatusOptions as o}<option value={o}>{o}</option>{/each}
								</select>
							</label>
						</div>
					</div>

					<!-- Time Windows section -->
					<div class="rounded-xl p-4" style="border:1px solid {borderColor}; background:{surface2};">
						<div style="font-size:10px; font-weight:700; letter-spacing:0.14em; text-transform:uppercase; color:{accent}; margin-bottom:8px;">Time Windows</div>
						<div class="grid gap-2" style="grid-template-columns:repeat(4,minmax(0,1fr));">
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Create time from</span>
								<input type="datetime-local" bind:value={searchCreateStart}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:12px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Create time to</span>
								<input type="datetime-local" bind:value={searchCreateEnd}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:12px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Modify time from</span>
								<input type="datetime-local" bind:value={searchModifyStart}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:12px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Modify time to</span>
								<input type="datetime-local" bind:value={searchModifyEnd}
									style="height:32px; border:1px solid {borderColor}; background:{cardBg}; color:{textPrimary}; border-radius:8px; padding:0 8px; font-size:12px;" />
							</label>
						</div>
					</div>

					<!-- Toolbar -->
					<div class="flex items-center justify-between gap-3">
						<div style="font-size:12px; color:{textSecondary};">Leave fields empty to broaden the search. Results capped to 50 records.</div>
						<div class="flex items-center gap-2">
							<button
								onclick={() => {
									searchRecordId = ''; searchTitle = ''; searchDocNo = ''; searchFileName = '';
									searchDocType = 'all'; searchParserName = ''; searchOperation = '';
									searchProcStatus = 'all'; searchCreateStart = ''; searchCreateEnd = '';
									searchModifyStart = ''; searchModifyEnd = '';
									searchResults = []; searchSelected = null; searchError = '';
								}}
								style="height:34px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:13px; cursor:pointer;"
							>Reset</button>
							<button
								onclick={runSearch}
								disabled={searchLoading}
								style="height:34px; min-width:120px; padding:0 16px; border:none; border-radius:8px; background:{accent}; color:white; font-size:13px; font-weight:600; cursor:pointer; opacity:{searchLoading ? 0.6 : 1};"
							>{searchLoading ? 'Searching…' : 'Search'}</button>
						</div>
					</div>
				</div>

				{#if searchError}
					<div style="margin:0 20px 12px; padding:10px 14px; border-radius:8px; background:rgba(239,68,68,0.12); border:1px solid rgba(239,68,68,0.3); color:#ef4444; font-size:13px;">{searchError}</div>
				{/if}

				<!-- Results -->
				<div class="flex-1 overflow-auto" style="min-height:160px; border-top:1px solid {borderColor};">
					{#if searchResults.length === 0 && !searchLoading}
						<div class="flex flex-col items-center justify-center h-full py-12" style="color:{textMuted};">
							<div style="font-size:2rem; margin-bottom:8px; opacity:0.35;">⌕</div>
							<div style="font-size:14px; font-weight:600; color:{textSecondary}; margin-bottom:4px;">Run a search to see records.</div>
							<div style="font-size:12px;">Use any combination of filters to narrow the list.</div>
						</div>
					{:else}
						<table style="width:100%; border-collapse:collapse; font-size:12px;">
							<thead style="position:sticky; top:0; background:{cardBg}; z-index:1;">
								<tr style="border-bottom:1px solid {borderColor};">
									<th style="padding:8px 12px; text-align:left; font-size:10px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted}; white-space:nowrap;">ID</th>
									<th style="padding:8px 12px; text-align:left; font-size:10px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted};">Type</th>
									<th style="padding:8px 12px; text-align:left; font-size:10px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted};">Title / Doc No</th>
									<th style="padding:8px 12px; text-align:left; font-size:10px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted};">File name</th>
									<th style="padding:8px 12px; text-align:left; font-size:10px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted};">Parser</th>
									<th style="padding:8px 12px; text-align:left; font-size:10px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted};">Status</th>
									<th style="padding:8px 12px; text-align:left; font-size:10px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; color:{textMuted}; white-space:nowrap;">Created</th>
								</tr>
							</thead>
							<tbody>
								{#each searchResults as r (r.id)}
									{@const statusSummary = searchStatusText(r)}
									<tr
										onclick={() => (searchSelected = r.id)}
										ondblclick={() => pickSearchResult(r)}
										style="
											border-bottom:1px solid {borderColor};
											background:{searchSelected === r.id ? accent + '18' : 'transparent'};
											cursor:pointer;
											outline:{searchSelected === r.id ? '1px solid ' + accent + '44' : 'none'};
										"
										onmouseenter={(e) => { if (searchSelected !== r.id) (e.currentTarget as HTMLElement).style.background = surface2; }}
										onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = searchSelected === r.id ? accent + '18' : 'transparent'; }}
									>
										<td style="padding:8px 12px; font-family:monospace; color:{textPrimary}; white-space:nowrap;">{r.id}</td>
										<td style="padding:8px 12px; color:{textSecondary};">{r.type}</td>
										<td style="padding:8px 12px; max-width:220px;">
											<div style="font-weight:500; color:{textPrimary}; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{recordDisplayName(r)}</div>
											<div style="font-size:11px; font-family:monospace; color:{textMuted}; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{recordDisplayDocNo(r)}</div>
										</td>
										<td style="padding:8px 12px; color:{textSecondary}; max-width:160px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{r.file_name ?? '—'}</td>
										<td style="padding:8px 12px; font-family:monospace; color:{textMuted};">{r.parser_name || '—'}</td>
										<td style="padding:8px 12px;">
											<div style="display:flex; flex-direction:column; gap:2px;">
												<span style="font-family:monospace; font-size:11px; color:{textSecondary};">{statusSummary.operation}</span>
												<span style="
													font-size:10px; font-weight:600; padding:1px 6px; border-radius:4px; width:fit-content;
													background:{statusSummary.procStatus.toLowerCase() === 'success' ? 'rgba(34,197,94,0.15)' : statusSummary.procStatus.toLowerCase().startsWith('fail') ? 'rgba(239,68,68,0.15)' : surface2};
													color:{statusSummary.procStatus.toLowerCase() === 'success' ? '#22c55e' : statusSummary.procStatus.toLowerCase().startsWith('fail') ? '#ef4444' : textMuted};
												">{statusSummary.procStatus}</span>
											</div>
										</td>
										<td style="padding:8px 12px; font-family:monospace; color:{textMuted}; white-space:nowrap;">{(r.create_time ?? '').slice(0, 19).replace('T', ' ')}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>
			</div>

			<!-- Footer -->
			<div class="flex-shrink-0 flex items-center justify-between px-5 py-3" style="border-top:1px solid {borderColor};">
				<div style="font-size:12px; color:{textMuted};">Double-click a row, or select &amp; press Apply Filters.</div>
				<div class="flex items-center gap-2">
					<button
						onclick={closeSearch}
						style="height:34px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:13px; cursor:pointer;"
					>Cancel</button>
					<button
						onclick={confirmSearchSelection}
						disabled={searchSelected == null}
						style="height:34px; min-width:120px; padding:0 16px; border:none; border-radius:8px; background:{accent}; color:white; font-size:13px; font-weight:600; cursor:pointer; opacity:{searchSelected == null ? 0.45 : 1};"
					>Apply Filters</button>
				</div>
			</div>
		</div>
	</div>
{/if}

{#if uploadDialogOpen}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:rgba(15,23,42,0.62);"
		onclick={closeUploadDialog}
		onkeydown={(e) => {
			if (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') closeUploadDialog();
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
			aria-label="Upload files dialog"
			tabindex="0"
		>
			<div class="flex items-center justify-between px-4 py-3" style="border-bottom:1px solid {borderColor};">
				<div>
					<h3 style="font-size:15px; font-weight:600; color:{textPrimary};">Upload File Dialog</h3>
					<div style="margin-top:4px; font-size:12px; color:{textMuted};">
						Active Knowledge Store:
						{#if knowledgeStoreState.activeStore}
							<span style="color:{textPrimary};">{knowledgeStoreState.activeStore.ks_name}</span>
							<span class="mono" style="margin-left:8px;">ID {knowledgeStoreState.activeStore.id}</span>
						{:else}
							<span style="color:#ef4444;">None selected</span>
						{/if}
					</div>
				</div>
				<button
					onclick={closeUploadDialog}
					style="height:30px; padding:0 12px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer;"
				>
					Close
				</button>
			</div>

			<div class="p-4">
				<div class="grid gap-3" style="grid-template-columns: repeat(2, minmax(0, 1fr));">
					<label class="flex flex-col gap-1.5">
						<span style="font-size:12px; color:{textMuted};">Type</span>
						<select
							bind:value={uploadType}
							style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
						>
							{#each docTypeOptions.filter((option) => option !== 'all') as option}
								<option value={option}>{option}</option>
							{/each}
						</select>
					</label>

					<label class="flex flex-col gap-1.5">
						<span style="font-size:12px; color:{textMuted};">Parser Name</span>
						<select
							bind:value={uploadParserName}
							style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
						>
							{#each uploadParserOptions as option}
								<option value={option}>{option}</option>
							{/each}
						</select>
					</label>

					<label class="flex flex-col gap-1.5">
						<span style="font-size:12px; color:{textMuted};">Title</span>
						<input bind:value={uploadTitle} type="text" style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;" />
					</label>

					<label class="flex flex-col gap-1.5">
						<span style="font-size:12px; color:{textMuted};">Doc No</span>
						<input bind:value={uploadDocNo} type="text" style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;" />
					</label>

					<label class="flex flex-col gap-1.5">
						<span style="font-size:12px; color:{textMuted};">Authors</span>
						<input bind:value={uploadAuthors} type="text" style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;" />
					</label>

					<label class="flex flex-col gap-1.5">
						<span style="font-size:12px; color:{textMuted};">Public Info</span>
						<input bind:value={uploadPublicInfo} type="text" style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;" />
					</label>

					<label class="flex flex-col gap-1.5">
						<span style="font-size:12px; color:{textMuted};">Private Info</span>
						<input bind:value={uploadPrivateInfo} type="text" style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;" />
					</label>

					<div class="rounded-lg p-3" style="border:1px solid {borderColor}; background:{surface2};">
						<div style="font-size:12px; color:{textMuted}; margin-bottom:6px;">Selected Files</div>
						{#if selectedFiles.length === 0}
							<div style="font-size:12px; color:{textSecondary};">No files selected yet.</div>
						{:else}
							<div class="space-y-1" style="max-height:90px; overflow:auto;">
								{#each selectedFiles as file}
									<div style="font-size:12px; color:{textPrimary};">{file.name}</div>
								{/each}
							</div>
						{/if}
					</div>

					<label class="flex flex-col gap-1.5" style="grid-column: 1 / -1;">
						<span style="font-size:12px; color:{textMuted};">Notes</span>
						<textarea bind:value={uploadNotes} rows="3" style="border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:10px;"></textarea>
					</label>

					<label class="flex flex-col gap-1.5" style="grid-column: 1 / -1;">
						<span style="font-size:12px; color:{textMuted};">ks_desc</span>
						<textarea bind:value={uploadKsDesc} rows="3" style="border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:10px;"></textarea>
					</label>
				</div>

				<input bind:this={filePicker} type="file" multiple class="hidden" onchange={onFileSelection} />

				<div class="mt-4 flex items-center gap-2">
					<button
						onclick={triggerFilePicker}
						type="button"
						style="height:36px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textPrimary}; font-size:13px; cursor:pointer;"
					>
						Browse and Pick Files
					</button>
					<button
						onclick={submitUpload}
						disabled={uploadSubmitting}
						type="button"
						style="height:36px; padding:0 14px; border:none; border-radius:8px; background:{accent}; color:white; font-size:13px; font-weight:600; cursor:pointer; opacity:{uploadSubmitting ? 0.65 : 1};"
					>
						{uploadSubmitting ? 'Uploading…' : 'Upload Files'}
					</button>
					{#if uploadError}
						<span style="font-size:12px; color:#ef4444;">{uploadError}</span>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}

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

			<div class="grid gap-4 p-4" style="grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items:stretch;">
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

				<div
					class="rounded-lg p-3 flex flex-col"
					style="border:1px solid {borderColor}; background:{surface2}; min-height:0;"
				>
					<div style="font-size:12px; font-weight:600; color:{textSecondary}; margin-bottom:8px;">Raw JSON</div>
					<pre
						bind:this={statusDialogRawJsonEl}
						style="margin:0; flex:1; min-height:0; overflow-x:hidden; overflow-y:{statusDialogHasOverflow ? 'auto' : 'hidden'}; white-space:pre-wrap; word-break:break-word; font-size:12px; color:{textPrimary};"
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
