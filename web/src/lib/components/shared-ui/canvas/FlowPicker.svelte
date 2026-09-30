<!-- web/src/lib/components/shared-ui/canvas/FlowPicker.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import { flowService, type FlowScope } from '$lib/services/flowService';
  import type { Flow } from '$lib/types/flow';
  import FlowCard from './FlowCard.svelte';

  let {
    darkMode       = true,
    currentUserId,
    onOpen,
    onNewEmpty,
    onClose,
  }: {
    darkMode?:     boolean;
    currentUserId: number;
    onOpen:     (flow: Flow) => void;
    onNewEmpty: () => void;
    onClose:    () => void;
  } = $props();

  type Tab = 'mine' | 'shared' | 'templates';
  let activeTab = $state<Tab>('mine');
  let flows     = $state<Flow[]>([]);
  let loading   = $state(false);
  let search    = $state('');
  let error     = $state('');

  const filtered = $derived(
    flows.filter(f =>
      f.flow_name.toLowerCase().includes(search.toLowerCase())
    )
  );

  async function loadFlows(scope: Tab) {
    loading = true;
    error   = '';
    try {
      const res = await flowService.list(scope as FlowScope);
      flows = res.flows ?? [];
    } catch (e: any) {
      error = e?.error?.message ?? m.flowpicker_failed_to_load_flows();
      flows = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => { loadFlows(activeTab); });

  async function handleSetDefault(flow: Flow) {
    try { await flowService.setDefault(flow.flow_id); await loadFlows(activeTab); }
    catch (e: any) { error = e?.error?.message ?? m.flowpicker_failed_to_set_default(); }
  }

  async function handleSaveAsTemplate(flow: Flow) {
    try { await flowService.saveAsTemplate(flow.flow_id); await loadFlows(activeTab); }
    catch (e: any) { error = e?.error?.message ?? m.flowpicker_failed_to_save_as_template(); }
  }

  async function handleDuplicate(flow: Flow) {
    try {
      const res = await flowService.fork(flow.flow_id);
      onOpen(res.flow);
    } catch (e: any) { error = e?.error?.message ?? m.flowpicker_failed_to_duplicate(); }
  }

  async function handleDelete(flow: Flow) {
    if (!confirm(m.flowpicker_delete({ flow_name: flow.flow_name }))) return;
    try { await flowService.delete(flow.flow_id); await loadFlows(activeTab); }
    catch (e: any) { error = e?.error?.message ?? m.flowpicker_failed_to_delete(); }
  }
</script>

<!-- Backdrop -->
<div class="fixed inset-0 z-50 flex items-center justify-center"
  style="background: rgba(0,0,0,0.7);"
  onclick={(e) => { if (e.target === e.currentTarget) onClose(); }}
  onkeydown={(e) => { if ((e.key === 'Escape' || e.key === 'Enter') && e.target === e.currentTarget) onClose(); }}
  role="dialog" aria-modal="true" aria-label={m.flowpicker_open_a_flow()}
  tabindex="0"
>
  <div class="rounded-xl overflow-hidden shadow-2xl"
    style="background:#111827; border:1px solid #1e2a3a; width:680px; max-width:95vw; max-height:80vh; display:flex; flex-direction:column;">

    <!-- Header -->
    <div style="padding:16px 20px; border-bottom:1px solid #1e2a3a; display:flex; justify-content:space-between; align-items:flex-start;">
      <div>
        <div style="font-size:15px; font-weight:600; color:#e2e8f0;">{m.flowpicker_open_a_flow()}</div>
        <div style="font-size:11px; color:#6b7280; margin-top:2px;">{m.flowpicker_select_a_flow_start_from()}</div>
      </div>
      <button onclick={onClose} style="color:#6b7280; font-size:16px; background:none; border:none; cursor:pointer;" aria-label={m.flowpicker_close()}>✕</button>
    </div>

    <!-- Tabs -->
    <div style="display:flex; border-bottom:1px solid #1e2a3a; padding:0 20px;">
      {#each (['mine','shared','templates'] as const) as tab}
        <button
          style="padding:10px 16px; font-size:11px; border:none; background:none; cursor:pointer;
            color:{activeTab===tab ? '#6366f1' : '#6b7280'};
            border-bottom:{activeTab===tab ? '2px solid #6366f1' : '2px solid transparent'};"
          onclick={() => { activeTab = tab; search = ''; }}
        >
          {tab === 'mine' ? m.flowpicker_my_flows() : tab === 'shared' ? m.flowpicker_shared_flows() : m.flowpicker_templates()}
        </button>
      {/each}
    </div>

    <!-- Search + New -->
    <div style="padding:12px 20px; display:flex; gap:8px; align-items:center;">
      <input
        bind:value={search}
        placeholder={m.flowpicker_search_flows()}
        style="flex:1; background:#1e2535; border:1px solid #374151; border-radius:6px; padding:6px 12px; font-size:11px; color:#94a3b8;"
      />
      <button onclick={onNewEmpty}
        style="background:#6366f1; border-radius:6px; padding:6px 12px; font-size:11px; color:white; border:none; cursor:pointer; white-space:nowrap;">
        {m.flowpicker_new_empty_flow()}
      </button>
    </div>

    <!-- Error -->
    {#if error}
      <div style="margin:0 20px 8px; padding:8px 12px; background:#1f1313; border:1px solid #ef4444; border-radius:6px; font-size:11px; color:#ef4444;">
        {error}
      </div>
    {/if}

    <!-- Grid -->
    <div style="flex:1; overflow-y:auto; padding:0 20px 20px;">
      {#if loading}
        <div style="text-align:center; padding:40px; color:#6b7280; font-size:12px;">{m.flowpicker_loading()}</div>
      {:else if filtered.length === 0}
        <div style="text-align:center; padding:40px; color:#6b7280; font-size:12px;">
          {search ? m.flowpicker_no_flows_match_your_search() : m.flowpicker_no_flows_yet()}
        </div>
      {:else}
        <div style="display:grid; grid-template-columns:repeat(3,1fr); gap:10px; padding-top:4px;">
          {#each filtered as flow (flow.flow_id)}
            <FlowCard
              {flow}
              {currentUserId}
              onOpen={(f) => onOpen(f)}
              onSetDefault={handleSetDefault}
              onSaveAsTemplate={handleSaveAsTemplate}
              onDuplicate={handleDuplicate}
              onDelete={handleDelete}
            />
          {/each}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div style="padding:10px 20px; border-top:1px solid #1e2a3a; display:flex; justify-content:space-between; align-items:center;">
      <span style="font-size:10px; color:#4b5563;">{m.flowpicker_right_click_a_card_for()}</span>
      <button onclick={onClose} style="background:#1e2535; border-radius:6px; padding:5px 12px; font-size:11px; color:#94a3b8; border:none; cursor:pointer;">{m.flowpicker_cancel()}</button>
    </div>
  </div>
</div>
