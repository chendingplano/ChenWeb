<script lang="ts">
	import { onMount, tick } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import SquareIcon from '@lucide/svelte/icons/square';
	import CheckSquareIcon from '@lucide/svelte/icons/check-square';
	import { shouldShowOverflowScrollbar } from '$lib/components/home3/kb-import-status-dialog.js';
	import { knowledgeStoreState } from '$lib/components/home3/knowledge-store-state.svelte';
	import type { KbInputRecord, ParseState } from '$lib/services/kbService';
	import { listKbInputs, uploadKbInputs, deleteKbInput, checkKbInputMD5s, getKbFrontendConfig } from '$lib/services/kbService';
	import KbInputSearchDialog from '$lib/components/home3/kb-input-search-dialog.svelte';
	import { createDefaultRecordBrowserFilters } from '$lib/components/home3/topic-tree-record-browser.js';
	import {
		ALL_CONFIGURABLE_PROCESSOR_IDS,
		ALL_PROCESSOR_IDS,
		MANDATORY_DISPLAY_STAGES,
		MANDATORY_PROCESSOR_IDS,
		PIPELINE_STAGES,
		computeStages
	} from './doc-processor-dashboard-state';

	let { darkMode = true }: { darkMode: boolean } = $props();

	const docTypeOptions = [
		'all',
		'pdf',
		'zip',
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
	// Extra list filters applied from the search dialog.
	let recordId = $state('');
	let title = $state('');
	let docNo = $state('');
	let parserName = $state('');
	let pipelineFilter = $state('');
	let procStatus = $state('all');
	let modifyStartTime = $state('');
	let modifyEndTime = $state('');
	let page = $state(1);
	let pageSize = $state(50);
	let sortField = $state('create_time');
	let sortDir = $state<'asc' | 'desc'>('desc');
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
	// Delete confirmation
	let deleteConfirmRecord = $state<KbInputRecord | null>(null);
	let deleteError = $state('');
	let deleteSubmitting = $state(false);

	// ── Restart pipeline dialog (mirrors the doc-processor dashboard) ──────
	// requiredProcessors: the configurable processors enabled in config.toml.
	let requiredProcessors = $state<string[]>(ALL_CONFIGURABLE_PROCESSOR_IDS);
	let selectableProcessorIds = $derived([...MANDATORY_PROCESSOR_IDS, ...requiredProcessors]);
	let CONFIGURABLE_PROCESSORS = $derived(
		selectableProcessorIds
			.map((id) => PIPELINE_STAGES.find((s) => s.id === id))
			.filter((s) => s !== undefined)
	);

	let restartTarget = $state<KbInputRecord | null>(null);
	let restartProcessors = $state<Record<string, boolean>>(
		Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, true]))
	);
	let restartParseFile = $state(false);
	let restartConvert = $state(false);
	let showRestartDialog = $state(false);
	let restarting = $state(false);
	let restartError = $state('');
	let restartToast = $state<{ kind: 'success' | 'error'; msg: string } | null>(null);

	let uploadType = $state('');
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
	let dirPicker = $state<HTMLInputElement | null>(null);
	let uploadRecursive = $state(true);
	let uploadDirProcessing = $state(false);
	let uploadSkippedCount = $state(0);

	const typeExtensions: Record<string, string[]> = {
		pdf: ['.pdf'],
		doc: ['.doc', '.docx'],
		excel: ['.xls', '.xlsx'],
		ppt: ['.ppt', '.pptx'],
		text: ['.txt'],
		json: ['.json'],
		xml: ['.xml'],
		markdown: ['.md', '.markdown'],
		typst: ['.typ'],
		zip: ['.zip']
	};

	function fileMD5Hex(buffer: ArrayBuffer): string {
		const bytes = new Uint8Array(buffer);
		function safeAdd(x: number, y: number): number {
			const lsw = (x & 0xffff) + (y & 0xffff);
			return (((x >> 16) + (y >> 16) + (lsw >> 16)) << 16) | (lsw & 0xffff);
		}
		function rol(n: number, s: number): number { return (n << s) | (n >>> (32 - s)); }
		function cmn(q: number, a: number, b: number, x: number, s: number, t: number): number {
			return safeAdd(rol(safeAdd(safeAdd(a, q), safeAdd(x, t)), s), b);
		}
		function ff(a: number, b: number, c: number, d: number, x: number, s: number, t: number) { return cmn((b & c) | (~b & d), a, b, x, s, t); }
		function gg(a: number, b: number, c: number, d: number, x: number, s: number, t: number) { return cmn((b & d) | (c & ~d), a, b, x, s, t); }
		function hh(a: number, b: number, c: number, d: number, x: number, s: number, t: number) { return cmn(b ^ c ^ d, a, b, x, s, t); }
		function ii(a: number, b: number, c: number, d: number, x: number, s: number, t: number) { return cmn(c ^ (b | ~d), a, b, x, s, t); }

		const len8 = bytes.length;
		const len32 = Math.ceil((len8 + 9) / 64) * 16;
		const w = new Int32Array(len32);
		for (let i = 0; i < len8; i++) w[i >> 2] |= bytes[i] << ((i % 4) * 8);
		w[len8 >> 2] |= 0x80 << ((len8 % 4) * 8);
		w[len32 - 2] = len8 * 8;

		let a = 1732584193, b = -271733879, c = -1732584194, d = 271733878;
		for (let i = 0; i < len32; i += 16) {
			const [aa, bb, cc, dd] = [a, b, c, d];
			a=ff(a,b,c,d,w[i+0],7,-680876936); d=ff(d,a,b,c,w[i+1],12,-389564586); c=ff(c,d,a,b,w[i+2],17,606105819); b=ff(b,c,d,a,w[i+3],22,-1044525330);
			a=ff(a,b,c,d,w[i+4],7,-176418897); d=ff(d,a,b,c,w[i+5],12,1200080426); c=ff(c,d,a,b,w[i+6],17,-1473231341); b=ff(b,c,d,a,w[i+7],22,-45705983);
			a=ff(a,b,c,d,w[i+8],7,1770035416); d=ff(d,a,b,c,w[i+9],12,-1958414417); c=ff(c,d,a,b,w[i+10],17,-42063); b=ff(b,c,d,a,w[i+11],22,-1990404162);
			a=ff(a,b,c,d,w[i+12],7,1804603682); d=ff(d,a,b,c,w[i+13],12,-40341101); c=ff(c,d,a,b,w[i+14],17,-1502002290); b=ff(b,c,d,a,w[i+15],22,1236535329);
			a=gg(a,b,c,d,w[i+1],5,-165796510); d=gg(d,a,b,c,w[i+6],9,-1069501632); c=gg(c,d,a,b,w[i+11],14,643717713); b=gg(b,c,d,a,w[i+0],20,-373897302);
			a=gg(a,b,c,d,w[i+5],5,-701558691); d=gg(d,a,b,c,w[i+10],9,38016083); c=gg(c,d,a,b,w[i+15],14,-660478335); b=gg(b,c,d,a,w[i+4],20,-405537848);
			a=gg(a,b,c,d,w[i+9],5,568446438); d=gg(d,a,b,c,w[i+14],9,-1019803690); c=gg(c,d,a,b,w[i+3],14,-187363961); b=gg(b,c,d,a,w[i+8],20,1163531501);
			a=gg(a,b,c,d,w[i+13],5,-1444681467); d=gg(d,a,b,c,w[i+2],9,-51403784); c=gg(c,d,a,b,w[i+7],14,1735328473); b=gg(b,c,d,a,w[i+12],20,-1926607734);
			a=hh(a,b,c,d,w[i+5],4,-378558); d=hh(d,a,b,c,w[i+8],11,-2022574463); c=hh(c,d,a,b,w[i+11],16,1839030562); b=hh(b,c,d,a,w[i+14],23,-35309556);
			a=hh(a,b,c,d,w[i+1],4,-1530992060); d=hh(d,a,b,c,w[i+4],11,1272893353); c=hh(c,d,a,b,w[i+7],16,-155497632); b=hh(b,c,d,a,w[i+10],23,-1094730640);
			a=hh(a,b,c,d,w[i+13],4,681279174); d=hh(d,a,b,c,w[i+0],11,-358537222); c=hh(c,d,a,b,w[i+3],16,-722521979); b=hh(b,c,d,a,w[i+6],23,76029189);
			a=hh(a,b,c,d,w[i+9],4,-640364487); d=hh(d,a,b,c,w[i+12],11,-421815835); c=hh(c,d,a,b,w[i+15],16,530742520); b=hh(b,c,d,a,w[i+2],23,-995338651);
			a=ii(a,b,c,d,w[i+0],6,-198630844); d=ii(d,a,b,c,w[i+7],10,1126891415); c=ii(c,d,a,b,w[i+14],15,-1416354905); b=ii(b,c,d,a,w[i+5],21,-57434055);
			a=ii(a,b,c,d,w[i+12],6,1700485571); d=ii(d,a,b,c,w[i+3],10,-1894986606); c=ii(c,d,a,b,w[i+10],15,-1051523); b=ii(b,c,d,a,w[i+1],21,-2054922799);
			a=ii(a,b,c,d,w[i+8],6,1873313359); d=ii(d,a,b,c,w[i+15],10,-30611744); c=ii(c,d,a,b,w[i+6],15,-1560198380); b=ii(b,c,d,a,w[i+13],21,1309151649);
			a=ii(a,b,c,d,w[i+4],6,-145523070); d=ii(d,a,b,c,w[i+11],10,-1120210379); c=ii(c,d,a,b,w[i+2],15,718787259); b=ii(b,c,d,a,w[i+9],21,-343485551);
			a=safeAdd(a,aa); b=safeAdd(b,bb); c=safeAdd(c,cc); d=safeAdd(d,dd);
		}
		return [a, b, c, d].map(n =>
			[(n&0xff),((n>>8)&0xff),((n>>16)&0xff),((n>>24)&0xff)]
				.map(v => v.toString(16).padStart(2,'0')).join('')
		).join('');
	}

	async function computeFileMD5(file: File): Promise<string> {
		const buf = await file.arrayBuffer();
		return fileMD5Hex(buf);
	}

	let totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let surface3 = $derived(darkMode ? '#1A1E2C' : '#F8F9FB');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let colorSuccess = $derived(darkMode ? '#34D399' : '#10B981');
	let colorError = $derived(darkMode ? '#F87171' : '#EF4444');

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

	function typeFromExtension(filename: string): string {
		const lower = filename.toLowerCase();
		for (const [type, exts] of Object.entries(typeExtensions)) {
			if (exts.some(e => lower.endsWith(e))) return type;
		}
		return '';
	}

	function resetUploadDialog() {
		uploadError = '';
		uploadType = '';
		uploadTitle = '';
		uploadDocNo = '';
		uploadAuthors = '';
		uploadPublicInfo = '';
		uploadPrivateInfo = '';
		uploadNotes = '';
		uploadKsDesc = '';
		uploadParserName = 'opendata';		// The default PDF parser
		selectedFiles = [];
		uploadSkippedCount = 0;
		uploadDirProcessing = false;
		if (filePicker) filePicker.value = '';
		if (dirPicker) dirPicker.value = '';
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

	function triggerDirPicker() {
		dirPicker?.click();
	}

	function onFileSelection(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		selectedFiles = Array.from(input.files ?? []);
		uploadSkippedCount = 0;
		if (selectedFiles.length === 1) {
			uploadType = typeFromExtension(selectedFiles[0].name);
		} else {
			uploadType = '';
		}
	}

	async function onDirSelection(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const all = Array.from(input.files ?? []);
		if (all.length === 0) return;

		let candidates = all.filter(f => typeFromExtension(f.name) !== '');

		if (!uploadRecursive) {
			candidates = candidates.filter(f => {
				const rel = (f as File & { webkitRelativePath?: string }).webkitRelativePath ?? f.name;
				return (rel.match(/\//g) ?? []).length <= 1;
			});
		}

		if (candidates.length === 0) {
			selectedFiles = [];
			uploadSkippedCount = 0;
			uploadError = 'No files with recognized extensions found in the selected directory.';
			return;
		}

		uploadType = '';

		uploadDirProcessing = true;
		uploadError = '';
		try {
			const md5List = await Promise.all(candidates.map(computeFileMD5));
			const existing = await checkKbInputMD5s(md5List);
			const kept: File[] = [];
			let skipped = 0;
			for (let i = 0; i < candidates.length; i++) {
				if (existing.has(md5List[i])) {
					skipped++;
				} else {
					kept.push(candidates[i]);
				}
			}
			selectedFiles = kept;
			uploadSkippedCount = skipped;
		} catch {
			selectedFiles = candidates;
			uploadSkippedCount = 0;
		} finally {
			uploadDirProcessing = false;
		}
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

		const commonPayload = {
			title: uploadTitle,
			doc_no: uploadDocNo,
			authors: uploadAuthors,
			public_info: uploadPublicInfo,
			private_info: uploadPrivateInfo,
			notes: uploadNotes,
			ks_desc: uploadKsDesc,
			parser_name: uploadParserName,
			ks_store_id: activeStore.id,
			tenant_id: activeStore.tenant_id
		};

		uploadSubmitting = true;
		try {
			if (selectedFiles.length === 1) {
				if (!uploadType) {
					uploadError = 'Please select a file type from the Type dropdown.';
					uploadSubmitting = false;
					return;
				}
				await uploadKbInputs({ ...commonPayload, type: uploadType, files: selectedFiles });
			} else {
				// Group by type derived from each file's extension; skip unrecognized
				const groups = new Map<string, File[]>();
				for (const file of selectedFiles) {
					const t = typeFromExtension(file.name);
					if (!t) continue;
					if (!groups.has(t)) groups.set(t, []);
					groups.get(t)!.push(file);
				}
				if (groups.size === 0) {
					uploadError = 'No files with recognized extensions to upload.';
					uploadSubmitting = false;
					return;
				}
				for (const [t, files] of groups) {
					await uploadKbInputs({ ...commonPayload, type: t, files });
				}
			}
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
				recordId,
				title,
				docNo,
				parserName,
				pipelineFilter,
				procStatus: pipelineFilter.trim() ? '' : procStatus === 'all' ? '' : procStatus,
				modifyStartTime,
				modifyEndTime,
				page,
				pageSize,
				orderBy: sortField,
				orderDir: sortDir,
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

	function toggleSort(field: string) {
		if (sortField === field) {
			sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		} else {
			sortField = field;
			sortDir = 'asc';
		}
		page = 1;
		loadRecords();
	}

	type RecordBrowserFilters = ReturnType<typeof createDefaultRecordBrowserFilters>;

	// Apply the filters chosen in the search dialog to the main list and reload.
	function applySearchFilters(_records: KbInputRecord[], filters: RecordBrowserFilters) {
		searchOpen = false;
		recordId = filters.searchRecordId ?? '';
		title = filters.searchTitle ?? '';
		docNo = filters.searchDocNo ?? '';
		fileName = filters.searchFileName ?? '';
		docType = filters.searchDocType ?? 'all';
		parserName = filters.searchParserName ?? '';
		pipelineFilter = filters.searchOperation ?? '';
		procStatus = filters.searchProcStatus ?? 'all';
		startTime = filters.searchCreateStart ?? '';
		endTime = filters.searchCreateEnd ?? '';
		modifyStartTime = filters.searchModifyStart ?? '';
		modifyEndTime = filters.searchModifyEnd ?? '';
		parseState = 'all';
		page = 1;
		loadRecords();
	}

	function openDeleteConfirm(record: KbInputRecord) {
		deleteConfirmRecord = record;
		deleteError = '';
		deleteSubmitting = false;
	}

	function closeDeleteConfirm() {
		deleteConfirmRecord = null;
		deleteError = '';
		deleteSubmitting = false;
	}

	async function confirmDelete() {
		if (!deleteConfirmRecord) return;
		deleteSubmitting = true;
		deleteError = '';
		try {
			await deleteKbInput(deleteConfirmRecord.id);
			closeDeleteConfirm();
			await loadRecords();
		} catch (err) {
			deleteError = err instanceof Error ? err.message : 'Failed to delete record';
		} finally {
			deleteSubmitting = false;
		}
	}

	// ── Restart pipeline ──────────────────────────────────────────────────

	async function publishEvent(subject: string, payload: Record<string, unknown>) {
		const res = await fetch('/api/v1/jetstream/events', {
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ subject, payload: JSON.stringify(payload) })
		});
		if (!res.ok) {
			const body = await res.json().catch(() => null);
			throw new Error(body?.error_msg ?? body?.message ?? `Request failed (${res.status})`);
		}
	}

	async function doLaunch(
		record: KbInputRecord,
		procs: Record<string, boolean>,
		reParse: boolean,
		reConvert: boolean
	) {
		// Re-parse triggers the whole downstream chain automatically (in auto mode).
		// Re-convert triggers convert + downstream doc processing.
		// Both supersede a standalone doc-processor publish to avoid double-processing.
		if (reParse) {
			await publishEvent('kb.pdf.staged', { record_id: String(record.id), type: record.type ?? 'pdf', status: 'success', force: true });
			return;
		}
		if (reConvert) {
			await publishEvent('kb.pdf.parsed', { record_id: String(record.id), type: 'pdf', status: 'success', force: true });
			return;
		}
		const chosen = selectableProcessorIds.filter((p) => procs[p]);
		const allChosen = chosen.length === selectableProcessorIds.length;
		const payload: Record<string, unknown> = { record_id: String(record.id), force: true };
		if (!allChosen) payload.operation = chosen;
		await publishEvent('kb.line-file-generated', payload);
	}

	function getDefaultRestartProcessors(record: KbInputRecord): Record<string, boolean> {
		const unfinishedStageIds = new Set(
			computeStages(record)
				.filter((stage) => stage.status !== 'success' && ALL_PROCESSOR_IDS.includes(stage.id))
				.map((stage) => stage.id)
		);

		if (!unfinishedStageIds.size) {
			return Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, selectableProcessorIds.includes(p)]));
		}

		return Object.fromEntries(ALL_PROCESSOR_IDS.map((p) => [p, unfinishedStageIds.has(p)]));
	}

	function openRestart(record: KbInputRecord) {
		restartTarget = record;
		restartProcessors = getDefaultRestartProcessors(record);
		restartParseFile = false;
		restartConvert = false;
		restartError = '';
		showRestartDialog = true;
	}

	async function confirmRestart() {
		if (!restartTarget) return;
		restarting = true;
		restartError = '';
		try {
			await doLaunch(restartTarget, restartProcessors, restartParseFile, restartConvert);
			showRestartDialog = false;
			restartTarget = null;
			restartToast = { kind: 'success', msg: 'Restart triggered' };
			setTimeout(() => { restartToast = null; }, 4000);
		} catch (err) {
			restartError = err instanceof Error ? err.message : 'Restart failed';
		} finally {
			restarting = false;
		}
	}

	function allRestartSelected(): boolean {
		return selectableProcessorIds.every((p) => restartProcessors[p]);
	}

	function toggleAllRestart() {
		const next = !allRestartSelected();
		restartProcessors = Object.fromEntries(selectableProcessorIds.map((p) => [p, next]));
	}

	let pageJumpInput = $state('1');

	$effect(() => {
		pageJumpInput = String(page);
	});

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

	function goToPage(target: number) {
		const clamped = Math.max(1, Math.min(totalPages, target));
		if (clamped === page) return;
		page = clamped;
		loadRecords();
	}

	function jumpToPage() {
		const parsed = parseInt(pageJumpInput, 10);
		if (!Number.isFinite(parsed)) {
			pageJumpInput = String(page);
			return;
		}
		goToPage(parsed);
	}

	function changePageSize(newSize: number) {
		pageSize = newSize;
		page = 1;
		loadRecords();
	}

	let visiblePageNumbers = $derived.by(() => {
		const delta = 2;
		const pages: Array<number | null> = [];
		const left = Math.max(1, page - delta);
		const right = Math.min(totalPages, page + delta);

		if (left > 1) {
			pages.push(1);
			if (left > 2) pages.push(null);
		}
		for (let i = left; i <= right; i++) pages.push(i);
		if (right < totalPages) {
			if (right < totalPages - 1) pages.push(null);
			pages.push(totalPages);
		}
		return pages;
	});

	let recordRangeStart = $derived((page - 1) * pageSize + 1);
	let recordRangeEnd = $derived(Math.min(page * pageSize, total));

	onMount(() => {
		loadRecords();
		getKbFrontendConfig()
			.then((cfg) => {
				requiredProcessors = cfg.required_processors ?? ALL_CONFIGURABLE_PROCESSOR_IDS;
			})
			.catch(() => {
				// Keep defaults on failure
			});
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

<div class="kb-import-root" style="background:{pageBg};">
	<div
		class="rounded-xl p-6 kb-import-header"
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
					onclick={() => { searchOpen = true; }}
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
		class="rounded-xl overflow-hidden kb-import-table-card"
		style="background:{cardBg}; border:1px solid {borderColor};"
	>
		<div class="kb-import-table-scroll">
			{#snippet sortHead(label: string, field: string)}
				<th class="cell head">
					<button
						type="button"
						onclick={() => toggleSort(field)}
						title="Sort by {label}"
						style="display:inline-flex; align-items:center; gap:4px; background:none; border:none; padding:0; margin:0; font:inherit; color:inherit; cursor:pointer; white-space:nowrap;"
					>
						<span>{label}</span>
						<span style="font-size:10px; line-height:1; opacity:{sortField === field ? 1 : 0.3}; color:{sortField === field ? accent : 'inherit'};">
							{sortField === field ? (sortDir === 'asc' ? '▲' : '▼') : '↕'}
						</span>
					</button>
				</th>
			{/snippet}
			<table style="width:100%; border-collapse:collapse; min-width:960px;">
				<thead style="background:{pageBg};">
					<tr>
						{@render sortHead('ID', 'id')}
						{@render sortHead('Title', 'title')}
						{@render sortHead('Doc No', 'doc_no')}
						{@render sortHead('Type', 'type')}
						{@render sortHead('File Name', 'file_name')}
						{@render sortHead('Parser', 'parser_name')}
						<th class="cell head">Parsing</th>
						<th class="cell head">Time</th>
						<th class="cell head">Convert</th>
						<th class="cell head">Time</th>
						{@render sortHead('Create Time', 'create_time')}
						{@render sortHead('Modify Time', 'modify_time')}
						<th class="cell head">Status</th>
					<th class="cell head">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#if !loading && records.length === 0}
						<tr>
							<td class="cell" colspan={14} style="text-align:center; color:{textMuted};">No records</td>
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
								<td class="cell">
									<div class="flex items-center gap-2">
										<button
											onclick={() => openRestart(record)}
											style="display:inline-flex; align-items:center; gap:4px; height:28px; padding:0 10px; border:1px solid {accent}40; border-radius:8px; background:{accentTint}; color:{accent}; font-size:12px; cursor:pointer;"
										>
											<RefreshCwIcon class="h-3.5 w-3.5" />
											Restart
										</button>
										<button
											onclick={() => openDeleteConfirm(record)}
											style="height:28px; padding:0 10px; border:1px solid rgba(239,68,68,0.4); border-radius:8px; background:rgba(239,68,68,0.1); color:#ef4444; font-size:12px; cursor:pointer;"
										>
											Delete
										</button>
									</div>
								</td>
							</tr>
						{/each}
					{/if}
				</tbody>
			</table>
		</div>

		<div
			class="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
			style="border-top:1px solid {borderColor}; background:{pageBg};"
		>
			<!-- Left: range + page size -->
			<div class="flex items-center gap-3" style="font-size:12px; color:{textMuted};">
				{#if total > 0}
					<span>
						{recordRangeStart}–{recordRangeEnd} of {total}
					</span>
				{:else}
					<span>0 records</span>
				{/if}
				<label class="flex items-center gap-1">
					<span>Per page:</span>
					<select
						value={pageSize}
						onchange={(e) => changePageSize(Number((e.currentTarget as HTMLSelectElement).value))}
						style="height:26px; border:1px solid {borderColor}; border-radius:6px; background:{surface2}; color:{textSecondary}; font-size:12px; padding:0 4px; cursor:pointer;"
					>
						{#each [20, 50, 100, 200] as n}
							<option value={n}>{n}</option>
						{/each}
					</select>
				</label>
			</div>

			<!-- Center: page buttons -->
			<div class="flex items-center gap-1">
				<button
					onclick={prevPage}
					disabled={page <= 1 || loading}
					style="height:28px; min-width:28px; padding:0 8px; border:1px solid {borderColor}; border-radius:6px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer; opacity:{page <= 1 || loading ? 0.4 : 1};"
				>‹</button>

				{#each visiblePageNumbers as p}
					{#if p === null}
						<span style="padding:0 4px; color:{textMuted}; font-size:12px;">…</span>
					{:else}
						<button
							onclick={() => goToPage(p)}
							disabled={loading}
							style="height:28px; min-width:28px; padding:0 6px; border:1px solid {p === page ? accent : borderColor}; border-radius:6px; background:{p === page ? accent + '22' : surface2}; color:{p === page ? accent : textSecondary}; font-size:12px; font-weight:{p === page ? 700 : 400}; cursor:pointer; opacity:{loading ? 0.6 : 1};"
						>{p}</button>
					{/if}
				{/each}

				<button
					onclick={nextPage}
					disabled={page >= totalPages || loading}
					style="height:28px; min-width:28px; padding:0 8px; border:1px solid {borderColor}; border-radius:6px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer; opacity:{page >= totalPages || loading ? 0.4 : 1};"
				>›</button>
			</div>

			<!-- Right: jump to page -->
			<div class="flex items-center gap-1" style="font-size:12px; color:{textMuted};">
				<span>Go to</span>
				<input
					type="number"
					min="1"
					max={totalPages}
					bind:value={pageJumpInput}
					onkeydown={(e) => { if (e.key === 'Enter') jumpToPage(); }}
					style="width:52px; height:26px; border:1px solid {borderColor}; border-radius:6px; background:{surface2}; color:{textSecondary}; font-size:12px; padding:0 6px; text-align:center;"
				/>
				<button
					onclick={jumpToPage}
					disabled={loading}
					style="height:26px; padding:0 10px; border:1px solid {borderColor}; border-radius:6px; background:{surface2}; color:{textSecondary}; font-size:12px; cursor:pointer; opacity:{loading ? 0.5 : 1};"
				>Go</button>
			</div>
		</div>
	</div>
</div>

<KbInputSearchDialog
	bind:open={searchOpen}
	scopeToActiveStore={true}
	onSelect={applySearchFilters}
/>

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
							disabled={selectedFiles.length > 1}
							style="height:36px; border:1px solid {uploadType === '' && selectedFiles.length === 1 ? '#ef4444' : borderColor}; background:{surface2}; color:{uploadType ? textPrimary : textMuted}; border-radius:8px; padding:0 10px; opacity:{selectedFiles.length > 1 ? 0.5 : 1};"
						>
							<option value="" disabled>-- select type --</option>
							{#each docTypeOptions.filter((option) => option !== 'all') as option}
								<option value={option}>{option}</option>
							{/each}
						</select>
						{#if selectedFiles.length > 1}
							<span style="font-size:11px; color:{textMuted};">Determined by each file's extension</span>
						{/if}
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
						<div class="flex items-center justify-between" style="margin-bottom:6px;">
							<span style="font-size:12px; color:{textMuted};">Selected Files</span>
							{#if uploadSkippedCount > 0}
								<span style="font-size:11px; color:#f59e0b;">{uploadSkippedCount} skipped (duplicate MD5)</span>
							{/if}
						</div>
						{#if uploadDirProcessing}
							<div style="font-size:12px; color:{textSecondary};">Checking for duplicates…</div>
						{:else if selectedFiles.length === 0}
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
				<input bind:this={dirPicker} type="file" class="hidden" onchange={onDirSelection} webkitdirectory />

				<div class="mt-4 flex flex-wrap items-center gap-2">
					<button
						onclick={triggerFilePicker}
						type="button"
						style="height:36px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textPrimary}; font-size:13px; cursor:pointer;"
					>
						Browse and Pick Files
					</button>
					<button
						onclick={triggerDirPicker}
						type="button"
						disabled={uploadDirProcessing}
						style="height:36px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textPrimary}; font-size:13px; cursor:pointer; opacity:{uploadDirProcessing ? 0.6 : 1};"
					>
						Browse Directory
					</button>
					<label class="flex items-center gap-1.5" style="font-size:13px; color:{textSecondary}; cursor:pointer; user-select:none;">
						<input type="checkbox" bind:checked={uploadRecursive} style="width:14px; height:14px; accent-color:{accent}; cursor:pointer;" />
						Recursive
					</label>
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

{#if deleteConfirmRecord}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-6"
		style="background:rgba(15,23,42,0.72); backdrop-filter:blur(3px);"
		onclick={closeDeleteConfirm}
		onkeydown={(e) => { if (e.key === 'Escape') closeDeleteConfirm(); }}
		role="button"
		tabindex="0"
	>
		<div
			class="w-full max-w-md rounded-xl overflow-hidden"
			style="background:{cardBg}; border:1px solid {borderColor};"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-label="Delete confirmation"
			tabindex="0"
		>
			<div class="px-5 py-4" style="border-bottom:1px solid {borderColor};">
				<h3 style="font-size:15px; font-weight:600; color:{textPrimary};">Delete Record</h3>
			</div>
			<div class="px-5 py-4">
				<p style="font-size:14px; color:{textSecondary}; margin-bottom:8px;">
					Are you sure you want to delete record <span style="color:{textPrimary}; font-weight:600;">#{deleteConfirmRecord.id}</span>
					{#if deleteConfirmRecord.file_name}— <span style="color:{textPrimary};">{deleteConfirmRecord.file_name}</span>{/if}?
				</p>
				<p style="font-size:12px; color:#ef4444;">This action cannot be undone.</p>
				{#if deleteError}
					<div style="margin-top:10px; font-size:12px; color:#ef4444;">{deleteError}</div>
				{/if}
			</div>
			<div class="flex items-center justify-end gap-2 px-5 py-3" style="border-top:1px solid {borderColor};">
				<button
					onclick={closeDeleteConfirm}
					style="height:34px; padding:0 14px; border:1px solid {borderColor}; border-radius:8px; background:{surface2}; color:{textSecondary}; font-size:13px; cursor:pointer;"
				>Cancel</button>
				<button
					onclick={confirmDelete}
					disabled={deleteSubmitting}
					style="height:34px; padding:0 16px; border:none; border-radius:8px; background:#ef4444; color:white; font-size:13px; font-weight:600; cursor:pointer; opacity:{deleteSubmitting ? 0.6 : 1};"
				>{deleteSubmitting ? 'Deleting…' : 'Delete'}</button>
			</div>
		</div>
	</div>
{/if}

{#if restartToast}
	<div
		class="fixed right-6 top-6 z-50 flex items-center gap-2 rounded-xl px-4 py-3"
		style="background:{restartToast.kind === 'success' ? colorSuccess : colorError}; color:white; font-size:13px; font-weight:600; box-shadow:0 16px 40px rgba(0,0,0,0.35);"
	>
		{restartToast.msg}
	</div>
{/if}

{#if showRestartDialog && restartTarget}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-40 flex items-center justify-center"
		style="background:rgba(0,0,0,0.6); backdrop-filter:blur(4px);"
		onmousedown={(e) => { if (e.target === e.currentTarget) { showRestartDialog = false; restartError = ''; } }}
	>
		<div
			class="mx-4 w-full max-w-md rounded-2xl p-6"
			style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 24px 64px rgba(0,0,0,0.4);"
		>
			<h3 style="font-size:16px; font-weight:600; color:{textPrimary}; margin:0 0 4px;">Restart pipeline</h3>
			<p style="font-size:13px; color:{textSecondary}; margin:0 0 16px;">
				Record <span style="color:{accent}; font-family:monospace; font-weight:600;">#{restartTarget.id}</span>
				— select processors to re-run:
			</p>

			<div class="mb-4 space-y-1.5">
				<!-- Optional pre-processor: Parse File -->
				<label
					class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
					style="background:{restartParseFile ? accentTint : surface2}; border:1px solid {restartParseFile ? accent + '40' : borderColor};"
					onmouseenter={(e) => { if (!restartParseFile) (e.currentTarget as HTMLElement).style.background = surface3; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = restartParseFile ? accentTint : surface2; }}
				>
					<input type="checkbox" bind:checked={restartParseFile} class="sr-only" />
					{#if restartParseFile}
						<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
					{:else}
						<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
					{/if}
					<span style="font-size:13px; color:{restartParseFile ? textPrimary : textSecondary}; font-family:monospace;">parse_file</span>
					<span style="font-size:12px; color:{textMuted}; margin-left:6px;">Parse File</span>
				</label>

				<!-- Optional pre-processor: Convert Parse Result -->
				<label
					class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
					style="background:{restartConvert ? accentTint : surface2}; border:1px solid {restartConvert ? accent + '40' : borderColor};"
					onmouseenter={(e) => { if (!restartConvert) (e.currentTarget as HTMLElement).style.background = surface3; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = restartConvert ? accentTint : surface2; }}
				>
					<input type="checkbox" bind:checked={restartConvert} class="sr-only" />
					{#if restartConvert}
						<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
					{:else}
						<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
					{/if}
					<span style="font-size:13px; color:{restartConvert ? textPrimary : textSecondary}; font-family:monospace;">convert_parse_result</span>
					<span style="font-size:12px; color:{textMuted}; margin-left:6px;">Convert Parse Result</span>
				</label>

				<!-- Mandatory (always-on) processors -->
				{#each MANDATORY_DISPLAY_STAGES as proc}
					<label class="flex cursor-not-allowed items-center gap-2.5 rounded-lg px-3 py-2" style="background:{surface2}; border:1px solid {borderColor}; opacity:0.6;">
						<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{colorSuccess};" />
						<span style="font-size:13px; color:{textSecondary}; font-family:monospace;">{proc.id}</span>
						<span style="font-size:12px; color:{textMuted}; margin-left:6px;">{proc.label}</span>
						<span style="font-size:10px; color:{textMuted}; margin-left:auto; font-family:monospace;">mandatory</span>
					</label>
				{/each}

				<!-- Configurable processors -->
				{#each CONFIGURABLE_PROCESSORS as proc}
					<label
						class="flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2"
						style="background:{restartProcessors[proc.id] ? accentTint : surface2}; border:1px solid {restartProcessors[proc.id] ? accent + '40' : borderColor};"
						onmouseenter={(e) => { if (!restartProcessors[proc.id]) (e.currentTarget as HTMLElement).style.background = surface3; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = restartProcessors[proc.id] ? accentTint : surface2; }}
					>
						<input type="checkbox" bind:checked={restartProcessors[proc.id]} class="sr-only" />
						{#if restartProcessors[proc.id]}
							<CheckSquareIcon class="h-4 w-4 flex-shrink-0" style="color:{accent};" />
						{:else}
							<SquareIcon class="h-4 w-4 flex-shrink-0" style="color:{textMuted};" />
						{/if}
						<span style="font-size:13px; color:{restartProcessors[proc.id] ? textPrimary : textSecondary}; font-family:monospace;">{proc.id}</span>
						<span style="font-size:12px; color:{textMuted}; margin-left:6px;">{proc.label}</span>
					</label>
				{/each}
			</div>

			<div class="mb-4 flex">
				<button
					onclick={toggleAllRestart}
					class="rounded-lg px-3 py-1.5"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:12px; cursor:pointer;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; }}
				>{allRestartSelected() ? 'Deselect all' : 'Select all'}</button>
			</div>

			{#if restartError}
				<div style="font-size:12px; color:{colorError}; margin-bottom:12px;">{restartError}</div>
			{/if}

			<div class="flex justify-end gap-2">
				<button
					onclick={() => { showRestartDialog = false; restartError = ''; }}
					class="rounded-lg px-4 py-2"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:13px; cursor:pointer;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textSecondary; }}
				>Cancel</button>
				<button
					onclick={confirmRestart}
					disabled={restarting || !(restartParseFile || restartConvert || selectableProcessorIds.some(p => restartProcessors[p]))}
					class="flex items-center gap-2 rounded-lg px-4 py-2"
					style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none;
					       cursor:{restarting ? 'not-allowed' : 'pointer'};
					       opacity:{restarting || !(restartParseFile || restartConvert || selectableProcessorIds.some(p => restartProcessors[p])) ? '0.6' : '1'};"
					onmouseenter={(e) => {
						if (!restarting) (e.currentTarget as HTMLElement).style.opacity = '0.88';
					}}
					onmouseleave={(e) => {
						(e.currentTarget as HTMLElement).style.opacity = restarting ? '0.6' : '1';
					}}
				>
					<RefreshCwIcon class="h-4 w-4" />
					{restarting ? 'Restarting…' : 'Restart'}
				</button>
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

	.kb-import-root {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
		padding: 24px;
		gap: 16px;
		overflow: hidden;
	}

	.kb-import-header {
		flex-shrink: 0;
	}

	.kb-import-table-card {
		flex: 1;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}

	.kb-import-table-scroll {
		flex: 1;
		min-height: 0;
		overflow: auto;
	}
</style>
