<script lang="ts">
	import { browser } from '$app/environment';
	import { onDestroy } from 'svelte';
	import { fly } from 'svelte/transition';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LogInIcon from '@lucide/svelte/icons/log-in';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import BrainIcon from '@lucide/svelte/icons/brain';
	import TagIcon from '@lucide/svelte/icons/tag';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import LockIcon from '@lucide/svelte/icons/lock';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import UsersIcon from '@lucide/svelte/icons/users';
	import PlayIcon from '@lucide/svelte/icons/play';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import TargetIcon from '@lucide/svelte/icons/target';
	import GitForkIcon from '@lucide/svelte/icons/git-fork';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import Share2Icon from '@lucide/svelte/icons/share-2';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import { knowledgeStoreState } from './knowledge-store-state.svelte';
	import {
		listKbSceneBlocks,
		type KbInputRecord,
		type KbSceneBlockRecord
	} from '$lib/services/kbService';

	let {
		darkMode = true,
		browserInstanceKey = 'scene-blocks',
		scopeToActiveStore = false,
		heroEyebrow = 'Subject Wiki',
		heroTitle = 'Scene Blocks',
		heroDescription = 'Inspect the event-driven scenes an LLM extracted from each document: who acts, what triggers the scene, how it unfolds, and how it resolves.',
		onFocusModeChange
	}: {
		darkMode?: boolean;
		browserInstanceKey?: string;
		scopeToActiveStore?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
		/** Notifies the host so it can fold/restore the surrounding Menu while a scene block is shown on the canvas. */
		onFocusModeChange?: (focused: boolean) => void;
	} = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#22c55e' : '#16a34a');
	// The Scene Map center node is a high-contrast disc that reads as the focal
	// point against the canvas in either theme (dark disc on light, light on dark).
	let centerBg = $derived(darkMode ? '#e8edf7' : '#0e1729');
	let centerInk = $derived(darkMode ? '#0b1220' : '#f6f8fc');

	const LIST_MIN = 360;
	const LIST_MAX = 860;
	const LIST_DEFAULT = 560;
	const FOCUS_PDF_MIN = 320;
	const FOCUS_PDF_MAX = 1460;
	const FOCUS_PDF_DEFAULT = 480;
	const listWidthKey = $derived(`scene-blocks:list-width:${browserInstanceKey}`);
	const focusPdfWidthKey = $derived(`scene-blocks:focus-pdf-width:${browserInstanceKey}`);

	let selectedRecordId = $state<number | null>(null);
	let recordCache = $state<Record<number, KbInputRecord>>({});
	let blocks = $state<KbSceneBlockRecord[]>([]);
	let loading = $state(false);
	let loadError = $state('');
	let focusedBlockId = $state<number | null>(null);
	let listWidth = $state(LIST_DEFAULT);
	let resizing = $state(false);
	let focusPdfWidth = $state(FOCUS_PDF_DEFAULT);
	let focusResizing = $state(false);
	let showDiscriminators = $state(false);
	let sceneTypeFilter = $state('__all__');

	// Scene Map (focus mode) — measured canvas box + currently hovered attribute.
	let mapW = $state(0);
	let mapH = $state(0);
	let hoveredAttr = $state<string | null>(null);
	let hoverTimer: ReturnType<typeof setTimeout> | null = null;
	let inspectorW = $state(320);
	let inspectorH = $state(260);

	let docPage = $state(1);
	let pdfZoom = $state(0.6);
	let pdfNumPages = $state(0);

	let activeRecord = $derived(
		selectedRecordId != null ? (recordCache[selectedRecordId] ?? null) : null
	);
	let viewerInputId = $derived(activeRecord?.id ?? null);
	let viewerFileUrl = $derived(viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : '');
	let viewerIsPdf = $derived((activeRecord?.file_name ?? '').trim().toLowerCase().endsWith('.pdf'));
	let focusedBlock = $derived(
		focusedBlockId != null ? (blocks.find((b) => b.id === focusedBlockId) ?? null) : null
	);
	let sceneTypeOptions = $derived.by(() => {
		const seen = new Set<string>();
		const out: string[] = [];
		for (const block of blocks) {
			const val = block.scene_type?.trim();
			if (!val || seen.has(val)) continue;
			seen.add(val);
			out.push(val);
		}
		return out.sort((a, b) => a.localeCompare(b));
	});
	let filteredFocusBlocks = $derived.by(() => {
		if (sceneTypeFilter === '__all__') return blocks;
		return blocks.filter((block) => (block.scene_type?.trim() || '') === sceneTypeFilter);
	});
	let focusedBlockIndex = $derived(
		focusedBlockId != null ? filteredFocusBlocks.findIndex((block) => block.id === focusedBlockId) : -1
	);
	let prevFocusedBlockId = $derived(
		focusedBlockIndex > 0 ? filteredFocusBlocks[focusedBlockIndex - 1]?.id ?? null : null
	);
	let nextFocusedBlockId = $derived(
		focusedBlockIndex >= 0 && focusedBlockIndex < filteredFocusBlocks.length - 1
			? filteredFocusBlocks[focusedBlockIndex + 1]?.id ?? null
			: null
	);
	let focusedMetaKeywords = $derived(focusedBlock ? strList(focusedBlock, 'keywords') : []);
	let focusedMetaStates = $derived(focusedBlock ? strList(focusedBlock, 'states') : []);
	let avgConfidence = $derived(
		blocks.length
			? Math.round(
					(blocks.reduce((sum, b) => sum + (Number(b.confidence) || 0), 0) / blocks.length) * 100
				)
			: 0
	);

	function clampListWidth(value: number) {
		if (!Number.isFinite(value)) return LIST_DEFAULT;
		return Math.max(LIST_MIN, Math.min(LIST_MAX, Math.round(value)));
	}

	function clampFocusPdfWidth(value: number) {
		if (!Number.isFinite(value)) return FOCUS_PDF_DEFAULT;
		return Math.max(FOCUS_PDF_MIN, Math.min(FOCUS_PDF_MAX, Math.round(value)));
	}

	$effect(() => {
		if (!browser) return;
		const saved = Number(localStorage.getItem(listWidthKey));
		if (Number.isFinite(saved) && saved > 0) listWidth = clampListWidth(saved);
	});

	$effect(() => {
		if (!browser) return;
		const saved = Number(localStorage.getItem(focusPdfWidthKey));
		if (Number.isFinite(saved) && saved > 0) {
			focusPdfWidth = clampFocusPdfWidth(saved);
			return;
		}
		// First-load default: keep the canvas at roughly 2/3 and the PDF at 1/3.
		focusPdfWidth = clampFocusPdfWidth(Math.round(window.innerWidth / 3));
	});

	function persistListWidth(next: number) {
		listWidth = clampListWidth(next);
		if (browser) localStorage.setItem(listWidthKey, String(listWidth));
	}

	function persistFocusPdfWidth(next: number) {
		focusPdfWidth = clampFocusPdfWidth(next);
		if (browser) localStorage.setItem(focusPdfWidthKey, String(focusPdfWidth));
	}

	async function handleRecordSelect(record: KbInputRecord) {
		recordCache = { ...recordCache, [record.id]: record };
		if (selectedRecordId === record.id && blocks.length) return;
		selectedRecordId = record.id;
		closeFocus();
		sceneTypeFilter = '__all__';
		loading = true;
		loadError = '';
		blocks = [];
		try {
			const response = await listKbSceneBlocks(record.id);
			blocks = response.results ?? [];
		} catch (error) {
			blocks = [];
			loadError =
				error instanceof Error ? error.message : 'Failed to load scene blocks for this record';
		} finally {
			loading = false;
		}
	}

	function focusBlock(id: number) {
		const block = blocks.find((item) => item.id === id) ?? null;
		const blockType = block?.scene_type?.trim() || '';
		if (sceneTypeFilter !== '__all__' && blockType !== sceneTypeFilter) {
			sceneTypeFilter = blockType || '__all__';
		}
		focusedBlockId = id;
		showDiscriminators = false;
		hoveredAttr = null;
		docPage = 1;
		onFocusModeChange?.(true);
	}

	function closeFocus() {
		if (focusedBlockId == null) return;
		focusedBlockId = null;
		showDiscriminators = false;
		hoveredAttr = null;
		onFocusModeChange?.(false);
	}

	onDestroy(() => {
		if (focusedBlockId != null) onFocusModeChange?.(false);
		if (hoverTimer) clearTimeout(hoverTimer);
	});

	function confidenceBand(value: number) {
		if (value >= 0.8) return 'high';
		if (value >= 0.5) return 'mid';
		return 'low';
	}

	function startResize(event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = listWidth;
		resizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (e: PointerEvent) => persistListWidth(startWidth + (e.clientX - startX));
		const up = () => {
			resizing = false;
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

	function onResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			persistListWidth(listWidth - 24);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			persistListWidth(listWidth + 24);
		}
	}

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

	function goToPrevScene() {
		if (prevFocusedBlockId == null) return;
		focusBlock(prevFocusedBlockId);
	}

	function goToNextScene() {
		if (nextFocusedBlockId == null) return;
		focusBlock(nextFocusedBlockId);
	}

	$effect(() => {
		if (focusedBlockId == null) return;
		if (sceneTypeFilter === '__all__') return;
		if (filteredFocusBlocks.some((block) => block.id === focusedBlockId)) return;
		if (filteredFocusBlocks.length > 0) {
			focusBlock(filteredFocusBlocks[0].id);
			return;
		}
		sceneTypeFilter = '__all__';
	});

	function arr(value: unknown): any[] {
		return Array.isArray(value) ? value : [];
	}
	function strList(block: KbSceneBlockRecord, field: string): string[] {
		return arr((block as any)[field]).filter((v) => typeof v === 'string' && v.trim() !== '');
	}
	function sortedActions(block: KbSceneBlockRecord) {
		return [...arr(block.actions)].sort(
			(a, b) => (Number(a?.sequence) || 0) - (Number(b?.sequence) || 0)
		);
	}

	// ── Scene Map model ───────────────────────────────────────────────
	// Four functional groups radiate from the scene; each carries the
	// attribute collections present on the block as satellite nodes.
	type AttrKind = 'str' | 'kw' | 'entity' | 'actions' | 'rels' | 'srcrefs' | 'disc';
	type AttrDef = { key: string; label: string; icon: any; kind: AttrKind; field: string };
	type GroupDef = {
		id: string;
		label: string;
		icon: any;
		ux: number;
		uy: number;
		attrs: AttrDef[];
	};

	const D = Math.SQRT1_2; // unit diagonal — groups sit at the four corners

	const SCENE_GROUPS: GroupDef[] = [
		{
			id: 'metadata',
			label: 'Metadata',
			icon: InfoIcon,
			ux: -D,
			uy: -D,
			attrs: [
				{ key: 'keywords', label: 'Keywords', icon: TagIcon, kind: 'kw', field: 'keywords' },
				{ key: 'states', label: 'States', icon: ActivityIcon, kind: 'str', field: 'states' }
			]
		},
		{
			id: 'inputs',
			label: 'Inputs & Resources',
			icon: LogInIcon,
			ux: D,
			uy: -D,
			attrs: [
				{ key: 'triggers', label: 'Triggers', icon: ZapIcon, kind: 'str', field: 'triggers' },
				{
					key: 'preconditions',
					label: 'Preconditions',
					icon: CircleCheckIcon,
					kind: 'str',
					field: 'preconditions'
				},
				{
					key: 'constraints',
					label: 'Constraints',
					icon: LockIcon,
					kind: 'str',
					field: 'constraints'
				},
				{
					key: 'resources',
					label: 'Resources',
					icon: DatabaseIcon,
					kind: 'entity',
					field: 'resources'
				},
				{
					key: 'source_refs',
					label: 'Source Refs',
					icon: FileTextIcon,
					kind: 'srcrefs',
					field: 'source_refs'
				}
			]
		},
		{
			id: 'actions',
			label: 'System Actions',
			icon: ZapIcon,
			ux: D,
			uy: D,
			attrs: [
				{ key: 'actors', label: 'Actors', icon: UsersIcon, kind: 'entity', field: 'actors' },
				{ key: 'actions', label: 'Actions', icon: PlayIcon, kind: 'actions', field: 'actions' },
				{
					key: 'decisions',
					label: 'Decisions',
					icon: GitBranchIcon,
					kind: 'str',
					field: 'decisions'
				},
				{
					key: 'resolutions',
					label: 'Resolutions',
					icon: FlagIcon,
					kind: 'str',
					field: 'resolutions'
				}
			]
		},
		{
			id: 'reasoning',
			label: 'Reasoning Logs',
			icon: BrainIcon,
			ux: -D,
			uy: D,
			attrs: [
				{ key: 'outcomes', label: 'Outcomes', icon: TargetIcon, kind: 'str', field: 'outcomes' },
				{
					key: 'root_causes',
					label: 'Root Causes',
					icon: GitForkIcon,
					kind: 'str',
					field: 'root_causes'
				},
				{
					key: 'failure_modes',
					label: 'Failure Modes',
					icon: TriangleAlertIcon,
					kind: 'str',
					field: 'failure_modes'
				},
				{
					key: 'relationships',
					label: 'Relationships',
					icon: Share2Icon,
					kind: 'rels',
					field: 'relationships'
				},
				{
					key: 'discriminators',
					label: 'Discriminators',
					icon: LayersIcon,
					kind: 'disc',
					field: 'discriminators'
				}
			]
		}
	];

	function attrRaw(block: KbSceneBlockRecord, def: AttrDef): any[] {
		if (def.kind === 'str' || def.kind === 'kw') return strList(block, def.field);
		if (def.kind === 'actions') return sortedActions(block);
		return arr((block as any)[def.field]);
	}

	function shortenLine(x1: number, y1: number, x2: number, y2: number, r1: number, r2: number) {
		const dx = x2 - x1;
		const dy = y2 - y1;
		const d = Math.hypot(dx, dy) || 1;
		const ux = dx / d;
		const uy = dy / d;
		return { x1: x1 + ux * r1, y1: y1 + uy * r1, x2: x2 - ux * r2, y2: y2 - uy * r2 };
	}

	let sceneMap = $derived.by(() => {
		const fb = focusedBlock;
		if (!fb || mapW < 1 || mapH < 1) return null;

		const W = mapW;
		const H = mapH;
		const cx = W / 2;
		const cy = H / 2;
		const base = Math.min(W, H);

		const Rc = Math.max(54, Math.min(94, base * 0.115)); // center disc radius
		const Rgn = Math.max(42, Math.min(58, base * 0.072)); // group node radius
		const Rsn = 21; // satellite node radius

		// A satellite can sit up to ~(Rg + Rs) from the center plus its node
		// radius and label box. Cap (Rg + Rs) conservatively (no diagonal
		// discount) so nodes and labels never clip any canvas edge.
		const reachY = cy - 12 - Rsn - 34; // label box below the lowest node
		const reachX = cx - 12 - 66; // half the max label width
		const reach = Math.max(120, Math.min(reachY, reachX));
		let Rs = Math.max(90, Math.min(200, base * 0.22));
		let Rg = Math.max(Rc + Rgn + 26, Math.min(330, base * 0.34, reach - Rs));
		if (reach - Rs < Rc + Rgn + 26) {
			Rs = Math.max(56, reach - (Rc + Rgn + 28));
			Rg = Rc + Rgn + 28;
		}

		const groups = SCENE_GROUPS.map((g) => {
			const gx = cx + g.ux * Rg;
			const gy = cy + g.uy * Rg;
			const present = g.attrs
				.map((def) => ({ def, count: attrRaw(fb, def).length }))
				.filter((s) => s.count > 0);
			const baseAng = Math.atan2(g.uy, g.ux);
			const k = present.length;
			const span = k > 1 ? Math.min(Math.PI * 0.62, (k - 1) * 0.44) : 0;
			const satellites = present.map((s, i) => {
				const ang = baseAng + (k > 1 ? (i - (k - 1) / 2) * (span / (k - 1)) : 0);
				const sx = gx + Math.cos(ang) * Rs;
				const sy = gy + Math.sin(ang) * Rs;
				const wire = shortenLine(gx, gy, sx, sy, Rgn + 3, Rsn + 3);
				return { ...s, key: s.def.key, x: sx, y: sy, ang, wire };
			});
			return {
				...g,
				x: gx,
				y: gy,
				empty: k === 0,
				satellites,
				spoke: shortenLine(cx, cy, gx, gy, Rc + 5, Rgn + 3)
			};
		});

		return { cx, cy, Rc, Rgn, Rsn, W, H, groups };
	});

	let selectedInfo = $derived.by(() => {
		if (!hoveredAttr || !focusedBlock) return null;
		for (const g of SCENE_GROUPS) {
			const def = g.attrs.find((a) => a.key === hoveredAttr);
			if (def) return { group: g, def, items: attrRaw(focusedBlock, def) };
		}
		return null;
	});

	let inspectorPos = $derived.by(() => {
		if (!sceneMap || !hoveredAttr || mapW < 720) return null;
		for (const group of sceneMap.groups) {
			const sat = group.satellites.find((s) => s.key === hoveredAttr);
			if (!sat) continue;

			const gap = 18;
			const pad = 14;
			const preferRight = sat.x <= sceneMap.W / 2;
			const rawLeft = preferRight
				? sat.x + sceneMap.Rsn + gap
				: sat.x - inspectorW - sceneMap.Rsn - gap;
			const rawTop = sat.y - inspectorH / 2;
			const left = Math.max(pad, Math.min(rawLeft, sceneMap.W - inspectorW - pad));
			const top = Math.max(pad, Math.min(rawTop, sceneMap.H - inspectorH - pad));
			return { left, top };
		}
		return null;
	});

	function satEnter(key: string) {
		if (hoverTimer) {
			clearTimeout(hoverTimer);
			hoverTimer = null;
		}
		hoveredAttr = key;
	}
	function satLeave() {
		hoverTimer = setTimeout(() => {
			hoveredAttr = null;
			hoverTimer = null;
		}, 220);
	}
	function inspEnter() {
		if (hoverTimer) {
			clearTimeout(hoverTimer);
			hoverTimer = null;
		}
	}
	function inspLeave() {
		hoveredAttr = null;
	}

	function onCanvasKeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape') return;
		if (focusedBlockId != null) closeFocus();
	}
