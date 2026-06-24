<script lang="ts">
    import { onMount, onDestroy } from 'svelte';
    import { getRequest, updateFinding, stopRequest } from '$lib/services/docReviewService';
    import type { RequestStatus, FindingItem, AspectStatus } from '$lib/services/docReviewService';
    import LoaderIcon from '@lucide/svelte/icons/loader';
    import XIcon from '@lucide/svelte/icons/x';
    import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
    import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
    import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';

    let { darkMode = true, requestId = 0, reportId = 0, docTitle = '', onNewReview }:
        { darkMode: boolean; requestId: number; reportId?: number; docTitle?: string; onNewReview?: () => void } = $props();

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
    let aspectStatuses = $state<AspectStatus[]>([]);
    let reportData = $state<any>(null);
    let error = $state('');
    let filterPass = $state('');
    let filterSeverity = $state('');
    let expandedFindings = $state<Set<number>>(new Set());
    let activeTab = $state<'findings' | 'report'>('findings');
    let isStopping = $state(false);

    // PDF file list dropdown state
    let pdfMenuOpen = $state(false);
    let pdfLoading = $state(false);
    let pdfFiles = $state<Array<{request_id: number; report_id: number; file_name: string; create_time: string; is_current: boolean}>>([]);

    async function fetchPDFFiles() {
        if (pdfLoading) return;
        pdfLoading = true;
        try {
            const res = await fetch(`/api/v1/doc-review/reports/${linkReportId}/pdf-files`, { credentials: 'same-origin' });
            const data = await res.json();
            pdfFiles = data.files || [];
        } catch {
            pdfFiles = [];
        }
        pdfLoading = false;
    }

    function togglePDFMenu() {
        if (!pdfMenuOpen) fetchPDFFiles();
        pdfMenuOpen = !pdfMenuOpen;
    }

    function closePDFMenu() { pdfMenuOpen = false; }

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
    // Report id may arrive via prop (legacy) or via the polled request after an
    // async run completes (DR15).
    let resolvedReportId = $derived(reportId || request?.report_id || 0);
    let linkReportId = $derived(resolvedReportId || requestId);

    // Polling
    let pollTimer: ReturnType<typeof setTimeout>;
    let isActive = $state(true);

    async function pollStatus() {
        if (!isActive) return;
        try {
            const result = await getRequest(requestId);
            request = result.request;
            findings = result.findings || [];
            aspectStatuses = result.aspect_statuses || [];

            if (request.status === 'completed' || request.status === 'failed' || request.status === 'stopped') {
                isActive = false;
                const rid = reportId || request.report_id;
                if (request.status === 'completed' && rid) {
                    try {
                        const res = await fetch(`/api/v1/doc-review/reports/${rid}`, { credentials: 'same-origin' });
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
{:else}
    <div style="padding: 1.5rem;">
        <!-- Back / status header -->
        <div style="display: flex; align-items: center; gap: 0.75rem; margin-bottom: 1.25rem;">
            <button onclick={() => onNewReview?.()}
                style="display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.4rem 0.9rem; background: transparent; color: {textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">← Back</button>
            <span style="color: {textMuted}; font-size: 0.85rem;">{docTitle || `Document #${request.input_record_id}`}</span>
            <span style="color: {textMuted}; font-size: 0.8rem; padding: 0.15rem 0.5rem; border: 1px solid {borderColor}; border-radius: 5px; font-family: monospace;">Request #{requestId}</span>
            <span style="margin-left: auto; font-size: 0.8rem; font-weight: 600; padding: 0.2rem 0.6rem; border-radius: 6px; background: {accentTint}; color: {accent}; text-transform: capitalize;">{request.status}</span>
            {#if request.status === 'accepted' || request.status === 'running'}
                <button onclick={handleStop} disabled={isStopping}
                    style="padding: 0.4rem 0.9rem; background: rgba(239,68,68,0.12); color: #ef4444; border: 1px solid rgba(239,68,68,0.3); border-radius: 8px; cursor: pointer; font-size: 0.85rem;">{isStopping ? 'Stopping…' : 'Stop'}</button>
            {/if}
        </div>

    <!-- Per-aspect status panel (always shown when statuses are available) -->
    {#if aspectStatuses.length > 0}
        <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 10px; padding: 1rem; margin-bottom: 1.25rem;">
            <div style="font-size: 0.8rem; font-weight: 600; color: {textSecondary}; margin-bottom: 0.6rem; text-transform: uppercase; letter-spacing: 0.05em;">Aspects</div>
            <div style="display: flex; flex-direction: column; gap: 0.4rem;">
                {#each aspectStatuses as s}
                    {@const statusColor = s.status === 'success' ? '#22c55e' : s.status === 'failed' ? '#ef4444' : s.status === 'running' ? accent : textMuted}
                    <div style="display: flex; align-items: center; gap: 0.6rem; font-size: 0.85rem;">
                        <span style="width: 8px; height: 8px; border-radius: 50%; background: {statusColor}; flex-shrink: 0;
                            {s.status === 'running' ? 'animation: pulse 1.2s ease-in-out infinite;' : ''}"></span>
                        <span style="color: {textPrimary}; font-weight: 500;">{s.aspect.replace(/_/g, ' ')}</span>
                        {#if s.pass}<span style="color: {textMuted}; font-size: 0.75rem;">({s.pass})</span>{/if}
                        <span style="color: {statusColor}; font-size: 0.75rem; font-weight: 600; text-transform: capitalize; margin-left: auto;">{s.status}</span>
                        {#if s.finding_count > 0}
                            <span style="color: {textMuted}; font-size: 0.75rem;">{s.finding_count} finding{s.finding_count !== 1 ? 's' : ''}</span>
                        {/if}
                        {#if s.error_message}
                            <span style="color: #ef4444; font-size: 0.75rem;" title={s.error_message}>error</span>
                        {/if}
                    </div>
                {/each}
            </div>
        </div>
    {/if}

    {#if request.status === 'accepted' || request.status === 'running'}
        <!-- Running: the Active Reviews monitor on the form conveys live progress -->
        <div style="display: flex; align-items: center; gap: 0.5rem; color: {textSecondary}; font-size: 0.9rem; padding: 1.5rem 0.25rem;">
            <LoaderIcon size={18} style="animation: spin 1s linear infinite; color: {accent};" />
            Reviewing the document… findings will appear here as soon as the job completes.
        </div>
    {:else if request.status === 'failed'}
        <!-- Failed State -->
        <div style="background: rgba(239,68,68,0.1); border: 1px solid rgba(239,68,68,0.3); border-radius: 12px; padding: 2rem; text-align: center;">
            <AlertCircleIcon size={48} style="color: #ef4444; margin-bottom: 1rem;" />
            <h2 style="color: {textPrimary}; font-size: 1.25rem; margin-bottom: 0.5rem;">Review Failed</h2>
            <p style="color: {textSecondary}; margin-bottom: 0.5rem;">{request.error_message}</p>
        </div>
    {:else if request.status === 'stopped'}
        <!-- Stopped State -->
        <div style="background: rgba(245,158,11,0.1); border: 1px solid rgba(245,158,11,0.3); border-radius: 12px; padding: 2rem; text-align: center;">
            <XIcon size={48} style="color: #f59e0b; margin-bottom: 1rem;" />
            <h2 style="color: {textPrimary}; font-size: 1.25rem; margin-bottom: 0.5rem;">Review Stopped</h2>
            <p style="color: {textSecondary};">The review was cancelled before completion.</p>
        </div>
    {:else}
        <!-- Completed State -->
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
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; gap: 1rem;">
                    <h2 style="font-size: 1.1rem; font-weight: 600;">Full Report</h2>
                    <div style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; justify-content: flex-end;">
                        <!-- 1. Open Report (highlighted) -->
                        <a href={`/home3/doc-review-report/${linkReportId}`} target="_blank"
                            style="padding: 0.4rem 0.85rem; background: {accent}; color: #fff; border-radius: 6px; text-decoration: none; font-size: 0.85rem; font-weight: 600; box-shadow: 0 0 0 2px {accentTint}, 0 2px 8px rgba(0,0,0,0.35);">
                            Open Report
                        </a>
                        <!-- 2. View Full Report PDF (dropdown listing all PDFs for this document) -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div style="position: relative;" onmouseleave={closePDFMenu}>
                            <button type="button" onclick={togglePDFMenu}
                                style="padding: 0.4rem 0.75rem; background: {pdfMenuOpen ? accent : accentTint}; color: {pdfMenuOpen ? '#fff' : accent}; border: none; border-radius: 6px; font-size: 0.85rem; cursor: pointer; display: flex; align-items: center; gap: 0.3rem;">
                                View Full Report PDF
                                <span style="font-size: 0.7rem; opacity: 0.8;">{pdfMenuOpen ? '▲' : '▼'}</span>
                            </button>
                            {#if pdfMenuOpen}
                                <div style="position: absolute; top: calc(100% + 4px); right: 0; z-index: 50; min-width: 320px; background: {cardBg}; border: 1px solid {borderColor}; border-radius: 8px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); overflow: hidden;">
                                    {#if pdfLoading}
                                        <div style="padding: 0.75rem 1rem; color: {textMuted}; font-size: 0.85rem;">Loading…</div>
                                    {:else if pdfFiles.length === 0}
                                        <div style="padding: 0.75rem 1rem; color: {textMuted}; font-size: 0.85rem;">No PDF files found. Regenerate the report to produce one.</div>
                                    {:else}
                                        <div style="padding: 0.4rem 1rem; font-size: 0.75rem; color: {textMuted}; border-bottom: 1px solid {borderColor}; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase;">
                                            {pdfFiles.length} Report PDF{pdfFiles.length !== 1 ? 's' : ''}
                                        </div>
                                        {#each pdfFiles as f (f.report_id)}
                                            <a href={`/api/v1/doc-review/reports/${f.report_id}/export?format=pdf`} target="_blank"
                                                onclick={closePDFMenu}
                                                style="display: flex; align-items: center; gap: 0.6rem; padding: 0.6rem 1rem; color: {f.is_current ? accent : textPrimary}; text-decoration: none; font-size: 0.85rem; background: {f.is_current ? accentTint : 'transparent'}; border-bottom: 1px solid {borderColor};">
                                                <span style="flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title={f.file_name}>{f.file_name}</span>
                                                {#if f.is_current}
                                                    <span style="font-size: 0.7rem; font-weight: 700; color: {accent}; flex-shrink: 0;">current</span>
                                                {/if}
                                                <span style="font-size: 0.72rem; color: {textMuted}; flex-shrink: 0;">{f.create_time.slice(0, 10)}</span>
                                            </a>
                                        {/each}
                                    {/if}
                                </div>
                            {/if}
                        </div>
                        <!-- 3. View Full Report JSON -->
                        <a href={`/api/v1/doc-review/reports/${linkReportId}`} target="_blank"
                            style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border-radius: 6px; text-decoration: none; font-size: 0.85rem;">
                            View Full Report JSON
                        </a>
                        <!-- 4. Export Markdown -->
                        <a href={`/api/v1/doc-review/reports/${linkReportId}/export?format=md`} target="_blank"
                            style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border-radius: 6px; text-decoration: none; font-size: 0.85rem;">
                            Export Markdown
                        </a>
                        <!-- 5. Review History (placeholder) -->
                        <button type="button"
                            style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border: none; border-radius: 6px; font-size: 0.85rem; cursor: pointer;">
                            Review History
                        </button>
                    </div>
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
            </div>
        {/if}
    {/if}
    </div>
{/if}

<style>
    @keyframes spin { to { transform: rotate(360deg); } }
    @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
</style>
