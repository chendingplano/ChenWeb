<script lang="ts">
 import { onMount, tick } from 'svelte';
 import { m } from '$lib/paraglide/messages.js';
 let { darkMode = true }: { darkMode: boolean } = $props();
 type Log = { id:string; content:string; state:string; truncated:boolean };
 type WindowState = { id:string; name:string; visible:boolean; paused:boolean; text:string; state:string; truncated:boolean; search:string; match:number; from:string; to:string; width:number; height:number; x:number; y:number; z:number };
 const names = [['chenweb','ChenWeb'],['doc-processor','Doc Processor'],['kratos','Kratos'],['nats','NATS'],['doc-service','Doc Service'],['pdf-parser','pdf-parser'],['caddy','Caddy'],['parser-result-converter','parser-result-converter'],['deepseek-watcher','deepseek-watcher']];
 let windows = $state<WindowState[]>(names.map(([id,name])=>({id,name,visible:true,paused:false,text:'',state:'',truncated:false,search:'',match:0,from:'',to:'',width:520,height:320,x:0,y:0,z:0})));
 let error=$state(false), popup=$state<string|null>(null), popupLeft=$state(0), popupTop=$state(0);
 let drag:{x:number;y:number;left:number;top:number}|null=null, loading=false;
 let bodies:Record<string,HTMLElement>={};
 let windowsMenu: HTMLDetailsElement;
 let topZ=0;
 let interaction: { id:string; type:'move'|'resize'; pointer:number; startX:number; startY:number; x:number; y:number; width:number; height:number } | null = null;
 function dismissMenu(event:PointerEvent) { if(windowsMenu?.open && event.target instanceof Node && !windowsMenu.contains(event.target)) windowsMenu.open=false; }
 function beginWindowInteraction(event:PointerEvent,w:WindowState,type:'move'|'resize') {
  if(event.button!==0)return;
  event.preventDefault(); w.z=++topZ;
  interaction={id:w.id,type,pointer:event.pointerId,startX:event.clientX,startY:event.clientY,x:w.x,y:w.y,width:w.width,height:w.height};
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
 }
 function updateWindowInteraction(event:PointerEvent) {
  if(!interaction || interaction.pointer!==event.pointerId)return;
  const w=windows.find(item=>item.id===interaction?.id); if(!w)return;
  const dx=event.clientX-interaction.startX,dy=event.clientY-interaction.startY;
  if(interaction.type==='move'){w.x=interaction.x+dx;w.y=interaction.y+dy}
  else {w.width=Math.max(250,Math.min(2000,interaction.width+dx));w.height=Math.max(180,Math.min(1400,interaction.height+dy))}
 }
 function endWindowInteraction(event:PointerEvent){if(interaction?.pointer===event.pointerId)interaction=null}
 function keyboardWindow(event:KeyboardEvent,w:WindowState,type:'move'|'resize') {
  const delta:Record<string,[number,number]>={ArrowLeft:[-10,0],ArrowRight:[10,0],ArrowUp:[0,-10],ArrowDown:[0,10]};
  const d=delta[event.key];if(!d)return;event.preventDefault();w.z=++topZ;
  if(type==='move'){w.x+=d[0];w.y+=d[1]}else{w.width=Math.max(250,Math.min(2000,w.width+d[0]));w.height=Math.max(180,Math.min(1400,w.height+d[1]))}
 }

 async function refresh() {
  if(loading)return; loading=true;
  try { const res=await fetch('/api/v1/system-admin/system-monitor',{credentials:'same-origin'}); if(!res.ok)throw new Error(); const data:{sources:Log[]}=await res.json();
   const follow=new Set(windows.filter(w=>{const el=bodies[w.id];return el && el.scrollHeight-el.scrollTop-el.clientHeight<32}).map(w=>w.id));
   for(const log of data.sources){const w=windows.find(item=>item.id===log.id);if(w&&!w.paused){w.text=log.content;w.state=log.state;w.truncated=log.truncated}}
   error=false;await tick();for(const id of follow){const el=bodies[id];if(el)el.scrollTop=el.scrollHeight}
  }catch{error=true}finally{loading=false}
 }
 onMount(()=>{refresh();const timer=setInterval(refresh,1000);return()=>clearInterval(timer)});
 function filtered(w:WindowState){if(!w.from&&!w.to)return w.text;const from=w.from?new Date(w.from).getTime():-Infinity,to=w.to?new Date(w.to).getTime():Infinity;let include=false;return w.text.split('\n').filter(line=>{const match=line.match(/\d{4}-\d\d-\d\d[T ]\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)?/);if(match){const t=Date.parse(match[0]);if(!Number.isNaN(t))include=t>=from&&t<=to}return include}).join('\n')}
 function pieces(w:WindowState){const text=filtered(w);if(!w.search)return [{text,hit:false,index:-1}];const low=text.toLowerCase(),needle=w.search.toLowerCase();let pos=0,i=0;const result:{text:string;hit:boolean;index:number}[]=[];for(;;){const found=low.indexOf(needle,pos);if(found<0)break;result.push({text:text.slice(pos,found),hit:false,index:-1},{text:text.slice(found,found+needle.length),hit:true,index:i++});pos=found+needle.length}result.push({text:text.slice(pos),hit:false,index:-1});return result}
 async function navigate(w:WindowState,direction:number){const count=pieces(w).filter(p=>p.hit).length;if(!count)return;w.match=(w.match+direction+count)%count;await tick();bodies[w.id]?.querySelector<HTMLElement>(`[data-match="${w.match}"]`)?.scrollIntoView({block:'nearest'})}
 function openFilter(e:MouseEvent,id:string){const r=(e.currentTarget as HTMLElement).getBoundingClientRect();popupLeft=Math.max(0,Math.min(r.left,innerWidth-330));popupTop=Math.max(0,Math.min(r.bottom+4,innerHeight-260));popup=id}
 function startDrag(e:PointerEvent){if((e.target as HTMLElement).closest('button'))return;drag={x:e.clientX,y:e.clientY,left:popupLeft,top:popupTop};(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)}
 function moveDrag(e:PointerEvent){if(drag){popupLeft=Math.max(0,Math.min(innerWidth-100,drag.left+e.clientX-drag.x));popupTop=Math.max(0,Math.min(innerHeight-50,drag.top+e.clientY-drag.y))}}
