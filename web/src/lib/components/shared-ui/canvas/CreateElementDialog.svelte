<!-- web/src/lib/components/shared-ui/canvas/CreateElementDialog.svelte -->
<!-- The WPI editor's Create Element form: defines a new element class as a list of attributes,
     starting with its class attributes, and stores it as a draft, to be implemented in code
     afterwards (OpenSpec wpi-create-element-class). Double-clicking a value edits it; double-clicking
     an instance attribute's name renames it. The corner grip resizes the dialog. -->
<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { createElementClass, WpiError } from '$lib/wpi/api';
  import type { Issue } from '$lib/wpi/types';
  import {
    attributeProblem,
    buildClassRow,
    draftAttribute,
    editProblem,
    editText,
    initialAttributes,
    isClassAttribute,
    parseValue,
    renameProblem,
    rowProblem,
    type AttributeProblem,
    type DraftAttribute,
  } from './element-class-form';

  let {
    onCreated,
    onClose,
  }: {
    onCreated: (classId: string) => void;
    onClose:   () => void;
  } = $props();

  let attributes = $state<DraftAttribute[]>(initialAttributes());

  // The attribute being entered.
  let attrName     = $state('');
  let attrDefault  = $state('');
  let attrRequired = $state(false);
  let attrError    = $state('');

  // The attribute whose name or value is being edited in the list.
  let editing   = $state<{ name: string; field: 'name' | 'value' } | null>(null);
  let editValue = $state('');
  let editError = $state('');

  // The dialog's size once the author has resized it; until then it fits its content.
  const MIN_WIDTH = 480;
  const MIN_HEIGHT = 320;
  let panel = $state<HTMLDivElement>();
  let size  = $state<{ width: number; height: number } | null>(null);

  let formError = $state('');
  let issues    = $state<Issue[]>([]);
  let saving    = $state(false);

  // A changed attribute may have fixed what the last create reported.
  function clearFormError() {
    formError = '';
    issues = [];
  }

  function problemText(p: AttributeProblem, name: string): string {
    switch (p) {
      case 'name':        return m.wpi_create_element_err_attribute_name();
      case 'reserved':    return m.wpi_create_element_err_reserved({ name });
      case 'duplicate':   return m.wpi_create_element_err_duplicate({ name });
      case 'no_default':  return m.wpi_create_element_err_no_default();
      case 'empty_value': return m.wpi_create_element_err_empty_value();
    }
  }

  function addAttribute() {
    const name = attrName.trim();
    const p = attributeProblem(name, attrRequired, attrDefault, attributes);
    if (p) {
      attrError = problemText(p, name);
      return;
    }
    attributes = [...attributes, draftAttribute(name, attrRequired, attrDefault)];
    clearFormError();
    attrName = '';
    attrDefault = '';
    attrRequired = false;
    attrError = '';
  }

  function removeAttribute(name: string) {
    attributes = attributes.filter((a) => a.name !== name);
    clearFormError();
  }

  function isEditing(attr: DraftAttribute, field: 'name' | 'value') {
    return editing?.name === attr.name && editing.field === field;
  }

  function startEdit(attr: DraftAttribute, field: 'name' | 'value') {
    // A double-click inside the open editor selects a word; it must not restart the edit.
    if (isEditing(attr, field)) return;
    // Class attribute names are fixed.
    if (field === 'name' && isClassAttribute(attr.name)) return;
    editing = { name: attr.name, field };
    editValue = field === 'name' ? attr.name : editText(attr);
    editError = '';
  }

  function commitEdit() {
    if (editing === null) return;
    const { name, field } = editing;
    if (field === 'name') {
      const newName = editValue.trim();
      const p = renameProblem(name, newName, attributes);
      if (p) {
        editError = problemText(p, newName);
        return;
      }
      attributes = attributes.map((a) => (a.name === name ? { name: newName, value: a.value } : a));
    } else {
      const p = editProblem(name, editValue);
      if (p) {
        editError = problemText(p, name);
        return;
      }
      attributes = attributes.map((a) => (a.name === name ? { name, value: parseValue(name, editValue) } : a));
    }
    editing = null;
    editError = '';
    clearFormError();
  }

  function cancelEdit() {
    editing = null;
    editError = '';
  }

  function onEditKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      commitEdit();
    } else if (e.key === 'Escape') {
      // Escape leaves the edit, not the form.
      e.stopPropagation();
      cancelEdit();
    }
  }

  function focusSelect(node: HTMLInputElement) {
    node.focus();
    node.select();
  }

  function onEditStartKeydown(e: KeyboardEvent, attr: DraftAttribute, field: 'name' | 'value') {
    if (e.key === 'Enter' || e.key === 'F2') {
      e.preventDefault();
      startEdit(attr, field);
    }
  }

  // The dialog is centred, so it grows on both sides: twice the pointer's movement keeps the
  // corner under the pointer.
  function startResize(e: PointerEvent) {
    if (!panel) return;
    e.preventDefault();
    const rect = panel.getBoundingClientRect();
    const start = { x: e.clientX, y: e.clientY, width: rect.width, height: rect.height };
    const handle = e.currentTarget as HTMLElement;
    handle.setPointerCapture(e.pointerId);
    const move = (ev: PointerEvent) => {
      size = {
        width: Math.max(MIN_WIDTH, Math.min(window.innerWidth - 16, start.width + 2 * (ev.clientX - start.x))),
        height: Math.max(MIN_HEIGHT, Math.min(window.innerHeight - 16, start.height + 2 * (ev.clientY - start.y))),
      };
    };
    const stop = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', stop);
      handle.removeEventListener('pointercancel', stop);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', stop);
    handle.addEventListener('pointercancel', stop);
  }

  async function create() {
    clearFormError();
    const row = buildClassRow(attributes);
    const p = rowProblem(row);
    if (p === 'class_id') {
      formError = m.wpi_create_element_err_class_id();
      return;
    }
    if (p === 'description') {
      formError = m.wpi_create_element_err_description();
      return;
    }
    const id = row.descriptor_id as string;
    saving = true;
    try {
      await createElementClass(row);
      onCreated(id);
    } catch (e) {
      if (e instanceof WpiError && e.status === 409) formError = m.wpi_create_element_err_exists({ id });
      else if (e instanceof WpiError && e.status === 403) formError = m.wpi_create_element_err_admin();
      else if (e instanceof WpiError && e.issues.length > 0) {
        formError = m.wpi_create_element_err_refused();
        issues = e.issues;
      } else formError = m.wpi_create_element_err_failed({ error: e instanceof Error ? e.message : String(e) });
    } finally {
      saving = false;
    }
  }

  // Enter adds the attribute from the name field; in the default, Enter starts a new line, so
  // Ctrl/Cmd+Enter adds it.
  function onNameKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      addAttribute();
    }
  }

  function onDefaultKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      addAttribute();
    }
  }
