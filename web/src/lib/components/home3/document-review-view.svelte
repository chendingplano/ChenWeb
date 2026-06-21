<script lang="ts">
    import { onMount } from 'svelte';
    import { listAspects, listTiers, submitRequest } from '$lib/services/docReviewService';
    import type { AspectInfo, TierInfo, FindingItem, ReferenceDoc } from '$lib/services/docReviewService';
    import { uploadKbInputs, listKnowledgeStores } from '$lib/services/kbService';
    import type { KbInputRecord, KnowledgeStoreRecord } from '$lib/services/kbService';
    import { knowledgeStoreState } from './knowledge-store-state.svelte';
    import KbInputSearchDialog from './kb-input-search-dialog.svelte';
    import DocReviewResultsView from './doc-review-results-view.svelte';
    import { appAuthStore } from '@chendingplano/shared';
    import SearchIcon from '@lucide/svelte/icons/search';
    import CheckIcon from '@lucide/svelte/icons/check';
    import XIcon from '@lucide/svelte/icons/x';
    import LoaderIcon from '@lucide/svelte/icons/loader';
    import UploadIcon from '@lucide/svelte/icons/upload';
    import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
    import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';

    let { darkMode = true }: { darkMode: boolean } = $props();

    // Design tokens
    let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
    let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
    let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
    let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
    let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
    let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
    let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
    let inputBg = $derived(darkMode ? '#0F1729' : '#F8FAFC');
    let successBg = $derived(darkMode ? 'rgba(34,197,94,0.15)' : 'rgba(34,197,94,0.10)');
    let errorBg = $derived(darkMode ? 'rgba(239,68,68,0.15)' : 'rgba(239,68,68,0.10)');

    // State
    let aspects = $state<AspectInfo[]>([]);
    let tiers = $state<TierInfo[]>([]);
    let currentStep = $state(1);
    let selectedDocId = $state<number | null>(null);
    let selectedDocTitle = $state('');
    let selectedTier = $state('must_review');
    let customAspects = $state<Set<string>>(new Set());
    let expandedGroups = $state<Set<string>>(new Set(['P1', 'P3', 'P5']));
    let requesterName = $state('');
    let notes = $state('');
    let referenceDocs = $state<ReferenceDoc[]>([]);
    let submittedRequestId = $state<number | null>(null);
    let submittedReportId = $state<number | null>(null);
    let submitError = $state('');
    let isSubmitting = $state(false);
    let reportTemplate = $state('');
    let docTemplate = $state('');

    // Step 1 input mode: pick an existing document or upload a new one
    let inputMode = $state<'search' | 'upload'>('search');
    let searchDialogOpen = $state(false);
    let uploadFile = $state<File | null>(null);
    const uploadParserOptions = ['paddleocr', 'opendata', 'mineru', 'docling'] as const;
    let uploadParser = $state<(typeof uploadParserOptions)[number]>('opendata');
    let isUploading = $state(false);
    let uploadError = $state('');
    let filePicker = $state<HTMLInputElement | null>(null);

    // Knowledge-store picker, shown when the user wants to upload but no store is active.
    let storeOptions = $state<KnowledgeStoreRecord[]>([]);
    let loadingStores = $state(false);
    let storeError = $state('');

    async function loadStoreOptions() {
        loadingStores = true;
        storeError = '';
        try {
            const res = await listKnowledgeStores();
            storeOptions = res.results ?? [];
        } catch (err) {
            storeError = err instanceof Error ? err.message : 'Failed to load knowledge stores.';
        } finally {
            loadingStores = false;
        }
    }

    function pickStore(store: KnowledgeStoreRecord) {
        knowledgeStoreState.setActiveStore(store);
        uploadError = '';
    }

    function changeStore() {
        knowledgeStoreState.setActiveStore(null);
        if (storeOptions.length === 0) loadStoreOptions();
    }

    const typeExtensions: Record<string, string[]> = {
        pdf: ['.pdf'], doc: ['.doc', '.docx'], excel: ['.xls', '.xlsx'],
        ppt: ['.ppt', '.pptx'], text: ['.txt'], json: ['.json'],
        xml: ['.xml'], markdown: ['.md', '.markdown'], typst: ['.typ'], zip: ['.zip'],
    };

    function typeFromExtension(filename: string): string {
        const lower = filename.toLowerCase();
        for (const [type, exts] of Object.entries(typeExtensions)) {
            if (exts.some(e => lower.endsWith(e))) return type;
        }
        return '';
    }

    // Derived: aspects grouped by group
    let aspectsByGroup = $derived.by(() => {
        const map: Record<string, AspectInfo[]> = {};
        for (const a of aspects) {
            if (!map[a.group]) map[a.group] = [];
            map[a.group].push(a);
        }
        return map;
    });

    // Derived: which aspects are selected based on tier (and any per-item edits)
    let effectiveAspects = $derived.by(() => {
        if (selectedTier === 'custom') {
            return [...customAspects];
        }
        const sel = tierSelections[selectedTier];
        if (sel) return [...sel];
        const tier = tiers.find(t => t.key === selectedTier);
        return tier?.aspect_names || [];
    });

    // Derived: group labels
    const groupLabels: Record<string, string> = {
        P1: 'Language & Style', P2: 'Structure & Organization',
        P3: 'Content Quality', P4: 'Consistency',
        P5: 'Technical & Compliance', P6: 'Meta & Process',
    };

    // Lookup: aspect name -> AspectInfo, for resolving a tier's aspect labels
    let aspectByName = $derived.by(() => {
        const map: Record<string, AspectInfo> = {};
        for (const a of aspects) map[a.name] = a;
        return map;
    });

    // Group a tier's aspect names by category for the foldable preview
    function groupAspectNames(names: string[]): Array<{ group: string; label: string; items: Array<{ name: string; label: string }> }> {
        const byGroup: Record<string, Array<{ name: string; label: string }>> = {};
        for (const name of names) {
            const info = aspectByName[name];
            const group = info?.group ?? '其他';
            if (!byGroup[group]) byGroup[group] = [];
            byGroup[group].push({ name, label: info?.label ?? name });
        }
        return Object.entries(byGroup)
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([group, items]) => ({ group, label: groupLabels[group] ?? group, items }));
    }

    // Which tier cards have their aspect list expanded
    let expandedTiers = $state<Set<string>>(new Set());
    function toggleTierAspects(key: string) {
        const next = new Set(expandedTiers);
        if (next.has(key)) next.delete(key); else next.add(key);
        expandedTiers = next;
    }

    // Per-tier mutable selection of aspect items. Initialized from each tier's
    // configured aspect_names; the user can toggle individual items or all.
    let tierSelections = $state<Record<string, Set<string>>>({});

    function initTierSelections() {
        const next: Record<string, Set<string>> = {};
        for (const t of tiers) next[t.key] = new Set(t.aspect_names);
        tierSelections = next;
    }

    function isAspectChecked(tierKey: string, name: string): boolean {
        return tierSelections[tierKey]?.has(name) ?? false;
    }

    function tierSelectedCount(tierKey: string): number {
        return tierSelections[tierKey]?.size ?? 0;
    }

    function groupSelectedCount(tierKey: string, items: Array<{ name: string }>): number {
        const sel = tierSelections[tierKey];
        if (!sel) return 0;
        return items.reduce((n, it) => n + (sel.has(it.name) ? 1 : 0), 0);
    }

    // Toggling an item (or select-all) also marks that tier as the chosen one,
    // so the customized selection is what gets submitted.
    function toggleAspectInTier(tierKey: string, name: string) {
        const next = new Set(tierSelections[tierKey] ?? []);
        if (next.has(name)) next.delete(name); else next.add(name);
        tierSelections = { ...tierSelections, [tierKey]: next };
        selectedTier = tierKey;
    }

    function allAspectsChecked(tier: TierInfo): boolean {
        const sel = tierSelections[tier.key];
        return !!sel && tier.aspect_names.length > 0 && sel.size === tier.aspect_names.length;
    }

    function toggleAllInTier(tier: TierInfo) {
        const next = allAspectsChecked(tier) ? new Set<string>() : new Set(tier.aspect_names);
        tierSelections = { ...tierSelections, [tier.key]: next };
        selectedTier = tier.key;
    }

    onMount(async () => {
        try {
            aspects = await listAspects();
            tiers = await listTiers();
            initTierSelections();
            // Default: auto-select must_review aspects for custom mode
            const mustTier = tiers.find(t => t.key === 'must_review');
            if (mustTier) mustTier.aspect_names.forEach(a => customAspects.add(a));
        } catch (e) {
            submitError = 'Failed to load aspects';
        }

        // Auto-fill requester name from auth if available
        const user = appAuthStore.getUser();
        if (user?.name) {
            requesterName = user.name;
        }
    });

    function selectDoc(doc: {id: number; title: string}) {
        selectedDocId = doc.id;
        selectedDocTitle = doc.title;
        currentStep = 2;
    }

    function onSearchSelect(records: KbInputRecord[]) {
        const record = records[0];
        if (!record) return;
        const title = record.title?.trim() || record.file_name?.trim() || `Document ${record.id}`;
        selectDoc({ id: record.id, title });
    }

    function switchMode(mode: 'search' | 'upload') {
        inputMode = mode;
        uploadError = '';
        if (mode === 'upload' && !knowledgeStoreState.activeStore && storeOptions.length === 0) {
            loadStoreOptions();
        }
    }

    function triggerFilePicker() {
        uploadError = '';
        filePicker?.click();
    }

    function onUploadFileSelect(e: Event) {
        const files = (e.target as HTMLInputElement).files;
        uploadFile = files && files.length > 0 ? files[0] : null;
        uploadError = '';
    }

    async function handleUpload() {
        if (!uploadFile) { uploadError = 'Pick a file to upload.'; return; }
        const type = typeFromExtension(uploadFile.name);
        if (!type) { uploadError = 'Unsupported file type.'; return; }
        const activeStore = knowledgeStoreState.activeStore;
        if (!activeStore?.tenant_id?.trim()) {
            uploadError = 'Select an active knowledge store before uploading.';
            return;
        }

        isUploading = true;
        uploadError = '';
        try {
            const result = await uploadKbInputs({
                type,
                title: uploadFile.name,
                parser_name: uploadParser,
                ks_store_id: activeStore.id,
                tenant_id: activeStore.tenant_id,
                files: [uploadFile],
            });
            const newId = result.ids?.[0];
            if (!newId) throw new Error('Upload succeeded but no record id was returned.');
            selectDoc({ id: newId, title: uploadFile.name });
        } catch (err) {
            uploadError = err instanceof Error ? err.message : 'Failed to upload file.';
        } finally {
            isUploading = false;
        }
    }

    function toggleAspect(name: string) {
        const next = new Set(customAspects);
        if (next.has(name)) next.delete(name); else next.add(name);
        customAspects = next;
    }

    function toggleGroup(group: string) {
        const next = new Set(expandedGroups);
        if (next.has(group)) next.delete(group); else next.add(group);
        expandedGroups = next;
    }

    async function refDocSearch() {
        const q = (document.getElementById('ref-doc-input') as HTMLInputElement)?.value;
        if (!q || q.length < 2) return;
        try {
            const res = await fetch(`/api/v1/kb/inputs?query=${encodeURIComponent(q)}&limit=5`, { credentials: 'same-origin' });
            const data = await res.json();
            const results = (data.inputs || data.records || data.data || []).slice(0, 5);
            for (const r of results) {
                if (!referenceDocs.some(d => d.record_id === r.id)) {
                    referenceDocs = [...referenceDocs, { record_id: r.id, doc_no: r.doc_no || '', title: r.title || r.file_name || `Document ${r.id}` }];
                }
            }
        } catch {}
    }

    async function handleSubmit() {
        if (!selectedDocId) { submitError = 'Please select a document'; return; }
        if (effectiveAspects.length === 0) { submitError = 'Select at least one aspect to review'; currentStep = 2; return; }
        if (!requesterName.trim()) { submitError = 'Please enter your name'; currentStep = 5; return; }

        isSubmitting = true;
        submitError = '';
        try {
            const result = await submitRequest({
                input_record_id: selectedDocId,
                tier: selectedTier,
                aspects: effectiveAspects,
                reference_docs: referenceDocs.length > 0 ? referenceDocs : undefined,
                notes: notes || undefined,
                requester_name: requesterName,
                requester_id: 0, // TODO: resolve from auth context
                report_template: reportTemplate || undefined,
                doc_template: docTemplate || undefined,
            });
            submittedRequestId = result.request_id;
            submittedReportId = result.report_id || null;
        } catch (e: any) {
            submitError = e.message || 'Submission failed';
        } finally {
            isSubmitting = false;
        }
    }
