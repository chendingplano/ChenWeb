<!-- web/src/lib/components/shared-ui/canvas/NodeDescriptorEditor.svelte -->
<!-- The properties panel's Node tab: the selected node's descriptor, the JSON View JSON shows,
     whose attributes are added, renamed, changed and removed here. The panel mounts it again
     for each node, so nothing typed carries over to the next one. -->
<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import type { Node } from '@xyflow/svelte';
  import type { JsonObject } from '$lib/wpi/types';
  import { valueText } from './element-attributes';
  import { isElementNode, nodeData, nodeJson } from './wpi-json';
  import { isClassKey, nameProblem, renamed, withValue, without, type NameProblem } from './node-descriptor';

  let {
    node,
    onUpdate,
  }: {
    node:     Node;
    onUpdate: (nodeId: string, data: Record<string, unknown>) => void;
  } = $props();

  const descriptor = $derived(nodeJson(node));

  function set(next: JsonObject) {
    onUpdate(node.id, nodeData(node, next));
  }

  function problemText(p: NameProblem, name: string): string {
    return p === 'name' ? m.propertiespanel_err_attribute_name() : m.propertiespanel_err_duplicate({ name });
  }

  // The value being typed in. Its text stays as typed until the field is left,
  // so reading it as JSON does not rewrite it under the cursor.
  let typing = $state<{ name: string; text: string } | null>(null);

  function valueOf(name: string): string {
    return typing?.name === name ? typing.text : valueText(descriptor[name]);
  }

  function typeValue(name: string, text: string) {
    typing = { name, text };
    set(withValue(node, descriptor, name, text));
  }

  // A rename is made when its field is left or Enter is pressed. A name that
  // cannot be used is put back, and the problem shown under the attribute.
  let renameError = $state<{ name: string; text: string } | null>(null);

  function rename(input: HTMLInputElement, from: string) {
    const to = input.value.trim();
    renameError = null;
    if (to === from) {
      input.value = from;
      return;
    }
    const p = nameProblem(descriptor, to);
    if (p) {
      input.value = from;
      renameError = { name: from, text: problemText(p, to) };
      return;
    }
    set(renamed(descriptor, from, to));
  }

  let newName  = $state('');
  let newValue = $state('');
  let addError = $state('');

  function add(e: SubmitEvent) {
    e.preventDefault();
    const name = newName.trim();
    const p = nameProblem(descriptor, name);
    if (p) {
      addError = problemText(p, name);
      return;
    }
    set(withValue(node, descriptor, name, newValue));
    newName = '';
    newValue = '';
    addError = '';
  }
</script>

<div class="flex-1 overflow-y-auto" style="padding:10px 12px; scrollbar-width:thin; scrollbar-color:var(--cv-border) transparent;">
  {#if !isElementNode(node)}
    <div class="hint" style="margin:0 0 10px;">{m.propertiespanel_node_data_note()}</div>
  {/if}

  {#each Object.keys(descriptor) as name (name)}
    {@const fixed = isClassKey(node, name)}
    <div style="margin-bottom:6px;">
      <div class="row">
        <input type="text" class="field" value={name} readonly={fixed} spellcheck="false" autocomplete="off"
          onchange={(e) => rename(e.currentTarget, name)}
        />
        <textarea class="field" rows="1" value={valueOf(name)} spellcheck="false"
          oninput={(e) => typeValue(name, e.currentTarget.value)}
          onblur={() => (typing = null)}
        ></textarea>
        {#if !fixed}
          <button type="button" class="icon" onclick={() => set(without(descriptor, name))}
            title={m.propertiespanel_remove_attribute({ name })}>✕</button>
        {/if}
      </div>
      {#if renameError?.name === name}
        <div class="error">{renameError.text}</div>
      {/if}
    </div>
  {/each}
  <div class="hint">{m.propertiespanel_node_value_hint()}</div>

  <!-- New attribute; Enter in its name adds it -->
  <form onsubmit={add} oninput={() => (addError = '')} style="margin-top:12px; padding-top:10px; border-top:1px solid var(--cv-divider);">
    <div style="font-size:9px; color:var(--cv-text-muted); margin-bottom:4px;">{m.propertiespanel_add_attribute()}</div>
    <div class="row">
      <input type="text" class="field" bind:value={newName} placeholder={m.propertiespanel_new_attribute_name()} spellcheck="false" autocomplete="off" />
      <textarea class="field" rows="1" bind:value={newValue} placeholder={m.propertiespanel_new_attribute_value()} spellcheck="false"></textarea>
      <button type="submit" class="icon" title={m.propertiespanel_add_attribute()}>+</button>
    </div>
    {#if addError}
      <div class="error">{addError}</div>
    {/if}
  </form>
</div>

<style>
  .row { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 3fr) 16px; gap: 4px; align-items: start; }
  .field {
    width: 100%; box-sizing: border-box;
    background: var(--cv-surface); border: 1px solid var(--cv-border); border-radius: 4px;
    padding: 4px 6px; font-size: 10px; color: var(--cv-text);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .field[readonly] { background: transparent; color: var(--cv-text-2); }
  textarea.field { resize: vertical; field-sizing: content; }
  .icon { background: none; border: none; cursor: pointer; color: var(--cv-text-muted); font-size: 12px; line-height: 22px; padding: 0; }
  .icon:hover { color: var(--cv-text); }
  .hint { font-size: 9px; color: var(--cv-text-faint); margin-top: 2px; }
  .error { font-size: 9px; color: #ef4444; margin-top: 2px; }
</style>
