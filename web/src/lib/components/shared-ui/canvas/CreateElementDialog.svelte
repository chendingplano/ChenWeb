<!-- web/src/lib/components/shared-ui/canvas/CreateElementDialog.svelte -->
<!-- The WPI editor's Create Element form: defines a new element class (format 0.3) as a list of
     attributes, starting with its class attributes, and stores it as a draft, to be implemented in
     code afterwards (OpenSpec wpi-create-element-class). Each instance attribute is an object with a
     help tip and a default. Double-clicking a value, help tip or instance attribute name edits it.
     The corner grip resizes the dialog. Given a classId, the same form edits that class's latest
     row instead (from an element node's tool bar); its class ID is fixed, and saving stores the
     next version with the same status. -->
<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { onMount } from 'svelte';
  import { createElementClass, getElementClass, updateElementClass, WpiError } from '$lib/wpi/api';
  import type { Issue } from '$lib/wpi/types';
  import {
    attributeFields,
    attributeFromFields,
    attributeProblem,
    attributesFromRow,
    buildClassRow,
    defaultValue,
    hasDefault,
    helpTip,
    initialAttributes,
    isClassAttribute,
    newAttribute,
    rowProblem,
    type AttributeProblem,
    type DraftAttribute,
  } from './element-class-form';
  import { canvasThemeVars } from './canvas-theme';

  let {
    darkMode = true,
    classId,
    onSaved,
    onClose,
  }: {
    darkMode?: boolean;
    // The class to edit; without it the form creates a new class.
    classId?:  string;
    onSaved:   (classId: string) => void;
    onClose:   () => void;
  } = $props();

  let attributes = $state<DraftAttribute[]>(initialAttributes());

  // Editing: the class row is loaded first; its status is preserved when saved.
  const editMode = $derived(classId !== undefined);
  let loaded    = $state(false);
  let loadError = $state('');
  let status    = $state<'draft' | 'active'>('draft');

  onMount(async () => {
    if (classId === undefined) return;
    try {
      const row = await getElementClass(classId);
      attributes = attributesFromRow(row.class);
      status = row.status;
      loaded = true;
    } catch (e) {
      loadError = e instanceof WpiError && e.status === 404
        ? m.wpi_edit_element_err_not_found({ id: classId })
        : m.wpi_edit_element_err_load({ error: e instanceof Error ? e.message : String(e) });
    }
  });

  // The attribute being entered.
  let attrName     = $state('');
  let attrDefault  = $state('');
  let attrTipEn    = $state('');
  let attrTipZh    = $state('');
  let attrRequired = $state(false);
  let attrError    = $state('');
  // The attribute loaded into those fields to be modified, and the fields as loaded (to tell
  // whether they have changed). Unapplied changes are asked about before they would be lost.
  let attrEditing  = $state<string | null>(null);
  let attrBase     = $state('');
  let pendingGuard = $state<(() => void) | null>(null);
  const formKey    = () => JSON.stringify([attrName, attrDefault, attrTipEn, attrTipZh, attrRequired]);
  // A class attribute's name is fixed; a description is entered in the help tip fields, any other
  // class text in the default value field.
  const classEdit   = $derived(attrEditing !== null && isClassAttribute(attrEditing));
  const descEdit    = $derived(attrEditing === 'description');
  const attrDirty  = $derived(attrEditing !== null && formKey() !== attrBase);

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
      case 'help_tip_en': return m.wpi_create_element_err_help_tip_en();
    }
  }

  function resetAttrForm() {
    attrName = '';
    attrDefault = '';
    attrTipEn = '';
    attrTipZh = '';
    attrRequired = false;
    attrError = '';
    attrEditing = null;
  }

  function addAttribute() {
    const name = attrName.trim();
    const p = attributeProblem(name, attrTipEn, attrTipZh, attributes);
    if (p) {
      attrError = problemText(p, name);
      return;
    }
    attributes = [...attributes, newAttribute(name, attrRequired, attrDefault, attrTipEn, attrTipZh)];
    clearFormError();
    resetAttrForm();
  }

  // Replaces the attribute being edited with the fields' contents; false when they are refused.
  function modifyAttribute(): boolean {
    const old = attrEditing;
    if (old === null) return false;
    const name = attrName.trim();
    const p = isClassAttribute(old) ? null : attributeProblem(name, attrTipEn, attrTipZh, attributes.filter((a) => a.name !== old));
    if (p) {
      attrError = problemText(p, name);
      return false;
    }
    const next = attributeFromFields({ name, defaultText: attrDefault, tipEn: attrTipEn, tipZh: attrTipZh, required: attrRequired });
    attributes = attributes.map((a) => (a.name === old ? next : a));
    clearFormError();
    resetAttrForm();
    return true;
  }

  function submitAttribute() {
    if (attrEditing !== null) modifyAttribute();
    else addAttribute();
  }

  // Runs proceed now, or after the author has said what becomes of unapplied changes.
  function guard(proceed: () => void) {
    if (attrDirty) pendingGuard = proceed;
    else proceed();
  }

  function applyPending() {
    const proceed = pendingGuard;
    if (!modifyAttribute()) {
      pendingGuard = null;
      return;
    }
    pendingGuard = null;
    proceed?.();
  }

  function discardPending() {
    const proceed = pendingGuard;
    pendingGuard = null;
    resetAttrForm();
    proceed?.();
  }

  function editAttribute(attr: DraftAttribute) {
    if (attrEditing === attr.name) return;
    // An edited class keeps its ID.
    if (editMode && attr.name === 'descriptor_id') return;
    guard(() => {
      const f = attributeFields(attr);
      attrName = f.name;
      attrDefault = f.defaultText;
      attrTipEn = f.tipEn;
      attrTipZh = f.tipZh;
      attrRequired = f.required;
      attrError = '';
      attrEditing = attr.name;
      attrBase = formKey();
    });
  }

  function requestClose() {
    guard(onClose);
  }

  function removeAttribute(name: string) {
    attributes = attributes.filter((a) => a.name !== name);
    if (attrEditing === name) resetAttrForm();
    clearFormError();
  }

  const isOptionalText = (name: string) => name === 'description' || name === 'implement_notes';

  // These class attributes are always listed, so removing one empties it; an empty one is left
  // out of the stored row.
  function clearClassText(name: string) {
    attributes = attributes.map((a) => (a.name === name ? { name, value: name === 'description' ? { en: '', 'zh-cn': '' } : '' } : a));
    if (attrEditing === name) resetAttrForm();
    clearFormError();
  }

  function focusButton(node: HTMLButtonElement) {
    node.focus();
  }

  // A save's error sits below the attribute list, which may be longer than the dialog.
  function reveal(node: HTMLElement) {
    node.scrollIntoView({ block: 'nearest' });
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

  async function save() {
    clearFormError();
    const row = buildClassRow(attributes);
    const p = rowProblem(row);
    if (p === 'class_id') {
      formError = m.wpi_create_element_err_class_id();
      return;
    }
    // Only a draft needs a description: an active class's code says what it is.
    if (p === 'description' && !(editMode && status === 'active')) {
      formError = m.wpi_create_element_err_description();
      return;
    }
    const id = row.descriptor_id as string;
    saving = true;
    try {
      if (classId !== undefined) await updateElementClass(classId, row);
      else await createElementClass(row);
      onSaved(id);
    } catch (e) {
      if (e instanceof WpiError && e.status === 409) formError = m.wpi_create_element_err_exists({ id });
      else if (e instanceof WpiError && e.status === 404) formError = m.wpi_edit_element_err_not_found({ id });
      else if (e instanceof WpiError && e.status === 403) formError = editMode ? m.wpi_edit_element_err_admin() : m.wpi_create_element_err_admin();
      else if (e instanceof WpiError && e.issues.length > 0) {
        formError = editMode ? m.wpi_edit_element_err_refused() : m.wpi_create_element_err_refused();
        issues = e.issues;
      } else {
        const error = e instanceof Error ? e.message : String(e);
        formError = editMode ? m.wpi_edit_element_err_failed({ error }) : m.wpi_create_element_err_failed({ error });
      }
    } finally {
      saving = false;
    }
  }

  // Enter adds the attribute from the one-line fields; in the default, Enter starts a new line,
  // so Ctrl/Cmd+Enter adds it.
  function onLineKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      submitAttribute();
    }
  }

  function onDefaultKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      submitAttribute();
    }
  }

  function onConfirmKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      pendingGuard = null;
    }
  }
