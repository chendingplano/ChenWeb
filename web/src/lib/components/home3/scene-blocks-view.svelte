<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LogInIcon from '@lucide/svelte/icons/log-in';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import BrainIcon from '@lucide/svelte/icons/brain';
	import TagIcon from '@lucide/svelte/icons/tag';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import LockIcon from '@lucide/svelte/icons/lock';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import UsersIcon from '@lucide/svelte/icons/users';
	import PlayIcon from '@lucide/svelte/icons/play';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import TargetIcon from '@lucide/svelte/icons/target';
	import GitForkIcon from '@lucide/svelte/icons/git-fork';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import Share2Icon from '@lucide/svelte/icons/share-2';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import KbExtractionView, { type AttrDef, type GroupDef } from './kb-extraction-view.svelte';
	import { buildSceneBlockMetaSections } from './scene-block-meta.js';
	import { listKbSceneBlocks, type KbSceneBlockRecord } from '$lib/services/kbService';

	let {
		darkMode = true,
		browserInstanceKey = 'scene-blocks',
		scopeToActiveStore = false,
		heroEyebrow = m.scene_blocks_subject_wiki(),
		heroTitle = m.scene_blocks_scene_blocks(),
		heroDescription = m.scene_blocks_inspect_the_event_driven_scenes(),
		onFocusModeChange
	}: {
		darkMode?: boolean;
		browserInstanceKey?: string;
		scopeToActiveStore?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
		onFocusModeChange?: (focused: boolean) => void;
	} = $props();

	const D = Math.SQRT1_2;

	const SCENE_GROUPS: GroupDef[] = [
		{
			id: 'metadata',
			label: m.scene_blocks_metadata(),
			icon: InfoIcon,
			ux: -D,
			uy: -D,
			attrs: [
				{ key: 'keywords', label: m.scene_blocks_keywords(), icon: TagIcon, kind: 'kw', field: 'keywords' },
				{ key: 'states', label: m.scene_blocks_states(), icon: ActivityIcon, kind: 'str', field: 'states' }
			]
		},
		{
			id: 'inputs',
			label: m.scene_blocks_inputs_resources(),
			icon: LogInIcon,
			ux: D,
			uy: -D,
			attrs: [
				{ key: 'triggers', label: m.scene_blocks_triggers(), icon: ZapIcon, kind: 'str', field: 'triggers' },
				{ key: 'preconditions', label: m.scene_blocks_preconditions(), icon: CircleCheckIcon, kind: 'str', field: 'preconditions' },
				{ key: 'constraints', label: m.scene_blocks_constraints(), icon: LockIcon, kind: 'str', field: 'constraints' },
				{ key: 'resources', label: m.scene_blocks_resources(), icon: DatabaseIcon, kind: 'entity', field: 'resources' },
				{ key: 'source_refs', label: m.scene_blocks_source_refs(), icon: FileTextIcon, kind: 'srcrefs', field: 'source_refs' }
			]
		},
		{
			id: 'actions',
			label: m.scene_blocks_system_actions(),
			icon: ZapIcon,
			ux: D,
			uy: D,
			attrs: [
				{ key: 'actors', label: m.scene_blocks_actors(), icon: UsersIcon, kind: 'entity', field: 'actors' },
				{ key: 'actions', label: m.scene_blocks_actions(), icon: PlayIcon, kind: 'actions', field: 'actions' },
				{ key: 'decisions', label: m.scene_blocks_decisions(), icon: GitBranchIcon, kind: 'str', field: 'decisions' },
				{ key: 'resolutions', label: m.scene_blocks_resolutions(), icon: FlagIcon, kind: 'str', field: 'resolutions' }
			]
		},
		{
			id: 'reasoning',
			label: m.scene_blocks_reasoning_logs(),
			icon: BrainIcon,
			ux: -D,
			uy: D,
			attrs: [
				{ key: 'outcomes', label: m.scene_blocks_outcomes(), icon: TargetIcon, kind: 'str', field: 'outcomes' },
				{ key: 'root_causes', label: m.scene_blocks_root_causes(), icon: GitForkIcon, kind: 'str', field: 'root_causes' },
				{ key: 'failure_modes', label: m.scene_blocks_failure_modes(), icon: TriangleAlertIcon, kind: 'str', field: 'failure_modes' },
				{ key: 'relationships', label: m.scene_blocks_relationships(), icon: Share2Icon, kind: 'rels', field: 'relationships' },
				{ key: 'discriminators', label: m.scene_blocks_discriminators(), icon: LayersIcon, kind: 'disc', field: 'discriminators' }
			]
		}
	];

	function arr(value: unknown): any[] {
		return Array.isArray(value) ? value : [];
	}
	function strList(block: KbSceneBlockRecord, field: string): string[] {
		return arr((block as any)[field]).filter((v) => typeof v === 'string' && v.trim() !== '');
	}
	function sortedActions(block: KbSceneBlockRecord) {
		return [...arr(block.actions)].sort(
			(a, b) => (Number(a?.sequence) || 0) - (Number(b?.sequence) || 0)
		);
	}

	function sceneAttrRaw(block: KbSceneBlockRecord, def: AttrDef): any[] {
		if (def.kind === 'str' || def.kind === 'kw') return strList(block, def.field);
		if (def.kind === 'actions') return sortedActions(block);
		return arr((block as any)[def.field]);
	}

	async function loadItems(recordId: number) {
		return listKbSceneBlocks(recordId);
	}
</script>

<KbExtractionView
	{darkMode}
	{browserInstanceKey}
	{scopeToActiveStore}
	{heroEyebrow}
	{heroTitle}
	{heroDescription}
	{onFocusModeChange}
	groups={SCENE_GROUPS}
	loadItems={loadItems}
	attrRaw={sceneAttrRaw}
	buildMetaSections={(b) => buildSceneBlockMetaSections(b) as Array<{ label: string; kind: 'text' | 'lines' | 'chips'; value?: string; items?: string[] }>}
	getItemId={(b) => b.id}
	getItemType={(b) => b.scene_type?.trim() ?? ''}
	getItemTitle={(b) => b.title?.trim() ?? ''}
	getItemTitleEn={(b) => b.title_en?.trim() ?? ''}
	getItemSummary={(b) => b.summary?.trim() ?? ''}
	getItemKeywords={(b) =>
		Array.isArray(b.keywords) ? b.keywords.filter((v: any) => typeof v === 'string' && v.trim()) : []}
	getItemConfidence={(b) => Number(b.confidence) || 0}
	getItemObjectId={(b) => b.object_id ?? ''}
	getItemSecondaryId={(b) => b.scene_id ?? ''}
	getItemSecondaryIdLabel={m.scene_blocks_scene_id()}
	getItemEvidenceLines={(b) => b.line_spans ?? []}
	getItemCreateTime={(b) => b.create_time ?? ''}
	storagePrefix="scene-blocks"
	itemsLabel={m.scene_blocks_scene_blocks()}
	itemLabelSingular={m.scene_blocks_scene_block()}
	canvasItemLabel={m.scene_blocks_scene_block_2()}
	itemTypeFilterLabel={m.scene_blocks_scene_type()}
	emptyTableName="kb.scene_objects"
	emptySubtitle={m.scene_blocks_scene_blocks_are_produced_by()}
	browserSubtitle={m.scene_blocks_search_filter_and_select_a()}
	canvasMapLabel={m.scene_blocks_scene_map()}
/>
