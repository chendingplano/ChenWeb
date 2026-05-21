<script lang="ts">
	import TagIcon from '@lucide/svelte/icons/tag';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LogInIcon from '@lucide/svelte/icons/log-in';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import UsersIcon from '@lucide/svelte/icons/users';
	import PackageIcon from '@lucide/svelte/icons/package';
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
		heroEyebrow = 'Subject Wiki',
		heroTitle = 'Products',
		heroDescription = 'Inspect the product and part relations an LLM extracted from each document: what is referenced, how it is used, and what requirements apply.',
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
			label: 'Metadata',
			icon: TagIcon,
			attrs: [
				{ key: 'product_type', label: 'Product Type', icon: TagIcon, kind: 'text', field: 'product_type' },
				{ key: 'canonical_name', label: 'Canonical Name', icon: FileTextIcon, kind: 'text', field: 'canonical_name' }
			]
		},
		{
			id: 'grounding',
			label: 'Grounding',
			icon: InfoIcon,
			attrs: [
				{ key: 'evidence_quote', label: 'Evidence Quote', icon: FileTextIcon, kind: 'text', field: 'evidence_quote' },
				{ key: 'confidence_reason', label: 'Confidence Reason', icon: InfoIcon, kind: 'text', field: 'confidence_reason' }
			]
		},
		{
			id: 'inputs',
			label: 'Inputs',
			icon: LogInIcon,
			attrs: [
				{ key: 'conditions', label: 'Conditions', icon: CircleCheckIcon, kind: 'str', field: 'conditions' },
				{ key: 'parameters', label: 'Parameters', icon: SettingsIcon, kind: 'str', field: 'parameters' }
			]
		},
		{
			id: 'actors',
			label: 'Actors',
			icon: UsersIcon,
			attrs: [
				{ key: 'responsible_actor', label: 'Responsible Actor', icon: UsersIcon, kind: 'text', field: 'responsible_actor' }
			]
		},
		{
			id: 'requirements',
			label: 'Requirements',
			icon: ClipboardListIcon,
			attrs: [
				{ key: 'obligation_level', label: 'Obligation Level', icon: CircleCheckIcon, kind: 'text', field: 'obligation_level' },
				{ key: 'exceptions', label: 'Exceptions', icon: TriangleAlertIcon, kind: 'str', field: 'exceptions' },
				{ key: 'requirement_text', label: 'Requirement Text', icon: FileTextIcon, kind: 'text', field: 'requirement_text' }
			]
		},
		{
			id: 'relations',
			label: 'Relations',
			icon: Share2Icon,
			attrs: [
				{ key: 'relation_type', label: 'Relation Type', icon: GitBranchIcon, kind: 'text', field: 'relation_type' },
				{ key: 'related_products', label: 'Related Products', icon: PackageIcon, kind: 'kw', field: 'related_products' }
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
	itemsLabel="Products"
	itemLabelSingular="product"
	canvasItemLabel="Product"
	itemTypeFilterLabel="Product Type"
	emptyTableName="kb.products"
	emptySubtitle="Products are produced by the extract-products processor once the document is processed."
	browserSubtitle="Search, filter, and select a record to inspect its extracted product relations."
	canvasMapLabel="Product Map"
/>
