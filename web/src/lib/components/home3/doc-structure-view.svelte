<script lang="ts">
	import {
		getKbDocStructure,
		getKbInput,
		type DocStructureLine,
		type KbInputRecord
	} from '$lib/services/kbService';
	import SharedPdfViewer from '$lib/components/home3/shared-pdf-viewer.svelte';

	let { darkMode = true }: { darkMode: boolean } = $props();

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

	const fontSerif = "'Cormorant Garamond', 'Playfair Display', Georgia, serif";
	const fontMono = "'JetBrains Mono', 'IBM Plex Mono', monospace";
	const fontSans = "'Inter Tight', system-ui, sans-serif";

	let recordIdInput = $state('');
	let currentInput = $state<KbInputRecord | null>(null);
	let correctedFile = $state('');
	let lines = $state<DocStructureLine[]>([]);
	let selectedLineKey = $state<string | null>(null);
	let highlightSelectionVersion = $state(0);
	let selectedHighlightTarget = $state<{
		page: number;
		coords: number[];
		label: string;
		version: number;
	} | null>(null);
	let headingsOnly = $state(false);
	let loading = $state(false);
	let errorMsg = $state('');

	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);

	type PdfPageViewport = {
		width: number;
		height: number;
		convertToViewportRectangle: (rect: number[]) => number[];
	};

	let isPdf = $derived((currentInput?.type ?? '').toLowerCase() === 'pdf');
	let fileUrl = $derived.by(() => {
		if (!currentInput) return '';
		return `/api/v1/kb/inputs/${currentInput.id}/file#page=${docPage}&zoom=page-width`;
	});

	function lineKey(line: DocStructureLine): string {
		return `${line.page_number}:${line.line_number}`;
	}

	function effectiveLineType(line: DocStructureLine): string {
		const corrected = line.corrected_line_type?.trim().toLowerCase() ?? '';
		if (corrected !== '' && corrected !== 'unchanged') return corrected;
		return line.line_type?.trim().toLowerCase() ?? '';
	}

	function isHeadingLine(line: DocStructureLine): boolean {
		const t = effectiveLineType(line);
		return /^heading(?:-\d+)?$/i.test(t) || t.includes('heading');
	}

	let filteredLines = $derived.by(() =>
		headingsOnly ? lines.filter((ln) => isHeadingLine(ln)) : lines
	);

	let headingCount = $derived.by(() => lines.filter((ln) => isHeadingLine(ln)).length);

	let selectedLine = $derived.by(() => {
		if (!selectedLineKey) return null;
		return lines.find((ln) => lineKey(ln) === selectedLineKey) ?? null;
	});

	let selectedLinesByPage = $derived.by(() => {
		const map = new Map<number, DocStructureLine[]>();
		if (!selectedLine) return map;
		if (!Array.isArray(selectedLine.coords) || selectedLine.coords.length < 4) return map;
		map.set(selectedLine.page_number, [selectedLine]);
		return map;
	});

	let highlightCount = $derived.by(() => {
		let n = 0;
		for (const arr of selectedLinesByPage.values()) n += arr.length;
		return n;
	});

	function renderStructureHighlights(
		pageNo: number,
		viewport: PdfPageViewport,
		overlay: HTMLDivElement
	) {
		const target = selectedHighlightTarget;
		if (!target || target.page !== pageNo || target.coords.length < 4) return;
		const [vx1, vy1, vx2, vy2] = viewport.convertToViewportRectangle(target.coords.slice(0, 4));
		const left = Math.max(0, Math.min(vx1, vx2) - 5);
		const top = Math.max(0, Math.min(vy1, vy2) - 4);
		const width = Math.abs(vx2 - vx1) + 10;
		const height = Math.abs(vy2 - vy1) + 8;
		if (width < 1 || height < 1) return;
		const mark = document.createElement('div');
		mark.className = 'pdf-highlight';
		mark.style.left = `${left}px`;
		mark.style.top = `${top}px`;
		mark.style.width = `${width}px`;
		mark.style.height = `${height}px`;
		mark.title = target.label;
		overlay.appendChild(mark);
	}

	async function doRetrieve() {
		errorMsg = '';
		const id = Number(recordIdInput.trim());
		if (!Number.isFinite(id) || id <= 0) {
			errorMsg = 'Enter a valid Record ID';
			return;
		}
		loading = true;
		lines = [];
		currentInput = null;
		selectedLineKey = null;
		highlightSelectionVersion = 0;
		selectedHighlightTarget = null;
		docPage = 1;
		pdfZoom = 0.5;
		pdfNumPages = 0;
		correctedFile = '';
		try {
			const [structureRes, inputRes] = await Promise.all([
				getKbDocStructure(id),
				getKbInput(id).catch(() => null)
			]);
			lines = structureRes.lines ?? [];
			correctedFile = structureRes.corrected_file ?? '';
			currentInput = inputRes?.record ?? null;
			const first = (headingsOnly ? lines.filter((ln) => isHeadingLine(ln)) : lines)[0];
			if (first) {
				await selectLine(first);
			}
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Failed to retrieve document structure';
		} finally {
			loading = false;
		}
	}

	async function selectLine(line: DocStructureLine) {
		selectedLineKey = lineKey(line);
		highlightSelectionVersion += 1;
		selectedHighlightTarget =
			Array.isArray(line.coords) && line.coords.length >= 4
				? {
						page: line.page_number,
						coords: line.coords.slice(0, 4),
						label: `page ${line.page_number}, line ${line.line_number}`,
						version: highlightSelectionVersion
					}
				: null;
		if (line.page_number > 0) {
			docPage = line.page_number;
		}
	}
