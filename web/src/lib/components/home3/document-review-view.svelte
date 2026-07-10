<script lang="ts">
    import { onMount } from 'svelte';
    import { listAspects, listTiers, submitRequest } from '$lib/services/docReviewService';
    import type { AspectInfo, TierInfo, FindingItem, ReferenceDoc, ReviewRunListItem } from '$lib/services/docReviewService';
    import { uploadKbInputs, listKnowledgeStores } from '$lib/services/kbService';
    import type { KbInputRecord, KnowledgeStoreRecord } from '$lib/services/kbService';
    import { knowledgeStoreState } from './knowledge-store-state.svelte';
    import KbInputSearchDialog from './kb-input-search-dialog.svelte';
    import DocReviewResultsView from './doc-review-results-view.svelte';
    import DocReviewMonitor from './doc-review-monitor.svelte';
    import DocReviewRequestsList from './doc-review-requests-list.svelte';
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
    // Single shared aspect selection (internal mechanism). Every tier On/Off toggle and
    // every aspect chip edits THIS one set, so the chosen check level and the actually
    // submitted aspects can never diverge (the old per-tier sets could strand a selection
    // in a tier you weren't submitting). A tier is "On" iff ≥1 of its aspects is in here.
    let selectedAspects = $state<Set<string>>(new Set());
    // Validation dialog: shown at submit when something required is missing; confirming
    // it jumps the wizard back to the offending step so the user can fix it.
    let dialogMessage = $state('');
    let dialogTargetStep = $state<number | null>(null);
    let requesterName = $state('');
    let notes = $state('');
    let reviewDepth = $state<1 | 2 | 3>(1);
    let referenceDocs = $state<ReferenceDoc[]>([]);
    let viewingRun = $state<{ requestId: number; runId?: number; reportId?: number } | null>(null);  // when set, show that job's results
    let submitBanner = $state('');                        // brief "review started" notice
    let submitError = $state('');
    let isSubmitting = $state(false);
    let reportTemplate = $state('');
    let docTemplate = $state('');
    // Bumped after a successful submit so the requests list reloads when shown.
    let requestsRefreshKey = $state(0);
    // Held here (not in the list component) so the displayed list + Search
    // selection survive while the results "View" window replaces the form.
    let requestsList = $state<ReviewRunListItem[]>([]);
    let requestsShowingSelection = $state(false);

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

    // Derived: the aspects to submit are exactly the shared selection.
    let effectiveAspects = $derived([...selectedAspects]);

    // Check-level label for the summary: if the selection exactly matches one tier's
    // full aspect set, show that tier; otherwise list the tiers that are On, or "—".
    let checkLevelLabel = $derived.by(() => {
        const exact = tiers.find(t => t.aspect_names.length > 0 && t.aspect_names.length === selectedAspects.size && t.aspect_names.every(n => selectedAspects.has(n)));
        if (exact) return exact.label;
        const on = tiers.filter(t => t.aspect_names.some(n => selectedAspects.has(n))).map(t => t.label);
        return on.length ? `Custom (${on.join(', ')})` : '—';
    });

    // Tier key persisted with the request (one of the built-in keys, or "custom").
    let submitTier = $derived.by(() => {
        const exact = tiers.find(t => t.aspect_names.length > 0 && t.aspect_names.length === selectedAspects.size && t.aspect_names.every(n => selectedAspects.has(n)));
        return exact ? exact.key : 'custom';
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

    function aspectChecked(name: string): boolean {
        return selectedAspects.has(name);
    }

    function toggleAspect(name: string) {
        const next = new Set(selectedAspects);
        if (next.has(name)) next.delete(name); else next.add(name);
        selectedAspects = next;
    }

    // How many of a tier's aspects are currently selected.
    function tierSelectedCount(tier: TierInfo): number {
        let n = 0;
        for (const name of tier.aspect_names) if (selectedAspects.has(name)) n++;
        return n;
    }

    // A tier is "On" iff at least one of its aspects is selected.
    function tierIsOn(tier: TierInfo): boolean {
        return tierSelectedCount(tier) > 0;
    }

    function allAspectsChecked(tier: TierInfo): boolean {
        return tier.aspect_names.length > 0 && tierSelectedCount(tier) === tier.aspect_names.length;
    }

    // Add or remove every aspect of a tier from the shared selection.
    function setAllInTier(tier: TierInfo, on: boolean) {
        const next = new Set(selectedAspects);
        for (const name of tier.aspect_names) {
            if (on) next.add(name); else next.delete(name);
        }
        selectedAspects = next;
    }

    // The On/Off toggle: Off→On adds all the tier's aspects, On→Off removes them all.
    function toggleTier(tier: TierInfo) {
        setAllInTier(tier, !tierIsOn(tier));
    }

    function groupSelectedCount(items: Array<{ name: string }>): number {
        return items.reduce((n, it) => n + (selectedAspects.has(it.name) ? 1 : 0), 0);
    }

    function showValidationDialog(message: string, targetStep: number) {
        dialogMessage = message;
        dialogTargetStep = targetStep;
    }

    function confirmValidationDialog() {
        if (dialogTargetStep !== null) currentStep = dialogTargetStep;
        dialogMessage = '';
        dialogTargetStep = null;
    }

    onMount(async () => {
        try {
            aspects = await listAspects();
            tiers = await listTiers();
            // Initial selection driven by [reviewers.<aspect>].checked in doc-review.local.toml.
            selectedAspects = new Set(aspects.filter(a => a.checked).map(a => a.name));
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
        if (mode === 'search') {
            // Search Library opens the document library search dialog directly.
            searchDialogOpen = true;
        }
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
        // Validation failures surface as a confirm dialog; confirming jumps back to the
        // step that needs fixing (no aspects → Step 2, which also blocks "Next" up front).
        if (!selectedDocId) { showValidationDialog('Please select a document to review.', 1); return; }
        if (effectiveAspects.length === 0) { showValidationDialog('Select at least one aspect to review.', 2); return; }
        if (!requesterName.trim()) { showValidationDialog('Please enter your name.', 4); return; }

        isSubmitting = true;
        submitError = '';
        try {
            const result = await submitRequest({
                input_record_id: selectedDocId,
                tier: submitTier,
                aspects: effectiveAspects,
                review_depth: reviewDepth,
                reference_docs: referenceDocs.length > 0 ? referenceDocs : undefined,
                notes: notes || undefined,
                requester_name: requesterName,
                requester_id: 0, // TODO: resolve from auth context
                report_template: reportTemplate || undefined,
                doc_template: docTemplate || undefined,
            });
            viewingRun = { requestId: result.request_id, runId: result.run_id, reportId: result.report_id };
            // A new submission resets any Search selection so the refreshed full
            // list (now including this request) is what shows on return.
            requestsShowingSelection = false;
            requestsRefreshKey += 1;
        } catch (e: any) {
            submitError = e.message || 'Submission failed';
        } finally {
            isSubmitting = false;
        }
    }

    function resetForm() {
        submitError = '';
        currentStep = 1;
        selectedDocId = null;
        selectedDocTitle = '';
        uploadFile = null;
        reviewDepth = 1;
    }
</script>

{#if viewingRun}
    <DocReviewResultsView
        {darkMode}
        requestId={viewingRun.requestId}
        runId={viewingRun.runId ?? 0}
        reportId={viewingRun.reportId ?? 0}
        docTitle={selectedDocTitle}
        onNewReview={() => { viewingRun = null; }}
    />
{:else}
    <div style="padding: 1.5rem; color: {textPrimary};">
        <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 0.5rem;">Document Review</h1>
        <p style="color: {textSecondary}; margin-bottom: 1.25rem;">
            Submit a document for AI-powered review across quality, compliance, and technical aspects.
        </p>

        <!-- DR15: live monitor of all in-flight review jobs -->
        <DocReviewMonitor {darkMode} onView={(id) => { viewingRun = { requestId: id }; }} onStop={() => { requestsRefreshKey += 1; }} />

        {#if submitBanner}
            <div style="margin-bottom: 1.25rem; padding: 0.75rem 1rem; background: {accentTint}; border: 1px solid {borderColor}; border-radius: 8px; color: {accent}; font-size: 0.9rem; display: flex; align-items: center; justify-content: space-between;">
                <span>{submitBanner}</span>
                <button onclick={() => submitBanner = ''} style="background: none; border: none; color: {accent}; cursor: pointer; font-size: 1rem;">×</button>
            </div>
        {/if}

        <!-- Step indicators -->
        <div style="display: flex; gap: 0.5rem; margin-bottom: 2rem; font-size: 0.8rem;">
            {#each ['Select Document', 'Check Level', 'References', 'Submit'] as step, i}
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
                    <!-- Search Library opens the search dialog directly; no inline search box. -->
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

        <!-- Step 2: Check Level & Aspects -->
        {#if currentStep === 2}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 0.35rem;">Step 2: Choose Check Level</h2>
                <p style="color: {textSecondary}; font-size: 0.85rem; margin-bottom: 1rem;">
                    Toggle a level On to review its aspects, then expand it to fine-tune. {effectiveAspects.length} aspect{effectiveAspects.length === 1 ? '' : 's'} selected.
                </p>
                <fieldset style="margin: 0 0 1rem 0; padding: 0; border: none;">
                    <legend style="font-size: 0.85rem; color: {textSecondary}; margin-bottom: 0.5rem;">Review depth</legend>
                    <div role="radiogroup" aria-label="Review depth" style="display: flex; gap: 0.5rem; flex-wrap: wrap;">
                        {#each [1, 2, 3] as depth}
                            <button
                                type="button"
                                role="radio"
                                aria-checked={reviewDepth === depth}
                                onclick={() => (reviewDepth = depth as 1 | 2 | 3)}
                                style="padding: 0.45rem 0.9rem; border-radius: 8px; border: 1px solid {reviewDepth === depth ? accent : borderColor}; background: {reviewDepth === depth ? accentTint : inputBg}; color: {reviewDepth === depth ? textPrimary : textSecondary}; cursor: pointer; font-size: 0.85rem; font-weight: 600;"
                            >
                                Depth {depth}
                            </button>
                        {/each}
                    </div>
                </fieldset>
                <div style="display: flex; flex-direction: column; gap: 0.75rem;">
                    {#each tiers as tier}
                        {@const on = tierIsOn(tier)}
                        <div style="background: {inputBg}; border: 2px solid {on ? accent : borderColor}; border-radius: 8px; overflow: hidden;">
                            <div style="display: flex; align-items: flex-start; gap: 0.75rem; padding: 1rem;">
                                <div style="flex: 1;">
                                    <div style="font-weight: 600; color: {textPrimary};">{tier.label}</div>
                                    <div style="font-size: 0.85rem; color: {textSecondary};">{tier.description} — {tierSelectedCount(tier)} of {tier.aspect_names.length} aspects selected</div>
                                </div>
                                <!-- On/Off toggle: On iff ≥1 of the tier's aspects is selected -->
                                <button type="button" role="switch" aria-checked={on} onclick={() => toggleTier(tier)}
                                    disabled={tier.aspect_names.length === 0}
                                    title={on ? 'On — click to remove this level’s aspects' : 'Off — click to add this level’s aspects'}
                                    style="display: inline-flex; align-items: center; gap: 0.45rem; background: none; border: none; cursor: {tier.aspect_names.length === 0 ? 'not-allowed' : 'pointer'}; opacity: {tier.aspect_names.length === 0 ? 0.4 : 1};">
                                    <span style="font-size: 0.72rem; font-weight: 700; width: 22px; text-align: right; color: {on ? accent : textMuted};">{on ? 'On' : 'Off'}</span>
                                    <span style="position: relative; width: 38px; height: 20px; border-radius: 10px; background: {on ? accent : borderColor}; transition: background 0.15s;">
                                        <span style="position: absolute; top: 2px; left: {on ? '20px' : '2px'}; width: 16px; height: 16px; border-radius: 50%; background: #fff; transition: left 0.15s;"></span>
                                    </span>
                                </button>
                            </div>
                            {#if tier.aspect_names.length > 0}
                                <button type="button" onclick={() => toggleTierAspects(tier.key)}
                                    style="display: flex; align-items: center; gap: 0.4rem; width: 100%; padding: 0.5rem 1rem; background: transparent; border: none; border-top: 1px solid {borderColor}; cursor: pointer; color: {textMuted}; font-size: 0.8rem; text-align: left;">
                                    {#if expandedTiers.has(tier.key)}
                                        <ChevronDownIcon size={14} />
                                    {:else}
                                        <ChevronRightIcon size={14} />
                                    {/if}
                                    {expandedTiers.has(tier.key) ? 'Hide' : 'View'} {tierSelectedCount(tier)}/{tier.aspect_names.length} selected aspects
                                </button>
                                {#if expandedTiers.has(tier.key)}
                                    <div style="padding: 0.25rem 1rem 0.85rem 2rem; display: flex; flex-direction: column; gap: 0.6rem;">
                                        <!-- Select / deselect all toggle -->
                                        <div style="display: flex; align-items: center; gap: 0.6rem;">
                                            <button type="button" onclick={() => setAllInTier(tier, !allAspectsChecked(tier))}
                                                style="display: inline-flex; align-items: center; gap: 0.35rem; padding: 0.25rem 0.7rem; background: {inputBg}; border: 1px solid {accent}; border-radius: 6px; cursor: pointer; color: {accent}; font-size: 0.78rem; font-weight: 600;">
                                                {allAspectsChecked(tier) ? 'Deselect all' : 'Select all'}
                                            </button>
                                            <span style="font-size: 0.75rem; color: {textMuted};">{tierSelectedCount(tier)} of {tier.aspect_names.length} selected</span>
                                        </div>
                                        {#each groupAspectNames(tier.aspect_names) as cat}
                                            <div>
                                                <div style="font-size: 0.75rem; font-weight: 600; color: {textSecondary}; margin-bottom: 0.25rem;">
                                                    {cat.group} — {cat.label}
                                                    <span style="color: {textMuted}; font-weight: 400;">({groupSelectedCount(cat.items)}/{cat.items.length})</span>
                                                </div>
                                                <div style="display: flex; flex-wrap: wrap; gap: 0.35rem;">
                                                    {#each cat.items as item}
                                                        {@const checked = aspectChecked(item.name)}
                                                        <button type="button" onclick={() => toggleAspect(item.name)}
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
                <!-- Wrap in a span so the help tooltip shows even while the button is disabled -->
                <span title={effectiveAspects.length === 0 ? 'Select at least one aspect to review before continuing.' : ''} style="display: inline-flex;">
                    <button onclick={() => currentStep = 3}
                        disabled={effectiveAspects.length === 0}
                        style="padding: 0.6rem 1.5rem; background: {effectiveAspects.length > 0 ? accent : borderColor}; color: {effectiveAspects.length > 0 ? '#fff' : textMuted}; border: none; border-radius: 8px; cursor: {effectiveAspects.length > 0 ? 'pointer' : 'not-allowed'}; font-size: 0.9rem;">Next →</button>
                </span>
            </div>
        {/if}

        <!-- Step 3: Supporting Documents -->
        {#if currentStep === 3}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem;">Step 3: Supporting Documents <span style="font-weight: 400; color: {textMuted};">(optional)</span></h2>
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
                <button onclick={() => currentStep = 2}
                    style="padding: 0.6rem 1.5rem; background: transparent; color: {textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">← Back</button>
                <button onclick={() => currentStep = 4}
                    style="padding: 0.6rem 1.5rem; background: {accent}; color: #fff; border: none; border-radius: 8px; cursor: pointer; font-size: 0.9rem;">Next →</button>
            </div>
        {/if}

        <!-- Step 4: Notes & Submit -->
        {#if currentStep === 4}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; margin-bottom: 1rem;">
                <h2 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem;">Step 4: Review Details</h2>
                <div style="margin-bottom: 1rem;">
                    <label for="review-requester-name" style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Your Name *</label>
                    <input id="review-requester-name" type="text" bind:value={requesterName} placeholder="Enter your name"
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                </div>
                <div style="margin-bottom: 1rem;">
                    <label for="review-notes" style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Notes (optional)</label>
                    <textarea id="review-notes" bind:value={notes} placeholder="e.g., Focus on sterilization validation sections..."
                        rows={4}
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem; resize: vertical;"></textarea>
                </div>
                <div style="margin-bottom: 1rem;">
                    <label for="review-report-template" style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Report Template (optional)</label>
                    <input id="review-report-template" type="text" bind:value={reportTemplate} placeholder="Template name or path"
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                </div>
                <div style="margin-bottom: 1rem;">
                    <label for="review-doc-template" style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Doc Template (optional)</label>
                    <input id="review-doc-template" type="text" bind:value={docTemplate} placeholder="Template name or path"
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
                    <div style="color: {textPrimary};">{checkLevelLabel}</div>
                    <div style="color: {textSecondary};">Aspects:</div>
                    <div style="color: {textPrimary};">{effectiveAspects.length} selected</div>
                    <div style="color: {textSecondary};">Review Depth:</div>
                    <div style="color: {textPrimary};">Depth {reviewDepth}</div>
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
                <button onclick={() => currentStep = 3}
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

        <!-- All document review requests, with a Search dialog -->
        <div style="margin-top: 2rem;">
            <DocReviewRequestsList
                {darkMode}
                refreshKey={requestsRefreshKey}
                bind:requests={requestsList}
                bind:showingSelection={requestsShowingSelection}
                onView={(run) => { viewingRun = { requestId: run.request_id, runId: run.run_id, reportId: run.report_id }; }}
            />
        </div>
    </div>
{/if}

<KbInputSearchDialog bind:open={searchDialogOpen} onSelect={onSearchSelect} />

<!-- Validation dialog: blocks submission when something required is missing; confirming
     jumps the wizard back to the step that needs fixing (dialogTargetStep). -->
{#if dialogMessage}
    <div role="dialog" aria-modal="true"
        style="position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.5);">
        <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; max-width: 380px; width: calc(100% - 2rem); box-shadow: 0 10px 40px rgba(0,0,0,0.35);">
            <h3 style="font-size: 1.05rem; font-weight: 700; color: {textPrimary}; margin-bottom: 0.5rem;">Cannot start review</h3>
            <p style="color: {textSecondary}; font-size: 0.9rem; margin-bottom: 1.25rem;">{dialogMessage}</p>
            <div style="display: flex; justify-content: flex-end;">
                <button onclick={confirmValidationDialog}
                    style="padding: 0.55rem 1.4rem; background: {accent}; color: #fff; border: none; border-radius: 8px; cursor: pointer; font-size: 0.9rem; font-weight: 600;">OK</button>
            </div>
        </div>
    </div>
{/if}

<style>
    @keyframes spin { to { transform: rotate(360deg); } }
</style>
