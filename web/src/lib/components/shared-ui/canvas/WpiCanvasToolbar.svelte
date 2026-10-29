<!-- web/src/lib/components/shared-ui/canvas/WpiCanvasToolbar.svelte -->
<!-- The WPI editor's tool bar, above the canvas: create nodes, load a page,
     view JSON, undo/redo, align and distribute the selection, save. -->
<script lang="ts" module>
  export type WpiNodeKind = 'page' | 'element' | 'action';
</script>

<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
  import type { AlignMode, DistributeAxis } from './align-nodes';
  import PlusIcon                 from '@lucide/svelte/icons/plus';
  import ChevronDownIcon          from '@lucide/svelte/icons/chevron-down';
  import FileIcon                 from '@lucide/svelte/icons/file';
  import SquareMousePointerIcon   from '@lucide/svelte/icons/square-mouse-pointer';
  import ZapIcon                  from '@lucide/svelte/icons/zap';
  import FolderOpenIcon           from '@lucide/svelte/icons/folder-open';
  import BracesIcon               from '@lucide/svelte/icons/braces';
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
  import EyeIcon from '@lucide/svelte/icons/eye';
  import SaveIcon                 from '@lucide/svelte/icons/save';
  import BookmarkPlusIcon         from '@lucide/svelte/icons/bookmark-plus';

  let {
    isDirty,
    canUndo,
    canRedo,
    selectedCount,
    onCreate,
    onLoadPage,
    onViewJson,
    onPreview,
    previewing,
    onUndo,
    onRedo,
    onAlign,
    onDistribute,
    onSave,
    onSaveAsTemplate,
  }: {
    isDirty:          boolean;
    canUndo:          boolean;
    canRedo:          boolean;
    selectedCount:    number;
    onCreate:         (kind: WpiNodeKind) => void;
    onLoadPage:       () => void;
    onViewJson:       () => void;
    onPreview:        () => void;
    previewing:       boolean;
    onUndo:           () => void;
    onRedo:           () => void;
    onAlign:          (mode: AlignMode) => void;
    onDistribute:     (axis: DistributeAxis) => void;
    onSave:           () => void;
    onSaveAsTemplate: () => void;
  } = $props();

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

  // The menu opens outside the editor, beyond its --cv-* colors, so it keeps the site theme's colors.
  const itemClass = 'text-[12px]';
</script>

<div class="flex items-center gap-1 px-2 flex-shrink-0 overflow-x-auto"
  style="height:40px; background:var(--cv-raised); border-bottom:1px solid var(--cv-divider);"
  role="toolbar" aria-label={m.wpi_toolbar_label()}>

  <button onclick={onPreview} class="toolbar-btn" aria-pressed={previewing} title={previewing ? m.wpi_preview_edit() : m.wpi_preview_action()}>
    <EyeIcon class="w-4 h-4" />
    <span style="font-size:11px; margin-left:4px;">{previewing ? m.wpi_preview_edit() : m.wpi_preview_action()}</span>
  </button>

  {#if !previewing}
  <!-- Create nodes -->
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>
      {#snippet child({ props })}
        <button {...props} class="toolbar-btn" style="color:var(--cv-text); gap:4px;">
          <PlusIcon class="w-4 h-4" />
          <span style="font-size:11px;">{m.wpi_toolbar_create_nodes()}</span>
          <ChevronDownIcon class="w-3 h-3" />
        </button>
      {/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content align="start" class="min-w-40">
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('page')}>
        <FileIcon class="w-4 h-4 text-muted-foreground" />{m.wpi_toolbar_create_page()}
      </DropdownMenu.Item>
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('element')}>
        <SquareMousePointerIcon class="w-4 h-4 text-muted-foreground" />{m.wpi_toolbar_create_element()}
      </DropdownMenu.Item>
      <DropdownMenu.Item class={itemClass} onclick={() => onCreate('action')}>
        <ZapIcon class="w-4 h-4 text-muted-foreground" />{m.wpi_toolbar_create_action()}
      </DropdownMenu.Item>
    </DropdownMenu.Content>
  </DropdownMenu.Root>

  <!-- Load page -->
  <button onclick={onLoadPage} class="toolbar-btn" title={m.wpi_toolbar_load_page()} aria-label={m.wpi_toolbar_load_page()}>
    <FolderOpenIcon class="w-4 h-4" />
  </button>

  <!-- View JSON: the selected node's, else the page's -->
  <button onclick={onViewJson} class="toolbar-btn" style="gap:4px;">
    <BracesIcon class="w-4 h-4" />
    <span style="font-size:11px;">{m.wpi_toolbar_view_json()}</span>
  </button>

  <div style="width:1px; height:20px; background:var(--cv-border); margin:0 4px;"></div>

  <!-- Undo / Redo -->
  <button onclick={onUndo} disabled={!canUndo} title={m.flowtoolbar_undo()} aria-label={m.flowtoolbar_undo()} class="toolbar-btn">
    <Undo2Icon class="w-4 h-4" />
  </button>
  <button onclick={onRedo} disabled={!canRedo} title={m.flowtoolbar_redo()} aria-label={m.flowtoolbar_redo()} class="toolbar-btn">
    <Redo2Icon class="w-4 h-4" />
  </button>

  <div style="width:1px; height:20px; background:var(--cv-border); margin:0 4px;"></div>

  <!-- Align (two or more selected) -->
  {#each alignTools as tool (tool.mode)}
    <button onclick={() => onAlign(tool.mode)} disabled={selectedCount < 2}
      title={tool.label} aria-label={tool.label} class="toolbar-btn">
      <tool.Icon class="w-4 h-4" />
    </button>
  {/each}

  <div style="width:1px; height:20px; background:var(--cv-border); margin:0 4px;"></div>

  <!-- Distribute (three or more selected) -->
  {#each distributeTools as tool (tool.axis)}
    <button onclick={() => onDistribute(tool.axis)} disabled={selectedCount < 3}
      title={tool.label} aria-label={tool.label} class="toolbar-btn">
      <tool.Icon class="w-4 h-4" />
    </button>
  {/each}

  <div class="flex-1"></div>

  <!-- Save / Template -->
  <button onclick={onSave} title={m.flowtoolbar_save()} aria-label={m.flowtoolbar_save()} class="toolbar-btn" style="color:{isDirty ? 'var(--cv-warn)' : ''}">
    <SaveIcon class="w-4 h-4" />
    <span style="font-size:10px; margin-left:3px;">{m.flowtoolbar_save()}</span>
  </button>
  <button onclick={onSaveAsTemplate} title={m.flowtoolbar_save_as_template()} aria-label={m.flowtoolbar_save_as_template()} class="toolbar-btn">
    <BookmarkPlusIcon class="w-4 h-4" />
  </button>
  {/if}
</div>

<style>
  .toolbar-btn {
    display: flex; align-items: center; flex-shrink: 0;
    background: none; border: none; cursor: pointer;
    color: var(--cv-text-2); border-radius: 4px; padding: 4px 6px;
    transition: background 0.1s;
  }
  .toolbar-btn:hover:not(:disabled) { background: var(--cv-hover); }
  .toolbar-btn:disabled { opacity: 0.35; cursor: not-allowed; }
</style>
