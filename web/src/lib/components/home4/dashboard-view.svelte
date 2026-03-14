<script lang="ts">
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import PlayIcon from '@lucide/svelte/icons/play';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import SearchIcon from '@lucide/svelte/icons/search';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import ClockIcon from '@lucide/svelte/icons/clock';
	import CheckCircleIcon from '@lucide/svelte/icons/check-circle';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// --- Colour tokens (all $derived from darkMode) ---
	let surface     = $derived(darkMode ? '#1E2535' : '#ffffff');       // card background
	let surface2    = $derived(darkMode ? '#252B3B' : '#f1f5f9');       // secondary surface
	let border      = $derived(darkMode ? '#2D3348' : '#E4E6EB');       // border / divider lines
	let textMain    = $derived(darkMode ? '#E2E8F0' : '#111827');       // primary text
	let textMuted   = $derived(darkMode ? '#64748b' : '#94a3b8');       // muted / secondary text
	let textSub     = $derived(darkMode ? '#94a3b8' : '#64748b');       // subdued text
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');       // primary accent (indigo)
	let accentTint  = $derived(darkMode ? 'rgba(129,140,248,0.10)' : 'rgba(99,102,241,0.08)'); // accent tint bg

	const stats = [
		{ label: 'Active Agents', value: '3', delta: '+1', up: true, icon: BotIcon, color: '#818CF8' },
		{ label: 'Running Tasks', value: '12', delta: '+5', up: true, icon: ActivityIcon, color: '#818cf8' },
		{ label: 'Skills Loaded', value: '47', delta: '-2', up: false, icon: ZapIcon, color: '#f59e0b' },
		{ label: 'Knowledge Docs', value: '234', delta: '+18', up: true, icon: BookOpenIcon, color: '#06b6d4' }
	];

	const recentActivity = [
		{ id: 1, type: 'agent', msg: 'Agent "ResearchBot" completed web search task', time: '2m ago', status: 'success' },
		{ id: 2, type: 'code', msg: 'Code review finished for auth.go — 3 issues found', time: '8m ago', status: 'warning' },
		{ id: 3, type: 'skill', msg: 'Skill "PDF Extractor" invoked by Personal Assistant', time: '15m ago', status: 'success' },
		{ id: 4, type: 'kb', msg: 'Indexed 12 new documents into Knowledge Base', time: '32m ago', status: 'success' },
		{ id: 5, type: 'agent', msg: 'Agent "SchedulerBot" started daily briefing pipeline', time: '1h ago', status: 'running' },
		{ id: 6, type: 'code', msg: 'Refactoring task queued for routes.go', time: '2h ago', status: 'pending' }
	];

	const quickActions = [
		{ label: 'New Agent', icon: BotIcon, color: '#818CF8' },
		{ label: 'Run Skill', icon: PlayIcon, color: '#818cf8' },
		{ label: 'Search KB', icon: SearchIcon, color: '#06b6d4' },
		{ label: 'Code Review', icon: CodeIcon, color: '#f59e0b' },
		{ label: 'Add Document', icon: PlusIcon, color: '#ec4899' },
		{ label: 'Schedule Task', icon: ClockIcon, color: '#f97316' }
	];

	const agentStatus = [
		{ name: 'ResearchBot', model: 'Claude Sonnet 4.6', status: 'active', tasks: 4 },
		{ name: 'SchedulerBot', model: 'GPT-4o', status: 'active', tasks: 2 },
		{ name: 'CodeReviewBot', model: 'Claude Sonnet 4.6', status: 'idle', tasks: 0 }
	];

	function statusColor(status: string) {
		if (status === 'success') return '#22C55E';
		if (status === 'warning') return '#f59e0b';
		if (status === 'running') return '#818cf8';
		return '#64748b';
	}
</script>

