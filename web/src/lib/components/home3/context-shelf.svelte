<script lang="ts">
	import BotIcon         from '@lucide/svelte/icons/bot';
	import ZapIcon         from '@lucide/svelte/icons/zap';
	import ActivityIcon    from '@lucide/svelte/icons/activity';
	import CpuIcon         from '@lucide/svelte/icons/cpu';
	import DatabaseIcon    from '@lucide/svelte/icons/database';
	import ClockIcon       from '@lucide/svelte/icons/clock';
	import TrendingUpIcon  from '@lucide/svelte/icons/trending-up';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import XIcon           from '@lucide/svelte/icons/x';
	import FindingDetailsPanel from '$lib/components/home3/finding-details-panel.svelte';
	import { findingShelf } from '$lib/components/home3/finding-shelf-store.svelte';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let {
		darkMode    = true,
		activeMenu  = null,
		width       = 280,
		open        = true,
		onDragStart,
		onClose
	}: {
		darkMode:    boolean;
		activeMenu:  ActiveSelection | null;
		width:       number;
		open:        boolean;
		onDragStart: (e: MouseEvent) => void;
		onClose:     () => void;
	} = $props();

	// --- Layout constants ---
	const radiusCard = '12px'; // cards, panels

	// --- Design tokens ---
	let surface       = $derived(darkMode ? '#1F2333'  : '#FFFFFF');  // shelf background
	let surface2      = $derived(darkMode ? '#252A3A'  : '#ECEEF2');  // card surface inside shelf
	let borderColor   = $derived(darkMode ? '#2D3348'  : '#E4E6EB');  // border
	let accent        = $derived(darkMode ? '#818CF8'  : '#6366F1');  // primary accent
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');

	// Suppress unused props/consts (declared for API completeness / design-token reference)
	void radiusCard; void open;

	let sectionId = $derived(activeMenu?.itemId ?? 'dashboard');

	// Model status data
	const models = [
		{ name: 'Claude Sonnet 4.6', status: 'online',   latency: '124ms', usage: 78 },
		{ name: 'GPT-4o',            status: 'online',   latency: '198ms', usage: 41 },
		{ name: 'Gemini Pro',        status: 'degraded', latency: '450ms', usage: 12 }
	];

	const systemMetrics = [
		{ label: 'CPU',       value: 23, unit: '%', color: '#34D399' },
		{ label: 'Memory',    value: 61, unit: '%', color: '#818CF8' },
		{ label: 'API Quota', value: 34, unit: '%', color: '#FBBF24' }
	];

	function modelStatusColor(status: string): string {
		return status === 'online' ? '#34D399' : '#FBBF24';
	}

	const fontMono = "'Fira Code', 'Cascadia Code', monospace";
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<aside
	class="flex-shrink-0 flex flex-col overflow-y-auto relative"
	style="width:{width}px; background:{surface}; border-left:1px solid {borderColor}; scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
