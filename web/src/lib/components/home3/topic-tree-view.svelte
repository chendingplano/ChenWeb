<script lang="ts">
	import PanelsTopLeftIcon from '@lucide/svelte/icons/panels-top-left';
	import Rows3Icon from '@lucide/svelte/icons/rows-3';
	import {
		getRecordTopics,
		listTopicGraph,
		updateRecordTopic,
		getKbFrontendConfig,
		type KbInputRecord,
		type ListTopicGraphResponse,
		type TopicCategoryNodeApi,
		type TopicCardApi
	} from '$lib/services/kbService';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import {
		buildTopicCategoryMetadataByPath,
		buildTopicCategoryPathDisplay,
		type TopicCategoryPathMetadata
	} from './topic-category-paths';
	import {
		createTopicTreeState,
		selectTopicTreeRecord,
		toggleTopicTreeListMode
	} from './topic-tree-state.js';
	import { selectTopicTreeViewerTarget } from './topic-tree-selection.js';
	import { formatTopicLineSpecs } from './topic-line-specs';
	import type { TopicRecordTarget, TopicTreeRecord } from './topic-types';

	let {
		darkMode = true,
		heroEyebrow = 'Semantic Web',
		heroTitle = 'Document Semantic Tree',
		heroDescription = 'Document-centric browser over topics extracted from chunks.',
		sidebarTitle = 'Selected Topic',
		loadErrorTitle = 'Topic Tree',
		itemSingular = 'topic',
		itemPlural = 'topics',
		getRecordItems = getRecordTopics,
		listCategoryGraph = listTopicGraph,
		scopeToActiveStore = false,
		browserInstanceKey = 'topic-tree'
	}: {
		darkMode?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
		sidebarTitle?: string;
		loadErrorTitle?: string;
		itemSingular?: string;
		itemPlural?: string;
		getRecordItems?: (recordId: number) => Promise<{ topics: TopicCardApi[] }>;
		listCategoryGraph?: () => Promise<ListTopicGraphResponse>;
		scopeToActiveStore?: boolean;
		browserInstanceKey?: string;
	} = $props();

	// Letterpress palette — kept in lockstep with metric-mgmt-view so this view
	// (and the record browser it hosts) matches the Metrics page in both themes.
	let panelBg = $derived(darkMode ? '#161A22' : '#FBF8F0');
	let panelAlt = $derived(darkMode ? '#1C212C' : '#F0EADB');
	let border = $derived(darkMode ? '#2A3140' : '#D7CFB8');
	let borderSoft = $derived(darkMode ? '#1F2530' : '#E5DEC8');
	let textMain = $derived(darkMode ? '#EDE7D3' : '#1A1410');
	let textMuted = $derived(darkMode ? '#B5AE94' : '#5C5345');
	let accent = $derived(darkMode ? '#D4A24C' : '#B8801E'); // brass
	let crimson = $derived(darkMode ? '#C8553D' : '#A23E26'); // selected/active

	let loadError = $state('');
	let errorDialogOpen = $state(false);
	let topicLoadingByRecordId = $state<Record<number, boolean>>({});
	let recordCache = $state<Record<number, TopicTreeRecord>>({});
	let treeState = $state(createTopicTreeState());
	let categoryMetadataByPath = $state<Record<string, TopicCategoryPathMetadata>>({});
	let categoryMetadataLoaded = $state(false);
	let truncatedCategoryPaths = $state<Record<string, boolean>>({});
	let docPage = $state(1);
	let pdfZoom = $state(0.5);
	let pdfNumPages = $state(0);
	let detailCardWidth = $state(340);
	let resizing = $state(false);
	let resizeStartX = 0;
	let resizeStartWidth = 0;

	let editingTopic = $state(false);
	let editTopicType = $state('');
	let editTopicText = $state('');
	let editTopicDescEn = $state('');
	let editKeywordsRaw = $state('');
	let editKeywordsEnRaw = $state('');
	let editCategoryPathsRaw = $state('');
	let editCategoryPathsEnRaw = $state('');
	let editSaving = $state(false);
	let editError = $state('');

	let topicTypeOptions = $state<string[]>([]);
	let configLoaded = $state(false);

	let activeRecord = $derived(
		treeState.selectedRecordId != null ? (recordCache[treeState.selectedRecordId] ?? null) : null
	);
	let selectedTopic = $derived(
		activeRecord?.topics.find((topic) => topic.id === treeState.selectedTopicId) ?? null
	);
	let viewerInputId = $derived(treeState.selectedPdfTarget?.inputId ?? activeRecord?.id ?? null);
	let viewerFileUrl = $derived(viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : '');
	let viewerIsPdf = $derived((activeRecord?.fileName ?? '').trim().toLowerCase().endsWith('.pdf'));
	let selectedTopicCategoryPaths = $derived(
		selectedTopic?.categoryPaths?.map((path) =>
			buildTopicCategoryPathDisplay(path, categoryMetadataByPath)
		) ?? []
	);

	type TopicPdfViewport = { width: number; height: number };

	function renderTopicHighlight(
		pageNo: number,
		viewport: TopicPdfViewport,
		overlay: HTMLDivElement
	) {
		if (!selectedTopic) return;
		const targets = selectedTopic.targets?.filter(
			(target: TopicRecordTarget) =>
				target.page === pageNo && Array.isArray(target.coords) && target.coords.length >= 4
		);
		if (!targets || targets.length === 0) return;
		for (const target of targets) {
			const vx1 = target.coords[0] * viewport.width / 1000, vy1 = target.coords[1] * viewport.height / 1000, vx2 = target.coords[2] * viewport.width / 1000, vy2 = target.coords[3] * viewport.height / 1000;
			const left = Math.max(0, Math.min(vx1, vx2) - 5);
			const top = Math.max(0, Math.min(vy1, vy2) - 4);
			const width = Math.abs(vx2 - vx1) + 10;
			const height = Math.abs(vy2 - vy1) + 8;
			if (width < 1 || height < 1) continue;
			const box = document.createElement('div');
			box.className = 'pdf-highlight';
			box.style.left = `${left}px`;
			box.style.top = `${top}px`;
			box.style.width = `${width}px`;
			box.style.height = `${height}px`;
			overlay.appendChild(box);
		}
	}

	function topicTreeStatusText(record: KbInputRecord): string {
		const items = record.status ?? [];
		const matched = [...items].reverse().find((item) => item != null);
		if (!matched) return 'pending';
		return (
			matched.proc_status?.trim() ||
			matched['proc-status']?.trim() ||
			matched.status?.trim() ||
			'pending'
		);
	}

	function mapKbInputToTopicTreeRecord(record: KbInputRecord): TopicTreeRecord {
		return {
			id: record.id,
			title:
				record.title?.trim() ||
				record.name?.trim() ||
				record.file_name?.trim() ||
				`Record #${record.id}`,
			fileName: record.file_name?.trim() || record.name?.trim() || '—',
			docType: record.type?.trim() || '—',
			docNo: record.doc_no?.trim() || '—',
			parserName: record.parser_name?.trim() || '—',
			procStatus: topicTreeStatusText(record),
			createTime: record.create_time ? new Date(record.create_time).toLocaleString() : '—',
			modifyTime: record.modify_time ? new Date(record.modify_time).toLocaleString() : '—',
			topics: []
		};
	}

	$effect(() => {
		const selectedPage = treeState.selectedPdfTarget?.page;
		if (selectedPage) {
			docPage = selectedPage;
		}
	});

	$effect(() => {
		if (activeRecord) {
			void ensureRecordTopics(activeRecord.id);
		}
	});

	$effect(() => {
		if (!categoryMetadataLoaded) {
			void ensureCategoryMetadata();
		}
	});

	$effect(() => {
		if (!configLoaded) {
			configLoaded = true;
			getKbFrontendConfig()
				.then((cfg) => {
					topicTypeOptions = cfg.topic_types ?? [];
				})
				.catch(() => {});
		}
	});

	function selectTopic(topic: TopicTreeRecord['topics'][number], recordId: number) {
		const next = selectTopicTreeViewerTarget(treeState, {
			recordId,
			topicId: topic.id,
			inputId: topic.inputId,
			page: topic.page
		});
		/*
		console.log('[topic-tree] selectTopic jump request', {
			recordId,
			topicId: topic.id,
			inputId: topic.inputId,
			jumpPage: next.page,
			selectedTopicPage: topic.page,
			lineNumbers: topic.sourceLineSpecs ?? [],
			targets: topic.targets ?? []
		});
		*/
		treeState = next.treeState;
		docPage = next.page;
	}

	async function ensureRecordTopics(recordId: number) {
		const current = recordCache[recordId];
		if (!current || current.topics.length > 0 || topicLoadingByRecordId[recordId]) return;

		topicLoadingByRecordId = { ...topicLoadingByRecordId, [recordId]: true };
		try {
			const response = await getRecordItems(recordId);
			recordCache = {
				...recordCache,
				[recordId]: {
					...current,
					topics: [...response.topics]
						.sort((a, b) => a.id.localeCompare(b.id, undefined, { numeric: true }))
						.map((topic) => ({ ...topic, recordId }))
				}
			};
			if (
				treeState.selectedRecordId === recordId &&
				response.topics[0] &&
				!treeState.selectedPdfTarget
			) {
				selectTopic({ ...response.topics[0], recordId }, recordId);
			}
		} catch (error) {
			loadError =
				error instanceof Error
					? error.message
					: `Failed to load ${itemPlural} for record ${recordId}`;
			errorDialogOpen = true;
		} finally {
			topicLoadingByRecordId = { ...topicLoadingByRecordId, [recordId]: false };
		}
	}

	async function ensureCategoryMetadata() {
		if (categoryMetadataLoaded) return;
		categoryMetadataLoaded = true;
		try {
			const response = await listCategoryGraph();
			categoryMetadataByPath = buildTopicCategoryMetadataByPath(
				(response.results ?? []) as TopicCategoryNodeApi[]
			);
		} catch (error) {
			console.warn('failed to load topic category metadata', error);
		}
	}

	function trackPathOverflow(node: HTMLElement, pathKey: string) {
		let currentPathKey = pathKey;
		const update = () => {
			const next = node.scrollWidth > node.clientWidth + 1;
			if (truncatedCategoryPaths[currentPathKey] !== next) {
				truncatedCategoryPaths = { ...truncatedCategoryPaths, [currentPathKey]: next };
			}
		};
		const resizeObserver = new ResizeObserver(update);
		resizeObserver.observe(node);
		queueMicrotask(update);
		return {
			update(nextPathKey: string) {
				currentPathKey = nextPathKey;
				queueMicrotask(update);
			},
			destroy() {
				resizeObserver.disconnect();
			}
		};
	}

	function handleRecordSelect(record: KbInputRecord) {
		const mapped = mapKbInputToTopicTreeRecord(record);
		recordCache = {
			...recordCache,
			[record.id]: {
				...(recordCache[record.id] ?? mapped),
				...mapped,
				topics: recordCache[record.id]?.topics ?? []
			}
		};
		treeState = selectTopicTreeRecord(treeState, record.id);
	}

	function mapBrowserRecord(record: KbInputRecord) {
		const mapped = mapKbInputToTopicTreeRecord(record);
		return {
			id: record.id,
			title: mapped.title,
			subtitle: mapped.fileName,
			meta: [mapped.docNo, mapped.parserName],
			status: mapped.procStatus,
			description: mapped.createTime,
			badges: [mapped.docType]
		};
	}

	function hasNonAscii(s: string): boolean {
		return /[^ -]/.test(s);
	}

	function arraysEqual(a: string[], b: string[]): boolean {
		return a.length === b.length && a.every((v, i) => v === b[i]);
	}

	function lookupTopicTypeEn(typeValue: string): string {
		if (!typeValue || !hasNonAscii(typeValue)) return '';
		for (const entry of topicTypeOptions) {
			const parts = entry.split('/');
			if (parts.length >= 2 && parts[0].trim() === typeValue.trim()) {
				return parts.slice(1).join('/').trim();
			}
			if (entry.trim() === typeValue.trim()) {
				return '';
			}
		}
		return '';
	}

	function startTopicEdit() {
		if (!selectedTopic) return;
		editTopicType = selectedTopic.topicType ?? '';
		editTopicText = selectedTopic.topicText ?? '';
		editTopicDescEn = selectedTopic.topicDescEn ?? '';
		editKeywordsRaw = (selectedTopic.topicKeywords ?? []).join('\n');
		editKeywordsEnRaw = (selectedTopic.topicKeywordsEn ?? []).join('\n');
		editCategoryPathsRaw = (selectedTopic.categoryPaths ?? []).join('\n');
		editCategoryPathsEnRaw = (selectedTopic.categoryPathsEn ?? []).join('\n');
		editError = '';
		editingTopic = true;
	}

	function cancelTopicEdit() {
		editingTopic = false;
		editError = '';
	}

	async function saveTopicEdit() {
		if (!selectedTopic || editSaving) return;
		editError = '';
		editSaving = true;
		const newKeywords = editKeywordsRaw
			.split('\n')
			.map((k) => k.trim())
			.filter(Boolean);
		const newKeywordsEn = editKeywordsEnRaw
			.split('\n')
			.map((k) => k.trim())
			.filter(Boolean);
		const newPaths = editCategoryPathsRaw
			.split('\n')
			.map((p) => p.trim())
			.filter(Boolean);
		const newPathsEn = editCategoryPathsEnRaw
			.split('\n')
			.map((p) => p.trim())
			.filter(Boolean);
		try {
			await updateRecordTopic({
				record_id: selectedTopic.recordId,
				topic_id: selectedTopic.id,
				topic_type: editTopicType.trim(),
				topic_text: editTopicText.trim(),
				topic_desc_en: editTopicDescEn.trim(),
				topic_keywords: newKeywords,
				topic_keywords_en: newKeywordsEn,
				category_paths: newPaths,
				category_paths_en: newPathsEn
			});
			const recordId = selectedTopic.recordId;
			const cached = recordCache[recordId];
			if (cached) {
				recordCache = {
					...recordCache,
					[recordId]: {
						...cached,
						topics: cached.topics.map((t) =>
							t.id === selectedTopic!.id
								? {
										...t,
										topicType: editTopicType.trim(),
										topicText: editTopicText.trim(),
										topicDescEn: editTopicDescEn.trim() || undefined,
										topicKeywords: newKeywords,
										topicKeywordsEn: newKeywordsEn.length > 0 ? newKeywordsEn : undefined,
										categoryPaths: newPaths,
										categoryPathsEn: newPathsEn.length > 0 ? newPathsEn : undefined
									}
								: t
						)
					}
				};
			}
			if (newPaths.join('\n') !== (selectedTopic.categoryPaths ?? []).join('\n')) {
				categoryMetadataLoaded = false;
			}
			editingTopic = false;
		} catch (err) {
			editError = err instanceof Error ? err.message : 'Failed to save topic';
		} finally {
			editSaving = false;
		}
	}

	function startResize(event: PointerEvent) {
		resizing = true;
		resizeStartX = event.clientX;
		resizeStartWidth = detailCardWidth;
		document.body.style.cursor = 'col-resize';
		function onMove(e: PointerEvent) {
			detailCardWidth = Math.max(180, Math.min(640, resizeStartWidth + e.clientX - resizeStartX));
		}
		function onUp() {
			resizing = false;
			document.body.style.cursor = '';
			window.removeEventListener('pointermove', onMove);
			window.removeEventListener('pointerup', onUp);
			window.removeEventListener('pointercancel', onUp);
		}
		window.addEventListener('pointermove', onMove);
		window.addEventListener('pointerup', onUp, { once: true });
		window.addEventListener('pointercancel', onUp, { once: true });
	}
