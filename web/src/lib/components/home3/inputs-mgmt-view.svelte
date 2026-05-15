<script lang="ts">
	import {
		getKbInput,
		updateKbInput,
		getRawLines,
		type KbInputRecord,
		type RawLine
	} from '$lib/services/kbService';
	import EditableMetadataSection from '$lib/components/home3/editable-metadata-section.svelte';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import { KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE } from './kb-input-record-browser-settings.js';
	import {
		buildKbInputDocMetadataRows,
		buildKbInputRecordMetadataRows,
		buildKbInputUpdatePayloadForMetadataEdit,
		getKbInputDocMetadataValue
	} from './kb-input-metadata.js';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// ---------- Aesthetic tokens: "archival reading room" ----------
	let pageBg = $derived(darkMode ? '#0E1116' : '#F5F1E8');
	let panelBg = $derived(darkMode ? '#161A22' : '#FBF8F0');
	let panelBgAlt = $derived(darkMode ? '#1C212C' : '#F0EADB');
	let inkLine = $derived(darkMode ? '#2A3140' : '#D7CFB8');
	let inkLineSoft = $derived(darkMode ? '#1F2530' : '#E5DEC8');
	let textPrimary = $derived(darkMode ? '#EDE7D3' : '#1A1410');
	let textSecondary = $derived(darkMode ? '#B5AE94' : '#5C5345');
	let textMuted = $derived(darkMode ? '#7C7560' : '#8F8472');
	let brass = $derived(darkMode ? '#D4A24C' : '#B8801E');
	let brassFaint = $derived(darkMode ? 'rgba(212,162,76,0.12)' : 'rgba(184,128,30,0.10)');
	let crimson = $derived(darkMode ? '#C8553D' : '#A23E26');
	let crimsonFaint = $derived(darkMode ? 'rgba(200,85,61,0.18)' : 'rgba(162,62,38,0.12)');
	let teal = $derived(darkMode ? '#5DAFA8' : '#2D7B73');

	const fontSerif = "'Cormorant Garamond', 'Playfair Display', Georgia, serif";
	const fontMono = "'JetBrains Mono', 'IBM Plex Mono', monospace";
	const fontSans = "'Inter Tight', system-ui, sans-serif";

	// ---------- State ----------
	let listTotal = $state(0);
	let selectedInputId = $state<number | null>(null);
	let currentInput = $state<KbInputRecord | null>(null);
	let loading = $state(false);
	let errorMsg = $state('');

	let rawLines = $state<RawLine[]>([]);
	let rawLoading = $state(false);
	let rawError = $state('');

	// Document viewer state
	let viewMode = $state<'document' | 'source'>('document');
	let docPage = $state<number>(1);

	let fileUrl = $derived.by(() => {
		if (!currentInput) return '';
		return `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width`;
	});

	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	let isText = $derived(
		['text', 'markdown', 'json', 'xml', 'typst', 'html'].includes(
			(currentInput?.type ?? '').toLowerCase()
		)
	);

	let pdfNumPages = $state(0);
	let pdfZoom = $state(0.5);

	type EditorKind = 'text' | 'textarea' | 'datetime' | 'array' | 'json';
	type RecordMetaRow = {
		label: string;
		key: string;
		value: string;
		rawValue: unknown;
		editable: boolean;
		editor?: EditorKind;
		editKey?: string;
		wide?: boolean;
		pathLike?: boolean;
	};
	type DocMetaRow = {
		label: string;
		key: string;
		value: string;
		rawValue: unknown;
		editable: boolean;
		editor: EditorKind;
		editKey?: string;
		wide?: boolean;
		pathLike?: boolean;
	};

	let editingFieldKey = $state<string | null>(null);
	let editingFieldEditor = $state<EditorKind>('text');
	let editingDraft = $state('');
	let editingError = $state('');
	let editingSaving = $state(false);

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
		'typst',
	];
	const procStatusOptions = ['all', 'success', 'fail'];

	function toPositiveInt(v: unknown): number | null {
		const n = typeof v === 'string' ? Number(v.trim()) : Number(v);
		if (!Number.isFinite(n)) return null;
		const i = Math.trunc(n);
		return i > 0 ? i : null;
	}

	let highlightKeys = $derived.by(() => new Set<string>());

	function formatMaybeDate(value?: string): string {
		if (!value) return '—';
		return value.replace('T', ' ').slice(0, 19);
	}

	function getDocMetadataValue(record: KbInputRecord | null): unknown {
		if (!record) return null;
		const candidateRecord = record as KbInputRecord & {
			doc_metadata?: unknown;
			docMetadata?: unknown;
		};

		const topLevel = candidateRecord.doc_metadata ?? candidateRecord.docMetadata;
		if (topLevel != null) return topLevel;

		const fromPublicInfo = extractDocMetadataFromUnknown(record.public_info);
		if (fromPublicInfo != null) return fromPublicInfo;

		const fromPrivateInfo = extractDocMetadataFromUnknown(record.private_info);
		if (fromPrivateInfo != null) return fromPrivateInfo;

		return null;
	}

	function safeJsonStringify(value: unknown): string {
		try {
			return JSON.stringify(value);
		} catch {
			return String(value);
		}
	}

	function normalizeJsonLike(value: unknown): unknown {
		if (typeof value !== 'string') return value;
		const trimmed = value.trim();
		if (!trimmed) return '';
		if (
			(trimmed.startsWith('{') && trimmed.endsWith('}')) ||
			(trimmed.startsWith('[') && trimmed.endsWith(']'))
		) {
			try {
				return JSON.parse(trimmed) as unknown;
			} catch {
				return value;
			}
		}
		return value;
	}

	function findFirstDocMetadata(node: unknown, seen = new WeakSet<object>()): unknown {
		const normalized = normalizeJsonLike(node);
		if (normalized === null || normalized === undefined) return null;
		if (typeof normalized !== 'object') return null;

		if (Array.isArray(normalized)) {
			for (const item of normalized) {
				const found = findFirstDocMetadata(item, seen);
				if (found != null) return found;
			}
			return null;
		}

		const obj = normalized as Record<string, unknown>;
		if (seen.has(obj)) return null;
		seen.add(obj);

		if (obj.doc_metadata != null) return obj.doc_metadata;
		if (obj.docMetadata != null) return obj.docMetadata;
		if (obj.document_metadata != null) return obj.document_metadata;
		if (obj.documentMetadata != null) return obj.documentMetadata;

		for (const value of Object.values(obj)) {
			const found = findFirstDocMetadata(value, seen);
			if (found != null) return found;
		}
		return null;
	}

	function extractDocMetadataFromUnknown(value: unknown): unknown {
		const normalized = normalizeJsonLike(value);
		if (normalized == null) return null;
		return findFirstDocMetadata(normalized);
	}

	function flattenDocMetadata(
		value: unknown,
		path = 'root',
		out: Array<{ key: string; value: string; rawValue: unknown }> = [],
		seen = new WeakSet<object>()
	): Array<{ key: string; value: string; rawValue: unknown }> {
		if (value === null || value === undefined) {
			out.push({ key: path, value: 'null', rawValue: null });
			return out;
		}
		if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
			out.push({ key: path, value: String(value), rawValue: value });
			return out;
		}
		if (Array.isArray(value)) {
			if (value.length === 0) {
				out.push({ key: path, value: '[]', rawValue: [] });
				return out;
			}
			const isStringArray = value.every((item) => typeof item === 'string');
			if (isStringArray) {
				out.push({
					key: path,
					value: JSON.stringify(value),
					rawValue: value
				});
				return out;
			}
			for (let i = 0; i < value.length; i += 1) {
				flattenDocMetadata(value[i], `${path}[${i}]`, out, seen);
			}
			return out;
		}
		if (typeof value === 'object') {
			const obj = value as Record<string, unknown>;
			if (seen.has(obj)) {
				out.push({ key: path, value: '[Circular]', rawValue: '[Circular]' });
				return out;
			}
			seen.add(obj);
			const entries = Object.entries(obj).sort(([a], [b]) => a.localeCompare(b));
			if (entries.length === 0) {
				out.push({ key: path, value: '{}', rawValue: {} });
				return out;
			}
			for (const [k, v] of entries) {
				const childPath = path === 'root' ? k : `${path}.${k}`;
				flattenDocMetadata(v, childPath, out, seen);
			}
			return out;
		}
		out.push({ key: path, value: safeJsonStringify(value), rawValue: value });
		return out;
	}

	function asDisplayText(value: unknown): string {
		if (value === null || value === undefined) return '—';
		if (typeof value === 'string') return value || '—';
		if (typeof value === 'number' || typeof value === 'boolean') return String(value);
		return safeJsonStringify(value);
	}

	function safePrettyJson(value: unknown): string {
		try {
			return JSON.stringify(value ?? null, null, 2);
		} catch {
			return safeJsonStringify(value);
		}
	}

	function parseAuthors(value: string | undefined): string[] {
		if (!value) return [];
		const trimmed = value.trim();
		if (!trimmed) return [];
		try {
			const parsed = JSON.parse(trimmed) as unknown;
			if (Array.isArray(parsed)) {
				return parsed
					.map((item) => (typeof item === 'string' ? item.trim() : String(item)))
					.filter((item) => item.length > 0);
			}
		} catch {
			// fallback to plain text
		}
		return trimmed
			.split(/[,\n]/)
			.map((item) => item.trim())
			.filter((item) => item.length > 0);
	}

	function toDateTimeLocalInput(value?: string): string {
		if (!value) return '';
		const d = new Date(value);
		if (Number.isNaN(d.getTime())) return '';
		const local = new Date(d.getTime() - d.getTimezoneOffset() * 60000);
		return local.toISOString().slice(0, 16);
	}

	function fromDateTimeLocalInput(value: string): string | null {
		const trimmed = value.trim();
		if (!trimmed) return null;
		const d = new Date(trimmed);
		if (Number.isNaN(d.getTime())) return null;
		return d.toISOString();
	}

	let recordMetaRows = $derived.by(() => buildKbInputRecordMetadataRows(currentInput));

	let docMetadataRows = $derived.by(() => buildKbInputDocMetadataRows(currentInput));

	let pagesGrouped = $derived.by(() => {
		const map = new Map<number, RawLine[]>();
		for (const ln of rawLines) {
			const arr = map.get(ln.page_number) ?? [];
			arr.push(ln);
			map.set(ln.page_number, arr);
		}
		return Array.from(map.entries())
			.sort((a, b) => a[0] - b[0])
			.map(([page, lines]) => ({
				page,
				lines: lines.sort((a, b) => a.line_number - b.line_number)
			}));
	});

	function resetDocumentState() {
		rawLines = [];
		rawError = '';
		rawLoading = false;
		docPage = 1;
		pdfNumPages = 0;
	}

	async function loadInputRecord(id: number) {
		errorMsg = '';
		loading = true;
		cancelFieldEdit();
		resetDocumentState();
		try {
			rawLoading = true;
			const [inputRes, rawRes] = await Promise.all([
				getKbInput(id),
				getRawLines(id).catch(() => null)
			]);
			currentInput = inputRes.record;
			selectedInputId = inputRes.record.id;
			rawLines = rawRes?.lines ?? [];
			rawError = rawRes ? '' : 'Failed to load raw lines';
		} catch (err) {
			currentInput = null;
			selectedInputId = null;
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve record';
		} finally {
			rawLoading = false;
			loading = false;
		}
	}

	function setMode(mode: 'document' | 'source') {
		viewMode = mode;
	}

	function asTrimmedString(value: unknown): string {
		return typeof value === 'string' ? value.trim() : '';
	}

	function looksLikeFileName(value: string): boolean {
		if (!value) return false;
		if (value.includes('/') || value.includes('\\')) return true;
		return /\.[a-z0-9]{2,8}$/i.test(value);
	}

	const titleMetadataKeys = new Set([
		'title',
		'doc_title',
		'document_title',
		'standard_title',
		'full_title',
		'name'
	]);

	const docNoMetadataKeys = new Set([
		'doc_no',
		'document_no',
		'document_number',
		'standard_no',
		'standard_number',
		'identifier',
		'docnum'
	]);

	function keyAlias(key: string): string {
		return key
			.trim()
			.toLowerCase()
			.replace(/[\s-]+/g, '_');
	}

	function findMetadataTextByKeys(
		value: unknown,
		keys: Set<string>,
		seen = new WeakSet<object>()
	): string {
		const normalized = normalizeJsonLike(value);
		if (normalized === null || normalized === undefined) return '';

		if (
			typeof normalized === 'string' ||
			typeof normalized === 'number' ||
			typeof normalized === 'boolean'
		) {
			return '';
		}

		if (Array.isArray(normalized)) {
			for (const item of normalized) {
				const found = findMetadataTextByKeys(item, keys, seen);
				if (found) return found;
			}
			return '';
		}

		if (typeof normalized !== 'object') return '';
		const obj = normalized as Record<string, unknown>;
		if (seen.has(obj)) return '';
		seen.add(obj);

		for (const [k, v] of Object.entries(obj)) {
			if (keys.has(keyAlias(k))) {
				const asText = asTrimmedString(v);
				if (asText) return asText;
			}
		}

		for (const v of Object.values(obj)) {
			const found = findMetadataTextByKeys(v, keys, seen);
			if (found) return found;
		}
		return '';
	}

	function inputDisplayDocNo(r: KbInputRecord): string {
		const direct = asTrimmedString(r.doc_no);
		if (direct) return direct;
		return findMetadataTextByKeys(getKbInputDocMetadataValue(r), docNoMetadataKeys);
	}

	function inputDisplayName(r: KbInputRecord): string {
		const directTitle = asTrimmedString(r.title);
		if (directTitle) return directTitle;

		const metaTitle = findMetadataTextByKeys(getKbInputDocMetadataValue(r), titleMetadataKeys);
		if (metaTitle) return metaTitle;

		const name = asTrimmedString(r.name);
		if (name && !looksLikeFileName(name)) return name;

		return `Input #${r.id}`;
	}

	function mapBrowserRecord(record: KbInputRecord) {
		return {
			id: record.id,
			title: inputDisplayName(record),
			subtitle: record.file_name?.trim() || record.name?.trim() || '—',
			meta: [inputDisplayDocNo(record) || '—', record.parser_name?.trim() || '—'],
			status: record.type?.trim() || '—',
			description: formatMaybeDate(record.create_time),
			badges: [record.type?.trim() || '—']
		};
	}

	async function saveInputMetadataRow(row: RecordMetaRow | DocMetaRow, draft: string, editor: EditorKind) {
		if (!currentInput) return;
		const payload = buildKbInputUpdatePayloadForMetadataEdit(currentInput, row, draft, editor);
		const updated = await updateKbInput(currentInput.id, payload);
		currentInput = updated.record;
		selectedInputId = updated.record.id;
	}

	function isEditingField(key: string): boolean {
		return editingFieldKey === key;
	}

	function editorDraftFromValue(editor: EditorKind, rawValue: unknown): string {
		switch (editor) {
			case 'json':
				return safePrettyJson(rawValue);
			case 'array':
				return Array.isArray(rawValue) ? rawValue.map((v) => String(v)).join('\n') : '';
			case 'datetime':
				return typeof rawValue === 'string' ? rawValue : '';
			case 'textarea':
			case 'text':
			default:
				return rawValue == null ? '' : String(rawValue);
		}
	}

	function startFieldEdit(fieldKey: string, editor: EditorKind | string, rawValue: unknown) {
		const normalizedEditor: EditorKind =
			editor === 'text' || editor === 'textarea' || editor === 'datetime' || editor === 'array' || editor === 'json'
				? editor
				: 'text';
		editingFieldKey = fieldKey;
		editingFieldEditor = normalizedEditor;
		editingDraft = editorDraftFromValue(normalizedEditor, rawValue);
		editingError = '';
	}

	function cancelFieldEdit() {
		if (editingSaving) return;
		editingFieldKey = null;
		editingDraft = '';
		editingError = '';
	}

	function parsePathSegments(path: string): Array<string | number> {
		if (!path || path === 'root') return [];
		const segments: Array<string | number> = [];
		const re = /([^[.\]]+)|\[(\d+)\]/g;
		let match: RegExpExecArray | null = null;
		while (true) {
			match = re.exec(path);
			if (!match) break;
			if (match[1]) {
				segments.push(match[1]);
			} else if (match[2]) {
				segments.push(Number(match[2]));
			}
		}
		return segments;
	}

	function deepCloneValue<T>(value: T): T {
		try {
			return structuredClone(value);
		} catch {
			try {
				return JSON.parse(JSON.stringify(value)) as T;
			} catch {
				return value;
			}
		}
	}

	function getValueByPath(root: unknown, path: string): unknown {
		if (!path || path === 'root') return root;
		const segments = parsePathSegments(path);
		let cur: unknown = root;
		for (const seg of segments) {
			if (cur == null || typeof cur !== 'object') return undefined;
			if (typeof seg === 'number') {
				if (!Array.isArray(cur)) return undefined;
				cur = cur[seg];
			} else {
				cur = (cur as Record<string, unknown>)[seg];
			}
		}
		return cur;
	}

	function setValueByPath(root: unknown, path: string, value: unknown): unknown {
		if (path === 'root') return value;
		const base = root && typeof root === 'object' ? deepCloneValue(root) : {};
		const segments = parsePathSegments(path);
		if (segments.length === 0) return value;

		let cursor: unknown = base;
		for (let i = 0; i < segments.length; i += 1) {
			const segment = segments[i];
			const isLast = i === segments.length - 1;
			const next = segments[i + 1];

			if (typeof segment === 'number') {
				if (!Array.isArray(cursor)) return base;
				if (isLast) {
					cursor[segment] = value;
					break;
				}
				if (cursor[segment] == null || typeof cursor[segment] !== 'object') {
					cursor[segment] = typeof next === 'number' ? [] : {};
				}
				cursor = cursor[segment];
				continue;
			}

			if (cursor == null || typeof cursor !== 'object' || Array.isArray(cursor)) return base;
			const obj = cursor as Record<string, unknown>;
			if (isLast) {
				obj[segment] = value;
				break;
			}
			if (obj[segment] == null || typeof obj[segment] !== 'object') {
				obj[segment] = typeof next === 'number' ? [] : {};
			}
			cursor = obj[segment];
		}
		return base;
	}

	function parseScalarWithHint(raw: string, original: unknown): unknown {
		const trimmed = raw.trim();
		if (original === null) {
			if (trimmed === '') return null;
			if (trimmed === 'null') return null;
		}
		if (typeof original === 'number') {
			const n = Number(trimmed);
			return Number.isNaN(n) ? raw : n;
		}
		if (typeof original === 'boolean') {
			if (trimmed.toLowerCase() === 'true') return true;
			if (trimmed.toLowerCase() === 'false') return false;
			return original;
		}
		return raw;
	}

	function parseAuthorsDraft(draft: string): string[] {
		return draft
			.split(/\r?\n|,/)
			.map((item) => item.trim())
			.filter((item) => item.length > 0);
	}

	async function saveFieldEdit() {
		if (!currentInput || !editingFieldKey || editingSaving) return;
		editingError = '';
		editingSaving = true;
		try {
			const payload: Record<string, unknown> = {};
			if (editingFieldKey.startsWith('field:')) {
				const field = editingFieldKey.replace(/^field:/, '');
				switch (field) {
					case 'title':
					case 'doc_no':
					case 'source':
						payload[field] = editingDraft.trim();
						break;
					case 'publish_date': {
						const iso = fromDateTimeLocalInput(editingDraft);
						if (editingDraft.trim() && !iso) {
							throw new Error('Publish date is invalid.');
						}
						payload.publish_date = iso;
						break;
					}
					case 'authors':
						payload.authors = parseAuthorsDraft(editingDraft);
						break;
					case 'owner': {
						const trimmed = editingDraft.trim();
						payload.owner = trimmed === '' ? null : trimmed;
						break;
					}
					case 'public_info':
					case 'private_info': {
						const text = editingDraft.trim();
						payload[field] = text ? (JSON.parse(text) as unknown) : null;
						break;
					}
					case 'notes':
					case 'error_msg':
						payload[field] = editingDraft;
						break;
					default:
						throw new Error('Unsupported editable field.');
				}
			} else if (editingFieldKey.startsWith('docmeta:')) {
				const path = editingFieldKey.replace(/^docmeta:/, '');
				const currentDocMeta = getDocMetadataValue(currentInput);
				const previousValue = getValueByPath(currentDocMeta, path);
				let nextValue: unknown = editingDraft;
				if (editingFieldEditor === 'json') {
					const text = editingDraft.trim();
					nextValue = text ? (JSON.parse(text) as unknown) : null;
				} else {
					nextValue = parseScalarWithHint(editingDraft, previousValue);
				}
				payload.doc_metadata = setValueByPath(currentDocMeta ?? {}, path, nextValue);
			}

			const updated = await updateKbInput(currentInput.id, payload);
			currentInput = updated.record;
			selectedInputId = updated.record.id;
			cancelFieldEdit();
		} catch (err) {
			editingError = err instanceof Error ? err.message : 'Failed to save changes.';
		} finally {
			editingSaving = false;
		}
	}