>
	<!-- Resize handle (left edge) -->
	<div
		class="absolute left-0 top-0 bottom-0 cursor-col-resize"
		style="width:4px; z-index:10; background:transparent;"
		onmousedown={onDragStart}
		onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accent + '40'; }}
		onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
	></div>

	<div class="p-4 space-y-4" style="padding-left:12px;">
		<!-- Header -->
		<div class="flex items-center justify-between pt-1 pl-1">
			<span class="text-xs font-semibold uppercase tracking-widest" style="color:{textMuted}; font-family:{fontMono};">
				{#if findingShelf.active}Finding
				{:else if sectionId === 'dashboard'}System Status
				{:else if sectionId === 'agents'}Agent Insights
				{:else if sectionId === 'skills'}Skill Analytics
				{:else if sectionId === 'applications'}App Status
				{:else if sectionId === 'coding'}Code Stats
				{:else if sectionId === 'personal'}Today's Plan
				{:else if sectionId === 'knowledge'}KB Insights
				{:else}Context Info
				{/if}
			</span>
			<button
				onclick={onClose}
				class="flex items-center justify-center w-6 h-6 rounded-md cursor-pointer transition-colors duration-150"
				style="color:{textMuted};"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; (e.currentTarget as HTMLElement).style.background = borderColor; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
				aria-label="Close context panel"
			>
				<XIcon class="w-3.5 h-3.5" />
			</button>
		</div>

		{#if findingShelf.active}
			<div class="finding-shelf-body">
				<FindingDetailsPanel
					finding={findingShelf.finding}
					requestId={findingShelf.requestId}
					runId={findingShelf.runId}
					dark={darkMode}
					onFocusMatchedUnit={findingShelf.onFocusMatchedUnit ?? undefined}
					onCloseMatchedUnits={findingShelf.onCloseMatchedUnits ?? undefined}
				/>
			</div>

		{:else if sectionId === 'dashboard' || !activeMenu}
			<!-- System Health card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<CpuIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">System Health</span>
				</div>
				<div class="space-y-3">
					{#each systemMetrics as m}
						<div>
							<div class="flex justify-between mb-1">
								<span class="text-xs" style="color:{textMuted};">{m.label}</span>
								<span class="text-xs font-medium" style="color:{textPrimary}; font-family:{fontMono};">{m.value}{m.unit}</span>
							</div>
							<div class="h-1.5 rounded-full" style="background:{borderColor};">
								<div
									class="h-full rounded-full transition-all duration-500"
									style="width:{m.value}%; background:{m.color}; box-shadow:0 0 6px {m.color}60;"
								></div>
							</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- AI Models card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<DatabaseIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">AI Models</span>
				</div>
				<div class="space-y-3">
					{#each models as m}
						<div class="flex items-center gap-3">
							<div
								class="w-2 h-2 rounded-full flex-shrink-0"
								style="background:{modelStatusColor(m.status)}; box-shadow:0 0 6px {modelStatusColor(m.status)}80;"
							></div>
							<div class="flex-1 min-w-0">
								<div class="text-xs font-medium truncate" style="color:{textPrimary};">{m.name}</div>
								<div class="text-xs" style="color:{textMuted}; font-family:{fontMono};">{m.latency}</div>
							</div>
							<div class="text-xs" style="color:{textMuted};">{m.usage}%</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- Upcoming card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<ClockIcon class="w-4 h-4" style="color:#06b6d4;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Upcoming</span>
				</div>
				<div class="space-y-1">
					{#each [
						{ time: '10:00', label: 'Team standup' },
						{ time: '14:00', label: 'Code review PR #142' },
						{ time: '16:30', label: 'Daily briefing digest' }
					] as event}
						<div class="flex items-center gap-3 py-1.5">
							<span class="text-xs w-10 flex-shrink-0" style="color:{textMuted}; font-family:{fontMono};">{event.time}</span>
							<div class="w-1 h-1 rounded-full flex-shrink-0" style="background:{accent};"></div>
							<span class="text-xs truncate" style="color:{textPrimary};">{event.label}</span>
						</div>
					{/each}
				</div>
			</div>

		{:else if sectionId === 'agents'}
			<!-- Agent stats 2x2 grid -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-4">
					<BotIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Agent Overview</span>
				</div>
				<div class="grid grid-cols-2 gap-3">
					{#each [
						{ label: 'Total',        value: '4',   color: accent },
						{ label: 'Active',       value: '2',   color: '#34D399' },
						{ label: 'Tasks Today',  value: '37',  color: '#FBBF24' },
						{ label: 'Success Rate', value: '98%', color: '#06b6d4' }
					] as stat}
						<div class="rounded-lg p-3 text-center" style="background:{darkMode ? '#1F2333' : '#FFFFFF'}; border:1px solid {borderColor};">
							<div class="text-lg font-bold" style="color:{stat.color}; font-family:{fontMono};">{stat.value}</div>
							<div class="text-xs mt-0.5" style="color:{textMuted};">{stat.label}</div>
						</div>
					{/each}
				</div>
			</div>
			<!-- Suggested agents -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<TrendingUpIcon class="w-4 h-4" style="color:#FBBF24;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Suggested Agents</span>
				</div>
				{#each ['DataAnalystBot', 'EmailCopilot', 'MeetingScribe'] as name}
					<div class="flex items-center gap-3 py-2" style="border-bottom:1px solid {borderColor};">
						<div class="w-7 h-7 rounded-lg flex items-center justify-center" style="background:{accent}20;">
							<BotIcon class="w-3.5 h-3.5" style="color:{accent};" />
						</div>
						<span class="text-sm flex-1" style="color:{textPrimary};">{name}</span>
						<button class="text-xs px-2 py-0.5 rounded-md cursor-pointer" style="background:{accent}15; color:{accent}; border:none;">Add</button>
					</div>
				{/each}
			</div>

		{:else if sectionId === 'skills'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-4">
					<ZapIcon class="w-4 h-4" style="color:#FBBF24;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Usage This Month</span>
				</div>
				<div class="space-y-3">
					{#each [
						{ name: 'Summarizer',      pct: 85 },
						{ name: 'Code Formatter',  pct: 64 },
						{ name: 'PDF Extractor',   pct: 45 },
						{ name: 'Web Scraper',     pct: 28 }
					] as s}
						<div>
							<div class="flex justify-between mb-1">
								<span class="text-xs" style="color:{textPrimary};">{s.name}</span>
								<span class="text-xs" style="color:{textMuted}; font-family:{fontMono};">{s.pct}%</span>
							</div>
							<div class="h-1.5 rounded-full" style="background:{borderColor};">
								<div class="h-full rounded-full" style="width:{s.pct}%; background:{accent};"></div>
							</div>
						</div>
					{/each}
				</div>
			</div>

		{:else if sectionId === 'coding'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<ActivityIcon class="w-4 h-4" style="color:#FBBF24;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">Code Stats</span>
				</div>
				{#each [
					{ label: 'Reviews completed', value: '14', icon: CircleCheckIcon, color: '#34D399' },
					{ label: 'Issues flagged',    value: '38', icon: AlertCircleIcon, color: '#FBBF24' },
					{ label: 'Lines generated',   value: '2.4k', icon: ZapIcon,        color: accent }
				] as s}
					<div class="flex items-center gap-3 py-2.5" style="border-bottom:1px solid {borderColor};">
						<s.icon class="w-4 h-4" style="color:{s.color};" />
						<span class="text-sm flex-1" style="color:{textPrimary};">{s.label}</span>
						<span class="text-sm font-bold" style="color:{s.color}; font-family:{fontMono};">{s.value}</span>
					</div>
				{/each}
			</div>

		{:else if sectionId === 'knowledge'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<DatabaseIcon class="w-4 h-4" style="color:#06b6d4;" />
					<span class="text-sm font-semibold" style="color:{textPrimary};">KB Stats</span>
				</div>
				{#each [
					{ label: 'Total documents', value: '234' },
					{ label: 'Indexed chunks',  value: '8.7k' },
					{ label: 'Searches today',  value: '42' },
					{ label: 'Avg relevance',   value: '94%' }
				] as s}
					<div class="flex justify-between py-2" style="border-bottom:1px solid {borderColor};">
						<span class="text-sm" style="color:{textMuted};">{s.label}</span>
						<span class="text-sm font-semibold" style="color:{textPrimary}; font-family:{fontMono};">{s.value}</span>
					</div>
				{/each}
			</div>

		{:else}
			<!-- Generic context -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="text-sm font-semibold mb-2" style="color:{textPrimary};">
					{activeMenu?.itemTitle ?? 'Information'}
				</div>
				<p class="text-sm" style="color:{textMuted};">
					Select an item from the navigation to see contextual information here.
				</p>
			</div>
		{/if}
	</div>
</aside>
