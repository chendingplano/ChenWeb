<script lang="ts">
 import { onMount } from 'svelte';
 import { COUNTRIES } from './country-list';
 import {
  listHolidayInfo, createHolidayInfo, updateHolidayInfo, deleteHolidayInfo,
  getCalendar, upsertCalendarDates, deleteCalendarDate, deleteCalendar,
  getDefaultCountry, setDefaultCountry, clearDefaultCountry,
  type HolidayInfo, type Calendar
 } from './calendar-admin-client';

 let { darkMode = true }: { darkMode?: boolean } = $props();
 const colors = $derived({ bg: darkMode ? '#171B26' : '#F2F4F7', card: darkMode ? '#1F2333' : '#fff', text: darkMode ? '#E2E8F0' : '#111827', muted: darkMode ? '#94A3B8' : '#6B7280', border: darkMode ? '#2D3348' : '#E4E6EB', accent: darkMode ? '#A5B4FC' : '#4F46E5' });

 const monthNames = ['January','February','March','April','May','June','July','August','September','October','November','December'];
 const weekdayNames = ['Su','Mo','Tu','We','Th','Fr','Sa'];

 let year = $state(new Date().getFullYear());
 let country = $state(COUNTRIES[0].code);
 let calendarType = $state('holidays');
 let defaultCountry = $state<string | null>(null);
 let isDefaultCountry = $derived(defaultCountry === country);
 let defaultCountryBusy = $state(false);

 let calendar = $state<Calendar | null>(null);
 let loading = $state(false);
 let error = $state('');

 let selected = $state(new Set<string>());
 let boundByDate = $derived(new Map((calendar?.dates ?? []).map((d) => [d.holiday_date, d])));

 let holidayInfos = $state<HolidayInfo[]>([]);
 let attachModal = $state(false);
 let attachHolidayId = $state<number | ''>('');
 let attachNew = $state(false);
 let newHoliday = $state({ name: '', description: '', note: '' });
 let attaching = $state(false);
 let attachError = $state('');

 let infoModal = $state(false);
 let infoEditing = $state<HolidayInfo | null>(null);
 let infoDraft = $state({ country: COUNTRIES[0].code, name: '', display_seqno: 1, description: '', note: '' });
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
  loading = true; error = '';
  try {
   calendar = await getCalendar(year, country, calendarType);
   selected = new Set();
  } catch (e) {
   error = e instanceof Error ? e.message : 'Unable to load calendar.';
  } finally {
   loading = false;
  }
 }

 async function loadHolidayInfos() {
  try { holidayInfos = await listHolidayInfo(country); } catch (e) { error = e instanceof Error ? e.message : 'Unable to load holiday info.'; }
 }

 async function loadDefaultCountry() {
  try {
   defaultCountry = await getDefaultCountry();
   if (defaultCountry) country = defaultCountry;
  } catch (e) {
   error = e instanceof Error ? e.message : 'Unable to load default country.';
  }
 }

 async function toggleDefaultCountry() {
  defaultCountryBusy = true;
  try {
   if (isDefaultCountry) { await clearDefaultCountry(); defaultCountry = null; }
   else { await setDefaultCountry(country); defaultCountry = country; }
  } catch (e) {
   error = e instanceof Error ? e.message : 'Unable to update default country.';
  } finally {
   defaultCountryBusy = false;
  }
 }

 onMount(async () => { await loadDefaultCountry(); loadCalendar(); loadHolidayInfos(); });

 function toggleDay(key: string) {
  const next = new Set(selected);
  if (next.has(key)) next.delete(key); else next.add(key);
  selected = next;
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
    if (!newHoliday.name.trim()) { attachError = 'Holiday name is required.'; attaching = false; return; }
    const created = await createHolidayInfo({ country, name: newHoliday.name.trim(), description: newHoliday.description.trim(), note: newHoliday.note.trim() });
    holidayInfoId = created.id;
    holidayInfos = [...holidayInfos, created];
   }
   if (!holidayInfoId || Number.isNaN(holidayInfoId)) { attachError = 'Select or create a holiday.'; attaching = false; return; }
   calendar = await upsertCalendarDates(year, country, calendarType, Array.from(selected), holidayInfoId);
   selected = new Set();
   attachModal = false;
  } catch (e) {
   attachError = e instanceof Error ? e.message : 'Unable to attach holiday.';
  } finally {
   attaching = false;
  }
 }

 async function removeBinding(date: string) {
  if (!calendar || !confirm(`Remove holiday on ${date}?`)) return;
  try { await deleteCalendarDate(calendar.id, date); await loadCalendar(); } catch (e) { error = e instanceof Error ? e.message : 'Unable to remove holiday.'; }
 }

 async function removeCalendar() {
  if (!calendar?.id || !confirm(`Delete the entire ${calendarType} calendar for ${country} ${year}?`)) return;
  try { await deleteCalendar(calendar.id); await loadCalendar(); } catch (e) { error = e instanceof Error ? e.message : 'Unable to delete calendar.'; }
 }

 function openNewInfo() { infoEditing = null; infoError = ''; infoDraft = { country, name: '', display_seqno: 1, description: '', note: '' }; infoModal = true; }
 function openEditInfo(h: HolidayInfo) { infoEditing = h; infoError = ''; infoDraft = { country: h.country, name: h.name, display_seqno: h.display_seqno, description: h.description, note: h.note }; infoModal = true; }

 async function saveInfo() {
   if (!infoDraft.country.trim() || !infoDraft.name.trim()) { infoError = 'Country and name are required.'; return; }
   if (infoEditing && (!Number.isInteger(infoDraft.display_seqno) || infoDraft.display_seqno < 1)) { infoError = 'Display sequence must be a positive integer.'; return; }
  infoSaving = true; infoError = '';
  try {
   if (infoEditing) await updateHolidayInfo(infoEditing.id, infoDraft);
   else await createHolidayInfo({ country: infoDraft.country, name: infoDraft.name, description: infoDraft.description, note: infoDraft.note });
   infoModal = false;
   await loadHolidayInfos();
  } catch (e) {
   infoError = e instanceof Error ? e.message : 'Unable to save holiday info.';
  } finally {
   infoSaving = false;
  }
 }

 async function removeInfo(h: HolidayInfo) {
  if (!confirm(`Delete holiday "${h.name}"?`)) return;
  try { await deleteHolidayInfo(h.id); await loadHolidayInfos(); } catch (e) { error = e instanceof Error ? e.message : 'Unable to delete holiday info.'; }
 }