</script>

<div
	class="doc-structure"
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
		--font-serif:{fontSerif};
		--font-mono:{fontMono};
		--font-sans:{fontSans};
	"
>
	<header class="header">
		<div class="header-left">
			<div class="eyebrow">Knowledge Base · Vol. IV</div>
			<h1 class="display">Document Structure</h1>
			<div class="subtitle">Inspect corrected structure labels and highlight their source region in the PDF.</div>
		</div>
	</header>

	<div class="body">
		<aside class="left">
			<div class="left-controls">
				<label class="field">
					<span class="field-label">Record ID</span>
					<div class="field-row">
						<input
							type="text"
							inputmode="numeric"
							bind:value={recordIdInput}
							placeholder="e.g. 1042"
							onkeydown={(e) => {
								if (e.key === 'Enter') doRetrieve();
							}}
						/>
					</div>
				</label>

				<div class="btn-row">
					<button class="btn btn-primary retrieve" onclick={doRetrieve} disabled={loading}>
						{#if loading}
							<span class="spinner"></span>Retrieving…
						{:else}
							Retrieve
						{/if}
					</button>
					<button
						class="btn btn-secondary"
						type="button"
						class:active={headingsOnly}
						onclick={() => {
							headingsOnly = !headingsOnly;
						}}
					>
						Headings Only
					</button>
				</div>

				{#if errorMsg}<div class="error">{errorMsg}</div>{/if}
			</div>

			<div class="left-meta">
				<div class="left-meta-title">Lines</div>
				<div class="left-meta-count">{filteredLines.length} shown / {lines.length} total</div>
			</div>

			<div class="line-list">
				{#if !loading && filteredLines.length === 0}
					<div class="empty">
						<div class="empty-title">No lines loaded</div>
						<div class="empty-sub">
							Enter a Record ID to load a `.corrected` file.
							{#if headingsOnly}No heading lines found in current result.{/if}
						</div>
					</div>
				{:else}
					{#each filteredLines as line (`${line.page_number}-${line.line_number}`)}
						<button
							type="button"
							class="line-card"
							class:selected={selectedLineKey === lineKey(line)}
							onclick={() => selectLine(line)}
						>
							<div class="line-ref-row">
								<span class="line-ref">P{line.page_number}:L{line.line_number}</span>
								<span class="line-type-chip">{line.corrected_line_type}</span>
							</div>
							<div class="line-content">{line.content || '—'}</div>
							<div class="line-sub">
								orig: {line.line_type}
								{#if line.font}
									· {line.font} {line.font_size}
								{/if}
							</div>
						</button>
					{/each}
				{/if}
			</div>
		</aside>

		<section class="right">
			<div class="right-toolbar">
				<div class="title">
					<span class="name">{currentInput?.file_name ?? 'No document loaded'}</span>
					{#if currentInput}<span class="type">{currentInput.type}</span>{/if}
				</div>
				<div class="stats">
					<span>{highlightCount} mark</span>
					<span>{headingCount} headings</span>
				</div>
			</div>

			{#if !currentInput}
				<div class="doc-empty">Retrieve a record to display document and structure highlights.</div>
			{:else}
				{#if isPdf}
					<SharedPdfViewer
						inputId={currentInput.id}
						{fileUrl}
						bind:page={docPage}
						bind:zoom={pdfZoom}
						bind:numPages={pdfNumPages}
						highlightVersion={selectedHighlightTarget
							? `${selectedHighlightTarget.page}:${selectedHighlightTarget.coords.join(',')}:${selectedHighlightTarget.version}`
							: `${selectedLineKey ?? ''}:${highlightSelectionVersion}`}
						renderHighlights={renderStructureHighlights}
					>
						<div slot="sidebar">
							<aside class="meta-panel">
								<div class="meta-title">Selected Line</div>
								{#if !selectedLine}
									<div class="meta-empty">Select a line from the left panel.</div>
								{:else}
									<div class="meta-row"><span>Page</span><strong>{selectedLine.page_number}</strong></div>
									<div class="meta-row"><span>Line</span><strong>{selectedLine.line_number}</strong></div>
									<div class="meta-row">
										<span>Corrected</span><strong>{selectedLine.corrected_line_type}</strong>
									</div>
									<div class="meta-row"><span>Original</span><strong>{selectedLine.line_type}</strong></div>
									<div class="meta-row mono">
										[{selectedLine.coords.map((n) => Math.trunc(n)).join(', ')}]
									</div>
								{/if}
								{#if correctedFile}
									<div class="meta-title corrected-title">Corrected File</div>
									<div class="meta-file">{correctedFile}</div>
								{/if}
							</aside>
						</div>
					</SharedPdfViewer>
				{:else}
					<iframe class="doc-iframe" src={fileUrl} title="Document viewer"></iframe>
				{/if}
			{/if}
		</section>
	</div>
</div>

<style>
	.doc-structure {
		height: 100%;
		display: grid;
		grid-template-rows: auto 1fr;
		background: var(--page-bg);
		color: var(--text-primary);
		font-family: var(--font-sans);
	}
	.header {
		display: flex;
		justify-content: space-between;
		padding: 18px 22px;
		border-bottom: 1px solid var(--ink-line);
		background: linear-gradient(180deg, var(--panel-bg), var(--panel-bg-alt));
	}
	.eyebrow {
		font-size: 12px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.display {
		font-family: var(--font-serif);
		font-size: 32px;
		line-height: 1.1;
		margin: 6px 0 2px;
	}
	.subtitle {
		color: var(--text-secondary);
		font-size: 14px;
	}
	.body {
		display: grid;
		grid-template-columns: 420px 1fr;
		min-height: 0;
	}
	.left {
		border-right: 1px solid var(--ink-line);
		background: var(--panel-bg);
		display: grid;
		grid-template-rows: auto auto 1fr;
		min-height: 0;
	}
	.left-controls {
		padding: 14px;
		border-bottom: 1px solid var(--ink-line-soft);
	}
	.field-label {
		display: block;
		font-size: 12px;
		color: var(--text-muted);
		margin-bottom: 8px;
	}
	.field-row {
		display: flex;
		gap: 8px;
	}
	input {
		width: 100%;
		height: 38px;
		padding: 0 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border-radius: 10px;
	}
	.btn-row {
		display: grid;
		grid-template-columns: 1fr auto;
		gap: 8px;
		margin-top: 10px;
	}
	.btn {
		height: 38px;
		border: none;
		border-radius: 10px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn-primary {
		background: var(--brass);
		color: #1d1508;
	}
	.btn-secondary {
		padding: 0 12px;
		background: var(--panel-bg-alt);
		color: var(--text-secondary);
		border: 1px solid var(--ink-line);
	}
	.btn-secondary.active {
		color: var(--brass);
		border-color: var(--brass);
		box-shadow: 0 0 0 1px var(--brass-faint) inset;
	}
	.btn:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.left-meta {
		display: flex;
		justify-content: space-between;
		padding: 10px 14px;
		border-bottom: 1px solid var(--ink-line-soft);
		color: var(--text-secondary);
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}
	.line-list {
		overflow: auto;
		padding: 10px;
		display: grid;
		gap: 10px;
	}
	.line-card {
		border: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
		border-radius: 12px;
		padding: 10px;
		text-align: left;
		cursor: pointer;
		color: inherit;
	}
	.line-card.selected {
		border-color: var(--brass);
		box-shadow: 0 0 0 1px var(--brass-faint) inset;
	}
	.line-ref-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		margin-bottom: 6px;
	}
	.line-ref {
		font-family: var(--font-mono);
		font-size: 11px;
		padding: 2px 7px;
		border-radius: 999px;
		background: var(--brass-faint);
		color: var(--brass);
	}
	.line-type-chip {
		font-size: 11px;
		text-transform: uppercase;
		color: var(--text-muted);
		letter-spacing: 0.06em;
	}
	.line-content {
		font-size: 13px;
		line-height: 1.4;
		margin-bottom: 6px;
	}
	.line-sub {
		font-size: 12px;
		color: var(--text-muted);
	}
	.right {
		min-width: 0;
		display: grid;
		grid-template-rows: auto 1fr;
		min-height: 0;
	}
	.right-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 12px 14px;
		border-bottom: 1px solid var(--ink-line);
		background: var(--panel-bg);
	}
	.title {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}
	.name {
		font-weight: 700;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.type {
		color: var(--text-muted);
		text-transform: uppercase;
		font-size: 11px;
		letter-spacing: 0.06em;
	}
	.stats {
		display: flex;
		gap: 14px;
		color: var(--text-secondary);
		font-size: 12px;
	}
	.doc-empty {
		padding: 20px;
		color: var(--text-secondary);
	}
	.doc-page-bar {
		display: flex;
		gap: 8px;
		align-items: center;
		padding: 10px 14px;
		border-bottom: 1px solid var(--ink-line-soft);
		background: var(--panel-bg-alt);
	}
	.page-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 30px;
		height: 30px;
		border: 1px solid var(--ink-line);
		border-radius: 8px;
		background: var(--panel-bg);
		text-decoration: none;
		color: var(--text-primary);
	}
	.page-input {
		width: 72px;
		height: 30px;
	}
	.pdf-stage {
		min-height: 0;
		overflow: auto;
		padding: 14px;
	}
	.pdf-layout {
		display: grid;
		grid-template-columns: 300px 1fr;
		gap: 14px;
	}
	.meta-panel {
		background: var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		border-radius: 12px;
		padding: 10px;
		height: fit-content;
	}
	.meta-title {
		font-weight: 700;
		margin-bottom: 8px;
	}
	.corrected-title {
		margin-top: 12px;
	}
	.meta-row {
		display: flex;
		justify-content: space-between;
		gap: 8px;
		margin-bottom: 8px;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.meta-row strong {
		color: var(--text-primary);
	}
	.meta-row.mono {
		font-family: var(--font-mono);
		font-size: 12px;
		word-break: break-all;
	}
	.meta-file {
		font-family: var(--font-mono);
		font-size: 11px;
		line-height: 1.4;
		color: var(--text-muted);
		word-break: break-all;
	}
	.meta-empty {
		color: var(--text-muted);
		font-size: 12px;
	}
	.pdf-canvas-col {
		display: grid;
		gap: 14px;
	}
	.pdf-page {
		position: relative;
		background: #fff;
		width: fit-content;
		margin: 0 auto;
		box-shadow: 0 10px 30px rgba(0, 0, 0, 0.2);
	}
	.pdf-overlay {
		position: absolute;
		inset: 0;
		pointer-events: none;
	}
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(212, 162, 76, 0.3);
		outline: 1px solid rgba(212, 162, 76, 0.85);
		border-radius: 2px;
	}
	.error {
		margin-top: 10px;
		padding: 8px 10px;
		border-radius: 8px;
		font-size: 12px;
		background: rgba(200, 85, 61, 0.15);
		color: #f3b7ac;
	}
	.empty {
		padding: 18px 10px;
		text-align: center;
		color: var(--text-secondary);
	}
	.empty-title {
		font-weight: 700;
		margin-bottom: 4px;
	}
	.empty-sub {
		font-size: 12px;
		color: var(--text-muted);
	}
	.doc-iframe {
		width: 100%;
		height: 100%;
		border: none;
	}
	.spinner {
		display: inline-block;
		width: 14px;
		height: 14px;
		border: 2px solid rgba(0, 0, 0, 0.2);
		border-top-color: rgba(0, 0, 0, 0.7);
		border-radius: 999px;
		animation: spin 0.8s linear infinite;
		vertical-align: -2px;
		margin-right: 6px;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (max-width: 1200px) {
		.body {
			grid-template-columns: 1fr;
		}
		.left {
			max-height: 44vh;
			border-right: none;
			border-bottom: 1px solid var(--ink-line);
		}
		.pdf-layout {
			grid-template-columns: 1fr;
		}
	}
</style>
