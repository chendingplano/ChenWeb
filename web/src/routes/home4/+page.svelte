<script lang="ts">
	import { goto } from '$app/navigation';
	import HeroHeader from '$lib/components/home4/hero-header.svelte';
	import TopNav from '$lib/components/home4/top-nav.svelte';
	import ContentPanel from '$lib/components/home4/content-panel.svelte';
	import RightDrawer from '$lib/components/home4/right-drawer.svelte';
	import CommandPalette from '$lib/components/home4/command-palette.svelte';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	// --- Layout dimension constants ---
	const HERO_HEADER_HEIGHT = 200; // hero header height in px
	const NAV_BAR_HEIGHT     = 44;  // top nav bar height in px
	const DRAWER_WIDTH       = 300; // right context drawer width in px (fixed)
	const CONTENT_OFFSET     = HERO_HEADER_HEIGHT + NAV_BAR_HEIGHT; // 244 — used in CSS calc()

	// Suppress unused constant warnings
	void CONTENT_OFFSET; void NAV_BAR_HEIGHT;

	// --- App state ---
	let darkMode            = $state(true);
	let activeMenu          = $state<ActiveSelection | null>({ itemId: 'dashboard', itemTitle: 'Dashboard' });
	let drawerOpen          = $state(false);
	let commandPaletteOpen  = $state(false);

	// --- ⌘K / Ctrl+K keyboard listener ---
	$effect(() => {
		function handleKeydown(e: KeyboardEvent) {
			if (!((e.metaKey || e.ctrlKey) && e.key === 'k')) return;
			// Do not intercept if focus is in a text input/textarea/contenteditable
			const target = e.target as HTMLElement;
			const tag = target.tagName.toLowerCase();
			if (tag === 'input' || tag === 'textarea' || target.isContentEditable) return;
			e.preventDefault();
			commandPaletteOpen = true;
		}
		document.addEventListener('keydown', handleKeydown);
		return () => document.removeEventListener('keydown', handleKeydown);
	});

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
</script>

<!-- Full-viewport container, flex-col, overflow hidden -->
<div
	class="flex flex-col overflow-hidden"
	style="height:100vh; background:{darkMode ? '#171B26' : '#F2F4F7'}; color:{darkMode ? '#E2E8F0' : '#111827'};"
>
	<!-- Hero Header (sticky, 200px) -->
	<div class="flex-shrink-0 sticky top-0 z-20">
		<HeroHeader
			{darkMode}
			height={HERO_HEADER_HEIGHT}
			onToggleDark={toggleDark}
		/>
	</div>

	<!-- Top Nav bar (sticky, 44px) -->
	<div class="flex-shrink-0 sticky z-20" style="top:{HERO_HEADER_HEIGHT}px;">
		<TopNav
			{darkMode}
			{activeMenu}
			{drawerOpen}
			onSelect={handleMenuSelect}
			onToggleDrawer={() => { drawerOpen = !drawerOpen; }}
			onOpenCommandPalette={() => { commandPaletteOpen = true; }}
		/>
	</div>

	<!-- Content panel (flex-1, scrollable) -->
	<ContentPanel
		{darkMode}
		{activeMenu}
		{drawerOpen}
		drawerWidth={DRAWER_WIDTH}
	/>

	<!-- Right drawer (fixed overlay, slides in from right) -->
	<RightDrawer
		{darkMode}
		open={drawerOpen}
		width={DRAWER_WIDTH}
		{activeMenu}
		onClose={() => { drawerOpen = false; }}
	/>

	<!-- Command palette modal -->
	<CommandPalette
		{darkMode}
		open={commandPaletteOpen}
		onClose={() => { commandPaletteOpen = false; }}
		onSelect={handleMenuSelect}
	/>
</div>
