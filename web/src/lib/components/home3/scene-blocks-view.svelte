<script lang="ts">
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
		heroEyebrow = 'Subject Wiki',
		heroTitle = 'Scene Blocks',
		heroDescription = 'Inspect the event-driven scenes an LLM extracted from each document: who acts, what triggers the scene, how it unfolds, and how it resolves.',
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
			label: 'Metadata',
			icon: InfoIcon,
			ux: -D,
			uy: -D,
			attrs: [
				{ key: 'keywords', label: 'Keywords', icon: TagIcon, kind: 'kw', field: 'keywords' },
				{ key: 'states', label: 'States', icon: ActivityIcon, kind: 'str', field: 'states' }
			]
		},
		{
			id: 'inputs',
			label: 'Inputs & Resources',
			icon: LogInIcon,
			ux: D,
			uy: -D,
			attrs: [
				{ key: 'triggers', label: 'Triggers', icon: ZapIcon, kind: 'str', field: 'triggers' },
				{ key: 'preconditions', label: 'Preconditions', icon: CircleCheckIcon, kind: 'str', field: 'preconditions' },
				{ key: 'constraints', label: 'Constraints', icon: LockIcon, kind: 'str', field: 'constraints' },
				{ key: 'resources', label: 'Resources', icon: DatabaseIcon, kind: 'entity', field: 'resources' },
				{ key: 'source_refs', label: 'Source Refs', icon: FileTextIcon, kind: 'srcrefs', field: 'source_refs' }
			]
		},
		{
			id: 'actions',
			label: 'System Actions',
			icon: ZapIcon,
			ux: D,
			uy: D,
			attrs: [
				{ key: 'actors', label: 'Actors', icon: UsersIcon, kind: 'entity', field: 'actors' },
				{ key: 'actions', label: 'Actions', icon: PlayIcon, kind: 'actions', field: 'actions' },
				{ key: 'decisions', label: 'Decisions', icon: GitBranchIcon, kind: 'str', field: 'decisions' },
				{ key: 'resolutions', label: 'Resolutions', icon: FlagIcon, kind: 'str', field: 'resolutions' }
			]
		},
		{
			id: 'reasoning',
			label: 'Reasoning Logs',
			icon: BrainIcon,
			ux: -D,
			uy: D,
			attrs: [
				{ key: 'outcomes', label: 'Outcomes', icon: TargetIcon, kind: 'str', field: 'outcomes' },
				{ key: 'root_causes', label: 'Root Causes', icon: GitForkIcon, kind: 'str', field: 'root_causes' },
				{ key: 'failure_modes', label: 'Failure Modes', icon: TriangleAlertIcon, kind: 'str', field: 'failure_modes' },
				{ key: 'relationships', label: 'Relationships', icon: Share2Icon, kind: 'rels', field: 'relationships' },
				{ key: 'discriminators', label: 'Discriminators', icon: LayersIcon, kind: 'disc', field: 'discriminators' }
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
	getItemSecondaryIdLabel="Scene ID"
	getItemEvidenceLines={(b) => b.evidence_lines ?? []}
	getItemCreateTime={(b) => b.create_time ?? ''}
	storagePrefix="scene-blocks"
	itemsLabel="Scene Blocks"
	itemLabelSingular="scene block"
	canvasItemLabel="Scene Block"
	itemTypeFilterLabel="Scene Type"
	emptyTableName="kb.scene_objects"
	emptySubtitle="Scene blocks are produced by the generate-scene-blocks processor once the document is processed."
	browserSubtitle="Search, filter, and select a record to inspect its extracted scene blocks."
	canvasMapLabel="Scene Map"
/>
