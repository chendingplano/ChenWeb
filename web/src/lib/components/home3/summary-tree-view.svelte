<script lang="ts">
	import { browser } from '$app/environment';
	import {
		getRecordSummaries,
		listSummaryGraph,
		type KbInputRecord,
		type TopicCategoryNodeApi
	} from '$lib/services/kbService';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import {
		buildTopicCategoryPathDisplay,
		buildTopicCategoryMetadataByPath,
		formatTopicCategoryConfidence,
		type TopicCategoryPathMetadata
	} from './topic-category-paths';
	import type { SummaryTreeRecord, SummaryTreeSummary } from './summary-types';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import TagIcon from '@lucide/svelte/icons/tag';
	import MapPinIcon from '@lucide/svelte/icons/map-pin';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import HashIcon from '@lucide/svelte/icons/hash';
	import ActivityIcon from '@lucide/svelte/icons/activity';

	let {
		darkMode = true,
		browserInstanceKey = 'summary-tree',
		onFocusModeChange
	}: {
		darkMode?: boolean;
		browserInstanceKey?: string;
		onFocusModeChange?: (focused: boolean) => void;
	} = $props();

	// ---------- Aesthetic tokens (archival reading room, blue/indigo accent) ----------
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
	let recordBrowserFolded = $state(false);
	let keywordFilter = $state('');
	let confidenceFilter = $state('');
	let currentRecord = $state<SummaryTreeRecord | null>(null);
	let summaries = $state<SummaryTreeSummary[]>([]);
	let selectedSummaryId = $state<string | null>(null);
	let summaryNameDropdownValue = $state<string>('');
	let loading = $state(false);
	let errorMsg = $state('');

	let categoryMetadataByPath = $state<Record<string, TopicCategoryPathMetadata>>({});
	let categoryMetadataLoaded = $state(false);

	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);
	let highlightSelectionVersion = $state(0);

	// ---------- Focus-mode info/PDF split (50/50, each panel min 25% of viewport) ----------
	const FOCUS_PDF_DEFAULT = 480;
	const FOCUS_PDF_WIDTH_KEY = 'summary-tree:focus-pdf-width';
	let focusPdfWidth = $state(FOCUS_PDF_DEFAULT);
	let focusResizing = $state(false);

	function focusPdfBounds() {
		if (!browser) return { min: 280, max: 1460 };
		const vw = window.innerWidth;
		return { min: Math.round(vw * 0.25), max: Math.round(vw * 0.75) };
	}

	function clampFocusPdfWidth(value: number) {
		const { min, max } = focusPdfBounds();
		if (!Number.isFinite(value)) return Math.round((min + max) / 2);
		return Math.max(min, Math.min(max, Math.round(value)));
	}

	function persistFocusPdfWidth(next: number) {
		focusPdfWidth = clampFocusPdfWidth(next);
		if (browser) localStorage.setItem(FOCUS_PDF_WIDTH_KEY, String(focusPdfWidth));
	}

	$effect(() => {
		if (!browser) return;
		const saved = Number(localStorage.getItem(FOCUS_PDF_WIDTH_KEY));
		if (Number.isFinite(saved) && saved > 0) {
			focusPdfWidth = clampFocusPdfWidth(saved);
			return;
		}
		focusPdfWidth = clampFocusPdfWidth(Math.round(window.innerWidth / 2));
	});

	function startFocusResize(event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = focusPdfWidth;
		focusResizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (e: PointerEvent) => persistFocusPdfWidth(startWidth - (e.clientX - startX));
		const up = () => {
			focusResizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', up);
			window.removeEventListener('pointercancel', up);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', up, { once: true });
		window.addEventListener('pointercancel', up, { once: true });
	}

	function onFocusResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			persistFocusPdfWidth(focusPdfWidth + 24);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			persistFocusPdfWidth(focusPdfWidth - 24);
		}
	}

	// ---------- Category metadata (for confidence + path display) ----------
	$effect(() => {
		if (!categoryMetadataLoaded) void ensureCategoryMetadata();
	});

	async function ensureCategoryMetadata() {
		if (categoryMetadataLoaded) return;
		categoryMetadataLoaded = true;
		try {
			const response = await listSummaryGraph();
			categoryMetadataByPath = buildTopicCategoryMetadataByPath(
				(response.results ?? []) as unknown as TopicCategoryNodeApi[]
			);
		} catch (error) {
			console.warn('failed to load summary category metadata', error);
		}
	}

	function summaryTreeStatusText(record: KbInputRecord): string {
		const items = record.status ?? [];
		const matched = [...items].reverse().find((item) => item != null);
		if (!matched) return 'pending';
		return (
			matched.proc_status?.trim() ||
			matched['proc-status']?.trim() ||
			matched.status?.trim() ||
			'pending'
		);
	}

	function mapKbInputToSummaryTreeRecord(record: KbInputRecord): SummaryTreeRecord {
		return {
			id: record.id,
			title:
				record.title?.trim() ||
				record.name?.trim() ||
				record.file_name?.trim() ||
				`Record #${record.id}`,
			fileName: record.file_name?.trim() || record.name?.trim() || '—',
			docType: record.type?.trim() || '—',
			docNo: record.doc_no?.trim() || '—',
			parserName: record.parser_name?.trim() || '—',
			procStatus: summaryTreeStatusText(record),
			createTime: record.create_time ? new Date(record.create_time).toLocaleString() : '—',
			modifyTime: record.modify_time ? new Date(record.modify_time).toLocaleString() : '—',
			summaries: []
		};
	}

	function mapBrowserRecord(record: KbInputRecord) {
		const mapped = mapKbInputToSummaryTreeRecord(record);
		return {
			id: record.id,
			title: mapped.title,
			subtitle: mapped.fileName,
			meta: [mapped.docNo, mapped.parserName],
			status: mapped.procStatus,
			description: mapped.createTime,
			badges: [mapped.docType]
		};
	}

	// ---------- Confidence (max over a summary's category paths) ----------
	function summaryConfidence(s: SummaryTreeSummary): number | null {
		let best: number | null = null;
		for (const raw of s.categoryPaths ?? []) {
			const c = categoryMetadataByPath[raw.trim()]?.confidence;
			if (Number.isFinite(c)) best = best == null ? (c as number) : Math.max(best, c as number);
		}
		return best;
	}
	function confidenceLabel(s: SummaryTreeSummary): string {
		const c = summaryConfidence(s);
		return c == null ? '—' : formatTopicCategoryConfidence(c);
	}

	function formatCoords(coords: number[]): string {
		if (!Array.isArray(coords) || coords.length < 4) return '';
		return `[${coords.map((n) => Math.trunc(n)).join(', ')}]`;
	}

	function summaryLabel(s: SummaryTreeSummary, idx: number): string {
		const snippet = (s.summaryText ?? '').trim().replace(/\s+/g, ' ').slice(0, 52);
		return `p.${s.page} · ${snippet || s.keywords?.[0] || `Summary ${idx + 1}`}`;
	}

	// ---------- Attribute model (mirrors the metrics info panel) ----------
	type AttrKind = 'text' | 'chips' | 'lines';
	type LineEntry = { head: string; content: string; lineType: string };
	type AttrDef = {
		key: string;
		label: string;
		icon: any;
		kind: AttrKind;
		value: string;
		items: string[];
		entries: LineEntry[];
		count: number;
		hasValue: boolean;
	};
	type GroupNode = {
		key: string;
		label: string;
		icon: any;
		count: number;
		filledCount: number;
		hasValue: boolean;
		attrs: AttrDef[];
	};

	function buildSummaryGroups(s: SummaryTreeSummary): GroupNode[] {
		const fmt = (v: unknown): string => (v == null || v === '' ? '' : String(v));
		const has = (v: unknown): boolean => v != null && v !== '';
		const textAttr = (key: string, label: string, icon: any, value: string, hasValue: boolean): AttrDef => ({
			key, label, icon, kind: 'text', value, items: [], entries: [], count: hasValue ? 1 : 0, hasValue
		});
		const chipsAttr = (key: string, label: string, icon: any, items: string[]): AttrDef => ({
			key, label, icon, kind: 'chips', value: items.join(', '), items, entries: [], count: items.length, hasValue: items.length > 0
		});
		const linesAttr = (key: string, label: string, icon: any, entries: LineEntry[]): AttrDef => ({
			key, label, icon, kind: 'lines',
			value: entries.map((e) => (e.content ? `${e.head}: ${e.content}` : e.head)).join('\n'),
			items: entries.map((e) => (e.content ? `${e.head}: ${e.content}` : e.head)),
			entries, count: entries.length, hasValue: entries.length > 0
		});

		const kwItems = (s.keywords ?? []).filter((v) => typeof v === 'string' && v.trim() !== '');
		const catItems = (s.categoryPaths ?? []).map((raw) => {
			const d = buildTopicCategoryPathDisplay(raw, categoryMetadataByPath);
			return d.confidenceLabel ? `${d.path} · ${d.confidenceLabel}` : d.path;
		});
		const targetEntries: LineEntry[] = (s.targets ?? [])
			.filter((t) => Array.isArray(t.coords) && t.coords.length >= 4)
			.map((t) => ({ head: `p.${t.page}`, content: formatCoords(t.coords), lineType: '' }));

		const metadata: AttrDef[] = [
			textAttr('summary_id', 'Summary ID', HashIcon, fmt(s.id), has(s.id)),
			textAttr('record_id', 'Record ID', HashIcon, fmt(s.recordId), has(s.recordId)),
			textAttr('input_id', 'Input ID', HashIcon, fmt(s.inputId), has(s.inputId)),
			textAttr('page', 'Page', MapPinIcon, fmt(s.page), has(s.page)),
			textAttr('confidence', 'Confidence', ActivityIcon, confidenceLabel(s), summaryConfidence(s) != null)
		];
		const content: AttrDef[] = [
			textAttr('summary_text', 'Summary', FileTextIcon, fmt(s.summaryText), has(s.summaryText))
		];
		const keywords: AttrDef[] = [chipsAttr('keywords', 'Keywords', TagIcon, kwItems)];
		const categories: AttrDef[] = [chipsAttr('category_paths', 'Category Paths', NetworkIcon, catItems)];
		const targets: AttrDef[] = [linesAttr('targets', 'Targets', MapPinIcon, targetEntries)];

		const specs: Array<{ key: string; label: string; icon: any; attrs: AttrDef[] }> = [
			{ key: 'g_metadata', label: 'Metadata', icon: BookOpenIcon, attrs: metadata },
			{ key: 'g_content', label: 'Content', icon: FileTextIcon, attrs: content },
			{ key: 'g_keywords', label: 'Keywords', icon: TagIcon, attrs: keywords },
			{ key: 'g_categories', label: 'Categories', icon: NetworkIcon, attrs: categories },
			{ key: 'g_targets', label: 'Targets', icon: MapPinIcon, attrs: targets }
		];
		return specs.map((spec) => {
			const filled = spec.attrs.filter((a) => a.hasValue).length;
			return {
				key: spec.key,
				label: spec.label,
				icon: spec.icon,
				count: spec.attrs.length,
				filledCount: filled,
				hasValue: filled > 0,
				attrs: spec.attrs
			};
		});
	}

	let selectedSummary = $derived.by(() => {
		if (selectedSummaryId == null) return null;
		return summaries.find((s) => s.id === selectedSummaryId) ?? null;
	});

	let summaryGroups = $derived.by(() => (selectedSummary ? buildSummaryGroups(selectedSummary) : []));

	let summaryDisplayLabel = $derived.by(() => {
		const s = selectedSummary;
		if (!s) return '';
		const idx = summaries.findIndex((x) => x.id === s.id);
		return summaryLabel(s, idx >= 0 ? idx : 0);
	});

	// ---------- Keyword list + filters ----------
	let allKeywords = $derived.by(() => {
		const set = new Set<string>();
		for (const s of summaries) for (const kw of s.keywords ?? []) set.add(kw);
		return [...set].sort((a, b) => a.localeCompare(b));
	});

	let filteredSummaries = $derived.by(() => {
		let result = summaries;
		const kw = keywordFilter.trim().toLowerCase();
		if (kw) {
			result = result.filter(
				(s) =>
					(s.keywords ?? []).some((k) => k.toLowerCase().includes(kw)) ||
					(s.summaryText ?? '').toLowerCase().includes(kw)
			);
		}
		const cf = confidenceFilter.trim();
		if (cf) {
			if (cf.startsWith('<')) {
				const th = parseFloat(cf.slice(1).trim());
				if (Number.isFinite(th)) result = result.filter((s) => (summaryConfidence(s) ?? 0) < th);
			} else {
				const th = parseFloat(cf);
				if (Number.isFinite(th)) result = result.filter((s) => (summaryConfidence(s) ?? 0) >= th);
			}
		}
		return result;
	});

	let selectedSummaryInFilteredIndex = $derived(
		filteredSummaries.findIndex((s) => s.id === selectedSummaryId)
	);
	let prevSummary = $derived(
		selectedSummaryInFilteredIndex > 0 ? filteredSummaries[selectedSummaryInFilteredIndex - 1] : null
	);
	let nextSummary = $derived(
		selectedSummaryInFilteredIndex >= 0 &&
			selectedSummaryInFilteredIndex < filteredSummaries.length - 1
			? filteredSummaries[selectedSummaryInFilteredIndex + 1]
			: null
	);

	// Keep selection valid as the filtered list changes.
	$effect(() => {
		const fs = filteredSummaries;
		if (selectedSummaryId == null) return;
		if (fs.length === 0) {
			selectedSummaryId = null;
		} else if (!fs.some((s) => s.id === selectedSummaryId)) {
			selectedSummaryId = fs[0].id;
		}
	});

	$effect(() => {
		summaryNameDropdownValue = selectedSummaryId ?? '';
	});

	// ---------- PDF viewer derived ----------
	let viewerInputId = $derived(selectedSummary?.inputId ?? currentRecord?.id ?? null);
	let viewerFileUrl = $derived(
		viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file#page=${docPage}&zoom=page-width` : ''
	);
	let viewerIsPdf = $derived((currentRecord?.fileName ?? '').trim().toLowerCase().endsWith('.pdf'));

	type SummaryPdfViewport = { convertToViewportRectangle: (rect: number[]) => number[] };

	function renderSummaryHighlight(pageNo: number, viewport: SummaryPdfViewport, overlay: HTMLDivElement) {
		const s = selectedSummary;
		if (!s) return;
		const targets = (s.targets ?? []).filter(
			(t) => t.page === pageNo && Array.isArray(t.coords) && t.coords.length >= 4
		);
		for (const target of targets) {
			const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(target.coords.slice(0, 4));
			const left = Math.max(0, Math.min(vx1, vx2) - 5);
			const top = Math.max(0, Math.min(vy1, vy2) - 4);
			const width = Math.abs(vx2 - vx1) + 10;
			const height = Math.abs(vy2 - vy1) + 8;
			if (width < 1 || height < 1) continue;
			const box = document.createElement('div');
			box.className = 'pdf-highlight';
			box.style.left = `${left}px`;
			box.style.top = `${top}px`;
			box.style.width = `${width}px`;
			box.style.height = `${height}px`;
			overlay.appendChild(box);
		}
	}

	// ---------- Handlers ----------
	async function handleRecordSelect(record: KbInputRecord) {
		if (currentRecord?.id === record.id) return;
		currentRecord = mapKbInputToSummaryTreeRecord(record);
		selectedSummaryId = null;
		summaries = [];
		errorMsg = '';
		loading = true;
		docPage = 1;
		pdfNumPages = 0;
		try {
			const response = await getRecordSummaries(record.id);
			summaries = (response.summaries ?? []).map((s) => ({ ...s, recordId: record.id }));
		} catch (error) {
			errorMsg =
				error instanceof Error ? error.message : `Failed to load summaries for record ${record.id}`;
		} finally {
			loading = false;
		}
	}

	function enterFocusMode() {
		if (recordBrowserFolded) return;
		recordBrowserFolded = true;
		onFocusModeChange?.(true);
	}

	function goBack() {
		if (!recordBrowserFolded) return;
		recordBrowserFolded = false;
		selectedSummaryId = null;
		onFocusModeChange?.(false);
	}

	function selectSummary(s: SummaryTreeSummary) {
		selectedSummaryId = s.id;
		highlightSelectionVersion += 1;
		docPage = s.page > 0 ? s.page : 1;
		enterFocusMode();
	}

	function onSummaryCardClick(event: MouseEvent, s: SummaryTreeSummary) {
		event.preventDefault();
		event.stopPropagation();
		selectSummary(s);
	}

	function goToPrevSummary() {
		if (prevSummary) selectSummary(prevSummary);
	}
	function goToNextSummary() {
		if (nextSummary) selectSummary(nextSummary);
	}
	function handleSummaryNameDropdown(e: Event) {
		const id = (e.target as HTMLSelectElement).value;
		const s = summaries.find((x) => x.id === id);
		if (s) selectSummary(s);
	}
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape' && recordBrowserFolded) goBack();
	}}
