<script lang="ts">
	import { goto } from '$app/navigation';
	import AIHeroHeader from '$lib/components/home2/ai-hero-header.svelte';
	import AISidebar from '$lib/components/home2/ai-sidebar.svelte';
	import ContentPanel from '$lib/components/home2/content-panel.svelte';
	import RightPanel from '$lib/components/home2/right-panel.svelte';


	export type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let darkMode = $state(true);
	let activeMenu = $state<ActiveSelection | null>(null);

	// Panel widths (px), constrained between min/max
	let sidebarWidth = $state(260);
	let rightPanelWidth = $state(300);

	const SIDEBAR_MIN = 180;
	const SIDEBAR_MAX = 420;
	const RIGHT_MIN = 200;
	const RIGHT_MAX = 500;

	let isDraggingLeft = false;
	let isDraggingRight = false;
	let dragStartX = 0;
	let dragStartWidth = 0;

	function startDragLeft(e: MouseEvent) {
		isDraggingLeft = true;
		dragStartX = e.clientX;
		dragStartWidth = sidebarWidth;
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		e.preventDefault();
	}

	function startDragRight(e: MouseEvent) {
		isDraggingRight = true;
		dragStartX = e.clientX;
		dragStartWidth = rightPanelWidth;
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		e.preventDefault();
	}

	function onMouseMove(e: MouseEvent) {
		if (isDraggingLeft) {
			const delta = e.clientX - dragStartX;
			sidebarWidth = Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, dragStartWidth + delta));
		}
		if (isDraggingRight) {
			// Dragging right-panel left-edge: leftward drag = wider right panel
			const delta = dragStartX - e.clientX;
			rightPanelWidth = Math.max(RIGHT_MIN, Math.min(RIGHT_MAX, dragStartWidth + delta));
		}
	}

	function onMouseUp() {
		isDraggingLeft = false;
		isDraggingRight = false;
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

	let borderColor = $derived(darkMode ? '#1E293B' : '#e2e8f0');
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="flex h-screen flex-col overflow-hidden transition-colors duration-300"
	style="background:{darkMode ? '#020617' : '#f8fafc'}; color:{darkMode ? '#F8FAFC' : '#0f172a'};"
>
	<AIHeroHeader {darkMode} onToggleDark={toggleDark} />

	<div class="flex flex-1 overflow-hidden select-none">
		<!-- Left sidebar -->
		<AISidebar onMenuSelect={handleMenuSelect} {activeMenu} {darkMode} width={sidebarWidth} />

		<!-- Left resize divider -->
		<div
			class="group flex-shrink-0 flex items-center justify-center cursor-col-resize"
			style="width:5px; background:{borderColor}; position:relative; transition: background 150ms;"
			onmousedown={startDragLeft}
			onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = '#22C55E50'; }}
			onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = borderColor; }}
			title="Drag to resize"
		>
			<!-- Grab indicator dots -->
			<div class="flex flex-col gap-1 opacity-0 group-hover:opacity-100 transition-opacity duration-150">
				{#each Array(4) as _}
					<div class="w-1 h-1 rounded-full" style="background:#22C55E;"></div>
				{/each}
			</div>
		</div>

		<!-- Middle content -->
		<ContentPanel {activeMenu} {darkMode} />

		<!-- Right resize divider -->
		<div
			class="group flex-shrink-0 flex items-center justify-center cursor-col-resize"
			style="width:5px; background:{borderColor}; position:relative; transition: background 150ms;"
			onmousedown={startDragRight}
			onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = '#22C55E50'; }}
			onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = borderColor; }}
			title="Drag to resize"
		>
			<div class="flex flex-col gap-1 opacity-0 group-hover:opacity-100 transition-opacity duration-150">
				{#each Array(4) as _}
					<div class="w-1 h-1 rounded-full" style="background:#22C55E;"></div>
				{/each}
			</div>
		</div>

		<!-- Right panel -->
		<RightPanel {activeMenu} {darkMode} width={rightPanelWidth} />
	</div>

</div>
