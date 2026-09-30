<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
 import { onMount } from 'svelte';
 import { COUNTRIES } from './country-list';
 import { listPeakHours, createPeakHours, updatePeakHours, deletePeakHours, validatePeakHoursDraft, type PeakHours, type PeakHoursInput, type ApplicableDaysMode } from './peak-hours-client';
 let { darkMode = true }: { darkMode?: boolean } = $props();
 let records = $state<PeakHours[]>([]), loading = $state(true), error = $state('');
 let query = $state(''), modal = $state(false), editing = $state(false), saving = $state(false), formError = $state('');
 const emptyDraft = (): PeakHoursInput => ({ name: '', hours: [], timezone: '', applicable_days: { mode: 'workdays' }, exclude_days: [], country: '' });
 let draft = $state<PeakHoursInput>(emptyDraft());
 let hourInput = $state(''), weekdaysInput = $state(''), monthDaysInput = $state(''), excludeSpecificInput = $state('');
 const fallbackTimezones = ['UTC', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles', 'America/Sao_Paulo', 'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Europe/Moscow', 'Africa/Cairo', 'Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Tokyo', 'Asia/Seoul', 'Asia/Singapore', 'Asia/Kolkata', 'Asia/Dubai', 'Australia/Sydney', 'Pacific/Auckland'];
 const timezones: string[] = (() => {
  try {
   const supported = (Intl as unknown as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.('timeZone');
   return supported && supported.length ? supported : fallbackTimezones;
  } catch {
   return fallbackTimezones;
  }
 })();
 const colors = $derived({ bg: darkMode ? '#171B26' : '#F2F4F7', card: darkMode ? '#1F2333' : '#fff', text: darkMode ? '#E2E8F0' : '#111827', muted: darkMode ? '#94A3B8' : '#6B7280', border: darkMode ? '#2D3348' : '#E4E6EB', accent: darkMode ? '#A5B4FC' : '#4F46E5' });
 let filtered = $derived(records.filter((r) => !query || [r.name, r.timezone, r.country].some((v) => v.toLowerCase().includes(query.toLowerCase()))));
 const weekdayOptions = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
 let excludeWeekends = $derived(draft.exclude_days.includes('weekends'));
 let excludeHolidays = $derived(draft.exclude_days.includes('holidays'));
 let excludeSpecific = $derived(draft.exclude_days.filter((e) => e !== 'weekends' && e !== 'holidays'));
 let holidaysNeedCountry = $derived(excludeHolidays && !draft.country.trim());
 let timezoneOptions = $derived(draft.timezone && !timezones.includes(draft.timezone) ? [draft.timezone, ...timezones] : timezones);

 async function load() { loading = true; error = ''; try { records = await listPeakHours(); } catch (e) { error = e instanceof Error ? e.message : m.peak_hours_unable_to_load_peak_hours(); } finally { loading = false; } }

 function openNew() { editing = false; formError = ''; draft = emptyDraft(); hourInput = ''; weekdaysInput = ''; monthDaysInput = ''; excludeSpecificInput = ''; modal = true; }
 function openEdit(r: PeakHours) {
  editing = true; formError = '';
  draft = { name: r.name, hours: [...r.hours], timezone: r.timezone, applicable_days: { mode: r.applicable_days.mode, days: r.applicable_days.days ? [...r.applicable_days.days] : undefined }, exclude_days: [...r.exclude_days], country: r.country };
  weekdaysInput = draft.applicable_days.mode === 'weekdays' ? (draft.applicable_days.days ?? []).join(', ') : '';
  monthDaysInput = draft.applicable_days.mode === 'days_of_month' ? (draft.applicable_days.days ?? []).join(', ') : '';
  excludeSpecificInput = excludeSpecific.join(', ');
  hourInput = '';
  modal = true;
 }

 function addHour() { const h = hourInput.trim(); if (h) draft.hours = [...draft.hours, h]; hourInput = ''; }
 function removeHour(i: number) { draft.hours = draft.hours.filter((_, idx) => idx !== i); }

 function setMode(mode: ApplicableDaysMode) { draft.applicable_days = { mode }; }
 function syncWeekdays() { draft.applicable_days = { mode: 'weekdays', days: weekdaysInput.split(',').map((s) => s.trim().toLowerCase()).filter(Boolean) }; }
 function syncMonthDays() { draft.applicable_days = { mode: 'days_of_month', days: monthDaysInput.split(',').map((s) => Number(s.trim())).filter((n) => !Number.isNaN(n)) }; }

 function toggleExcludeKeyword(keyword: 'weekends' | 'holidays', checked: boolean) {
  const rest = draft.exclude_days.filter((e) => e !== keyword);
  draft.exclude_days = checked ? [...rest, keyword] : rest;
 }
 function syncExcludeSpecific() {
  const keywords = draft.exclude_days.filter((e) => e === 'weekends' || e === 'holidays');
  const specific = excludeSpecificInput.split(',').map((s) => s.trim()).filter(Boolean);
  draft.exclude_days = [...keywords, ...specific];
 }

 async function save() {
  syncExcludeSpecific();
  formError = validatePeakHoursDraft(draft) ?? '';
  if (formError) return;
  saving = true;
  try {
   if (editing) await updatePeakHours(draft.name, { hours: draft.hours, timezone: draft.timezone, applicable_days: draft.applicable_days, exclude_days: draft.exclude_days, country: draft.country });
   else await createPeakHours(draft);
   modal = false;
   await load();
  } catch (e) { formError = e instanceof Error ? e.message : m.peak_hours_unable_to_save_peak_hours(); } finally { saving = false; }
 }

 async function remove(r: PeakHours) {
  if (!confirm(m.peak_hours_delete_peak_hours({ name: r.name }))) return;
  try { await deletePeakHours(r.name); await load(); } catch (e) { error = e instanceof Error ? e.message : m.peak_hours_unable_to_delete_peak_hours(); }
 }

 function describeDays(r: PeakHours): string {
  if (r.applicable_days.mode === 'workdays') return m.peak_hours_workdays();
  if (r.applicable_days.mode === 'weekdays') return (r.applicable_days.days ?? []).join(', ') || 'weekdays';
  return m.peak_hours_day_of_month({ days: (r.applicable_days.days ?? []).join(', ') });
 }

 onMount(load);
</script>

<div class="page" style={`background:${colors.bg};color:${colors.text};user-select:text`}>
 <section class="card intro" style={`background:${colors.card};border-color:${colors.border}`}>
  <div><h1>{m.peak_hours_peak_hours()}</h1><p>{m.peak_hours_named_timezone_aware_windows_of()}</p></div>
  <div class="actions"><button onclick={openNew}>{m.peak_hours_new_peak_hours()}</button><button class="secondary" onclick={load}>{m.peak_hours_refresh()}</button></div>
 </section>
 <section class="card" style={`background:${colors.card};border-color:${colors.border}`}>
  <div class="filters"><input aria-label={m.peak_hours_search_peak_hours()} bind:value={query} placeholder={m.peak_hours_search_name_timezone_country()} /></div>
  {#if loading}<div class="state">{m.peak_hours_loading_peak_hours()}</div>{:else if error}<div class="state error">{error}</div>{:else}<div class="table-wrap"><table><thead><tr><th>{m.peak_hours_name()}</th><th>{m.peak_hours_hours()}</th><th>{m.peak_hours_timezone()}</th><th>{m.peak_hours_applicable_days()}</th><th>{m.peak_hours_exclude()}</th><th>{m.peak_hours_country()}</th><th></th></tr></thead><tbody>{#each filtered as r}<tr><td>{r.name}</td><td class="literal">{r.hours.join(', ')}</td><td>{r.timezone}</td><td>{describeDays(r)}</td><td>{r.exclude_days.join(', ') || '—'}</td><td>{r.country || '—'}</td><td><button class="link" onclick={() => openEdit(r)}>{m.peak_hours_edit()}</button><button class="link danger" onclick={() => remove(r)}>{m.peak_hours_delete()}</button></td></tr>{:else}<tr><td colspan="7" class="state">{m.peak_hours_no_peak_hours_definitions_match()}</td></tr>{/each}</tbody></table></div>{/if}
 </section>
</div>

{#if modal}<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && (modal = false)}><div class="modal" role="dialog" aria-modal="true" aria-label={editing ? m.peak_hours_edit_peak_hours() : m.peak_hours_new_peak_hours_2()} style={`background:${colors.card};color:${colors.text};border-color:${colors.border};user-select:text`}>
 <div class="modal-head"><h2>{editing ? m.peak_hours_edit_peak_hours_2() : m.peak_hours_new_peak_hours()}</h2><button class="link" onclick={() => (modal = false)}>{m.peak_hours_close()}</button></div>
 <div class="form">
  <label>{m.peak_hours_name()}{#if editing}<input bind:value={draft.name} disabled />{:else}<input bind:value={draft.name} />{/if}</label>
  <label>{m.peak_hours_timezone_iana()}<select bind:value={draft.timezone}><option value="" disabled>{m.peak_hours_select_a_timezone()}</option>{#each timezoneOptions as tz}<option value={tz}>{tz}</option>{/each}</select></label>

  <div class="group">
   <span class="group-label">{m.peak_hours_hours()}</span>
   <div class="chip-row">{#each draft.hours as h, i}<span class="chip">{h}<button class="chip-x" onclick={() => removeHour(i)} aria-label={`Remove ${h}`}>×</button></span>{/each}</div>
   <div class="inline"><input bind:value={hourInput} placeholder="09:00-12:00" /><button class="secondary" onclick={addHour}>{m.peak_hours_add()}</button></div>
   <small>{m.peak_hours_24_hour_hh_mm_two()}</small>
  </div>

  <div class="group">
   <span class="group-label">{m.peak_hours_applicable_days()}</span>
   <div class="inline"><label class="radio"><input type="radio" name="mode" checked={draft.applicable_days.mode === 'workdays'} onchange={() => setMode('workdays')} /> {m.peak_hours_workdays()}</label><label class="radio"><input type="radio" name="mode" checked={draft.applicable_days.mode === 'weekdays'} onchange={() => { setMode('weekdays'); syncWeekdays(); }} /> {m.peak_hours_weekdays()}</label><label class="radio"><input type="radio" name="mode" checked={draft.applicable_days.mode === 'days_of_month'} onchange={() => { setMode('days_of_month'); syncMonthDays(); }} /> {m.peak_hours_days_of_month()}</label></div>
   {#if draft.applicable_days.mode === 'weekdays'}<input bind:value={weekdaysInput} oninput={syncWeekdays} placeholder={m.peak_hours_mon_tue_wed()} /><small>{m.peak_hours_any_of({ weekdayOptions: weekdayOptions.join(', ') })}</small>{/if}
   {#if draft.applicable_days.mode === 'days_of_month'}<input bind:value={monthDaysInput} oninput={syncMonthDays} placeholder="1, 15" /><small>{m.peak_hours_comma_separated_day_numbers_1()}</small>{/if}
  </div>

  <div class="group">
   <span class="group-label">{m.peak_hours_exclude_days_any_match_excludes()}</span>
   <div class="inline"><label class="check"><input type="checkbox" checked={excludeWeekends} onchange={(e) => toggleExcludeKeyword('weekends', (e.target as HTMLInputElement).checked)} /> {m.peak_hours_weekends()}</label><label class="check"><input type="checkbox" checked={excludeHolidays} onchange={(e) => toggleExcludeKeyword('holidays', (e.target as HTMLInputElement).checked)} /> {m.peak_hours_holidays()}</label></div>
   {#if holidaysNeedCountry}<small class="hint">{m.peak_hours_set_a_country_below_to()}</small>{/if}
   <input bind:value={excludeSpecificInput} oninput={syncExcludeSpecific} placeholder="2026-12-25, 2026-12-24..2026-12-31" />
   <small>{m.peak_hours_comma_separated_iso_dates_or()}</small>
  </div>

  <label>{m.peak_hours_country_for_holiday_exclusion()}<select bind:value={draft.country}><option value="">{m.peak_hours_none()}</option>{#each COUNTRIES as c}<option value={c.code}>{c.name}</option>{/each}</select></label>
 </div>
 {#if formError}<div class="error">{formError}</div>{/if}
 <div class="modal-actions"><button class="secondary" onclick={() => (modal = false)}>{m.peak_hours_cancel()}</button><button onclick={save} disabled={saving}>{saving ? m.peak_hours_saving() : m.peak_hours_save()}</button></div>
</div></div>{/if}

<style>
 .page{min-height:100%;padding:24px;display:grid;gap:16px;font:13px system-ui}.card{border:1px solid;border-radius:12px;padding:20px;box-shadow:0 4px 18px #0001}.intro{display:flex;justify-content:space-between;gap:16px}.intro h1{margin:0 0 8px;font-size:22px}.intro p{margin:0 0 10px;color:#94a3b8}.actions,.filters,.modal-actions,.inline{display:flex;gap:8px;align-items:center}.filters{margin-bottom:16px}.filters input{flex:1}input,select{border:1px solid #64748b;border-radius:7px;padding:8px;background:transparent;color:inherit;min-width:0}button{border:0;border-radius:7px;padding:8px 12px;background:#6366f1;color:white;cursor:pointer}.secondary{background:transparent;border:1px solid #64748b;color:inherit}.link{background:transparent;color:#a5b4fc;padding:4px}.link.danger{color:#f87171}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse;white-space:nowrap}th,td{text-align:left;border-bottom:1px solid #64748b44;padding:11px 8px}.literal{font-family:ui-monospace,monospace}.state{text-align:center;padding:42px;color:#94a3b8}.error{color:#fca5a5;margin:12px 0}.hint{color:#facc15}.backdrop{position:fixed;inset:0;background:#0008;display:grid;place-items:center;padding:20px;z-index:20}.modal{max-width:640px;width:100%;max-height:90vh;overflow:auto;border:1px solid;border-radius:12px;padding:22px}.modal-head{display:flex;justify-content:space-between;align-items:center}.modal-head h2{margin:0}.form{display:grid;gap:14px;margin:18px 0}.form label{display:grid;gap:5px}.group{display:grid;gap:6px;border:1px solid #64748b33;border-radius:8px;padding:10px}.group-label{font-weight:600;color:#94a3b8;font-size:12px;text-transform:uppercase}.chip-row{display:flex;flex-wrap:wrap;gap:6px}.chip{display:inline-flex;align-items:center;gap:4px;background:#64748b22;border-radius:6px;padding:3px 6px;font-family:ui-monospace,monospace}.chip-x{background:none;padding:0 2px;color:inherit;font-size:14px;line-height:1}.radio,.check{display:flex!important;align-items:center;gap:5px;grid-template-columns:auto 1fr;white-space:nowrap}
</style>
