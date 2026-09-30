<!-- web/src/lib/components/shared-ui/canvas/FlowToolbar.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import type { Flow } from '$lib/types/flow';
  import Undo2Icon        from '@lucide/svelte/icons/undo-2';
  import Redo2Icon        from '@lucide/svelte/icons/redo-2';
  import ZoomInIcon       from '@lucide/svelte/icons/zoom-in';
  import MaximizeIcon     from '@lucide/svelte/icons/maximize';
  import SaveIcon         from '@lucide/svelte/icons/save';
  import PlayIcon         from '@lucide/svelte/icons/play';
  import XIcon            from '@lucide/svelte/icons/x';
  import ChevronDownIcon  from '@lucide/svelte/icons/chevron-down';
  import BookmarkPlusIcon from '@lucide/svelte/icons/bookmark-plus';

  function focus(node: HTMLElement) { node.focus(); }

  let {
    activeFlow,
    isDirty,
    canUndo,
    canRedo,
    darkMode = true,
    onPickerOpen,
    onRename,
    onUndo,
    onRedo,
    onFitView,
    onSave,
    onSaveAsTemplate,
    onClose,
  }: {
    activeFlow:       import('$lib/types/flow').Flow | null;
    isDirty:          boolean;
    canUndo:          boolean;
    canRedo:          boolean;
    darkMode?:        boolean;
    onPickerOpen:     () => void;
    onRename:         (name: string) => void;
    onUndo:           () => void;
    onRedo:           () => void;
    onFitView:        () => void;
    onSave:           () => void;
    onSaveAsTemplate: () => void;
    onClose:          () => void;
  } = $props();

  let editingName = $state(false);
  let nameInput   = $state('');

  function startRename() {
    nameInput   = activeFlow?.flow_name ?? '';
    editingName = true;
  }
  function commitRename() {
    const trimmed = nameInput.trim().slice(0, 255);
    if (trimmed && trimmed !== activeFlow?.flow_name) onRename(trimmed);
    editingName = false;
  }
</script>

<div class="flex items-center gap-2 px-3 flex-shrink-0"
  style="height:44px; background:#161b27; border-bottom:1px solid #1e2a3a; z-index:10;">

  <!-- Flow name + picker toggle -->
  <div class="flex items-center gap-1 flex-1 min-w-0">
    <div class="w-5 h-5 rounded flex-shrink-0" style="background:#6366f1;"></div>
    {#if editingName}
      <input
        bind:value={nameInput}
        onblur={commitRename}
        onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') editingName=false; }}
        class="rounded px-2 py-1"
        style="background:#0d1117; border:1px solid #6366f1; color:#e2e8f0; font-size:12px; max-width:200px;"
        use:focus
        maxlength="255"
      />
    {:else}
      <button
        onclick={startRename}
        class="rounded px-2 py-1 flex items-center gap-1 hover:bg-white/5 transition-colors"
        style="background:#1e2535; color:#e2e8f0; font-size:12px; border:none; cursor:pointer; max-width:200px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
        title={m.flowtoolbar_click_to_rename()}
      >
        {activeFlow?.flow_name ?? m.flowtoolbar_untitled_flow()}
        {#if isDirty}<span style="color:#f59e0b; font-size:10px;">•</span>{/if}
      </button>
    {/if}
    <button
      onclick={onPickerOpen}
      title={m.flowtoolbar_open_flow_picker()}
      style="background:#1e2535; border:none; border-radius:4px; padding:3px 5px; cursor:pointer; color:#6b7280; display:flex; align-items:center;"
    >
      <ChevronDownIcon class="w-3 h-3" />
    </button>
  </div>

  <!-- Divider -->
  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Undo / Redo -->
  <button onclick={onUndo} disabled={!canUndo} title={m.flowtoolbar_undo()} class="toolbar-btn" aria-label={m.flowtoolbar_undo()}>
    <Undo2Icon class="w-4 h-4" />
  </button>
  <button onclick={onRedo} disabled={!canRedo} title={m.flowtoolbar_redo()} class="toolbar-btn" aria-label={m.flowtoolbar_redo()}>
    <Redo2Icon class="w-4 h-4" />
  </button>

  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Zoom / Fit -->
  <button onclick={onFitView} title={m.flowtoolbar_fit_view()} class="toolbar-btn" aria-label={m.flowtoolbar_fit_view()}>
    <MaximizeIcon class="w-4 h-4" />
  </button>

  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Save / Template / Run -->
  <button onclick={onSave} title={m.flowtoolbar_save()} class="toolbar-btn" style="color:{isDirty ? '#fbbf24' : ''}" aria-label={m.flowtoolbar_save()}>
    <SaveIcon class="w-4 h-4" />
    <span style="font-size:10px; margin-left:3px;">{m.flowtoolbar_save()}</span>
  </button>
  <button onclick={onSaveAsTemplate} title={m.flowtoolbar_save_as_template()} class="toolbar-btn" aria-label={m.flowtoolbar_save_as_template()}>
    <BookmarkPlusIcon class="w-4 h-4" />
  </button>
  <button title={m.flowtoolbar_run_not_yet_implemented()} class="toolbar-btn" style="background:#6366f1; color:white; border-radius:4px; padding:4px 10px;" aria-label={m.flowtoolbar_run()}>
    <PlayIcon class="w-4 h-4" />
    <span style="font-size:10px; margin-left:3px;">{m.flowtoolbar_run()}</span>
  </button>

  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Close -->
  <button onclick={onClose} title={m.flowtoolbar_close_canvas()} class="toolbar-btn" aria-label={m.flowtoolbar_close()}>
    <XIcon class="w-4 h-4" />
  </button>
</div>

<style>
  .toolbar-btn {
    display: flex; align-items: center;
    background: none; border: none; cursor: pointer;
    color: #94a3b8; border-radius: 4px; padding: 4px 6px;
    transition: background 0.1s;
  }
  .toolbar-btn:hover:not(:disabled) { background: rgba(255,255,255,0.05); }
  .toolbar-btn:disabled { opacity: 0.35; cursor: not-allowed; }
</style>
