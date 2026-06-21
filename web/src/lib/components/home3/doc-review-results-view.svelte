<script lang="ts">
    import { onMount, onDestroy } from 'svelte';
    import { getRequest, updateFinding, stopRequest } from '$lib/services/docReviewService';
    import type { RequestStatus, FindingItem } from '$lib/services/docReviewService';
    import LoaderIcon from '@lucide/svelte/icons/loader';
    import XIcon from '@lucide/svelte/icons/x';
    import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
    import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
    import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';

    let { darkMode = true, requestId = 0, reportId = 0 }: { darkMode: boolean; requestId: number; reportId?: number } = $props();

    // Design tokens
    let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
    let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
    let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
    let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
    let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
    let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
    let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
    let inputBg = $derived(darkMode ? '#252A3A' : '#F2F4F7');
    let successBg = $derived(darkMode ? 'rgba(34,197,94,0.15)' : 'rgba(34,197,94,0.10)');

    // State
    let request = $state<RequestStatus | null>(null);
    let findings = $state<FindingItem[]>([]);
    let reportData = $state<any>(null);
    let error = $state('');
    let filterPass = $state('');
    let filterSeverity = $state('');
    let expandedFindings = $state<Set<number>>(new Set());
    let activeTab = $state<'findings' | 'report'>('findings');
    let isStopping = $state(false);

    // Derived counts
    let highCount = $derived(findings.filter(f => f.severity === 'high').length);
    let mediumCount = $derived(findings.filter(f => f.severity === 'medium').length);
    let lowCount = $derived(findings.filter(f => f.severity === 'low').length);
    let pendingCount = $derived(findings.filter(f => f.review_status === 'pending').length);

    // Derived filtered findings
    let filteredFindings = $derived.by(() => {
        let result = findings;
        if (filterPass) result = result.filter(f => f.pass === filterPass);
        if (filterSeverity) result = result.filter(f => f.severity === filterSeverity);
        return result;
    });

    // Derived unique passes
    let passes = $derived([...new Set(findings.map(f => f.pass))].sort());

    // Effective report ID for links (fall back to requestId)
    let linkReportId = $derived(reportId || requestId);

    // Polling
    let pollTimer: ReturnType<typeof setTimeout>;
    let isActive = $state(true);

    async function pollStatus() {
        if (!isActive) return;
        try {
            const result = await getRequest(requestId);
            request = result.request;
            findings = result.findings || [];

            if (request.status === 'completed' || request.status === 'failed' || request.status === 'stopped') {
                isActive = false;
                if (request.status === 'completed' && reportId) {
                    try {
                        const res = await fetch(`/api/v1/doc-review/reports/${reportId}`, { credentials: 'same-origin' });
                        const data = await res.json();
                        if (data.status && data.report) {
                            reportData = data.report;
                        }
                    } catch {}
                }
            }
        } catch (e: any) {
            error = e.message;
            isActive = false;
        }
    }

    let intervalId: ReturnType<typeof setInterval>;
    onMount(async () => {
        await pollStatus();
        if (isActive) {
            intervalId = setInterval(pollStatus, 3000);
        }
    });

    onDestroy(() => {
        clearInterval(intervalId);
    });

    async function handleAcceptReject(findingId: number, status: string) {
        try {
            await updateFinding(findingId, status);
            findings = findings.map(f => f.id === findingId ? { ...f, review_status: status } : f);
        } catch (e: any) {
            error = e.message;
        }
    }

    async function handleStop() {
        isStopping = true;
        try {
            await stopRequest(requestId);
            await pollStatus();
        } catch (e: any) {
            error = e.message;
        }
        isStopping = false;
    }

    function toggleFinding(id: number) {
        const next = new Set(expandedFindings);
        if (next.has(id)) next.delete(id); else next.add(id);
        expandedFindings = next;
    }
</script>

