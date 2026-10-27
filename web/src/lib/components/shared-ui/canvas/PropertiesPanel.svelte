<!-- web/src/lib/components/shared-ui/canvas/PropertiesPanel.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import type { FlowNode, NodeType } from '$lib/types/flow';
  import type { ElementAttribute, ElementClassSummary } from '$lib/wpi/api';
  import type { JsonObject } from '$lib/wpi/types';
  import { localized } from '$lib/wpi/text';
  import { valueText, withAttribute } from './element-attributes';
  import NodeDescriptorEditor from './NodeDescriptorEditor.svelte';

  let {
    node,
    nodeType,
    elementClass,
    nodeTab  = false,
    width    = 240,
    darkMode = true,
    onUpdate,
  }: {
    node:      FlowNode | null;
    nodeType:  NodeType | null;
    // The class of a WPI element node, whose instance attributes are edited
    // instead of nodeType's: null when the class is not in the class list,
    // left out for other nodes.
    elementClass?: ElementClassSummary | null;
    // Tabs Properties and Node, where Node edits the node's descriptor (the WPI editor).
    nodeTab?:  boolean;
    width?:    number;
    darkMode?: boolean;
    onUpdate:  (nodeId: string, data: Record<string, unknown>) => void;
  } = $props();

  // Map of attribute → UI control type
  const SELECT_ATTRS: Record<string, string[]> = {
    model:      ['gpt-4o', 'gpt-4o-mini', 'claude-opus-4-6', 'claude-sonnet-4-6', 'claude-haiku-4-5'],
    language:   ['typescript', 'python', 'go', 'rust', 'java', 'c++', 'bash'],
    file_type:  ['txt', 'csv', 'json', 'pdf', 'md', 'html'],
    media_type: ['image', 'audio', 'video'],
    method:     ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'],
    operation:  ['status', 'clone', 'pull', 'push', 'commit', 'diff'],
  };
  const MASKED_ATTRS  = new Set(['auth_token']);
  const TEXTAREA_ATTRS = new Set(['system_prompt', 'content', 'tool_config', 'headers', 'rule_expression']);

  type Tab = 'properties' | 'node';
  let tab = $state<Tab>('properties');
  const TABS = $derived<{ id: Tab; label: string }[]>([
    { id: 'properties', label: m.propertiespanel_tab_properties() },
    { id: 'node',       label: m.propertiespanel_tab_node() },
  ]);

  function update(key: string, value: unknown) {
    if (!node) return;
    onUpdate(node.id, { ...node.data, [key]: value });
  }

  // An element node keeps its instance attributes in data.attrs; data.label is
  // the node's name, which may differ from an instance attribute called label.
  function attrsOf(n: FlowNode): JsonObject {
    return (n.data.attrs as JsonObject | undefined) ?? {};
  }

  // The attribute field being typed in. Its text stays as typed until the field
  // is left, so reading it as JSON does not rewrite it under the cursor.
  let typing = $state<{ nodeId: string; name: string; text: string } | null>(null);

  function attrText(n: FlowNode, name: string): string {
    if (typing && typing.nodeId === n.id && typing.name === name) return typing.text;
    return valueText(attrsOf(n)[name]);
  }

  function typeAttr(name: string, text: string) {
    if (!node) return;
    typing = { nodeId: node.id, name, text };
    update('attrs', withAttribute(attrsOf(node), name, text));
  }

  function attrPlaceholder(attr: ElementAttribute): string {
    if (attr.default_value === null) return m.propertiespanel_required();
    if (attr.default_value === undefined) return m.propertiespanel_no_default();
    return m.propertiespanel_default({ value: valueText(attr.default_value) });
  }
</script>

<aside
  class="flex flex-col h-full overflow-hidden flex-shrink-0"
  style="width:{width}px; background:var(--cv-panel); border-left:1px solid var(--cv-divider);"
