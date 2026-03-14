<script lang="ts">
	import { goto } from '$app/navigation';
	import HeroHeader from '$lib/components/home3/hero-header.svelte';
	import NavRail from '$lib/components/home3/nav-rail.svelte';
	import ContentPanel from '$lib/components/home3/content-panel.svelte';
	import ContextShelf from '$lib/components/home3/context-shelf.svelte';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	// --- Layout dimensions (adjust here to change panel sizes) ---
	const HERO_HEADER_HEIGHT = 200; // hero header height in px — increase for more visual impact
	const RAIL_WIDTH_COLLAPSED = 56; // collapsed icon-rail width in px
	const RAIL_WIDTH_DEFAULT = 240; // default expanded rail width in px
	const RAIL_WIDTH_MIN = 180; // minimum draggable rail width in px
	const RAIL_WIDTH_MAX = 320; // maximum draggable rail width in px
	const SHELF_WIDTH_DEFAULT = 280; // default context shelf width in px
	const SHELF_WIDTH_MIN = 220; // minimum draggable shelf width in px
	const SHELF_WIDTH_MAX = 480; // maximum draggable shelf width in px

	// --- Shadows (adjust here to change depth/elevation) ---
	const shadowCard = '0 1px 3px rgba(0,0,0,0.08), 0 1px 2px rgba(0,0,0,0.04)'; // resting card
	const shadowCardDark = '0 1px 3px rgba(0,0,0,0.30)'; // resting card (dark)
	const shadowHover = '0 4px 12px rgba(0,0,0,0.10)'; // card on hover
	const shadowDropdown = '0 8px 24px rgba(0,0,0,0.12)'; // floating panels

	// --- Border radii (adjust here to change roundedness — used in child components) ---
	const radiusCard = '12px'; // cards, panels
	const radiusButton = '8px'; // buttons, inputs, tags
	const radiusChip = '6px'; // small status chips, badges
	void radiusCard;

	// --- Animation durations (adjust here to change transition feel) ---
	const durationHover = '150ms'; // hover colour transitions
	const durationPanel = '200ms'; // panel expand/collapse (rail, shelf)
	const durationAurora = '18s'; // aurora blob drift cycle (slower = calmer)

	// Design tokens exported for reference — used by child components or at callsites
	// (TypeScript may warn on unused; values are intentionally declared here as single source of truth)
	const _tokens = {
		shadowCard,
		shadowCardDark,
		shadowHover,
		shadowDropdown,
		radiusButton,
		radiusChip,
		durationHover,
		durationAurora
	};
	void _tokens;

	// --- App state ---
	let darkMode = $state(true);
	let railWidth = $state(RAIL_WIDTH_DEFAULT); // expanded width, preserved across pin/unpin
	let railPinned = $state(false); // when unpinned, rail collapses on mouseleave
	let railExpanded = $state(false); // tracks hover state when unpinned
	let shelfWidth = $state(SHELF_WIDTH_DEFAULT); // context shelf width
	let shelfOpen = $state(true); // context shelf visibility
	let activeMenu = $state<ActiveSelection | null>({ itemId: 'dashboard', itemTitle: 'Dashboard' });

	// --- Rail collapse/restore for full-screen panels (e.g. canvas-01) ---
	let prevRailWidth = $state(RAIL_WIDTH_DEFAULT);
	let prevRailPinned = $state(false);

	function handleCollapseRail() {
		prevRailWidth = railWidth;
		prevRailPinned = railPinned;
		railWidth = RAIL_WIDTH_COLLAPSED;
		railPinned = false;
		railExpanded = false;
	}

	function handleRestoreRail() {
		railWidth = prevRailWidth;
		railPinned = prevRailPinned;
		railExpanded = prevRailPinned;
	}

	// --- Drag resize state ---
	let isDraggingRail = false;
	let isDraggingShelf = false;
	let dragStartX = 0;
	let dragStartWidth = 0;

	function startRailDrag(e: MouseEvent) {
		isDraggingRail = true;
		dragStartX = e.clientX;
		dragStartWidth = railWidth;
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		e.preventDefault();
	}

	function startShelfDrag(e: MouseEvent) {
		isDraggingShelf = true;
		dragStartX = e.clientX;
		dragStartWidth = shelfWidth;
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		e.preventDefault();
	}

	function onMouseMove(e: MouseEvent) {
		if (isDraggingRail) {
			const delta = e.clientX - dragStartX;
			railWidth = Math.max(RAIL_WIDTH_MIN, Math.min(RAIL_WIDTH_MAX, dragStartWidth + delta));
		}
		if (isDraggingShelf) {
			// Dragging shelf left edge: leftward drag = wider shelf
			const delta = dragStartX - e.clientX;
			shelfWidth = Math.max(SHELF_WIDTH_MIN, Math.min(SHELF_WIDTH_MAX, dragStartWidth + delta));
		}
	}

	function onMouseUp() {
		isDraggingRail = false;
		isDraggingShelf = false;
		document.removeEventListener('mousemove', onMouseMove);
		document.removeEventListener('mouseup', onMouseUp);
	}

	function toggleDark() {
		darkMode = !darkMode;
	}

	function handleMenuSelect(selection: ActiveSelection) {
		if (selection.itemId === '__logout__') {
			handleLogout();
			return;
		}
		activeMenu = selection;
	}

	async function handleLogout() {
		try {
			const res = await fetch('/auth/logout', { method: 'DELETE', credentials: 'same-origin' });
			if (res.ok) goto('/login');
		} catch (err) {
			console.error('Logout error:', err);
		}
	}

	function handleTogglePin() {
		railPinned = !railPinned;
		if (!railPinned) railExpanded = false;
	}

	function handleRailHoverChange(hovered: boolean) {
		if (!railPinned) railExpanded = hovered;
	}

	// Derived colours for the root layout dividers
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB'); // border / divider lines
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1'); // primary accent (indigo)

	// Effective rail width for layout (kept for reference; actual width computed inside NavRail)
	const _railCollapsed = RAIL_WIDTH_COLLAPSED;
	void _railCollapsed;
	// let effectiveRailWidth = $derived((railExpanded || railPinned) ? railWidth : RAIL_WIDTH_COLLAPSED);
	// void effectiveRailWidth;
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="flex flex-col overflow-hidden"
	style="height:100vh; background:{darkMode ? '#171B26' : '#F2F4F7'}; color:{darkMode
		? '#E2E8F0'
		: '#111827'};"