</script>

<div class="page" style={`background:${colors.bg};color:${colors.text}`}>
 <section class="card intro" style={`background:${colors.card};border-color:${colors.border}`}>
  <div><h1>Holiday Calendar</h1><p>Define holiday calendars by year, country, and calendar type. Select day cells to attach a holiday.</p></div>
  <div class="controls">
   <label>Year<input type="number" bind:value={year} onchange={loadCalendar} /></label>
   <label>Country<select bind:value={country} onchange={() => { loadCalendar(); loadHolidayInfos(); }}>{#each COUNTRIES as c}<option value={c.code}>{c.name} ({c.code})</option>{/each}</select></label>
   <label>Calendar Type<input bind:value={calendarType} onchange={loadCalendar} /></label>
   <button onclick={loadCalendar}>Refresh</button>
  </div>
 </section>

 {#if error}<div class="card error" style={`background:${colors.card};border-color:${colors.border}`}>{error}</div>{/if}

 <section class="card" style={`background:${colors.card};border-color:${colors.border}`}>
  <div class="toolbar">
   <span>{selected.size} day(s) selected</span>
   <button disabled={selected.size === 0} onclick={openAttach}>Attach Holiday</button>
   <button class="secondary" disabled={selected.size === 0} onclick={() => (selected = new Set())}>Clear Selection</button>
   {#if calendar?.id}<button class="danger" onclick={removeCalendar}>Delete Calendar</button>{/if}
  </div>
  {#if loading}<div class="state">Loading calendar…</div>
  {:else}
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
         <button
          class="cell"
          class:selected={selected.has(key)}
          class:bound={!!bound}
          title={bound ? bound.holiday_info_name : ''}
          onclick={() => (bound ? removeBinding(key) : toggleDay(key))}
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
   <h2>Holiday Definitions ({country})</h2>
   <label class="check inline"><input type="checkbox" checked={isDefaultCountry} disabled={defaultCountryBusy} onchange={toggleDefaultCountry} /> Set as default country</label>
   <button onclick={openNewInfo}>New Holiday</button>
  </div>
  <div class="table-wrap"><table><thead><tr><th>Order</th><th>Name</th><th>Description</th><th>Note</th><th></th></tr></thead><tbody>
   {#each holidayInfos as h}<tr><td>{h.display_seqno}</td><td>{h.name}</td><td>{h.description}</td><td>{h.note}</td><td><button class="link" onclick={() => openEditInfo(h)}>Edit</button><button class="link danger" onclick={() => removeInfo(h)}>Delete</button></td></tr>
   {:else}<tr><td colspan="5" class="state">No holiday definitions for {country} yet.</td></tr>{/each}
  </tbody></table></div>
 </section>
</div>

{#if attachModal}
<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && (attachModal = false)}>
 <div class="modal" role="dialog" aria-modal="true" aria-label="Attach holiday" style={`background:${colors.card};color:${colors.text};border-color:${colors.border}`}>
  <div class="modal-head"><h2>Attach Holiday to {selected.size} Date(s)</h2><button class="link" onclick={() => (attachModal = false)}>Close</button></div>
  <div class="form">
   <label class="check"><input type="checkbox" bind:checked={attachNew} /> Create a new holiday</label>
   {#if attachNew}
    <label>Name<input bind:value={newHoliday.name} /></label>
    <label>Description<input bind:value={newHoliday.description} /></label>
    <label>Note<input bind:value={newHoliday.note} /></label>
   {:else}
    <label>Existing Holiday<select bind:value={attachHolidayId}><option value="">Select…</option>{#each holidayInfos as h}<option value={h.id}>{h.name}</option>{/each}</select></label>
   {/if}
  </div>
  {#if attachError}<div class="error">{attachError}</div>{/if}
  <div class="modal-actions"><button class="secondary" onclick={() => (attachModal = false)}>Cancel</button><button onclick={confirmAttach} disabled={attaching}>{attaching ? 'Saving…' : 'Attach'}</button></div>
 </div>
</div>
{/if}

{#if infoModal}
<div class="backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && (infoModal = false)}>
 <div class="modal" role="dialog" aria-modal="true" aria-label={infoEditing ? 'Edit holiday' : 'New holiday'} style={`background:${colors.card};color:${colors.text};border-color:${colors.border}`}>
  <div class="modal-head"><h2>{infoEditing ? 'Edit Holiday' : 'New Holiday'}</h2><button class="link" onclick={() => (infoModal = false)}>Close</button></div>
  <div class="form">
   <label>Country<select bind:value={infoDraft.country}>{#each COUNTRIES as c}<option value={c.code}>{c.name} ({c.code})</option>{/each}</select></label>
   <label>Name<input bind:value={infoDraft.name} /></label>
   {#if infoEditing}<label>Display order<input type="number" min="1" step="1" bind:value={infoDraft.display_seqno} /></label>{/if}
   <label>Description<input bind:value={infoDraft.description} /></label>
   <label>Note<input bind:value={infoDraft.note} /></label>
  </div>
  {#if infoError}<div class="error">{infoError}</div>{/if}
  <div class="modal-actions"><button class="secondary" onclick={() => (infoModal = false)}>Cancel</button><button onclick={saveInfo} disabled={infoSaving}>{infoSaving ? 'Saving…' : 'Save'}</button></div>
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
 .months{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:16px}
 .month{border:1px solid;border-radius:10px;padding:10px}
 .month h3{margin:0 0 8px;font-size:13px;text-align:center}
 .weekdays,.days{display:grid;grid-template-columns:repeat(7,1fr);gap:2px;text-align:center}
 .weekdays span{color:#94a3b8;font-size:11px}
 .cell{aspect-ratio:1;border-radius:5px;border:1px solid transparent;background:transparent;color:inherit;font-size:11px;cursor:pointer;padding:0}
 .cell.empty{cursor:default}
 .cell.selected{background:#6366f1;color:#fff}
 .cell.bound{background:#15803d;color:#fff}
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
