<script lang="ts">
	import { m as msg } from '$lib/paraglide/messages.js';
 import { onMount } from 'svelte';
 import { COUNTRIES } from './country-list';
 import { CALENDAR_TYPES } from './calendar-types';
 import {
  listHolidayInfo, createHolidayInfo, updateHolidayInfo, deleteHolidayInfo,
  getCalendar, createCalendar, upsertCalendarDates, deleteCalendarDate, deleteCalendar,
  getDefaultCountry, setDefaultCountry, clearDefaultCountry,
  type HolidayInfo, type Calendar, type DayKind
 } from './calendar-admin-client';

 let { darkMode = true }: { darkMode?: boolean } = $props();
 const colors = $derived({ bg: darkMode ? '#171B26' : '#F2F4F7', card: darkMode ? '#1F2333' : '#fff', text: darkMode ? '#E2E8F0' : '#111827', muted: darkMode ? '#94A3B8' : '#6B7280', border: darkMode ? '#2D3348' : '#E4E6EB', accent: darkMode ? '#A5B4FC' : '#4F46E5' });

 const monthNames = [msg.calendar_admin_january(),msg.calendar_admin_february(),msg.calendar_admin_march(),msg.calendar_admin_april(),msg.calendar_admin_may(),msg.calendar_admin_june(),msg.calendar_admin_july(),msg.calendar_admin_august(),msg.calendar_admin_september(),msg.calendar_admin_october(),msg.calendar_admin_november(),msg.calendar_admin_december()];
 const weekdayNames = [msg.calendar_admin_su(),msg.calendar_admin_mo(),msg.calendar_admin_tu(),msg.calendar_admin_we(),msg.calendar_admin_th(),msg.calendar_admin_fr(),msg.calendar_admin_sa()];

 let year = $state(new Date().getFullYear());
 let country = $state(COUNTRIES[0].code);
 let calendarType = $state(CALENDAR_TYPES[0].code);
 let defaultCountry = $state<string | null>(null);
 let isDefaultCountry = $derived(defaultCountry === country);
 let defaultCountryBusy = $state(false);

 let calendar = $state<Calendar | null>(null);
 // Country + calendar type identify a holiday info: the list of specific holidays (元旦, 春节, …),
 // shared across years. It exists once it has at least one holiday.
 let infoKeyValid = $derived(!!country && calendarType.trim() !== '');
 let infosLoaded = $state(false);
 // Year + country + calendar type identify the holidays of one year: the holiday info bound to dates.
 let keyValid = $derived(Number.isInteger(year) && year > 0 && infoKeyValid);
 // Days can only be selected once that year's holidays exist in the database.
 let calendarExists = $derived(!!calendar?.id);
 let creating = $state(false);
 let loading = $state(false);
 let error = $state('');

 // Pending selections, one set per day kind; selectMode picks which set a click edits.
 let selectMode = $state<DayKind>('holiday');
 let selectedHolidays = $state(new Set<string>());
 let selectedAdjusted = $state(new Set<string>());
 let selectedCount = $derived(selectedHolidays.size + selectedAdjusted.size);
 let boundByDate = $derived(new Map((calendar?.dates ?? []).map((d) => [d.holiday_date, d])));

 // Staged edits to saved bindings, keyed by date: the new day kind, or null to remove the binding.
 // Modify saves them; Clear Selection or reloading the calendar discards them.
 let pendingEdits = $state(new Map<string, DayKind | null>());
 let modifying = $state(false);
 // New selections can only join an existing holiday, so they need at least one saved day.
 let canModify = $derived(!modifying && (pendingEdits.size > 0 || (selectedCount > 0 && (calendar?.dates ?? []).length > 0)));

 let holidayInfos = $state<HolidayInfo[]>([]);
 let holidayInfoExists = $derived(holidayInfos.length > 0);
 let attachModal = $state(false);
 let attachHolidayId = $state<number | ''>('');
 let attachNew = $state(false);
 let newHoliday = $state({ name: '', description: '', note: '' });
 let attaching = $state(false);
 let attachError = $state('');

 let infoModal = $state(false);
 let infoEditing = $state<HolidayInfo | null>(null);
 let infoDraft = $state({ country: COUNTRIES[0].code, calendar_type: CALENDAR_TYPES[0].code, name: '', display_seqno: 1, description: '', note: '' });
 let infoSaving = $state(false);
 let infoError = $state('');

 function pad(n: number) { return String(n).padStart(2, '0'); }
 function dateKey(y: number, m: number, d: number) { return `${y}-${pad(m + 1)}-${pad(d)}`; }

 function monthCells(y: number, m: number) {
  const firstWeekday = new Date(y, m, 1).getDay();
  const daysInMonth = new Date(y, m + 1, 0).getDate();
  const cells: (number | null)[] = Array(firstWeekday).fill(null);
  for (let d = 1; d <= daysInMonth; d++) cells.push(d);
  return cells;
 }

 async function loadCalendar() {
  if (!keyValid) { calendar = null; clearSelection(); return; }
  loading = true; error = '';
  try {
   calendar = await getCalendar(year, country, calendarType.trim());
   clearSelection();
  } catch (e) {
   error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_load_calendar();
  } finally {
   loading = false;
  }
 }

 async function createCurrentCalendar() {
  if (!keyValid) return;
  creating = true; error = '';
  try {
   calendar = await createCalendar(year, country, calendarType.trim());
   clearSelection();
  } catch (e) {
   error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_create_calendar();
  } finally {
   creating = false;
  }
 }

 async function loadHolidayInfos() {
  if (!infoKeyValid) { holidayInfos = []; return; }
  infosLoaded = false;
  try { holidayInfos = await listHolidayInfo(country, calendarType); } catch (e) { error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_load_holiday_info(); }
  finally { infosLoaded = true; }
 }

 async function loadDefaultCountry() {
  try {
   defaultCountry = await getDefaultCountry();
   if (defaultCountry) country = defaultCountry;
  } catch (e) {
   error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_load_default_country();
  }
 }

 async function toggleDefaultCountry() {
  defaultCountryBusy = true;
  try {
   if (isDefaultCountry) { await clearDefaultCountry(); defaultCountry = null; }
   else { await setDefaultCountry(country); defaultCountry = country; }
  } catch (e) {
   error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_update_default_country();
  } finally {
   defaultCountryBusy = false;
  }
 }

 onMount(async () => { await loadDefaultCountry(); loadCalendar(); loadHolidayInfos(); });

 function clearSelection() { selectedHolidays = new Set(); selectedAdjusted = new Set(); pendingEdits = new Map(); }

 function effectiveKind(key: string): DayKind | null | undefined {
  return pendingEdits.has(key) ? pendingEdits.get(key) : boundByDate.get(key)?.day_kind;
 }

 // Clicking a saved day in the active mode toggles it between that kind and removed.
 // An edit that restores the saved kind is dropped, so Modify only sees real changes.
 function toggleBound(key: string, savedKind: DayKind) {
  const next = effectiveKind(key) === selectMode ? null : selectMode;
  const edits = new Map(pendingEdits);
  if (next === savedKind) edits.delete(key); else edits.set(key, next);
  pendingEdits = edits;
 }

 // The holiday of the saved day closest to date; a new day joins that holiday.
 function nearestHolidayId(date: string): number {
  const t = Date.parse(date);
  let best = calendar!.dates[0];
  for (const d of calendar!.dates) if (Math.abs(Date.parse(d.holiday_date) - t) < Math.abs(Date.parse(best.holiday_date) - t)) best = d;
  return best.holiday_info_id;
 }

 // Saves every change to the active calendar: staged edits to saved days, and new selections,
 // each bound to the holiday of its nearest saved day.
 async function saveModifications() {
  if (!calendar?.id || !canModify) return;
  modifying = true; error = '';
  // Upserts go through the upsert endpoint, which takes one holiday per call.
  const byHoliday = new Map<number, { holidays: string[]; adjusted: string[] }>();
  const removals: string[] = [];
  const changes: [string, DayKind | null][] = [
   ...pendingEdits,
   ...[...selectedHolidays].map((d): [string, DayKind] => [d, 'holiday']),
   ...[...selectedAdjusted].map((d): [string, DayKind] => [d, 'adjusted'])
  ];
  for (const [date, kind] of changes) {
   if (kind === null) { removals.push(date); continue; }
   const holidayInfoId = boundByDate.get(date)?.holiday_info_id ?? nearestHolidayId(date);
   const group = byHoliday.get(holidayInfoId) ?? { holidays: [], adjusted: [] };
   (kind === 'holiday' ? group.holidays : group.adjusted).push(date);
   byHoliday.set(holidayInfoId, group);
  }
  try {
   for (const [holidayInfoId, g] of byHoliday) await upsertCalendarDates(year, country, calendarType, g.holidays, holidayInfoId, g.adjusted);
   for (const date of removals) await deleteCalendarDate(calendar.id, date);
  } catch (e) {
   error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_save_changes();
  } finally {
   modifying = false;
   await loadCalendar();
  }
 }

 // Toggle key in the active mode's set; adding it there removes it from the other set.
 function toggleDay(key: string) {
  const mine = new Set(selectMode === 'holiday' ? selectedHolidays : selectedAdjusted);
  const other = new Set(selectMode === 'holiday' ? selectedAdjusted : selectedHolidays);
  if (mine.has(key)) mine.delete(key); else { mine.add(key); other.delete(key); }
  if (selectMode === 'holiday') { selectedHolidays = mine; selectedAdjusted = other; }
  else { selectedAdjusted = mine; selectedHolidays = other; }
 }

 function openAttach() {
  attachError = ''; attachHolidayId = ''; attachNew = false;
  newHoliday = { name: '', description: '', note: '' };
  attachModal = true;
 }

 async function confirmAttach() {
  attachError = '';
  let holidayInfoId = typeof attachHolidayId === 'number' ? attachHolidayId : NaN;
  attaching = true;
  try {
   if (attachNew) {
    if (!newHoliday.name.trim()) { attachError = msg.calendar_admin_holiday_name_is_required(); attaching = false; return; }
    const created = await createHolidayInfo({ country, calendar_type: calendarType, name: newHoliday.name.trim(), description: newHoliday.description.trim(), note: newHoliday.note.trim() });
    holidayInfoId = created.id;
    holidayInfos = [...holidayInfos, created];
   }
   if (!holidayInfoId || Number.isNaN(holidayInfoId)) { attachError = msg.calendar_admin_select_or_create_a_holiday(); attaching = false; return; }
   calendar = await upsertCalendarDates(year, country, calendarType, Array.from(selectedHolidays), holidayInfoId, Array.from(selectedAdjusted));
   clearSelection();
   attachModal = false;
  } catch (e) {
   attachError = e instanceof Error ? e.message : msg.calendar_admin_unable_to_attach_holiday();
  } finally {
   attaching = false;
  }
 }

 async function removeCalendar() {
  if (!calendar?.id || !confirm(msg.calendar_admin_delete_the_entire_calendar_for({ calendarType, country, year }))) return;
  try { await deleteCalendar(calendar.id); await loadCalendar(); } catch (e) { error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_delete_calendar(); }
 }

 function openNewInfo() { infoEditing = null; infoError = ''; infoDraft = { country, calendar_type: calendarType, name: '', display_seqno: 1, description: '', note: '' }; infoModal = true; }
 function openEditInfo(h: HolidayInfo) { infoEditing = h; infoError = ''; infoDraft = { country: h.country, calendar_type: h.calendar_type, name: h.name, display_seqno: h.display_seqno, description: h.description, note: h.note }; infoModal = true; }

 async function saveInfo() {
   if (!infoDraft.country.trim() || !infoDraft.name.trim()) { infoError = msg.calendar_admin_country_and_name_are_required(); return; }
   if (infoEditing && (!Number.isInteger(infoDraft.display_seqno) || infoDraft.display_seqno < 1)) { infoError = msg.calendar_admin_display_sequence_must_be_a(); return; }
  infoSaving = true; infoError = '';
  try {
   if (infoEditing) await updateHolidayInfo(infoEditing.id, infoDraft);
   else await createHolidayInfo({ country: infoDraft.country, calendar_type: infoDraft.calendar_type, name: infoDraft.name, description: infoDraft.description, note: infoDraft.note });
   infoModal = false;
   await loadHolidayInfos();
  } catch (e) {
   infoError = e instanceof Error ? e.message : msg.calendar_admin_unable_to_save_holiday_info();
  } finally {
   infoSaving = false;
  }
 }

 async function removeInfo(h: HolidayInfo) {
  if (!confirm(msg.calendar_admin_delete_holiday({ name: h.name }))) return;
  try { await deleteHolidayInfo(h.id); await loadHolidayInfos(); } catch (e) { error = e instanceof Error ? e.message : msg.calendar_admin_unable_to_delete_holiday_info(); }
 }
