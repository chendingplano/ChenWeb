<script lang="ts">
	import { onMount } from 'svelte';
	import { getLocale } from '$lib/paraglide/runtime';
	import { getPageConfig, type PageConfig } from '$lib/services/pageConfigService';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';

	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import MessageSquareIcon from '@lucide/svelte/icons/message-square';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import LayoutGridIcon from '@lucide/svelte/icons/layout-grid';
	import CodeIcon from '@lucide/svelte/icons/code-2';
	import UserIcon from '@lucide/svelte/icons/user';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import InfoIcon from '@lucide/svelte/icons/info';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import PanelLeftIcon from '@lucide/svelte/icons/panel-left';
	import PanelLeftCloseIcon from '@lucide/svelte/icons/panel-left-close';
	import MoreHorizontalIcon from '@lucide/svelte/icons/more-horizontal';
	import UserCircle2Icon from '@lucide/svelte/icons/user-circle-2';
	import CreditCardIcon from '@lucide/svelte/icons/credit-card';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import WorkflowIcon from '@lucide/svelte/icons/workflow';
	import ShieldIcon from '@lucide/svelte/icons/shield';
	import BookMarkedIcon from '@lucide/svelte/icons/book-marked';
	import BrainIcon from '@lucide/svelte/icons/brain';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import VideoIcon from '@lucide/svelte/icons/video';
	import LayersIcon from '@lucide/svelte/icons/layers';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	type NavGrandchild = { id: string; label: string };
	type NavChild = { id: string; label: string; children?: NavGrandchild[] };
	type NavItem = {
		id: string;
		label: string;
		icon: any; // lucide component
		group?: string;
		children?: NavChild[];
		href?: string;
	};

	let {
		darkMode = true,
		activeMenu = null,
		autoShrinkExpand = false,
		expanded = false,
		width = 240,
		pageKey = undefined,
		onSelect,
		onToggleRail,
		onWidthDragStart,
		onHoverChange
	}: {
		darkMode: boolean;
		activeMenu: ActiveSelection | null;
		autoShrinkExpand: boolean;
		expanded: boolean;
		width: number;
		// DB-backed page-config key (kb.page_def.page_key). When set, the menu's
		// visibility + labels are overlaid from GET /api/v1/page-config/:pageKey
		// (spec 2026072001 §11). Left undefined (e.g. /home3) → full hardcoded menu.
		pageKey?: string;
		onSelect: (sel: ActiveSelection) => void;
		onToggleRail: () => void;
		onWidthDragStart: (e: MouseEvent) => void;
		onHoverChange: (hovered: boolean) => void;
	} = $props();

	// --- Layout constants ---
	const RAIL_WIDTH_COLLAPSED = 56; // collapsed icon-rail width in px
	const RAIL_TRANSITION = '200ms ease'; // panel slide animation

	// --- Typography ---
	const fontMono = "'Fira Code', 'Cascadia Code', monospace"; // monospace for badges

	// --- Design tokens ---
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2'); // rail background
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB'); // border / divider lines
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1'); // primary accent
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)'); // tint
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827'); // headings
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280'); // body text
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF'); // placeholder

	// Suppress unused
	void fontMono;

	// Effective rail width
	let effectiveWidth = $derived(expanded ? width : RAIL_WIDTH_COLLAPSED);
	let showLabels = $derived(expanded);

	// Accordion expand state per item (top-level) and sub-group
	let accordionOpen = $state<Record<string, boolean>>({});
	let subAccordionOpen = $state<Record<string, boolean>>({});

	// Nav item definitions
	const mainNav: NavItem[] = [
		{
			id: 'dashboard',
			label: 'Dashboard',
			icon: LayoutDashboardIcon,
			group: 'Workspace',
			children: [
				{ id: 'doc-processor-dashboard', label: 'Doc Processor' },
				{ id: 'llm-activities', label: 'LLM Activities' }
			]
		},
		{ id: 'chat', label: 'Chat', icon: MessageSquareIcon, group: 'Workspace' },
		{
			id: 'agents',
			label: 'Agents',
			icon: BotIcon,
			group: 'Workspace',
			children: [
				{ id: 'agents-my', label: 'My Agents' },
				{ id: 'agents-browse', label: 'Browse Library' },
				{ id: 'agents-create', label: 'Create Agent' }
			]
		},
		{
			id: 'skills',
			label: 'Skills',
			icon: ZapIcon,
			group: 'Workspace',
			children: [
				{ id: 'skills-all', label: 'All Skills' },
				{ id: 'skills-active', label: 'Active' },
				{ id: 'skills-create', label: 'New Skill' }
			]
		},
		{
			id: 'applications',
			label: 'Applications',
			icon: LayoutGridIcon,
			group: 'Workspace',
			children: [
				{ id: 'apps-installed', label: 'Installed' },
				{ id: 'apps-browse', label: 'Browse' },
				{ id: 'apps-configure', label: 'Configure' },
				{ id: 'apps-generate-doc', label: 'Generate Doc' },
				{ id: 'apps-document-review', label: 'Document Review' }
			]
		},
		{
			id: 'coding',
			label: 'Coding Assistant',
			icon: CodeIcon,
			group: 'Workspace',
			children: [
				{ id: 'coding-review', label: 'Code Review' },
				{ id: 'coding-gen', label: 'Code Generation' },
				{ id: 'coding-debug', label: 'Debugger' }
			]
		},
		{
			id: 'personal',
			label: 'Personal Assistant',
			icon: UserIcon,
			group: 'Workspace',
			children: [
				{ id: 'personal-tasks', label: 'Tasks' },
				{ id: 'personal-calendar', label: 'Calendar' },
				{ id: 'personal-email', label: 'Email' }
			]
		},
		{
			id: 'knowledge',
			label: 'Knowledge System',
			icon: BookOpenIcon,
			group: 'Workspace',
			href: '/home3/knowledge'
		},
		{
			id: 'knowledge-engineering',
			label: 'Knowledge Engineering',
			icon: BrainIcon,
			group: 'Workspace',
			children: [{ id: 'ke-research-topics', label: 'Research Topics' }]
		},
		{
			id: 'ontology',
			label: 'Ontology',
			icon: LayersIcon,
			group: 'Workspace',
			children: [{ id: 'ontology-doc-facets', label: 'Doc Facets' }]
		},
		{
			id: 'tools',
			label: 'Tools',
			icon: WorkflowIcon,
			group: 'Workspace',
			children: [
				{ id: 'kb-search-lab', label: 'KB Search Lab' },
				{ id: 'flow', label: 'Flow' },
				{ id: 'prompt-optimizer', label: 'Prompt Optimizer' },
				{ id: 'openmetadata', label: 'OpenMetadata' },
				{ id: 'cdm-editor', label: 'CDM Editor' }
			]
		},
		{
			id: 'agent-platform',
			label: 'Agent Platform',
			icon: BotIcon,
			group: 'Workspace',
			children: [
				{ id: 'ap-board', label: 'Board' },
				{ id: 'ap-agents', label: 'Agents' },
				{ id: 'ap-projects', label: 'Projects' }
			]
		},
		{
			id: 'system-admin',
			label: 'System Admin',
			icon: ShieldIcon,
			group: 'System Admin',
			children: [
				{
					id: 'jetstream',
					label: 'JetStream',
					children: [
						{ id: 'sysadmin-jetstream-logs', label: 'JetStream Logs' },
						{ id: 'sysadmin-jetstream-events', label: 'JetStream Events' },
						{ id: 'sysadmin-jetstream-subjects', label: 'JetStream Subjects' }
					]
				},
				{
					id: 'sysadmin-logs',
					label: 'Logs',
					children: [
						{ id: 'sysadmin-doc-proc-logs', label: 'Doc Processor Logs' },
						{ id: 'sysadmin-llm-usage-logs', label: 'LLM Usage Logs' },
						{ id: 'sysadmin-doc-review-logs', label: 'Doc Review Logs' }
					]
				},
				{
					id: 'sysadmin-llm',
					label: 'LLM',
					children: [
						{ id: 'sysadmin-llm-accounts', label: 'LLM Accounts' },
						{ id: 'sysadmin-llm-model-profiles', label: 'Model Profiles' },
						{ id: 'sysadmin-llm-models', label: 'LLM Models' }
					]
				},
				{
					id: 'sysadmin-db',
					label: 'Database Maintenance',
					children: [
						{ id: 'sysadmin-db-consistency', label: 'Consistency Check' },
						{ id: 'sysadmin-db-clean-artifact-data', label: 'Clean Artifact Data' },
						{ id: 'sysadmin-db-maint-log', label: 'Maintenance Log' },
						{ id: 'sysadmin-db-resolve-ambiguous', label: 'Resolve Ambiguous Objects' },
						{ id: 'sysadmin-db-resolve-metric-range-types', label: 'Resolve Metric Range Types' },
						{ id: 'sysadmin-db-resolve-orphaned-labels', label: 'Resolve Orphaned Labels' }
					]
				},
				{
					id: 'sysadmin-users',
					label: 'Users and Accesses',
					children: [
						{ id: 'sysadmin-user-management', label: 'User Management' },
						{ id: 'sysadmin-role-management', label: 'Role Management' },
						{ id: 'sysadmin-access-controls', label: 'Access Controls' }
					]
				},
				{
					id: 'sysadmin-benchmark',
					label: 'Benchmark',
					children: [{ id: 'sysadmin-benchmark-setup', label: 'Setup' }]
				},
				{
					id: 'sysadmin-resources',
					label: 'Resources',
					children: [
						{ id: 'sysadmin-resources-videos', label: 'Videos' },
						{
							id: 'sysadmin-resources-external-terminology',
							label: 'External Terminology Resources'
						},
						{
							id: 'sysadmin-resources-review-external-terminology',
							label: 'Review External Resources'
						}
					]
				},
				{
					id: 'sysadmin-keyword-normalization',
					label: 'Keyword Normalization',
					children: [{ id: 'sysadmin-keyword-rewrite-rules', label: 'Rewrite Rules' }]
				},
				{ id: 'sysadmin-schedules', label: 'Schedules' },
				{
					id: 'sysadmin-doc-process-pipeline',
					label: 'Doc Process Pipeline',
					children: [
						{ id: 'sysadmin-doc-process-dag', label: 'Doc Process DAG' },
						{ id: 'sysadmin-doc-process-processors', label: 'Doc Processors' },
						{ id: 'sysadmin-doc-process-semantic-decision-candidates', label: 'Semantic Decision Candidates' },
						{ id: 'sysadmin-doc-process-semantic-assertions', label: 'Semantic Assertions' },
						{ id: 'sysadmin-doc-process-assertion-evidence', label: 'Assertion Evidence' },
						{ id: 'sysadmin-doc-process-semantic-retry-queue', label: 'Semantic Retry Queue' }
					]
				},
				{ id: 'sysadmin-page-config', label: 'Page Content' }
			]
		},
		{
			id: 'my-workspace',
			label: 'My Workspace',
			icon: BookMarkedIcon,
			group: 'Personal',
			children: [{ id: 'diary', label: 'Diary' }]
		}
	];

	const bottomNav: NavItem[] = [
		{ id: 'settings', label: 'Settings', icon: SettingsIcon },
		{ id: 'about', label: 'About', icon: InfoIcon }
	];

	// Resources page (pageKey='resources') gets its own menu tree, rendered in
	// place of the workspace `mainNav`. A dedicated tree — rather than DB-hiding
	// the workspace tree — keeps these items off /home3 (which passes no pageKey,
	// so the page-config overlay is fail-open there). Labels/visibility are still
	// overlaid from the seeded `resources` page-config.
	const resourcesNav: NavItem[] = [
		{
			id: 'documents',
			label: 'Documents',
			icon: FolderIcon,
			group: 'Resources',
			children: [
				{ id: 'docs-users-manual', label: "User's Manual" },
				{ id: 'docs-development', label: 'Development' }
			]
		},
		{
			id: 'videos',
			label: 'Videos',
			icon: VideoIcon,
			group: 'Resources',
			children: [{ id: 'videos-training', label: 'Training' }]
		}
	];

	// The workspace tree is the default; `resources` swaps in its own tree.
	const activeMainNav = $derived(pageKey === 'resources' ? resourcesNav : mainNav);

	// ── DB-backed page config (overlay model, spec 2026072001 §11) ───────────
	// The menu tree, ids, icons, and routes stay page-owned above. When `pageKey`
	// is set, config only overrides labels, hides items, or restricts them by
	// role. `null` until the fetch resolves (or on error, or when pageKey is
	// unset) so the full default menu renders — fail open.
	let pageConfig = $state<PageConfig | null>(null);

	onMount(() => {
		if (!pageKey) return;
		getPageConfig(pageKey, getLocale())
			.then((cfg) => {
				pageConfig = cfg;
			})
			.catch(() => {
				// Keep the full menu with default labels on failure.
			});
	});

	// Fail open before load / on error: everything visible with default labels.
	// Once loaded, an id is hidden only if the resolver put it in `hidden`; its
	// label comes from the resolved override, else the hardcoded default.
	const isVisible = (id: string) => pageConfig === null || !pageConfig.hidden.has(id);
	const labelFor = (id: string, fallback: string) => pageConfig?.overrides[id]?.label ?? fallback;

	// Prune the tree by visibility and apply label overrides, preserving the
	// page-owned shape. Filtering rules mirror the Wiki menu (spec 2026072001
	// §5.1): hiding a node hides that node; a parent whose descendants all hid
	// collapses away.
	function buildVisible(items: NavItem[]): NavItem[] {
		const out: NavItem[] = [];
		for (const item of items) {
			if (!isVisible(item.id)) continue;
			let children: NavChild[] | undefined;
			if (item.children) {
				children = [];
				for (const child of item.children) {
					if (!isVisible(child.id)) continue;
					let grandchildren: NavGrandchild[] | undefined;
					if (child.children) {
						grandchildren = child.children
							.filter((gc) => isVisible(gc.id))
							.map((gc) => ({ ...gc, label: labelFor(gc.id, gc.label) }));
						if (grandchildren.length === 0) continue; // sub-group collapses
					}
					children.push({
						...child,
						label: labelFor(child.id, child.label),
						...(grandchildren ? { children: grandchildren } : {})
					});
				}
				if (children.length === 0) continue; // parent collapses when all children hid
			}
			out.push({
				...item,
				label: labelFor(item.id, item.label),
				...(children ? { children } : {})
			});
		}
		return out;
	}

	const displayMainNav = $derived(buildVisible(activeMainNav));
	const displayBottomNav = $derived(buildVisible(bottomNav));

	// Every id the active menu owns — the source of truth for valid entry_keys.
	// Used to flag config rows that match nothing (a stale/typo entry_key), which
	// are otherwise silently inert (spec 2026072001 §4.4).
	const knownNavIds = $derived.by(() => {
		const ids = new Set<string>();
		for (const item of [...activeMainNav, ...bottomNav]) {
			ids.add(item.id);
			for (const child of item.children ?? []) {
				ids.add(child.id);
				for (const gc of child.children ?? []) ids.add(gc.id);
			}
		}
		return ids;
	});

	const unknownConfigIds = $derived(
		pageConfig
			? [...Object.keys(pageConfig.overrides), ...pageConfig.hidden].filter(
					(id) => !knownNavIds.has(id)
				)
			: []
	);

	$effect(() => {
		if (unknownConfigIds.length > 0) {
			console.warn(
				`[page-config] ${pageKey} returned unrecognized nav entry id(s), ignored: ${unknownConfigIds.join(', ')}`
			);
		}
	});

	const user = { name: 'Alex Johnson', email: 'alex@example.com' };

	function isItemActive(item: NavItem): boolean {
		return !!activeMenu && activeMenu.itemId === item.id;
	}

	function isChildActive(child: NavChild): boolean {
		return !!activeMenu && activeMenu.childId === child.id;
	}

	function selectItem(item: NavItem, child?: NavChild) {
		if (!child && item.href) {
			window.open(`${item.href}?dark=${darkMode ? '1' : '0'}`, '_blank', 'noopener');
			return;
		}
		if (child?.id === 'kb-metrics') {
			window.open(`/home3/metrics?dark=${darkMode ? '1' : '0'}`, '_blank', 'noopener');
			return;
		}
		if (child?.id === 'kb-input-details') {
			window.open(`/home3/inputs?dark=${darkMode ? '1' : '0'}`, '_blank', 'noopener');
			return;
		}
		if (child?.id === 'kb-chunks') {
			window.open(`/home3/chunks?dark=${darkMode ? '1' : '0'}`, '_blank', 'noopener');
			return;
		}
		if (child?.id === 'kb-doc-structure') {
			window.open(`/home3/doc-structure?dark=${darkMode ? '1' : '0'}`, '_blank', 'noopener');
			return;
		}
		onSelect({
			itemId: item.id,
			childId: child?.id,
			itemTitle: item.label,
			childTitle: child?.label
		});
		if (child) return;
		if (item.children) {
			accordionOpen[item.id] = !accordionOpen[item.id];
		}
	}

	function toggleAccordion(id: string) {
		accordionOpen[id] = !accordionOpen[id];
	}

	function toggleSubAccordion(id: string) {
		subAccordionOpen[id] = !subAccordionOpen[id];
	}

	function isGrandchildActive(gc: NavGrandchild): boolean {
		return !!activeMenu && activeMenu.childId === gc.id;
	}

	function isSubGroupActive(child: NavChild): boolean {
		return !!child.children && child.children.some((gc) => isGrandchildActive(gc));
	}

	// Hover background
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.6)' : 'rgba(228,230,235,0.7)');
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<aside
	class="relative flex flex-shrink-0 flex-col overflow-hidden"
	style="width:{effectiveWidth}px; background:{surface2}; border-right:1px solid {borderColor}; transition:width {RAIL_TRANSITION};"
	onmouseenter={() => onHoverChange(true)}
	onmouseleave={() => onHoverChange(false)}