>
	<!-- Hero Header (fixed height) -->
	<HeroHeader {darkMode} height={HERO_HEADER_HEIGHT} onToggleDark={toggleDark} />

	<!-- Main body: rail + content + shelf -->
	<div class="flex flex-1 overflow-hidden select-none">
		<!-- Nav Rail -->
		<NavRail
			{darkMode}
			{activeMenu}
			pinned={railPinned}
			expanded={railExpanded}
			width={railWidth}
			onSelect={handleMenuSelect}
			onTogglePin={handleTogglePin}
			onWidthDragStart={startRailDrag}
			onHoverChange={handleRailHoverChange}
		/>

		<!-- Rail resize divider (only when pinned) -->
		{#if railPinned}
			<div
				class="group flex flex-shrink-0 cursor-col-resize items-center justify-center"
				style="width:4px; background:{borderColor}; transition:background {durationPanel};"
				onmousedown={startRailDrag}
				onmouseenter={(e) => {
					(e.currentTarget as HTMLElement).style.background = accent + '50';
				}}
				onmouseleave={(e) => {
					(e.currentTarget as HTMLElement).style.background = borderColor;
				}}
				title="Drag to resize rail"
			>
				<div
					class="flex flex-col gap-1 opacity-0 transition-opacity duration-150 group-hover:opacity-100"
				>
					{#each Array(4) as _}
						<div class="h-1 w-1 rounded-full" style="background:{accent};"></div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Content Panel (scrollable) -->
		<ContentPanel
			{darkMode}
			{activeMenu}
			{shelfOpen}
			onToggleShelf={() => {
				shelfOpen = !shelfOpen;
			}}
			onCollapseRail={handleCollapseRail}
			onRestoreRail={handleRestoreRail}
		/>

		<!-- Shelf resize divider (only when open) -->
		{#if shelfOpen}
			<div
				class="group flex flex-shrink-0 cursor-col-resize items-center justify-center"
				style="width:4px; background:{borderColor}; transition:background {durationPanel};"
				onmousedown={startShelfDrag}
				onmouseenter={(e) => {
					(e.currentTarget as HTMLElement).style.background = accent + '50';
				}}
				onmouseleave={(e) => {
					(e.currentTarget as HTMLElement).style.background = borderColor;
				}}
				title="Drag to resize shelf"
			>
				<div
					class="flex flex-col gap-1 opacity-0 transition-opacity duration-150 group-hover:opacity-100"
				>
					{#each Array(4) as _}
						<div class="h-1 w-1 rounded-full" style="background:{accent};"></div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Context Shelf -->
		{#if shelfOpen}
			<ContextShelf
				{darkMode}
				{activeMenu}
				width={shelfWidth}
				open={shelfOpen}
				onDragStart={startShelfDrag}
				onClose={() => {
					shelfOpen = false;
				}}
			/>
		{/if}
	</div>
</div>