</script>

<!-- Backdrop. It renders outside the editor's root, so it sets the --cv-* colors itself. -->
<div class="fixed inset-0 z-50 flex items-center justify-center select-text"
  style="{canvasThemeVars(darkMode)} background: var(--cv-backdrop);"
  onclick={(e) => { if (e.target === e.currentTarget) requestClose(); }}
  onkeydown={(e) => { if (e.key === 'Escape') requestClose(); }}
  role="dialog" aria-modal="true" aria-labelledby="create-element-title"
  tabindex="-1"
>
  <div bind:this={panel} class="rounded-xl overflow-hidden shadow-2xl"
    style="position:relative; background:var(--cv-panel); border:1px solid var(--cv-divider); display:flex; flex-direction:column;
      width:{size ? `${size.width}px` : '760px'}; height:{size ? `${size.height}px` : 'auto'};
      max-width:calc(100vw - 16px); max-height:{size ? 'calc(100vh - 16px)' : '85vh'};">

    <!-- Header -->
    <div style="padding:16px 20px; border-bottom:1px solid var(--cv-divider); display:flex; justify-content:space-between; align-items:flex-start;">
      <div>
        <div id="create-element-title" style="font-size:15px; font-weight:600; color:var(--cv-text);">
          {classId !== undefined ? m.wpi_edit_element_title({ id: classId }) : m.wpi_create_element_title()}
        </div>
        <div style="font-size:11px; color:var(--cv-text-muted); margin-top:2px;">
          {#if !editMode}{m.wpi_create_element_subtitle()}
          {:else if loaded}{status === 'draft' ? m.wpi_edit_element_subtitle_draft() : m.wpi_edit_element_subtitle_active()}{/if}
        </div>
      </div>
      <button onclick={requestClose} style="color:var(--cv-text-muted); font-size:16px; background:none; border:none; cursor:pointer;" aria-label={m.wpi_create_element_close()}>✕</button>
    </div>

    <div style="flex:1; overflow-y:auto; padding:16px 20px;">
      {#if loadError}
        <div class="error">{loadError}</div>
      {:else if editMode && !loaded}
        <div style="font-size:11px; color:var(--cv-text-muted);">{m.wpi_edit_element_loading()}</div>
      {:else}
      <!-- New attribute; a change to any of its fields clears the last add's error -->
      <div class="section" style="margin-top:0;">{m.wpi_create_element_attributes()}</div>
      <div oninput={() => (attrError = '')}>
      <div style="display:flex; gap:8px; align-items:flex-start; flex-wrap:wrap;">
        <label class="field" style="flex:0 1 180px; min-width:120px; margin:0;">
          <span>{m.wpi_create_element_attribute_name()}</span>
          <input bind:value={attrName} onkeydown={onLineKeydown} disabled={classEdit} autocomplete="off" />
        </label>
        <label class="field" style="flex:1 1 240px; margin:0;">
          <span>{m.wpi_create_element_default_value()}</span>
          <textarea bind:value={attrDefault} onkeydown={onDefaultKeydown} disabled={attrRequired || descEdit} rows="3" spellcheck="false"></textarea>
        </label>
      </div>
      <small class="hint">{m.wpi_create_element_default_hint()}</small>
      <div style="display:flex; gap:8px; flex-wrap:wrap; margin-top:8px;">
        <label class="field" style="flex:1 1 200px; margin:0;">
          <span>{m.wpi_create_element_help_tip_en()}</span>
          <textarea bind:value={attrTipEn} onkeydown={onDefaultKeydown} disabled={classEdit && !descEdit} rows="3" spellcheck="false"></textarea>
        </label>
        <label class="field" style="flex:1 1 200px; margin:0;">
          <span>{m.wpi_create_element_help_tip_zh()}</span>
          <textarea bind:value={attrTipZh} onkeydown={onDefaultKeydown} disabled={classEdit && !descEdit} rows="3" spellcheck="false"></textarea>
        </label>
      </div>
      <div style="display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:8px;">
        <label style="display:flex; align-items:center; gap:6px; font-size:11px; color:var(--cv-text-2);">
          <input type="checkbox" bind:checked={attrRequired} disabled={classEdit} style="accent-color:var(--cv-accent);" />
          {m.wpi_create_element_required()}
        </label>
        {#if attrEditing !== null}
          <span style="display:flex; gap:8px;">
            <button type="button" class="btn" onclick={resetAttrForm}>{m.wpi_create_element_cancel()}</button>
            <button type="button" class="btn primary" onclick={modifyAttribute} title={m.wpi_create_element_modify_shortcut()}>{m.wpi_create_element_modify()}</button>
          </span>
        {:else}
          <button type="button" class="btn" onclick={addAttribute} title={m.wpi_create_element_add_attribute_shortcut()}>{m.wpi_create_element_add_attribute()}</button>
        {/if}
      </div>
      </div>
      {#if attrError}
        <div class="error" style="margin-top:6px;">{attrError}</div>
      {/if}

      <!-- The class's attributes -->
      <div style="margin-top:12px; border:1px solid var(--cv-divider); border-radius:6px; overflow:hidden;">
        <table style="width:100%; border-collapse:collapse; font-size:11px;">
          <thead>
            <tr style="background:var(--cv-raised); color:var(--cv-text-muted); text-align:left;">
              <th style="padding:6px 12px; font-weight:500; width:22%;">{m.wpi_create_element_attribute_name()}</th>
              <th style="width:56px;"></th>
              <th style="padding:6px 12px; font-weight:500; width:38%;">{m.wpi_create_element_value()}</th>
              <th style="padding:6px 12px; font-weight:500;">{m.wpi_create_element_help_tip()}</th>
              <th style="width:56px;"></th>
            </tr>
          </thead>
          <tbody>
            {#each attributes as attr (attr.name)}
              <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
              <tr class="attr-row" class:editing={attrEditing === attr.name} style="border-top:1px solid var(--cv-divider); color:var(--cv-text);"
                ondblclick={() => editAttribute(attr)}>
                <td style="padding:6px 12px; font-family:monospace; word-break:break-all;">{attr.name}</td>
                <td style="padding:0 6px;">
                  {#if isClassAttribute(attr.name)}<span class="tag">{m.wpi_create_element_class_attribute()}</span>{/if}
                </td>
                <td style="padding:6px 12px; font-family:monospace; word-break:break-all;">
                  {#if attr.name === 'descriptor_id' && attr.value === ''}
                    <span class="muted">{m.wpi_create_element_double_click_to_set()}</span>
                  {:else if attr.name === 'implement_notes' && attr.value === ''}
                    <span class="muted">{m.wpi_create_element_double_click_to_write_notes()}</span>
                  {:else if attr.name === 'implement_notes'}
                    <span class="notes">{attr.value}</span>
                  {:else if attr.name === 'descriptor_id' || attr.name === 'schema_version'}
                    {attr.value}
                  {:else if isClassAttribute(attr.name)}
                    {JSON.stringify(attr.value)}
                  {:else if !hasDefault(attr)}
                    <span class="muted">{m.wpi_create_element_no_default()}</span>
                  {:else}
                    {JSON.stringify(defaultValue(attr))}
                    {#if defaultValue(attr) === null}<span class="muted"> · {m.wpi_create_element_must_set()}</span>{/if}
                  {/if}
                </td>
                <td style="padding:6px 12px; font-family:monospace; word-break:break-all;">
                  {#if !isClassAttribute(attr.name)}
                    {#if helpTip(attr) === undefined}
                      <span class="muted">{m.wpi_create_element_double_click_to_add()}</span>
                    {:else}
                      {JSON.stringify(helpTip(attr))}
                    {/if}
                  {/if}
                </td>
                <td style="padding:0 6px; text-align:center; white-space:nowrap;">
                  {#if !(editMode && attr.name === 'descriptor_id')}
                    <button type="button" onclick={() => editAttribute(attr)}
                      aria-label={m.wpi_create_element_edit_attribute({ name: attr.name })}
                      title={m.wpi_create_element_edit_attribute({ name: attr.name })}
                      style="color:var(--cv-text-muted); background:none; border:none; cursor:pointer;">✎</button>
                  {/if}
                  {#if !isClassAttribute(attr.name) || isOptionalText(attr.name)}
                    <button type="button" onclick={() => (isClassAttribute(attr.name) ? clearClassText(attr.name) : removeAttribute(attr.name))}
                      aria-label={m.wpi_create_element_remove_attribute({ name: attr.name })}
                      title={m.wpi_create_element_remove_attribute({ name: attr.name })}
                      style="color:var(--cv-text-muted); background:none; border:none; cursor:pointer;">✕</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <small class="hint">{m.wpi_create_element_edit_hint()}</small>
      {#if formError}
        <div class="error" style="margin-top:12px;" use:reveal>
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
      {/if}
    </div>

    <!-- Footer -->
    <div style="padding:12px 20px; border-top:1px solid var(--cv-divider); display:flex; justify-content:flex-end; gap:8px;">
      <button type="button" class="btn" onclick={requestClose}>{m.wpi_create_element_cancel()}</button>
      <button type="button" class="btn primary" onclick={() => guard(save)} disabled={saving || (editMode && !loaded)}>
        {#if editMode}
          {saving ? m.wpi_edit_element_saving() : m.wpi_edit_element_save()}
        {:else}
          {saving ? m.wpi_create_element_creating() : m.wpi_create_element_create()}
        {/if}
      </button>
    </div>

    <!-- Unapplied changes to the attribute being edited -->
    {#if pendingGuard}
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div class="confirm-backdrop" role="alertdialog" aria-modal="true" aria-labelledby="unsaved-title" tabindex="-1" onkeydown={onConfirmKeydown}>
        <div class="confirm">
          <div id="unsaved-title" style="font-size:13px; font-weight:600; color:var(--cv-text);">{m.wpi_create_element_unsaved_title({ name: attrEditing ?? '' })}</div>
          <div style="font-size:11px; color:var(--cv-text-muted); margin-top:4px;">{m.wpi_create_element_unsaved_body()}</div>
          <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:14px;">
            <button type="button" class="btn" onclick={() => (pendingGuard = null)}>{m.wpi_create_element_unsaved_keep()}</button>
            <button type="button" class="btn" onclick={discardPending}>{m.wpi_create_element_unsaved_discard()}</button>
            <button type="button" class="btn primary" onclick={applyPending} use:focusButton>{m.wpi_create_element_unsaved_apply()}</button>
          </div>
        </div>
      </div>
    {/if}

    <!-- Resize grip -->
    <div class="grip select-none" title={m.wpi_create_element_resize()} aria-hidden="true" onpointerdown={startResize}></div>
  </div>
</div>

<style>
  .field { display: block; margin-bottom: 12px; }
  .field span { display: block; font-size: 10px; color: var(--cv-text-muted); margin-bottom: 3px; }
  .field input, .field textarea {
    width: 100%; box-sizing: border-box;
    background: var(--cv-surface); border: 1px solid var(--cv-border); border-radius: 4px;
    padding: 5px 8px; font-size: 12px; color: var(--cv-text);
  }
  .field textarea { display: block; min-height: 60px; resize: vertical; font-family: monospace; line-height: 1.4; }
  .field input:disabled, .field textarea:disabled { opacity: 0.4; }
  .hint { display: block; font-size: 10px; color: var(--cv-text-faint); margin-top: 3px; }
  .section {
    font-size: 9px; color: var(--cv-accent); letter-spacing: 1px; text-transform: uppercase;
    margin: 16px 0 8px;
  }
  .attr-row:hover, .attr-row.editing { background: var(--cv-hover); }
  .attr-row.editing { box-shadow: inset 2px 0 0 var(--cv-accent); }
  .confirm-backdrop {
    position: absolute; inset: 0; z-index: 5; display: flex; align-items: center; justify-content: center;
    background: var(--cv-backdrop);
  }
  .confirm {
    width: min(420px, calc(100% - 32px)); padding: 16px 18px; border-radius: 8px;
    background: var(--cv-panel); border: 1px solid var(--cv-border); box-shadow: 0 8px 24px rgba(0,0,0,.35);
  }
  .muted { color: var(--cv-text-muted); font-family: system-ui, sans-serif; }
  .notes { white-space: pre-wrap; font-family: system-ui, sans-serif; }
  .grip {
    position: absolute; right: 2px; bottom: 2px; width: 14px; height: 14px; cursor: nwse-resize;
    background: linear-gradient(135deg, transparent 0 50%, var(--cv-text-faint) 50% 58%, transparent 58% 70%, var(--cv-text-faint) 70% 78%, transparent 78%);
  }
  .tag {
    padding: 0 5px; border-radius: 3px;
    background: var(--cv-surface); color: var(--cv-accent-2); font-size: 9px; font-family: system-ui, sans-serif;
  }
  .btn {
    background: var(--cv-surface); border: 1px solid var(--cv-border); border-radius: 6px;
    padding: 5px 12px; font-size: 11px; color: var(--cv-text); cursor: pointer; white-space: nowrap;
  }
  .btn:hover:not(:disabled) { background: var(--cv-surface-hover); }
  .btn.primary { background: var(--cv-accent); border-color: var(--cv-accent); color: #fff; }
  .btn.primary:hover:not(:disabled) { background: #4f46e5; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .error {
    padding: 8px 12px; background: var(--cv-error-bg); border: 1px solid #ef4444; border-radius: 6px;
    font-size: 11px; color: #ef4444;
  }
</style>
