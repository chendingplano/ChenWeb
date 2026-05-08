<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import SharedPdfViewer from './shared-pdf-viewer.svelte';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';
	import {
		clampPdfSidebarWidth,
		clampPdfViewWindowZoom,
		readPdfViewWindowSettings,
		writePdfViewWindowSettings
	} from './pdf-view-window-settings.js';

	let {
		inputId,
		fileUrl,
		page = $bindable(1),
		zoom = $bindable(0.5),
		numPages = $bindable(0),
		highlightVersion = 0,
		renderHighlights,
		loadingLabel = 'Rendering page…',
		respectPageRotation = true,
		sidebarMinWidth = 140,
		sidebarMaxWidth = 420,
		sidebarDefaultWidth = 270,
		sidebarTitle = 'Metadata',
		sidebarSettingsKey = null,
		sidebarWidthSettingLabel = 'Metadata Panel Width',
		zoomSettingLabel = 'Zoom',
		darkMode = true,
		showingLines = $bindable(false),
		selectedLines = $bindable([] as number[]),
		onselect,
		ondragmove,
		toolbar,
		linesView,
		sidebar
	}: {
		inputId: number | null;
		fileUrl: string;
		page?: number;
		zoom?: number;
		numPages?: number;
		highlightVersion?: number | string;
		renderHighlights?: (pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) => void;
		loadingLabel?: string;
		respectPageRotation?: boolean;
		sidebarMinWidth?: number;
		sidebarMaxWidth?: number;
		sidebarDefaultWidth?: number;
		sidebarTitle?: string;
		sidebarSettingsKey?: string | null;
		sidebarWidthSettingLabel?: string;
		zoomSettingLabel?: string;
		darkMode?: boolean;
		showingLines?: boolean;
		selectedLines?: number[];
		onselect?: (detail: {
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}) => void;
		ondragmove?: (detail: {
			pageNumber: number;
			viewportY1: number;
			viewportY2: number;
			viewport: PdfPageViewport;
		}) => void;
		toolbar?: Snippet;
		linesView?: Snippet;
		sidebar?: Snippet;
	} = $props();

	let pillSurface = $derived(darkMode ? 'rgba(15,23,42,0.55)' : 'rgba(255,255,255,0.72)');
	let pillBorder = $derived(darkMode ? 'rgba(148,163,184,0.18)' : 'rgba(100,116,139,0.18)');
	let pillText = $derived(darkMode ? '#94a3b8' : '#64748b');
	let pillHover = $derived(darkMode ? 'rgba(99,102,241,0.14)' : 'rgba(99,102,241,0.10)');

	let panelWidth = $state(sidebarDefaultWidth);
	let resizing = $state(false);
	let settingsOpen = $state(false);
	let storage: Storage | null = null;

	function sidebarWidthOptions() {
		return {
			minWidth: sidebarMinWidth,
			maxWidth: sidebarMaxWidth,
			defaultWidth: sidebarDefaultWidth
		};
	}

	function clamp(w: number): number {
		return clampPdfSidebarWidth(w, sidebarWidthOptions());
	}

	function savePanelWidth(nextWidth: number) {
		panelWidth = clamp(nextWidth);
		if (!sidebarSettingsKey || !storage) return;
		writePdfViewWindowSettings(
			storage,
			sidebarSettingsKey,
			{ sidebarWidth: panelWidth, zoom },
			sidebarWidthOptions()
		);
	}

	function zoomPercentLabel(value: number): string {
		return `${Math.round(value * 100)}%`;
	}

	function saveZoom(nextZoom: number) {
		zoom = clampPdfViewWindowZoom(nextZoom);
		if (!sidebarSettingsKey || !storage) return;
		writePdfViewWindowSettings(
			storage,
			sidebarSettingsKey,
			{ sidebarWidth: panelWidth, zoom },
			sidebarWidthOptions()
		);
	}

	function closeSettings() {
		settingsOpen = false;
	}

	function startResize(event: PointerEvent) {
		event.preventDefault();
		const handle = event.currentTarget as HTMLElement | null;
		const startX = event.clientX;
		const startWidth = panelWidth;
		resizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		handle?.setPointerCapture?.(event.pointerId);

		const handleMove = (e: PointerEvent) => {
			savePanelWidth(startWidth + (e.clientX - startX));
		};
		const handleUp = (e: PointerEvent) => {
			resizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			handle?.releasePointerCapture?.(e.pointerId);
			window.removeEventListener('pointermove', handleMove);
			window.removeEventListener('pointerup', handleUp);
			window.removeEventListener('pointercancel', handleUp);
		};

		window.addEventListener('pointermove', handleMove);
		window.addEventListener('pointerup', handleUp, { once: true });
		window.addEventListener('pointercancel', handleUp, { once: true });
	}

	function onResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			savePanelWidth(panelWidth - 16);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			savePanelWidth(panelWidth + 16);
		} else if (event.key === 'Home') {
			event.preventDefault();
			savePanelWidth(sidebarMinWidth);
		} else if (event.key === 'End') {
			event.preventDefault();
			savePanelWidth(sidebarMaxWidth);
		}
	}

	function onClickAway(e: MouseEvent) {
		if (selectedLines.length === 0) return;
		const target = e.target as HTMLElement;
		if (target.closest('.pvw-tool-btn')) return;
		selectedLines = [];
	}

	onMount(() => {
		storage = window.localStorage;
		const settings = readPdfViewWindowSettings(
			storage,
			sidebarSettingsKey,
			sidebarWidthOptions()
		);
		panelWidth = settings.sidebarWidth;
		zoom = settings.zoom;
		window.addEventListener('click', onClickAway, { capture: true });
		return () => window.removeEventListener('click', onClickAway, { capture: true });
	});

	$effect(() => {
		zoom = clampPdfViewWindowZoom(zoom);
	});
