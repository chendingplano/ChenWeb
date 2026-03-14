<script lang="ts">
	import XIcon from '@lucide/svelte/icons/x';
	import CpuIcon from '@lucide/svelte/icons/cpu';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import ClockIcon from '@lucide/svelte/icons/clock';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import TrendingUpIcon from '@lucide/svelte/icons/trending-up';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let {
		darkMode,
		open,
		width,
		activeMenu,
		onClose
	}: {
		darkMode: boolean;
		open: boolean;
		width: number;
		activeMenu: ActiveSelection | null;
		onClose: () => void;
	} = $props();

	// --- Layout constants ---
	const DRAWER_ANIM_DURATION = '250ms'; // slide animation
	const TOP_OFFSET = 244;              // HERO_HEADER_HEIGHT + NAV_BAR_HEIGHT

	// --- Colour tokens (all $derived from darkMode) ---
	let surface     = $derived(darkMode ? '#1E2535' : '#ffffff');       // card background
	let surface2    = $derived(darkMode ? '#252B3B' : '#f1f5f9');       // secondary surface
	let border      = $derived(darkMode ? '#2D3348' : '#E4E6EB');       // border / divider lines
	let textMain    = $derived(darkMode ? '#E2E8F0' : '#111827');       // primary text
	let textMuted   = $derived(darkMode ? '#64748b' : '#94a3b8');       // muted text
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');       // primary accent (indigo)
	let drawerBg    = $derived(darkMode ? '#171B26' : '#F2F4F7');       // drawer background

	let sectionId = $derived(activeMenu?.itemId ?? 'dashboard');

	const models = [
		{ name: 'Claude Sonnet 4.6', status: 'online', latency: '124ms', usage: 78 },
		{ name: 'GPT-4o', status: 'online', latency: '198ms', usage: 41 },
		{ name: 'Gemini Pro', status: 'degraded', latency: '450ms', usage: 12 }
	];

	const systemMetrics = [
		{ label: 'CPU', value: 23, unit: '%', color: '#22C55E' },
		{ label: 'Memory', value: 61, unit: '%', color: '#818CF8' },
		{ label: 'API Quota', value: 34, unit: '%', color: '#f59e0b' }
	];
</script>

<!-- Fixed right drawer -->
<div
	style="
		position:fixed;
		right:0;
		top:{TOP_OFFSET}px;
		height:calc(100vh - {TOP_OFFSET}px);
		width:{width}px;
		transform:translateX({open ? '0' : '100%'});
		transition:transform {DRAWER_ANIM_DURATION} ease;
		background:{drawerBg};
		border-left:1px solid {border};
		z-index:40;
		overflow-y:auto;
		scrollbar-width:thin;
		scrollbar-color:{border} transparent;
	"