<!-- Loading State -->
{#if !request}
    <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 4rem 2rem; color: {textSecondary};">
        <LoaderIcon size={32} style="animation: spin 1s linear infinite; margin-bottom: 1rem;" />
        <div>Loading review request...</div>
    </div>
{:else if request.status === 'accepted' || request.status === 'running'}
    <!-- Running State -->
    <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 4rem 2rem;">
        <LoaderIcon size={48} style="animation: spin 1s linear infinite; color: {accent}; margin-bottom: 1rem;" />
        <h2 style="color: {textPrimary}; font-size: 1.25rem; margin-bottom: 0.5rem;">Review In Progress</h2>
        <p style="color: {textSecondary}; margin-bottom: 1rem;">
            Status: <strong>{request.status}</strong>
        </p>
        <button onclick={handleStop} disabled={isStopping}
            style="padding: 0.5rem 1rem; background: rgba(239,68,68,0.15); color: #ef4444; border: 1px solid rgba(239,68,68,0.3); border-radius: 8px; cursor: pointer;">
            {isStopping ? 'Stopping...' : 'Stop Review'}
        </button>
    </div>
{:else if request.status === 'failed'}
    <!-- Failed State -->
    <div style="padding: 2rem;">
        <div style="background: rgba(239,68,68,0.1); border: 1px solid rgba(239,68,68,0.3); border-radius: 12px; padding: 2rem; text-align: center;">
            <AlertCircleIcon size={48} style="color: #ef4444; margin-bottom: 1rem;" />
            <h2 style="color: {textPrimary}; font-size: 1.25rem; margin-bottom: 0.5rem;">Review Failed</h2>
            <p style="color: {textSecondary}; margin-bottom: 0.5rem;">{request.error_message}</p>
        </div>
    </div>
{:else if request.status === 'stopped'}
    <!-- Stopped State -->
    <div style="padding: 2rem;">
        <div style="background: rgba(245,158,11,0.1); border: 1px solid rgba(245,158,11,0.3); border-radius: 12px; padding: 2rem; text-align: center;">
            <XIcon size={48} style="color: #f59e0b; margin-bottom: 1rem;" />
            <h2 style="color: {textPrimary}; font-size: 1.25rem; margin-bottom: 0.5rem;">Review Stopped</h2>
            <p style="color: {textSecondary};">The review was cancelled before completion.</p>
        </div>
    </div>
{:else}
    <!-- Completed State -->
    <div style="padding: 1.5rem;">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem;">
            <div>
                <h1 style="font-size: 1.5rem; font-weight: 700; color: {textPrimary};">Review Results</h1>
                <p style="color: {textSecondary}; font-size: 0.85rem;">{findings.length} findings · {request.tier}</p>
            </div>
            <div style="display: flex; gap: 0.5rem;">
                <button onclick={() => activeTab = 'findings'}
                    style="padding: 0.4rem 1rem; background: {activeTab === 'findings' ? accentTint : 'transparent'}; color: {activeTab === 'findings' ? accent : textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">Findings</button>
                <button onclick={() => activeTab = 'report'}
                    style="padding: 0.4rem 1rem; background: {activeTab === 'report' ? accentTint : 'transparent'}; color: {activeTab === 'report' ? accent : textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">Report</button>
            </div>
        </div>

        <!-- Summary Cards -->
        <div style="display: grid; grid-template-columns: repeat(4, 1fr); gap: 1rem; margin-bottom: 1.5rem;">
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1rem; text-align: center;">
                <div style="font-size: 2rem; font-weight: 700; color: {textPrimary};">{findings.length}</div>
                <div style="font-size: 0.8rem; color: {textMuted};">Total Findings</div>
            </div>
            <div style="background: {cardBg}; border: 1px solid rgba(239,68,68,0.3); border-radius: 12px; padding: 1rem; text-align: center;">
                <div style="font-size: 2rem; font-weight: 700; color: #ef4444;">{highCount}</div>
                <div style="font-size: 0.8rem; color: #ef4444;">High Severity</div>
            </div>
            <div style="background: {cardBg}; border: 1px solid rgba(245,158,11,0.3); border-radius: 12px; padding: 1rem; text-align: center;">
                <div style="font-size: 2rem; font-weight: 700; color: #f59e0b;">{mediumCount}</div>
                <div style="font-size: 0.8rem; color: #f59e0b;">Medium Severity</div>
            </div>
            <div style="background: {cardBg}; border: 1px solid rgba(34,197,94,0.3); border-radius: 12px; padding: 1rem; text-align: center;">
                <div style="font-size: 2rem; font-weight: 700; color: #22c55e;">{lowCount}</div>
                <div style="font-size: 0.8rem; color: #22c55e;">Low Severity</div>
            </div>
        </div>

        <!-- Findings Tab -->
        {#if activeTab === 'findings'}
            <!-- Filters -->
            <div style="display: flex; gap: 0.5rem; margin-bottom: 1rem;">
                <select bind:value={filterPass} style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.4rem 0.75rem; color: {textPrimary}; font-size: 0.85rem;">
                    <option value="">All Passes</option>
                    {#each passes as p}
                        <option value={p}>{p}</option>
                    {/each}
                </select>
                <select bind:value={filterSeverity} style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 8px; padding: 0.4rem 0.75rem; color: {textPrimary}; font-size: 0.85rem;">
                    <option value="">All Severities</option>
                    <option value="high">High</option>
                    <option value="medium">Medium</option>
                    <option value="low">Low</option>
                </select>
                <span style="margin-left: auto; color: {textMuted}; font-size: 0.85rem; align-self: center;">
                    {filteredFindings.length} of {findings.length} · {pendingCount} pending review
                </span>
            </div>

            <!-- Findings List -->
            <div style="display: flex; flex-direction: column; gap: 0.5rem;">
                {#each filteredFindings as finding (finding.id)}
                    <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 8px; overflow: hidden;">
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div onclick={() => toggleFinding(finding.id)} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleFinding(finding.id); }}
                            role="button" tabindex="0"
                            style="display: flex; align-items: center; gap: 0.75rem; width: 100%; text-align: left; padding: 0.75rem 1rem; background: transparent; border: none; cursor: pointer; color: {textPrimary};">
                            <div style="width: 4px; height: 32px; border-radius: 2px;
                                background: {finding.severity === 'high' ? '#ef4444' : finding.severity === 'medium' ? '#f59e0b' : '#22c55e'};">
                            </div>
                            <div style="flex: 1; min-width: 0;">
                                <div style="font-weight: 600; font-size: 0.9rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                                    {finding.title}
                                </div>
                                <div style="display: flex; gap: 0.5rem; font-size: 0.75rem; color: {textMuted};">
                                    <span>{finding.pass} · {finding.aspect}</span>
                                    <span style="text-transform: capitalize;">{finding.finding_type.replace(/_/g, ' ')}</span>
                                </div>
                            </div>
                            <div style="display: flex; gap: 0.25rem; align-items: center;">
                                {#if finding.review_status === 'pending'}
                                    <button onclick={(e) => { e.stopPropagation(); handleAcceptReject(finding.id, 'accepted'); }}
                                        style="padding: 0.25rem 0.5rem; background: {successBg}; color: #22c55e; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Accept</button>
                                    <button onclick={(e) => { e.stopPropagation(); handleAcceptReject(finding.id, 'rejected'); }}
                                        style="padding: 0.25rem 0.5rem; background: rgba(239,68,68,0.1); color: #ef4444; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Reject</button>
                                {:else}
                                    <span style="font-size: 0.75rem; color: {textMuted}; text-transform: capitalize;">{finding.review_status}</span>
                                {/if}
                            </div>
                            {#if expandedFindings.has(finding.id)}
                                <ChevronDownIcon size={16} style="color: {textMuted};" />
                            {:else}
                                <ChevronRightIcon size={16} style="color: {textMuted};" />
                            {/if}
                        </div>
                        {#if expandedFindings.has(finding.id)}
                            <div style="padding: 0 1rem 0.75rem; border-top: 1px solid {borderColor};">
                                <p style="color: {textSecondary}; font-size: 0.85rem; margin-top: 0.5rem;">{finding.description}</p>
                                {#if finding.evidence}
                                    <div style="margin-top: 0.5rem; padding: 0.5rem; background: {inputBg}; border-radius: 6px; font-size: 0.8rem; color: {textMuted}; font-family: monospace;">
                                        {finding.evidence}
                                    </div>
                                {/if}
                                {#if finding.suggestion}
                                    <p style="margin-top: 0.5rem; color: {accent}; font-size: 0.85rem;"><strong>Suggestion:</strong> {finding.suggestion}</p>
                                {/if}
                                {#if finding.location}
                                    <p style="margin-top: 0.25rem; color: {textMuted}; font-size: 0.8rem;">Location: {finding.location}</p>
                                {/if}
                                <p style="margin-top: 0.25rem; color: {textMuted}; font-size: 0.75rem;">Confidence: {Math.round(finding.confidence * 100)}%</p>
                            </div>
                        {/if}
                    </div>
                {/each}
                {#if filteredFindings.length === 0}
                    <div style="padding: 2rem; text-align: center; color: {textMuted}; background: {cardBg}; border: 1px solid {borderColor}; border-radius: 8px;">
                        No findings match the current filters.
                    </div>
                {/if}
            </div>
        {/if}

        <!-- Report Tab -->
        {#if activeTab === 'report'}
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1.5rem; color: {textPrimary};">
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
                    <h2 style="font-size: 1.1rem; font-weight: 600;">Full Report</h2>
                    <a href={`/api/v1/doc-review/reports/${linkReportId}/export?format=md`} target="_blank"
                        style="padding: 0.4rem 0.75rem; background: {accent}; color: #fff; border-radius: 6px; text-decoration: none; font-size: 0.85rem;">
                        Export Markdown
                    </a>
                    <a href={`/api/v1/doc-review/reports/${linkReportId}/html`} target="_blank"
                        style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border-radius: 6px; text-decoration: none; font-size: 0.85rem;">
                        View HTML
                    </a>
                </div>
                <p style="color: {textSecondary}; margin-bottom: 1rem;">
                    Total: {findings.length} findings · {highCount} high · {mediumCount} medium · {lowCount} low
                </p>
                {#each passes as pass}
                    <div style="margin-bottom: 1rem;">
                        <h3 style="font-weight: 600; color: {accent}; margin-bottom: 0.5rem; font-size: 0.95rem;">{pass}</h3>
                        {#each findings.filter(f => f.pass === pass).slice(0, 5) as finding}
                            <div style="padding: 0.5rem 0.75rem; border-left: 2px solid {borderColor}; margin-bottom: 0.25rem; font-size: 0.85rem;">
                                <span style="color: {finding.severity === 'high' ? '#ef4444' : finding.severity === 'medium' ? '#f59e0b' : '#22c55e'}; font-weight: 600;">{finding.severity.toUpperCase()}</span>
                                {' '}{finding.title}
                            </div>
                        {/each}
                    </div>
                {/each}
                <div style="text-align: center; margin-top: 1rem;">
                    <a href={`/api/v1/doc-review/reports/${linkReportId}`} target="_blank"
                        style="color: {accent}; font-size: 0.85rem;">View Full Report JSON →</a>
                </div>
            </div>
        {/if}
    </div>
{/if}

<style>
    @keyframes spin { to { transform: rotate(360deg); } }
</style>