</script>

<svelte:window onkeydown={focusedBlockId != null ? onCanvasKeydown : undefined} />

<div
	class="scene-shell"
	class:resizing={resizing || focusResizing}
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --panel-bg:${panelBg}; --panel-bg-alt:${panelAlt}; --ink-line:${border}; --ink-line-soft:rgba(148,163,184,0.16); --center-bg:${centerBg}; --center-ink:${centerInk};`}
	style:--focus-pdf-width={`${focusPdfWidth}px`}
>
	{#snippet pdfPanel(ctxBlock: KbSceneBlockRecord | null)}
		<div class="pdf-card">
			<div class="pdf-head">
				<div class="eyebrow">Source document</div>
				{#if ctxBlock}
					<div class="pdf-ctx" title={ctxBlock.title}>{ctxBlock.title}</div>
				{/if}
			</div>
			{#if viewerInputId && viewerIsPdf}
				<PdfViewWindow
					inputId={viewerInputId}
					fileUrl={viewerFileUrl}
					bind:page={docPage}
					bind:zoom={pdfZoom}
					bind:numPages={pdfNumPages}
					{darkMode}
				/>
			{:else if viewerInputId}
				<div class="empty-state pdf-empty">
					<div class="empty-title">Source isn't a PDF</div>
					<div class="empty-copy">
						This record's source file can't be previewed here. Scene block detail is available on
						the canvas.
					</div>
				</div>
			{:else}
				<div class="empty-state pdf-empty">
					<div class="empty-copy">Select a record to preview its source document.</div>
				</div>
			{/if}
		</div>
	{/snippet}

	{#if focusedBlock}
		{@const fb = focusedBlock}
		{@const conf = Number(fb.confidence) || 0}
		<div class="focus-grid" transition:fly={{ y: 8, duration: 180 }}>
			<div class="canvas">
				<div class="canvas-bar">
					<div class="canvas-bar-left">
						<button type="button" class="back-btn" onclick={closeFocus}>
							<ArrowLeftIcon class="h-4 w-4" />
							<span>Back</span>
						</button>
						<div class="canvas-crumb mono">
							{#if fb.scene_type?.trim()}<span class="crumb-type">{fb.scene_type}</span>{/if}
							Scene Block{#if fb.object_id}<span class="crumb-sep">·</span>{fb.object_id}{/if}
						</div>
					</div>
					<div class="canvas-controls">
						<label class="scene-filter">
							<span class="scene-filter-label">Scene Type</span>
							<select bind:value={sceneTypeFilter} class="scene-filter-select">
								<option value="__all__">All scene types</option>
								{#each sceneTypeOptions as sceneType}
									<option value={sceneType}>{sceneType}</option>
								{/each}
							</select>
						</label>
						<div class="scene-nav" aria-label="Scene navigation">
							<button
								type="button"
								class="nav-btn"
								aria-label="Previous scene"
								title="Previous scene"
								disabled={prevFocusedBlockId == null}
								onclick={goToPrevScene}
							>
								<ChevronLeftIcon class="h-4 w-4" />
							</button>
							<button
								type="button"
								class="nav-btn"
								aria-label="Next scene"
								title="Next scene"
								disabled={nextFocusedBlockId == null}
								onclick={goToNextScene}
							>
								<ChevronRightIcon class="h-4 w-4" />
							</button>
						</div>
					</div>
				</div>

				<div class="canvas-body">
					<header class="map-head">
						<div class="eyebrow">Scene Map</div>
						{#if fb.summary?.trim()}
							<p class="map-sub">{fb.summary}</p>
						{:else}
							<p class="map-sub">Functional structure of the extracted scene.</p>
						{/if}
					</header>

					<div class="scene-map" bind:clientWidth={mapW} bind:clientHeight={mapH}>
						{#if sceneMap}
							{@const m = sceneMap}
							<svg
								class="map-wires"
								width={m.W}
								height={m.H}
								viewBox="0 0 {m.W} {m.H}"
								aria-hidden="true"
							>
								{#each m.groups as g (g.id)}
									{#if !g.empty}
										<line
											class="wire spoke"
											x1={g.spoke.x1}
											y1={g.spoke.y1}
											x2={g.spoke.x2}
											y2={g.spoke.y2}
										/>
									{/if}
									{#each g.satellites as s (s.key)}
										<line
											class="wire"
											class:active={hoveredAttr === s.key}
											x1={s.wire.x1}
											y1={s.wire.y1}
											x2={s.wire.x2}
											y2={s.wire.y2}
										/>
									{/each}
								{/each}
							</svg>

							<div
								class="node center"
								title={fb.title?.trim() || fb.scene_id || 'Scene'}
								style="left:{m.cx}px; top:{m.cy}px; width:{m.Rc * 2}px; height:{m.Rc * 2}px;"
							>
								<span class="center-title">
									{fb.title?.trim() || fb.scene_id || 'Scene'}
								</span>
							</div>
							<div class="center-cap" style="left:{m.cx}px; top:{m.cy + m.Rc + 12}px;">
								{#if fb.object_id}
									<span class="cap-id mono">{fb.object_id}</span>
								{/if}
							</div>

							{#each m.groups as g (g.id)}
								<div
									class="node group"
									class:is-empty={g.empty}
									style="left:{g.x}px; top:{g.y}px; width:{m.Rgn * 2}px; height:{m.Rgn * 2}px;"
								>
									<g.icon class="group-ic" />
									<span class="group-label">{g.label}</span>
								</div>

								{#each g.satellites as s (s.key)}
									{@const Def = s.def.icon}
									<button
										type="button"
										class="node sat"
										class:active={hoveredAttr === s.key}
										style="left:{s.x}px; top:{s.y}px; width:{m.Rsn * 2}px; height:{m.Rsn * 2}px;"
										title="{s.def.label} ({s.count})"
										onmouseenter={() => satEnter(s.key)}
										onmouseleave={satLeave}
									>
										<Def class="sat-ic" />
										<span class="sat-badge mono">{s.count}</span>
									</button>
									<button
										type="button"
										class="sat-label"
										class:active={hoveredAttr === s.key}
										style="left:{s.x}px; top:{s.y + m.Rsn + 7}px;"
										tabindex="-1"
										onmouseenter={() => satEnter(s.key)}
										onmouseleave={satLeave}
									>
										{s.def.label}
									</button>
								{/each}
							{/each}
						{:else}
							<div class="map-measuring">Laying out scene map…</div>
						{/if}

						<aside class="map-legend" aria-label="Map legend">
							<div class="legend-h">Map Legend</div>
							<div class="legend-row">
								<span class="lg lg-scene"></span><span>Scene</span>
							</div>
							<div class="legend-row">
								<span class="lg lg-group"></span><span>Functional group</span>
							</div>
							<div class="legend-row">
								<span class="lg lg-attr"></span><span>Attribute</span>
							</div>
							<div class="legend-hint">Hover an attribute node to inspect its values</div>
						</aside>

						<aside class="map-meta-card" aria-label="Scene metadata">
							<div class="meta-card-head">
								<div class="meta-card-title">{fb.title?.trim() || fb.scene_id || 'Scene'}</div>
								{#if fb.summary?.trim()}
									<p class="meta-card-summary">
										<span class="meta-summary-label">Summary:</span>
										{fb.summary}
									</p>
								{/if}
							</div>
							<div class="meta-card-body">
								{#if fb.scene_type?.trim()}
									<div class="meta-row">
										<span class="meta-label">Scene Type</span>
										<span class="meta-val">{fb.scene_type}</span>
									</div>
								{/if}
								<div class="meta-row">
									<span class="meta-label">Confidence</span>
									<span class="cap-conf conf-{confidenceBand(conf)}" title="Extraction confidence">
										<span class="cap-conf-dot"></span>
										<span class="mono">{Math.round(conf * 100)}%</span>
									</span>
								</div>
								{#if fb.object_id}
									<div class="meta-row">
										<span class="meta-label">Object ID</span>
										<span class="meta-val mono">{fb.object_id}</span>
									</div>
								{/if}
								{#if fb.scene_id}
									<div class="meta-row">
										<span class="meta-label">Scene ID</span>
										<span class="meta-val mono">{fb.scene_id}</span>
									</div>
								{/if}
								{#if focusedMetaKeywords.length}
									<div class="meta-row meta-row-col">
										<span class="meta-label">Keywords</span>
										<div class="chips">
											{#each focusedMetaKeywords as kw}<span class="kw">{kw}</span>{/each}
										</div>
									</div>
								{/if}
								{#if focusedMetaStates.length}
									<div class="meta-row meta-row-col">
										<span class="meta-label">States</span>
										<ul class="lines">
											{#each focusedMetaStates as st}<li>{st}</li>{/each}
										</ul>
									</div>
								{/if}
							</div>
						</aside>

						{#if selectedInfo}
							{@const si = selectedInfo}
							{@const SiIcon = si.def.icon}
							<aside
								class="map-inspector"
								class:is-floating={!!inspectorPos}
								style={inspectorPos
									? `left:${inspectorPos.left}px; top:${inspectorPos.top}px;`
									: undefined}
								bind:clientWidth={inspectorW}
								bind:clientHeight={inspectorH}
								transition:fly={{ y: 12, duration: 160 }}
								onmouseenter={inspEnter}
								onmouseleave={inspLeave}
							>
								<header class="insp-head">
									<span class="insp-ic"><SiIcon class="h-4 w-4" /></span>
									<span class="insp-titles">
										<span class="insp-group">{si.group.label}</span>
										<span class="insp-name">{si.def.label}</span>
									</span>
									<span class="insp-count mono">{si.items.length}</span>
								</header>
								<div class="insp-body">
									{#if si.def.kind === 'kw'}
										<div class="chips">
											{#each si.items as kw}<span class="kw">{kw}</span>{/each}
										</div>
									{:else if si.def.kind === 'str'}
										<ul class="lines">
											{#each si.items as item}<li>{item}</li>{/each}
										</ul>
									{:else if si.def.kind === 'entity'}
										<div class="chips">
											{#each si.items as e}
												<span class="entity">
													{#if e?.type}<span class="entity-type">{e.type}</span>{/if}
													<span class="entity-name">{e?.name ?? '—'}</span>
												</span>
											{/each}
										</div>
									{:else if si.def.kind === 'actions'}
										<ol class="actions">
											{#each si.items as act, ai}
												<li>
													<span class="act-seq mono">
														{String(Number(act?.sequence) || ai + 1).padStart(2, '0')}
													</span>
													<span class="act-body">
														{#if act?.actor}<span class="act-actor">{act.actor}</span>{/if}
														<span class="act-text">{act?.action ?? ''}</span>
													</span>
												</li>
											{/each}
										</ol>
									{:else if si.def.kind === 'rels'}
										<div class="rels">
											{#each si.items as rel}
												<span class="rel">
													<span class="rel-type">{rel?.type ?? 'related'}</span>
													<span class="rel-arrow" aria-hidden="true">→</span>
													<span class="rel-target">{rel?.target ?? '—'}</span>
												</span>
											{/each}
										</div>
									{:else if si.def.kind === 'srcrefs'}
										<ul class="srcrefs">
											{#each si.items as ref}
												<li>
													<span class="src-kind">{ref?.evidence_type ?? 'ref'}</span>
													<span class="src-loc">{ref?.reference ?? ref?.source_id ?? '—'}</span>
												</li>
											{/each}
										</ul>
									{:else if si.def.kind === 'disc'}
										<div class="disc-wrap">
											{#each si.items as d}
												<div class="disc">
													{#if d?.intent}<div class="disc-intent">{d.intent}</div>{/if}
													{#if arr(d?.domain).length}
														<div class="chips">
															{#each arr(d.domain) as dom}<span class="kw">{dom}</span>{/each}
														</div>
													{/if}
													{#each arr(d?.discriminators) as dd}
														<div class="disc-item">
															<span class="disc-cat">{dd?.category ?? '—'}</span>
															<span class="disc-val">{dd?.value ?? ''}</span>
															{#if dd?.confidence != null}
																<span class="disc-conf mono"
																	>{Math.round((Number(dd.confidence) || 0) * 100)}%</span
																>
															{/if}
														</div>
													{/each}
												</div>
											{/each}
										</div>
									{/if}
								</div>
							</aside>
						{/if}
					</div>
				</div>
			</div>

			<button
				type="button"
				class="resize-handle focus-resize-handle"
				class:active={focusResizing}
				aria-label="Resize the source document panel"
				onpointerdown={startFocusResize}
				onkeydown={onFocusResizerKeydown}
			>
				<span class="resize-grip" aria-hidden="true"></span>
			</button>

			{@render pdfPanel(focusedBlock)}
		</div>
	{:else}
		<div class="hero">
			<div class="hero-copy">
				<div class="eyebrow">{heroEyebrow}</div>
				<h2>{heroTitle}</h2>
				<p>{heroDescription}</p>
			</div>
			{#if activeRecord && !loading && blocks.length}
				<div class="hero-stat" aria-label="Scene block summary for the selected record">
					<span class="hero-stat-count">{blocks.length}</span>
					<span class="hero-stat-label">scene {blocks.length === 1 ? 'block' : 'blocks'}</span>
					<span class="hero-stat-sep" aria-hidden="true">·</span>
					<span class="hero-stat-conf">avg confidence {avgConfidence}%</span>
				</div>
			{/if}
		</div>

		<div class="workspace">
			<KbInputRecordBrowser
				{darkMode}
				instanceKey={browserInstanceKey}
				title="kb.inputs"
				subtitle="Search, filter, and select a record to inspect its extracted scene blocks."
				emptyTitle="No records found."
				emptySubtitle="Use Search or Retrieve to browse kb.inputs for scene blocks."
				{scopeToActiveStore}
				{selectedRecordId}
				onSelect={handleRecordSelect}
			/>

			<div class="right-panel">
				<div class="right-tabs">
					<div class="tab active">Scene Blocks</div>
					{#if activeRecord}
						<div class="tab passive" title={activeRecord.file_name ?? ''}>
							{activeRecord.title?.trim() ||
								activeRecord.file_name?.trim() ||
								`Record #${activeRecord.id}`}
						</div>
					{/if}
				</div>

				{#if !activeRecord}
					<div class="empty-state">
						<div class="empty-title">Select a record</div>
						<div class="empty-copy">
							Choose a document from the left to read the scene blocks extracted from it.
						</div>
					</div>
				{:else}
					<div class="detail-grid">
						<div class="scene-card" style="width:{listWidth}px; flex:0 0 {listWidth}px;">
							<div class="scene-card-head">
								<div class="eyebrow">Extracted scenes</div>
								<div class="scene-card-count">
									{#if loading}loading…{:else}{blocks.length} total{/if}
								</div>
							</div>

							<div class="scene-list">
								{#if loading}
									{#each Array(4) as _, i (i)}
										<div class="skeleton-row" style={`animation-delay:${i * 60}ms`}>
											<div class="sk sk-pill"></div>
											<div class="sk sk-title"></div>
											<div class="sk sk-line"></div>
										</div>
									{/each}
								{:else if loadError}
									<div class="error-card">
										<div class="error-title">Couldn't load scene blocks</div>
										<div class="error-copy">{loadError}</div>
									</div>
								{:else if blocks.length === 0}
									<div class="empty-state">
										<div class="empty-title">No scene blocks yet</div>
										<div class="empty-copy">
											This record has no entries in <code>kb.scene_objects</code>. Scene blocks are
											produced by the generate-scene-blocks processor once the document is
											processed.
										</div>
									</div>
								{:else}
									{#each blocks as block, idx (block.id)}
										{@const conf = Number(block.confidence) || 0}
										<div class="scene-block">
											<button
												type="button"
												class="scene-row"
												title="Open this scene block"
												onclick={() => focusBlock(block.id)}
											>
												<span class="row-index mono">{String(idx + 1).padStart(2, '0')}</span>
												<span class="row-main">
													<span class="row-top">
														{#if block.scene_type?.trim()}
															<span class="scene-type">{block.scene_type}</span>
														{/if}
														<span class="row-title">
															{block.title?.trim() || block.scene_id || `Scene ${idx + 1}`}
														</span>
													</span>
													{#if block.summary?.trim()}
														<span class="row-summary">{block.summary}</span>
													{/if}
													{#if strList(block, 'keywords').length}
														<span class="row-keywords">
															{#each strList(block, 'keywords').slice(0, 6) as kw}
																<span class="kw">{kw}</span>
															{/each}
														</span>
													{/if}
												</span>
												<span class="row-conf" title="Extraction confidence">
													<span class="conf-meter conf-{confidenceBand(conf)}">
														<span class="conf-fill" style="width:{Math.round(conf * 100)}%"></span>
													</span>
													<span class="conf-num mono">{Math.round(conf * 100)}%</span>
												</span>
												<span class="row-go" aria-hidden="true">
													<ChevronRightIcon class="h-4 w-4" />
												</span>
											</button>
										</div>
									{/each}
								{/if}
							</div>
						</div>

						<button
							type="button"
							class="resize-handle"
							class:active={resizing}
							aria-label="Resize the scene block panel"
							onpointerdown={startResize}
							onkeydown={onResizerKeydown}
						>
							<span class="resize-grip" aria-hidden="true"></span>
						</button>

						{@render pdfPanel(null)}
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.scene-shell {
		position: relative;
		display: flex;
		height: 100%;
		flex-direction: column;
		color: var(--text);
		padding: 1rem;
		gap: 1rem;
	}
	.scene-shell.resizing {
		cursor: col-resize;
		user-select: none;
	}

	.hero {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1.5rem;
		padding: 1.1rem 1.25rem;
		border-radius: 24px;
		border: 1px solid var(--border);
		background:
			radial-gradient(circle at top right, rgba(34, 197, 94, 0.13), transparent 44%),
			linear-gradient(180deg, rgba(15, 23, 42, 0.82), rgba(15, 23, 42, 0.62));
	}
	.hero-copy {
		min-width: 0;
	}
	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--muted);
	}
	h2 {
		margin: 0.3rem 0 0;
		font-size: 1.5rem;
		font-weight: 700;
		letter-spacing: -0.01em;
	}
	.hero p {
		margin: 0.45rem 0 0;
		max-width: 56ch;
		font-size: 0.92rem;
		line-height: 1.6;
		color: var(--muted);
	}
	.hero-stat {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-shrink: 0;
		white-space: nowrap;
		font-size: 0.82rem;
		color: var(--muted);
	}
	.hero-stat-count {
		font-size: 1.55rem;
		font-weight: 800;
		line-height: 1;
		color: var(--text);
	}
	.hero-stat-label {
		font-weight: 600;
	}
	.hero-stat-sep {
		opacity: 0.5;
	}
	.hero-stat-conf {
		font-variant-numeric: tabular-nums;
	}

	.workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: auto minmax(0, 1fr);
		grid-template-rows: minmax(0, 1fr);
		gap: 1rem;
	}

	.right-panel {
		display: flex;
		min-height: 0;
		flex-direction: column;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
		padding: 1rem;
	}
	.right-tabs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
		margin-bottom: 1rem;
	}
	.tab {
		border-radius: 14px;
		padding: 0.6rem 0.9rem;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.3);
		font-size: 0.85rem;
		font-weight: 700;
		max-width: 30ch;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tab.active {
		border-color: rgba(34, 197, 94, 0.36);
		background: rgba(34, 197, 94, 0.14);
		color: #a7f3d0;
	}
	.tab.passive {
		color: var(--muted);
		font-weight: 600;
	}

	.detail-grid {
		display: flex;
		min-height: 0;
		flex: 1;
		align-items: stretch;
		gap: 0;
	}
	.scene-card,
	.pdf-card {
		display: flex;
		flex-direction: column;
		min-height: 0;
		border-radius: 20px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(15, 23, 42, 0.26);
	}
	.scene-card {
		flex: 0 0 auto;
		overflow: hidden;
	}
	.pdf-card {
		flex: 1 1 0;
		min-width: 0;
		padding: 1rem;
	}
	.scene-card-head,
	.pdf-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		padding: 1rem 1.1rem;
		border-bottom: 1px solid rgba(148, 163, 184, 0.1);
	}
	.pdf-head {
		padding: 0 0 0.85rem;
	}
	.scene-card-count {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.7rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.pdf-ctx {
		font-size: 0.8rem;
		color: var(--muted);
		max-width: 32ch;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.scene-list {
		display: flex;
		flex-direction: column;
		min-height: 0;
		flex: 1;
		overflow-y: auto;
		padding: 0.9rem;
		gap: 0.65rem;
	}

	.scene-block {
		flex: 0 0 auto;
		border: 1px solid rgba(148, 163, 184, 0.13);
		border-radius: 16px;
		background: rgba(2, 6, 23, 0.28);
		overflow: hidden;
		transition:
			border-color 160ms ease,
			background 160ms ease;
	}
	.scene-block:hover {
		border-color: color-mix(in srgb, var(--accent) 50%, transparent);
		background: color-mix(in srgb, var(--accent) 6%, rgba(2, 6, 23, 0.3));
	}
	.scene-row {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto auto;
		align-items: start;
		gap: 0.7rem;
		width: 100%;
		text-align: left;
		padding: 0.85rem 0.95rem;
		background: transparent;
		border: 0;
		cursor: pointer;
		color: inherit;
	}
	.scene-row:hover {
		background: rgba(148, 163, 184, 0.06);
	}
	.row-index {
		font-size: 0.74rem;
		font-weight: 700;
		color: var(--muted);
		padding-top: 0.15rem;
	}
	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	}
	.row-go {
		display: inline-flex;
		align-items: center;
		color: var(--muted);
		padding-top: 0.3rem;
		transition:
			transform 160ms ease,
			color 160ms ease;
	}
	.scene-row:hover .row-go {
		color: var(--accent);
		transform: translateX(2px);
	}
	.row-main {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		min-width: 0;
	}
	.row-top {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.scene-type {
		display: inline-flex;
		align-items: center;
		padding: 0.16rem 0.5rem;
		border-radius: 999px;
		font-size: 0.66rem;
		font-weight: 700;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: #a7f3d0;
		background: rgba(34, 197, 94, 0.12);
		border: 1px solid rgba(34, 197, 94, 0.25);
	}
	.row-title {
		font-size: 0.95rem;
		font-weight: 700;
		color: var(--text);
		line-height: 1.35;
	}
	.row-summary {
		font-size: 0.84rem;
		line-height: 1.5;
		color: var(--muted);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.row-keywords {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
		margin-top: 0.1rem;
	}
	.kw {
		font-size: 0.68rem;
		padding: 0.12rem 0.42rem;
		border-radius: 6px;
		color: var(--muted);
		background: rgba(148, 163, 184, 0.1);
	}
	.row-conf {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.3rem;
		padding-top: 0.15rem;
	}
	.conf-meter {
		width: 52px;
		height: 4px;
		border-radius: 999px;
		background: rgba(148, 163, 184, 0.18);
		overflow: hidden;
	}
	.conf-fill {
		display: block;
		height: 100%;
		border-radius: 999px;
	}
	.conf-high .conf-fill {
		background: var(--accent);
	}
	.conf-mid .conf-fill {
		background: #d6a93c;
	}
	.conf-low .conf-fill {
		background: #b9657a;
	}
	.conf-num {
		font-size: 0.68rem;
		color: var(--muted);
		font-variant-numeric: tabular-nums;
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.entity {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
		padding: 0.28rem 0.55rem;
		border-radius: 8px;
		background: rgba(148, 163, 184, 0.1);
		border: 1px solid rgba(148, 163, 184, 0.14);
		font-size: 0.82rem;
		color: var(--text);
	}
	.entity-type {
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.lines {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.lines li {
		position: relative;
		padding-left: 1rem;
		font-size: 0.86rem;
		line-height: 1.55;
		color: var(--text);
	}
	.lines li::before {
		content: '';
		position: absolute;
		left: 0.1rem;
		top: 0.62rem;
		width: 4px;
		height: 4px;
		border-radius: 50%;
		background: color-mix(in srgb, var(--accent) 70%, transparent);
	}
	.actions {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.actions li {
		display: flex;
		gap: 0.6rem;
		align-items: baseline;
	}
	.act-seq {
		font-size: 0.7rem;
		font-weight: 700;
		color: var(--accent);
		padding: 0.06rem 0.36rem;
		border-radius: 6px;
		background: rgba(34, 197, 94, 0.12);
		flex-shrink: 0;
	}
	.act-body {
		font-size: 0.86rem;
		line-height: 1.5;
		color: var(--text);
	}
	.act-actor {
		font-weight: 700;
		color: var(--text);
	}
	.act-actor::after {
		content: ':';
		margin-right: 0.3rem;
		color: var(--muted);
	}
	.rels {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.rel {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.26rem 0.55rem;
		border-radius: 8px;
		background: rgba(148, 163, 184, 0.08);
		border: 1px solid rgba(148, 163, 184, 0.14);
		font-size: 0.8rem;
	}
	.rel-type {
		color: var(--muted);
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.rel-arrow {
		color: var(--accent);
	}
	.rel-target {
		color: var(--text);
	}
	.srcrefs {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	.srcrefs li {
		display: flex;
		gap: 0.55rem;
		align-items: baseline;
		font-size: 0.82rem;
	}
	.src-kind {
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--muted);
		background: rgba(148, 163, 184, 0.1);
		padding: 0.12rem 0.4rem;
		border-radius: 6px;
		flex-shrink: 0;
	}
	.src-loc {
		color: var(--text);
		word-break: break-word;
	}
	.disc-wrap {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.disc {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.65rem 0.75rem;
		border-radius: 10px;
		background: rgba(15, 23, 42, 0.4);
		border: 1px solid rgba(148, 163, 184, 0.12);
	}
	.disc-intent {
		font-size: 0.84rem;
		font-weight: 600;
		color: var(--text);
	}
	.disc-item {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
		font-size: 0.8rem;
	}
	.disc-cat {
		font-size: 0.62rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted);
		flex-shrink: 0;
	}
	.disc-val {
		color: var(--text);
	}
	.disc-conf {
		margin-left: auto;
		color: var(--muted);
		font-size: 0.72rem;
	}
	.resize-handle {
		flex: 0 0 16px;
		position: relative;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		outline: none;
		align-self: stretch;
	}
	.resize-handle::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 7px;
		width: 1px;
		background: var(--ink-line);
		opacity: 0.75;
	}
	.resize-grip {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 8px;
		height: 52px;
		transform: translate(-50%, -50%);
		border-radius: 999px;
		background:
			radial-gradient(circle, var(--muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
	}
	.resize-handle.active .resize-grip,
	.resize-handle:hover .resize-grip,
	.resize-handle:focus-visible .resize-grip {
		border-color: var(--accent);
	}

	.empty-state {
		display: grid;
		place-items: center;
		align-content: center;
		flex: 1;
		min-height: 220px;
		text-align: center;
		padding: 2rem;
		color: var(--muted);
		gap: 0.5rem;
	}
	.empty-title {
		font-size: 1rem;
		font-weight: 700;
		color: var(--text);
	}
	.empty-copy {
		font-size: 0.86rem;
		line-height: 1.6;
		max-width: 42ch;
	}
	.empty-copy code {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.82em;
		padding: 0.1rem 0.3rem;
		border-radius: 5px;
		background: rgba(148, 163, 184, 0.14);
	}
	.error-card {
		margin: 0.4rem;
		border-radius: 14px;
		border: 1px solid rgba(248, 113, 113, 0.28);
		background: rgba(127, 29, 29, 0.16);
		padding: 0.95rem 1.05rem;
	}
	.error-title {
		font-weight: 700;
		color: #fecaca;
		margin-bottom: 0.3rem;
	}
	.error-copy {
		font-size: 0.85rem;
		line-height: 1.5;
		color: #fca5a5;
	}

	.skeleton-row {
		flex: 0 0 auto;
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		padding: 0.95rem;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.1);
		background: rgba(2, 6, 23, 0.28);
		animation: pulse 1.5s ease-in-out infinite;
	}
	.sk {
		border-radius: 6px;
		background: rgba(148, 163, 184, 0.16);
	}
	.sk-pill {
		width: 78px;
		height: 14px;
	}
	.sk-title {
		width: 70%;
		height: 13px;
	}
	.sk-line {
		width: 92%;
		height: 10px;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.45;
		}
	}

	/* ── Focus mode: Canvas | PDF Viewer ─────────────────────────────── */
	.focus-grid {
		display: flex;
		min-height: 0;
		flex: 1;
		align-items: stretch;
		gap: 1rem;
	}
	.canvas {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1 1 0;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
		overflow: hidden;
	}
	.focus-grid > .pdf-card {
		flex: 0 0 var(--focus-pdf-width, 480px);
		min-width: 0;
		padding: 1rem;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
	}
	.focus-resize-handle {
		flex-shrink: 0;
	}
	.canvas-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.85rem 1.1rem;
		border-bottom: 1px solid rgba(148, 163, 184, 0.14);
		flex-shrink: 0;
	}
	.canvas-bar-left {
		display: flex;
		align-items: center;
		gap: 0.9rem;
		min-width: 0;
	}
	.canvas-controls {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.75rem;
		flex-wrap: wrap;
	}
	.back-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.5rem 0.95rem;
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.22);
		background: rgba(148, 163, 184, 0.08);
		color: var(--text);
		font-size: 0.85rem;
		font-weight: 700;
		cursor: pointer;
		transition:
			border-color 150ms ease,
			background 150ms ease,
			color 150ms ease;
	}
	.back-btn:hover {
		border-color: color-mix(in srgb, var(--accent) 55%, transparent);
		background: color-mix(in srgb, var(--accent) 12%, transparent);
		color: var(--accent);
	}
	.scene-filter {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		min-width: 0;
	}
	.scene-filter-label {
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
		flex-shrink: 0;
	}
	.scene-filter-select {
		min-width: 180px;
		max-width: 280px;
		padding: 0.48rem 2rem 0.48rem 0.78rem;
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.22);
		background: rgba(15, 23, 42, 0.42);
		color: var(--text);
		font-size: 0.82rem;
		line-height: 1.2;
		appearance: none;
		cursor: pointer;
		background-image:
			linear-gradient(45deg, transparent 50%, var(--muted) 50%),
			linear-gradient(135deg, var(--muted) 50%, transparent 50%);
		background-position:
			calc(100% - 16px) calc(50% - 2px),
			calc(100% - 11px) calc(50% - 2px);
		background-size: 5px 5px, 5px 5px;
		background-repeat: no-repeat;
	}
	.scene-filter-select:focus-visible {
		outline: none;
		border-color: color-mix(in srgb, var(--accent) 60%, transparent);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent);
	}
	.scene-nav {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.nav-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 38px;
		height: 38px;
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.22);
		background: rgba(148, 163, 184, 0.08);
		color: var(--text);
		cursor: pointer;
		transition: border-color 150ms ease, background 150ms ease, color 150ms ease, opacity 150ms ease;
	}
	.nav-btn:hover:not(:disabled) {
		border-color: color-mix(in srgb, var(--accent) 55%, transparent);
		background: color-mix(in srgb, var(--accent) 12%, transparent);
		color: var(--accent);
	}
	.nav-btn:disabled {
		opacity: 0.42;
		cursor: not-allowed;
	}
	.nav-btn:focus-visible {
		outline: none;
		border-color: color-mix(in srgb, var(--accent) 60%, transparent);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent);
	}
	.canvas-crumb {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.72rem;
		letter-spacing: 0.06em;
		color: var(--muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.crumb-type {
		padding: 0.16rem 0.5rem;
		border-radius: 999px;
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--accent);
		background: color-mix(in srgb, var(--accent) 14%, transparent);
		border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
	}
	.crumb-sep {
		margin: 0 0.1rem;
		opacity: 0.5;
	}
	.canvas-body {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
		min-height: 0;
		flex: 1;
		padding: 1.05rem 1.25rem 1.15rem;
	}
	.map-head {
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	.map-sub {
		margin: 0.15rem 0 0;
		font-size: 0.86rem;
		line-height: 1.55;
		color: var(--muted);
		max-width: 78ch;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.scene-map {
		position: relative;
		flex: 1 1 auto;
		min-height: 440px;
		border-radius: 18px;
		border: 1px solid var(--border);
		overflow: hidden;
		background:
			radial-gradient(
				circle at 50% 47%,
				color-mix(in srgb, var(--accent) 8%, transparent),
				transparent 58%
			),
			var(--panel-alt);
	}
	.scene-map::before {
		content: '';
		position: absolute;
		inset: 0;
		background-image: radial-gradient(
			color-mix(in srgb, var(--muted) 24%, transparent) 1px,
			transparent 1.5px
		);
		background-size: 26px 26px;
		background-position: -13px -13px;
		opacity: 0.45;
		pointer-events: none;
	}
	.map-wires {
		position: absolute;
		inset: 0;
		overflow: visible;
		pointer-events: none;
	}
	.wire {
		fill: none;
		stroke: color-mix(in srgb, var(--muted) 34%, transparent);
		stroke-width: 1.25;
		transition:
			stroke 150ms ease,
			stroke-width 150ms ease;
	}
	.wire.spoke {
		stroke: color-mix(in srgb, var(--muted) 46%, transparent);
		stroke-width: 1.5;
	}
	.wire.active {
		stroke: var(--accent);
		stroke-width: 2;
	}

	.node {
		position: absolute;
		transform: translate(-50%, -50%);
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.center {
		border-radius: 50%;
		background: var(--center-bg);
		color: var(--center-ink);
		text-align: center;
		z-index: 3;
		box-shadow:
			0 14px 34px -12px rgba(2, 6, 23, 0.55),
			0 0 0 7px color-mix(in srgb, var(--center-bg) 16%, transparent);
	}
	.center-title {
		padding: 0 0.95rem;
		font-size: clamp(0.82rem, 1.3vw, 1.02rem);
		font-weight: 800;
		line-height: 1.22;
		letter-spacing: -0.01em;
		display: -webkit-box;
		-webkit-line-clamp: 4;
		line-clamp: 4;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.center-cap {
		position: absolute;
		transform: translate(-50%, 0);
		z-index: 3;
		display: flex;
		align-items: center;
		gap: 0.45rem;
		white-space: nowrap;
	}
	.cap-conf {
		display: inline-flex;
		align-items: center;
		gap: 0.34rem;
		padding: 0.16rem 0.5rem;
		border-radius: 999px;
		background: color-mix(in srgb, var(--muted) 16%, transparent);
		color: var(--text);
		font-size: 0.72rem;
	}
	.cap-conf-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--muted);
	}
	.cap-conf.conf-high .cap-conf-dot {
		background: var(--accent);
	}
	.cap-conf.conf-mid .cap-conf-dot {
		background: #d6a93c;
	}
	.cap-conf.conf-low .cap-conf-dot {
		background: #b9657a;
	}
	.cap-id {
		font-size: 0.7rem;
		color: var(--muted);
	}

	.group {
		flex-direction: column;
		gap: 0.18rem;
		border-radius: 50%;
		background: var(--panel);
		border: 1.5px solid color-mix(in srgb, var(--text) 26%, transparent);
		color: var(--text);
		text-align: center;
		z-index: 2;
		box-shadow: 0 8px 20px -12px rgba(2, 6, 23, 0.55);
	}
	.group.is-empty {
		opacity: 0.4;
		filter: saturate(0.4);
	}
	:global(.group .group-ic) {
		width: 17px;
		height: 17px;
		color: var(--muted);
	}
	.group-label {
		padding: 0 0.4rem;
		font-size: clamp(0.55rem, 0.85vw, 0.66rem);
		font-weight: 800;
		letter-spacing: 0.07em;
		text-transform: uppercase;
		line-height: 1.15;
		color: var(--text);
	}

	.sat {
		border-radius: 50%;
		padding: 0;
		cursor: pointer;
		color: var(--accent);
		background: color-mix(in srgb, var(--accent) 15%, var(--panel));
		border: 1px solid color-mix(in srgb, var(--accent) 40%, transparent);
		z-index: 2;
		transition:
			transform 150ms cubic-bezier(0.22, 1, 0.36, 1),
			box-shadow 150ms ease,
			background 150ms ease,
			border-color 150ms ease;
	}
	.sat:hover {
		transform: translate(-50%, -50%) scale(1.09);
		border-color: var(--accent);
	}
	.sat:focus-visible {
		outline: none;
		box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent) 30%, transparent);
	}
	:global(.sat .sat-ic) {
		width: 17px;
		height: 17px;
	}
	.sat-badge {
		position: absolute;
		top: -7px;
		right: -7px;
		min-width: 17px;
		height: 17px;
		padding: 0 4px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 999px;
		background: var(--accent);
		color: #fff;
		font-size: 0.58rem;
		font-weight: 700;
		line-height: 1;
		box-shadow: 0 0 0 2px var(--panel-alt);
	}
	.sat.active {
		background: var(--accent);
		color: #fff;
		border-color: var(--accent);
		box-shadow: 0 0 0 5px color-mix(in srgb, var(--accent) 24%, transparent);
	}
	.sat.active .sat-badge {
		background: var(--panel);
		color: var(--accent);
	}
	.sat-label {
		position: absolute;
		transform: translate(-50%, 0);
		z-index: 2;
		max-width: 124px;
		padding: 0;
		border: 0;
		background: transparent;
		text-align: center;
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		line-height: 1.25;
		color: var(--muted);
		cursor: pointer;
		word-break: break-word;
		transition: color 150ms ease;
	}
	.sat-label:hover {
		color: var(--text);
	}
	.sat-label.active {
		color: var(--accent);
	}
	.map-measuring {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		font-size: 0.82rem;
		color: var(--muted);
	}

	.map-legend {
		position: absolute;
		right: 14px;
		bottom: 14px;
		z-index: 4;
		display: flex;
		flex-direction: column;
		gap: 0.38rem;
		padding: 0.7rem 0.85rem;
		border-radius: 12px;
		background: color-mix(in srgb, var(--panel) 94%, transparent);
		border: 1px solid var(--border);
		box-shadow: 0 12px 28px -16px rgba(2, 6, 23, 0.5);
	}
	.legend-h {
		font-size: 0.58rem;
		font-weight: 800;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
		margin-bottom: 0.05rem;
	}
	.legend-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.72rem;
		color: var(--text);
	}
	.lg {
		width: 12px;
		height: 12px;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.lg-scene {
		background: var(--center-bg);
		border: 1px solid color-mix(in srgb, var(--text) 28%, transparent);
	}
	.lg-group {
		background: transparent;
		border: 1.5px solid color-mix(in srgb, var(--text) 42%, transparent);
	}
	.lg-attr {
		background: color-mix(in srgb, var(--accent) 24%, var(--panel));
		border: 1px solid var(--accent);
	}
	.legend-hint {
		margin-top: 0.15rem;
		max-width: 156px;
		font-size: 0.66rem;
		line-height: 1.4;
		color: var(--muted);
		opacity: 0.85;
	}

	.map-meta-card {
		position: absolute;
		left: 14px;
		top: 14px;
		z-index: 4;
		width: min(256px, 34%);
		max-height: calc(50% - 20px);
		display: flex;
		flex-direction: column;
		border-radius: 14px;
		border: 1px solid var(--border);
		background: var(--panel);
		box-shadow: 0 20px 44px -18px rgba(2, 6, 23, 0.6);
		overflow: hidden;
	}
	.meta-card-head {
		padding: 0.65rem 0.75rem;
		border-bottom: 1px solid color-mix(in srgb, var(--border) 70%, transparent);
		flex-shrink: 0;
	}
	.meta-card-title {
		font-size: 0.84rem;
		font-weight: 700;
		color: var(--text);
		line-height: 1.25;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.meta-card-summary {
		margin: 0.28rem 0 0;
		font-size: 0.73rem;
		line-height: 1.45;
		color: var(--muted);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.meta-summary-label {
		font-weight: 700;
		color: var(--text);
	}
	.meta-card-body {
		display: flex;
		flex-direction: column;
		gap: 0.42rem;
		padding: 0.65rem 0.75rem;
		overflow-y: auto;
	}
	.meta-row {
		display: flex;
		align-items: baseline;
		gap: 0.45rem;
		font-size: 0.77rem;
		flex-wrap: wrap;
	}
	.meta-row-col {
		flex-direction: column;
		gap: 0.28rem;
	}
	.meta-label {
		font-size: 0.58rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
		flex-shrink: 0;
	}
	.meta-val {
		color: var(--text);
		font-size: 0.73rem;
		word-break: break-all;
		min-width: 0;
	}

	.map-inspector {
		position: absolute;
		left: 14px;
		bottom: 14px;
		z-index: 5;
		width: min(360px, 44%);
		max-height: calc(100% - 28px);
		display: flex;
		flex-direction: column;
		border-radius: 14px;
		border: 1px solid var(--border);
		background: var(--panel);
		box-shadow: 0 20px 44px -18px rgba(2, 6, 23, 0.6);
		overflow: hidden;
	}
	.map-inspector.is-floating {
		bottom: auto;
	}
	.insp-head {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.7rem 0.8rem;
		border-bottom: 1px solid color-mix(in srgb, var(--border) 70%, transparent);
	}
	.insp-ic {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 30px;
		height: 30px;
		border-radius: 9px;
		flex-shrink: 0;
		color: var(--accent);
		background: color-mix(in srgb, var(--accent) 15%, transparent);
	}
	.insp-titles {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
	}
	.insp-group {
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.insp-name {
		font-size: 0.92rem;
		font-weight: 700;
		color: var(--text);
		line-height: 1.2;
	}
	.insp-count {
		font-size: 0.74rem;
		color: var(--muted);
		background: color-mix(in srgb, var(--muted) 15%, transparent);
		padding: 0.1rem 0.42rem;
		border-radius: 6px;
	}
	.insp-body {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		padding: 0.8rem;
		overflow-y: auto;
	}

	@media (max-width: 1180px) {
		.map-inspector {
			width: min(320px, 58%);
		}
	}

	@media (max-width: 720px) {
		.scene-map {
			min-height: 380px;
		}
		.map-legend,
		.map-meta-card {
			display: none;
		}
		.map-inspector {
			left: 10px;
			top: auto !important;
			right: 10px;
			bottom: 10px;
			width: auto;
		}
	}

	@media (max-width: 980px) {
		.workspace {
			grid-template-columns: minmax(0, 1fr);
		}
		.detail-grid {
			flex-direction: column;
			gap: 1rem;
		}
		.scene-card {
			width: 100% !important;
			flex: none !important;
		}
		.resize-handle {
			display: none;
		}
		.hero {
			flex-direction: column;
			align-items: flex-start;
			gap: 1rem;
		}
		.focus-grid {
			flex-direction: column;
		}
		.canvas-bar {
			flex-direction: column;
			align-items: stretch;
		}
		.canvas-bar-left,
		.canvas-controls {
			width: 100%;
		}
		.canvas-controls {
			justify-content: space-between;
		}
		.scene-filter {
			flex: 1 1 220px;
		}
		.scene-filter-select {
			max-width: none;
			flex: 1;
		}
		.focus-resize-handle {
			display: none;
		}
		.focus-grid > .pdf-card {
			flex: 1 1 auto;
			min-height: 420px;
		}
	}
</style>
