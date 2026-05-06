<script lang="ts">
	import type { Snippet } from 'svelte';
	import SharedPdfViewer from './shared-pdf-viewer.svelte';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';

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
		sidebar?: Snippet;
	} = $props();

	let panelWidth = $state(sidebarDefaultWidth);
	let resizing = $state(false);

	function clamp(w: number): number {
		return Math.min(sidebarMaxWidth, Math.max(sidebarMinWidth, Math.round(w)));
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
			panelWidth = clamp(startWidth + (e.clientX - startX));
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
			panelWidth = clamp(panelWidth - 16);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			panelWidth = clamp(panelWidth + 16);
		} else if (event.key === 'Home') {
			event.preventDefault();
			panelWidth = sidebarMinWidth;
		} else if (event.key === 'End') {
			event.preventDefault();
			panelWidth = sidebarMaxWidth;
		}
	}
</script>

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
>
	<div slot="sidebar">
		{#if sidebar}
			<div class="pvw-shell" style={`width:${panelWidth}px;`}>
				{@render sidebar()}
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
	</div>
</SharedPdfViewer>

<style>
	.pvw-shell {
		position: relative;
		flex: 0 0 auto;
		height: 100%;
		padding-right: 16px;
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
		.pvw-resizer {
			display: none;
		}
	}
</style>
