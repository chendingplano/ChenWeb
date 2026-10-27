<!-- web/src/lib/components/shared-ui/canvas/CreateElementDialog.svelte -->
<!-- The WPI editor's Create Element form: defines a new element class (its ID, description and
     instance attributes) and stores it as a draft, to be implemented in code afterwards
     (OpenSpec wpi-create-element-class). -->
<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { createElementClass, WpiError } from '$lib/wpi/api';
  import type { Issue } from '$lib/wpi/types';
  import {
    attributeProblem,
    buildClassRow,
    CLASS_ID_PATTERN,
    draftAttribute,
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

  let classId       = $state('');
  let descriptionEn = $state('');
  let descriptionZh = $state('');
  let attributes    = $state<DraftAttribute[]>([]);

  // The attribute being entered.
  let attrName     = $state('');
  let attrDefault  = $state('');
  let attrRequired = $state(false);
  let attrError    = $state('');

  let formError = $state('');
  let issues    = $state<Issue[]>([]);
  let saving    = $state(false);

  function problemText(p: AttributeProblem, name: string): string {
    switch (p) {
      case 'name':       return m.wpi_create_element_err_attribute_name();
      case 'reserved':   return m.wpi_create_element_err_reserved({ name });
      case 'duplicate':  return m.wpi_create_element_err_duplicate({ name });
      case 'no_default': return m.wpi_create_element_err_no_default();
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
    attrName = '';
    attrDefault = '';
    attrRequired = false;
    attrError = '';
  }

  function removeAttribute(name: string) {
    attributes = attributes.filter((a) => a.name !== name);
  }

  async function create() {
    formError = '';
    issues = [];
    const id = classId.trim();
    if (!CLASS_ID_PATTERN.test(id)) {
      formError = m.wpi_create_element_err_class_id();
      return;
    }
    if (descriptionEn.trim() === '') {
      formError = m.wpi_create_element_err_description();
      return;
    }
    saving = true;
    try {
      await createElementClass(buildClassRow(id, descriptionEn, descriptionZh, attributes));
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

  function onAttrKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
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
  <div class="rounded-xl overflow-hidden shadow-2xl"
    style="background:#111827; border:1px solid #1e2a3a; width:620px; max-width:95vw; max-height:85vh; display:flex; flex-direction:column;">

    <!-- Header -->
    <div style="padding:16px 20px; border-bottom:1px solid #1e2a3a; display:flex; justify-content:space-between; align-items:flex-start;">
      <div>
        <div id="create-element-title" style="font-size:15px; font-weight:600; color:#e2e8f0;">{m.wpi_create_element_title()}</div>
        <div style="font-size:11px; color:#6b7280; margin-top:2px;">{m.wpi_create_element_subtitle()}</div>
      </div>
      <button onclick={onClose} style="color:#6b7280; font-size:16px; background:none; border:none; cursor:pointer;" aria-label={m.wpi_create_element_close()}>✕</button>
    </div>

    <div style="flex:1; overflow-y:auto; padding:16px 20px;">
      <!-- Class -->
      <label class="field">
        <span>{m.wpi_create_element_class_id()}</span>
        <input bind:value={classId} maxlength="128" autocomplete="off" />
        <small>{m.wpi_create_element_class_id_hint()}</small>
      </label>
      <label class="field">
        <span>{m.wpi_create_element_description_en()}</span>
        <input bind:value={descriptionEn} />
      </label>
      <label class="field">
        <span>{m.wpi_create_element_description_zh()}</span>
        <input bind:value={descriptionZh} />
      </label>

      <!-- Attributes -->
      <div class="section">{m.wpi_create_element_attributes()}</div>
      <div style="display:flex; gap:8px; align-items:flex-end; flex-wrap:wrap;">
        <label class="field" style="flex:1 1 140px; margin:0;">
          <span>{m.wpi_create_element_attribute_name()}</span>
          <input bind:value={attrName} onkeydown={onAttrKeydown} autocomplete="off" />
        </label>
        <label class="field" style="flex:1 1 160px; margin:0;">
          <span>{m.wpi_create_element_default_value()}</span>
          <input bind:value={attrDefault} onkeydown={onAttrKeydown} disabled={attrRequired} autocomplete="off" />
        </label>
        <label style="display:flex; align-items:center; gap:6px; font-size:11px; color:#94a3b8; padding-bottom:6px;">
          <input type="checkbox" bind:checked={attrRequired} style="accent-color:#6366f1;" />
          {m.wpi_create_element_required()}
        </label>
        <button type="button" class="btn" onclick={addAttribute}>{m.wpi_create_element_add_attribute()}</button>
      </div>
      <small class="hint">{m.wpi_create_element_default_hint()}</small>
      {#if attrError}
        <div class="error" style="margin-top:6px;">{attrError}</div>
      {/if}

      <div style="margin-top:10px; border:1px solid #1e2a3a; border-radius:6px; overflow:hidden;">
        {#if attributes.length === 0}
          <div style="padding:10px 12px; font-size:11px; color:#4b5563; font-style:italic;">{m.wpi_create_element_no_attributes()}</div>
        {:else}
          <table style="width:100%; border-collapse:collapse; font-size:11px;">
            <thead>
              <tr style="background:#161b27; color:#6b7280; text-align:left;">
                <th style="padding:6px 12px; font-weight:500;">{m.wpi_create_element_attribute_name()}</th>
                <th style="padding:6px 12px; font-weight:500;">{m.wpi_create_element_default_value()}</th>
                <th style="width:32px;"></th>
              </tr>
            </thead>
            <tbody>
              {#each attributes as attr (attr.name)}
                <tr style="border-top:1px solid #1e2a3a; color:#e2e8f0;">
                  <td style="padding:6px 12px; font-family:monospace;">{attr.name}</td>
                  <td style="padding:6px 12px; font-family:monospace; word-break:break-all;">
                    {JSON.stringify(attr.value)}
                    {#if attr.value === null}<span style="color:#6b7280; font-family:inherit;"> · {m.wpi_create_element_must_set()}</span>{/if}
                  </td>
                  <td style="padding:0 6px; text-align:center;">
                    <button type="button" onclick={() => removeAttribute(attr.name)}
                      aria-label={m.wpi_create_element_remove_attribute({ name: attr.name })}
                      title={m.wpi_create_element_remove_attribute({ name: attr.name })}
                      style="color:#6b7280; background:none; border:none; cursor:pointer;">✕</button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>

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
  </div>
</div>

<style>
  .field { display: block; margin-bottom: 12px; }
  .field span { display: block; font-size: 10px; color: #6b7280; margin-bottom: 3px; }
  .field input {
    width: 100%; box-sizing: border-box;
    background: #1e2535; border: 1px solid #374151; border-radius: 4px;
    padding: 5px 8px; font-size: 12px; color: #e2e8f0;
  }
  .field input:disabled { opacity: 0.4; }
  .field small, .hint { display: block; font-size: 10px; color: #4b5563; margin-top: 3px; }
  .section {
    font-size: 9px; color: #6366f1; letter-spacing: 1px; text-transform: uppercase;
    margin: 16px 0 8px;
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
