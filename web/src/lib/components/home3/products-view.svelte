<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Snippet } from 'svelte';
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
	import { GoldRunFilter, goldRunBadges } from './gold-run-filter.svelte';
	import GoldRunFilters from './gold-run-filters.svelte';
	import {
		listKbProducts,
		type KbDroppedProduct,
		type KbInputRecord,
		type KbProductRecord
	} from '$lib/services/kbService';

	let {
		darkMode = true,
		browserInstanceKey = 'products',
		scopeToActiveStore = false,
		heroEyebrow = m.products_knowledge_system_vol_iii(),
		heroTitle = m.products_products_provenance(),
		heroDescription = m.products_browse_product_records_extracted_from(),
		onFocusModeChange,
		source = 'kb'
	}: {
		darkMode?: boolean;
		browserInstanceKey?: string;
		scopeToActiveStore?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
		onFocusModeChange?: (focused: boolean) => void;
		/** 'testbed': read-only Gold Products over one extract-products-benchmark run. */
		source?: 'kb' | 'testbed';
	} = $props();

	const gold = new GoldRunFilter('products', { allowAllVersions: true });
	$effect(() => {
		if (source === 'testbed') void gold.load();
	});
	let productView = $state<'gold' | 'dropped'>('gold');
	// Offset keeps dropped-candidate item IDs apart from gold row IDs in the shared list.
	const DROPPED_ID_OFFSET = 1_000_000_000;

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

	// A dropped candidate shown in the product list: its rule ID is the type, its rationale the summary.
	function droppedItem(d: KbDroppedProduct): KbProductRecord {
		return {
			id: DROPPED_ID_OFFSET + d.id,
			product_rel_id: d.candidate_id,
			product_name: d.text ?? '',
			canonical_name: d.text ?? '',
			product_type: d.drop_reason,
			relation_type: d.decision,
			relation_summary: d.reason ?? '',
			evidence_lines: d.source_line_spans,
			conditions: [],
			exceptions: [],
			parameters: [],
			related_products: [],
			confidence: 0,
			model_name: '',
			prompt_name: '',
			create_time: '',
			modify_time: ''
		};
	}

	async function loadItems(recordId: number, record: KbInputRecord) {
		if (source !== 'testbed') return listKbProducts(recordId);
		// Benchmark entries carry exactly one run; the gold rows are that run's.
		const run = record.gold_metric_runs?.[0];
		if (!run) throw new Error(m.products_gold_no_run());
		const response = await listKbProducts(recordId, run);
		return productView === 'dropped'
			? { ...response, results: (response.dropped ?? []).map(droppedItem) }
			: response;
	}

	function mapGoldRecord(input: KbInputRecord) {
		return {
			id: input.id,
			title: input.title?.trim() || input.name?.trim() || input.file_name?.trim() || `#${input.id}`,
			subtitle: input.file_name || input.name || '',
			meta: [input.doc_no || '', input.type || ''].filter(Boolean),
			status: input.type || '',
			description: m.products_gold_select_document(),
			badges: [input.type || ''].filter(Boolean),
			benchmarkRuns: goldRunBadges(input)
		};
	}
</script>

{#snippet goldControls()}
	<GoldRunFilters filter={gold} />
	<label class="gold-view">
		<span>{m.products_gold_view()}</span>
		<select bind:value={productView}>
			<option value="gold">{m.products_gold_view_gold()}</option>
			<option value="dropped">{m.products_gold_view_dropped()}</option>
		</select>
	</label>
{/snippet}

<KbExtractionView
	{darkMode}
	browserInstanceKey={source === 'testbed' ? 'gold-products' : browserInstanceKey}
	{scopeToActiveStore}
	{heroEyebrow}
	heroTitle={source === 'testbed' ? m.products_gold_title() : heroTitle}
	heroDescription={source === 'testbed' ? m.products_gold_subtitle() : heroDescription}
	{onFocusModeChange}
	recordListParams={source === 'testbed' ? gold.listParams : undefined}
	recordListPausedMessage={source === 'testbed' ? gold.pausedMessage : ''}
	recordBrowserControls={source === 'testbed' ? (goldControls as Snippet) : undefined}
	mapRecord={source === 'testbed' ? mapGoldRecord : undefined}
	reloadToken={productView}
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
	storagePrefix={source === 'testbed' ? 'gold-products' : 'products'}
	itemsLabel={m.products_products()}
	itemLabelSingular={m.products_product()}
	canvasItemLabel={m.products_product_2()}
	itemTypeFilterLabel={m.products_product_type()}
	emptyTableName="kb.products"
	emptySubtitle={m.products_products_are_produced_by_the()}
	browserSubtitle={m.products_search_filter_and_select_a()}
	canvasMapLabel={m.products_product_map()}
/>

<style>
	.gold-view {
		display: flex;
		flex-direction: column;
		gap: 4px;
		margin-top: 8px;
		font-family: var(--font-mono, ui-monospace, monospace);
		font-size: 11px;
		color: var(--text-muted);
	}
	.gold-view select {
		padding: 6px 8px;
		border: 1px solid var(--ink-line);
		border-radius: 4px;
		background: var(--panel-bg);
		color: var(--text-primary);
		font: inherit;
		font-size: 12px;
	}
</style>