</script>
<svelte:window onpointerdown={dismissMenu} />
<div class:light={!darkMode} class="monitor select-text">
 <header><div><h1>{m.sm_title()}</h1><p>{m.sm_description()}</p></div><details bind:this={windowsMenu}><summary>{m.sm_windows()}</summary><div class="choices">{#each windows as w}<label><input type="checkbox" bind:checked={w.visible}/> {w.name}</label>{/each}</div></details></header>
 {#if error}<p role="alert">{m.sm_load_error()}</p>{/if}
 <div class="grid">{#each windows as w (w.id)}{#if w.visible}
  <section class="window" style:width="{w.width}px" style:height="{w.height}px" style:transform="translate({w.x}px, {w.y}px)" style:z-index={w.z}>
   <div class="bar"><button type="button" class="move-handle" aria-label={m.sm_move_window({name:w.name})} title={m.sm_move_window({name:w.name})} onpointerdown={(e)=>beginWindowInteraction(e,w,'move')} onpointermove={updateWindowInteraction} onpointerup={endWindowInteraction} onpointercancel={endWindowInteraction} onkeydown={(e)=>keyboardWindow(e,w,'move')}>⠿</button><strong><span class:paused={w.paused} class="dot"></span>{w.name}</strong><div class="actions"><button type="button" onclick={()=>w.paused=!w.paused}>{w.paused?m.sm_resume():m.sm_pause()}</button><button type="button" onclick={(e)=>openFilter(e,w.id)}>{m.sm_filter()}</button><button type="button" aria-label={m.sm_close()} title={m.sm_close()} onclick={()=>w.visible=false}>×</button></div></div>
   <div class="controls"><input type="search" placeholder={m.sm_search()} aria-label={m.sm_search()} bind:value={w.search} oninput={()=>w.match=0}/><button type="button" aria-label={m.sm_prev()} title={m.sm_prev()} onclick={()=>navigate(w,-1)}>↑</button><button type="button" aria-label={m.sm_next()} title={m.sm_next()} onclick={()=>navigate(w,1)}>↓</button><label>{m.sm_width()} <input type="number" min="250" max="2000" step="20" bind:value={w.width}/></label><label>{m.sm_height()} <input type="number" min="180" max="1400" step="20" bind:value={w.height}/></label></div>
   {#if w.truncated}<div class="notice">{m.sm_truncated()}</div>{/if}
   <div class="log" bind:this={bodies[w.id]}>{#if w.state==='missing'}{m.sm_missing()}{:else if w.state==='error'}{m.sm_error()}{:else if !filtered(w)}{m.sm_empty()}{:else}<pre>{#each pieces(w) as part}{#if part.hit}<mark class:current={part.index===w.match} data-match={part.index}>{part.text}</mark>{:else}{part.text}{/if}{/each}</pre>{/if}</div>
   <button type="button" class="resize-handle" aria-label={m.sm_resize_window({name:w.name})} title={m.sm_resize_window({name:w.name})} onpointerdown={(e)=>beginWindowInteraction(e,w,'resize')} onpointermove={updateWindowInteraction} onpointerup={endWindowInteraction} onpointercancel={endWindowInteraction} onkeydown={(e)=>keyboardWindow(e,w,'resize')}>◢</button>
  </section>{/if}{/each}</div>
 {#if popup}{@const w=windows.find(item=>item.id===popup)}{#if w}<div class="filter-dialog" role="dialog" aria-label={m.sm_filter()} style:left="{popupLeft}px" style:top="{popupTop}px">
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="dialog-head" onpointerdown={startDrag} onpointermove={moveDrag} onpointerup={()=>drag=null} onpointercancel={()=>drag=null}><strong>{m.sm_filter()} · {w.name}</strong><button type="button" aria-label={m.sm_close()} onclick={()=>popup=null}>×</button></div>
  <label>{m.sm_from()}<input type="datetime-local" bind:value={w.from}/></label><label>{m.sm_to()}<input type="datetime-local" bind:value={w.to}/></label><p>{m.sm_filter_info()}</p><button type="button" onclick={()=>{w.from='';w.to=''}}>{m.sm_reset()}</button>
 </div>{/if}{/if}
</div>
<style>
 .monitor{--bg:#111923;--panel:#1b2632;--line:#364654;--text:#e2e9ed;--muted:#9cb0bc;--accent:#72d5b2;background:var(--bg);color:var(--text);min-height:100%;padding:24px;font:14px system-ui}.monitor.light{--bg:#eef2f3;--panel:#fff;--line:#cbd5da;--text:#192c34;--muted:#506b76;--accent:#087e61}header,.bar,.controls,.dialog-head{display:flex;align-items:center;justify-content:space-between;gap:8px}h1{font-size:24px;font-weight:700}header p{color:var(--muted);margin:4px 0 18px}button,summary{cursor:pointer}button,summary,input{border:1px solid var(--line);background:var(--panel);color:var(--text);border-radius:5px;padding:5px 8px}details{position:relative}.choices{position:absolute;z-index:5;right:0;background:var(--panel);border:1px solid var(--line);padding:12px;min-width:210px;display:grid;gap:8px}.grid{display:flex;flex-wrap:wrap;align-items:flex-start;gap:14px}.window{position:relative;display:flex;flex-direction:column;min-width:250px;min-height:180px;max-width:100%;border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--panel)}.bar{padding:10px 12px;border-bottom:1px solid var(--line)}.bar strong{flex:1;min-width:0}.move-handle{cursor:grab;touch-action:none;user-select:none;padding:3px 6px}.move-handle:active{cursor:grabbing}.resize-handle{position:absolute;right:0;bottom:0;cursor:nwse-resize;touch-action:none;user-select:none;border:0;padding:2px 5px;background:var(--panel);color:var(--muted)}.dot{display:inline-block;width:8px;height:8px;border-radius:50%;background:var(--accent);margin-right:8px}.dot.paused{background:#e9b75c}.actions{display:flex;gap:4px}.controls{justify-content:flex-start;flex-wrap:wrap;padding:7px;border-bottom:1px solid var(--line)}.controls input[type=search]{min-width:70px;flex:1}.controls input[type=number]{width:65px}.controls label{color:var(--muted);font-size:11px}.notice{color:var(--muted);padding:4px 10px;font-size:11px}.log{overflow:auto;flex:1;padding:10px;font:12px/1.5 ui-monospace,monospace;white-space:pre-wrap;overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere;margin:0}mark{background:#806a21;color:white}mark.current{background:#f9c84c;color:#111}.filter-dialog{position:fixed;z-index:100;width:320px;min-width:260px;min-height:210px;max-width:95vw;max-height:95vh;overflow:auto;resize:both;background:var(--panel);border:1px solid var(--line);box-shadow:0 14px 40px #0006;border-radius:8px;padding:12px}.dialog-head{cursor:move;border-bottom:1px solid var(--line);padding-bottom:8px}.filter-dialog label{display:grid;gap:4px;margin-top:10px}.filter-dialog p{color:var(--muted);font-size:11px;margin:10px 0}
</style>
