<script lang="ts">
	import { page } from '$app/state';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import SearchIcon from '@lucide/svelte/icons/search';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import ListTreeIcon from '@lucide/svelte/icons/list-tree';
	import BarChart3Icon from '@lucide/svelte/icons/bar-chart-3';
	import BoxesIcon from '@lucide/svelte/icons/boxes';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import KbImportView from '$lib/components/home3/kb-import-view.svelte';
	import MetricMgmtView from '$lib/components/home3/metric-mgmt-view.svelte';
	import InputsMgmtView from '$lib/components/home3/inputs-mgmt-view.svelte';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';
	import ChunkMgmtView from '$lib/components/home3/chunk-mgmt-view.svelte';
	import KnowledgeStoreView from '$lib/components/home3/knowledge-store-view.svelte';

	type KbSectionId =
		| 'kb-search'
		| 'kb-import'
		| 'kb-input-details'
		| 'kb-metrics'
		| 'kb-doc-structure'
		| 'kb-chunks';

	type KbMenuItem = {
		id: KbSectionId;
		label: string;
		description: string;
		icon: any;
	};

	const menuItems: KbMenuItem[] = [
		{ id: 'kb-search', label: 'Knowledge Stores', description: 'Explore indexed knowledge', icon: SearchIcon },
		{ id: 'kb-import', label: 'Documents', description: 'Review imported records', icon: FileTextIcon },
		{ id: 'kb-input-details', label: 'Document Details', description: 'Inspect source inputs', icon: DatabaseIcon },
		{ id: 'kb-metrics', label: 'Metrics', description: 'Manage extracted metrics', icon: BarChart3Icon },
		{
			id: 'kb-doc-structure',
			label: 'Document Structure',
			description: 'Inspect parsed hierarchy',
			icon: ListTreeIcon
		},
		{ id: 'kb-chunks', label: 'Chunks', description: 'Browse chunk output', icon: BoxesIcon }
	];

	let darkMode = $derived(page.url.searchParams.get('dark') !== '0');
	let initialSection = $derived((page.url.searchParams.get('section') as KbSectionId | null) ?? 'kb-search');
	let activeSection = $state<KbSectionId>('kb-search');

	$effect(() => {
		if (menuItems.some((item) => item.id === initialSection)) {
			activeSection = initialSection;
		}
	});

	let activeItem = $derived(menuItems.find((item) => item.id === activeSection) ?? menuItems[3]);
	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let panelBg = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let contentBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.6)' : 'rgba(228,230,235,0.7)');

	function selectSection(id: KbSectionId) {
		activeSection = id;
	}
</script>

<div class="kb-page flex overflow-hidden" style="background:{pageBg}; color:{textPrimary};">
	<aside
		class="kb-menu flex flex-col overflow-hidden"
		style="background:{panelBg}; border-right:1px solid {borderColor};"
	>
		<div class="flex items-center gap-3 px-4 py-4" style="border-bottom:1px solid {borderColor};">
			<div
				class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg"
				style="background:{accentTint}; color:{accent}; border:1px solid {accent}30;"
			>
				<BookOpenIcon class="h-5 w-5" />
			</div>
			<div class="min-w-0">
				<div class="truncate" style="font-size:14px; font-weight:700; color:{textPrimary};">
					Knowledge System
				</div>
				<div class="truncate" style="font-size:12px; color:{textMuted};">
					Documents, metrics, structure
				</div>
			</div>
		</div>

		<nav class="flex-1 overflow-y-auto px-2 py-3" style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;">
			{#each menuItems as item (item.id)}
				<button
					type="button"
					onclick={() => selectSection(item.id)}
					class="mb-1 flex w-full cursor-pointer items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors duration-150"
					style="
						background:{activeSection === item.id ? accentTint : 'transparent'};
						color:{activeSection === item.id ? accent : textSecondary};
						border:none;
						border-left:2px solid {activeSection === item.id ? accent : 'transparent'};
					"
					onmouseenter={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (activeSection !== item.id) el.style.background = hoverBg;
						el.style.color = textPrimary;
					}}
					onmouseleave={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (activeSection !== item.id) el.style.background = 'transparent';
						el.style.color = activeSection === item.id ? accent : textSecondary;
					}}
				>
					<item.icon class="h-5 w-5 flex-shrink-0" />
					<span class="min-w-0">
						<span class="block truncate" style="font-size:14px; font-weight:600;">{item.label}</span>
						<span class="block truncate" style="font-size:12px; color:{textMuted};">{item.description}</span>
					</span>
				</button>
			{/each}
		</nav>
	</aside>

	<main class="flex min-w-0 flex-1 flex-col overflow-hidden" style="background:{contentBg};">
		<header
			class="flex flex-shrink-0 items-center justify-between px-6 py-3"
			style="background:{contentBg}; border-bottom:1px solid {borderColor};"
		>
			<div class="min-w-0">
				<div class="truncate" style="font-size:13px; color:{textMuted};">Knowledge System</div>
				<h1 class="truncate" style="font-size:18px; font-weight:700; color:{textPrimary};">
					{activeItem.label}
				</h1>
			</div>
		</header>

		<div class="min-h-0 flex-1 overflow-hidden">
			{#if activeSection === 'kb-search'}
				<KnowledgeStoreView {darkMode} />
			{:else if activeSection === 'kb-import'}
				<KbImportView {darkMode} />
			{:else if activeSection === 'kb-input-details'}
				<InputsMgmtView {darkMode} />
			{:else if activeSection === 'kb-metrics'}
				<MetricMgmtView {darkMode} />
			{:else if activeSection === 'kb-doc-structure'}
				<DocStructureView {darkMode} />
			{:else if activeSection === 'kb-chunks'}
				<ChunkMgmtView {darkMode} />
			{/if}
		</div>
	</main>
</div>

<style>
	.kb-page {
		height: 100vh;
	}

	.kb-menu {
		width: 280px;
		flex: 0 0 280px;
	}

	@media (max-width: 760px) {
		.kb-page {
			flex-direction: column;
		}

		.kb-menu {
			width: 100%;
			flex: 0 0 auto;
			max-height: 44vh;
			border-right: none !important;
		}
	}
</style>