</script>

<div class="pvw-container">
	{#if showingLines && linesView}
		<div class="pvw-lines-host">
			{@render linesView()}
		</div>
	{:else}
		<SharedPdfViewer
			{inputId}
			{fileUrl}
			bind:page
			bind:zoom
			bind:numPages
			{highlightVersion}
			{renderHighlights}
			{loadingLabel}
			{respectPageRotation}
			{onselect}
			{ondragmove}
		>
			{#snippet pageBarTool()}
				{#if toolbar}
					<div
						class="pvw-toolbar-pill"
						style={`--pvw-surf:${pillSurface}; --pvw-bdc:${pillBorder}; --pvw-tc:${pillText}; --pvw-hvr:${pillHover};`}
					>
						{@render toolbar()}
					</div>
				{/if}
			{/snippet}
			{#snippet sidebarContent()}
				{#if sidebar}
					<div class="pvw-shell" style={`width:${panelWidth}px;`}>
						<aside class="pvw-sidebar-panel">
							<div class="pvw-sidebar-head">
								<div class="pvw-sidebar-title">{sidebarTitle}</div>
								<button
									type="button"
									class="pvw-settings-btn"
									class:active={settingsOpen}
									aria-expanded={settingsOpen}
									aria-label="Sidebar settings"
									onclick={() => {
										settingsOpen = !settingsOpen;
									}}
								>
									Settings
								</button>
							</div>
							<div class="pvw-sidebar-body">
								{@render sidebar()}
							</div>
						</aside>
						<button
							type="button"
							class="pvw-resizer"
							class:active={resizing}
							aria-label="Resize panel"
							onpointerdown={startResize}
							onkeydown={onResizerKeydown}
						>
							<span class="pvw-resizer-grip" aria-hidden="true"></span>
						</button>
					</div>
				{/if}
			{/snippet}
		</SharedPdfViewer>
	{/if}
</div>

{#if settingsOpen}
	<div
		class="pvw-settings-dialog-overlay"
		aria-hidden="true"
		onclick={closeSettings}
		onkeydown={(event) => {
			if (event.key === 'Escape') closeSettings();
		}}
	>
		<div
			class="pvw-settings-dialog"
			role="dialog"
			aria-modal="true"
			aria-label="PDF viewer settings"
			tabindex="0"
			onclick={(event) => event.stopPropagation()}
			onkeydown={(event) => event.stopPropagation()}
		>
			<div class="pvw-settings-dialog-head">
				<div class="pvw-settings-dialog-title">Settings</div>
				<button type="button" class="pvw-settings-close-btn" onclick={closeSettings}>Close</button>
			</div>
			<div class="pvw-settings-dialog-body">
				<label class="pvw-settings-field">
					<span class="pvw-settings-label">{sidebarWidthSettingLabel} ({panelWidth}px)</span>
					<input
						class="pvw-settings-range"
						type="range"
						min={sidebarMinWidth}
						max={sidebarMaxWidth}
						step="1"
						value={panelWidth}
						oninput={(event) => {
							savePanelWidth(Number((event.currentTarget as HTMLInputElement).value));
						}}
					/>
				</label>
				<label class="pvw-settings-field">
					<span class="pvw-settings-label">{zoomSettingLabel} ({zoomPercentLabel(zoom)})</span>
					<input
						class="pvw-settings-range"
						type="range"
						min="0.1"
						max="3"
						step="0.05"
						value={zoom}
						oninput={(event) => {
							saveZoom(Number((event.currentTarget as HTMLInputElement).value));
						}}
					/>
				</label>
			</div>
		</div>
	</div>
{/if}

<style>
	.pvw-container {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}

	.pvw-toolbar-pill {
		display: inline-flex;
		align-items: center;
		gap: 0;
		border-radius: 12px;
		border: 1px solid var(--pvw-bdc);
		background: var(--pvw-surf);
		backdrop-filter: blur(8px);
		padding: 3px;
		position: relative;
		z-index: 10;
	}

	.pvw-lines-host {
		flex: 1;
		min-height: 0;
		overflow: auto;
		background: var(--panel-bg);
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}

	.pvw-lines-host::-webkit-scrollbar {
		width: 10px;
	}
	.pvw-lines-host::-webkit-scrollbar-thumb {
		background: var(--ink-line);
		border-radius: 999px;
	}
	.pvw-lines-host::-webkit-scrollbar-track {
		background: transparent;
	}

	.pvw-shell {
		position: relative;
		flex: 0 0 auto;
		height: 100%;
		min-height: 0;
		max-height: 100%;
		align-self: stretch;
		padding-right: 16px;
	}

	.pvw-sidebar-panel {
		height: 100%;
		min-height: 0;
		max-height: 100%;
		display: flex;
		flex-direction: column;
		min-width: 0;
		background: var(--panel-bg);
		border: 1px solid var(--ink-line);
		overflow: hidden;
	}

	.pvw-sidebar-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 12px;
		border-bottom: 1px solid var(--ink-line-soft);
		background: var(--panel-bg);
		flex-shrink: 0;
	}

	.pvw-sidebar-title {
		font-family: var(--font-serif, serif);
		font-size: 24px;
		line-height: 1;
		color: var(--brass);
	}

	.pvw-settings-btn {
		height: 28px;
		padding: 0 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-secondary);
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		cursor: pointer;
	}

	.pvw-settings-btn:hover,
	.pvw-settings-btn.active,
	.pvw-settings-btn:focus-visible {
		border-color: var(--brass);
		color: var(--brass);
		outline: none;
	}

	.pvw-settings-field {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.pvw-settings-label {
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-muted);
	}

	.pvw-settings-range {
		width: 100%;
		accent-color: var(--brass);
	}

	.pvw-settings-dialog-overlay {
		position: fixed;
		inset: 0;
		z-index: 40;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 24px;
		background: rgba(9, 12, 17, 0.62);
		backdrop-filter: blur(4px);
	}

	.pvw-settings-dialog {
		width: min(420px, 100%);
		border: 1px solid var(--ink-line);
		background: var(--panel-bg);
		box-shadow: 0 24px 80px rgba(0, 0, 0, 0.38);
	}

	.pvw-settings-dialog-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 14px 16px;
		border-bottom: 1px solid var(--ink-line-soft);
	}

	.pvw-settings-dialog-title {
		font-family: var(--font-serif, serif);
		font-size: 22px;
		color: var(--brass);
		line-height: 1;
	}

	.pvw-settings-close-btn {
		height: 30px;
		padding: 0 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-secondary);
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		cursor: pointer;
	}

	.pvw-settings-close-btn:hover,
	.pvw-settings-close-btn:focus-visible {
		border-color: var(--brass);
		color: var(--brass);
		outline: none;
	}

	.pvw-settings-dialog-body {
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 16px;
	}

	.pvw-sidebar-body {
		flex: 1;
		min-height: 0;
		max-height: 100%;
		overflow-y: auto;
		overflow-x: hidden;
		padding: 12px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}

	.pvw-sidebar-body::-webkit-scrollbar {
		width: 10px;
	}

	.pvw-sidebar-body::-webkit-scrollbar-thumb {
		background: var(--ink-line);
		border-radius: 999px;
	}

	.pvw-sidebar-body::-webkit-scrollbar-track {
		background: transparent;
	}

	.pvw-resizer {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		min-height: 120px;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		user-select: none;
		touch-action: none;
		outline: none;
		z-index: 4;
	}

	.pvw-resizer::before {
		content: '';
		width: 1px;
		height: 100%;
		background: var(--ink-line);
		opacity: 0.8;
		transition: background 150ms ease;
	}

	.pvw-resizer:hover::before,
	.pvw-resizer.active::before,
	.pvw-resizer:focus-visible::before {
		background: var(--brass);
	}

	.pvw-resizer-grip {
		position: absolute;
		width: 8px;
		height: 52px;
		border-radius: 999px;
		background:
			radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
		transition:
			border-color 150ms ease,
			background-color 150ms ease;
	}

	.pvw-resizer:hover .pvw-resizer-grip,
	.pvw-resizer.active .pvw-resizer-grip,
	.pvw-resizer:focus-visible .pvw-resizer-grip {
		border-color: var(--brass);
		background:
			radial-gradient(circle, var(--brass) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
	}

	@media (max-width: 1200px) {
		.pvw-shell {
			position: static;
			height: auto;
			width: auto !important;
			padding-right: 0;
		}
		.pvw-sidebar-panel {
			height: auto;
		}
		.pvw-resizer {
			display: none;
		}
	}
</style>