/>

<div
	class="summary-tree"
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
			<div class="eyebrow">Knowledge System · Document Summaries</div>
			<h1 class="display">Document&nbsp;<span class="amp">Tree</span></h1>
			<div class="subtitle">
				A document-centric reading room for extracted summaries — locate, verify, return to source.
			</div>
		</div>
		<div class="header-right">
			<span class="meta-label">RECORD</span><span class="meta-val">{currentRecord?.id ?? '—'}</span>
			<span class="meta-label">TYPE</span><span class="meta-val">{currentRecord?.docType ?? '—'}</span>
			<span class="meta-label">SUMMARIES</span><span class="meta-val"
				>{summaries.length.toString().padStart(3, '0')}</span
			>
		</div>
	</header>

	<div class="body" style={recordBrowserFolded ? 'grid-template-columns: minmax(0, 1fr)' : ''}>
		{#if !recordBrowserFolded}
			<KbInputRecordBrowser
				{darkMode}
				instanceKey={browserInstanceKey}
				title="kb.inputs"
				subtitle="Search, filter, and select input records before inspecting their summaries."
				emptyTitle="No records yet"
				emptySubtitle="Use Search or Retrieve to browse kb.inputs."
				autoSelectFirstRecord={false}
				selectedRecordId={currentRecord?.id ?? null}
				mapRecord={mapBrowserRecord}
				onSelect={handleRecordSelect}
				onError={(error) => {
					errorMsg = error.message;
				}}
			/>

			<aside class="summary-sidebar">
				<div class="left-meta">
					<div class="left-meta-title">Summaries</div>
					<div class="left-meta-count">{summaries.length} found</div>
				</div>

				<div class="summary-list">
					{#if errorMsg}
						<div class="error">{errorMsg}</div>
					{:else if loading}
						<div class="empty">
							<div class="empty-glyph">⌕</div>
							<div class="empty-title">Loading summaries</div>
							<div class="empty-sub">Reading summaries for the selected record…</div>
						</div>
					{:else if !currentRecord}
						<div class="empty">
							<div class="empty-glyph">§</div>
							<div class="empty-title">No record selected</div>
							<div class="empty-sub">Select a record from kb.inputs to populate the summaries index.</div>
						</div>
					{:else if summaries.length === 0}
						<div class="empty">
							<div class="empty-glyph">§</div>
							<div class="empty-title">No summaries yet</div>
							<div class="empty-sub">This record has no extracted summaries.</div>
						</div>
					{:else}
						{#each summaries as s, idx (s.id)}
							<button
								type="button"
								class="summary-card"
								class:selected={selectedSummaryId === s.id}
								onclick={(event) => onSummaryCardClick(event, s)}
							>
								<div class="card-rule" aria-hidden="true"></div>
								<div class="card-body">
									<div class="card-row-top">
										<div class="card-index">№ {String(idx + 1).padStart(3, '0')}</div>
										<div class="card-conf" title="Confidence">{confidenceLabel(s)}</div>
									</div>
									<div class="card-name">p.{s.page}</div>
									{#if s.summaryText}
										<div class="card-desc">{s.summaryText}</div>
									{/if}
									<div class="card-foot">
										<span class="chip">
											<span class="chip-dot"></span>
											{(s.targets ?? []).length} target{(s.targets ?? []).length === 1 ? '' : 's'}
										</span>
										{#each (s.keywords ?? []).slice(0, 3) as kw (kw)}
											<span class="chip chip-quiet">{kw}</span>
										{/each}
									</div>
								</div>
							</button>
						{/each}
					{/if}
				</div>
			</aside>
		{/if}

		<!-- ============ RIGHT PANEL ============ -->
		<section class="right" class:focus-split={recordBrowserFolded}>
			{#if recordBrowserFolded}
				<div class="metric-canvas-wrap">
					<div class="canvas-toolbar">
						<button type="button" class="toolbar-back" onclick={goBack} title="Back to record list">
							<ArrowLeftIcon class="toolbar-icon" />
							<span>Back</span>
						</button>
						<div class="toolbar-filters">
							<select
								class="toolbar-select"
								value={summaryNameDropdownValue}
								onchange={handleSummaryNameDropdown}
								title="Jump to summary"
							>
								<option value="">— Summary by name —</option>
								{#each summaries as s, idx (s.id)}
									<option value={s.id}>{summaryLabel(s, idx)}</option>
								{/each}
							</select>
							<div class="toolbar-kw-wrap">
								<input
									class="toolbar-kw-input"
									type="text"
									list="summary-keywords-datalist-focus"
									placeholder="Filter by keyword…"
									bind:value={keywordFilter}
								/>
								<datalist id="summary-keywords-datalist-focus">
									{#each allKeywords as kw (kw)}
										<option value={kw}></option>
									{/each}
								</datalist>
								{#if keywordFilter}
									<button
										type="button"
										class="toolbar-kw-clear"
										onclick={() => (keywordFilter = '')}
										title="Clear keyword filter"
										aria-label="Clear keyword filter"
									>×</button>
								{/if}
							</div>
							<div class="toolbar-kw-wrap">
								<input
									class="toolbar-kw-input"
									type="text"
									list="summary-confidence-options"
									placeholder="Confidence…"
									title="Filter by category confidence. Type a value like 0.85, or <0.50 for below-threshold."
									bind:value={confidenceFilter}
								/>
								<datalist id="summary-confidence-options">
									<option value="0.90"></option>
									<option value="0.80"></option>
									<option value="0.70"></option>
									<option value="0.60"></option>
									<option value="0.50"></option>
									<option value="<0.50"></option>
								</datalist>
								{#if confidenceFilter}
									<button
										type="button"
										class="toolbar-kw-clear"
										onclick={() => (confidenceFilter = '')}
										title="Clear confidence filter"
										aria-label="Clear confidence filter"
									>×</button>
								{/if}
							</div>
						</div>
						<div class="toolbar-nav">
							<button
								type="button"
								class="toolbar-nav-btn"
								disabled={!prevSummary}
								onclick={goToPrevSummary}
								title="Previous summary"
							><ChevronLeftIcon class="toolbar-icon" /></button>
							<span class="toolbar-nav-pos">
								{selectedSummaryInFilteredIndex >= 0
									? `${selectedSummaryInFilteredIndex + 1} / ${filteredSummaries.length}`
									: `— / ${filteredSummaries.length}`}
							</span>
							<button
								type="button"
								class="toolbar-nav-btn"
								disabled={!nextSummary}
								onclick={goToNextSummary}
								title="Next summary"
							><ChevronRightIcon class="toolbar-icon" /></button>
						</div>
					</div>
					<div class="metric-canvas">
						{#if selectedSummary}
							<div class="attr-view">
								<div class="attr-view-header">
									<div class="attr-view-name">{summaryDisplayLabel}</div>
								</div>
								{#each summaryGroups as g (g.key)}
									{@const GIcon = g.icon}
									<section class="attr-group" class:attr-group-empty={!g.hasValue}>
										<div class="attr-group-head">
											<span class="attr-group-ic"><GIcon class="h-4 w-4" /></span>
											<span class="attr-group-label">{g.label}</span>
											<span class="attr-group-count">{g.filledCount}/{g.count}</span>
										</div>
										<div class="attr-group-body">
											{#each g.attrs as a (a.key)}
												<div class="gip-row" class:gip-row-col={a.kind !== 'text'} class:gip-row-empty={!a.hasValue}>
													<span class="gip-label">{a.label}</span>
													{#if !a.hasValue}
														<span class="gip-empty">—</span>
													{:else if a.kind === 'text'}
														<span class="gip-val">{a.value}</span>
													{:else if a.kind === 'chips'}
														<div class="gip-chips">
															{#each a.items as item, i (`${a.key}-${i}`)}<span class="gip-chip">{item}</span>{/each}
														</div>
													{:else}
														<div class="gip-line-cards">
															{#each a.entries as entry, i (`${a.key}-${i}`)}
																<div class="gip-line-card">
																	<div class="gip-line-head">
																		<span class="gip-line-loc">{entry.head}</span>
																		{#if entry.lineType}
																			<span class="gip-line-type">{entry.lineType}</span>
																		{/if}
																	</div>
																	{#if entry.content}
																		<div class="gip-line-body">{entry.content}</div>
																	{/if}
																</div>
															{/each}
														</div>
													{/if}
												</div>
											{/each}
										</div>
									</section>
								{/each}
							</div>
						{:else}
							<div class="canvas-empty">
								<div class="canvas-empty-mark">◎</div>
								<div class="canvas-empty-title">Select a summary</div>
								<div class="canvas-empty-sub">Click a summary from the list to view its attributes.</div>
							</div>
						{/if}
					</div>
				</div>
				<button
					type="button"
					class="focus-resize-handle"
					class:active={focusResizing}
					aria-label="Resize the source document panel"
					onpointerdown={startFocusResize}
					onkeydown={onFocusResizerKeydown}
				>
					<span class="focus-resize-grip" aria-hidden="true"></span>
				</button>
			{/if}
			<div class="doc-frame-wrap" style:flex-basis={recordBrowserFolded ? `${focusPdfWidth}px` : null}>
				{#if !currentRecord}
					<div class="doc-empty">
						<div class="doc-empty-mark">⌬</div>
						<div class="doc-empty-title">Awaiting selection</div>
						<div class="doc-empty-sub">
							Once you select a record, the original document appears here.<br />
							Click any summary on the left to jump to its source page.
						</div>
					</div>
				{:else if viewerInputId && viewerIsPdf}
					<PdfViewWindow
						inputId={viewerInputId}
						fileUrl={viewerFileUrl}
						bind:page={docPage}
						bind:zoom={pdfZoom}
						bind:numPages={pdfNumPages}
						highlightVersion={selectedSummary
							? `${selectedSummary.id}:${highlightSelectionVersion}`
							: 'summary-tree'}
						renderHighlights={renderSummaryHighlight}
						{darkMode}
					/>
				{:else if viewerInputId}
					<iframe class="doc-frame" title={currentRecord.fileName} src={viewerFileUrl}></iframe>
				{:else}
					<div class="doc-empty">
						<div class="doc-empty-mark">⌬</div>
						<div class="doc-empty-title">No document</div>
						<div class="doc-empty-sub">This record has no displayable source document.</div>
					</div>
				{/if}
			</div>
		</section>
	</div>
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400&family=JetBrains+Mono:wght@400;500;600&family=Inter+Tight:wght@400;500;600&display=swap');

	.summary-tree {
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
		padding: 24px 40px 18px;
		border-bottom: 1px solid var(--ink-line);
		position: relative;
		gap: 24px;
		flex-shrink: 0;
	}
	.header::after {
		content: '';
		position: absolute;
		left: 40px;
		right: 40px;
		bottom: -1px;
		height: 1px;
		background: linear-gradient(90deg, var(--brass), transparent 35%, transparent 65%, var(--brass));
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
		font-size: 40px;
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
		grid-template-columns: auto 360px minmax(0, 1fr);
		min-height: 0;
		min-width: 0;
		overflow: hidden;
	}
	@media (max-width: 1480px) {
		.body {
			grid-template-columns: auto 320px minmax(0, 1fr);
		}
	}

	/* ---------- SUMMARY SIDEBAR ---------- */
	.summary-sidebar {
		display: flex;
		flex-direction: column;
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		min-width: 0;
		min-height: 0;
		overflow: hidden;
	}
	.error {
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--crimson);
		padding: 8px 10px;
		background: var(--crimson-faint);
		border-left: 3px solid var(--crimson);
	}
	.left-meta {
		padding: 18px 24px 8px;
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.left-meta-title {
		font-family: var(--font-serif);
		font-size: 22px;
		font-weight: 500;
		color: var(--text-primary);
	}
	.left-meta-count {
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		text-transform: uppercase;
	}
	.summary-list {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 8px 16px 24px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.summary-list::-webkit-scrollbar {
		width: 8px;
	}
	.summary-list::-webkit-scrollbar-thumb {
		background: var(--ink-line);
	}
	.empty {
		text-align: center;
		padding: 60px 20px;
		color: var(--text-muted);
	}
	.empty-glyph {
		font-family: var(--font-serif);
		font-size: 56px;
		color: var(--brass);
		opacity: 0.4;
		line-height: 1;
		margin-bottom: 12px;
	}
	.empty-title {
		font-family: var(--font-serif);
		font-size: 18px;
		color: var(--text-secondary);
		margin-bottom: 4px;
	}
	.empty-sub {
		font-size: 12px;
		max-width: 240px;
		margin: 0 auto;
		line-height: 1.5;
	}
	.summary-card {
		all: unset;
		cursor: pointer;
		display: flex;
		background: var(--panel-bg-alt);
		border: 1px solid var(--ink-line-soft);
		position: relative;
		transition: border-color 150ms, background 150ms;
	}
	.summary-card:hover {
		border-color: var(--brass);
		background: var(--panel-bg);
	}
	.summary-card.selected {
		border-color: var(--crimson);
		background: var(--panel-bg);
	}
	.card-rule {
		width: 4px;
		background: var(--ink-line);
		flex-shrink: 0;
		transition: background 150ms;
	}
	.summary-card:hover .card-rule {
		background: var(--brass);
	}
	.summary-card.selected .card-rule {
		background: var(--crimson);
	}
	.card-body {
		padding: 12px 14px 12px 12px;
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.card-row-top {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.card-index {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.1em;
		color: var(--text-muted);
		text-transform: uppercase;
	}
	.card-conf {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--teal);
		font-weight: 600;
	}
	.card-name {
		font-family: var(--font-serif);
		font-size: 17px;
		line-height: 1.2;
		font-weight: 500;
		color: var(--text-primary);
	}
	.summary-card.selected .card-name {
		color: var(--crimson);
	}
	.card-desc {
		font-size: 12px;
		line-height: 1.45;
		color: var(--text-secondary);
		display: -webkit-box;
		line-clamp: 2;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.card-foot {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
		margin-top: 2px;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		padding: 3px 8px;
		background: transparent;
		border: 1px solid var(--ink-line);
		color: var(--text-secondary);
	}
	.chip-dot {
		width: 4px;
		height: 4px;
		background: var(--brass);
		border-radius: 50%;
	}
	.chip-quiet {
		color: var(--text-muted);
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
	.right.focus-split {
		flex-direction: row;
	}
	.metric-canvas-wrap {
		flex: 1 1 0;
		min-width: 25%;
		min-height: 0;
		display: flex;
		flex-direction: column;
		border-right: 1px solid var(--ink-line);
		overflow: hidden;
	}
	.canvas-toolbar {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		flex-shrink: 0;
		flex-wrap: wrap;
	}
	.toolbar-back {
		all: unset;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 4px 10px 4px 6px;
		border-radius: 6px;
		border: 1px solid var(--ink-line);
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		color: var(--text-secondary);
		background: var(--panel-bg);
		transition: border-color 120ms, color 120ms;
	}
	.toolbar-back:hover {
		border-color: var(--brass);
		color: var(--brass);
	}
	:global(.toolbar-icon) {
		width: 13px;
		height: 13px;
		flex-shrink: 0;
		pointer-events: none;
	}
	.toolbar-filters {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 1;
		min-width: 0;
	}
	.toolbar-select {
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 4px 8px;
		max-width: 280px;
		min-width: 0;
	}
	.toolbar-select:focus {
		outline: none;
		border-color: var(--brass);
	}
	.toolbar-kw-wrap {
		position: relative;
		display: inline-flex;
		align-items: center;
	}
	.toolbar-kw-input {
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 4px 22px 4px 8px;
		width: 180px;
	}
	.toolbar-kw-input:focus {
		outline: none;
		border-color: var(--brass);
	}
	.toolbar-kw-clear {
		all: unset;
		position: absolute;
		right: 6px;
		top: 50%;
		transform: translateY(-50%);
		cursor: pointer;
		color: var(--text-muted);
		font-size: 14px;
		line-height: 1;
		padding: 0 4px;
	}
	.toolbar-kw-clear:hover {
		color: var(--brass);
	}
	.toolbar-nav {
		display: flex;
		align-items: center;
		gap: 4px;
		flex-shrink: 0;
	}
	.toolbar-nav-btn {
		all: unset;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 26px;
		height: 26px;
		border-radius: 6px;
		border: 1px solid var(--ink-line);
		color: var(--text-secondary);
		background: var(--panel-bg);
		transition: border-color 120ms, color 120ms, opacity 120ms;
	}
	.toolbar-nav-btn:disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}
	.toolbar-nav-btn:not(:disabled):hover {
		border-color: var(--brass);
		color: var(--brass);
	}
	.toolbar-nav-pos {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
		padding: 0 6px;
		min-width: 56px;
		text-align: center;
	}
	.metric-canvas {
		flex: 1 1 0;
		min-width: 0;
		min-height: 0;
		overflow-y: auto;
		overflow-x: hidden;
		background: radial-gradient(ellipse at 30% 50%, rgba(212, 162, 76, 0.06), transparent 55%),
			radial-gradient(ellipse at 70% 50%, rgba(200, 85, 61, 0.05), transparent 55%), var(--page-bg);
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.metric-canvas::-webkit-scrollbar {
		width: 8px;
	}
	.metric-canvas::-webkit-scrollbar-thumb {
		background: var(--ink-line);
	}

	/* ---- Focus split resize handle ---- */
	.focus-resize-handle {
		flex: 0 0 14px;
		position: relative;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		outline: none;
		align-self: stretch;
		touch-action: none;
	}
	.focus-resize-handle::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 6px;
		width: 2px;
		background: var(--ink-line);
		opacity: 0.6;
		transition: background 140ms, opacity 140ms;
	}
	.focus-resize-handle:hover::before,
	.focus-resize-handle.active::before,
	.focus-resize-handle:focus-visible::before {
		background: var(--brass);
		opacity: 1;
	}
	.focus-resize-grip {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 7px;
		height: 48px;
		transform: translate(-50%, -50%);
		border-radius: 999px;
		background: radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 5px 10px
				repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
	}
	.focus-resize-handle.active .focus-resize-grip,
	.focus-resize-handle:hover .focus-resize-grip,
	.focus-resize-handle:focus-visible .focus-resize-grip {
		border-color: var(--brass);
	}

	/* ---- Attribute text view ---- */
	.attr-view {
		padding: 16px 20px 40px;
		display: flex;
		flex-direction: column;
	}
	.attr-view-header {
		padding-bottom: 16px;
		border-bottom: 1px solid var(--ink-line);
		margin-bottom: 4px;
	}
	.attr-view-name {
		font-family: var(--font-serif);
		font-size: 22px;
		font-weight: 500;
		line-height: 1.25;
		color: var(--text-primary);
	}
	.attr-group {
		background: color-mix(in srgb, var(--panel-bg-alt) 90%, white);
		border: 1px solid var(--ink-line-soft);
		border-radius: 10px;
		padding: 12px 14px;
		margin-top: 10px;
	}
	.attr-group-empty {
		opacity: 0.55;
	}
	.attr-group-head {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 10px;
	}
	.attr-group-ic {
		display: inline-flex;
		color: var(--brass);
		flex-shrink: 0;
	}
	.attr-group-label {
		font-family: var(--font-serif);
		font-size: 14px;
		font-weight: 600;
		color: var(--brass);
		flex: 1;
		letter-spacing: 0.01em;
	}
	.attr-group-count {
		font-family: var(--font-mono);
		font-size: 10px;
		color: var(--text-muted);
		letter-spacing: 0.06em;
	}
	.attr-group-body {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding-left: 8px;
	}

	/* ---- Attribute rows (shared with the metrics info panel) ---- */
	.gip-row {
		display: flex;
		align-items: baseline;
		gap: 10px;
	}
	.gip-row-col {
		flex-direction: column;
		align-items: stretch;
		gap: 5px;
	}
	.gip-row-empty {
		opacity: 0.55;
	}
	.gip-label {
		font-family: var(--font-mono);
		font-size: 9px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--text-muted);
		flex-shrink: 0;
		min-width: 80px;
	}
	.gip-val {
		font-family: var(--font-sans);
		font-size: 12px;
		color: var(--text-primary);
		line-height: 1.45;
		word-break: break-word;
		flex: 1 1 auto;
		min-width: 0;
	}
	.gip-empty {
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-muted);
	}
	.gip-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.gip-chip {
		display: inline-flex;
		align-items: center;
		padding: 2px 8px;
		border-radius: 999px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		font-size: 11px;
		line-height: 1.4;
		font-family: var(--font-sans);
	}
	.gip-line-cards {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.gip-line-card {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 8px 10px;
		border: 1px solid var(--ink-line);
		border-radius: 6px;
		background: var(--panel-bg-alt);
		box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.08);
	}
	.gip-line-head {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}
	.gip-line-loc {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--brass);
	}
	.gip-line-type {
		font-family: var(--font-mono);
		font-size: 9px;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text-muted);
		padding: 1px 6px;
		border: 1px solid var(--ink-line);
		border-radius: 999px;
		background: var(--panel-bg);
	}
	.gip-line-body {
		font-family: var(--font-sans);
		font-size: 12px;
		color: var(--text-primary);
		line-height: 1.5;
		word-break: break-word;
	}

	.canvas-empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 100%;
		padding: 60px 20px;
		color: var(--text-muted);
		gap: 8px;
	}
	.canvas-empty-mark {
		font-family: var(--font-serif);
		font-size: 48px;
		opacity: 0.3;
		color: var(--brass);
		line-height: 1;
	}
	.canvas-empty-title {
		font-family: var(--font-serif);
		font-size: 18px;
		color: var(--text-secondary);
	}
	.canvas-empty-sub {
		font-size: 12px;
		max-width: 220px;
		text-align: center;
		line-height: 1.5;
	}

	/* ---- PDF panel ---- */
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
	.right.focus-split .doc-frame-wrap {
		flex: 0 0 480px;
		min-width: 25%;
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
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(200, 85, 61, 0.18);
		border: 1px solid rgba(200, 85, 61, 0.9);
		box-shadow: inset 0 0 0 1px rgba(255, 210, 179, 0.22);
	}
	.doc-empty {
		text-align: center;
		padding: 80px 20px;
		color: var(--text-muted);
		margin: auto;
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
</style>