</script>

<div
	class="metric-mgmt"
	style="
		--page-bg:{pageBg};
		--panel-bg:{panelBg};
		--panel-bg-alt:{panelBgAlt};
		--ink-line:{inkLine};
		--ink-line-soft:{inkLineSoft};
		--text-primary:{textPrimary};
		--text-secondary:{textSecondary};
		--text-muted:{textMuted};
		--brass:{brass};
		--brass-faint:{brassFaint};
		--crimson:{crimson};
		--crimson-faint:{crimsonFaint};
		--teal:{teal};
		--font-serif:{fontSerif};
		--font-mono:{fontMono};
		--font-sans:{fontSans};
	"
>
	<header class="header">
		<div class="header-left">
			<div class="eyebrow">Knowledge Base · Vol. III</div>
			<h1 class="display">Inputs&nbsp;<span class="amp">&amp;</span>&nbsp;Documents</h1>
			<div class="subtitle">
				A reading room for input records — open details, inspect source, verify documents.
			</div>
		</div>
		<div class="header-right">
			<span class="meta-label">RECORD</span><span class="meta-val">{currentInput?.id ?? '—'}</span>
			<span class="meta-label">TYPE</span><span class="meta-val">{currentInput?.type ?? '—'}</span>
			<span class="meta-label">INPUTS</span><span class="meta-val"
				>{listTotal.toString().padStart(3, '0')}</span
			>
		</div>
	</header>

	<div class="body">
		<!-- ============ LEFT PANEL ============ -->
		<aside class="left">
			<KbInputRecordBrowser
				{darkMode}
				instanceKey="inputs-mgmt"
				title="kb.inputs"
				subtitle="Search, filter, and select input records before inspecting document details."
				emptyTitle="No records yet"
				emptySubtitle="Use Search or Retrieve to browse kb.inputs."
				pageSize={KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE}
				selectedRecordId={selectedInputId}
				mapRecord={mapBrowserRecord}
				onSelect={(record) => {
					// Set selectedInputId synchronously so the browser's
					// selectedRecordId prop updates immediately.
					// If we wait until the async loadInputRecord completes,
					// the browser's sync effect will see the stale prop value
					// and overwrite its internal selection, creating an
					// infinite oscillation between records.
					selectedInputId = record.id;
					void loadInputRecord(record.id);
				}}
				onResultsChange={({ total }) => {
					listTotal = total;
				}}
				onError={(error) => {
					errorMsg = error.message;
				}}
			/>
		</aside>

		<!-- ============ RIGHT PANEL ============ -->
		<section class="right">
			<div class="right-toolbar">
				<div class="right-title">
					{#if currentInput}
						<span class="title-glyph">¶</span>
						<span class="title-name" title={currentInput.file_name ?? ''}
							>{currentInput.file_name ?? '(unnamed file)'}</span
						>
						<span class="title-type">{currentInput.type}</span>
					{:else}
						<span class="title-glyph">¶</span>
						<span class="title-name muted">No document loaded</span>
					{/if}
				</div>

				<div class="tabs" role="tablist" aria-label="Right panel mode">
					<button
						class="tab"
						class:active={viewMode === 'document'}
						onclick={() => setMode('document')}
						role="tab"
						aria-selected={viewMode === 'document'}
					>
						Document
					</button>
					<button
						class="tab"
						class:active={viewMode === 'source'}
						onclick={() => setMode('source')}
						role="tab"
						aria-selected={viewMode === 'source'}
					>
						Source&nbsp;Lines
					</button>
				</div>

				<div class="right-stats">
					{#if currentInput && viewMode === 'document'}
						<span class="stat"
							><span class="stat-num">{String(docPage).padStart(3, '0')}</span><span
								class="stat-label">page</span
							></span
						>
						<span class="stat"
							><span class="stat-num">{highlightKeys.size}</span><span class="stat-label"
								>marks</span
							></span
						>
					{:else if pagesGrouped.length > 0}
						<span class="stat"
							><span class="stat-num">{pagesGrouped.length}</span><span class="stat-label"
								>pages</span
							></span
						>
						<span class="stat"
							><span class="stat-num">{rawLines.length}</span><span class="stat-label">lines</span
							></span
						>
						<span class="stat"
							><span class="stat-num">{highlightKeys.size}</span><span class="stat-label"
								>marks</span
							></span
						>
					{/if}
				</div>
			</div>

			{#if viewMode === 'document'}
				<div class="doc-frame-wrap">
					{#if !currentInput}
						<div class="doc-empty">
							<div class="doc-empty-mark">⌬</div>
							<div class="doc-empty-title">Awaiting selection</div>
							<div class="doc-empty-sub">
								Once you retrieve a record, the original document appears here.<br />
								Click any input record on the left to open its document.
							</div>
						</div>
					{:else}
						{#if isPdf}
							<PdfViewWindow
								inputId={currentInput.id}
								{fileUrl}
								bind:page={docPage}
								bind:zoom={pdfZoom}
								bind:numPages={pdfNumPages}
								highlightVersion={currentInput.id}
								respectPageRotation={false}
								sidebarMinWidth={260}
								sidebarMaxWidth={620}
								sidebarDefaultWidth={340}
								sidebarTitle="Document"
								sidebarSettingsKey="inputs-mgmt-pdf-sidebar"
								sidebarWidthSettingLabel="Document Panel Width"
							>
								{#snippet sidebar()}
									<EditableMetadataSection
										title="Record Fields"
										rows={recordMetaRows}
										emptyText="No record loaded."
										canEdit={true}
										onSave={saveInputMetadataRow}
									/>

									<EditableMetadataSection
										title="Doc Metadata"
										rows={docMetadataRows}
										emptyText="No doc_metadata available."
										canEdit={true}
										onSave={saveInputMetadataRow}
									/>
								{/snippet}
							</PdfViewWindow>
						{:else}
							<iframe
								class="doc-frame"
								title={currentInput.file_name ?? `Record ${currentInput.id}`}
								src={fileUrl}
							></iframe>
						{/if}

						{#if isText}
							<div class="doc-foot-hint">
								This file is rendered as text by your browser. For exact line highlighting, switch
								to the <button class="inline-tab-btn" onclick={() => setMode('source')}
									>Source&nbsp;Lines</button
								> tab.
							</div>
						{:else if !isPdf}
							<div class="doc-foot-hint">
								Inline preview support varies by file type. For line-level highlights, use the <button
									class="inline-tab-btn"
									onclick={() => setMode('source')}>Source&nbsp;Lines</button
								> tab.
							</div>
						{/if}
					{/if}
				</div>
			{:else}
				<div class="document">
					{#if rawLoading}
						<div class="doc-status"><span class="dot-loop"></span>Reading raw_line file…</div>
					{:else if rawError}
						<div class="doc-error">
							<div class="doc-error-title">⚠ Cannot render document</div>
							<div class="doc-error-msg">{rawError}</div>
						</div>
					{:else if pagesGrouped.length === 0}
						<div class="doc-empty">
							<div class="doc-empty-mark">⌬</div>
							<div class="doc-empty-title">Awaiting selection</div>
							<div class="doc-empty-sub">
								Once you retrieve a record, the parsed document appears here.<br />
								Click any input record on the left to open its source lines.
							</div>
						</div>
					{:else}
						{#each pagesGrouped as pg (pg.page)}
							<article id={`page-${pg.page}`} class="page">
								<div class="page-edge" aria-hidden="true"></div>
								<header class="page-head">
									<span class="page-folio">page</span>
									<span class="page-num">{String(pg.page).padStart(3, '0')}</span>
									<span class="page-rule"></span>
									<span class="page-count">{pg.lines.length} lines</span>
								</header>
								<div class="page-body">
									{#each pg.lines as ln (ln.line_number)}
										{@const k = `${ln.page_number}:${ln.line_number}`}
										{@const hl = highlightKeys.has(k)}
										<div
											id={`raw-${ln.page_number}-${ln.line_number}`}
											class="line"
											class:highlight={hl}
											class:line-heading={ln.line_type?.includes('header') ||
												ln.line_type === 'title'}
											class:line-list={ln.line_type === 'list-item'}
											class:line-footer={ln.line_type === 'footer'}
										>
											<span class="line-no">{String(ln.line_number).padStart(4, '0')}</span>
											<span class="line-type">{ln.line_type}</span>
											<span class="line-content">{ln.content}</span>
										</div>
									{/each}
								</div>
							</article>
						{/each}
					{/if}
				</div>
			{/if}
		</section>
	</div>
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400&family=JetBrains+Mono:wght@400;500;600&family=Inter+Tight:wght@400;500;600&display=swap');

	.metric-mgmt {
		height: 100%;
		min-height: 100%;
		display: flex;
		flex-direction: column;
		background: var(--page-bg);
		color: var(--text-primary);
		font-family: var(--font-sans);
		overflow: hidden;
	}

	/* ---------- HEADER ---------- */
	.header {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		padding: 32px 40px 24px;
		border-bottom: 1px solid var(--ink-line);
		position: relative;
		gap: 24px;
	}
	.header::after {
		content: '';
		position: absolute;
		left: 40px;
		right: 40px;
		bottom: -1px;
		height: 1px;
		background: linear-gradient(
			90deg,
			var(--brass),
			transparent 35%,
			transparent 65%,
			var(--brass)
		);
		opacity: 0.45;
	}
	.eyebrow {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--brass);
		margin-bottom: 6px;
	}
	.display {
		font-family: var(--font-serif);
		font-size: 46px;
		line-height: 1;
		font-weight: 500;
		letter-spacing: -0.01em;
		margin: 0 0 8px;
	}
	.display .amp {
		font-style: italic;
		color: var(--brass);
		font-weight: 400;
	}
	.subtitle {
		font-family: var(--font-serif);
		font-style: italic;
		font-size: 15px;
		color: var(--text-secondary);
		max-width: 520px;
	}
	.header-right {
		display: grid;
		grid-template-columns: auto auto;
		gap: 6px 18px;
		align-items: baseline;
		padding: 12px 18px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
	}
	.meta-label {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.16em;
		color: var(--text-muted);
		text-transform: uppercase;
	}
	.meta-val {
		font-family: var(--font-mono);
		font-size: 13px;
		color: var(--text-primary);
		text-align: right;
		min-width: 60px;
	}

	/* ---------- BODY ---------- */
	.body {
		flex: 1;
		height: 100%;
		display: grid;
		grid-template-columns: 410px 1fr;
		min-height: 0;
		min-width: 0;
		overflow: hidden;
	}

	/* ---------- LEFT ---------- */
	.left {
		display: flex;
		flex-direction: column;
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		min-width: 0;
		min-height: 0;
		overflow: hidden;
	}

	/* ---------- RIGHT ---------- */
	.right {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-width: 0;
		min-height: 0;
		background: var(--page-bg);
		overflow: hidden;
	}
	.right-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 16px 28px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg);
		gap: 16px;
	}
	.right-title {
		display: flex;
		align-items: baseline;
		gap: 10px;
		min-width: 0;
		flex: 1;
	}
	.title-glyph {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--brass);
		line-height: 1;
	}
	.title-name {
		font-family: var(--font-serif);
		font-size: 19px;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		flex: 1;
		min-width: 0;
	}
	.title-name.muted {
		color: var(--text-muted);
		font-style: italic;
	}
	.title-type {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		padding: 2px 6px;
		border: 1px solid var(--ink-line);
	}
	.right-stats {
		display: flex;
		gap: 18px;
		flex-shrink: 0;
	}
	.stat {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		line-height: 1;
	}
	.stat-num {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--brass);
		font-weight: 500;
	}
	.stat-label {
		font-family: var(--font-mono);
		font-size: 9px;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--text-muted);
		margin-top: 4px;
	}

	.document {
		flex: 1;
		overflow-y: auto;
		padding: 32px 36px 80px;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.document::-webkit-scrollbar {
		width: 10px;
	}
	.document::-webkit-scrollbar-thumb {
		background: var(--ink-line);
	}

	/* Inline document viewer */
	.doc-frame-wrap {
		flex: 1;
		height: 100%;
		min-height: 0;
		display: flex;
		flex-direction: column;
		padding: 12px 20px 16px;
		gap: 12px;
		overflow: hidden;
	}
	.doc-frame {
		flex: 1;
		width: 100%;
		min-width: 0;
		min-height: 0;
		border: 1px solid var(--ink-line);
		background: #0a0d14;
	}
	.doc-foot-hint {
		font-size: 12px;
		line-height: 1.5;
		color: var(--text-secondary);
		padding: 8px 2px 0;
	}
	.inline-tab-btn {
		all: unset;
		cursor: pointer;
		color: var(--brass);
		font-family: var(--font-mono);
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		border-bottom: 1px dotted var(--brass);
	}
	.inline-tab-btn:hover {
		color: var(--text-primary);
		border-bottom-color: var(--text-primary);
	}
	.doc-status,
	.doc-error,
	.doc-empty {
		text-align: center;
		padding: 80px 20px;
		color: var(--text-muted);
	}
	.doc-status {
		font-family: var(--font-mono);
		font-size: 13px;
	}
	.dot-loop {
		display: inline-block;
		width: 8px;
		height: 8px;
		background: var(--brass);
		border-radius: 50%;
		margin-right: 8px;
		vertical-align: middle;
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
			transform: scale(0.85);
		}
		50% {
			opacity: 1;
			transform: scale(1.1);
		}
	}
	.doc-error-title {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--crimson);
		margin-bottom: 8px;
	}
	.doc-error-msg {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-secondary);
	}
	.doc-empty-mark {
		font-size: 56px;
		color: var(--brass);
		opacity: 0.35;
		line-height: 1;
		margin-bottom: 16px;
	}
	.doc-empty-title {
		font-family: var(--font-serif);
		font-size: 26px;
		color: var(--text-secondary);
		margin-bottom: 8px;
	}
	.doc-empty-sub {
		font-size: 13px;
		line-height: 1.6;
	}

	/* Page rendering */
	.page {
		max-width: 820px;
		margin: 0 auto 36px;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		position: relative;
	}
	.page-edge {
		position: absolute;
		top: 0;
		bottom: 0;
		left: -6px;
		width: 6px;
		background: linear-gradient(180deg, var(--brass), var(--ink-line));
		opacity: 0.55;
	}
	.page-head {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px 28px;
		border-bottom: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
	}
	.page-folio {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.page-num {
		font-family: var(--font-serif);
		font-size: 22px;
		color: var(--brass);
		font-weight: 500;
		line-height: 1;
	}
	.page-rule {
		flex: 1;
		height: 1px;
		background: var(--ink-line);
	}
	.page-count {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}
	.page-body {
		padding: 20px 28px 28px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.line {
		display: grid;
		grid-template-columns: 50px 90px 1fr;
		gap: 14px;
		align-items: baseline;
		padding: 6px 8px;
		border-left: 2px solid transparent;
		transition:
			background 200ms,
			border-color 200ms;
		font-family: var(--font-serif);
		font-size: 15px;
		line-height: 1.55;
		color: var(--text-primary);
	}
	.line:hover {
		background: var(--panel-bg-alt);
	}
	.line-no {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		text-align: right;
		letter-spacing: 0.04em;
	}
	.line-type {
		font-family: var(--font-mono);
		font-size: 9px;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		padding-top: 3px;
	}
	.line-content {
		min-width: 0;
		word-break: break-word;
	}
	.line.line-heading .line-content {
		font-size: 19px;
		font-weight: 600;
		color: var(--text-primary);
	}
	.line.line-list .line-content::before {
		content: '— ';
		color: var(--brass);
	}
	.line.line-footer {
		opacity: 0.55;
		font-style: italic;
	}

	.line.highlight {
		background: var(--crimson-faint);
		border-left-color: var(--crimson);
		box-shadow: inset 0 0 0 1px rgba(200, 85, 61, 0.18);
		animation: hl-flash 700ms ease-out;
	}
	.line.highlight .line-no {
		color: var(--crimson);
		font-weight: 600;
	}
	.line.highlight .line-type {
		color: var(--crimson);
	}
	@keyframes hl-flash {
		0% {
			background: rgba(200, 85, 61, 0.45);
		}
		100% {
			background: var(--crimson-faint);
		}
	}


</style>
