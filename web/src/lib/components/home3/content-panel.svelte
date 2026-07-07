<script lang="ts">
	import DashboardView from '$lib/components/home3/dashboard-view.svelte';
	import DocProcessorDashboardView from '$lib/components/home3/doc-processor-dashboard-view.svelte';
	import AppFooter     from '$lib/components/home3/app-footer.svelte';
	import KbImportView  from '$lib/components/home3/kb-import-view.svelte';
	import MetricMgmtView from '$lib/components/home3/metric-mgmt-view.svelte';
	import DocGenView    from '$lib/components/home3/doc-gen-view.svelte';
import DocumentReviewView from '$lib/components/home3/document-review-view.svelte';
	import PromptOptimizerView from '$lib/components/home3/prompt-optimizer-view.svelte';
	import OpenMetadataWorkspace from '$lib/components/home3/openmetadata-workspace.svelte';
	import JetStreamLogsView from '$lib/components/home3/jetstream-logs-view.svelte';
	import DocProcLogsView from '$lib/components/home3/doc-proc-logs-view.svelte';
	import LLMUsageLogsView from '$lib/components/home3/llm-usage-logs-view.svelte';
	import JetStreamEventsView from '$lib/components/home3/jetstream-events-view.svelte';
	import JetStreamSubjectsView from '$lib/components/home3/jetstream-subjects-view.svelte';
	import DiaryView from '$lib/components/home3/diary-view.svelte';
	import ResearchTopicsView from '$lib/components/home3/research-topics-view.svelte';
	import LLMActivitiesView from '$lib/components/home3/llm-activities-view.svelte';
	import LLMAccountsView from '$lib/components/home3/llm-accounts-view.svelte';
	import LLMModelProfilesView from '$lib/components/home3/llm-model-profiles-view.svelte';
	import LLMModelsView from '$lib/components/home3/llm-models-view.svelte';
	import DbConsistencyView from '$lib/components/home3/db-consistency-view.svelte';
	import DbMaintLogView from '$lib/components/home3/db-maint-log-view.svelte';
	import ResolveAmbiguousObjectsView from '$lib/components/home3/resolve-ambiguous-objects-view.svelte';
	import KanbanBoardView from '$lib/components/home3/kanban-board-view.svelte';
	import AgentsView from '$lib/components/home3/agents-view.svelte';
	import ProjectsView from '$lib/components/home3/projects-view.svelte';
	import SkillMgmtView from '$lib/components/home3/skill-mgmt-view.svelte';
	import KbSearchLabView from '$lib/components/home3/kb-search-lab-view.svelte';
	import Canvas01      from '$lib/components/shared-ui/canvas-01.svelte';
	import Chatter01     from '$lib/components/shared-ui/chatter-01.svelte';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import PanelRightIcon   from '@lucide/svelte/icons/panel-right';
	import BotIcon          from '@lucide/svelte/icons/bot';
	import ZapIcon          from '@lucide/svelte/icons/zap';
	import LayoutGridIcon   from '@lucide/svelte/icons/layout-grid';
	import CodeIcon         from '@lucide/svelte/icons/code-2';
	import UserIcon         from '@lucide/svelte/icons/user';
	import BookOpenIcon     from '@lucide/svelte/icons/book-open';
	import SettingsIcon     from '@lucide/svelte/icons/settings';
	import InfoIcon         from '@lucide/svelte/icons/info';
	import ShieldIcon       from '@lucide/svelte/icons/shield';
	import BookMarkedIcon   from '@lucide/svelte/icons/book-marked';
	import BrainIcon        from '@lucide/svelte/icons/brain';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	let {
		darkMode       = true,
		activeMenu     = null,
		shelfOpen      = true,
		autoShrinkExpand = false,
		docReviewKey   = 0,
		railOffset     = 56,
		onToggleShelf,
		onAutoShrinkExpandChange = (_enabled: boolean) => {},
	}: {
		darkMode:        boolean;
		activeMenu:      ActiveSelection | null;
		shelfOpen:       boolean;
		autoShrinkExpand?: boolean;
		docReviewKey?:   number;
		railOffset?:     number;
		onToggleShelf:   () => void;
		onAutoShrinkExpandChange?: (enabled: boolean) => void;
	} = $props();

	// --- Layout constants ---
	const radiusCard   = '12px'; // cards, panels
	const radiusButton = '8px';  // buttons

	// --- Design tokens ---
	let pageBg        = $derived(darkMode ? '#171B26'  : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333'  : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A'  : '#ECEEF2');
	let borderColor   = $derived(darkMode ? '#2D3348'  : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8'  : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');

	// Section icon map
	const sectionIcons: Record<string, any> = {
		agents:       BotIcon,
		chat:         BotIcon,
		skills:       ZapIcon,
		applications: LayoutGridIcon,
		coding:       CodeIcon,
		personal:     UserIcon,
		knowledge:    BookOpenIcon,
		settings:     SettingsIcon,
		about:        InfoIcon,
		'system-admin': ShieldIcon,
		jetstream: ShieldIcon,
		'my-workspace': BookMarkedIcon,
		'knowledge-engineering': BrainIcon
	};

	// Section descriptions
	const sectionDesc: Record<string, string> = {
		agents:       'Manage, browse, and create AI agents for your workflows.',
		chat:         'Chat with multiple agents and models in session tabs.',
		skills:       'Discover and manage modular skills that extend your agents.',
		applications: 'Connect and manage third-party app integrations.',
		coding:       'AI-powered coding assistance: review, generate, and debug.',
		personal:     'Your personal AI assistant for tasks, calendar, and email.',
		knowledge:    'Your document library and semantic search knowledge base.',
		settings:     'Configure your workspace, models, and integrations.',
		about:        'Version info, credits, and system information.',
		jetstream: 'Operational monitoring and diagnostics for JetStream services.',
		'system-admin': 'System administration: JetStream monitoring, doc processor logs, and diagnostics.',
		'my-workspace': 'Your personal workspace: diary, notes, and resources.',
		'knowledge-engineering': 'Manage research topic beans: articles, thoughts, designs, specs, and readings.'
	};

	let sectionId = $derived(activeMenu?.itemId ?? 'dashboard');
	let isDashboard = $derived(sectionId === 'dashboard' || !activeMenu);
