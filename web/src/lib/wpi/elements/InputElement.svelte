<script lang="ts">
 import { Input } from '$lib/components/ui/input';
 import { m } from '$lib/paraglide/messages.js';
 import { getRuntime } from '../runtime.svelte';
 import { fieldErrorText, localized } from '../text';
 import { validateInput } from '../input-validation';
 import type { Json } from '../types';
 import type { HTMLInputAttributes } from 'svelte/elements';
 import type { ElementProps } from './index';
 let { instanceId, element }: ElementProps = $props();
 const rt = getRuntime();
 const id = $props.id();
 const d = $derived(element.desc);
 const editable = $derived(d.binding_mode === 'read-write');
 let unboundValue = $state<Json>('');
 const value = $derived(d.data_binding ? rt.read(instanceId, d.data_binding) : unboundValue);
 const serverErrors = $derived(rt.errorsFor(instanceId, d.data_binding));
 let touched = $state(false);
 let clientErrors = $state<string[]>([]);
 let labelBox = $state<HTMLElement>();
 let truncated = $state(false);
 let labelOpen = $state(false);
 let helpOpen = $state(false);
 let composing = false;
 const placement = $derived(d.label_placement ?? 'left');
 const horizontal = $derived(placement === 'left' || placement === 'right');
 const allErrors = $derived([...new Set([...clientErrors, ...serverErrors])]);
 function check(): boolean {
  touched = true;
  clientErrors = validateInput(value == null ? '' : String(value), d);
  return clientErrors.length === 0;
 }
 $effect(() => rt.registerInput(instanceId, id, (focus) => {
  const valid = check();
  if (!valid && focus) document.getElementById(id)?.focus();
  return valid;
 }));
 $effect(() => {
  const box = labelBox;
  localized(d.label); d.label_max_lines; d.label_max_width;
  if (!box) return;
  const measure = () => { truncated = box.scrollHeight > box.clientHeight || box.scrollWidth > box.clientWidth; };
  const observer = new ResizeObserver(measure); observer.observe(box); measure();
  return () => observer.disconnect();
 });
 function edit(raw: string) {
  let next: Json = raw;
  if (d.input_type === 'number') next = raw === '' || Number.isNaN(Number(raw)) ? (raw === '' ? null : raw) : Number(raw);
  if (d.data_binding) rt.write(instanceId, d.data_binding, next);
  else unboundValue = next;
  if (touched) clientErrors = validateInput(raw, d);
 }
 function onInput(event: Event) {
  const el = event.currentTarget as HTMLInputElement;
  if (composing) return;
  if (!composing && d.input_type !== 'number' && d.max_length !== undefined && Array.from(el.value).length > d.max_length && Array.from(value == null ? '' : String(value)).length <= d.max_length) el.value = Array.from(el.value).slice(0, d.max_length).join('');
  edit(el.value);
 }
</script>

<div class="container"><div class:horizontal class:reverse={placement === 'right' || placement === 'bottom'} class="field" style:--hgap={d.label_horizontal_gap ?? '10px'} style:--vgap={d.label_vertical_gap ?? '10px'}>
 <div class="caption" title={truncated ? localized(d.label) : undefined} style:max-width={d.label_max_width ?? '160px'} style:min-height={horizontal ? '36px' : undefined} style:align-items={horizontal ? ({top:'start',center:'center',bottom:'end'}[d.label_vertical_align ?? 'center']) : undefined}>
  <label for={id} class="label" style:text-align={d.label_horizontal_align ?? 'right'} style:font-family={d.label_font_family} style:font-size={d.label_font_size ?? '14px'} style:color={d.label_color} style:font-weight={d.label_font_weight ?? '500'} style:font-style={d.label_font_style ?? 'normal'}>
   <span bind:this={labelBox} class="label-text" style:--lines={d.label_max_lines ?? 1}>{localized(d.label)}</span>{#if d.required}<span aria-hidden="true"> *</span>{/if}
  </label>
  {#if truncated}
   <button type="button" aria-label={m.wpi_input_caption()} aria-expanded={labelOpen} onclick={() => labelOpen = true} onmouseenter={() => labelOpen = true} onmouseleave={(e) => { if (document.activeElement !== e.currentTarget) labelOpen = false; }} onfocus={() => labelOpen = true} onblur={() => labelOpen = false} onkeydown={(e) => { if (e.key === 'Escape') labelOpen = false; }}>ⓘ</button>
   {#if labelOpen}<span class="popup" role="tooltip">{localized(d.label)}</span>{/if}
  {/if}
 </div>
 <div class="control">
  <Input {id} type={d.input_type ?? 'text'} value={value == null ? '' : String(value)} readonly={!editable} required={editable && d.required} placeholder={localized(d.placeholder) || undefined} autocomplete={d.autocomplete as HTMLInputAttributes['autocomplete']} aria-invalid={allErrors.length > 0} aria-describedby={[d.help_text ? `${id}-help` : '', allErrors.length ? `${id}-errors` : ''].filter(Boolean).join(' ') || undefined} oninput={onInput} onblur={check} oncompositionstart={() => composing = true} oncompositionend={(e) => { composing = false; onInput(e); }}/>
  {#if d.help_text}<p id="{id}-help" class="help">{localized(d.help_text)}</p>{/if}
  {#if d.help_tip}<div class="tip"><button type="button" aria-label={m.wpi_input_help()} aria-expanded={helpOpen} onclick={() => helpOpen = !helpOpen} onkeydown={(e) => { if (e.key === 'Escape') helpOpen = false; }}>ⓘ</button>{#if helpOpen}<span>{localized(d.help_tip)}</span>{/if}</div>{/if}
  {#if allErrors.length}<div id="{id}-errors" class="errors">{#each allErrors as code (code)}<p>{fieldErrorText(code)}</p>{/each}</div>{/if}
 </div>
</div></div>

<style>
 .container { container-type:inline-size; min-width:0; }
 .field { display:flex; flex-direction:column; gap:var(--vgap); min-width:0; }
 .field.horizontal { flex-direction:row; gap:var(--hgap); align-items:start; }
 .field.reverse { flex-direction:column-reverse; }
 .field.horizontal.reverse { flex-direction:row-reverse; }
 .caption { position:relative; min-width:0; display:flex; gap:4px; align-items:start; }
 .label { min-width:0; width:100%; color:var(--muted-foreground); }
 .label-text { display:-webkit-box; -webkit-box-orient:vertical; -webkit-line-clamp:var(--lines); overflow:hidden; overflow-wrap:anywhere; }
 .control { flex:1; min-width:120px; }
 @container (max-width: 250px) { .field.horizontal, .field.horizontal.reverse { flex-direction:column; gap:var(--vgap); } .field.horizontal .caption { min-height:0 !important; } .control { width:100%; min-width:0; } }
 .help,.tip,.errors { margin-top:4px; font-size:0.75rem; }
 .errors { color:var(--destructive); }
 .popup { position:absolute; z-index:10; top:100%; left:0; padding:4px 8px; background:var(--background); color:var(--foreground); border:1px solid currentColor; width:max-content; max-width:240px; }
</style>