>
	<!-- Header with close button -->
	<div
		class="flex items-center justify-between px-4 py-3 flex-shrink-0"
		style="border-bottom:1px solid {border}; position:sticky; top:0; background:{drawerBg}; z-index:1;"
	>
		<span class="text-xs font-semibold uppercase tracking-widest" style="color:{textMuted}; font-family:'Fira Code',monospace;">
			{#if sectionId === 'dashboard'}System Status
			{:else if sectionId === 'agents'}Agent Insights
			{:else if sectionId === 'skills'}Skill Analytics
			{:else if sectionId === 'applications'}App Status
			{:else if sectionId === 'coding'}Code Stats
			{:else if sectionId === 'knowledge'}KB Insights
			{:else}Context Info{/if}
		</span>
		<button
			onclick={onClose}
			class="flex items-center justify-center w-7 h-7 rounded-md transition-colors duration-150 cursor-pointer"
			style="color:{textMuted};"
			onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = border; (e.currentTarget as HTMLElement).style.color = textMain; }}
			onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.color = textMuted; }}
			aria-label="Close drawer"
		>
			<XIcon class="w-4 h-4" />
		</button>
	</div>

	<!-- Scrollable content -->
	<div class="p-4 space-y-4">
		{#if sectionId === 'dashboard' || !activeMenu}
			<!-- System Health card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-3">
					<CpuIcon class="w-4 h-4" style="color:#22C55E;" />
					<span class="text-sm font-semibold" style="color:{textMain};">System Health</span>
				</div>
				<div class="space-y-3">
					{#each systemMetrics as m}
						<div>
							<div class="flex justify-between mb-1">
								<span class="text-xs" style="color:{textMuted};">{m.label}</span>
								<span class="text-xs font-medium" style="color:{textMain}; font-family:'Fira Code',monospace;">{m.value}{m.unit}</span>
							</div>
							<div class="h-1.5 rounded-full" style="background:{border};">
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
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-3">
					<DatabaseIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textMain};">AI Models</span>
				</div>
				<div class="space-y-3">
					{#each models as m}
						<div class="flex items-center gap-3">
							<div
								class="w-2 h-2 rounded-full flex-shrink-0"
								style="background:{m.status === 'online' ? '#22C55E' : '#f59e0b'}; box-shadow:0 0 6px {m.status === 'online' ? '#22C55E' : '#f59e0b'}80;"
							></div>
							<div class="flex-1 min-w-0">
								<div class="text-xs font-medium truncate" style="color:{textMain};">{m.name}</div>
								<div class="text-xs" style="color:{textMuted}; font-family:'Fira Code',monospace;">{m.latency}</div>
							</div>
							<div class="text-xs" style="color:{textMuted};">{m.usage}%</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- Upcoming card -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-3">
					<ClockIcon class="w-4 h-4" style="color:#06b6d4;" />
					<span class="text-sm font-semibold" style="color:{textMain};">Upcoming</span>
				</div>
				<div class="space-y-2">
					{#each [
						{ time: '10:00', label: 'Team standup', type: 'meeting' },
						{ time: '14:00', label: 'Code review PR #142', type: 'task' },
						{ time: '16:30', label: 'Daily briefing digest', type: 'ai' }
					] as event}
						<div class="flex items-center gap-3 py-1.5">
							<span class="text-xs font-mono w-10 flex-shrink-0" style="color:{textMuted};">{event.time}</span>
							<div class="w-1 h-1 rounded-full flex-shrink-0" style="background:{accent};"></div>
							<span class="text-xs truncate" style="color:{textMain};">{event.label}</span>
						</div>
					{/each}
				</div>
			</div>

		{:else if sectionId === 'agents'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-4">
					<BotIcon class="w-4 h-4" style="color:{accent};" />
					<span class="text-sm font-semibold" style="color:{textMain};">Agent Overview</span>
				</div>
				<div class="grid grid-cols-2 gap-3">
					{#each [
						{ label: 'Total', value: '4', color: accent },
						{ label: 'Active', value: '2', color: '#818cf8' },
						{ label: 'Tasks Today', value: '37', color: '#f59e0b' },
						{ label: 'Success Rate', value: '98%', color: '#06b6d4' }
					] as stat}
						<div class="rounded-lg p-3 text-center" style="background:{surface}; border:1px solid {border};">
							<div class="text-lg font-bold" style="color:{stat.color}; font-family:'Fira Code',monospace;">{stat.value}</div>
							<div class="text-xs mt-0.5" style="color:{textMuted};">{stat.label}</div>
						</div>
					{/each}
				</div>
			</div>

			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-3">
					<TrendingUpIcon class="w-4 h-4" style="color:#f59e0b;" />
					<span class="text-sm font-semibold" style="color:{textMain};">Suggested Agents</span>
				</div>
				{#each ['DataAnalystBot', 'EmailCopilot', 'MeetingScribe'] as name}
					<div class="flex items-center gap-3 py-2" style="border-bottom:1px solid {border};">
						<div class="w-7 h-7 rounded-lg flex items-center justify-center" style="background:rgba(129,140,248,0.1);">
							<BotIcon class="w-3.5 h-3.5" style="color:{accent};" />
						</div>
						<span class="text-sm flex-1" style="color:{textMain};">{name}</span>
						<button class="text-xs px-2 py-0.5 rounded-md cursor-pointer" style="background:rgba(129,140,248,0.1); color:{accent};">
							Add
						</button>
					</div>
				{/each}
			</div>

		{:else if sectionId === 'skills'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-4">
					<ZapIcon class="w-4 h-4" style="color:#f59e0b;" />
					<span class="text-sm font-semibold" style="color:{textMain};">Usage This Month</span>
				</div>
				<div class="space-y-3">
					{#each [
						{ name: 'Summarizer', pct: 85 },
						{ name: 'Code Formatter', pct: 64 },
						{ name: 'PDF Extractor', pct: 45 },
						{ name: 'Web Scraper', pct: 28 }
					] as s}
						<div>
							<div class="flex justify-between mb-1">
								<span class="text-xs" style="color:{textMain};">{s.name}</span>
								<span class="text-xs" style="color:{textMuted}; font-family:'Fira Code',monospace;">{s.pct}%</span>
							</div>
							<div class="h-1.5 rounded-full" style="background:{border};">
								<div class="h-full rounded-full" style="width:{s.pct}%; background:#f59e0b;"></div>
							</div>
						</div>
					{/each}
				</div>
			</div>

		{:else if sectionId === 'coding'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-3">
					<ActivityIcon class="w-4 h-4" style="color:#f59e0b;" />
					<span class="text-sm font-semibold" style="color:{textMain};">Code Stats</span>
				</div>
				{#each [
					{ label: 'Reviews completed', value: '14', icon: CircleCheckIcon, color: '#22C55E' },
					{ label: 'Issues flagged', value: '38', icon: AlertCircleIcon, color: '#f59e0b' },
					{ label: 'Lines generated', value: '2.4k', icon: ZapIcon, color: accent }
				] as s}
					<div class="flex items-center gap-3 py-2.5" style="border-bottom:1px solid {border};">
						<s.icon class="w-4 h-4" style="color:{s.color};" />
						<span class="text-sm flex-1" style="color:{textMain};">{s.label}</span>
						<span class="text-sm font-bold" style="color:{s.color}; font-family:'Fira Code',monospace;">{s.value}</span>
					</div>
				{/each}
			</div>

		{:else if sectionId === 'knowledge'}
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="flex items-center gap-2 mb-3">
					<DatabaseIcon class="w-4 h-4" style="color:#06b6d4;" />
					<span class="text-sm font-semibold" style="color:{textMain};">KB Stats</span>
				</div>
				{#each [
					{ label: 'Total documents', value: '234' },
					{ label: 'Indexed chunks', value: '8.7k' },
					{ label: 'Searches today', value: '42' },
					{ label: 'Avg relevance', value: '94%' }
				] as s}
					<div class="flex justify-between py-2" style="border-bottom:1px solid {border};">
						<span class="text-sm" style="color:{textMuted};">{s.label}</span>
						<span class="text-sm font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">{s.value}</span>
					</div>
				{/each}
			</div>

		{:else}
			<!-- Coming soon placeholder -->
			<div class="rounded-xl p-4" style="background:{surface2}; border:1px solid {border};">
				<div class="text-sm font-semibold mb-2" style="color:{textMain};">
					{activeMenu?.itemTitle ?? 'Information'}
				</div>
				<p class="text-sm" style="color:{textMuted};">
					Context shelf content for this section is coming soon.
				</p>
			</div>
		{/if}
	</div>
</div>
