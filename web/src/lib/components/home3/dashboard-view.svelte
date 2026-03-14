<script lang="ts">
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PlayIcon from '@lucide/svelte/icons/play';
	import FileIcon from '@lucide/svelte/icons/file';
	import MessageSquareIcon from '@lucide/svelte/icons/message-square';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import InfoIcon from '@lucide/svelte/icons/info';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// --- Layout constants ---
	const radiusCard = '12px'; // cards, panels
	const radiusButton = '8px'; // buttons, inputs
	const shadowCard = '0 1px 3px rgba(0,0,0,0.08), 0 1px 2px rgba(0,0,0,0.04)'; // resting card (light)
	const shadowCardDark = '0 1px 3px rgba(0,0,0,0.30)'; // resting card (dark)

	// --- Typography ---
	const fontMono = "'Fira Code', 'Cascadia Code', monospace"; // data / badges

	// --- Design tokens ---
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF'); // card surface
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2'); // secondary surface
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB'); // border
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1'); // primary accent (indigo)
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let colorSuccess = $derived(darkMode ? '#34D399' : '#10B981');
	let colorWarning = $derived(darkMode ? '#FBBF24' : '#F59E0B');
	let shadow = $derived(darkMode ? shadowCardDark : shadowCard);

	// KPI cards
	const kpiCards = [
		{ label: 'Active Agents', value: 3, icon: BotIcon, color: '#818CF8', colorDark: '#818CF8' },
		{
			label: 'Running Tasks',
			value: 12,
			icon: ActivityIcon,
			color: '#F59E0B',
			colorDark: '#FBBF24'
		},
		{ label: 'Skills Loaded', value: 47, icon: ZapIcon, color: '#10B981', colorDark: '#34D399' },
		{
			label: 'Knowledge Docs',
			value: 234,
			icon: BookOpenIcon,
			color: '#06b6d4',
			colorDark: '#06b6d4'
		}
	];

	function kpiColor(c: { color: string; colorDark: string }): string {
		return darkMode ? c.colorDark : c.color;
	}

	// Activity feed
	const activities = [
		{
			type: 'success',
			icon: CircleCheckIcon,
			message: 'DataAnalystBot completed report synthesis',
			time: '2m ago',
			status: 'success'
		},
		{
			type: 'warning',
			icon: AlertCircleIcon,
			message: 'GPT-4o latency spike detected (450ms)',
			time: '8m ago',
			status: 'warning'
		},
		{
			type: 'info',
			icon: InfoIcon,
			message: 'PDF Extractor skill updated to v1.4.0',
			time: '15m ago',
			status: 'info'
		},
		{
			type: 'success',
			icon: CircleCheckIcon,
			message: 'Knowledge base indexed 14 new documents',
			time: '32m ago',
			status: 'success'
		},
		{
			type: 'info',
			icon: InfoIcon,
			message: 'Code Review session started for PR #218',
			time: '1h ago',
			status: 'info'
		},
		{
			type: 'success',
			icon: CircleCheckIcon,
			message: 'EmailCopilot drafted 3 responses',
			time: '2h ago',
			status: 'success'
		}
	];

	function activityStatusColor(status: string): string {
		if (status === 'success') return colorSuccess;
		if (status === 'warning') return colorWarning;
		return accent;
	}

	// Quick Launch
	const quickLaunch = [
		{ label: 'New Agent', icon: BotIcon, color: '#818CF8' },
		{ label: 'Run Skill', icon: PlayIcon, color: '#10B981' },
		{ label: 'Import Doc', icon: FileIcon, color: '#06b6d4' },
		{ label: 'Chat', icon: MessageSquareIcon, color: '#F59E0B' },
		{ label: 'Review Code', icon: CodeIcon, color: '#8B5CF6' },
		{ label: 'Schedule', icon: CalendarIcon, color: '#EC4899' }
	];

	// Agent status mini-list
	const agents = [
		{
			name: 'DataAnalystBot',
			model: 'Claude Sonnet 4.6',
			status: 'active',
			lastActivity: '2m ago'
		},
		{ name: 'EmailCopilot', model: 'GPT-4o', status: 'active', lastActivity: '15m ago' },
		{ name: 'MeetingScribe', model: 'Claude Sonnet 4.6', status: 'idle', lastActivity: '2h ago' }
	];

	function agentStatusColor(status: string): string {
		if (status === 'active') return colorSuccess;
		if (status === 'idle') return colorWarning;
		return textMuted;
	}