</script>

<main
	class="relative flex-1 overflow-y-auto flex flex-col min-w-0"
	style="background:{pageBg}; scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
>
	<!-- Topbar: breadcrumb + shelf toggle -->
	{#if !isDashboard}
		<div
			class="flex items-center justify-between px-6 py-3 flex-shrink-0 sticky top-0"
			style="background:{pageBg}; border-bottom:1px solid {borderColor}; z-index:5;"
		>
			<!-- Breadcrumb -->
			<nav class="flex items-center gap-1.5" style="font-size:13px; color:{textMuted};">
				<span>Home</span>
				<ChevronRightIcon class="w-3 h-3" />
				<span style="color:{textSecondary};">{activeMenu?.itemTitle}</span>
				{#if activeMenu?.childTitle}
					<ChevronRightIcon class="w-3 h-3" />
					<span style="color:{textPrimary}; font-weight:500;">{activeMenu.childTitle}</span>
				{/if}
			</nav>
			<!-- Shelf toggle -->
			<button
				onclick={onToggleShelf}
				class="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 cursor-pointer transition-colors duration-150"
				style="background:{accentTint}; color:{accent}; font-size:12px; border:none;"
				aria-label={shelfOpen ? 'Close context panel' : 'Open context panel'}
			>
				<PanelRightIcon class="w-3.5 h-3.5" />
				{shelfOpen ? 'Close panel' : 'Open panel'}
			</button>
		</div>
	{/if}

	<!-- Main content area -->
	<div class="flex-1">
		{#if activeMenu?.childId === 'doc-processor-dashboard'}
			<DocProcessorDashboardView {darkMode} />
		{:else if activeMenu?.childId === 'llm-activities'}
			<LLMActivitiesView {darkMode} />
		{:else if activeMenu?.childId === 'kb-search-lab'}
			<KbSearchLabView {darkMode} />
		{:else if activeMenu?.childId === 'flow'}
			<Canvas01
				{darkMode}
				{railOffset}
				onClose={() => { /* no-op — handled by parent navigation */ }}
			/>
		{:else if activeMenu?.childId === 'kb-import'}
			<KbImportView {darkMode} />
		{:else if activeMenu?.childId === 'kb-metrics'}
			<MetricMgmtView {darkMode} />
		{:else if activeMenu?.childId === 'apps-generate-doc'}
			<DocGenView {darkMode} />
		{:else if activeMenu?.childId === 'apps-document-review'}
			{#key docReviewKey}
				<DocumentReviewView {darkMode} />
			{/key}
		{:else if activeMenu?.childId === 'prompt-optimizer'}
			<PromptOptimizerView {darkMode} />
		{:else if activeMenu?.childId === 'openmetadata'}
			<OpenMetadataWorkspace {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-jetstream-logs'}
			<JetStreamLogsView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-jetstream-events'}
			<JetStreamEventsView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-jetstream-subjects'}
			<JetStreamSubjectsView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-doc-proc-logs'}
			<DocProcLogsView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-llm-usage-logs'}
			<LLMUsageLogsView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-llm-accounts'}
			<LLMAccountsView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-llm-model-profiles'}
			<LLMModelProfilesView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-llm-models'}
			<LLMModelsView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-db-consistency'}
			<DbConsistencyView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-db-maint-log'}
			<DbMaintLogView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-db-resolve-ambiguous'}
			<ResolveAmbiguousObjectsView {darkMode} />
		{:else if activeMenu?.childId === 'diary'}
			<DiaryView {darkMode} />
		{:else if activeMenu?.childId === 'ke-research-topics'}
			<ResearchTopicsView {darkMode} />
		{:else if activeMenu?.itemId === 'skills' && activeMenu?.childId === 'skills-active'}
			<SkillMgmtView {darkMode} initialFilter="activated" />
		{:else if activeMenu?.itemId === 'skills' && activeMenu?.childId === 'skills-create'}
			<SkillMgmtView {darkMode} openAdd={true} />
		{:else if activeMenu?.itemId === 'skills'}
			<SkillMgmtView {darkMode} />
	{:else if activeMenu?.childId === 'ap-board'}
			<KanbanBoardView {darkMode} />
		{:else if activeMenu?.childId === 'ap-agents'}
			<AgentsView {darkMode} />
		{:else if activeMenu?.childId === 'ap-projects'}
			<ProjectsView {darkMode} />
		{:else if sectionId === 'chat'}
			<Chatter01 {darkMode} />
		{:else if isDashboard}
			<!-- Dashboard shelf toggle in top-right -->
			<div class="flex justify-end px-6 pt-4">
				<button
					onclick={onToggleShelf}
					class="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 cursor-pointer transition-colors duration-150"
					style="background:{accentTint}; color:{accent}; font-size:12px; border:none;"
					aria-label={shelfOpen ? 'Close context panel' : 'Open context panel'}
				>
					<PanelRightIcon class="w-3.5 h-3.5" />
					{shelfOpen ? 'Close panel' : 'Open panel'}
				</button>
			</div>
			<DashboardView {darkMode} />
		{:else}
			<!-- Section header + placeholder content -->
			<div class="p-6">
				<div
					class="rounded-xl p-6 mb-6"
					style="background:{cardBg}; border:1px solid {borderColor}; border-radius:{radiusCard};"
				>
					<div class="flex items-start justify-between">
						<div class="flex items-center gap-4">
							{#if sectionIcons[sectionId]}
								{@const IconComponent = sectionIcons[sectionId]}
								<div
									role="presentation"
									class="flex items-center justify-center rounded-xl flex-shrink-0"
									style="width:48px; height:48px; background:{accentTint}; border:1px solid {accent}30;"
								>
									<IconComponent class="w-6 h-6" style="color:{accent};" />
								</div>
							{/if}
							<div>
								<h1 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">
									{activeMenu?.childTitle ?? activeMenu?.itemTitle}
								</h1>
								<p style="font-size:14px; color:{textSecondary};">
									{sectionDesc[sectionId] ?? 'Select a section from the navigation.'}
								</p>
							</div>
						</div>
						<button
							class="flex items-center gap-2 rounded-lg px-4 py-2 cursor-pointer transition-opacity duration-150"
							style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none; border-radius:{radiusButton};"
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.88'; }}
							onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
						>
							+ New
						</button>
					</div>
				</div>

				{#if sectionId === 'settings'}
					<div
						class="rounded-xl p-6 space-y-6"
						style="background:{cardBg}; border:1px solid {borderColor}; border-radius:{radiusCard}; min-height:300px;"
					>
						<div>
							<h2 style="font-size:16px; font-weight:600; color:{textPrimary}; margin-bottom:6px;">
								Navigation Settings
							</h2>
							<p style="font-size:13px; color:{textSecondary};">
								Control how the left navigation panel expands and collapses.
							</p>
						</div>

						<div
							class="rounded-xl p-5"
							style="background:{surface2}; border:1px solid {borderColor};"
						>
							<div class="flex items-start justify-between gap-6">
								<div>
									<div style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">
										Auto Shrink/Expand
									</div>
									<p style="font-size:13px; color:{textSecondary}; line-height:1.5;">
										When enabled, the left panel stays collapsed until you move over it.
										When disabled, the panel only expands or shrinks when you use the rail button.
									</p>
								</div>
								<button
									type="button"
									role="switch"
									aria-checked={autoShrinkExpand}
									aria-label="Toggle auto shrink expand"
									onclick={() => onAutoShrinkExpandChange(!autoShrinkExpand)}
									class="relative flex-shrink-0 rounded-full cursor-pointer transition-colors duration-200"
									style="
										width:52px;
										height:30px;
										padding:3px;
										border:none;
										background:{autoShrinkExpand ? accent : borderColor};
									"
								>
									<span
										class="block rounded-full transition-transform duration-200"
										style="
											width:24px;
											height:24px;
											background:white;
											transform:translateX({autoShrinkExpand ? '22px' : '0'});
										"
									></span>
								</button>
							</div>
							<div
								class="mt-4 inline-flex items-center rounded-full px-3 py-1"
								style="background:{autoShrinkExpand ? accentTint : borderColor}; color:{autoShrinkExpand ? accent : textMuted}; font-size:12px; font-weight:600;"
							>
								Default: Disabled
							</div>
						</div>
					</div>
				{:else}
					<!-- Placeholder content card -->
					<div
						role="presentation"
						class="rounded-xl p-8 flex items-center justify-center"
						style="background:{cardBg}; border:1px solid {borderColor}; border-radius:{radiusCard}; min-height:300px;"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = cardBg; }}
					>
						<div class="text-center">
							{#if sectionIcons[sectionId]}
								{@const IconComponent = sectionIcons[sectionId]}
								<IconComponent class="w-12 h-12 mx-auto mb-4" style="color:{accent}; opacity:0.4;" />
							{/if}
							<p style="font-size:16px; font-weight:500; color:{textSecondary}; margin-bottom:8px;">
								{activeMenu?.childTitle ?? activeMenu?.itemTitle}
							</p>
							<p style="font-size:13px; color:{textMuted};">
								Content for this section will appear here.
							</p>
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>

	{#if sectionId !== 'chat'}
		<!-- AppFooter at the bottom of the scroll area -->
		<div style="margin-top:48px;">
			<AppFooter {darkMode} />
		</div>
	{/if}
</main>
