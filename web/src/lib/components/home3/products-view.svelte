<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import TagIcon from '@lucide/svelte/icons/tag';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LogInIcon from '@lucide/svelte/icons/log-in';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import UsersIcon from '@lucide/svelte/icons/users';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import Share2Icon from '@lucide/svelte/icons/share-2';
	import ClipboardListIcon from '@lucide/svelte/icons/clipboard-list';
	import KbExtractionView, { type AttrDef, type GroupDef } from './kb-extraction-view.svelte';
	import { buildProductMetaSections } from './product-meta.js';
	import { listKbProducts, type KbProductRecord } from '$lib/services/kbService';

	let {
		darkMode = true,
		browserInstanceKey = 'products',
		scopeToActiveStore = false,
		heroEyebrow = m.products_knowledge_system_vol_iii(),
		heroTitle = m.products_products_provenance(),
		heroDescription = m.products_browse_product_records_extracted_from(),
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

	const PRODUCT_GROUPS: GroupDef[] = [
		{
			id: 'metadata',
			label: m.products_metadata(),
			icon: TagIcon,
			attrs: [
				{ key: 'product_type', label: m.products_product_type(), icon: TagIcon, kind: 'text', field: 'product_type' },
				{ key: 'canonical_name', label: m.products_canonical_name(), icon: FileTextIcon, kind: 'text', field: 'canonical_name' }
			]
		},
		{
			id: 'grounding',
			label: m.products_grounding(),
			icon: InfoIcon,
			attrs: [
				{ key: 'evidence_quote', label: m.products_evidence_quote(), icon: FileTextIcon, kind: 'text', field: 'evidence_quote' },
				{ key: 'confidence_reason', label: m.products_confidence_reason(), icon: InfoIcon, kind: 'text', field: 'confidence_reason' }
			]
		},
		{
			id: 'inputs',
			label: m.products_inputs(),
			icon: LogInIcon,
			attrs: [
				{ key: 'conditions', label: m.products_conditions(), icon: CircleCheckIcon, kind: 'str', field: 'conditions' },
				{ key: 'parameters', label: m.products_parameters(), icon: SettingsIcon, kind: 'str', field: 'parameters' }
			]
		},
		{
			id: 'actors',
			label: m.products_actors(),
			icon: UsersIcon,
			attrs: [
				{ key: 'responsible_actor', label: m.products_responsible_actor(), icon: UsersIcon, kind: 'text', field: 'responsible_actor' }
			]
		},
		{
			id: 'requirements',
			label: m.products_requirements(),
			icon: ClipboardListIcon,
			attrs: [
				{ key: 'obligation_level', label: m.products_obligation_level(), icon: CircleCheckIcon, kind: 'text', field: 'obligation_level' },
				{ key: 'exceptions', label: m.products_exceptions(), icon: TriangleAlertIcon, kind: 'str', field: 'exceptions' },
				{ key: 'requirement_text', label: m.products_requirement_text(), icon: FileTextIcon, kind: 'text', field: 'requirement_text' }
			]
		},
		{
			id: 'relations',
			label: m.products_relations(),
			icon: Share2Icon,
			attrs: [
				{ key: 'relation_type', label: m.products_relation_type(), icon: GitBranchIcon, kind: 'text', field: 'relation_type' }
			]
		}
	];

	function arr(value: unknown): any[] {
		return Array.isArray(value) ? value : [];
	}

	function productAttrRaw(product: KbProductRecord, def: AttrDef): any[] {
		if (def.kind === 'text') {
			const val = (product as any)[def.field];
			return typeof val === 'string' && val.trim() ? [val.trim()] : [];
		}
		if (def.kind === 'str' || def.kind === 'kw') {
			return arr((product as any)[def.field]).filter(
				(v: any) => typeof v === 'string' && v.trim()
			);
		}
		return arr((product as any)[def.field]);
	}

	async function loadItems(recordId: number) {
		return listKbProducts(recordId);
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
	groups={PRODUCT_GROUPS}
	loadItems={loadItems}
	attrRaw={productAttrRaw}
	buildMetaSections={(p) => buildProductMetaSections(p) as Array<{ label: string; kind: 'text' | 'lines' | 'chips'; value?: string; items?: string[] }>}
	getItemId={(p) => p.id}
	getItemType={(p) => p.product_type?.trim() ?? ''}
	getItemTitle={(p) => p.product_name?.trim() ?? ''}
	getItemTitleEn={(p) => p.product_name_en?.trim() ?? ''}
	getItemSummary={(p) => p.relation_summary?.trim() ?? ''}
	getItemKeywords={(_p) => []}
	getItemConfidence={(p) => Number(p.confidence) || 0}
	getItemObjectId={(p) => p.product_rel_id ?? ''}
	getItemSecondaryId={(_p) => ''}
	getItemEvidenceLines={(p) => p.evidence_lines ?? []}
	getItemCreateTime={(p) => p.create_time ?? ''}
	storagePrefix="products"
	itemsLabel={m.products_products()}
	itemLabelSingular={m.products_product()}
	canvasItemLabel={m.products_product_2()}
	itemTypeFilterLabel={m.products_product_type()}
	emptyTableName="kb.products"
	emptySubtitle={m.products_products_are_produced_by_the()}
	browserSubtitle={m.products_search_filter_and_select_a()}
	canvasMapLabel={m.products_product_map()}
/>
