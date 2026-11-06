<!-- web/src/lib/components/shared-ui/canvas/NodePalette.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import SettingsIcon from '@lucide/svelte/icons/settings';
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import type { NodeType } from '$lib/types/flow';

  let {
    nodeTypes       = [],
    extraCategories = [],
    darkMode        = true,
    width = 200,
    subcategoryLabel = (sub: string) => sub.toUpperCase(),
    onDragStart,
    doubleClickDrop = false,
    onDropModeChange,
    wheelZoom = false,
    onWheelModeChange,
    onDoubleClick,
  }: {
    nodeTypes:        NodeType[];
    // Folders shown after the node-type categories, even while no node type belongs to them.
    extraCategories?: string[];
    darkMode?:        boolean;
    width?: number;
    // The label of a sub-folder: node types with a subcategory are listed in one
    // inside their category's folder (OpenSpec wpi-primitive-validators, D9).
    subcategoryLabel?: (sub: string) => string;
    onDragStart:      (event: DragEvent, nodeType: NodeType) => void;
    doubleClickDrop?: boolean;
    onDropModeChange?: (enabled: boolean) => void;
    wheelZoom?: boolean;
    onWheelModeChange?: (enabled: boolean) => void;
    onDoubleClick?: (nodeType: NodeType) => void;
  } = $props();

  let search = $state('');
  let settingsOpen = $state(false);
  let settingsButton: HTMLButtonElement;
  let settingsMenu: HTMLDivElement;

  $effect(() => {
    if (!settingsOpen) return;
    function dismiss(event: PointerEvent) {
      const target = event.target as Node;
      if (!settingsButton.contains(target) && !settingsMenu.contains(target)) settingsOpen = false;
    }
    document.addEventListener('pointerdown', dismiss, true);
    return () => document.removeEventListener('pointerdown', dismiss, true);
  });
  // Folders start folded; while searching, every folder with a match is shown open.
  let expanded = $state<Record<string, boolean>>({});

  const searching = $derived(search !== '');

  // Extra folders keep their given order after the others, even once node types belong to them.
  const categories = $derived(
    [...new Set([...nodeTypes.map(n => n.category).filter(c => !extraCategories.includes(c)), ...extraCategories])]
  );

  function filteredByCategory(cat: string) {
    return nodeTypes.filter(n =>
      n.category === cat &&
      n.label.toLowerCase().includes(search.toLowerCase())
    );
  }

  // A folder's sub-folders, in the order their first node types come.
  function subcategories(items: NodeType[]) {
    return [...new Set(items.map(n => n.subcategory).filter((s): s is string => !!s))];
  }

  function categoryLabel(cat: string) {
    switch (cat) {
      case 'AI':       return m.nodepalette_category_ai();
      case 'Data':     return m.nodepalette_category_data();
      case 'Actions':  return m.nodepalette_category_actions();
      case 'Pages':    return m.nodepalette_category_pages();
      case 'Elements': return m.nodepalette_category_elements();
      case 'Value editors': return m.nodepalette_category_value_editors();
      case 'Validators': return m.nodepalette_category_validators();
      default:         return cat.toUpperCase();
    }
  }
</script>

