<script lang="ts">
    import { onMount } from 'svelte';
    import { listAspects, listTiers, submitRequest, getRequest, updateFinding } from '$lib/services/docReviewService';
    import type { AspectInfo, TierInfo, FindingItem, ReferenceDoc } from '$lib/services/docReviewService';
    import DocReviewResultsView from './doc-review-results-view.svelte';
    import SearchIcon from '@lucide/svelte/icons/search';
    import CheckIcon from '@lucide/svelte/icons/check';
    import XIcon from '@lucide/svelte/icons/x';
    import LoaderIcon from '@lucide/svelte/icons/loader';
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
    let submitError = $state('');
    let isSubmitting = $state(false);
    let searchQuery = $state('');
    let docSearchResults = $state<Array<{id: number; title: string}>>([]);
    let isSearching = $state(false);

    // Derived: aspects grouped by group
    let aspectsByGroup = $derived.by(() => {
        const map: Record<string, AspectInfo[]> = {};
        for (const a of aspects) {
            if (!map[a.group]) map[a.group] = [];
            map[a.group].push(a);
        }
        return map;
    });

    // Derived: which aspects are selected based on tier
    let effectiveAspects = $derived.by(() => {
        if (selectedTier === 'custom') {
            return [...customAspects];
        }
        const tier = tiers.find(t => t.key === selectedTier);
        return tier?.aspect_names || [];
    });

    // Derived: group labels
    const groupLabels: Record<string, string> = {
        P1: 'Language & Style', P2: 'Structure & Organization',
        P3: 'Content Quality', P4: 'Consistency',
        P5: 'Technical & Compliance', P6: 'Meta & Process',
    };

    onMount(async () => {
        try {
            aspects = await listAspects();
            tiers = await listTiers();
            // Default: auto-select must_review aspects for custom mode
            const mustTier = tiers.find(t => t.key === 'must_review');
            if (mustTier) mustTier.aspect_names.forEach(a => customAspects.add(a));
        } catch (e) {
            submitError = 'Failed to load aspects';
        }
    });

    // Document search (debounced)
    let searchTimer: ReturnType<typeof setTimeout>;
    function onSearchInput(e: Event) {
        const q = (e.target as HTMLInputElement).value;
        searchQuery = q;
        clearTimeout(searchTimer);
        if (q.length < 2) { docSearchResults = []; return; }
        searchTimer = setTimeout(async () => {
            isSearching = true;
            try {
                const res = await fetch(`/api/v1/kb/inputs?query=${encodeURIComponent(q)}&limit=10`, { credentials: 'same-origin' });
                const data = await res.json();
                docSearchResults = (data.inputs || data.records || data.data || []).map((r: any) => ({
                    id: r.id, title: r.title || r.file_name || `Document ${r.id}`,
                }));
            } catch { docSearchResults = []; }
            isSearching = false;
        }, 300);
    }

    function selectDoc(doc: {id: number; title: string}) {
        selectedDocId = doc.id;
        selectedDocTitle = doc.title;
        docSearchResults = [];
        searchQuery = doc.title;
        currentStep = 2;
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

    async function handleSubmit() {
        if (!selectedDocId) { submitError = 'Please select a document'; return; }
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
            });
            submittedRequestId = result.request_id;
        } catch (e: any) {
            submitError = e.message || 'Submission failed';
        } finally {
            isSubmitting = false;
        }
    }
</script>

{#if submittedRequestId}
    <DocReviewResultsView {darkMode} requestId={submittedRequestId} />
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
                <div style="position: relative;">
                    <div style="display: flex; align-items: center; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem;">
                        <SearchIcon size={16} style="color: {textMuted}; margin-right: 0.5rem;" />
                        <input type="text" placeholder="Search documents..."
                            value={searchQuery} oninput={onSearchInput}
                            style="flex: 1; background: transparent; border: none; outline: none; color: {textPrimary}; font-size: 0.9rem;" />
                        {#if isSearching}
                            <LoaderIcon size={14} style="color: {textMuted};" />
                        {/if}
                    </div>
                    {#if docSearchResults.length > 0}
                        <div style="position: absolute; top: 100%; left: 0; right: 0; background: {cardBg}; border: 1px solid {borderColor}; border-radius: 8px; margin-top: 4px; z-index: 10; max-height: 240px; overflow-y: auto;">
                            {#each docSearchResults as doc}
                                <button onclick={() => selectDoc(doc)}
                                    style="display: block; width: 100%; text-align: left; padding: 0.75rem 1rem; background: transparent; border: none; color: {textPrimary}; cursor: pointer; font-size: 0.9rem;
                                    border-bottom: 1px solid {borderColor};">
                                    {doc.title}
                                </button>
                            {/each}
                        </div>
                    {/if}
                </div>
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
                        <label style="display: flex; align-items: flex-start; gap: 0.75rem; padding: 1rem; background: {inputBg}; border: 2px solid {selectedTier === tier.key ? accent : borderColor}; border-radius: 8px; cursor: pointer;">
                            <input type="radio" name="tier" value={tier.key}
                                checked={selectedTier === tier.key}
                                onchange={() => selectedTier = tier.key}
                                style="margin-top: 0.2rem;" />
                            <div>
                                <div style="font-weight: 600; color: {textPrimary};">{tier.label}</div>
                                <div style="font-size: 0.85rem; color: {textSecondary};">{tier.description} — {tier.aspect_names.length} aspects</div>
                            </div>
                        </label>
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
                    <input type="text" placeholder="Reference document title or ID"
                        style="flex: 1; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                    <button onclick={() => {/* TODO: add search for reference docs */}}
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
                    <input type="text" placeholder="Template name or path"
                        style="width: 100%; background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.5rem 0.75rem; color: {textPrimary}; font-size: 0.9rem;" />
                </div>
                <div style="margin-bottom: 1rem;">
                    <label style="display: block; margin-bottom: 0.3rem; color: {textSecondary}; font-size: 0.85rem;">Doc Template (optional)</label>
                    <input type="text" placeholder="Template name or path"
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

<style>
    @keyframes spin { to { transform: rotate(360deg); } }
</style>
