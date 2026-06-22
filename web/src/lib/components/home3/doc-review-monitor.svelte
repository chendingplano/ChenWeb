<script lang="ts">
    import { onMount, onDestroy } from 'svelte';
    import { listActiveJobs, listAspects, stopRequest } from '$lib/services/docReviewService';
    import type { ActiveJob, AspectStatus, AspectInfo } from '$lib/services/docReviewService';
    import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
    import XCircleIcon from '@lucide/svelte/icons/x-circle';
    import ClockIcon from '@lucide/svelte/icons/clock';
    import SquareIcon from '@lucide/svelte/icons/square';
    import EyeIcon from '@lucide/svelte/icons/eye';
    import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

    let {
        darkMode = true,
        pollMs = 3000,
        onView,
    }: {
        darkMode?: boolean;
        pollMs?: number;
        onView?: (requestId: number) => void;
    } = $props();

    // Design tokens (match doc-processor dashboard)
    let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
    let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
    let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
    let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
    let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
    let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
    let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
    let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
    let colorSuccess = $derived(darkMode ? '#34D399' : '#10B981');
    let colorSuccessTint = $derived(darkMode ? 'rgba(52,211,153,0.15)' : 'rgba(16,185,129,0.10)');
    let colorError = $derived(darkMode ? '#F87171' : '#EF4444');
    let colorErrorTint = $derived(darkMode ? 'rgba(248,113,113,0.15)' : 'rgba(239,68,68,0.10)');

    type StageStatus = 'pending' | 'running' | 'success' | 'failed';

    let jobs = $state<ActiveJob[]>([]);
    let loadError = $state('');
    let stoppingIds = $state<Set<number>>(new Set());
    let aspectLabels = $state<Record<string, string>>({});
    let now = $state(Date.now());

    function prettify(name: string): string {
        return name.split('_').map((w) => (w ? w[0].toUpperCase() + w.slice(1) : w)).join(' ');
    }
    function labelFor(name: string): string {
        return aspectLabels[name] || prettify(name);
    }

    function statusColor(status: StageStatus): string {
        if (status === 'success') return colorSuccess;
        if (status === 'failed') return colorError;
        if (status === 'running') return accent;
        return textMuted;
    }
    function statusBg(status: StageStatus): string {
        if (status === 'success') return colorSuccessTint;
        if (status === 'failed') return colorErrorTint;
        if (status === 'running') return accentTint;
        return 'transparent';
    }

    function finishedCount(job: ActiveJob): number {
        return job.aspects.filter((a) => a.status === 'success' || a.status === 'failed').length;
    }

    function fmtClock(s?: string): string {
        if (!s) return '—';
        return s.slice(0, 19).replace('T', ' ');
    }

    function elapsed(job: ActiveJob): string {
        const startStr = job.start_time || job.create_time;
        if (!startStr) return '';
        const start = new Date(startStr).getTime();
        if (Number.isNaN(start)) return '';
        const secs = Math.max(0, Math.floor((now - start) / 1000));
        const m = Math.floor(secs / 60);
        const s = secs % 60;
        return m > 0 ? `${m}m ${s}s` : `${s}s`;
    }

    async function poll() {
        try {
            jobs = await listActiveJobs();
            loadError = '';
        } catch (e: any) {
            loadError = e?.message || 'Failed to load active jobs';
        }
    }

    async function handleStop(requestId: number) {
        const next = new Set(stoppingIds); next.add(requestId); stoppingIds = next;
        try {
            await stopRequest(requestId);
            await poll();
        } catch (e: any) {
            loadError = e?.message || 'Failed to stop review';
        } finally {
            const n = new Set(stoppingIds); n.delete(requestId); stoppingIds = n;
        }
    }

    let pollTimer: ReturnType<typeof setInterval>;
    let clock: ReturnType<typeof setInterval>;
    onMount(async () => {
        try {
            const all: AspectInfo[] = await listAspects();
            const map: Record<string, string> = {};
            for (const a of all) map[a.name] = a.label;
            aspectLabels = map;
        } catch { /* fall back to prettified names */ }
        await poll();
        pollTimer = setInterval(poll, pollMs);
        clock = setInterval(() => { now = Date.now(); }, 1000);
    });
    onDestroy(() => { clearInterval(pollTimer); clearInterval(clock); });
</script>

