<!-- web/src/lib/components/shared-ui/canvas/WpiCanvasToolbar.svelte -->
<!-- The WPI editor's tool bar, above the canvas: flow name and picker, create nodes, load a page,
     undo/redo, align and distribute the selection, save. -->
<script lang="ts" module>
  export type WpiNodeKind = 'page' | 'element' | 'action';
</script>

<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
  import type { Flow } from '$lib/types/flow';
  import type { AlignMode, DistributeAxis } from './align-nodes';
  import PlusIcon                 from '@lucide/svelte/icons/plus';
  import ChevronDownIcon          from '@lucide/svelte/icons/chevron-down';
  import FileIcon                 from '@lucide/svelte/icons/file';
  import SquareMousePointerIcon   from '@lucide/svelte/icons/square-mouse-pointer';
  import ZapIcon                  from '@lucide/svelte/icons/zap';
  import FolderOpenIcon           from '@lucide/svelte/icons/folder-open';
  import Undo2Icon                from '@lucide/svelte/icons/undo-2';
  import Redo2Icon                from '@lucide/svelte/icons/redo-2';
  import AlignStartVerticalIcon   from '@lucide/svelte/icons/align-start-vertical';
  import AlignCenterVerticalIcon  from '@lucide/svelte/icons/align-center-vertical';
  import AlignEndVerticalIcon     from '@lucide/svelte/icons/align-end-vertical';
  import AlignStartHorizontalIcon from '@lucide/svelte/icons/align-start-horizontal';
  import AlignCenterHorizontalIcon from '@lucide/svelte/icons/align-center-horizontal';
  import AlignEndHorizontalIcon   from '@lucide/svelte/icons/align-end-horizontal';
  import DistributeHorizontalIcon from '@lucide/svelte/icons/align-horizontal-distribute-center';
  import DistributeVerticalIcon   from '@lucide/svelte/icons/align-vertical-distribute-center';
  import SaveIcon                 from '@lucide/svelte/icons/save';
  import BookmarkPlusIcon         from '@lucide/svelte/icons/bookmark-plus';

  function focus(node: HTMLElement) { node.focus(); }

  let {
    activeFlow,
    isDirty,
    canUndo,
    canRedo,
    selectedCount,
    onPickerOpen,
    onRename,
    onCreate,
    onLoadPage,
    onUndo,
    onRedo,
    onAlign,
    onDistribute,
    onSave,
    onSaveAsTemplate,
  }: {
    activeFlow:       Flow | null;
    isDirty:          boolean;
    canUndo:          boolean;
    canRedo:          boolean;
    selectedCount:    number;
    onPickerOpen:     () => void;
    onRename:         (name: string) => void;
    onCreate:         (kind: WpiNodeKind) => void;
    onLoadPage:       () => void;
    onUndo:           () => void;
    onRedo:           () => void;
    onAlign:          (mode: AlignMode) => void;
    onDistribute:     (axis: DistributeAxis) => void;
    onSave:           () => void;
    onSaveAsTemplate: () => void;
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

  const alignTools = $derived([
    { mode: 'left'   as AlignMode, label: m.wpi_toolbar_align_left(),   Icon: AlignStartVerticalIcon },
    { mode: 'center' as AlignMode, label: m.wpi_toolbar_align_center(), Icon: AlignCenterVerticalIcon },
    { mode: 'right'  as AlignMode, label: m.wpi_toolbar_align_right(),  Icon: AlignEndVerticalIcon },
    { mode: 'top'    as AlignMode, label: m.wpi_toolbar_align_top(),    Icon: AlignStartHorizontalIcon },
    { mode: 'middle' as AlignMode, label: m.wpi_toolbar_align_middle(), Icon: AlignCenterHorizontalIcon },
    { mode: 'bottom' as AlignMode, label: m.wpi_toolbar_align_bottom(), Icon: AlignEndHorizontalIcon },
  ]);
  const distributeTools = $derived([
    { axis: 'horizontal' as DistributeAxis, label: m.wpi_toolbar_distribute_horizontal(), Icon: DistributeHorizontalIcon },
    { axis: 'vertical'   as DistributeAxis, label: m.wpi_toolbar_distribute_vertical(),   Icon: DistributeVerticalIcon },
  ]);

  // The editor is always dark, so the menu does not follow the site theme.
  const itemClass = 'text-[12px] text-[#e2e8f0] data-highlighted:bg-white/5 data-highlighted:text-[#e2e8f0]';
</script>

<div class="flex items-center gap-1 px-2 flex-shrink-0 overflow-x-auto"
  style="height:40px; background:#161b27; border-bottom:1px solid #1e2a3a;"
  role="toolbar" aria-label={m.wpi_toolbar_label()}>

  <!-- Flow name + picker toggle -->
  <div class="flex items-center gap-1 flex-shrink-0">
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
      aria-label={m.flowtoolbar_open_flow_picker()}
      style="background:#1e2535; border:none; border-radius:4px; padding:3px 5px; cursor:pointer; color:#6b7280; display:flex; align-items:center;"
    >
      <ChevronDownIcon class="w-3 h-3" />
    </button>
  </div>

  <div style="width:1px; height:20px; background:#374151; margin:0 4px;"></div>

  <!-- Create nodes -->
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>
      {#snippet child({ props })}
        <button {...props} class="toolbar-btn" style="color:#e2e8f0; gap:4px;">
          <PlusIcon class="w-4 h-4" />
          <span style="font-size:11px;">{m.wpi_toolbar_create_nodes()}</span>
          <ChevronDownIcon class="w-3 h-3" />
        </button>
      {/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content align="start" class="min-w-40 bg-[#1e2535] border-[#374151]">
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('page')}>
        <FileIcon class="w-4 h-4 text-[#94a3b8]" />{m.wpi_toolbar_create_page()}
      </DropdownMenu.Item>
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('element')}>
        <SquareMousePointerIcon class="w-4 h-4 text-[#94a3b8]" />{m.wpi_toolbar_create_element()}
      </DropdownMenu.Item>
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('action')}>
        <ZapIcon class="w-4 h-4 text-[#94a3b8]" />{m.wpi_toolbar_create_action()}
      </DropdownMenu.Item>
    </DropdownMenu.Content>
  </DropdownMenu.Root>

  <!-- Load page -->
  <button onclick={onLoadPage} class="toolbar-btn" style="gap:4px;">
    <FolderOpenIcon class="w-4 h-4" />
    <span style="font-size:11px;">{m.wpi_toolbar_load_page()}</span>
  </button>

  <div style="width:1px; height:20px; background:#374151; margin:0 4px;"></div>

  <!-- Undo / Redo -->
  <button onclick={onUndo} disabled={!canUndo} title={m.flowtoolbar_undo()} aria-label={m.flowtoolbar_undo()} class="toolbar-btn">
    <Undo2Icon class="w-4 h-4" />
  </button>
  <button onclick={onRedo} disabled={!canRedo} title={m.flowtoolbar_redo()} aria-label={m.flowtoolbar_redo()} class="toolbar-btn">
    <Redo2Icon class="w-4 h-4" />
  </button>

  <div style="width:1px; height:20px; background:#374151; margin:0 4px;"></div>

  <!-- Align (two or more selected) -->
  {#each alignTools as tool (tool.mode)}
    <button onclick={() => onAlign(tool.mode)} disabled={selectedCount < 2}
      title={tool.label} aria-label={tool.label} class="toolbar-btn">
      <tool.Icon class="w-4 h-4" />
    </button>
  {/each}

  <div style="width:1px; height:20px; background:#374151; margin:0 4px;"></div>

  <!-- Distribute (three or more selected) -->
  {#each distributeTools as tool (tool.axis)}
    <button onclick={() => onDistribute(tool.axis)} disabled={selectedCount < 3}
      title={tool.label} aria-label={tool.label} class="toolbar-btn">
      <tool.Icon class="w-4 h-4" />
    </button>
  {/each}

  <div class="flex-1"></div>

  <!-- Save / Template -->
  <button onclick={onSave} title={m.flowtoolbar_save()} aria-label={m.flowtoolbar_save()} class="toolbar-btn" style="color:{isDirty ? '#fbbf24' : ''}">
    <SaveIcon class="w-4 h-4" />
    <span style="font-size:10px; margin-left:3px;">{m.flowtoolbar_save()}</span>
  </button>
  <button onclick={onSaveAsTemplate} title={m.flowtoolbar_save_as_template()} aria-label={m.flowtoolbar_save_as_template()} class="toolbar-btn">
    <BookmarkPlusIcon class="w-4 h-4" />
  </button>
</div>

<style>
  .toolbar-btn {
    display: flex; align-items: center; flex-shrink: 0;
    background: none; border: none; cursor: pointer;
    color: #94a3b8; border-radius: 4px; padding: 4px 6px;
    transition: background 0.1s;
  }
  .toolbar-btn:hover:not(:disabled) { background: rgba(255,255,255,0.05); }
  .toolbar-btn:disabled { opacity: 0.35; cursor: not-allowed; }
</style>