{#snippet entry(nodeType: NodeType)}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    draggable="true"
    ondragstart={(e) => onDragStart(e, nodeType)}
    ondblclick={() => { if (doubleClickDrop) onDoubleClick?.(nodeType); }}
    class="flex items-center gap-2 rounded px-2 py-1.5 mb-1 cursor-grab hover:bg-(--cv-hover) transition-colors select-none"
    style="background:var(--cv-surface); border:1px solid transparent; font-size:10px; color:var(--cv-text);"
    title={nodeType.title ?? m.nodepalette_drag_onto_canvas()}
  >
    <span style="font-size:14px; flex-shrink:0;">{getIcon(nodeType.icon)}</span>
    {nodeType.label}
  </div>
{/snippet}

<aside
  class="flex flex-col h-full overflow-hidden shrink-0"
  style="width:{width}px; background:var(--cv-panel); border-right:1px solid var(--cv-divider);"
>
  <div style="padding:8px; border-bottom:1px solid var(--cv-divider);">
    <div style="display:flex; align-items:center; justify-content:space-between; margin-bottom:6px; position:relative;">
      <span style="font-size:9px; color:var(--cv-accent); letter-spacing:1px;">{m.nodepalette_node_palette()}</span>
      {#if onDropModeChange}
        <button bind:this={settingsButton} type="button" aria-label={m.nodepalette_settings()} title={m.nodepalette_settings()} aria-expanded={settingsOpen} aria-haspopup="menu" onclick={() => settingsOpen = !settingsOpen} style="border:0; background:transparent; color:var(--cv-text-2); cursor:pointer; padding:3px;"><SettingsIcon size={14} /></button>
        {#if settingsOpen}
          <div bind:this={settingsMenu} role="menu" style="position:absolute; right:0; top:100%; z-index:2; min-width:180px; padding:4px; background:var(--cv-panel); border:1px solid var(--cv-border); box-shadow:0 4px 12px var(--cv-shadow);">
            <button type="button" role="menuitemradio" aria-checked={!doubleClickDrop} onclick={() => { onDropModeChange(false); settingsOpen = false; }} style="display:block; width:100%; text-align:left; padding:6px; border:0; background:transparent; color:var(--cv-text); cursor:pointer;">{!doubleClickDrop ? '✓ ' : '　'}{m.nodepalette_drag_to_drop()}</button>
            <button type="button" role="menuitemradio" aria-checked={doubleClickDrop} onclick={() => { onDropModeChange(true); settingsOpen = false; }} style="display:block; width:100%; text-align:left; padding:6px; border:0; background:transparent; color:var(--cv-text); cursor:pointer;">{doubleClickDrop ? '✓ ' : '　'}{m.nodepalette_double_click_to_drop()}</button>
            {#if onWheelModeChange}
              <div role="separator" style="border-top:1px solid var(--cv-border); margin:4px 0;"></div>
              <button type="button" role="menuitemradio" aria-checked={wheelZoom} onclick={() => { onWheelModeChange(true); settingsOpen = false; }} style="display:block; width:100%; text-align:left; padding:6px; border:0; background:transparent; color:var(--cv-text); cursor:pointer;">{wheelZoom ? '✓ ' : '　'}{m.nodepalette_wheel_to_zoom()}</button>
              <button type="button" role="menuitemradio" aria-checked={!wheelZoom} onclick={() => { onWheelModeChange(false); settingsOpen = false; }} style="display:block; width:100%; text-align:left; padding:6px; border:0; background:transparent; color:var(--cv-text); cursor:pointer;">{!wheelZoom ? '✓ ' : '　'}{m.nodepalette_wheel_to_scroll()}</button>
            {/if}
          </div>
        {/if}
      {/if}
    </div>
    <input
      bind:value={search}
      placeholder={m.nodepalette_search_nodes()}
      style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text-2); box-sizing:border-box;"
    />
  </div>

  <div class="flex-1 overflow-y-auto" style="padding:8px; scrollbar-width:thin; scrollbar-color:var(--cv-border) transparent;">
    {#each categories as cat}
      {@const items = filteredByCategory(cat)}
      {#if items.length > 0 || !searching}
        {@const open = searching || !!expanded[cat]}
        <button
          type="button"
          onclick={() => (expanded[cat] = !expanded[cat])}
          aria-expanded={open}
          class="flex items-center gap-1 w-full text-left text-(--cv-text-faint) hover:text-(--cv-text-2) transition-colors"
          style="font-size:8px; letter-spacing:1px; margin:8px 0 4px 0; padding:0 4px 0 0; background:none; border:none; cursor:pointer;"
        >
          <ChevronRightIcon size={10} class="shrink-0 transition-transform {open ? 'rotate-90' : ''}" />
          <span class="flex-1">{categoryLabel(cat)}</span>
          <span>{items.length}</span>
        </button>
        {#if open}
          {#if items.length === 0}
            <div style="font-size:9px; color:var(--cv-text-faint); font-style:italic; padding:2px 8px 4px 14px;">{m.nodepalette_no_nodes_yet()}</div>
          {/if}
          {#each items.filter(n => !n.subcategory) as nodeType (nodeType.id)}{@render entry(nodeType)}{/each}
          {#each subcategories(items) as sub (sub)}
            {@const key = `${cat}/${sub}`}
            {@const subItems = items.filter(n => n.subcategory === sub)}
            {@const subOpen = searching || !!expanded[key]}
            <button
              type="button"
              onclick={() => (expanded[key] = !expanded[key])}
              aria-expanded={subOpen}
              class="flex items-center gap-1 w-full text-left text-(--cv-text-faint) hover:text-(--cv-text-2) transition-colors"
              style="font-size:8px; letter-spacing:1px; margin:4px 0 4px 0; padding:0 4px 0 10px; background:none; border:none; cursor:pointer;"
            >
              <ChevronRightIcon size={10} class="shrink-0 transition-transform {subOpen ? 'rotate-90' : ''}" />
              <span class="flex-1">{subcategoryLabel(sub)}</span>
              <span>{subItems.length}</span>
            </button>
            {#if subOpen}
              <div style="padding-left:10px;">
                {#each subItems as nodeType (nodeType.id)}{@render entry(nodeType)}{/each}
              </div>
            {/if}
          {/each}
        {/if}
      {/if}
    {/each}
  </div>
</aside>

<script module lang="ts">
  // Simple icon emoji map — replace with lucide icons if desired
  const ICON_MAP: Record<string, string> = {
    Bot: '🤖', Terminal: '💻', Type: '📝', FileText: '📄',
    File: '📁', Image: '🎬', Wrench: '🔧', Plug: '🔌',
    Globe: '📡', Filter: '📋', GitBranch: '🗂', ShieldCheck: '🛡',
  };
  export function getIcon(name: string) { return ICON_MAP[name] ?? '⬡'; }
</script>