{#if jobs.length > 0}
    <div style="background:{cardBg}; border:1px solid {borderColor}; border-radius:12px; padding:16px 18px; margin-bottom:1rem;">
        <div style="display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom:12px;">
            <div style="font-size:15px; font-weight:600; color:{textPrimary};">
                Active Reviews <span style="color:{textMuted}; font-weight:400; font-size:13px;">({jobs.length})</span>
            </div>
            <div style="display:flex; align-items:center; gap:8px; font-size:11px; color:{textMuted};">
                <RefreshCwIcon class="h-3 w-3" style="animation:drm-spin 2s linear infinite;" />
                refreshes every {Math.round(pollMs / 1000)} s
            </div>
        </div>

        <div style="display:flex; flex-direction:column; gap:12px;">
            {#each jobs as job (job.request_id)}
                <div style="background:{surface2}; border:1px solid {borderColor}; border-radius:10px; padding:12px 14px;">
                    <!-- Job row -->
                    <div style="display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom:10px;">
                        <div style="display:flex; align-items:center; gap:10px; min-width:0;">
                            <span style="flex-shrink:0; font-family:monospace; font-size:11px; padding:2px 8px; border-radius:6px; background:{accentTint}; color:{accent};">#{job.request_id}</span>
                            <span style="font-weight:600; color:{textPrimary}; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{job.doc_title || `Document #${job.input_record_id}`}</span>
                            <span style="flex-shrink:0; font-size:11px; padding:2px 8px; border-radius:6px; background:{cardBg}; color:{textSecondary};">{job.tier}</span>
                            <span style="flex-shrink:0; font-size:11px; color:{textMuted};">{finishedCount(job)}/{job.aspects.length} done</span>
                        </div>
                        <div style="display:flex; align-items:center; gap:6px;">
                            <button onclick={() => handleStop(job.request_id)} disabled={stoppingIds.has(job.request_id)}
                                style="display:flex; align-items:center; gap:4px; padding:5px 10px; background:{cardBg}; border:1px solid {borderColor}; border-radius:8px; color:{textMuted}; font-size:12px; cursor:{stoppingIds.has(job.request_id) ? 'not-allowed' : 'pointer'}; opacity:{stoppingIds.has(job.request_id) ? 0.5 : 1};">
                                <SquareIcon class="h-3 w-3" />
                                {stoppingIds.has(job.request_id) ? 'Stopping…' : 'Stop'}
                            </button>
                            <button onclick={() => onView?.(job.request_id)}
                                style="display:flex; align-items:center; gap:4px; padding:5px 10px; background:{accentTint}; border:1px solid {accent}30; border-radius:8px; color:{accent}; font-size:12px; font-weight:500; cursor:pointer;">
                                <EyeIcon class="h-3 w-3" />
                                View
                            </button>
                        </div>
                    </div>

                    <!-- Aspect node chain (real per-aspect status) -->
                    <div style="display:flex; align-items:flex-start; gap:0; overflow-x:auto; padding-bottom:4px;">
                        {#each job.aspects as a, i}
                            {@const st = a.status as StageStatus}
                            <div style="position:relative; display:flex; flex-shrink:0; flex-direction:column; align-items:center; min-width:72px;" title={`${labelFor(a.aspect)}${a.error_message ? ' — ' + a.error_message : ''}`}>
                                <div style="position:relative; display:flex; align-items:center; justify-content:center; width:28px; height:28px; border-radius:9999px;
                                    background:{statusBg(st)}; border:2px solid {statusColor(st)}{st === 'pending' ? '40' : ''};
                                    {st === 'running' ? 'animation:drm-pulse-ring 1.5s ease-in-out infinite;' : ''}">
                                    {#if st === 'success'}
                                        <CircleCheckIcon class="h-3.5 w-3.5" style="color:{colorSuccess};" />
                                    {:else if st === 'failed'}
                                        <XCircleIcon class="h-3.5 w-3.5" style="color:{colorError};" />
                                    {:else if st === 'running'}
                                        <div style="width:8px; height:8px; border-radius:9999px; background:{accent}; animation:drm-pulse 1s ease-in-out infinite;"></div>
                                    {:else}
                                        <ClockIcon class="h-3 w-3" style="color:{textMuted}; opacity:0.5;" />
                                    {/if}
                                    {#if st === 'success' && a.finding_count > 0}
                                        <span style="position:absolute; top:-6px; right:-8px; min-width:15px; height:15px; padding:0 3px; display:flex; align-items:center; justify-content:center; border-radius:9999px; background:{colorError}; color:#fff; font-size:9px; font-weight:700;">{a.finding_count}</span>
                                    {/if}
                                </div>
                                <div style="font-size:9px; color:{statusColor(st)}{st === 'pending' ? '80' : ''}; margin-top:4px; text-align:center; white-space:nowrap; max-width:72px; overflow:hidden; text-overflow:ellipsis;">{labelFor(a.aspect)}</div>
                            </div>
                            {#if i < job.aspects.length - 1}
                                <div style="flex:0 0 16px; height:2px; background:{borderColor}; margin-top:13px;"></div>
                            {/if}
                        {/each}
                    </div>

                    <!-- Status line -->
                    <div style="margin-top:10px; border-top:1px solid {borderColor}; padding-top:8px; font-size:11px; color:{textMuted}; font-family:monospace; display:flex; gap:16px; flex-wrap:wrap;">
                        <span>By: {job.requester_name || '—'}</span>
                        <span>Started: {fmtClock(job.start_time || job.create_time)}</span>
                        {#if elapsed(job)}<span>Elapsed: {elapsed(job)}</span>{/if}
                    </div>
                </div>
            {/each}
        </div>

        {#if loadError}
            <div style="margin-top:8px; font-size:12px; color:{colorError};">{loadError}</div>
        {/if}
    </div>
{/if}

<style>
    @keyframes drm-spin { to { transform: rotate(360deg); } }
    @keyframes drm-pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
    @keyframes drm-pulse-ring {
        0%, 100% { box-shadow: 0 0 0 0 rgba(129,140,248,0.0); }
        50% { box-shadow: 0 0 0 4px rgba(129,140,248,0.18); }
    }
</style>