</script>

<div class="page" style={`background:${colors.bg};color:${colors.text}`}>
 <section class="card intro" style={`background:${colors.card};border-color:${colors.border}`}>
  <div><h1>{msg.calendar_admin_holiday_calendar()}</h1><p>{msg.calendar_admin_country_and_calendar_type_identify()}</p></div>
  <div class="controls">
   <label>{msg.calendar_admin_year()}<input type="number" bind:value={year} onchange={loadCalendar} /></label>
   <label>{msg.calendar_admin_country()}<select bind:value={country} onchange={() => { loadCalendar(); loadHolidayInfos(); }}>{#each COUNTRIES as c}<option value={c.code}>{c.name} ({c.code})</option>{/each}</select></label>
   <label>{msg.calendar_admin_calendar_type()}<select bind:value={calendarType} onchange={() => { loadCalendar(); loadHolidayInfos(); }}>{#each CALENDAR_TYPES as t}<option value={t.code}>{t.name}</option>{/each}</select></label>
   <button disabled={!keyValid} onclick={loadCalendar}>{msg.calendar_admin_refresh()}</button>
  </div>
 </section>

 {#if error}<div class="card error" style={`background:${colors.card};border-color:${colors.border}`}>{error}</div>{/if}

 <section class="card" style={`background:${colors.card};border-color:${colors.border}`}>
  <div class="toolbar">
   <span>{msg.calendar_admin_holiday_day_s_adjusted_day({ selectedHolidaysCount: selectedHolidays.size, selectedAdjustedCount: selectedAdjusted.size })}{#if pendingEdits.size}{msg.calendar_admin_saved_day_s_changed({ pendingEditsCount: pendingEdits.size })}{/if}</span>
   <button class="mode" class:active={selectMode === 'holiday'} aria-pressed={selectMode === 'holiday'} disabled={!calendarExists} onclick={() => (selectMode = 'holiday')}>{msg.calendar_admin_set_holidays()}</button>
   <button class="mode adjusted" class:active={selectMode === 'adjusted'} aria-pressed={selectMode === 'adjusted'} disabled={!calendarExists} onclick={() => (selectMode = 'adjusted')}>{msg.calendar_admin_set_adjusted_days()}</button>
   <button disabled={!calendarExists || selectedCount === 0} onclick={openAttach}>{msg.calendar_admin_attach_holiday()}</button>
   <button disabled={!canModify} onclick={saveModifications}>{modifying ? msg.calendar_admin_saving() : msg.calendar_admin_modify()}</button>
   <button class="secondary" disabled={selectedCount === 0 && pendingEdits.size === 0} onclick={clearSelection}>{msg.calendar_admin_clear_selection()}</button>
   {#if calendar?.id}<button class="danger" onclick={removeCalendar}>{msg.calendar_admin_delete_calendar()}</button>{/if}
  </div>
  <div class="legend">
   <span><i class="swatch selected"></i>{msg.calendar_admin_selected_holiday()}</span>
   <span><i class="swatch selected-adjusted"></i>{msg.calendar_admin_selected_adjusted_day()}</span>
   <span><i class="swatch bound"></i>{msg.calendar_admin_holiday()}</span>
   <span><i class="swatch bound-adjusted"></i>{msg.calendar_admin_adjusted_working_day()}</span>
   <span><i class="swatch changed"></i>{msg.calendar_admin_unsaved_change()}</span>
  </div>
  {#if !keyValid}<div class="state">{msg.calendar_admin_enter_a_year_country_and()}</div>
  {:else if loading}<div class="state">{msg.calendar_admin_loading_calendar()}</div>
  {:else}
   {#if !calendarExists}
    <div class="state create">
     {#if holidayInfoExists}
      <p>{msg.calendar_admin_no()} <strong>{calendarType.trim()}</strong> {msg.calendar_admin_holidays_exist_for({ country, year })}</p>
      <button disabled={creating} onclick={createCurrentCalendar}>{creating ? msg.calendar_admin_creating() : msg.calendar_admin_create()}</button>
     {:else}
      <p>{msg.calendar_admin_create_the_holiday_info_for({ country, calendarType: calendarType.trim(), year })}</p>
     {/if}
    </div>
   {/if}
   <div class="months">
    {#each monthNames as monthName, m}
     <div class="month" style={`border-color:${colors.border}`}>
      <h3>{monthName}</h3>
      <div class="weekdays">{#each weekdayNames as w}<span>{w}</span>{/each}</div>
      <div class="days">
       {#each monthCells(year, m) as day}
        {#if day === null}<span class="cell empty"></span>
        {:else}
         {@const key = dateKey(year, m, day)}
         {@const bound = boundByDate.get(key)}
         {@const kind = bound ? effectiveKind(key) : undefined}
         <button
          class="cell"
          disabled={!calendarExists}
          class:selected={selectedHolidays.has(key)}
          class:selected-adjusted={selectedAdjusted.has(key)}
          class:bound={kind === 'holiday'}
          class:bound-adjusted={kind === 'adjusted'}
          class:changed={pendingEdits.has(key)}
          title={bound ? `${bound.holiday_info_name}${kind === 'adjusted' ? msg.calendar_admin_adjusted_working_day_2() : kind === null ? ' (will be removed)' : ''}` : ''}
          onclick={() => (bound ? toggleBound(key, bound.day_kind) : toggleDay(key))}
         >{day}</button>
        {/if}
       {/each}
      </div>
     </div>
    {/each}
   </div>
  {/if}
 </section>

 <section class="card" style={`background:${colors.card};border-color:${colors.border}`}>
  <div class="toolbar">
   <h2>{msg.calendar_admin_holiday_info({ country, calendarType })}</h2>
   <label class="check inline"><input type="checkbox" checked={isDefaultCountry} disabled={defaultCountryBusy} onchange={toggleDefaultCountry} /> {msg.calendar_admin_set_as_default_country()}</label>
   {#if holidayInfoExists}<button onclick={openNewInfo}>{msg.calendar_admin_new_holiday()}</button>{/if}
  </div>
  {#if !infoKeyValid}<div class="state">{msg.calendar_admin_choose_a_country_and_calendar()}</div>
  {:else if holidayInfoExists}
   <div class="table-wrap"><table><thead><tr><th>{msg.calendar_admin_order()}</th><th>{msg.calendar_admin_name()}</th><th>{msg.calendar_admin_description()}</th><th>{msg.calendar_admin_note()}</th><th></th></tr></thead><tbody>
    {#each holidayInfos as h}<tr><td>{h.display_seqno}</td><td>{h.name}</td><td>{h.description}</td><td>{h.note}</td><td><button class="link" onclick={() => openEditInfo(h)}>{msg.calendar_admin_edit()}</button><button class="link danger" onclick={() => removeInfo(h)}>{msg.calendar_admin_delete()}</button></td></tr>{/each}
   </tbody></table></div>
  {:else if infosLoaded}
   <div class="state create">
    <p>{msg.calendar_admin_no_holiday_info_exists_for({ country, calendarType: calendarType.trim() })}</p>
    <button onclick={openNewInfo}>{msg.calendar_admin_create()}</button>
   </div>
  {/if}
 </section>
</div>

{#if attachModal}
<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && (attachModal = false)}>
 <div class="modal" role="dialog" aria-modal="true" aria-label={msg.calendar_admin_attach_holiday_2()} style={`background:${colors.card};color:${colors.text};border-color:${colors.border}`}>
  <div class="modal-head"><h2>{msg.calendar_admin_attach_holiday_to_holiday_day({ selectedHolidaysCount: selectedHolidays.size, selectedAdjustedCount: selectedAdjusted.size })}</h2><button class="link" onclick={() => (attachModal = false)}>{msg.calendar_admin_close()}</button></div>
  <div class="form">
   <label class="check"><input type="checkbox" bind:checked={attachNew} /> {msg.calendar_admin_create_a_new_holiday()}</label>
   {#if attachNew}
    <label>{msg.calendar_admin_name()}<input bind:value={newHoliday.name} /></label>
    <label>{msg.calendar_admin_description()}<input bind:value={newHoliday.description} /></label>
    <label>{msg.calendar_admin_note()}<input bind:value={newHoliday.note} /></label>
   {:else}
    <label>{msg.calendar_admin_existing_holiday()}<select bind:value={attachHolidayId}><option value="">{msg.calendar_admin_select()}</option>{#each holidayInfos as h}<option value={h.id}>{h.name}</option>{/each}</select></label>
   {/if}
  </div>
  {#if attachError}<div class="error">{attachError}</div>{/if}
  <div class="modal-actions"><button class="secondary" onclick={() => (attachModal = false)}>{msg.calendar_admin_cancel()}</button><button onclick={confirmAttach} disabled={attaching}>{attaching ? msg.calendar_admin_saving() : msg.calendar_admin_attach()}</button></div>
 </div>
</div>
{/if}

{#if infoModal}
<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && (infoModal = false)}>
 <div class="modal" role="dialog" aria-modal="true" aria-label={infoEditing ? msg.calendar_admin_edit_holiday() : msg.calendar_admin_new_holiday_2()} style={`background:${colors.card};color:${colors.text};border-color:${colors.border}`}>
  <div class="modal-head"><h2>{infoEditing ? msg.calendar_admin_edit_holiday_2() : msg.calendar_admin_new_holiday()}</h2><button class="link" onclick={() => (infoModal = false)}>{msg.calendar_admin_close()}</button></div>
  <div class="form">
   <label>{msg.calendar_admin_country()}<select bind:value={infoDraft.country}>{#each COUNTRIES as c}<option value={c.code}>{c.name} ({c.code})</option>{/each}</select></label>
   <label>{msg.calendar_admin_calendar_type()}<select bind:value={infoDraft.calendar_type}>{#each CALENDAR_TYPES as t}<option value={t.code}>{t.name}</option>{/each}</select></label>
   <label>{msg.calendar_admin_name()}<input bind:value={infoDraft.name} /></label>
   {#if infoEditing}<label>{msg.calendar_admin_display_order()}<input type="number" min="1" step="1" bind:value={infoDraft.display_seqno} /></label>{/if}
   <label>{msg.calendar_admin_description()}<input bind:value={infoDraft.description} /></label>
   <label>{msg.calendar_admin_note()}<input bind:value={infoDraft.note} /></label>
  </div>
  {#if infoError}<div class="error">{infoError}</div>{/if}
  <div class="modal-actions"><button class="secondary" onclick={() => (infoModal = false)}>{msg.calendar_admin_cancel()}</button><button onclick={saveInfo} disabled={infoSaving}>{infoSaving ? msg.calendar_admin_saving() : msg.calendar_admin_save()}</button></div>
 </div>
</div>
{/if}

<style>
 .page{min-height:100%;padding:24px;display:grid;gap:16px;font:13px system-ui}
 .card{border:1px solid;border-radius:12px;padding:20px;box-shadow:0 4px 18px #0001}
 .intro{display:flex;justify-content:space-between;gap:16px;flex-wrap:wrap}
 .intro h1{margin:0 0 8px;font-size:22px}
 .intro p{margin:0;color:#94a3b8}
 .controls{display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap}
 .controls label{display:grid;gap:4px;font-size:12px}
 input,select{border:1px solid #64748b;border-radius:7px;padding:8px;background:transparent;color:inherit;min-width:0}
 input[type=number]{width:90px}
 button{border:0;border-radius:7px;padding:8px 12px;background:#6366f1;color:white;cursor:pointer}
 button:disabled{opacity:.5;cursor:default}
 .secondary{background:transparent;border:1px solid #64748b;color:inherit}
 .danger{background:#b91c1c}
 .link{background:transparent;color:#a5b4fc;padding:4px}
 .link.danger{color:#f87171}
 .toolbar{display:flex;gap:10px;align-items:center;margin-bottom:14px;flex-wrap:wrap}
 .toolbar h2{margin:0;font-size:16px;flex:1}
 .error{color:#fca5a5}
 .state{text-align:center;padding:24px;color:#94a3b8}
 .cell:disabled{cursor:default}
 .create p{margin:0 0 12px}
 .months{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:16px}
 .month{border:1px solid;border-radius:10px;padding:10px}
 .month h3{margin:0 0 8px;font-size:13px;text-align:center}
 .weekdays,.days{display:grid;grid-template-columns:repeat(7,1fr);gap:2px;text-align:center}
 .weekdays span{color:#94a3b8;font-size:11px}
 .cell{aspect-ratio:1;border-radius:5px;border:1px solid transparent;background:transparent;color:inherit;font-size:11px;cursor:pointer;padding:0}
 .cell.empty{cursor:default}
 .cell.selected{background:#6366f1;color:#fff}
 .cell.selected-adjusted{background:#f59e0b;color:#1c1917}
 .cell.bound{background:#15803d;color:#fff}
 .cell.bound-adjusted{background:#7dd3fc;color:#0c4a6e}
 .cell.changed{border:2px dashed #ec4899}
 .mode{background:transparent;border:1px solid #6366f1;color:inherit}
 .mode.active{background:#6366f1;color:#fff}
 .mode.adjusted{border-color:#f59e0b}
 .mode.adjusted.active{background:#d97706;color:#fff}
 .legend{display:flex;gap:16px;flex-wrap:wrap;margin:-4px 0 12px;font-size:11px;color:#94a3b8}
 .legend span{display:flex;align-items:center;gap:6px}
 .swatch{width:12px;height:12px;border-radius:3px;display:inline-block;box-sizing:border-box}
 .swatch.selected{background:#6366f1}
 .swatch.selected-adjusted{background:#f59e0b}
 .swatch.bound{background:#15803d}
 .swatch.bound-adjusted{background:#7dd3fc}
 .swatch.changed{border:2px dashed #ec4899}
 .table-wrap{overflow:auto}
 table{width:100%;border-collapse:collapse}
 th,td{text-align:left;border-bottom:1px solid #64748b44;padding:9px 8px}
 .backdrop{position:fixed;inset:0;background:#0008;display:grid;place-items:center;padding:20px;z-index:20}
 .modal{max-width:480px;width:100%;border:1px solid;border-radius:12px;padding:22px}
 .modal-head{display:flex;justify-content:space-between;align-items:center}
 .modal-head h2{margin:0;font-size:16px}
 .form{display:grid;gap:12px;margin:16px 0}
 .form label{display:grid;gap:5px}
 .check{display:flex!important;align-items:center;grid-template-columns:auto 1fr;gap:8px}
 .check.inline{font-size:12px;color:#94a3b8;gap:6px}
 .modal-actions{display:flex;gap:8px;justify-content:flex-end}
</style>