</script>

{#if submittedRequestId}
    <DocReviewResultsView {darkMode} requestId={submittedRequestId} reportId={submittedReportId ?? undefined} />
{:else}
    <div style="padding: 1.5rem; color: {textPrimary};">
        <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 0.5rem;">Document Review</h1>
        <p style="color: {textSecondary}; margin-bottom: 2rem;">
            Submit a document for AI-powered review across quality, compliance, and technical aspects.
        </p>

        <!-- Step indicators -->
        <div style="display: flex; gap: 0.5rem; margin-bottom: 2rem; font-size: 0.8rem;">
            {#each ['Select Document', 'Check Level', 'Customize', 'References', 'Submit'] as step, i}
                <div style="display: flex; align-items: center; gap: 0.25rem;">
                    <div style="width: 24px; height: 24px; border-radius: 50%; display: flex; align-items: center; justify-content: center;
                        background: {i + 1 <= currentStep ? accent : borderColor}; color: {i + 1 <= currentStep ? '#fff' : textMuted};
                        font-size: 0.75rem; font-weight: 600;">
                        {i + 1 <= currentStep ? '✓' : i + 1}
                    </div>
                    <span style="color: {i + 1 === currentStep ? accent : textMuted}; white-space: nowrap;">{step}</span>
                </div>
            {/each}
        </div>

        <!-- Step 1: Document Search -->
        {#if currentStep === 1}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem;">Step 1: Select Document</h2>

                <!-- Mode toggle: search the library or upload a new file -->
                <div style="display: inline-flex; gap: 0.25rem; padding: 0.25rem; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; margin-bottom: 1rem;">
                    {#each [['search', 'Search Library'], ['upload', 'Upload File']] as [mode, label]}
                        <button onclick={() => switchMode(mode as 'search' | 'upload')}
                            style="display: flex; align-items: center; gap: 0.4rem; padding: 0.4rem 0.9rem; border: none; border-radius: 6px; cursor: pointer; font-size: 0.85rem; font-weight: 600;
                            background: {inputMode === mode ? accent : 'transparent'}; color: {inputMode === mode ? '#fff' : textSecondary};">
                            {#if mode === 'search'}<SearchIcon size={14} />{:else}<UploadIcon size={14} />{/if}
                            {label}
                        </button>
                    {/each}
                </div>

                {#if inputMode === 'search'}
                    <button onclick={() => searchDialogOpen = true} type="button"
                        style="display: flex; align-items: center; gap: 0.5rem; width: 100%; padding: 0.65rem 0.85rem; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; color: {textSecondary}; font-size: 0.9rem; text-align: left;">
                        <SearchIcon size={16} style="color: {textMuted};" />
                        Search the document library…
                    </button>
                {:else if !knowledgeStoreState.activeStore}
                    <!-- No active knowledge store: prompt the user to pick one before uploading -->
                    <div style="display: flex; flex-direction: column; gap: 0.75rem;">
                        <div style="padding: 0.75rem; background: {accentTint}; border: 1px solid {borderColor}; border-radius: 8px; font-size: 0.85rem; color: {textSecondary};">
                            No active knowledge store selected. Pick one below to upload the document into.
                        </div>
                        {#if loadingStores}
                            <div style="display: flex; align-items: center; gap: 0.5rem; color: {textMuted}; font-size: 0.85rem;">
                                <LoaderIcon size={14} style="animation: spin 1s linear infinite;" /> Loading knowledge stores…
                            </div>
                        {:else if storeOptions.length === 0}
                            <div style="font-size: 0.85rem; color: {textSecondary};">
                                No knowledge stores found. Create one under Knowledge → Knowledge Stores first.
                            </div>
                        {:else}
                            <div style="display: flex; flex-wrap: wrap; gap: 0.5rem;">
                                {#each storeOptions as store}
                                    <button onclick={() => pickStore(store)} type="button"
                                        style="display: flex; flex-direction: column; align-items: flex-start; gap: 0.2rem; padding: 0.65rem 0.85rem; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; min-width: 160px; text-align: left;">
                                        <span style="color: {textPrimary}; font-weight: 600; font-size: 0.9rem;">{store.ks_name}</span>
                                        <span style="color: {textMuted}; font-size: 0.75rem;">ID {store.id}{store.ks_desc ? ` · ${store.ks_desc}` : ''}</span>
                                    </button>
                                {/each}
                            </div>
                        {/if}
                        {#if storeError}
                            <div style="font-size: 0.85rem; color: #ef4444;">{storeError}</div>
                        {/if}
                        <button onclick={loadStoreOptions} type="button"
                            style="align-self: flex-start; padding: 0.35rem 0.85rem; background: transparent; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; color: {textSecondary}; font-size: 0.8rem;">
                            Refresh
                        </button>
                    </div>
                {:else}
                    <!-- Upload a new document to review -->
                    <div style="display: flex; flex-direction: column; gap: 0.75rem;">
                        <div style="font-size: 0.8rem; color: {textMuted};">
                            Uploads land in knowledge store: <span style="color: {textSecondary};">{knowledgeStoreState.activeStore.ks_name}</span>
                            <button onclick={changeStore} type="button"
                                style="margin-left: 0.5rem; background: none; border: none; color: {accent}; cursor: pointer; font-size: 0.8rem; text-decoration: underline;">change</button>
                        </div>
                        <input bind:this={filePicker} type="file" onchange={onUploadFileSelect}
                            style="display: none;" />
                        <button onclick={triggerFilePicker} type="button"
                            style="display: flex; align-items: center; justify-content: center; gap: 0.5rem; padding: 1.25rem; background: {inputBg}; border: 1px dashed {borderColor}; border-radius: 8px; cursor: pointer; color: {textSecondary}; font-size: 0.9rem;">
                            <UploadIcon size={18} />
                            {uploadFile ? uploadFile.name : 'Browse and pick a file…'}
                        </button>
                        <div style="display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap;">
                            <label style="display: flex; align-items: center; gap: 0.5rem; font-size: 0.85rem; color: {textSecondary};">
                                Parser
                                <select bind:value={uploadParser}
                                    style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 6px; padding: 0.35rem 0.5rem; color: {textPrimary}; font-size: 0.85rem;">
                                    {#each uploadParserOptions as opt}
                                        <option value={opt}>{opt}</option>
                                    {/each}
                                </select>
                            </label>
                            <button onclick={handleUpload}
                                disabled={!uploadFile || isUploading}
                                style="display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 1.25rem; border: none; border-radius: 8px; cursor: {!uploadFile || isUploading ? 'not-allowed' : 'pointer'}; font-size: 0.9rem; font-weight: 600;
                                background: {!uploadFile || isUploading ? borderColor : accent}; color: {!uploadFile || isUploading ? textMuted : '#fff'};">
                                {#if isUploading}
                                    <LoaderIcon size={14} style="animation: spin 1s linear infinite;" />
                                    Uploading…
                                {:else}
                                    Upload & Select
                                {/if}
                            </button>
                        </div>
                        {#if uploadError}
                            <div style="font-size: 0.85rem; color: #ef4444;">{uploadError}</div>
                        {/if}
                    </div>
                {/if}
                {#if selectedDocTitle}
                    <div style="margin-top: 1rem; padding: 0.75rem; background: {accentTint}; border-radius: 8px; display: flex; align-items: center; gap: 0.5rem;">
                        <CheckIcon size={16} style="color: {accent};" />
                        <span style="color: {accent};">Selected: {selectedDocTitle}</span>
                    </div>
                {/if}
            </div>
            <button onclick={() => selectedDocId && (currentStep = 2)}
                disabled={!selectedDocId}
                style="padding: 0.6rem 1.5rem; background: {selectedDocId ? accent : borderColor}; color: {selectedDocId ? '#fff' : textMuted}; border: none; border-radius: 8px; cursor: {selectedDocId ? 'pointer' : 'not-allowed'}; font-size: 0.9rem;">
                Next →
            </button>
        {/if}

        <!-- Step 2: Check Level -->
        {#if currentStep === 2}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem;">Step 2: Choose Check Level</h2>
                <div style="display: flex; flex-direction: column; gap: 0.75rem;">
                    {#each tiers as tier}
                        <div style="background: {inputBg}; border: 2px solid {selectedTier === tier.key ? accent : borderColor}; border-radius: 8px; overflow: hidden;">
                            <label style="display: flex; align-items: flex-start; gap: 0.75rem; padding: 1rem; cursor: pointer;">
                                <input type="radio" name="tier" value={tier.key}
                                    checked={selectedTier === tier.key}
                                    onchange={() => selectedTier = tier.key}
                                    style="margin-top: 0.2rem;" />
                                <div style="flex: 1;">
                                    <div style="font-weight: 600; color: {textPrimary};">{tier.label}</div>
                                    <div style="font-size: 0.85rem; color: {textSecondary};">{tier.description} — {tierSelectedCount(tier.key)} of {tier.aspect_names.length} aspects selected</div>
                                </div>
                            </label>
                            {#if tier.aspect_names.length > 0}
                                <button type="button" onclick={() => toggleTierAspects(tier.key)}
                                    style="display: flex; align-items: center; gap: 0.4rem; width: 100%; padding: 0.5rem 1rem; background: transparent; border: none; border-top: 1px solid {borderColor}; cursor: pointer; color: {textMuted}; font-size: 0.8rem; text-align: left;">
                                    {#if expandedTiers.has(tier.key)}
                                        <ChevronDownIcon size={14} />
                                    {:else}
                                        <ChevronRightIcon size={14} />
                                    {/if}
                                    {expandedTiers.has(tier.key) ? 'Hide' : 'View'} {tierSelectedCount(tier.key)}/{tier.aspect_names.length} selected aspects
                                </button>
                                {#if expandedTiers.has(tier.key)}
                                    <div style="padding: 0.25rem 1rem 0.85rem 2rem; display: flex; flex-direction: column; gap: 0.6rem;">
                                        <!-- Select / deselect all toggle -->
                                        <div style="display: flex; align-items: center; gap: 0.6rem;">
                                            <button type="button" onclick={() => toggleAllInTier(tier)}
                                                style="display: inline-flex; align-items: center; gap: 0.35rem; padding: 0.25rem 0.7rem; background: {inputBg}; border: 1px solid {accent}; border-radius: 6px; cursor: pointer; color: {accent}; font-size: 0.78rem; font-weight: 600;">
                                                {allAspectsChecked(tier) ? 'Deselect all' : 'Select all'}
                                            </button>
                                            <span style="font-size: 0.75rem; color: {textMuted};">{tierSelectedCount(tier.key)} of {tier.aspect_names.length} selected</span>
                                        </div>
                                        {#each groupAspectNames(tier.aspect_names) as cat}
                                            <div>
                                                <div style="font-size: 0.75rem; font-weight: 600; color: {textSecondary}; margin-bottom: 0.25rem;">
                                                    {cat.group} — {cat.label}
                                                    <span style="color: {textMuted}; font-weight: 400;">({groupSelectedCount(tier.key, cat.items)}/{cat.items.length})</span>
                                                </div>
                                                <div style="display: flex; flex-wrap: wrap; gap: 0.35rem;">
                                                    {#each cat.items as item}
                                                        {@const checked = isAspectChecked(tier.key, item.name)}
                                                        <button type="button" onclick={() => toggleAspectInTier(tier.key, item.name)}
                                                            title={checked ? 'Click to deselect' : 'Click to select'}
                                                            style="display: inline-flex; align-items: center; gap: 0.3rem; padding: 0.2rem 0.55rem; border-radius: 6px; cursor: pointer; font-size: 0.78rem;
                                                            background: {checked ? accentTint : 'transparent'}; border: 1px solid {checked ? accent : borderColor}; color: {checked ? textPrimary : textMuted};">
                                                            {#if checked}
                                                                <CheckIcon size={12} style="color: {accent};" />
                                                            {:else}
                                                                <span style="width: 12px; height: 12px; border: 1px solid {textMuted}; border-radius: 3px; display: inline-block;"></span>
                                                            {/if}
                                                            {item.label}
                                                        </button>
                                                    {/each}
                                                </div>
                                            </div>
                                        {/each}
                                    </div>
                                {/if}
                            {/if}
                        </div>
                    {/each}
                </div>
            </div>
            <div style="display: flex; gap: 0.5rem;">
                <button onclick={() => currentStep = 1}
                    style="padding: 0.6rem 1.5rem; background: transparent; color: {textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">← Back</button>
                <button onclick={() => currentStep = selectedTier === 'custom' ? 3 : 4}
                    style="padding: 0.6rem 1.5rem; background: {accent}; color: #fff; border: none; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">Next →</button>
            </div>
        {/if}

        <!-- Step 3: Customize Aspects -->
        {#if currentStep === 3}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem;">Step 3: Customize Aspects</h2>
                {#each Object.entries(aspectsByGroup) as [group, groupAspects]}
                    <div style="margin-bottom: 0.75rem;">
                        <button onclick={() => toggleGroup(group)}
                            style="display: flex; align-items: center; gap: 0.5rem; width: 100%; text-align: left; padding: 0.6rem 0.75rem; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; color: {textPrimary}; font-weight: 600; font-size: 0.9rem;">
                            {#if expandedGroups.has(group)}
                                <ChevronDownIcon size={16} />
                            {:else}
                                <ChevronRightIcon size={16} />
                            {/if}
                            {group} — {groupLabels[group] || group}
                            <span style="margin-left: auto; font-size: 0.8rem; color: {textMuted};">
                                {groupAspects.filter(a => customAspects.has(a.name)).length}/{groupAspects.length}
                            </span>
                        </button>
                        {#if expandedGroups.has(group)}
                            <div style="padding: 0.5rem 0 0 1.5rem; display: flex; flex-direction: column; gap: 0.25rem;">
                                {#each groupAspects as aspect}
                                    <label style="display: flex; align-items: center; gap: 0.5rem; padding: 0.3rem 0.5rem; border-radius: 4px; cursor: pointer; font-size: 0.85rem; color: {textSecondary};">
                                        <input type="checkbox" checked={customAspects.has(aspect.name)} onchange={() => toggleAspect(aspect.name)} />
                                        {aspect.label}
                                    </label>
                                {/each}
                            </div>
                        {/if}
                    </div>
                {/each}
            </div>
            <div style="display: flex; gap: 0.5rem;">
                <button onclick={() => currentStep = 2}
                    style="padding: 0.6rem 1.5rem; background: transparent; color: {textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">← Back</button>
                <button onclick={() => currentStep = 4}
                    disabled={customAspects.size === 0}
                    style="padding: 0.6rem 1.5rem; background: {customAspects.size > 0 ? accent : borderColor}; color: {customAspects.size > 0 ? '#fff' : textMuted}; border: none; border-radius: 8px; cursor: {customAspects.size > 0 ? 'pointer' : 'not-allowed'}; font-size: 0.9rem;">Next →</button>
            </div>
        {/if}

        <!-- Step 4: Supporting Documents -->
        {#if currentStep === 4}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem;">Step 4: Supporting Documents <span style="font-weight: 400; color: {textMuted};">(optional)</span></h2>
                <p style="color: {textSecondary}; font-size: 0.85rem; margin-bottom: 1rem;">
                    Add reference standards or supporting documents for compliance checking.
                </p>
                {#each referenceDocs as doc, i}
                    <div style="display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 0.75rem; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; margin-bottom: 0.5rem;">
                        <span style="flex: 1; color: {textPrimary}; font-size: 0.9rem;">{doc.title}</span>
                        <button onclick={() => referenceDocs = referenceDocs.filter((_, idx) => idx !== i)}
                            style="background: none; border: none; color: {textMuted}; cursor: pointer;">
                            <XIcon size={14} />
                        </button>
                    </div>
                {/each}
                <div style="display: flex; gap: 0.5rem;">
                    <input id="ref-doc-input" type="text" placeholder="Reference document title or ID"
                        style="flex: 1; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                    <button onclick={refDocSearch}
                        style="padding: 0.5rem 1rem; background: {accentTint}; color: {accent}; border: none; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">Add</button>
                </div>
            </div>
            <div style="display: flex; gap: 0.5rem;">
                <button onclick={() => currentStep = selectedTier === 'custom' ? 3 : 2}
                    style="padding: 0.6rem 1.5rem; background: transparent; color: {textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">← Back</button>
                <button onclick={() => currentStep = 5}
                    style="padding: 0.6rem 1.5rem; background: {accent}; color: #fff; border: none; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">Next →</button>
            </div>
        {/if}

        <!-- Step 5: Notes & Submit -->
        {#if currentStep === 5}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem;">Step 5: Review Details</h2>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Your Name *</label>
                    <input type="text" bind:value={requesterName} placeholder="Enter your name"
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                </div>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Notes (optional)</label>
                    <textarea bind:value={notes} placeholder="e.g., Focus on sterilization validation sections..."
                        rows={4}
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem; resize: vertical;"></textarea>
                </div>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Report Template (optional)</label>
                    <input type="text" bind:value={reportTemplate} placeholder="Template name or path"
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                </div>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Doc Template (optional)</label>
                    <input type="text" bind:value={docTemplate} placeholder="Template name or path"
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                </div>
            </div>

            <!-- Review Summary -->
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h3 style="font-size: 1rem; font-weight: 600; margin-bottom: 0.75rem;">Review Summary</h3>
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; font-size: 0.85rem;">
                    <div style="color: {textSecondary};">Document:</div>
                    <div style="color: {textPrimary};">{selectedDocTitle}</div>
                    <div style="color: {textSecondary};">Check Level:</div>
                    <div style="color: {textPrimary};">{tiers.find(t => t.key === selectedTier)?.label || selectedTier}</div>
                    <div style="color: {textSecondary};">Aspects:</div>
                    <div style="color: {textPrimary};">{effectiveAspects.length} selected</div>
                    <div style="color: {textSecondary};">Requester:</div>
                    <div style="color: {textPrimary};">{requesterName}</div>
                </div>
            </div>

            {#if submitError}
                <div style="padding: 0.75rem; background: {errorBg}; border: 1px solid rgba(239,68,68,0.3); border-radius: 8px; color: #ef4444; margin-bottom: 1rem; font-size: 0.9rem;">
                    {submitError}
                </div>
            {/if}

            <div style="display: flex; gap: 0.5rem;">
                <button onclick={() => currentStep = 4}
                    disabled={isSubmitting}
                    style="padding: 0.6rem 1.5rem; background: transparent; color: {textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">← Back</button>
                <button onclick={handleSubmit}
                    disabled={isSubmitting || !requesterName.trim()}
                    style="flex: 1; padding: 0.75rem; background: {accent}; color: #fff; border: none; border-radius: 8px; cursor: pointer; font-size: 1rem; font-weight: 600; display: flex; align-items: center; justify-content: center; gap: 0.5rem;">
                    {#if isSubmitting}
                        <LoaderIcon size={16} style="animation: spin 1s linear infinite;" />
                        Submitting...
                    {:else}
                        Start Review
                    {/if}
                </button>
            </div>
        {/if}
    </div>
{/if}

<KbInputSearchDialog bind:open={searchDialogOpen} onSelect={onSearchSelect} />

<style>
    @keyframes spin { to { transform: rotate(360deg); } }
</style>
