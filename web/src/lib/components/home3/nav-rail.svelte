<script lang="ts">
	import { onMount } from 'svelte';
	import { getLocale } from '$lib/paraglide/runtime';
	import { m } from '$lib/paraglide/messages.js';
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
	import FlaskConicalIcon from '@lucide/svelte/icons/flask-conical';

	type ActiveSelection = {
		itemId: string;
		childId?: string;
		itemTitle: string;
		childTitle?: string;
	};

	type NavGreatGrandchild = { id: string; label: string };
	// `href` on a leaf opens that route in a new tab instead of selecting a view.
	type NavGrandchild = { id: string; label: string; href?: string; children?: NavGreatGrandchild[] };
	type NavChild = { id: string; label: string; href?: string; children?: NavGrandchild[] };
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

	// Group headings. `group` stays an English id (it is compared between items);
	// only its display text is localised.
	const GROUP_LABEL: Record<string, string> = {
		Workspace: m.nav_group_workspace(),
		'System Admin': m.nav_group_system_admin(),
		Personal: m.nav_group_personal(),
		Resources: m.nav_group_resources(),
		Development: m.nav_group_development()
	};

	// Nav item definitions. Labels are Paraglide messages (nav_<id>) in en and
	// zh-cn; kb.page_config can still override a label per language, and still
	// controls visibility/access (spec 2026072001 §11, ADR 2026093001).
	const mainNav: NavItem[] = [
		{
			id: 'dashboard',
			label: m.nav_dashboard(),
			icon: LayoutDashboardIcon,
			group: 'Workspace',
			children: [
				{ id: 'doc-processor-dashboard', label: m.nav_doc_processor_dashboard() },
				{ id: 'llm-activities', label: m.nav_llm_activities() }
			]
		},
		{ id: 'chat', label: m.nav_chat(), icon: MessageSquareIcon, group: 'Workspace' },
		{ id: 'agent-services', label: m.nav_agent_services(), icon: BookMarkedIcon, group: 'Workspace', href: '/home3/agent-services' },
		{
			id: 'agents',
			label: m.nav_agents(),
			icon: BotIcon,
			group: 'Workspace',
			children: [
				{ id: 'agents-my', label: m.nav_agents_my() },
				{ id: 'agents-browse', label: m.nav_agents_browse() },
				{ id: 'agents-create', label: m.nav_agents_create() }
			]
		},
		{
			id: 'skills',
			label: m.nav_skills(),
			icon: ZapIcon,
			group: 'Workspace',
			children: [
				{ id: 'skills-all', label: m.nav_skills_all() },
				{ id: 'skills-active', label: m.nav_skills_active() },
				{ id: 'skills-create', label: m.nav_skills_create() }
			]
		},
		{
			id: 'applications',
			label: m.nav_applications(),
			icon: LayoutGridIcon,
			group: 'Workspace',
			children: [
				{ id: 'apps-installed', label: m.nav_apps_installed() },
				{ id: 'apps-browse', label: m.nav_apps_browse() },
				{ id: 'apps-configure', label: m.nav_apps_configure() },
				{ id: 'apps-generate-doc', label: m.nav_apps_generate_doc() },
				{ id: 'apps-document-review', label: m.nav_apps_document_review() },
				{ id: 'apps-product-review', label: m.nav_apps_product_review() }
			]
		},
		{
			id: 'coding',
			label: m.nav_coding(),
			icon: CodeIcon,
			group: 'Workspace',
			children: [
				{ id: 'coding-review', label: m.nav_coding_review() },
				{ id: 'coding-gen', label: m.nav_coding_gen() },
				{ id: 'coding-debug', label: m.nav_coding_debug() }
			]
		},
		{
			id: 'personal',
			label: m.nav_personal(),
			icon: UserIcon,
			group: 'Workspace',
			children: [
				{ id: 'personal-tasks', label: m.nav_personal_tasks() },
				{ id: 'personal-calendar', label: m.nav_personal_calendar() },
				{ id: 'personal-email', label: m.nav_personal_email() }
			]
		},
		{
			id: 'knowledge',
			label: m.nav_knowledge(),
			icon: BookOpenIcon,
			group: 'Workspace',
			href: '/home3/knowledge'
		},
		{
			id: 'knowledge-engineering',
			label: m.nav_knowledge_engineering(),
			icon: BrainIcon,
			group: 'Workspace',
			children: [{ id: 'ke-research-topics', label: m.nav_ke_research_topics() }]
		},
		{
			id: 'ontology',
			label: m.nav_ontology(),
			icon: LayersIcon,
			group: 'Workspace',
			children: [{ id: 'ontology-doc-facets', label: m.nav_ontology_doc_facets() }]
		},
		{
			id: 'tools',
			label: m.nav_tools(),
			icon: WorkflowIcon,
			group: 'Workspace',
			children: [
				{ id: 'kb-search-lab', label: m.nav_kb_search_lab() },
				{ id: 'flow', label: m.nav_flow() },
				{ id: 'prompt-optimizer', label: m.nav_prompt_optimizer() },
				{ id: 'openmetadata', label: m.nav_openmetadata() },
				{ id: 'cdm-editor', label: m.nav_cdm_editor() }
			]
		},
		{
			id: 'agent-platform',
			label: m.nav_agent_platform(),
			icon: BotIcon,
			group: 'Workspace',
			children: [
				{ id: 'ap-board', label: m.nav_ap_board() },
				{ id: 'ap-agents', label: m.nav_ap_agents() },
				{ id: 'ap-projects', label: m.nav_ap_projects() }
			]
		},
		{
			id: 'system-admin',
			label: m.nav_system_admin(),
			icon: ShieldIcon,
			group: 'System Admin',
			children: [
				{
					id: 'jetstream',
					label: m.nav_jetstream(),
					children: [
						{ id: 'sysadmin-jetstream-logs', label: m.nav_sysadmin_jetstream_logs() },
						{ id: 'sysadmin-jetstream-events', label: m.nav_sysadmin_jetstream_events() },
						{ id: 'sysadmin-jetstream-subjects', label: m.nav_sysadmin_jetstream_subjects() }
					]
				},
				{
					id: 'sysadmin-logs',
					label: m.nav_sysadmin_logs(),
					children: [
						{ id: 'sysadmin-doc-proc-logs', label: m.nav_sysadmin_doc_proc_logs() },
						{ id: 'sysadmin-llm-usage-logs', label: m.nav_sysadmin_llm_usage_logs() },
						{ id: 'sysadmin-doc-review-logs', label: m.nav_sysadmin_doc_review_logs() }
					]
				},
				{
					id: 'sysadmin-llm',
					label: m.nav_sysadmin_llm(),
					children: [
						{ id: 'sysadmin-llm-accounts', label: m.nav_sysadmin_llm_accounts() },
						{ id: 'sysadmin-llm-embedding', label: m.nav_sysadmin_llm_embedding() },
						{ id: 'sysadmin-llm-model-profiles', label: m.nav_sysadmin_llm_model_profiles() },
						{ id: 'sysadmin-llm-models', label: m.nav_sysadmin_llm_models() },
						{ id: 'sysadmin-llm-chat-sessions', label: m.nav_sysadmin_llm_chat_sessions() },
						{ id: 'sysadmin-llm-pi-sessions', label: m.nav_sysadmin_llm_pi_sessions() },
						// /development groups Review Metrics with Gold Metrics (testbed.metrics,
						// a development tool) under Metrics; other pages keep Review Metrics here.
						...(pageKey === 'development'
							? [
									{
										id: 'sysadmin-llm-metrics',
										label: m.nav_sysadmin_llm_metrics(),
										children: [
											{ id: 'sysadmin-llm-review-metrics', label: m.nav_sysadmin_llm_review_metrics() },
											{ id: 'sysadmin-llm-metrics-gold', label: m.nav_sysadmin_llm_metrics_gold() }
										]
									}
								]
							: [{ id: 'sysadmin-llm-review-metrics', label: m.nav_sysadmin_llm_review_metrics() }]),
						{
							id: 'sysadmin-llm-decision-models',
							label: m.nav_sysadmin_llm_decision_models(),
							children: [
								{ id: 'sysadmin-llm-decision-models-playground', label: m.nav_sysadmin_llm_decision_models_playground() }
							]
						}
					]
				},
				{
					id: 'sysadmin-db',
					label: m.nav_sysadmin_db(),
					children: [
						{ id: 'sysadmin-db-consistency', label: m.nav_sysadmin_db_consistency() },
						{ id: 'sysadmin-db-clean-artifact-data', label: m.nav_sysadmin_db_clean_artifact_data() },
						{ id: 'sysadmin-db-maint-log', label: m.nav_sysadmin_db_maint_log() },
						{ id: 'sysadmin-db-resolve-ambiguous', label: m.nav_sysadmin_db_resolve_ambiguous() },
						{ id: 'sysadmin-db-resolve-metric-range-types', label: m.nav_sysadmin_db_resolve_metric_range_types() },
						{ id: 'sysadmin-db-resolve-orphaned-labels', label: m.nav_sysadmin_db_resolve_orphaned_labels() }
					]
				},
				{
					id: 'sysadmin-users',
					label: m.nav_sysadmin_users(),
					children: [
						{ id: 'sysadmin-user-management', label: m.nav_sysadmin_user_management() },
						{ id: 'sysadmin-role-management', label: m.nav_sysadmin_role_management() },
						{ id: 'sysadmin-access-controls', label: m.nav_sysadmin_access_controls() }
					]
				},
				{
					id: 'sysadmin-benchmark',
					label: m.nav_sysadmin_benchmark(),
					children: [{ id: 'sysadmin-benchmark-setup', label: m.nav_sysadmin_benchmark_setup() }]
				},
				{
					id: 'sysadmin-resources',
					label: m.nav_sysadmin_resources(),
					children: [
						{ id: 'sysadmin-resources-videos', label: m.nav_sysadmin_resources_videos() },
						{ id: 'sysadmin-resources-product-drawings', label: m.nav_sysadmin_resources_product_drawings() },
						{
							id: 'sysadmin-resources-import-product-names',
							label: m.nav_sysadmin_resources_import_product_names(),
							children: [
								{ id: 'sysadmin-resources-china-mechanical-product-names', label: m.nav_sysadmin_resources_china_mechanical_product_names() }
							]
						},
						{
							id: 'sysadmin-resources-external-terminology',
							label: m.nav_sysadmin_resources_external_terminology()
						},
						{
							id: 'sysadmin-resources-review-external-terminology',
							label: m.nav_sysadmin_resources_review_external_terminology()
						},
						{ id: 'sysadmin-resources-sync-data', label: m.nav_sysadmin_resources_sync_data() }
					]
				},
				{
					id: 'sysadmin-keyword-normalization',
					label: m.nav_sysadmin_keyword_normalization(),
					children: [{ id: 'sysadmin-keyword-rewrite-rules', label: m.nav_sysadmin_keyword_rewrite_rules() }]
				},
				{
					id: 'sysadmin-doc-process-pipeline',
					label: m.nav_sysadmin_doc_process_pipeline(),
					children: [
						{ id: 'sysadmin-doc-process-dag', label: m.nav_sysadmin_doc_process_dag() },
						{ id: 'sysadmin-doc-process-processors', label: m.nav_sysadmin_doc_process_processors() },
						{ id: 'sysadmin-doc-process-semantic-decision-candidates', label: m.nav_sysadmin_doc_process_semantic_decision_candidates() },
						{ id: 'sysadmin-doc-process-semantic-assertions', label: m.nav_sysadmin_doc_process_semantic_assertions() },
						{ id: 'sysadmin-doc-process-assertion-evidence', label: m.nav_sysadmin_doc_process_assertion_evidence() },
						{ id: 'sysadmin-doc-process-semantic-retry-queue', label: m.nav_sysadmin_doc_process_semantic_retry_queue() }
					]
				},
				{
					id: 'sysadmin-system',
					label: m.nav_sysadmin_system(),
					children: [
						{ id: 'sysadmin-system-calendar', label: m.nav_sysadmin_system_calendar() },
						{ id: 'sysadmin-system-peak-hours', label: m.nav_sysadmin_system_peak_hours() },
						{ id: 'sysadmin-system-releases', label: m.nav_sysadmin_system_releases() },
						{ id: 'sysadmin-system-prices', label: m.nav_sysadmin_system_prices() },
						{ id: 'sysadmin-schedules', label: m.nav_sysadmin_schedules() },
						{ id: 'sysadmin-page-config', label: m.nav_sysadmin_page_config() }
					]
				}
			]
		},
		{
			id: 'my-workspace',
			label: m.nav_my_workspace(),
			icon: BookMarkedIcon,
			group: 'Personal',
			children: [{ id: 'diary', label: m.nav_diary() }]
		}
	];

	const bottomNav: NavItem[] = [
		{ id: 'settings', label: m.nav_settings(), icon: SettingsIcon },
		{ id: 'about', label: m.nav_about(), icon: InfoIcon }
	];

	// Resources page (pageKey='resources') gets its own menu tree, rendered in
	// place of the workspace `mainNav`. A dedicated tree — rather than DB-hiding
	// the workspace tree — keeps these items off /home3 (which passes no pageKey,
	// so the page-config overlay is fail-open there). Labels/visibility are still
	// overlaid from the seeded `resources` page-config.
	const resourcesNav: NavItem[] = [
		{
			id: 'documents',
			label: m.nav_documents(),
			icon: FolderIcon,
			group: 'Resources',
			children: [
				{ id: 'docs-users-manual', label: m.nav_docs_users_manual() },
				{ id: 'docs-development', label: m.nav_docs_development() }
			]
		},
		{
			id: 'videos',
			label: m.nav_videos(),
			icon: VideoIcon,
			group: 'Resources',
			children: [{ id: 'videos-training', label: m.nav_videos_training() }]
		}
	];

	// Development page (pageKey='development') appends its own section to the
	// workspace tree, kept off /home3 for the same reason as `resourcesNav`.
	const developmentNav: NavItem[] = [
		{
			id: 'development',
			label: m.nav_development(),
			icon: FlaskConicalIcon,
			group: 'Development',
			children: [
				{
					id: 'dev-demos',
					label: m.nav_dev_demos(),
					children: [
						{ id: 'dev-demos-main-page', label: m.nav_dev_demos_main_page(), href: '/home7' },
						{ id: 'dev-demos-jenny-main-page', label: m.nav_dev_demos_jenny_main_page(), href: '/home8' }
					]
				}
			]
		}
	];

	// The workspace tree is the default; `resources` swaps in its own tree and
	// `development` extends it.
	const activeMainNav = $derived(
		pageKey === 'resources'
			? resourcesNav
			: pageKey === 'development'
				? [...mainNav, ...developmentNav]
				: mainNav
	);

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

	type UserProfile = { name: string; email: string };
	let user = $state<UserProfile>({ name: '', email: '' });
	let userDisplayName = $derived(user.name || user.email);
	let userInitials = $derived(
		userDisplayName
			.split(' ')
			.map((part) => part[0])
			.join('')
	);

	onMount(() => {
		fetch('/api/v1/ai-assistant/user-info', { credentials: 'same-origin' })
			.then(async (response) => {
				if (!response.ok) return;
				const payload: unknown = await response.json();
				if (
					typeof payload !== 'object' ||
					payload === null ||
					!('user' in payload) ||
					typeof payload.user !== 'object' ||
					payload.user === null ||
					!('name' in payload.user) ||
					!('email' in payload.user) ||
					typeof payload.user.name !== 'string' ||
					typeof payload.user.email !== 'string'
				) {
					return;
				}

				user = {
					name: payload.user.name.trim(),
					email: payload.user.email.trim()
				};
			})
			.catch(() => {
				// Keep the profile blank when the current identity cannot be loaded.
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
						grandchildren = [];
						for (const gc of child.children) {
							if (!isVisible(gc.id)) continue;
							const greatGrandchildren = gc.children
								?.filter((ggc) => isVisible(ggc.id))
								.map((ggc) => ({ ...ggc, label: labelFor(ggc.id, ggc.label) }));
							if (gc.children && greatGrandchildren?.length === 0) continue;
							grandchildren.push({
								...gc,
								label: labelFor(gc.id, gc.label),
								...(greatGrandchildren ? { children: greatGrandchildren } : {})
							});
						}
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
				for (const gc of child.children ?? []) {
					for (const ggc of gc.children ?? []) ids.add(ggc.id);
				}
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

	function isItemActive(item: NavItem): boolean {
		return !!activeMenu && activeMenu.itemId === item.id;
	}

	function isChildActive(child: NavChild): boolean {
		return !!activeMenu && activeMenu.childId === child.id;
	}

	function selectItem(item: NavItem, child?: NavChild | NavGrandchild) {
		const href = child ? child.href : item.href;
		if (href) {
			window.open(`${href}?dark=${darkMode ? '1' : '0'}`, '_blank', 'noopener');
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
		return !!activeMenu && (activeMenu.childId === gc.id || !!gc.children?.some((ggc) => activeMenu.childId === ggc.id));
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
				<span style="font-size:13px; font-weight:600; color:{accent};">{m.nav_rail_title()}</span>
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
					aria-label={autoShrinkExpand ? m.nav_rail_disable_auto_shrink_expand() : m.nav_rail_shrink_navigation()}
					title={autoShrinkExpand ? m.nav_rail_disable_auto_shrink_expand() : m.nav_rail_shrink_navigation()}
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
				aria-label={autoShrinkExpand ? m.nav_rail_disable_auto_shrink_expand() : m.nav_rail_expand_navigation()}
				title={autoShrinkExpand ? m.nav_rail_disable_auto_shrink_expand() : m.nav_rail_expand_navigation()}
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
						{GROUP_LABEL[item.group] ?? item.group}
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
							{#if gc.children}
								<button
									onclick={() => toggleSubAccordion(gc.id)}
									class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left transition-colors duration-150"
									style="color:{isGrandchildActive(gc) ? accent : textMuted};background:{isGrandchildActive(gc) ? accentTint : 'transparent'};font-size:12px;"
								>
									<span class="flex-1 truncate">{gc.label}</span>
									<ChevronDownIcon class="h-3 w-3" style="transform:rotate({subAccordionOpen[gc.id] ? '180deg' : '0deg'});" />
								</button>
								{#if subAccordionOpen[gc.id]}
									<div class="ml-3" style="border-left:2px solid {borderColor};">
										{#each gc.children as ggc (ggc.id)}
											<button
												onclick={() => selectItem(item, ggc)}
												class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left text-xs transition-colors duration-150"
												style="color:{activeMenu?.childId === ggc.id ? accent : textMuted};background:{activeMenu?.childId === ggc.id ? accentTint : 'transparent'};"
											>
												<span class="h-1 w-1 rounded-full" style="background:currentColor;opacity:.5;"></span>
												{ggc.label}
											</button>
										{/each}
									</div>
								{/if}
							{:else}
								<button
									onclick={() => selectItem(item, gc)}
									class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left transition-colors duration-150"
									style="color:{isGrandchildActive(gc) ? accent : textMuted};background:{isGrandchildActive(gc) ? accentTint : 'transparent'};font-size:13px;"
								>
									<span class="h-1 w-1 flex-shrink-0 rounded-full" style="background:currentColor;opacity:.5;"></span>
									{gc.label}
								</button>
							{/if}
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
					{userInitials}
				</div>
				<!-- Name + email -->
				<div class="min-w-0 flex-1">
					<div class="truncate" style="font-size:13px; font-weight:500; color:{textPrimary};">
						{userDisplayName}
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
								aria-label={m.nav_user_menu()}
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
							{m.nav_user_info()}
						</DropdownMenu.Item>
						<DropdownMenu.Item
							disabled
						>
							<CreditCardIcon class="mr-2 h-4 w-4" />
							{m.nav_account()}
						</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item
							onclick={() => onSelect({ itemId: '__logout__', itemTitle: 'Logout' })}
						>
							<LogOutIcon class="mr-2 h-4 w-4" />
							{m.nav_log_out()}
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
							aria-label={m.nav_user_menu()}
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
						{m.nav_user_info()}
					</DropdownMenu.Item>
					<DropdownMenu.Item
						disabled
					>
						<CreditCardIcon class="mr-2 h-4 w-4" />
						{m.nav_account()}
					</DropdownMenu.Item>
					<DropdownMenu.Separator />
					<DropdownMenu.Item
						onclick={() => onSelect({ itemId: '__logout__', itemTitle: 'Logout' })}
					>
						<LogOutIcon class="mr-2 h-4 w-4" />
						{m.nav_log_out()}
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