>
  {#if nodeTab}
    <div role="tablist" style="display:flex; gap:16px; padding:0 12px; border-bottom:1px solid var(--cv-divider); flex-shrink:0;">
      {#each TABS as t (t.id)}
        <button type="button" role="tab" aria-selected={tab === t.id} onclick={() => (tab = t.id)}
          style="background:none; border:none; border-bottom:2px solid {tab === t.id ? 'var(--cv-accent)' : 'transparent'}; margin-bottom:-1px; padding:9px 0 7px; font-size:11px; font-weight:500; color:{tab === t.id ? 'var(--cv-accent)' : 'var(--cv-text-muted)'}; cursor:pointer;"
        >{t.label}</button>
      {/each}
    </div>
  {/if}
  {#if !node || !nodeType}
    <div class="flex-1 flex items-center justify-center" style="color:var(--cv-text-faint); font-size:12px;">
      {m.propertiespanel_select_a_node_to_edit()}
    </div>
  {:else if nodeTab && tab === 'node'}
    {#key node.id}
      <NodeDescriptorEditor {node} {onUpdate} />
    {/key}
  {:else}
    <div style="padding:12px 12px 0; border-bottom:1px solid var(--cv-divider);">
      {#if !nodeTab}
        <div style="font-size:9px; color:var(--cv-accent); letter-spacing:1px; margin-bottom:8px;">{m.propertiespanel_properties()}</div>
      {/if}
      <!-- Node name -->
      <div style="margin-bottom:10px;">
        <label for="prop-node-label" style="font-size:9px; color:var(--cv-text-muted); display:block; margin-bottom:3px;">{m.propertiespanel_name()}</label>
        <input
          id="prop-node-label"
          type="text"
          maxlength="100"
          value={String(node.data.label ?? nodeType.label)}
          oninput={(e) => update('label', (e.target as HTMLInputElement).value)}
          style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); box-sizing:border-box;"
        />
      </div>
    </div>

    <div class="flex-1 overflow-y-auto" style="padding:10px 12px; scrollbar-width:thin; scrollbar-color:var(--cv-border) transparent;">
      {#if elementClass !== undefined}
        <!-- Element class and its instance attributes -->
        <div style="margin-bottom:10px;">
          <label for="prop-element-class" style="font-size:9px; color:var(--cv-text-muted); display:block; margin-bottom:3px;">{m.propertiespanel_element_class()}</label>
          <input id="prop-element-class" type="text" readonly value={String(node.data.element ?? '')}
            style="width:100%; background:transparent; border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text-2); box-sizing:border-box;"
          />
        </div>
        {#if elementClass === null}
          <div style="font-size:10px; color:var(--cv-text-faint);">{m.propertiespanel_element_class_unavailable({ id: String(node.data.element ?? '') })}</div>
        {:else}
          {#each Object.entries(elementClass.attributes) as [name, attr] (name)}
            <div style="margin-bottom:10px;">
              <label for={`prop-attr-${name}`} style="font-size:9px; color:var(--cv-text-muted); display:block; margin-bottom:3px;">
                {name}{#if attr.default_value === null}<span title={m.propertiespanel_required()} style="color:var(--cv-accent);"> *</span>{/if}
              </label>
              <textarea
                id={`prop-attr-${name}`}
                rows="1"
                value={attrText(node, name)}
                placeholder={attrPlaceholder(attr)}
                oninput={(e) => typeAttr(name, (e.target as HTMLTextAreaElement).value)}
                onblur={() => (typing = null)}
                style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); resize:vertical; field-sizing:content; box-sizing:border-box;"
              ></textarea>
              {#if attr.help_tip}
                <div style="font-size:9px; color:var(--cv-text-faint); margin-top:2px;">{localized(attr.help_tip)}</div>
              {/if}
            </div>
          {/each}
        {/if}
      {/if}

      <!-- Attributes -->
      {#each Object.entries(nodeType.defaultData) as [key]}
        <div style="margin-bottom:10px;">
          <label for={`prop-attr-${key}`} style="font-size:9px; color:var(--cv-text-muted); display:block; margin-bottom:3px; text-transform:capitalize;">
            {key.replace(/_/g, ' ')}
          </label>
          {#if SELECT_ATTRS[key]}
            <select
              id={`prop-attr-${key}`}
              value={String(node.data[key] ?? nodeType.defaultData[key])}
              onchange={(e) => update(key, (e.target as HTMLSelectElement).value)}
              style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text);"
            >
              {#each SELECT_ATTRS[key] as opt}
                <option value={opt}>{opt}</option>
              {/each}
            </select>
          {:else if key === 'temperature'}
            <div class="flex items-center gap-2">
              <input id={`prop-attr-${key}`} type="range" min="0" max="2" step="0.1"
                value={Number(node.data[key] ?? nodeType.defaultData[key])}
                oninput={(e) => update(key, parseFloat((e.target as HTMLInputElement).value))}
                style="flex:1;"
              />
              <span style="font-size:10px; color:var(--cv-text-2); min-width:24px;">{node.data[key] ?? nodeType.defaultData[key]}</span>
            </div>
          {:else if TEXTAREA_ATTRS.has(key)}
            <textarea
              id={`prop-attr-${key}`}
              rows="4"
              value={String(node.data[key] ?? nodeType.defaultData[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLTextAreaElement).value)}
              style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); resize:vertical; box-sizing:border-box;"
            ></textarea>
          {:else if MASKED_ATTRS.has(key)}
            <input id={`prop-attr-${key}`} type="password"
              value={String(node.data[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLInputElement).value)}
              style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); box-sizing:border-box;"
            />
          {:else}
            <input id={`prop-attr-${key}`} type="text"
              value={String(node.data[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLInputElement).value)}
              style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); box-sizing:border-box;"
            />
          {/if}
        </div>
      {/each}

      <!-- Connectors (read-only) -->
      {#if nodeType.inputs.length > 0 || nodeType.outputs.length > 0}
      <div style="margin-top:8px;">
        <div style="font-size:9px; color:var(--cv-text-muted); margin-bottom:6px;">{m.propertiespanel_connectors()}</div>
        {#each nodeType.inputs as inp}
          <div style="background:var(--cv-surface); border-radius:4px; padding:4px 8px; margin-bottom:3px; font-size:9px; color:var(--cv-accent-2);">{m.propertiespanel_in({ inp })}</div>
        {/each}
        {#each nodeType.outputs as out}
          <div style="background:var(--cv-surface); border-radius:4px; padding:4px 8px; margin-bottom:3px; font-size:9px; color:var(--cv-accent);">{m.propertiespanel_out({ out })}</div>
        {/each}
      </div>
      {/if}
    </div>
  {/if}
</aside>