<div class="p-6 space-y-6">
	<!-- Greeting -->
	<div>
		<h2 class="text-xl font-semibold" style="color:{textMain}; font-family:'Fira Code',monospace;">
			Good morning, Alex 👋
		</h2>
		<p class="text-sm mt-1" style="color:{textMuted};">
			Here's what's happening with your AI workspace today.
		</p>
	</div>

	<!-- Stats grid — 4 KPI cards -->
	<div class="grid grid-cols-2 gap-4 xl:grid-cols-4">
		{#each stats as stat}
			<div
				class="rounded-xl p-4 transition-all duration-200 cursor-default"
				style="background:{surface}; border:1px solid {border};"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = stat.color + '50'; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = border; }}
			>
				<div class="flex items-start justify-between mb-3">
					<div
						class="flex items-center justify-center w-9 h-9 rounded-lg"
						style="background:{stat.color}20;"
					>
						<stat.icon class="w-4 h-4" style="color:{stat.color};" />
					</div>
					<div
						class="flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full"
						style="color:{stat.up ? '#22C55E' : '#f43f5e'}; background:{stat.up ? 'rgba(34,197,94,0.1)' : 'rgba(244,63,94,0.1)'};"
					>
						{#if stat.up}
							<ArrowUpIcon class="w-3 h-3" />
						{:else}
							<ArrowDownIcon class="w-3 h-3" />
						{/if}
						{stat.delta}
					</div>
				</div>
				<div class="text-2xl font-bold" style="color:{textMain}; font-family:'Fira Code',monospace;">
					{stat.value}
				</div>
				<div class="text-xs mt-1" style="color:{textMuted};">{stat.label}</div>
			</div>
		{/each}
	</div>

	<!-- Middle row: Activity feed (60%) + Quick Launch (40%) -->
	<div class="grid gap-6 lg:grid-cols-5">
		<!-- Activity feed (60%) -->
		<div class="lg:col-span-3">
			<h3 class="text-sm font-semibold uppercase tracking-widest mb-3" style="color:{textMuted}; font-family:'Fira Code',monospace;">
				Recent Activity
			</h3>
			<div
				class="rounded-xl overflow-hidden"
				style="border:1px solid {border};"
			>
				{#each recentActivity as item, i}
					<div
						class="flex items-start gap-3 px-4 py-3 transition-colors duration-150"
						style="background:{i % 2 === 0 ? surface : surface2 + '80'}; border-bottom:{i < recentActivity.length - 1 ? `1px solid ${border}` : 'none'};"
					>
						<div
							class="mt-0.5 w-2 h-2 rounded-full flex-shrink-0"
							style="background:{statusColor(item.status)}; box-shadow: 0 0 6px {statusColor(item.status)}60;"
						></div>
						<div class="flex-1 min-w-0">
							<p class="text-sm" style="color:{textMain};">{item.msg}</p>
						</div>
						<div class="flex items-center gap-1.5 flex-shrink-0">
							{#if item.status === 'running'}
								<div class="w-1.5 h-1.5 rounded-full" style="background:{accent}; animation:pulse 1.5s infinite;"></div>
							{:else if item.status === 'success'}
								<CheckCircleIcon class="w-3.5 h-3.5" style="color:#22C55E;" />
							{/if}
							<span class="text-xs" style="color:{textMuted}; font-family:'Fira Code',monospace;">{item.time}</span>
						</div>
					</div>
				{/each}
			</div>
		</div>

		<!-- Quick Launch (40%) -->
		<div class="lg:col-span-2">
			<h3 class="text-sm font-semibold uppercase tracking-widest mb-3" style="color:{textMuted}; font-family:'Fira Code',monospace;">
				Quick Actions
			</h3>
			<div class="grid grid-cols-3 gap-3">
				{#each quickActions as action}
					<button
						class="flex flex-col items-center gap-2 rounded-xl p-3 transition-all duration-200 cursor-pointer"
						style="background:{surface}; border:1px solid {border};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accentTint; (e.currentTarget as HTMLElement).style.borderColor = action.color + '40'; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = surface; (e.currentTarget as HTMLElement).style.borderColor = border; }}
					>
						<div
							class="flex items-center justify-center w-8 h-8 rounded-lg"
							style="background:{action.color}20;"
						>
							<action.icon class="w-4 h-4" style="color:{action.color};" />
						</div>
						<span class="text-xs font-medium" style="color:{textSub};">{action.label}</span>
					</button>
				{/each}
			</div>
		</div>
	</div>

	<!-- Agent status mini-list (bottom) -->
	<div>
		<h3 class="text-sm font-semibold uppercase tracking-widest mb-3" style="color:{textMuted}; font-family:'Fira Code',monospace;">
			Agent Status
		</h3>
		<div class="rounded-xl overflow-hidden" style="border:1px solid {border};">
			{#each agentStatus as agent, i}
				<div
					class="flex items-center gap-4 px-4 py-3"
					style="background:{i % 2 === 0 ? surface : surface2 + '80'}; border-bottom:{i < agentStatus.length - 1 ? `1px solid ${border}` : 'none'};"
				>
					<div
						class="w-2 h-2 rounded-full flex-shrink-0"
						style="background:{agent.status === 'active' ? '#22C55E' : border}; box-shadow:{agent.status === 'active' ? '0 0 6px #22C55E80' : 'none'};"
					></div>
					<div class="flex-1 min-w-0">
						<div class="text-sm font-medium" style="color:{textMain};">{agent.name}</div>
						<div class="text-xs" style="color:{textMuted}; font-family:'Fira Code',monospace;">{agent.model}</div>
					</div>
					<span
						class="text-xs px-2 py-0.5 rounded-full font-medium"
						style="background:{agent.status === 'active' ? accentTint : surface2}; color:{agent.status === 'active' ? accent : textMuted};"
					>
						{agent.status}
					</span>
					<span class="text-xs flex-shrink-0" style="color:{textMuted}; font-family:'Fira Code',monospace;">
						{agent.tasks} tasks
					</span>
				</div>
			{/each}
		</div>
	</div>
</div>

<style>
	@keyframes pulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.4; }
	}
</style>
