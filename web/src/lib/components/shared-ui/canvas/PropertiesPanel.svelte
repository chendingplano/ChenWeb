<!-- web/src/lib/components/shared-ui/canvas/PropertiesPanel.svelte -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
  import type { FlowNode, NodeType } from '$lib/types/flow';
  import type { ElementAttribute, ElementClassSummary } from '$lib/wpi/api';
  import type { JsonObject } from '$lib/wpi/types';
  import { localized } from '$lib/wpi/text';
  import { CHAT_AGENTS } from '$lib/wpi/chat-agents';
  import { valueText, withAttribute, withSourceFields, withSourceProperty, withListSourceDefaults, withSelectedTable } from './element-attributes';
  import NodeDescriptorEditor from './NodeDescriptorEditor.svelte';
  import TablePicker from '../database-picker/TablePicker.svelte';
  import FieldPicker from '../database-picker/FieldPicker.svelte';

  let {
    node,
    nodeType,
    elementClass,
    measuredSize,
    triggerActions = [],
    panes,
    onMovePane,
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
    measuredSize?: { width: number; height: number };
    triggerActions?: string[];
    // The leaf panes of the page, for moving an element node to another one.
    panes?:    string[];
    onMovePane?: (nodeId: string, pane: string) => void;
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

  function attributeLabel(name: string): string {
    switch (name) {
      case 'label': return m.propertiespanel_attr_label();
      case 'data_binding': return m.wpi_input_attr_data_binding();
      case 'binding_mode': return m.wpi_input_attr_binding_mode();
      case 'input_type': return m.wpi_input_attr_input_type();
      case 'label_placement': return m.wpi_input_attr_label_placement();
      case 'label_horizontal_gap': return m.wpi_input_attr_label_horizontal_gap();
      case 'label_vertical_gap': return m.wpi_input_attr_label_vertical_gap();
      case 'label_horizontal_align': return m.wpi_input_attr_label_horizontal_align();
      case 'label_vertical_align': return m.wpi_input_attr_label_vertical_align();
      case 'label_max_width': return m.wpi_input_attr_label_max_width();
      case 'label_max_lines': return m.wpi_input_attr_label_max_lines();
      case 'label_font_family': return m.wpi_input_attr_label_font_family();
      case 'label_font_size': return m.wpi_input_attr_label_font_size();
      case 'label_color': return m.wpi_input_attr_label_color();
      case 'label_font_weight': return m.wpi_input_attr_label_font_weight();
      case 'label_font_style': return m.wpi_input_attr_label_font_style();
      case 'placeholder': return m.wpi_input_attr_placeholder();
      case 'help_text': return m.wpi_input_attr_help_text();
      case 'help_tip': return m.wpi_input_attr_help_tip();
      case 'autocomplete': return m.wpi_input_attr_autocomplete();
      case 'required': return m.wpi_input_attr_required();
      case 'max_length': return m.wpi_input_attr_max_length();
      case 'validation': return m.wpi_input_attr_validation();
      case 'min_length': return m.wpi_input_attr_min_length();
      case 'pattern': return m.wpi_input_attr_pattern();
      case 'min': return m.wpi_input_attr_min();
      case 'max': return m.wpi_input_attr_max();
      case 'step': return m.wpi_input_attr_step();
      case 'min_width': return m.wpi_input_attr_min_width();
      case 'max_width': return m.wpi_input_attr_max_width();
      case 'rows': return m.wpi_chat_attr_rows();
      case 'slash_commands': return m.wpi_chat_attr_slash_commands();
      case 'toolbar': return m.wpi_chat_attr_toolbar();
      case 'agents': return m.wpi_chat_attr_agents();
      case 'default_agent': return m.wpi_chat_attr_default_agent();
      case 'default_model': return m.wpi_chat_attr_default_model();
      case 'tabs': return m.wpi_chat_attr_tabs();
      case 'initial_tab': return m.wpi_chat_attr_initial_tab();

      case 'width': return m.propertiespanel_attr_width();
      case 'height': return m.propertiespanel_attr_height();
      case 'label_hori_align': return m.propertiespanel_attr_horizontal_alignment();
      case 'label_vert_align': return m.propertiespanel_attr_vertical_alignment();
      case 'on_click': return m.propertiespanel_attr_on_click();
      default: return name.replace(/_/g, ' ').replace(/^./, (c) => c.toUpperCase());
    }
  }

  function actionLabel(id: string): string {
    switch (id) {
      case 'save': return m.propertiespanel_trigger_save();
      case 'save-page': return m.propertiespanel_trigger_save_page();
      case 'open-page': return m.propertiespanel_trigger_open_page();
      case 'close-page': return m.propertiespanel_trigger_close_page();
      default: return id.replace(/-/g, ' ');
    }
  }

  const inputGroups: [string, string[]][] = [
    ['general', ['label','data_binding','binding_mode','input_type']],
    ['caption_layout', ['label_placement','label_horizontal_gap','label_vertical_gap','label_horizontal_align','label_vertical_align','label_max_width','label_max_lines']],
    ['caption_appearance', ['label_font_family','label_font_size','label_color','label_font_weight','label_font_style']],
    ['input_help', ['placeholder','help_text','help_tip','autocomplete']],
    ['validation', ['required','max_length','validation']],
    ['size', ['width','height','min_width','max_width']]
  ];
  // Properties groups by code module (code_modules/interpreter-*.md); a class
  // is grouped by the module that draws it.
  const moduleGroups: Record<string, [string, string[]][]> = {
    input: inputGroups,
    'chat-composer': [
      ['message_box', ['placeholder','rows','slash_commands']],
      ['toolbar', ['toolbar']],
      ['assistant', ['agents','default_agent','default_model']],
      ['size', ['width','height','min_width','max_width']]
    ],
    'chat-panel': [
      ['tabs', ['tabs','initial_tab']],
      ['size', ['width','height','min_width','max_width']]
    ]
  };
  const module = $derived(elementClass?.code_module_id ?? String(node?.data.element ?? ''));
  function groupLabel(id: string): string {
    switch (id) {
      case 'general': return m.wpi_input_group_general();
      case 'caption_layout': return m.wpi_input_group_caption_layout();
      case 'caption_appearance': return m.wpi_input_group_caption_appearance();
      case 'input_help': return m.wpi_input_group_input_help();
      case 'validation': return m.wpi_input_group_validation();
      case 'size': return m.wpi_input_group_size();
      case 'message_box': return m.wpi_chat_group_message_box();
      case 'toolbar': return m.wpi_chat_group_toolbar();
      case 'assistant': return m.wpi_chat_group_assistant();
      case 'tabs': return m.wpi_chat_group_tabs();
      default: return m.wpi_input_group_other();
    }
  }

  // Ordered choices of the chat modules: checked entries are stored, in order.
  // An absent attribute means all of them; agents and tabs keep at least one.
  const chatLists: Record<string, Record<string, string[]>> = {
    'chat-composer': { toolbar: ['agent','model','prompt','text_editor','new_session'], agents: CHAT_AGENTS },
    'chat-panel': { tabs: ['chat','dialog'] }
  };
  function chatToken(name: string, token: string): string {
    if (name === 'toolbar') {
      switch (token) {
        case 'agent': return m.wpi_chat_control_agent();
        case 'model': return m.wpi_chat_control_model();
        case 'prompt': return m.wpi_chat_control_prompt();
        case 'text_editor': return m.wpi_chat_control_text_editor();
        case 'new_session': return m.wpi_chat_control_new_session();
      }
    }
    if (name === 'tabs') return token === 'chat' ? m.wpi_chat_tab_chat() : m.wpi_chat_tab_dialog();
    return token;
  }
  function chatList(n: FlowNode, name: string, attr: ElementAttribute | undefined): string[] {
    const value = attrsOf(n)[name] ?? attr?.default_value;
    return Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : chatLists[module]?.[name] ?? [];
  }
  function setChatList(n: FlowNode, name: string, next: string[]) {
    if (!next.length && name !== 'toolbar') return;
    update('attrs', { ...attrsOf(n), [name]: next });
  }
  function moved(list: string[], at: number, by: number): string[] {
    const out = [...list];
    [out[at], out[at + by]] = [out[at + by], out[at]];
    return out;
  }
  function setChoice(n: FlowNode, name: string, value: string) {
    if (value) update('attrs', { ...attrsOf(n), [name]: value });
    else resetAttribute(n, name);
  }

  function orderedAttributes(attributes: Record<string, ElementAttribute>): [string, ElementAttribute | null][] {
    const groups = moduleGroups[module];
    if (groups) {
      const out: [string, ElementAttribute | null][] = [['__pane', null]];
      const used = new Set<string>();
      for (const [group, names] of groups) {
        const present = names.filter(name => name in attributes);
        if (!present.length) continue;
        out.push([`__group_${group}`, null]);
        for (const name of present) { out.push([name, attributes[name]]); used.add(name); }
      }
      const other = Object.keys(attributes).filter(name => !used.has(name));
      if (other.length) { out.push(['__group_other', null]); for (const name of other) out.push([name, attributes[name]]); }
      return out;
    }
    const order = ['label', 'width', 'height', 'label_hori_align', 'label_vert_align', '__pane', 'on_click'];
    const entries: [string, ElementAttribute | null][] = Object.entries(attributes);
    entries.push(['__pane', null]);
    return entries.sort(([a], [b]) => {
      const ai = order.indexOf(a), bi = order.indexOf(b);
      return (ai < 0 ? order.length : ai) - (bi < 0 ? order.length : bi);
    });
  }

  const inputChoices: Record<string, string[]> = {
    input_type: ['text','password','email','number','tel','url','search'],
    binding_mode: ['read-only','read-write'], label_placement: ['left','top','right','bottom'],
    label_horizontal_align: ['left','center','right'], label_vertical_align: ['top','center','bottom'],
    label_font_style: ['normal','italic','oblique']
  };
  function inputAttrVisible(n: FlowNode, name: string): boolean {
    if (n.data.element !== 'input' || !elementClass) return true;
    const attrs = attrsOf(n);
    const get = (key: string) => attrs[key] ?? elementClass?.attributes[key]?.default_value;
    const horizontal = ['left','right'].includes(String(get('label_placement') ?? 'left'));
    if (name === 'label_horizontal_gap' || name === 'label_vertical_align') return horizontal;
    if (name === 'label_vertical_gap') return !horizontal;
    if (name === 'max_length') return get('input_type') !== 'number';
    return true;
  }
  function resetAttribute(n: FlowNode, name: string) {
    const attrs = { ...attrsOf(n) }; delete attrs[name]; update('attrs', attrs);
  }
  function setValidation(node: FlowNode, attr: ElementAttribute, key: string, raw: string) {
    const attrs = attrsOf(node);
    const initial = attrs.validation ?? attr.default_value;
    const rules: JsonObject = initial && typeof initial === 'object' && !Array.isArray(initial) ? { ...initial } : {};
    if (!raw) delete rules[key]; else rules[key] = key === 'pattern' ? raw : Number(raw);
    update('attrs', { ...attrs, validation: rules });
  }

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
  let newColumn = $state({ label: '', field: '', width: '' });
  let addingColumn = $state(false);
  let addingOrder = $state(false);
  let newOrder = $state({ column: '', direction: 'asc' });

  function columnsOf(n: FlowNode, attr: ElementAttribute): { label: string; field: string; width?: string }[] {
    const value = attrsOf(n).columns ?? attr.default_value;
    return Array.isArray(value) ? value as { label: string; field: string; width?: string }[] : [];
  }

  $effect(() => {
    node?.id;
    newColumn = { label: '', field: '', width: '' };
    addingColumn = false;
    addingOrder = false;
    newOrder = { column: '', direction: 'asc' };
  });

  function addColumn(attr: ElementAttribute) {
    if (!node || !newColumn.label.trim() || !newColumn.field.trim()) return;
    const columns = columnsOf(node, attr);
    update('attrs', withAttribute(attrsOf(node), 'columns', JSON.stringify([...columns, { label: newColumn.label.trim(), field: newColumn.field.trim(), ...(newColumn.width.trim() ? { width: newColumn.width.trim() } : {}) }])));
    newColumn = { label: '', field: '', width: '' };
    addingColumn = false;
  }

  function attrText(n: FlowNode, name: string, attr: ElementAttribute): string {
    if (typing && typing.nodeId === n.id && typing.name === name) return typing.text;
    const value = attrsOf(n)[name] ?? attr.default_value;
    if ((name === 'width' || name === 'height') && measuredSize && (value === undefined || value === null || value === 'auto')) return `${measuredSize[name]}px`;
    return valueText(value ?? undefined);
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
      {#if elementClass !== undefined}
        {@const elementName = String(node.data.label ?? nodeType.label)}
        {@const descriptorId = String(node.data.element ?? '')}
        <div style="font-size:14px; font-weight:700; color:var(--cv-text); margin-bottom:10px;">{m.propertiespanel_element_title({ name: elementName })}{#if descriptorId && descriptorId !== elementName} ({descriptorId}){/if}</div>
      {:else}
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
      {/if}
    </div>

    <div class="flex-1 overflow-y-auto" style="padding:10px 12px; scrollbar-width:thin; scrollbar-color:var(--cv-border) transparent;">
      {#if elementClass !== undefined}
        {#if elementClass === null}
          <div style="margin-bottom:10px;">
            <div style="font-size:9px; color:var(--cv-text-muted); margin-bottom:3px;">{m.wpi_element_pane()}</div>
            <div style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); box-sizing:border-box;">{String(node.data.pane ?? '')}</div>
          </div>
          <div style="font-size:10px; color:var(--cv-text-faint);">{m.propertiespanel_element_class_unavailable({ id: String(node.data.element ?? '') })}</div>
        {:else}
          {#each orderedAttributes(elementClass.attributes) as [name, attr] (name)}
            {#if name === '__pane'}
              <div style="margin-bottom:10px;">
                <div style="font-size:9px; color:var(--cv-text-muted); margin-bottom:3px;">{m.wpi_element_pane()}</div>
                <div style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); box-sizing:border-box;">{String(node.data.pane ?? '')}</div>
              </div>
            {:else if name.startsWith('__group_')}
              <h3 style="font-size:10px; font-weight:700; color:var(--cv-text); margin:12px 0 6px;">{groupLabel(name.slice(8))}</h3>
            {:else if attr && inputAttrVisible(node, name)}
            <div style="margin-bottom:10px;">
              <div style="display:flex; align-items:center; justify-content:space-between; margin-bottom:3px;">
                <label for={`prop-attr-${name}`} style="font-size:9px; color:var(--cv-text-muted);">
                  {attributeLabel(name)}{#if attr.default_value === null}<span title={m.propertiespanel_required()} style="color:var(--cv-accent);"> *</span>{/if}
                </label>
                {#if moduleGroups[module] && attr.instance_modifiable !== false && name in attrsOf(node)}
                  <button type="button" class="column-remove" onclick={() => resetAttribute(node, name)}>{m.wpi_input_reset()}</button>
                {/if}
                {#if name === 'columns' && node.data.element === 'list' && attr.instance_modifiable !== false}
                  <div style="display:flex; gap:3px;">
                    <button class="column-remove" type="button" onclick={() => (addingColumn = true)}>{m.propertiespanel_add_column()}</button>
                    {#if addingColumn}
                      <button class="column-remove" type="button" onclick={() => { addingColumn = false; newColumn = { label: '', field: '', width: '' }; }}>{m.wpi_create_element_cancel()}</button>
                    {/if}
                  </div>
                {/if}
              </div>
              {#if attr.instance_modifiable === false}
                <!-- The class fixes it: every instance has the class's value, so the node cannot set it. -->
                <textarea
                  id={`prop-attr-${name}`}
                  rows="1"
                  readonly
                  value={valueText(attr.default_value)}
                  style="width:100%; background:transparent; border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text-2); resize:none; field-sizing:content; box-sizing:border-box;"
                ></textarea>
                <div style="font-size:9px; color:var(--cv-text-faint); margin-top:2px;">{m.propertiespanel_fixed()}</div>
              {:else if node.data.element === 'input' && inputChoices[name]}
                <select id={`prop-attr-${name}`} class="column-input" value={String(attrsOf(node)[name] ?? attr.default_value ?? '')} onchange={(e) => update('attrs', withAttribute(attrsOf(node), name, e.currentTarget.value))}>
                  {#each inputChoices[name] as choice}<option value={choice}>{choice}</option>{/each}
                </select>
              {:else if chatLists[module]?.[name]}
                {@const all = chatLists[module][name]}
                {@const chosen = chatList(node, name, attr)}
                <ul style="list-style:none; margin:0; padding:0;">
                  {#each [...chosen, ...all.filter((t) => !chosen.includes(t))] as token (token)}
                    {@const at = chosen.indexOf(token)}
                    <li style="display:flex; align-items:center; gap:4px; font-size:10px; color:var(--cv-text); margin-bottom:2px;">
                      <input id={`prop-${name}-${token}`} type="checkbox" checked={at >= 0}
                        onchange={(e) => setChatList(node, name, e.currentTarget.checked ? [...chosen, token] : chosen.filter((t) => t !== token))} />
                      <label for={`prop-${name}-${token}`} style="flex:1;">{chatToken(name, token)}</label>
                      {#if at >= 0}
                        <button type="button" class="column-remove" aria-label={m.wpi_chat_move_up({ name: chatToken(name, token) })} disabled={at === 0} onclick={() => setChatList(node, name, moved(chosen, at, -1))}>↑</button>
                        <button type="button" class="column-remove" aria-label={m.wpi_chat_move_down({ name: chatToken(name, token) })} disabled={at === chosen.length - 1} onclick={() => setChatList(node, name, moved(chosen, at, 1))}>↓</button>
                      {/if}
                    </li>
                  {/each}
                </ul>
              {:else if (module === 'chat-composer' && name === 'default_agent') || (module === 'chat-panel' && name === 'initial_tab')}
                {@const list = chatList(node, name === 'default_agent' ? 'agents' : 'tabs', elementClass.attributes[name === 'default_agent' ? 'agents' : 'tabs'])}
                <select id={`prop-attr-${name}`} class="column-input" value={String(attrsOf(node)[name] ?? attr.default_value ?? '')} onchange={(e) => setChoice(node, name, e.currentTarget.value)}>
                  <option value="">{m.wpi_chat_choice_first()}</option>
                  {#each list as choice (choice)}<option value={choice}>{name === 'initial_tab' ? chatToken('tabs', choice) : choice}</option>{/each}
                </select>
              {:else if module === 'chat-composer' && name === 'slash_commands'}
                <input id={`prop-attr-${name}`} type="checkbox" checked={(attrsOf(node)[name] ?? attr.default_value ?? true) !== false} onchange={(e) => update('attrs', { ...attrsOf(node), slash_commands: e.currentTarget.checked })} />
              {:else if module === 'chat-composer' && name === 'rows'}
                <input id={`prop-attr-${name}`} class="column-input" type="number" min="1" max="20" step="1" value={String(attrsOf(node)[name] ?? attr.default_value ?? 3)}
                  oninput={(e) => { const n = e.currentTarget.valueAsNumber; if (Number.isInteger(n)) update('attrs', { ...attrsOf(node), rows: n }); }} />
              {:else if node.data.element === 'input' && name === 'required'}
                <input id={`prop-attr-${name}`} type="checkbox" checked={Boolean(attrsOf(node)[name] ?? attr.default_value)} onchange={(e) => update('attrs', { ...attrsOf(node), required: e.currentTarget.checked })} />
              {:else if node.data.element === 'input' && name === 'validation'}
                {@const rulesValue = attrsOf(node).validation ?? attr.default_value}
                {@const rules = rulesValue && typeof rulesValue === 'object' && !Array.isArray(rulesValue) ? rulesValue : {}}
                {#each (String(attrsOf(node).input_type ?? elementClass.attributes.input_type?.default_value ?? 'text') === 'number' ? ['min','max','step'] : ['min_length','pattern']) as rule}
                  <label for={`prop-rule-${rule}`} style="font-size:9px; display:block; margin-top:4px;">{attributeLabel(rule)}</label>
                  <input id={`prop-rule-${rule}`} class="column-input" type={rule === 'pattern' ? 'text' : 'number'} value={String(rules[rule] ?? '')} oninput={(e) => setValidation(node, attr, rule, e.currentTarget.value)} />
                {/each}
              {:else if name === 'on_click'}
                {@const actionValue = attrsOf(node)[name] ?? attr.default_value}
                {@const action = actionValue && typeof actionValue === 'object' && !Array.isArray(actionValue) ? actionValue : {}}
                {@const actionId = typeof action.action === 'string' ? action.action : ''}
                <select id={`prop-attr-${name}`} value={actionId} onchange={(e) => {
                  const selected = e.currentTarget.value;
                  if (selected === actionId) return;
                  update('attrs', withAttribute(attrsOf(node), name, selected ? JSON.stringify({ action: selected }) : ''));
                }} style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); box-sizing:border-box;">
                  <option value="">{m.propertiespanel_select_action()}</option>
                  {#if actionId && !triggerActions.includes(actionId)}<option value={actionId}>{actionId}</option>{/if}
                  {#each triggerActions as id (id)}<option value={id}>{actionLabel(id)}</option>{/each}
                </select>
              {:else if name === 'source' && node.data.element === 'list'}
                {@const sourceValue = attrsOf(node).source ?? attr.default_value}
                {@const source = sourceValue && typeof sourceValue === 'object' && !Array.isArray(sourceValue) ? sourceValue : {}}
                {@const tableValue = String(source.table ?? '')}
                <div class="source-properties">
                  <label for="source-limit">{m.propertiespanel_source_limit()}</label>
                  <input id="source-limit" class="column-input" type="number" min="1" step="1" value={source.limit ?? ''} oninput={(e) => { const value = (e.target as HTMLInputElement).value; update('attrs', withSourceProperty(attrsOf(node), attr.default_value, 'limit', value === '' ? undefined : Number(value))); }} />
                  <span>{m.propertiespanel_source_table()}</span>
                  <TablePicker value={tableValue} onSelect={(value) => update('attrs', withSelectedTable(attrsOf(node), attr.default_value, value))} />
                  <span>{m.propertiespanel_source_fields()}</span>
                  <FieldPicker table={tableValue} fields={Array.isArray(source.fields) ? source.fields : []} onChange={(fields) => update('attrs', withListSourceDefaults(withSourceFields(attrsOf(node), attr.default_value, fields), attr.default_value))} />
                  <label for="source-action">{m.propertiespanel_source_action()}</label>
                  <select id="source-action" class="column-input" value={String(source.action || (tableValue ? 'load' : ''))} onchange={(e) => update('attrs', withSourceProperty(attrsOf(node), attr.default_value, 'action', (e.target as HTMLSelectElement).value))}>
                    <option value="">{m.propertiespanel_select_action()}</option><option value="load">{m.propertiespanel_action_load()}</option><option value="query">{m.propertiespanel_action_query()}</option>
                  </select>
                  <label for="source-result">{m.propertiespanel_source_result()}</label>
                  <select id="source-result" class="column-input" value={String(source.result || (tableValue ? 'list' : ''))} onchange={(e) => update('attrs', withSourceProperty(attrsOf(node), attr.default_value, 'result', (e.target as HTMLSelectElement).value))}>
                    <option value="">{m.propertiespanel_select_result()}</option><option value="list">{m.propertiespanel_result_list()}</option>
                  </select>
                </div>
              {:else if name === 'columns' && node.data.element === 'list'}
            {@const columns = columnsOf(node, attr)}
            {#if addingColumn}
              <div style="display:grid; grid-template-columns:minmax(0,1fr) minmax(0,1fr) minmax(0,0.8fr) 25px; gap:3px; margin-bottom:4px;">
                <input class="column-input" aria-label={m.propertiespanel_column_label()} placeholder={m.propertiespanel_column_label()} bind:value={newColumn.label} />
                <input class="column-input" aria-label={m.propertiespanel_column_field()} placeholder={m.propertiespanel_column_field()} bind:value={newColumn.field} />
                <input class="column-input" aria-label={m.propertiespanel_column_width()} placeholder={m.propertiespanel_column_width()} bind:value={newColumn.width} />
                <button class="column-remove" type="button" aria-label={m.wpi_json_apply()} disabled={!newColumn.label.trim() || !newColumn.field.trim()} onclick={() => addColumn(attr)}>+</button>
              </div>
            {/if}
            {#each columns as column, index (index)}
              <div style="display:grid; grid-template-columns:minmax(0,1fr) minmax(0,1fr) minmax(0,0.8fr) 25px; gap:3px; margin-bottom:4px;">
                <input class="column-input" aria-label={m.propertiespanel_column_label()} value={column.label} oninput={(e) => update('attrs', withAttribute(attrsOf(node), 'columns', JSON.stringify(columns.map((c, i) => i === index ? { ...c, label: (e.target as HTMLInputElement).value } : c))))} />
                <input class="column-input" aria-label={m.propertiespanel_column_field()} value={column.field} oninput={(e) => update('attrs', withAttribute(attrsOf(node), 'columns', JSON.stringify(columns.map((c, i) => i === index ? { ...c, field: (e.target as HTMLInputElement).value } : c))))} />
                <input class="column-input" aria-label={m.propertiespanel_column_width()} placeholder={m.propertiespanel_column_width()} value={column.width ?? ''} oninput={(e) => update('attrs', withAttribute(attrsOf(node), 'columns', JSON.stringify(columns.map((c, i) => i === index ? { ...c, width: (e.target as HTMLInputElement).value } : c))))} />
                <button class="column-remove" type="button" aria-label={m.propertiespanel_remove_column()} onclick={() => update('attrs', withAttribute(attrsOf(node), 'columns', JSON.stringify(columns.filter((_, i) => i !== index))))}>×</button>
              </div>
            {/each}

              {:else}
                <textarea
                  id={`prop-attr-${name}`}
                  rows="1"
                  value={attrText(node, name, attr)}
                  placeholder={attrPlaceholder(attr)}
                  oninput={(e) => typeAttr(name, (e.target as HTMLTextAreaElement).value)}
                  onblur={() => (typing = null)}
                  style="width:100%; background:var(--cv-surface); border:1px solid var(--cv-border); border-radius:4px; padding:4px 8px; font-size:10px; color:var(--cv-text); resize:vertical; field-sizing:content; box-sizing:border-box;"
                ></textarea>
              {/if}
              {#if name === 'source' && node.data.element === 'list' && attr.instance_modifiable !== false}
                {@const source = attrsOf(node).source ?? attr.default_value}
                {#if source && typeof source === 'object' && !Array.isArray(source)}
                  {@const order = Array.isArray(source.order_by) ? source.order_by : []}
                  <div style="display:flex; align-items:center; justify-content:space-between; margin:6px 0 3px;">
                    <span style="font-size:9px; color:var(--cv-text-muted);">{m.propertiespanel_order_by()}</span>
                    <div style="display:flex; gap:3px;">
                      <button class="column-remove" type="button" onclick={() => (addingOrder = true)}>{m.propertiespanel_add_order()}</button>
                      {#if addingOrder}<button class="column-remove" type="button" onclick={() => { addingOrder = false; newOrder = { column: '', direction: 'asc' }; }}>{m.wpi_create_element_cancel()}</button>{/if}
                    </div>
                  </div>
                  {#if addingOrder}
                    <div class="order-row">
                      <input class="column-input" aria-label={m.propertiespanel_source_column()} placeholder={m.propertiespanel_source_column()} bind:value={newOrder.column} />
                      <select class="column-input" aria-label={m.propertiespanel_direction()} bind:value={newOrder.direction}><option value="asc">{m.propertiespanel_direction_asc()}</option><option value="desc">{m.propertiespanel_direction_desc()}</option></select>
                      <button class="column-remove" type="button" aria-label={m.wpi_json_apply()} disabled={!newOrder.column.trim()} onclick={() => { update('attrs', withSourceProperty(attrsOf(node), attr.default_value, 'order_by', [...order, { column: newOrder.column.trim(), direction: newOrder.direction }])); newOrder = { column: '', direction: 'asc' }; addingOrder = false; }}>+</button>
                    </div>
                  {/if}
                  {#each order as item, index (index)}
                    {#if item && typeof item === 'object' && !Array.isArray(item)}
                      <div class="order-row">
                        <input class="column-input" aria-label={m.propertiespanel_source_column()} value={String(item.column ?? '')} oninput={(e) => update('attrs', withSourceProperty(attrsOf(node), attr.default_value, 'order_by', order.map((v, i) => i === index ? { ...item, column: (e.target as HTMLInputElement).value } : v)))} />
                        <select class="column-input" aria-label={m.propertiespanel_direction()} value={String(item.direction ?? 'asc')} onchange={(e) => update('attrs', withSourceProperty(attrsOf(node), attr.default_value, 'order_by', order.map((v, i) => i === index ? { ...item, direction: (e.target as HTMLSelectElement).value } : v)))}><option value="asc">{m.propertiespanel_direction_asc()}</option><option value="desc">{m.propertiespanel_direction_desc()}</option></select>
                        <button class="column-remove" type="button" aria-label={m.propertiespanel_remove_order()} onclick={() => update('attrs', withSourceProperty(attrsOf(node), attr.default_value, 'order_by', order.filter((_, i) => i !== index)))}>×</button>
                      </div>
                    {/if}
                  {/each}
                {/if}
              {/if}
              {#if attr.help_tip}
                <div style="font-size:9px; color:var(--cv-text-faint); margin-top:2px;">{localized(attr.help_tip)}</div>
              {/if}
            </div>
            {/if}
          {/each}
        {/if}
      {/if}

      <!-- Attributes -->
      {#each Object.entries(elementClass === undefined ? nodeType.defaultData : {}) as [key]}
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

<style>
  .source-properties { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 2fr); gap: 4px; align-items: center; font-size: 9px; color: var(--cv-text-muted); }
  .order-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 25px; gap: 3px; margin-bottom: 4px; }
  .column-input { min-width: 0; width: 100%; box-sizing: border-box; background: var(--cv-surface); border: 1px solid var(--cv-border); border-radius: 4px; padding: 4px; font-size: 10px; color: var(--cv-text); }
  .column-remove, .column-add { background: var(--cv-surface); border: 1px solid var(--cv-border); border-radius: 4px; padding: 4px 7px; font-size: 10px; color: var(--cv-text); cursor: pointer; }
  .column-add { margin-top: 5px; }
  .column-add:disabled, .column-remove:disabled { opacity: 0.5; cursor: default; }
</style>