</script>

<div
	class="tree-shell"
	class:resizing
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --panel-bg:${panelBg}; --panel-bg-alt:${panelAlt}; --ink-line:${border}; --ink-line-soft:${borderSoft}; --text-primary:${textMain}; --text-secondary:${textMuted}; --text-muted:${textMuted}; --brass:${accent}; --crimson:${crimson};`}
>
	{#if errorDialogOpen && loadError}
		<div
			class="error-overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (errorDialogOpen = false)}
			onkeydown={(event) => {
				if (event.key === 'Escape') errorDialogOpen = false;
			}}
		>
			<div
				class="error-dialog"
				role="dialog"
				aria-modal="true"
				aria-label="Topic Tree Load Error"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				<div class="eyebrow">Load Error</div>
				<h3>Could not load {loadErrorTitle} data</h3>
				<p class="dialog-copy">{loadError}</p>
				<div class="dialog-actions">
					<button type="button" class="ghost" onclick={() => (errorDialogOpen = false)}
						>Close</button
					>
				</div>
			</div>
		</div>
	{/if}

	<div class="hero">
		<div>
			<div class="eyebrow">{heroEyebrow}</div>
			<h2>{heroTitle}</h2>
			<p>{heroDescription}</p>
		</div>
		<div class="hero-actions">
			<button
				type="button"
				class="ghost"
				onclick={() => (treeState = toggleTopicTreeListMode(treeState))}
			>
				{#if treeState.listMode === 'compact'}
					<PanelsTopLeftIcon class="h-4 w-4" />
					Expanded View
				{:else}
					<Rows3Icon class="h-4 w-4" />
					Compact View
				{/if}
			</button>
		</div>
	</div>

	<div class="workspace">
		<KbInputRecordBrowser
			{darkMode}
			instanceKey={browserInstanceKey}
			title="kb.inputs"
			subtitle={`Search, filter, and select records before inspecting their ${itemPlural}.`}
			emptyTitle="No records found."
			emptySubtitle={`Use Search or Retrieve to browse kb.inputs for ${itemPlural}.`}
			{scopeToActiveStore}
			selectedRecordId={treeState.selectedRecordId}
			renderMode={treeState.listMode === 'cards' ? 'cards' : 'compact'}
			mapRecord={mapBrowserRecord}
			onSelect={handleRecordSelect}
		/>

		<div class="right-panel">
			<div class="right-tabs">
				<div class="tab active">PDF Display</div>
				{#if activeRecord}
					<div class="tab passive" title={activeRecord.fileName}>{activeRecord.fileName}</div>
				{/if}
			</div>

			{#if activeRecord}
				<div class="detail-grid">
					<div
						class="detail-card"
						style="width:{detailCardWidth}px; flex: 0 0 {detailCardWidth}px;"
					>
						<div class="eyebrow">Selected Record</div>
						<h3>{activeRecord.title}</h3>
						<div class="detail-meta">
							<span>{activeRecord.docNo}</span>
							<span>{activeRecord.docType}</span>
							<span>{activeRecord.parserName}</span>
						</div>
						<div class="topic-snippets">
							{#each activeRecord.topics as topic}
								<button
									type="button"
									class:selected={treeState.selectedTopicId === topic.id}
									class="snippet"
									onclick={() => selectTopic(topic, activeRecord.id)}
								>
									<div class="snippet-head">
										<div class="snippet-id-group">
											<span class="snippet-id">#{topic.id}</span>
											{#if topic.topicType}
												<span class="snippet-type">{topic.topicType}</span>
											{/if}
										</div>
										<span class="snippet-page">p.{topic.page}</span>
									</div>
									{#if topic.topicKeywords.length > 0}
										<div class="keyword-row snippet-kw-row">
											{#each topic.topicKeywords as kw}
												<span class="keyword">{kw}</span>
											{/each}
										</div>
									{/if}
									{#if topic.sourceLineSpecs && topic.sourceLineSpecs.length > 0}
										<div class="snippet-info-row">
											<span class="snippet-info-label">Lines</span>
											<span class="snippet-info-val"
												>{formatTopicLineSpecs(topic.sourceLineSpecs)}</span
											>
										</div>
									{/if}
									{#if topic.categoryPaths && topic.categoryPaths.length > 0}
										<div class="snippet-info-row">
											<span class="snippet-info-label">Category</span>
											<span class="snippet-info-val snippet-cat-val"
												>{topic.categoryPaths.join(' · ')}</span
											>
										</div>
									{/if}
									<p>{topic.topicText}</p>
								</button>
							{/each}
						</div>
					</div>

					<button
						type="button"
						class="resize-handle"
						class:active={resizing}
						aria-label="Resize selected record panel"
						onpointerdown={startResize}
					>
						<span class="resize-grip" aria-hidden="true"></span>
					</button>

					<div class="pdf-card">
						<div class="eyebrow">PDF Display</div>
						{#if viewerInputId && treeState.selectedPdfTarget && viewerIsPdf}
							<PdfViewWindow
								inputId={viewerInputId}
								fileUrl={viewerFileUrl}
								bind:page={docPage}
								bind:zoom={pdfZoom}
								bind:numPages={pdfNumPages}
								highlightVersion={selectedTopic
									? `${selectedTopic.id}:${selectedTopic.targets.map((target) => `${target.page}:${target.coords.join(',')}`).join('|')}`
									: 'topic-tree'}
								renderHighlights={renderTopicHighlight}
								sidebarMinWidth={240}
								sidebarMaxWidth={520}
								sidebarDefaultWidth={320}
								{sidebarTitle}
								sidebarSettingsKey="topic-tree-pdf-sidebar"
								sidebarWidthSettingLabel="Panel Width"
							>
								{#snippet sidebar()}
									{#if selectedTopic}
										{#if editingTopic}
											<div class="topic-sidebar-edit">
												<div class="topic-edit-field">
													<label class="topic-sidebar-label" for="edit-topic-type">TYPE</label>
													<input
														id="edit-topic-type"
														class="topic-edit-input"
														type="text"
														list="topic-type-options"
														bind:value={editTopicType}
														placeholder="select or type…"
														autocomplete="off"
													/>
													<datalist id="topic-type-options">
														{#each topicTypeOptions as opt (opt)}
															<option value={opt}></option>
														{/each}
													</datalist>
												</div>
												<div class="topic-edit-field">
													<label class="topic-sidebar-label" for="edit-topic-desc"
														>{(
															itemSingular.charAt(0).toUpperCase() + itemSingular.slice(1)
														).toUpperCase()} DESCRIPTION</label
													>
													<textarea
														id="edit-topic-desc"
														class="topic-edit-textarea"
														bind:value={editTopicText}
														rows={4}
														placeholder="topic description (CN)"
													></textarea>
												</div>
												<div class="topic-edit-field">
													<label class="topic-sidebar-label" for="edit-topic-desc-en"
														>{(
															itemSingular.charAt(0).toUpperCase() + itemSingular.slice(1)
														).toUpperCase()} DESCRIPTION (EN)</label
													>
													<textarea
														id="edit-topic-desc-en"
														class="topic-edit-textarea"
														bind:value={editTopicDescEn}
														rows={4}
														placeholder="topic description (EN)"
													></textarea>
												</div>
												<div class="topic-edit-field">
													<label class="topic-sidebar-label" for="edit-category-paths"
														>CATEGORY PATHS</label
													>
													<p class="topic-edit-hint">One path per line (e.g. Root/Sub/Child)</p>
													<textarea
														id="edit-category-paths"
														class="topic-edit-textarea"
														bind:value={editCategoryPathsRaw}
														rows={3}
														placeholder="Root/Category/Subcategory"
													></textarea>
												</div>
												<div class="topic-edit-field">
													<label class="topic-sidebar-label" for="edit-category-paths-en"
														>CATEGORY PATHS (EN)</label
													>
													<p class="topic-edit-hint">One path per line</p>
													<textarea
														id="edit-category-paths-en"
														class="topic-edit-textarea"
														bind:value={editCategoryPathsEnRaw}
														rows={3}
														placeholder="Root/Category/Subcategory (EN)"
													></textarea>
												</div>
												<div class="topic-edit-field">
													<label class="topic-sidebar-label" for="edit-keywords">KEYWORDS</label>
													<p class="topic-edit-hint">One keyword per line</p>
													<textarea
														id="edit-keywords"
														class="topic-edit-textarea"
														bind:value={editKeywordsRaw}
														rows={3}
														placeholder="keyword (CN)"
													></textarea>
												</div>
												<div class="topic-edit-field">
													<label class="topic-sidebar-label" for="edit-keywords-en"
														>KEYWORDS (EN)</label
													>
													<p class="topic-edit-hint">One keyword per line</p>
													<textarea
														id="edit-keywords-en"
														class="topic-edit-textarea"
														bind:value={editKeywordsEnRaw}
														rows={3}
														placeholder="keyword (EN)"
													></textarea>
												</div>
												{#if editError}
													<p class="topic-edit-error">{editError}</p>
												{/if}
												<div class="topic-edit-actions">
													<button
														type="button"
														class="topic-edit-btn topic-edit-btn-save"
														disabled={editSaving}
														onclick={saveTopicEdit}
													>
														{editSaving ? 'Saving…' : 'Save'}
													</button>
													<button
														type="button"
														class="topic-edit-btn topic-edit-btn-cancel"
														disabled={editSaving}
														onclick={cancelTopicEdit}>Cancel</button
													>
												</div>
											</div>
										{:else}
											<div class="topic-sidebar-block">
												<div class="topic-sidebar-row">
													<span>{itemSingular} ID</span>
													<strong>{selectedTopic.id}</strong>
												</div>
												<div class="topic-sidebar-row">
													<span>Record ID</span>
													<strong>{selectedTopic.recordId}</strong>
												</div>
												<div class="topic-sidebar-row">
													<span>Page</span>
													<strong>{selectedTopic.page}</strong>
												</div>
												<div class="topic-sidebar-row">
													<span>Type</span>
													<span class="topic-type-display">
														<strong>{selectedTopic.topicType || '—'}</strong>
														{#if hasNonAscii(selectedTopic.topicType)}
															{@const typeEn = lookupTopicTypeEn(selectedTopic.topicType)}
															{#if typeEn}
																<span class="topic-type-en">({typeEn})</span>
															{/if}
														{/if}
													</span>
												</div>
												<div class="topic-sidebar-row">
													<span>Line Numbers</span>
													<strong>{formatTopicLineSpecs(selectedTopic.sourceLineSpecs)}</strong>
												</div>
											</div>
											<div class="topic-sidebar-block">
												<div class="topic-sidebar-label-row">
													<div class="topic-sidebar-label">
														{itemSingular.charAt(0).toUpperCase() + itemSingular.slice(1)} Description
													</div>
													<button type="button" class="topic-edit-trigger" onclick={startTopicEdit}
														>Edit</button
													>
												</div>
												<p class="topic-sidebar-copy">{selectedTopic.topicText}</p>
												{#if selectedTopic.topicDescEn}
													<p class="topic-sidebar-copy topic-desc-en">
														{selectedTopic.topicDescEn}
													</p>
												{/if}
											</div>
											<div class="topic-sidebar-block">
												<div class="topic-sidebar-label">Category Paths</div>
												{#if selectedTopicCategoryPaths.length > 0}
													<div class="category-path-list">
														{#each selectedTopicCategoryPaths as categoryPath, idx (`${categoryPath.path}-${idx}`)}
															<div class="category-path-row">
																<div class="category-path-line">
																	<div
																		class="category-path-segments"
																		use:trackPathOverflow={categoryPath.path}
																	>
																		{#each categoryPath.segments as segment, segmentIdx (`${segment.path}-${segmentIdx}`)}
																			<span
																				class="keyword-chip category-path-chip"
																				title={segment.tooltip}>{segment.label}</span
																			>
																			{#if segmentIdx < categoryPath.segments.length - 1}
																				<span class="category-path-separator">/</span>
																			{/if}
																		{/each}
																	</div>
																	{#if truncatedCategoryPaths[categoryPath.path]}
																		<span class="category-path-ellipsis" title={categoryPath.path}
																			>…</span
																		>
																	{/if}
																	{#if categoryPath.confidenceLabel}
																		<span class="category-path-confidence"
																			>{categoryPath.confidenceLabel}</span
																		>
																	{/if}
																</div>
															</div>
														{/each}
													</div>
												{:else}
													<p class="topic-sidebar-copy muted">No category paths assigned.</p>
												{/if}
											</div>
											{@const catPathsEn = selectedTopic.categoryPathsEn ?? []}
											{#if catPathsEn.length > 0 && !arraysEqual(selectedTopic.categoryPaths ?? [], catPathsEn)}
												<div class="topic-sidebar-block">
													<div class="topic-sidebar-label">Category Paths (English)</div>
													<div class="category-path-list">
														{#each catPathsEn as enPath, idx (`en-cat-${enPath}-${idx}`)}
															{@const enSegments = enPath.split('/').filter(Boolean)}
															<div class="category-path-row">
																<div class="category-path-line">
																	<div class="category-path-segments">
																		{#each enSegments as seg, segIdx (`en-seg-${seg}-${segIdx}`)}
																			<span class="keyword-chip category-path-chip">{seg}</span>
																			{#if segIdx < enSegments.length - 1}
																				<span class="category-path-separator">/</span>
																			{/if}
																		{/each}
																	</div>
																</div>
															</div>
														{/each}
													</div>
												</div>
											{/if}
											<div class="topic-sidebar-block">
												<div class="topic-sidebar-label">Keywords</div>
												{#if selectedTopic.topicKeywords.length > 0}
													<div class="keyword-list">
														{#each selectedTopic.topicKeywords as kw, idx (`${kw}-${idx}`)}
															<span class="keyword-chip">{kw}</span>
														{/each}
													</div>
												{:else}
													<p class="topic-sidebar-copy muted">No keywords extracted.</p>
												{/if}
											</div>
											{@const kwEn = selectedTopic.topicKeywordsEn ?? []}
											{#if kwEn.length > 0 && !arraysEqual(selectedTopic.topicKeywords, kwEn)}
												<div class="topic-sidebar-block">
													<div class="topic-sidebar-label">Keywords (English)</div>
													<div class="keyword-list">
														{#each kwEn as kw, idx (`en-kw-${kw}-${idx}`)}
															<span class="keyword-chip keyword-chip-en">{kw}</span>
														{/each}
													</div>
												</div>
											{/if}
										{/if}
									{:else}
										<div class="topic-sidebar-empty">
											Select a {itemSingular} to inspect it alongside the source PDF.
										</div>
									{/if}
								{/snippet}
							</PdfViewWindow>
						{:else if viewerInputId && treeState.selectedPdfTarget}
							<iframe class="pdf-fallback-frame" title={activeRecord.fileName} src={viewerFileUrl}
							></iframe>
						{:else if topicLoadingByRecordId[activeRecord.id]}
							<div class="empty-state pdf-empty">Loading {itemPlural} for the selected record…</div>
						{:else if activeRecord.topics.length === 0}
							<div class="empty-state pdf-empty">
								No {itemPlural} are available for this record yet.
							</div>
						{:else}
							<div class="empty-state pdf-empty">
								Select a {itemSingular} to move the PDF display to the relevant page.
							</div>
						{/if}
					</div>
				</div>
			{:else}
				<div class="empty-state">
					Select a record from the left panel to inspect its {itemPlural}.
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.tree-shell {
		position: relative;
		display: flex;
		height: 100%;
		flex-direction: column;
		color: var(--text);
	}

	.tree-shell.resizing {
		cursor: col-resize;
		user-select: none;
	}

	.error-overlay {
		position: absolute;
		inset: 0;
		z-index: 24;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1.5rem;
		background: rgba(2, 6, 23, 0.62);
		backdrop-filter: blur(10px);
	}

	.error-dialog {
		width: min(540px, 100%);
		border-radius: 22px;
		border: 1px solid rgba(248, 113, 113, 0.28);
		background: var(--panel);
		padding: 1.25rem;
		box-shadow: 0 30px 80px rgba(15, 23, 42, 0.5);
	}

	.dialog-copy {
		margin-top: 0.55rem;
		color: var(--muted);
	}

	.dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.hero {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
		padding: 1.1rem 1.2rem;
		border-radius: 24px;
		background:
			radial-gradient(circle at top right, color-mix(in srgb, var(--accent) 14%, transparent), transparent 42%),
			linear-gradient(180deg, color-mix(in srgb, var(--panel-alt) 86%, transparent), color-mix(in srgb, var(--panel-alt) 66%, transparent));
		border: 1px solid var(--border);
	}

	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}
	h2,
	h3,
	p {
		margin: 0;
	}
	h2 {
		margin-top: 0.3rem;
		font-size: 1.5rem;
	}
	.hero p {
		margin-top: 0.45rem;
		max-width: 52rem;
		color: var(--muted);
	}

	.hero-actions {
		display: flex;
		gap: 0.75rem;
	}

	.ghost {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		border-radius: 14px;
		padding: 0.8rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.16);
		cursor: pointer;
	}

	.ghost {
		background: color-mix(in srgb, var(--panel-alt) 36%, transparent);
		color: var(--text);
	}

	.workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: auto minmax(0, 1fr);
		grid-template-rows: minmax(0, 1fr);
		gap: 1rem;
	}

	.right-panel {
		min-height: 0;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
		display: flex;
		flex-direction: column;
		padding: 1rem;
	}

	.snippet-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.detail-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin: 0.75rem 0;
	}
	.detail-meta span {
		border-radius: 999px;
		padding: 0.25rem 0.55rem;
		background: rgba(148, 163, 184, 0.12);
		font-size: 0.72rem;
		color: var(--muted);
	}

	.right-tabs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
		margin-bottom: 1rem;
	}

	.tab {
		border-radius: 14px;
		padding: 0.72rem 0.9rem;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: color-mix(in srgb, var(--panel-alt) 30%, transparent);
		font-weight: 700;
	}
	.tab.active {
		border-color: color-mix(in srgb, var(--accent) 36%, transparent);
		background: color-mix(in srgb, var(--accent) 16%, transparent);
		color: var(--accent);
	}
	.tab.passive {
		color: var(--muted);
	}

	.detail-grid {
		display: flex;
		min-height: 0;
		flex: 1;
		align-items: stretch;
		gap: 0;
	}
	.detail-card {
		flex: 0 0 auto;
		overflow: hidden;
	}
	.pdf-card {
		flex: 1 1 0;
		min-width: 0;
		display: flex;
		flex-direction: column;
		min-height: 0;
		overflow: hidden;
	}
	.resize-handle {
		flex: 0 0 16px;
		position: relative;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		outline: none;
		user-select: none;
		align-self: stretch;
	}
	.resize-handle::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 7px;
		width: 1px;
		background: var(--ink-line);
		opacity: 0.8;
	}
	.resize-grip {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 8px;
		height: 52px;
		transform: translate(-50%, -50%);
		border-radius: 999px;
		background:
			radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y,
			var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
	}
	.resize-handle.active .resize-grip,
	.resize-handle:hover .resize-grip,
	.resize-handle:focus-visible .resize-grip {
		border-color: var(--accent);
	}
	.detail-card,
	.pdf-card {
		display: flex;
		flex-direction: column;
		min-height: 0;
		border-radius: 20px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: color-mix(in srgb, var(--panel-alt) 26%, transparent);
		padding: 1rem;
	}

	.topic-snippets {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		gap: 0.75rem;
		margin-top: 1rem;
		overflow: auto;
		padding-right: 0.25rem;
		scrollbar-width: thin;
		scrollbar-color: rgba(148, 163, 184, 0.25) transparent;
	}
	.topic-snippets::-webkit-scrollbar {
		width: 6px;
	}
	.topic-snippets::-webkit-scrollbar-thumb {
		background: rgba(148, 163, 184, 0.25);
		border-radius: 999px;
	}
	.topic-snippets::-webkit-scrollbar-track {
		background: transparent;
	}

	.snippet {
		position: relative;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: color-mix(in srgb, var(--panel-alt) 34%, transparent);
		padding: 0.85rem;
		text-align: left;
		color: inherit;
		transition:
			border-color 150ms ease,
			background 150ms ease,
			box-shadow 150ms ease,
			transform 150ms ease;
	}
	.snippet:hover {
		border-color: color-mix(in srgb, var(--accent) 34%, transparent);
	}
	.snippet.selected {
		border-color: color-mix(in srgb, var(--crimson) 58%, transparent);
		background:
			linear-gradient(180deg, color-mix(in srgb, var(--crimson) 14%, transparent), color-mix(in srgb, var(--crimson) 6%, transparent)),
			color-mix(in srgb, var(--panel-alt) 90%, transparent);
		box-shadow:
			0 0 0 1px color-mix(in srgb, var(--crimson) 18%, transparent) inset,
			0 16px 36px rgba(15, 23, 42, 0.24);
		transform: translateY(-1px);
	}
	.snippet p {
		margin-top: 0.5rem;
		font-size: 0.84rem;
		color: var(--muted);
		line-height: 1.5;
	}
	.snippet.selected p {
		color: var(--text);
	}

	.keyword-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
	}
	.keyword {
		border-radius: 999px;
		padding: 0.18rem 0.48rem;
		background: color-mix(in srgb, var(--accent) 14%, transparent);
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--accent);
	}
	.snippet-id {
		border-radius: 999px;
		padding: 0.18rem 0.48rem;
		background: rgba(148, 163, 184, 0.1);
		font-size: 0.7rem;
		font-weight: 700;
		color: var(--muted);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.02em;
	}
	.snippet-id-group {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.3rem;
	}
	.snippet-type {
		border-radius: 999px;
		padding: 0.18rem 0.48rem;
		background: color-mix(in srgb, var(--accent) 14%, transparent);
		font-size: 0.68rem;
		font-weight: 600;
		color: var(--accent);
		letter-spacing: 0.03em;
	}
	.snippet-page {
		flex: none;
		font-size: 0.76rem;
		font-weight: 600;
		color: var(--muted);
		white-space: nowrap;
	}
	.snippet-kw-row {
		margin-top: 0.4rem;
	}
	.snippet-info-row {
		display: flex;
		align-items: baseline;
		gap: 0.4rem;
		margin-top: 0.35rem;
	}
	.snippet-info-label {
		flex: none;
		font-size: 0.66rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
		opacity: 0.7;
	}
	.snippet-info-val {
		font-size: 0.74rem;
		color: var(--muted);
		overflow-wrap: anywhere;
		line-height: 1.4;
	}
	.snippet-cat-val {
		font-style: italic;
	}

	.empty-state {
		display: flex;
		flex: 1;
		align-items: center;
		justify-content: center;
		border-radius: 18px;
		border: 1px dashed rgba(148, 163, 184, 0.2);
		background: color-mix(in srgb, var(--panel-alt) 28%, transparent);
		padding: 1rem;
		text-align: center;
		color: var(--muted);
	}

	.pdf-card :global(.doc-page-bar) {
		margin-top: 0.85rem;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 16px 16px 0 0;
	}
	.pdf-card :global(.pdf-stage) {
		margin-top: -1px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 0 0 16px 16px;
		background: color-mix(in srgb, var(--panel-alt) 28%, transparent);
	}

	.topic-sidebar-block + .topic-sidebar-block {
		margin-top: 1rem;
		padding-top: 1rem;
		border-top: 1px solid rgba(148, 163, 184, 0.1);
	}
	.topic-sidebar-row {
		display: grid;
		grid-template-columns: 76px minmax(0, 1fr);
		gap: 0.6rem;
		margin-bottom: 0.6rem;
		align-items: start;
	}
	.topic-sidebar-row span {
		font-size: 0.76rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--muted);
	}
	.topic-sidebar-row strong,
	.topic-sidebar-copy {
		min-width: 0;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.topic-sidebar-copy {
		margin-top: 0.55rem;
		font-size: 0.88rem;
		line-height: 1.6;
		color: var(--text);
	}
	.topic-sidebar-copy.topic-desc-en {
		color: var(--muted);
		font-style: italic;
	}
	.topic-sidebar-copy.muted,
	.topic-sidebar-empty {
		color: var(--muted);
	}
	.topic-sidebar-empty {
		font-size: 0.9rem;
		line-height: 1.6;
	}

	.keyword-list {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin-top: 0.6rem;
	}
	.keyword-chip {
		display: inline-flex;
		align-items: center;
		padding: 0.32rem 0.58rem;
		border-radius: 999px;
		border: 1px solid color-mix(in srgb, var(--accent) 22%, transparent);
		background: color-mix(in srgb, var(--accent) 12%, transparent);
		font-size: 0.78rem;
		color: var(--accent);
	}
	.category-path-list {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		margin-top: 0.6rem;
	}
	.category-path-row {
		min-width: 0;
	}
	.category-path-line {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		min-width: 0;
	}
	.category-path-segments {
		flex: 1 1 0;
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
	}
	.category-path-chip {
		vertical-align: middle;
	}
	.category-path-separator {
		display: inline-block;
		margin: 0 0.28rem;
		color: var(--muted);
		vertical-align: middle;
	}
	.category-path-ellipsis {
		flex: none;
		cursor: help;
		font-size: 0.82rem;
		font-weight: 700;
		color: var(--muted);
		user-select: none;
		letter-spacing: 0.04em;
	}
	.category-path-confidence {
		flex: none;
		font-size: 0.76rem;
		font-weight: 700;
		color: var(--muted);
	}

	.topic-type-display {
		display: flex;
		align-items: baseline;
		gap: 0.35rem;
		flex-wrap: wrap;
	}
	.topic-type-en {
		font-size: 0.74rem;
		color: var(--muted);
		font-style: italic;
		font-weight: 400;
	}
	.keyword-chip-en {
		border-color: rgba(148, 163, 184, 0.2);
		background: rgba(148, 163, 184, 0.08);
		color: var(--muted);
		font-style: italic;
	}

	.topic-sidebar-label-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.topic-edit-trigger {
		flex: none;
		font-size: 0.7rem;
		font-weight: 700;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--accent);
		border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
		border-radius: 8px;
		padding: 0.2rem 0.5rem;
		background: color-mix(in srgb, var(--accent) 8%, transparent);
		cursor: pointer;
		transition:
			background 150ms ease,
			border-color 150ms ease;
	}
	.topic-edit-trigger:hover {
		background: color-mix(in srgb, var(--accent) 16%, transparent);
		border-color: color-mix(in srgb, var(--accent) 50%, transparent);
	}

	.topic-sidebar-edit {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
		padding-bottom: 0.5rem;
	}
	.topic-edit-field {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	.topic-edit-hint {
		margin: 0;
		font-size: 0.68rem;
		color: var(--muted);
		opacity: 0.7;
	}
	.topic-edit-input,
	.topic-edit-textarea {
		width: 100%;
		box-sizing: border-box;
		background: color-mix(in srgb, var(--panel-alt) 60%, transparent);
		border: 1px solid rgba(148, 163, 184, 0.22);
		border-radius: 10px;
		padding: 0.5rem 0.65rem;
		color: var(--text);
		font-size: 0.84rem;
		line-height: 1.5;
		font-family: inherit;
		resize: vertical;
		transition: border-color 150ms ease;
	}
	.topic-edit-input:focus,
	.topic-edit-textarea:focus {
		outline: none;
		border-color: color-mix(in srgb, var(--accent) 50%, transparent);
	}
	.topic-edit-textarea {
		min-height: 72px;
	}
	.topic-edit-error {
		margin: 0;
		font-size: 0.8rem;
		color: #f87171;
		line-height: 1.4;
	}
	.topic-edit-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.25rem;
	}
	.topic-edit-btn {
		flex: 1;
		border-radius: 10px;
		padding: 0.55rem 0.75rem;
		font-size: 0.82rem;
		font-weight: 700;
		cursor: pointer;
		border: 1px solid;
		transition:
			opacity 150ms ease,
			background 150ms ease;
	}
	.topic-edit-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.topic-edit-btn-save {
		background: color-mix(in srgb, var(--accent) 20%, transparent);
		border-color: color-mix(in srgb, var(--accent) 45%, transparent);
		color: var(--accent);
	}
	.topic-edit-btn-save:not(:disabled):hover {
		background: color-mix(in srgb, var(--accent) 30%, transparent);
	}
	.topic-edit-btn-cancel {
		background: rgba(148, 163, 184, 0.08);
		border-color: rgba(148, 163, 184, 0.2);
		color: var(--muted);
	}
	.topic-edit-btn-cancel:not(:disabled):hover {
		background: rgba(148, 163, 184, 0.14);
	}

	.pdf-fallback-frame {
		margin-top: 0.85rem;
		width: 100%;
		min-height: 540px;
		flex: 1;
		border: 1px solid rgba(148, 163, 184, 0.14);
		border-radius: 16px;
		background: white;
	}

	:global(.pdf-highlight) {
		position: absolute;
		background: color-mix(in srgb, var(--accent) 25%, transparent);
		outline: 1px solid color-mix(in srgb, var(--accent) 80%, transparent);
		border-radius: 2px;
	}

	@media (max-width: 980px) {
		.workspace {
			grid-template-columns: minmax(0, 1fr);
		}
		.detail-grid {
			flex-direction: column;
		}
		.detail-card {
			width: 100% !important;
			flex: none !important;
		}
		.resize-handle {
			display: none;
		}
	}
</style>
