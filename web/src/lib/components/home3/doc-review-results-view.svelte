<script lang="ts">
    import { onMount, onDestroy } from 'svelte';
    import { getRequest, restartRequest, updateFinding, stopRequest, translateFinding } from '$lib/services/docReviewService';
    import type { RequestStatus, FindingItem, AspectStatus, ReviewPackageInfo } from '$lib/services/docReviewService';
    import { getKbFrontendConfig } from '$lib/services/kbService';
    import LoaderIcon from '@lucide/svelte/icons/loader';
    import XIcon from '@lucide/svelte/icons/x';
    import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
    import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
    import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
    import JsonTreeViewer from './json-tree-viewer.svelte';
    import { marked } from 'marked';

    let { darkMode = true, requestId = 0, runId = 0, reportId = 0, docTitle = '', onNewReview }:
        { darkMode: boolean; requestId: number; runId?: number; reportId?: number; docTitle?: string; onNewReview?: () => void } = $props();

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

    type DialogCopy = {
        title: string;
        noTranslation: (lang: string) => string;
        cancel: string;
        translate: string;
        translating: (lang: string) => string;
    };

    const DIALOG_COPY: Record<string, DialogCopy> = {
        en: {
            title: 'Translate finding',
            noTranslation: (lang) => `No ${lang} translation exists yet. Translate this finding now?`,
            cancel: 'Cancel',
            translate: 'Translate',
            translating: (lang) => `Translating to ${lang}…`
        },
        'zh-cn': {
            title: '翻译发现项',
            noTranslation: (lang) => `尚无 ${lang} 翻译版本。是否现在翻译？`,
            cancel: '取消',
            translate: '翻译',
            translating: (lang) => `正在翻译为 ${lang}…`
        },
        ja: {
            title: '所見を翻訳',
            noTranslation: (lang) => `${lang} の翻訳はまだありません。今すぐ翻訳しますか？`,
            cancel: 'キャンセル',
            translate: '翻訳',
            translating: (lang) => `${lang} に翻訳中…`
        },
        ko: {
            title: '결과 번역',
            noTranslation: (lang) => `아직 ${lang} 번역이 없습니다. 지금 번역하시겠습니까?`,
            cancel: '취소',
            translate: '번역',
            translating: (lang) => `${lang}(으)로 번역 중…`
        }
    };

    function dialogCopyFor(lang: string): DialogCopy {
        return DIALOG_COPY[lang] ?? DIALOG_COPY.en;
    }

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
    let packages = $state<ReviewPackageInfo[]>([]);
    let supportedLanguages = $state<string[]>(['en']);
    let defaultLanguage = $state('en');
    let findingLanguage = $state<Record<number, string>>({});
    let translating = $state<Record<number, boolean>>({});
    let pendingConfirm = $state<{ id: number; language: string } | null>(null);
    let translateError = $state<Record<number, string>>({});
    let autoChecked = $state<Set<number>>(new Set());

    // JSON viewer modal state
    let jsonModalOpen = $state(false);
    let jsonModalData = $state<unknown>(null);
    let jsonModalLoading = $state(false);

    // Markdown viewer modal state
    let mdModalOpen = $state(false);
    let mdModalHtml = $state('');
    let mdModalLoading = $state(false);

    // Rerun dialog state
    let rerunDialogOpen = $state(false);
    let isRerunning = $state(false);
    let currentRunId = $state(runId);
    let currentReportId = $state(reportId);

    async function openJsonModal() {
        jsonModalOpen = true;
        if (jsonModalData !== null) return;
        jsonModalLoading = true;
        try {
            const res = await fetch(`/api/v1/doc-review/reports/${linkReportId}`, { credentials: 'same-origin' });
            const data = await res.json();
            jsonModalData = data;
        } catch (e: any) {
            jsonModalData = { error: e.message };
        }
        jsonModalLoading = false;
    }

    function closeJsonModal() { jsonModalOpen = false; }

    async function openMdModal() {
        mdModalOpen = true;
        if (mdModalHtml) return;
        mdModalLoading = true;
        try {
            const res = await fetch(`/api/v1/doc-review/reports/${linkReportId}/export?format=md`, { credentials: 'same-origin' });
            const text = await res.text();
            mdModalHtml = await marked.parse(text);
        } catch (e: any) {
            mdModalHtml = `<p style="color:#ef4444">Failed to load markdown: ${e.message}</p>`;
        }
        mdModalLoading = false;
    }

    function closeMdModal() { mdModalOpen = false; }

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

    // Chart helpers
    const CHART_COLORS = ['#818CF8','#34D399','#F59E0B','#EF4444','#60A5FA','#A78BFA','#FB7185','#FBBF24','#4ADE80','#38BDF8'];

    function buildDonutSlices(items: { count: number; color: string; label: string }[], r: number) {
        const total = items.reduce((s, i) => s + i.count, 0);
        if (total === 0) return [];
        const C = 2 * Math.PI * r;
        let cumAngle = -90;
        return items.filter(i => i.count > 0).map(item => {
            const fraction = item.count / total;
            const dashLen = fraction * C;
            const startAngle = cumAngle;
            cumAngle += fraction * 360;
            return { color: item.color, label: item.label, count: item.count, strokeDasharray: `${dashLen} ${C}`, transform: `rotate(${startAngle}, 50, 50)` };
        });
    }

    function splitLabel(label: string, maxLen = 18): string[] {
        if (label.length <= maxLen) return [label];
        const mid = label.lastIndexOf(' ', maxLen);
        if (mid > maxLen / 3) {
            const rest = label.slice(mid + 1);
            return [label.slice(0, mid), rest.length > maxLen ? rest.slice(0, maxLen - 1) + '…' : rest];
        }
        const rest = label.slice(maxLen);
        return [label.slice(0, maxLen), rest.length > maxLen ? rest.slice(0, maxLen - 1) + '…' : rest];
    }

    // Selection & hover state
    let deselectedSeverities = $state(new Set<string>());
    let deselectedPackages = $state(new Set<string>());
    let deselectedReviewers = $state(new Set<string>());
    let hoverSev = $state<{ label: string; count: number; color: string } | null>(null);
    let hoverPkg = $state<{ label: string; count: number; color: string } | null>(null);
    let hoverRev = $state<{ label: string; count: number; color: string } | null>(null);

    function toggleSeverity(label: string) {
        const next = new Set(deselectedSeverities);
        if (next.has(label)) next.delete(label); else next.add(label);
        deselectedSeverities = next;
    }
    function togglePackage(key: string) {
        const next = new Set(deselectedPackages);
        if (next.has(key)) next.delete(key); else next.add(key);
        deselectedPackages = next;
    }
    function toggleReviewer(aspect: string) {
        const next = new Set(deselectedReviewers);
        if (next.has(aspect)) next.delete(aspect); else next.add(aspect);
        deselectedReviewers = next;
    }

    // Severity chart
    let severityItems = $derived(
        [
            { label: 'High', count: highCount, color: '#ef4444' },
            { label: 'Medium', count: mediumCount, color: '#f59e0b' },
            { label: 'Low', count: lowCount, color: '#22c55e' },
        ].sort((a, b) => b.count - a.count)
    );
    let severitySlices = $derived(
        buildDonutSlices(severityItems.filter(i => !deselectedSeverities.has(i.label)), 35)
    );

    // Package chart
    let packageChartData = $derived.by(() => {
        const countByPass = new Map<string, number>();
        for (const f of findings) countByPass.set(f.pass, (countByPass.get(f.pass) ?? 0) + 1);
        const sortedKeys = [...countByPass.keys()].sort();
        const nonEmpty = [...countByPass.entries()]
            .map(([key, count]) => ({
                key, label: packages.find(p => p.key === key)?.label ?? key, count,
                color: CHART_COLORS[sortedKeys.indexOf(key) % CHART_COLORS.length],
            }))
            .sort((a, b) => b.count - a.count);
        const emptyLabels = packages.filter(p => !countByPass.has(p.key)).map(p => p.label || p.key);
        const activeItems = nonEmpty.filter(p => !deselectedPackages.has(p.key));
        return { slices: buildDonutSlices(activeItems.map(p => ({ count: p.count, color: p.color, label: p.label })), 35), nonEmpty, emptyLabels };
    });

    // Reviewer chart
    let reviewerChartData = $derived.by(() => {
        const countByAspect = new Map<string, number>();
        for (const f of findings) countByAspect.set(f.aspect, (countByAspect.get(f.aspect) ?? 0) + 1);
        const sortedAspects = [...countByAspect.keys()].sort();
        const nonEmpty = [...countByAspect.entries()]
            .map(([aspect, count]) => ({
                aspect, label: aspect.replace(/_/g, ' '), count,
                color: CHART_COLORS[sortedAspects.indexOf(aspect) % CHART_COLORS.length],
            }))
            .sort((a, b) => b.count - a.count);
        const emptyLabels = aspectStatuses.filter(s => s.finding_count === 0).map(s => s.aspect.replace(/_/g, ' '));
        const activeItems = nonEmpty.filter(r => !deselectedReviewers.has(r.aspect));
        return { slices: buildDonutSlices(activeItems.map(r => ({ count: r.count, color: r.color, label: r.label })), 35), nonEmpty, emptyLabels };
    });

    // Elapsed time
    let elapsedSeconds = $derived.by(() => {
        if (!request?.start_time || !request?.end_time) return null;
        return Math.round((new Date(request.end_time).getTime() - new Date(request.start_time).getTime()) / 1000);
    });

    let nonEmptyPackageCount = $derived(packageChartData.nonEmpty.length);
    let nonEmptyReviewerCount = $derived(reviewerChartData.nonEmpty.length);

    // Effective report ID for links (fall back to requestId)
    // Report id may arrive via prop (legacy) or via the polled request after an
    // async run completes (DR15).
    let resolvedReportId = $derived(currentReportId || request?.report_id || 0);
    let linkReportId = $derived(resolvedReportId || requestId);
    let viewStatus = $derived(request?.run_status || request?.status || '');

    // Polling
    let pollTimer: ReturnType<typeof setTimeout>;
    let isActive = $state(true);

    async function pollStatus() {
        if (!isActive) return;
        try {
            const query = currentRunId ? { runId: currentRunId } : {};
            const result = await getRequest(requestId, query);
            request = result.request;
            findings = result.findings || [];
            aspectStatuses = result.aspect_statuses || [];
            packages = result.packages || [];
            if (request?.latest_run_id) currentRunId = request.latest_run_id;
            if (request?.report_id) currentReportId = request.report_id;

            if (viewStatus === 'completed' || viewStatus === 'failed' || viewStatus === 'stopped') {
                isActive = false;
                stopPolling();
                const rid = currentReportId || request.report_id;
                if (viewStatus === 'completed' && rid) {
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
            stopPolling();
        }
    }

    let intervalId: ReturnType<typeof setInterval> | undefined;

    function startPolling() {
        clearInterval(intervalId);
        intervalId = setInterval(pollStatus, 3000);
    }

    function stopPolling() {
        clearInterval(intervalId);
        intervalId = undefined;
    }

    onMount(async () => {
        await pollStatus();
        if (isActive) {
            startPolling();
        }
        try {
            const config = await getKbFrontendConfig();
            supportedLanguages = config.supported_languages?.length ? config.supported_languages : ['en'];
            defaultLanguage = config.default_language?.[0] ?? 'en';
        } catch {
            supportedLanguages = ['en'];
            defaultLanguage = 'en';
        }
    });

    onDestroy(() => {
        stopPolling();
    });

    async function handleAcceptReject(findingId: number, status: string) {
        try {
            await updateFinding(findingId, status);
            findings = findings.map(f => f.id === findingId ? { ...f, review_status: status } : f);
        } catch (e: any) {
            error = e.message;
        }
    }

    function applyTranslatedFinding(id: number, translated: FindingItem, language: string) {
        translateError = { ...translateError, [id]: '' };
        findings = findings.map(f =>
            f.id === id
                ? { ...f, title: translated.title, description: translated.description, suggestion: translated.suggestion }
                : f
        );
        findingLanguage = { ...findingLanguage, [id]: language };
    }

    async function handleLanguageChange(finding: FindingItem, newLanguage: string) {
        const previous = findingLanguage[finding.id] ?? defaultLanguage;
        translating = { ...translating, [finding.id]: true };
        try {
            const resp = await translateFinding(finding.id, newLanguage, false);
            if (resp.needs_confirmation) {
                pendingConfirm = { id: finding.id, language: newLanguage };
                return;
            }
            applyTranslatedFinding(finding.id, resp.finding, newLanguage);
        } catch (e: any) {
            translateError = { ...translateError, [finding.id]: e.message };
            findingLanguage = { ...findingLanguage, [finding.id]: previous };
        } finally {
            translating = { ...translating, [finding.id]: false };
        }
    }

    async function confirmTranslate(finding: FindingItem) {
        if (!pendingConfirm || pendingConfirm.id !== finding.id) return;
        const language = pendingConfirm.language;
        translating = { ...translating, [finding.id]: true };
        try {
            const resp = await translateFinding(finding.id, language, true);
            applyTranslatedFinding(finding.id, resp.finding, language);
        } catch (e: any) {
            translateError = { ...translateError, [finding.id]: e.message };
        } finally {
            translating = { ...translating, [finding.id]: false };
            pendingConfirm = null;
        }
    }

    function cancelTranslate() {
        pendingConfirm = null;
    }

    function handlePageLanguageChange(newLanguage: string) {
        defaultLanguage = newLanguage;
        findingLanguage = {};
        pendingConfirm = null;
        translateError = {};
        autoChecked = new Set(expandedFindings);
        for (const id of expandedFindings) {
            const finding = findings.find(f => f.id === id);
            if (finding) handleLanguageChange(finding, newLanguage);
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

    function openRerunDialog() {
        rerunDialogOpen = true;
    }

    function closeRerunDialog() {
        if (isRerunning) return;
        rerunDialogOpen = false;
    }

    async function confirmRerun() {
        if (!request || isRerunning) return;
        isRerunning = true;
        error = '';
        try {
            const restarted = await restartRequest(requestId);
            rerunDialogOpen = false;
            stopPolling();
            isActive = true;
            activeTab = 'findings';
            currentRunId = restarted.runId;
            currentReportId = 0;
            request = {
                ...request,
                status: 'accepted',
                run_status: 'pending',
                latest_run_id: restarted.runId,
                report_id: 0,
                start_time: undefined,
                end_time: undefined,
                error_message: undefined,
            };
            findings = [];
            aspectStatuses = [];
            packages = [];
            reportData = null;
            pdfMenuOpen = false;
            jsonModalOpen = false;
            mdModalOpen = false;
            await pollStatus();
            if (isActive) {
                startPolling();
            }
        } catch (e: any) {
            error = e.message;
        }
        isRerunning = false;
    }

    function toggleFinding(id: number) {
        const next = new Set(expandedFindings);
        if (next.has(id)) {
            next.delete(id);
        } else {
            next.add(id);
            if (!autoChecked.has(id)) {
                autoChecked = new Set(autoChecked).add(id);
                const finding = findings.find(f => f.id === id);
                if (finding) handleLanguageChange(finding, defaultLanguage);
            }
        }
        expandedFindings = next;
    }

    function progressPercent(progress?: number): number {
        const value = Number.isFinite(progress) ? progress ?? 0 : 0;
        return Math.max(0, Math.min(100, Math.round(value * 100)));
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
            <label style="display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.8rem; color: {textMuted};">
                <span>Language</span>
                <select
                    value={defaultLanguage}
                    onchange={(e) => handlePageLanguageChange((e.target as HTMLSelectElement).value)}
                    style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 6px; padding: 0.25rem 0.5rem; color: {textPrimary}; font-size: 0.8rem;">
                    {#each supportedLanguages as lang (lang)}
                        <option value={lang}>{lang}</option>
                    {/each}
                </select>
            </label>
            <span style="margin-left: auto; font-size: 0.8rem; font-weight: 600; padding: 0.2rem 0.6rem; border-radius: 6px; background: {accentTint}; color: {accent}; text-transform: capitalize;">{viewStatus}</span>
            {#if viewStatus === 'accepted' || viewStatus === 'pending' || viewStatus === 'running'}
                <button onclick={handleStop} disabled={isStopping}
                    style="padding: 0.4rem 0.9rem; background: rgba(239,68,68,0.12); color: #ef4444; border: 1px solid rgba(239,68,68,0.3); border-radius: 8px; cursor: pointer; font-size: 0.85rem;">{isStopping ? 'Stopping…' : 'Stop'}</button>
            {/if}
        </div>

        {#if error}
            <div style="margin-bottom: 1rem; padding: 0.75rem 1rem; background: rgba(239,68,68,0.1); border: 1px solid rgba(239,68,68,0.25); border-radius: 10px; color: #fca5a5; font-size: 0.9rem;">
                {error}
            </div>
        {/if}


    {#if viewStatus === 'accepted' || viewStatus === 'pending' || viewStatus === 'running'}
        <!-- Running: the Active Reviews monitor on the form conveys live progress -->
        <div style="display: flex; align-items: center; gap: 0.5rem; color: {textSecondary}; font-size: 0.9rem; padding: 1.5rem 0.25rem;">
            <LoaderIcon size={18} style="animation: spin 1s linear infinite; color: {accent};" />
            Reviewing the document… findings will appear here as soon as the job completes.
        </div>
    {:else if viewStatus === 'failed'}
        <!-- Failed State -->
        <div style="background: rgba(239,68,68,0.1); border: 1px solid rgba(239,68,68,0.3); border-radius: 12px; padding: 2rem; text-align: center;">
            <AlertCircleIcon size={48} style="color: #ef4444; margin-bottom: 1rem;" />
            <h2 style="color: {textPrimary}; font-size: 1.25rem; margin-bottom: 0.5rem;">Review Failed</h2>
            <p style="color: {textSecondary}; margin-bottom: 0.5rem;">{request.error_message}</p>
        </div>
    {:else if viewStatus === 'stopped'}
        <!-- Stopped State -->
        <div style="background: rgba(245,158,11,0.1); border: 1px solid rgba(245,158,11,0.3); border-radius: 12px; padding: 2rem; text-align: center;">
            <XIcon size={48} style="color: #f59e0b; margin-bottom: 1rem;" />
            <h2 style="color: {textPrimary}; font-size: 1.25rem; margin-bottom: 0.5rem;">Review Stopped</h2>
            <p style="color: {textSecondary};">The review was cancelled before completion.</p>
        </div>
    {:else}
        <!-- Completed State -->
        <!-- Header -->
        <div style="margin-bottom: 1.5rem;">
            <h1 style="font-size: 1.5rem; font-weight: 700; color: {textPrimary};">Review Results</h1>
            <p style="color: {textSecondary}; font-size: 0.85rem;">{findings.length} findings · {request.tier}</p>
        </div>

        <!-- Summary Charts -->
        <div style="display: grid; grid-template-columns: repeat(2, 1fr); gap: 1rem; margin-bottom: 1.5rem;">

            <!-- Severity Distribution -->
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1rem;">
                <div style="font-size: 0.72rem; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 0.75rem;">Severity Distribution</div>
                <div style="display: flex; align-items: center; gap: 1.25rem;">
                    <svg viewBox="0 0 100 100" width="304" height="304" style="flex-shrink: 0;">
                        <circle cx="50" cy="50" r="35" fill="none" stroke="{borderColor}" stroke-width="16" style="pointer-events: none;"/>
                        {#each severitySlices as s}
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <circle cx="50" cy="50" r="35" fill="none" stroke="{s.color}" stroke-width="16"
                                stroke-dasharray="{s.strokeDasharray}" transform="{s.transform}" style="cursor: pointer;"
                                onmouseenter={() => hoverSev = { label: s.label, count: s.count, color: s.color }}
                                onmouseleave={() => hoverSev = null}/>
                        {/each}
                        {#if hoverSev}
                            <text x="50" y="44" text-anchor="middle" dominant-baseline="central" font-size="9" font-weight="800" fill="{hoverSev.color}" style="pointer-events: none;">{hoverSev.count}</text>
                            <text x="50" y="57" text-anchor="middle" dominant-baseline="central" font-size="4.5" fill="{textSecondary}" style="pointer-events: none;">{hoverSev.label}</text>
                        {/if}
                    </svg>
                    <div style="display: flex; flex-direction: column; gap: 0.55rem; flex: 1; min-width: 0;">
                        {#each severityItems as item}
                            {@const isOff = deselectedSeverities.has(item.label)}
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <div onclick={() => toggleSeverity(item.label)} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleSeverity(item.label); }}
                                role="button" tabindex="0"
                                style="display: flex; align-items: center; gap: 0.5rem; font-size: 0.85rem; cursor: pointer; user-select: none; opacity: {isOff ? 0.35 : 1}; transition: opacity 0.15s;">
                                <span style="width: 12px; height: 12px; border-radius: 2px; background: {isOff ? textMuted : item.color}; flex-shrink: 0; transition: background 0.15s;"></span>
                                <span style="color: {textSecondary}; flex: 1; text-decoration: {isOff ? 'line-through' : 'none'};">{item.label}</span>
                                <span style="font-weight: 700; color: {isOff ? textMuted : item.color};">{item.count}</span>
                            </div>
                        {/each}
                    </div>
                </div>
            </div>

            <!-- By Package -->
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1rem;">
                <div style="font-size: 0.72rem; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 0.75rem;">By Package</div>
                <div style="display: flex; align-items: center; gap: 1.25rem;">
                    <svg viewBox="0 0 100 100" width="304" height="304" style="flex-shrink: 0;">
                        <circle cx="50" cy="50" r="35" fill="none" stroke="{borderColor}" stroke-width="16" style="pointer-events: none;"/>
                        {#each packageChartData.slices as s}
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <circle cx="50" cy="50" r="35" fill="none" stroke="{s.color}" stroke-width="16"
                                stroke-dasharray="{s.strokeDasharray}" transform="{s.transform}" style="cursor: pointer;"
                                onmouseenter={() => hoverPkg = { label: s.label, count: s.count, color: s.color }}
                                onmouseleave={() => hoverPkg = null}/>
                        {/each}
                        {#if hoverPkg}
                            {@const pkgParts = splitLabel(hoverPkg.label)}
                            <text x="50" y="{pkgParts.length > 1 ? '40' : '44'}" text-anchor="middle" dominant-baseline="central" font-size="9" font-weight="800" fill="{hoverPkg.color}" style="pointer-events: none;">{hoverPkg.count}</text>
                            <text x="50" y="{pkgParts.length > 1 ? '51' : '57'}" text-anchor="middle" dominant-baseline="central" font-size="4.5" fill="{textSecondary}" style="pointer-events: none;">{pkgParts[0]}</text>
                            {#if pkgParts.length > 1}
                                <text x="50" y="58" text-anchor="middle" dominant-baseline="central" font-size="4.5" fill="{textSecondary}" style="pointer-events: none;">{pkgParts[1]}</text>
                            {/if}
                        {/if}
                    </svg>
                    <div style="flex: 1; min-width: 0; overflow-y: auto; max-height: 280px;">
                        {#each packageChartData.nonEmpty as item}
                            {@const isOff = deselectedPackages.has(item.key)}
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <div onclick={() => togglePackage(item.key)} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') togglePackage(item.key); }}
                                role="button" tabindex="0"
                                style="display: flex; align-items: center; gap: 0.45rem; font-size: 0.8rem; margin-bottom: 0.35rem; cursor: pointer; user-select: none; opacity: {isOff ? 0.35 : 1}; transition: opacity 0.15s;">
                                <span style="width: 9px; height: 9px; border-radius: 2px; background: {isOff ? textMuted : item.color}; flex-shrink: 0; transition: background 0.15s;"></span>
                                <span style="color: {textSecondary}; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-decoration: {isOff ? 'line-through' : 'none'};">{item.label}</span>
                                <span style="font-weight: 600; color: {isOff ? textMuted : textPrimary};">{item.count}</span>
                            </div>
                        {/each}
                        {#if packageChartData.emptyLabels.length > 0}
                            <div style="margin-top: 0.4rem; padding-top: 0.4rem; border-top: 1px solid {borderColor}; font-size: 0.72rem; color: {textMuted}; line-height: 1.5;">
                                No findings: {packageChartData.emptyLabels.join(', ')}
                            </div>
                        {/if}
                    </div>
                </div>
            </div>

            <!-- By Reviewer -->
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1rem;">
                <div style="font-size: 0.72rem; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 0.75rem;">By Reviewer</div>
                <div style="display: flex; align-items: center; gap: 1.25rem;">
                    <svg viewBox="0 0 100 100" width="304" height="304" style="flex-shrink: 0;">
                        <circle cx="50" cy="50" r="35" fill="none" stroke="{borderColor}" stroke-width="16" style="pointer-events: none;"/>
                        {#each reviewerChartData.slices as s}
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <circle cx="50" cy="50" r="35" fill="none" stroke="{s.color}" stroke-width="16"
                                stroke-dasharray="{s.strokeDasharray}" transform="{s.transform}" style="cursor: pointer;"
                                onmouseenter={() => hoverRev = { label: s.label, count: s.count, color: s.color }}
                                onmouseleave={() => hoverRev = null}/>
                        {/each}
                        {#if hoverRev}
                            {@const revParts = splitLabel(hoverRev.label)}
                            <text x="50" y="{revParts.length > 1 ? '40' : '44'}" text-anchor="middle" dominant-baseline="central" font-size="9" font-weight="800" fill="{hoverRev.color}" style="pointer-events: none;">{hoverRev.count}</text>
                            <text x="50" y="{revParts.length > 1 ? '51' : '57'}" text-anchor="middle" dominant-baseline="central" font-size="4.5" fill="{textSecondary}" style="pointer-events: none;">{revParts[0]}</text>
                            {#if revParts.length > 1}
                                <text x="50" y="58" text-anchor="middle" dominant-baseline="central" font-size="4.5" fill="{textSecondary}" style="pointer-events: none;">{revParts[1]}</text>
                            {/if}
                        {/if}
                    </svg>
                    <div style="flex: 1; min-width: 0; overflow-y: auto; max-height: 280px;">
                        {#each reviewerChartData.nonEmpty as item}
                            {@const isOff = deselectedReviewers.has(item.aspect)}
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <div onclick={() => toggleReviewer(item.aspect)} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleReviewer(item.aspect); }}
                                role="button" tabindex="0"
                                style="display: flex; align-items: center; gap: 0.45rem; font-size: 0.8rem; margin-bottom: 0.35rem; cursor: pointer; user-select: none; opacity: {isOff ? 0.35 : 1}; transition: opacity 0.15s;">
                                <span style="width: 9px; height: 9px; border-radius: 2px; background: {isOff ? textMuted : item.color}; flex-shrink: 0; transition: background 0.15s;"></span>
                                <span style="color: {textSecondary}; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-decoration: {isOff ? 'line-through' : 'none'};">{item.label}</span>
                                <span style="font-weight: 600; color: {isOff ? textMuted : textPrimary};">{item.count}</span>
                            </div>
                        {/each}
                        {#if reviewerChartData.emptyLabels.length > 0}
                            <div style="margin-top: 0.4rem; padding-top: 0.4rem; border-top: 1px solid {borderColor}; font-size: 0.72rem; color: {textMuted}; line-height: 1.5;">
                                No findings: {reviewerChartData.emptyLabels.join(', ')}
                            </div>
                        {/if}
                    </div>
                </div>
            </div>

            <!-- Review Metadata -->
            <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px; padding: 1rem;">
                <div style="font-size: 0.72rem; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 0.75rem;">Review Metadata</div>
                <div style="display: flex; flex-direction: column; gap: 0.5rem;">
                    <div style="display: flex; justify-content: space-between; align-items: baseline; font-size: 0.82rem; gap: 0.5rem;">
                        <span style="color: {textSecondary};">Start Time</span>
                        <span style="color: {textPrimary}; font-weight: 500; font-family: monospace; font-size: 0.78rem;">{request.start_time ? new Date(request.start_time).toLocaleString() : '—'}</span>
                    </div>
                    <div style="display: flex; justify-content: space-between; align-items: baseline; font-size: 0.82rem; gap: 0.5rem;">
                        <span style="color: {textSecondary};">Time Used</span>
                        <span style="color: {textPrimary}; font-weight: 600; font-family: monospace;">{elapsedSeconds !== null ? `${elapsedSeconds}s` : '—'}</span>
                    </div>
                    <div style="display: flex; justify-content: space-between; align-items: baseline; font-size: 0.82rem; gap: 0.5rem;">
                        <span style="color: {textSecondary};">Total Findings</span>
                        <span style="color: {textPrimary}; font-weight: 600; font-family: monospace;">{findings.length}</span>
                    </div>
                    <div style="display: flex; justify-content: space-between; align-items: baseline; font-size: 0.82rem; gap: 0.5rem;">
                        <span style="color: {textSecondary};">Non-Empty Packages</span>
                        <span style="color: {textPrimary}; font-weight: 600; font-family: monospace;">{nonEmptyPackageCount}</span>
                    </div>
                    <div style="display: flex; justify-content: space-between; align-items: baseline; font-size: 0.82rem; gap: 0.5rem;">
                        <span style="color: {textSecondary};">Non-Empty Reviewers</span>
                        <span style="color: {textPrimary}; font-weight: 600; font-family: monospace;">{nonEmptyReviewerCount}</span>
                    </div>
                </div>
            </div>

        </div>

        <!-- Tab switcher -->
        <div style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1rem; flex-wrap: wrap;">
            <button onclick={() => activeTab = 'findings'}
                style="padding: 0.4rem 1rem; background: {activeTab === 'findings' ? accentTint : 'transparent'}; color: {activeTab === 'findings' ? accent : textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">Findings</button>
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
                    <div style="position: absolute; top: calc(100% + 4px); left: 0; z-index: 50; min-width: 320px; background: {cardBg}; border: 1px solid {borderColor}; border-radius: 8px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); overflow: hidden;">
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
            <button type="button" onclick={openJsonModal}
                style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border: none; border-radius: 6px; font-size: 0.85rem; cursor: pointer;">
                View Full Report JSON
            </button>
            <!-- 4. View Full Report Markdown -->
            <button type="button" onclick={openMdModal}
                style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border: none; border-radius: 6px; font-size: 0.85rem; cursor: pointer;">
                View Full Report Markdown
            </button>
            <!-- 5. Download Report PDF -->
            <a href={`/api/v1/doc-review/reports/${linkReportId}/export?format=pdf`} download
                style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border-radius: 6px; text-decoration: none; font-size: 0.85rem;">
                Download Report PDF
            </a>
            <!-- 6. Download Report Markdown -->
            <a href={`/api/v1/doc-review/reports/${linkReportId}/export?format=md`} download
                style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border-radius: 6px; text-decoration: none; font-size: 0.85rem;">
                Download Report Markdown
            </a>
            <!-- 7. Review History (placeholder) -->
            <button type="button"
                style="padding: 0.4rem 0.75rem; background: {accentTint}; color: {accent}; border: none; border-radius: 6px; font-size: 0.85rem; cursor: pointer;">
                Review History
            </button>
            <button onclick={openRerunDialog} disabled={isRerunning}
                style="padding: 0.4rem 1rem; background: transparent; color: {isRerunning ? textMuted : textSecondary}; border: 1px solid {borderColor}; border-radius: 8px; cursor: {isRerunning ? 'default' : 'pointer'}; font-size: 0.85rem;">{isRerunning ? 'Rerunning…' : 'Rerun'}</button>
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
                                <select
                                    value={findingLanguage[finding.id] ?? defaultLanguage}
                                    disabled={translating[finding.id]}
                                    onclick={(e) => e.stopPropagation()}
                                    onchange={(e) => handleLanguageChange(finding, (e.target as HTMLSelectElement).value)}
                                    style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 4px; padding: 0.2rem 0.4rem; color: {textPrimary}; font-size: 0.75rem;">
                                    {#each supportedLanguages as lang (lang)}
                                        <option value={lang}>{lang}</option>
                                    {/each}
                                </select>
                                {#if translating[finding.id]}
                                    <LoaderIcon size={14} style="animation: spin 1s linear infinite; color: {accent};" />
                                {/if}
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
                                {#if translateError[finding.id]}
                                    <p style="margin-top: 0.5rem; color: #ef4444; font-size: 0.8rem;">Translation failed: {translateError[finding.id]}</p>
                                {/if}
                                <p style="color: {textSecondary}; font-size: 0.85rem; margin-top: 0.5rem;">{finding.description}</p>
                                {#if finding.evidence}
                                    <div style="margin-top: 0.5rem; padding: 0.5rem; background: {inputBg}; border-radius: 6px; font-size: 0.8rem; color: {textMuted}; font-family: monospace;">
                                        {finding.evidence}
                                    </div>
                                {/if}
                                {#if finding.suggestion}
                                    <p style="margin-top: 0.5rem; color: {accent}; font-size: 0.85rem;"><strong>Suggestion:</strong> {finding.suggestion}</p>
                                {/if}
                                <p style="margin-top: 0.25rem; color: {textMuted}; font-size: 0.75rem;">Confidence: {Math.round(finding.confidence * 100)}%</p>
                                {#if finding.location}
                                    <p style="margin-top: 0.25rem; color: {textMuted}; font-size: 0.8rem;">Location: {finding.location}</p>
                                {/if}
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

    {/if}
    </div>
{/if}

{#if rerunDialogOpen && request}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
        onclick={(e) => { if (e.target === e.currentTarget) closeRerunDialog(); }}
        onkeydown={(e) => { if (e.key === 'Escape') closeRerunDialog(); }}
        role="dialog"
        aria-modal="true"
        tabindex="-1"
        style="position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.55); backdrop-filter: blur(2px); padding: 1rem;"
    >
        <div style="background: {cardBg}; border: 1px solid {borderColor}; border-radius: 14px; width: min(100%, 720px); box-shadow: 0 24px 64px rgba(0,0,0,0.45); overflow: hidden;">
            <div style="padding: 1.25rem 1.5rem 0.75rem;">
                <h2 style="font-size: 1.15rem; font-weight: 700; color: {textPrimary}; margin: 0 0 0.35rem;">Review Summary</h2>
                <p style="color: {textSecondary}; font-size: 0.9rem; margin: 0;">This will re-run the saved request with the same review setup.</p>
            </div>
            <div style="padding: 0.5rem 1.5rem 1.25rem;">
                <div style="display: grid; grid-template-columns: minmax(140px, 180px) 1fr; gap: 0.65rem 1.25rem; padding: 1rem 1.1rem; background: {darkMode ? 'rgba(15,23,42,0.35)' : '#F8FAFC'}; border: 1px solid {borderColor}; border-radius: 12px;">
                    <span style="color: {textSecondary}; font-size: 0.9rem; font-weight: 600;">Document:</span>
                    <span style="color: {textPrimary}; font-size: 0.95rem;">{docTitle || `Document #${request.input_record_id}`}</span>
                    <span style="color: {textSecondary}; font-size: 0.9rem; font-weight: 600;">Check Level:</span>
                    <span style="color: {textPrimary}; font-size: 0.95rem;">{request.tier}</span>
                    <span style="color: {textSecondary}; font-size: 0.9rem; font-weight: 600;">Aspects:</span>
                    <span style="color: {textPrimary}; font-size: 0.95rem;">{request.aspects.length} selected</span>
                    <span style="color: {textSecondary}; font-size: 0.9rem; font-weight: 600;">Requester:</span>
                    <span style="color: {textPrimary}; font-size: 0.95rem;">{request.requester_name || 'Unknown requester'}</span>
                </div>
            </div>
            <div style="display: flex; gap: 0.75rem; padding: 0 1.5rem 1.5rem;">
                <button type="button" onclick={closeRerunDialog} disabled={isRerunning}
                    style="padding: 0.75rem 1.1rem; background: transparent; color: {textSecondary}; border: 1px solid {borderColor}; border-radius: 10px; cursor: {isRerunning ? 'default' : 'pointer'}; font-size: 0.95rem; min-width: 110px;">← Back</button>
                <button type="button" onclick={confirmRerun} disabled={isRerunning}
                    style="flex: 1; padding: 0.85rem 1.25rem; background: {accent}; color: #fff; border: none; border-radius: 10px; cursor: {isRerunning ? 'default' : 'pointer'}; font-size: 1rem; font-weight: 700; display: flex; align-items: center; justify-content: center; gap: 0.5rem;">
                    {#if isRerunning}
                        <LoaderIcon size={16} style="animation: spin 1s linear infinite;" />
                        Rerunning...
                    {:else}
                        Rerun Review
                    {/if}
                </button>
            </div>
        </div>
    </div>
{/if}

<!-- JSON Viewer Modal -->
{#if jsonModalOpen}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
        onclick={(e) => { if (e.target === e.currentTarget) closeJsonModal(); }}
        onkeydown={(e) => { if (e.key === 'Escape') closeJsonModal(); }}
        role="dialog"
        aria-modal="true"
        tabindex="-1"
        style="position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.6); backdrop-filter: blur(2px);"
    >
        <div style="background: {darkMode ? '#161B27' : '#FFFFFF'}; border: 1px solid {borderColor}; border-radius: 14px; width: min(90vw, 880px); max-height: 85vh; display: flex; flex-direction: column; box-shadow: 0 24px 64px rgba(0,0,0,0.5);">
            <!-- Modal header -->
            <div style="display: flex; align-items: center; justify-content: space-between; padding: 1rem 1.25rem; border-bottom: 1px solid {borderColor}; flex-shrink: 0;">
                <div>
                    <span style="font-weight: 600; font-size: 0.95rem; color: {textPrimary};">Full Report JSON</span>
                    <span style="margin-left: 0.6rem; font-size: 0.75rem; color: {textMuted}; font-family: monospace;">report #{linkReportId}</span>
                </div>
                <div style="display: flex; align-items: center; gap: 0.5rem;">
                    <a href={`/api/v1/doc-review/reports/${linkReportId}`} target="_blank"
                        style="padding: 0.3rem 0.65rem; background: {accentTint}; color: {accent}; border-radius: 6px; text-decoration: none; font-size: 0.8rem;">
                        Raw JSON ↗
                    </a>
                    <button type="button" onclick={closeJsonModal}
                        style="padding: 0.3rem 0.65rem; background: transparent; color: {textMuted}; border: 1px solid {borderColor}; border-radius: 6px; cursor: pointer; font-size: 0.8rem; line-height: 1;">✕</button>
                </div>
            </div>
            <!-- Modal body -->
            <div style="overflow: auto; padding: 1.25rem; font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Consolas', monospace; font-size: 0.82rem; line-height: 1.6; flex: 1;">
                {#if jsonModalLoading}
                    <div style="display: flex; align-items: center; gap: 0.5rem; color: {textSecondary}; padding: 2rem; justify-content: center;">
                        <LoaderIcon size={20} style="animation: spin 1s linear infinite;" />
                        Loading…
                    </div>
                {:else if jsonModalData !== null}
                    <JsonTreeViewer value={jsonModalData} depth={0} {darkMode} />
                {/if}
            </div>
        </div>
    </div>
{/if}

<!-- Markdown Viewer Modal -->
{#if mdModalOpen}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
        onclick={(e) => { if (e.target === e.currentTarget) closeMdModal(); }}
        onkeydown={(e) => { if (e.key === 'Escape') closeMdModal(); }}
        role="dialog"
        aria-modal="true"
        tabindex="-1"
        style="position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.6); backdrop-filter: blur(2px);"
    >
        <div style="background: {darkMode ? '#161B27' : '#FFFFFF'}; border: 1px solid {borderColor}; border-radius: 14px; width: min(90vw, 900px); max-height: 85vh; display: flex; flex-direction: column; box-shadow: 0 24px 64px rgba(0,0,0,0.5);">
            <!-- Modal header -->
            <div style="display: flex; align-items: center; justify-content: space-between; padding: 1rem 1.25rem; border-bottom: 1px solid {borderColor}; flex-shrink: 0;">
                <div>
                    <span style="font-weight: 600; font-size: 0.95rem; color: {textPrimary};">Full Report</span>
                    <span style="margin-left: 0.6rem; font-size: 0.75rem; color: {textMuted}; font-family: monospace;">report #{linkReportId}</span>
                </div>
                <div style="display: flex; align-items: center; gap: 0.5rem;">
                    <a href={`/api/v1/doc-review/reports/${linkReportId}/export?format=md`} target="_blank"
                        style="padding: 0.3rem 0.65rem; background: {accentTint}; color: {accent}; border-radius: 6px; text-decoration: none; font-size: 0.8rem;">
                        Raw Markdown ↗
                    </a>
                    <button type="button" onclick={closeMdModal}
                        style="padding: 0.3rem 0.65rem; background: transparent; color: {textMuted}; border: 1px solid {borderColor}; border-radius: 6px; cursor: pointer; font-size: 0.8rem; line-height: 1;">✕</button>
                </div>
            </div>
            <!-- Modal body -->
            <div class="md-body" style="overflow: auto; padding: 1.5rem 2rem; flex: 1; color: {textPrimary};">
                {#if mdModalLoading}
                    <div style="display: flex; align-items: center; gap: 0.5rem; color: {textSecondary}; padding: 2rem; justify-content: center;">
                        <LoaderIcon size={20} style="animation: spin 1s linear infinite;" />
                        Loading…
                    </div>
                {:else}
                    {@html mdModalHtml}
                {/if}
            </div>
        </div>
    </div>
{/if}

<!-- Translate Confirm Modal -->
{#if pendingConfirm}
    {@const confirmId = pendingConfirm.id}
    {@const pendingFinding = findings.find(f => f.id === confirmId)}
    {@const copy = dialogCopyFor(pendingConfirm.language)}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
        onclick={(e) => { if (e.target === e.currentTarget && !translating[confirmId]) cancelTranslate(); }}
        onkeydown={(e) => { if (e.key === 'Escape' && !translating[confirmId]) cancelTranslate(); }}
        role="dialog"
        aria-modal="true"
        tabindex="-1"
        style="position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.6); backdrop-filter: blur(2px);"
    >
        <div style="background: {darkMode ? '#161B27' : '#FFFFFF'}; border: 1px solid {borderColor}; border-radius: 14px; width: min(90vw, 420px); box-shadow: 0 24px 64px rgba(0,0,0,0.5); padding: 1.5rem;">
            <div style="font-weight: 600; font-size: 1rem; color: {textPrimary}; margin-bottom: 0.5rem;">{copy.title}</div>
            {#if pendingFinding}
                <div style="font-size: 0.85rem; color: {textSecondary}; margin-bottom: 1rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{pendingFinding.title}</div>
            {/if}
            {#if translating[pendingConfirm.id]}
                <div style="display: flex; align-items: center; gap: 0.6rem; color: {textSecondary}; font-size: 0.9rem; padding: 0.5rem 0;">
                    <LoaderIcon size={18} style="animation: spin 1s linear infinite; color: {accent};" />
                    {copy.translating(pendingConfirm.language)}
                </div>
            {:else}
                <div style="font-size: 0.9rem; color: {textPrimary}; margin-bottom: 1.25rem;">
                    {copy.noTranslation(pendingConfirm.language)}
                </div>
                <div style="display: flex; justify-content: flex-end; gap: 0.5rem;">
                    <button onclick={cancelTranslate}
                        style="padding: 0.4rem 0.9rem; background: transparent; color: {textMuted}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">{copy.cancel}</button>
                    <button onclick={() => pendingFinding && confirmTranslate(pendingFinding)}
                        style="padding: 0.4rem 0.9rem; background: {successBg}; color: #22c55e; border: none; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">{copy.translate}</button>
                </div>
            {/if}
        </div>
    </div>
{/if}

<style>
    @keyframes spin { to { transform: rotate(360deg); } }
    @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }

    .md-body :global(h1) { font-size: 1.6rem; font-weight: 700; margin: 1.2rem 0 0.6rem; }
    .md-body :global(h2) { font-size: 1.3rem; font-weight: 600; margin: 1rem 0 0.5rem; }
    .md-body :global(h3) { font-size: 1.1rem; font-weight: 600; margin: 0.9rem 0 0.4rem; }
    .md-body :global(p)  { line-height: 1.7; margin: 0.5rem 0; font-size: 0.9rem; }
    .md-body :global(ul), .md-body :global(ol) { padding-left: 1.5rem; margin: 0.5rem 0; font-size: 0.9rem; }
    .md-body :global(li) { margin: 0.2rem 0; line-height: 1.6; }
    .md-body :global(strong) { font-weight: 700; }
    .md-body :global(em) { font-style: italic; }
    .md-body :global(code) { font-family: 'JetBrains Mono', monospace; font-size: 0.82rem; background: rgba(99,102,241,0.12); padding: 0.1em 0.35em; border-radius: 4px; }
    .md-body :global(pre) { background: rgba(0,0,0,0.25); border-radius: 8px; padding: 1rem; overflow-x: auto; margin: 0.75rem 0; }
    .md-body :global(pre code) { background: transparent; padding: 0; font-size: 0.82rem; }
    .md-body :global(blockquote) { border-left: 3px solid rgba(129,140,248,0.5); padding-left: 1rem; margin: 0.5rem 0; opacity: 0.8; }
    .md-body :global(hr) { border: none; border-top: 1px solid rgba(255,255,255,0.1); margin: 1rem 0; }
    .md-body :global(table) { width: 100%; border-collapse: collapse; margin: 0.75rem 0; font-size: 0.85rem; }
    .md-body :global(th), .md-body :global(td) { padding: 0.45rem 0.75rem; border: 1px solid rgba(255,255,255,0.12); text-align: left; }
    .md-body :global(th) { font-weight: 600; background: rgba(99,102,241,0.12); }
</style>