>
	<!-- Rail mode button at top -->
	<div
		class="flex flex-shrink-0 items-center px-2"
		style="height:48px; border-bottom:1px solid {borderColor};"
	>
		{#if showLabels}
			<div class="flex w-full items-center justify-between px-1">
				<span style="font-size:13px; font-weight:600; color:{accent};">Navigation</span>
				<button
					onclick={onToggleRail}
					class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg transition-colors duration-150"
					style="color:{textMuted};"
					onmouseenter={(e) => {
						(e.currentTarget as HTMLElement).style.color = accent;
					}}
					onmouseleave={(e) => {
						(e.currentTarget as HTMLElement).style.color = textMuted;
					}}
					aria-label={autoShrinkExpand ? 'Disable auto shrink/expand' : 'Shrink navigation'}
					title={autoShrinkExpand ? 'Disable auto shrink/expand' : 'Shrink navigation'}
				>
					{#if autoShrinkExpand}
						<PanelLeftIcon class="h-4 w-4" />
					{:else}
						<PanelLeftCloseIcon class="h-4 w-4" />
					{/if}
				</button>
			</div>
		{:else}
			<button
				onclick={onToggleRail}
				class="flex h-8 w-full cursor-pointer items-center justify-center rounded-lg transition-colors duration-150"
				style="color:{textMuted};"
				onmouseenter={(e) => {
					(e.currentTarget as HTMLElement).style.color = accent;
				}}
				onmouseleave={(e) => {
					(e.currentTarget as HTMLElement).style.color = textMuted;
				}}
				aria-label={autoShrinkExpand ? 'Disable auto shrink/expand' : 'Expand navigation'}
				title={autoShrinkExpand ? 'Disable auto shrink/expand' : 'Expand navigation'}
			>
				<PanelLeftIcon class="h-5 w-5" />
			</button>
		{/if}
	</div>

	<!-- Main nav items (scrollable) -->
	<nav
		class="flex-1 overflow-y-auto py-2"
		style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
	>
		{#each displayMainNav as item, index (item.id)}
			<div class="mb-0.5 px-2">
				{#if showLabels && item.group && (index === 0 || displayMainNav[index - 1].group !== item.group)}
					<div
						class="px-2 py-2 text-xs tracking-wide uppercase"
						style="color:{textMuted}; font-weight:600;"
					>
						{item.group}
					</div>
				{/if}
				<!-- Parent item button -->
				<button
					onclick={() => (item.children ? toggleAccordion(item.id) : selectItem(item))}
					class="flex w-full cursor-pointer items-center gap-3 rounded-lg transition-colors duration-150"
					style="
						padding: {showLabels ? '8px 10px' : '9px 0'};
						justify-content: {showLabels ? 'flex-start' : 'center'};
						background: {isItemActive(item) ? accentTint : 'transparent'};
						color: {isItemActive(item) ? accent : textSecondary};
						border-left: {isItemActive(item) && showLabels ? '2px solid ' + accent : '2px solid transparent'};
					"
					onmouseenter={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = hoverBg;
						el.style.color = textPrimary;
					}}
					onmouseleave={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = 'transparent';
						el.style.color = isItemActive(item) ? accent : textSecondary;
					}}
					title={!showLabels ? item.label : undefined}
					aria-label={item.label}
				>
					<item.icon class="flex-shrink-0" style="width:20px; height:20px;" />
					{#if showLabels}
						<span class="flex-1 truncate text-left" style="font-size:14px; font-weight:500;"
							>{item.label}</span
						>
						{#if item.children}
							<ChevronDownIcon
								class="flex-shrink-0 transition-transform duration-200"
								style="width:14px; height:14px; transform: rotate({accordionOpen[item.id]
									? '180deg'
									: '0deg'});"
							/>
						{/if}
					{/if}
				</button>

				<!-- Sub-items (accordion, expanded rail only) -->
				{#if showLabels && item.children && accordionOpen[item.id]}
					<div class="mt-0.5 mb-1 ml-3" style="border-left:2px solid {borderColor};">
						{#each item.children as child (child.id)}
							{#if child.children}
								<!-- Sub-group: foldable header -->
								<button
									onclick={() => toggleSubAccordion(child.id)}
									class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 transition-colors duration-150"
									style="
										color: {isSubGroupActive(child) ? accent : textSecondary};
										background: {isSubGroupActive(child) ? accentTint : 'transparent'};
										font-size: 13px; font-weight: 500;
									"
									onmouseenter={(e) => {
										const el = e.currentTarget as HTMLElement;
										if (!isSubGroupActive(child)) {
											el.style.background = hoverBg;
											el.style.color = textPrimary;
										}
									}}
									onmouseleave={(e) => {
										const el = e.currentTarget as HTMLElement;
										if (!isSubGroupActive(child)) {
											el.style.background = 'transparent';
											el.style.color = isSubGroupActive(child) ? accent : textSecondary;
										}
									}}
								>
									<span class="flex-1 truncate text-left">{child.label}</span>
									<ChevronDownIcon
										class="flex-shrink-0 transition-transform duration-200"
										style="width:12px; height:12px; transform: rotate({subAccordionOpen[child.id]
											? '180deg'
											: '0deg'});"
									/>
								</button>
								<!-- Grandchildren -->
								{#if subAccordionOpen[child.id]}
									<div class="ml-3" style="border-left:2px solid {borderColor};">
										{#each child.children as gc (gc.id)}
											<button
												onclick={() => selectItem(item, gc)}
												class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left transition-colors duration-150"
												style="
													color: {isGrandchildActive(gc) ? accent : textMuted};
													background: {isGrandchildActive(gc) ? accentTint : 'transparent'};
													font-size: 13px;
												"
												onmouseenter={(e) => {
													const el = e.currentTarget as HTMLElement;
													if (!isGrandchildActive(gc)) {
														el.style.background = hoverBg;
														el.style.color = textPrimary;
													}
												}}
												onmouseleave={(e) => {
													const el = e.currentTarget as HTMLElement;
													if (!isGrandchildActive(gc)) {
														el.style.background = 'transparent';
														el.style.color = textMuted;
													}
												}}
											>
												<div
													class="h-1 w-1 flex-shrink-0 rounded-full"
													style="background:currentColor; opacity:0.5;"
												></div>
												{gc.label}
											</button>
										{/each}
									</div>
								{/if}
							{:else}
								<!-- Flat leaf child (existing behaviour) -->
								<button
									onclick={() => selectItem(item, child)}
									class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left transition-colors duration-150"
									style="
										color: {isChildActive(child) ? accent : textMuted};
										background: {isChildActive(child) ? accentTint : 'transparent'};
										font-size: 13px;
									"
									onmouseenter={(e) => {
										const el = e.currentTarget as HTMLElement;
										if (!isChildActive(child)) {
											el.style.background = hoverBg;
											el.style.color = textPrimary;
										}
									}}
									onmouseleave={(e) => {
										const el = e.currentTarget as HTMLElement;
										if (!isChildActive(child)) {
											el.style.background = 'transparent';
											el.style.color = textMuted;
										}
									}}
								>
									<div
										class="h-1 w-1 flex-shrink-0 rounded-full"
										style="background:currentColor; opacity:0.5;"
									></div>
									{child.label}
								</button>
							{/if}
						{/each}
					</div>
				{/if}
			</div>
		{/each}

		<!-- Separator -->
		<div class="mx-3 my-2" style="height:1px; background:{borderColor};"></div>

		<!-- Bottom nav -->
		{#each displayBottomNav as item (item.id)}
			<div class="mb-0.5 px-2">
				<button
					onclick={() => selectItem(item)}
					class="flex w-full cursor-pointer items-center gap-3 rounded-lg transition-colors duration-150"
					style="
						padding: {showLabels ? '8px 10px' : '9px 0'};
						justify-content: {showLabels ? 'flex-start' : 'center'};
						background: {isItemActive(item) ? accentTint : 'transparent'};
						color: {isItemActive(item) ? accent : textMuted};
						border-left: {isItemActive(item) && showLabels ? '2px solid ' + accent : '2px solid transparent'};
					"
					onmouseenter={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = hoverBg;
						el.style.color = textPrimary;
					}}
					onmouseleave={(e) => {
						const el = e.currentTarget as HTMLElement;
						if (!isItemActive(item)) el.style.background = 'transparent';
						el.style.color = isItemActive(item) ? accent : textMuted;
					}}
					title={!showLabels ? item.label : undefined}
					aria-label={item.label}
				>
					<item.icon class="flex-shrink-0" style="width:20px; height:20px;" />
					{#if showLabels}
						<span class="flex-1 truncate text-left" style="font-size:14px; font-weight:500;"
							>{item.label}</span
						>
					{/if}
				</button>
			</div>
		{/each}
	</nav>

	<!-- User section at bottom -->
	<div class="flex-shrink-0 p-2" style="border-top:1px solid {borderColor};">
		{#if showLabels}
			<div class="flex items-center gap-2 rounded-lg p-2" style="background:{hoverBg};">
				<!-- Avatar -->
				<div
					class="flex flex-shrink-0 items-center justify-center rounded-lg text-xs font-semibold"
					style="width:32px; height:32px; background:{accentTint}; color:{accent}; border:1px solid {accent}30;"
				>
					{user.name
						.split(' ')
						.map((n) => n[0])
						.join('')}
				</div>
				<!-- Name + email -->
				<div class="min-w-0 flex-1">
					<div class="truncate" style="font-size:13px; font-weight:500; color:{textPrimary};">
						{user.name}
					</div>
					<div class="truncate" style="font-size:11px; color:{textMuted};">{user.email}</div>
				</div>
				<!-- Three-dots dropdown -->
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<button
								{...props}
								class="flex h-6 w-6 flex-shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors duration-150"
								style="color:{textMuted};"
								onmouseenter={(e) => {
									(e.currentTarget as HTMLElement).style.color = textPrimary;
									(e.currentTarget as HTMLElement).style.background = borderColor;
								}}
								onmouseleave={(e) => {
									(e.currentTarget as HTMLElement).style.color = textMuted;
									(e.currentTarget as HTMLElement).style.background = 'transparent';
								}}
								aria-label="User menu"
							>
								<MoreHorizontalIcon class="h-4 w-4" />
							</button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content class="min-w-44 rounded-lg" side="top" align="end" sideOffset={8}>
						<DropdownMenu.Item
							onclick={() => onSelect({ itemId: '__user_info__', itemTitle: 'User Info' })}
						>
							<UserCircle2Icon class="mr-2 h-4 w-4" />
							User Info
						</DropdownMenu.Item>
						<DropdownMenu.Item
							onclick={() => onSelect({ itemId: '__account__', itemTitle: 'Account' })}
						>
							<CreditCardIcon class="mr-2 h-4 w-4" />
							Account
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item
							onclick={() => onSelect({ itemId: '__logout__', itemTitle: 'Logout' })}
						>
							<LogOutIcon class="mr-2 h-4 w-4" />
							Log Out
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</div>
		{:else}
			<!-- Collapsed: avatar only with dropdown -->
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<button
							{...props}
							class="flex h-9 w-full cursor-pointer items-center justify-center rounded-lg"
							style="background:{accentTint}; color:{accent}; font-size:11px; font-weight:600;"
							aria-label="User menu"
							title={user.name}
						>
							{user.name
								.split(' ')
								.map((n) => n[0])
								.join('')}
						</button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content class="min-w-44 rounded-lg" side="right" align="end" sideOffset={8}>
					<DropdownMenu.Item
						onclick={() => onSelect({ itemId: '__user_info__', itemTitle: 'User Info' })}
					>
						<UserCircle2Icon class="mr-2 h-4 w-4" />
						User Info
					</DropdownMenu.Item>
					<DropdownMenu.Item
						onclick={() => onSelect({ itemId: '__account__', itemTitle: 'Account' })}
					>
						<CreditCardIcon class="mr-2 h-4 w-4" />
						Account
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item
						onclick={() => onSelect({ itemId: '__logout__', itemTitle: 'Logout' })}
					>
						<LogOutIcon class="mr-2 h-4 w-4" />
						Log Out
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		{/if}
	</div>

	<!-- Resize handle (right edge, only visible when expanded/pinned) -->
	{#if showLabels}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="absolute top-0 right-0 bottom-0 cursor-col-resize"
			style="width:4px; z-index:10;"
			onmousedown={onWidthDragStart}
		></div>
	{/if}
</aside>