</script>

<!-- Backdrop -->
<div class="fixed inset-0 z-50 flex items-center justify-center select-text"
  style="background: rgba(0,0,0,0.7);"
  onclick={(e) => { if (e.target === e.currentTarget) onClose(); }}
  onkeydown={(e) => { if (e.key === 'Escape') onClose(); }}
  role="dialog" aria-modal="true" aria-labelledby="create-element-title"
  tabindex="-1"
>
  <div bind:this={panel} class="rounded-xl overflow-hidden shadow-2xl"
    style="position:relative; background:#111827; border:1px solid #1e2a3a; display:flex; flex-direction:column;
      width:{size ? `${size.width}px` : '620px'}; height:{size ? `${size.height}px` : 'auto'};
      max-width:calc(100vw - 16px); max-height:{size ? 'calc(100vh - 16px)' : '85vh'};">

    <!-- Header -->
    <div style="padding:16px 20px; border-bottom:1px solid #1e2a3a; display:flex; justify-content:space-between; align-items:flex-start;">
      <div>
        <div id="create-element-title" style="font-size:15px; font-weight:600; color:#e2e8f0;">{m.wpi_create_element_title()}</div>
        <div style="font-size:11px; color:#6b7280; margin-top:2px;">{m.wpi_create_element_subtitle()}</div>
      </div>
      <button onclick={onClose} style="color:#6b7280; font-size:16px; background:none; border:none; cursor:pointer;" aria-label={m.wpi_create_element_close()}>✕</button>
    </div>

    <div style="flex:1; overflow-y:auto; padding:16px 20px;">
      <!-- New attribute -->
      <div class="section" style="margin-top:0;">{m.wpi_create_element_attributes()}</div>
      <div style="display:flex; gap:8px; align-items:flex-start; flex-wrap:wrap;">
        <label class="field" style="flex:0 1 180px; min-width:120px; margin:0;">
          <span>{m.wpi_create_element_attribute_name()}</span>
          <input bind:value={attrName} onkeydown={onNameKeydown} autocomplete="off" />
        </label>
        <label class="field" style="flex:1 1 240px; margin:0;">
          <span>{m.wpi_create_element_default_value()}</span>
          <textarea bind:value={attrDefault} onkeydown={onDefaultKeydown} disabled={attrRequired} rows="3" spellcheck="false"></textarea>
        </label>
      </div>
      <small class="hint">{m.wpi_create_element_default_hint()}</small>
      <div style="display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:8px;">
        <label style="display:flex; align-items:center; gap:6px; font-size:11px; color:#94a3b8;">
          <input type="checkbox" bind:checked={attrRequired} style="accent-color:#6366f1;" />
          {m.wpi_create_element_required()}
        </label>
        <button type="button" class="btn" onclick={addAttribute} title={m.wpi_create_element_add_attribute_shortcut()}>{m.wpi_create_element_add_attribute()}</button>
      </div>
      {#if attrError}
        <div class="error" style="margin-top:6px;">{attrError}</div>
      {/if}

      <!-- The class's attributes -->
      <div style="margin-top:12px; border:1px solid #1e2a3a; border-radius:6px; overflow:hidden;">
        <table style="width:100%; border-collapse:collapse; font-size:11px;">
          <thead>
            <tr style="background:#161b27; color:#6b7280; text-align:left;">
              <th style="padding:6px 12px; font-weight:500; width:30%;">{m.wpi_create_element_attribute_name()}</th>
              <th style="width:64px;"></th>
              <th style="padding:6px 12px; font-weight:500;">{m.wpi_create_element_value()}</th>
              <th style="width:32px;"></th>
            </tr>
          </thead>
          <tbody>
            {#each attributes as attr (attr.name)}
              <tr class="attr-row" style="border-top:1px solid #1e2a3a; color:#e2e8f0;">
                <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
                <td style="padding:3px 6px; font-family:monospace; word-break:break-all;" ondblclick={() => startEdit(attr, 'name')}>
                  {#if isEditing(attr, 'name')}
                    <input class="edit" bind:value={editValue} onkeydown={onEditKeydown} onblur={commitEdit}
                      aria-label={m.wpi_create_element_rename({ name: attr.name })} autocomplete="off" use:focusSelect />
                  {:else if isClassAttribute(attr.name)}
                    <span class="name">{attr.name}</span>
                  {:else}
                    <button type="button" class="value" title={m.wpi_create_element_double_click_to_rename()}
                      onkeydown={(e) => onEditStartKeydown(e, attr, 'name')}>{attr.name}</button>
                  {/if}
                </td>
                <td style="padding:0 6px;">
                  {#if isClassAttribute(attr.name)}<span class="tag">{m.wpi_create_element_class_attribute()}</span>{/if}
                </td>
                <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
                <td style="padding:3px 6px; font-family:monospace; word-break:break-all;" ondblclick={() => startEdit(attr, 'value')}>
                  {#if isEditing(attr, 'value')}
                    <input class="edit" bind:value={editValue} onkeydown={onEditKeydown} onblur={commitEdit}
                      aria-label={m.wpi_create_element_edit_value({ name: attr.name })} autocomplete="off" use:focusSelect />
                  {:else}
                    <button type="button" class="value" title={m.wpi_create_element_double_click_to_edit()}
                      onkeydown={(e) => onEditStartKeydown(e, attr, 'value')}>
                      {#if attr.name === 'descriptor_id' && attr.value === ''}
                        <span class="muted">{m.wpi_create_element_double_click_to_set()}</span>
                      {:else if attr.name === 'descriptor_id' || attr.name === 'schema_version'}
                        {attr.value}
                      {:else}
                        {JSON.stringify(attr.value)}
                        {#if attr.value === null}<span class="muted"> · {m.wpi_create_element_must_set()}</span>{/if}
                      {/if}
                    </button>
                  {/if}
                </td>
                <td style="padding:0 6px; text-align:center;">
                  {#if !isClassAttribute(attr.name)}
                    <button type="button" onclick={() => removeAttribute(attr.name)}
                      aria-label={m.wpi_create_element_remove_attribute({ name: attr.name })}
                      title={m.wpi_create_element_remove_attribute({ name: attr.name })}
                      style="color:#6b7280; background:none; border:none; cursor:pointer;">✕</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <small class="hint">{m.wpi_create_element_edit_hint()}</small>
      {#if editError}
        <div class="error" style="margin-top:6px;">{editError}</div>
      {/if}

      {#if formError}
        <div class="error" style="margin-top:12px;">
          {formError}
          {#if issues.length > 0}
            <ul style="margin:4px 0 0 16px; list-style:disc;">
              {#each issues as issue}
                <li><code>{issue.path || '/'}</code> {issue.message}</li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div style="padding:12px 20px; border-top:1px solid #1e2a3a; display:flex; justify-content:flex-end; gap:8px;">
      <button type="button" class="btn" onclick={onClose}>{m.wpi_create_element_cancel()}</button>
      <button type="button" class="btn primary" onclick={create} disabled={saving}>
        {saving ? m.wpi_create_element_creating() : m.wpi_create_element_create()}
      </button>
    </div>

    <!-- Resize grip -->
    <div class="grip select-none" title={m.wpi_create_element_resize()} aria-hidden="true" onpointerdown={startResize}></div>
  </div>
</div>

<style>
  .field { display: block; margin-bottom: 12px; }
  .field span { display: block; font-size: 10px; color: #6b7280; margin-bottom: 3px; }
  .field input, .field textarea {
    width: 100%; box-sizing: border-box;
    background: #1e2535; border: 1px solid #374151; border-radius: 4px;
    padding: 5px 8px; font-size: 12px; color: #e2e8f0;
  }
  .field textarea { display: block; min-height: 60px; resize: vertical; font-family: monospace; line-height: 1.4; }
  .field input:disabled, .field textarea:disabled { opacity: 0.4; }
  .hint { display: block; font-size: 10px; color: #4b5563; margin-top: 3px; }
  .section {
    font-size: 9px; color: #6366f1; letter-spacing: 1px; text-transform: uppercase;
    margin: 16px 0 8px;
  }
  .attr-row:hover { background: rgba(255, 255, 255, 0.03); }
  .value {
    display: block; width: 100%; text-align: left; padding: 3px 6px;
    background: none; border: 1px solid transparent; border-radius: 4px;
    color: inherit; font: inherit; cursor: text; user-select: text;
  }
  .value:focus-visible { border-color: #6366f1; outline: none; }
  .edit {
    width: 100%; box-sizing: border-box;
    background: #0d1117; border: 1px solid #6366f1; border-radius: 4px;
    padding: 3px 6px; font: inherit; color: #e2e8f0;
  }
  .name { display: block; padding: 3px 6px; border: 1px solid transparent; }
  .muted { color: #6b7280; font-family: system-ui, sans-serif; }
  .grip {
    position: absolute; right: 2px; bottom: 2px; width: 14px; height: 14px; cursor: nwse-resize;
    background: linear-gradient(135deg, transparent 0 50%, #4b5563 50% 58%, transparent 58% 70%, #4b5563 70% 78%, transparent 78%);
  }
  .tag {
    padding: 0 5px; border-radius: 3px;
    background: #1e2535; color: #818cf8; font-size: 9px; font-family: system-ui, sans-serif;
  }
  .btn {
    background: #1e2535; border: 1px solid #374151; border-radius: 6px;
    padding: 5px 12px; font-size: 11px; color: #e2e8f0; cursor: pointer; white-space: nowrap;
  }
  .btn:hover:not(:disabled) { background: #273046; }
  .btn.primary { background: #6366f1; border-color: #6366f1; color: #fff; }
  .btn.primary:hover:not(:disabled) { background: #4f46e5; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .error {
    padding: 8px 12px; background: #1f1313; border: 1px solid #ef4444; border-radius: 6px;
    font-size: 11px; color: #ef4444;
  }
</style>