</script>

<div class="space-y-5 p-6">
	<!-- 1. Stats row: 4 KPI cards -->
	<div class="grid grid-cols-4 gap-4">
		{#each kpiCards as card}
			<div
				class="flex items-center gap-3 rounded-xl p-4"
				style="background:{cardBg}; border:1px solid {borderColor}; border-radius:{radiusCard}; box-shadow:{shadow};"
			>
				<div
					class="flex flex-shrink-0 items-center justify-center rounded-lg"
					style="width:40px; height:40px; background:{kpiColor(card)}20; border:1px solid {kpiColor(
						card
					)}30;"
				>
					<card.icon class="h-5 w-5" style="color:{kpiColor(card)};" />
				</div>
				<div>
					<div
						style="font-size:28px; font-weight:700; color:{kpiColor(
							card
						)}; font-family:{fontMono}; line-height:1;"
					>
						{card.value}
					</div>
					<div style="font-size:12px; color:{textMuted}; margin-top:2px;">{card.label}</div>
				</div>
			</div>
		{/each}
	</div>

	<!-- 2. Middle row: Activity feed + Quick Launch -->
	<div class="flex gap-4">
		<!-- Activity feed (60%) -->
		<div
			class="flex-1 rounded-xl p-4"
			style="background:{cardBg}; border:1px solid {borderColor}; border-radius:{radiusCard}; box-shadow:{shadow}; flex:0 0 60%;"
		>
			<div class="mb-3 flex items-center justify-between">
				<span style="font-size:14px; font-weight:600; color:{textPrimary};">Activity Feed</span>
				<span style="font-size:11px; color:{textMuted};">Recent events</span>
			</div>
			<div class="space-y-2">
				{#each activities as evt}
					<div
						role="presentation"
						class="flex items-start gap-3 rounded-lg p-2.5 transition-colors duration-150"
						style="border:1px solid {borderColor};"
						onmouseenter={(e) => {
							(e.currentTarget as HTMLElement).style.background = surface2;
						}}
						onmouseleave={(e) => {
							(e.currentTarget as HTMLElement).style.background = 'transparent';
						}}
					>
						<evt.icon
							class="mt-0.5 h-4 w-4 flex-shrink-0"
							style="color:{activityStatusColor(evt.status)};"
						/>
						<span class="flex-1 text-sm leading-snug" style="color:{textSecondary}; font-size:13px;"
							>{evt.message}</span
						>
						<div class="flex flex-shrink-0 items-center gap-2">
							<span style="font-size:11px; color:{textMuted}; white-space:nowrap;">{evt.time}</span>
							<span
								class="rounded-full px-1.5 py-0.5 text-xs"
								style="background:{activityStatusColor(evt.status)}20; color:{activityStatusColor(
									evt.status
								)}; font-size:10px; white-space:nowrap;">{evt.status}</span
							>
						</div>
					</div>
				{/each}
			</div>
		</div>

		<!-- Quick Launch (40%) -->
		<div
			class="rounded-xl p-4"
			style="background:{cardBg}; border:1px solid {borderColor}; border-radius:{radiusCard}; box-shadow:{shadow}; flex:0 0 calc(40% - 1rem);"
		>
			<div class="mb-3">
				<span style="font-size:14px; font-weight:600; color:{textPrimary};">Quick Launch</span>
			</div>
			<div class="grid grid-cols-2 gap-2">
				{#each quickLaunch as action}
					<button
						class="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2.5 text-left transition-colors duration-150"
						style="background:{surface2}; border:1px solid {borderColor}; border-radius:{radiusButton}; color:{textSecondary};"
						onmouseenter={(e) => {
							const el = e.currentTarget as HTMLElement;
							el.style.background = accentTint;
							el.style.borderColor = accent + '40';
							el.style.color = textPrimary;
						}}
						onmouseleave={(e) => {
							const el = e.currentTarget as HTMLElement;
							el.style.background = surface2;
							el.style.borderColor = borderColor;
							el.style.color = textSecondary;
						}}
					>
						<action.icon class="h-4 w-4 flex-shrink-0" style="color:{action.color};" />
						<span style="font-size:13px; font-weight:500;">{action.label}</span>
					</button>
				{/each}
			</div>
			<!-- "New" button -->
			<button
				class="mt-3 flex w-full cursor-pointer items-center justify-center gap-2 rounded-lg py-2 transition-colors duration-150"
				style="background:{accent}; color:white; border-radius:{radiusButton}; font-size:13px; font-weight:600; border:none;"
				onmouseenter={(e) => {
					(e.currentTarget as HTMLElement).style.opacity = '0.9';
				}}
				onmouseleave={(e) => {
					(e.currentTarget as HTMLElement).style.opacity = '1';
				}}
			>
				<PlusIcon class="h-4 w-4" />
				New Agent
			</button>
		</div>
	</div>

	<!-- 3. Agent status mini-list -->
	<div
		class="rounded-xl p-4"
		style="background:{cardBg}; border:1px solid {borderColor}; border-radius:{radiusCard}; box-shadow:{shadow};"
	>
		<div class="mb-3 flex items-center justify-between">
			<span style="font-size:14px; font-weight:600; color:{textPrimary};">Agent Status</span>
			<span style="font-size:11px; color:{accent}; cursor:pointer;">View all →</span>
		</div>
		<div class="overflow-x-auto">
			<table class="w-full" style="border-collapse:collapse;">
				<thead>
					<tr style="border-bottom:1px solid {borderColor};">
						<th
							class="pb-2 text-left"
							style="font-size:11px; color:{textMuted}; font-weight:500; padding-right:24px;"
							>AGENT</th
						>
						<th
							class="pb-2 text-left"
							style="font-size:11px; color:{textMuted}; font-weight:500; padding-right:24px;"
							>MODEL</th
						>
						<th
							class="pb-2 text-left"
							style="font-size:11px; color:{textMuted}; font-weight:500; padding-right:24px;"
							>STATUS</th
						>
						<th class="pb-2 text-left" style="font-size:11px; color:{textMuted}; font-weight:500;"
							>LAST ACTIVE</th
						>
					</tr>
				</thead>
				<tbody>
					{#each agents as agent}
						<tr style="border-bottom:1px solid {borderColor};">
							<td
								class="py-2.5"
								style="font-size:13px; font-weight:500; color:{textPrimary}; padding-right:24px;"
								>{agent.name}</td
							>
							<td class="py-2.5" style="padding-right:24px;">
								<span
									class="rounded-full px-2 py-0.5"
									style="font-size:11px; font-family:{fontMono}; background:{accentTint}; color:{accent};"
									>{agent.model}</span
								>
							</td>
							<td class="py-2.5" style="padding-right:24px;">
								<div class="flex items-center gap-1.5">
									<div
										class="h-2 w-2 rounded-full"
										style="background:{agentStatusColor(agent.status)};"
									></div>
									<span style="font-size:13px; color:{agentStatusColor(agent.status)};"
										>{agent.status}</span
									>
								</div>
							</td>
							<td class="py-2.5" style="font-size:12px; color:{textMuted};">{agent.lastActivity}</td
							>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
</div>
