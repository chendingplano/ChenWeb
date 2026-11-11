<script lang="ts">
 import { elementMinimum, belowElementMinimum, clampElementMinimum } from '$lib/wpi/element-minimum';
 // The page being edited, drawn on the canvas with the interpreter's layout and
 // element components (OpenSpec wpi-pages-own-panes, design D12): its pane tree,
 // and in each leaf pane its element nodes. Each element has a selection
 // overlay; each leaf pane is a drop target (data-editor-pane) and is selected
 // by its tag or a click on its empty area. Each element is the drop target of
 // validators (data-editor-element, OpenSpec wpi-primitive-validators).
 import { onMount, tick, untrack } from 'svelte';
 import CopyIcon from '@lucide/svelte/icons/copy';
 import Trash2Icon from '@lucide/svelte/icons/trash-2';
 import PencilIcon from '@lucide/svelte/icons/pencil';
 import EyeIcon from '@lucide/svelte/icons/eye';
 import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
 import { getNodeActions } from './node-actions';
 import type { Node } from '@xyflow/svelte';
 import { m } from '$lib/paraglide/messages.js';
 import { getElementClass } from '$lib/wpi/api';
 import { AppRuntime, setRuntime } from '$lib/wpi/runtime.svelte';
 import { resolveEntry, isFixed } from '$lib/wpi/entries';
 import { cssLength, effectiveRoot, flowItemStyle, flowMarginStyle, flowStyle, inheritPaneColors, inheritPaneBorder, paneBorderStyle, isLeaf, pageBoxStyle, panelStyle, paneStyle, rootPaneStyle, placeStyle, splitStyle, surfaceColorStyle } from '$lib/wpi/layout';
 import { flowGeometry } from '$lib/wpi/layouters/flow-geometry';
 import ElementView from '$lib/wpi/elements/Element.svelte';
 import ErrorBox from '$lib/wpi/ErrorBox.svelte';
 import type { Bundle, ClassRow, ElementInstance, JsonObject, PageDescriptor, PaneSpec, SurfaceColors, PaneBorder } from '$lib/wpi/types';
 import { instanceJson, isElementNode, paneNodes } from './wpi-json';
 import { MIN_PANE_SIZE } from './pane-edit';
 import { pageDragSize, type PageSide } from './page-resize';
 import { resizeElement, resizeHandles, resizeOutline as elementResizeOutline, resizeLimitChanges, type ResizeLimit, type ResizeLimitChange, type ElementHandle } from './element-resize';
 import { dropIndex } from './element-order';

 let { darkMode, pageName, dirty, openedAt, autoSavedAt, descriptor, nodes, selectedId, selectedPane, pageLayouterAvailable, elementLayouterAvailable, size, zoom, onResizeStart, onResize, onSelect, onSelectPane, onViewLayouter, onMeasure, onPaneResize, onPageResize, onPageCancel, onMeasurePanes, onSelectionGeometry, onElementResize, onMoveElement }: {
  darkMode: boolean; pageName: string; dirty: boolean; openedAt: number | null; autoSavedAt: number | null; descriptor: JsonObject; nodes: Node[]; selectedId: string | null; selectedPane: string | null;
  pageLayouterAvailable: boolean; elementLayouterAvailable: boolean;
  size: { width: number; height: number }; zoom: number;
  onElementResize: (id: string, size: { width?: number; height?: number }, changes: ResizeLimitChange[]) => void;
  onMoveElement: (id: string, index: number) => void;
  onResizeStart: () => void; onResize: (size: { width: number; height: number }) => void;
  onSelect: (id: string) => void; onSelectPane: (pane: string) => void;
  onViewLayouter: (view: 'page-layouter' | 'element-layouter') => void;
  onPaneResize: (a: string, b: string, axis: 'width' | 'height', first: number, second: number, parentSize: number) => void;
  onMeasurePanes: (panes: { id: string; width: number; height: number; parentWidth: number; parentHeight: number }[]) => void;
  onPageResize: (axis: 'width' | 'height', value: number) => void;
  onPageCancel: (original: JsonObject) => void;
  onMeasure: (id: string, size: { width: number; height: number }) => void;
  // The selected element's and pane's top-left corner, in page pixels from the page's.
  onSelectionGeometry?: (geometry: { element?: { x: number; y: number }; pane?: { x: number; y: number } }) => void;
 } = $props();
 const actions = getNodeActions();
 let previewElement: HTMLElement;
 let resizeNotice = $state<{ id: string; message: string; left: number; top: number; size: { width?: number; height?: number }; changes: ResizeLimitChange[] } | null>(null);
 let resizeCancelButton = $state<HTMLButtonElement>();
 $effect(() => {
  if (resizeNotice && resizeNotice.id !== selectedId) resizeNotice = null;
  else if (resizeNotice) tick().then(() => resizeCancelButton?.focus());
 });
 let toolbarPosition = $state<{ left: number; top: number } | null>(null);
 function positionToolbar() {
  const target = previewElement?.querySelector<HTMLElement>('.overlay.selected');
  if (!target) { toolbarPosition = null; return; }
  const root = previewElement.getBoundingClientRect();
  const rect = target.getBoundingClientRect();
  const scale = zoom || 1;
  if (rect.bottom <= root.top || rect.top >= root.bottom || rect.right <= root.left || rect.left >= root.right) { toolbarPosition = null; return; }
  toolbarPosition = { left: Math.max(55, Math.min(size.width - 55, (rect.left + rect.width / 2 - root.left) / scale)), top: (rect.top - root.top) / scale - 4 };
 }
 let lastGeometry = '';
 function reportGeometry() {
  const box = previewElement?.querySelector<HTMLElement>('[data-page-box]')?.getBoundingClientRect();
  const scale = zoom || 1;
  const at = (el: Element | null | undefined) => {
   if (!box || !el) return undefined;
   const r = el.getBoundingClientRect();
   return { x: Math.round((r.left - box.left) / scale), y: Math.round((r.top - box.top) / scale) };
  };
  const geometry = {
   element: at(previewElement?.querySelector('.overlay.selected')?.parentElement),
   pane: selectedPane ? at(previewElement?.querySelector(`[data-pane-id="${CSS.escape(selectedPane)}"]`)) : undefined
  };
  const key = JSON.stringify(geometry);
  if (key === lastGeometry) return;
  lastGeometry = key;
  onSelectionGeometry?.(geometry);
 }
 $effect(() => {
  selectedId; selectedPane; nodes; descriptor; size; zoom; ready;
  void tick().then(() => { positionToolbar(); reportGeometry(); });
 });
 function measurePanes() {
  requestAnimationFrame(() => {
   onMeasurePanes(Array.from(previewElement.querySelectorAll<HTMLElement>('[data-pane-id]')).map((pane) => ({ id: pane.dataset.paneId!, width: pane.getBoundingClientRect().width / (zoom || 1), height: pane.getBoundingClientRect().height / (zoom || 1), parentWidth: (pane.parentElement?.getBoundingClientRect().width ?? 0) / (zoom || 1), parentHeight: (pane.parentElement?.getBoundingClientRect().height ?? 0) / (zoom || 1) })));
  });
 }
 let elementSizes = $state<Record<string, { width: number; height: number }>>({});
 function measureElement(host: HTMLElement, id: string) {
  const observer = new ResizeObserver(() => {
   const width = host.offsetWidth, height = host.offsetHeight;
   if (elementSizes[id]?.width !== width || elementSizes[id]?.height !== height) elementSizes = { ...elementSizes, [id]: { width, height } };
  });
  observer.observe(host);
  return { destroy() { observer.disconnect(); } };
 }
 let elementOutline = $state<{ x: number; y: number; width: number; height: number } | null>(null);
 let elementDrag: { host: HTMLElement; box: { x: number; y: number; width: number; height: number }; pointer: number; id: string; handle: ElementHandle; x: number; y: number; zoom: number; size: { width: number; height: number }; value: { width?: number; height?: number } } | null = null;
 function startElementResize(event: PointerEvent, id: string, handle: ElementHandle) {
  if (event.button !== 0 || !elementSizes[id]) return;
  const scale = zoom || 1;
  const target = (event.currentTarget as HTMLElement).parentElement!.getBoundingClientRect();
  const preview = previewElement.getBoundingClientRect();
  const box = { x: (target.left - preview.left) / scale - previewElement.clientLeft, y: (target.top - preview.top) / scale - previewElement.clientTop, ...elementSizes[id] };
  elementDrag = { host: (event.currentTarget as HTMLElement).parentElement!, box, pointer: event.pointerId, id, handle, x: event.clientX, y: event.clientY, zoom: scale, size: elementSizes[id], value: {} };
  elementOutline = box;
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  event.preventDefault(); event.stopPropagation();
 }
 function moveElementResize(event: PointerEvent) {
  const d = elementDrag;
  if (!d || d.pointer !== event.pointerId) return;
  d.value = resizeElement(d.size, d.handle, event.clientX - d.x, event.clientY - d.y, d.zoom);
  elementOutline = elementResizeOutline(d.box, d.handle, d.value);
  event.stopPropagation();
 }
 function commitElementResize(id: string, size: { width?: number; height?: number }, host: HTMLElement) {
  const node = nodes.find((n) => n.id === id);
  if (!node) return;
  size = { ...size };
  if (size.width === elementSizes[id]?.width) delete size.width;
  if (size.height === elementSizes[id]?.height) delete size.height;
  if (size.width === undefined && size.height === undefined) return;
  const pane = String(node.data.pane ?? '');
  const item = entries(pane).find((entry) => entry.node.id === id);
  if (!item || 'error' in item.entry) return;
  const minimum = elementMinimum(item.entry.module, item.entry.desc.editor);

  const row = (node.data.class ?? classes[String(node.data.element ?? '')]) as ClassRow | undefined;
  if (row && ((size.width !== undefined && isFixed(row, 'width')) || (size.height !== undefined && isFixed(row, 'height')))) {
   alert(m.wpi_resize_fixed_limit()); return;
  }
  const config = panelConfig(pane);
  const desc = item.entry.desc as unknown as Record<string, unknown>;
  const limits: ResizeLimit[] = [];
  const probe = document.createElement('div');
  probe.style.cssText = 'position:absolute;visibility:hidden;pointer-events:none;box-sizing:border-box;min-width:0;min-height:0;max-width:none;max-height:none;padding:0;border:0';
  host.parentElement!.appendChild(probe);
  for (const [source, key, axis, kind, value] of [
   ['element', 'min_width', 'width', 'min', desc.min_width], ['element', 'max_width', 'width', 'max', desc.max_width],
   ['panel', 'min_elem_width', 'width', 'min', config.min_elem_width ?? '20px'], ['panel', 'max_elem_width', 'width', 'max', config.max_elem_width ?? '300px'],
   ['panel', 'min_row_height', 'height', 'min', config.min_row_height ?? '30px'], ['panel', 'max_row_height', 'height', 'max', config.max_row_height ?? '200px']
  ] as const) {
   if (value === undefined || value === null) continue;
   const length = typeof value === 'number' || /^\d+(?:\.\d+)?$/.test(String(value).trim()) ? `${value}px` : String(value);
   if (!CSS.supports(axis, length)) continue;
   probe.style[axis] = length;
   const pixels = probe.getBoundingClientRect()[axis] / (zoom || 1);
   limits.push({ source, key, axis, kind, pixels });
   probe.style[axis] = '0px';
  }
  probe.remove();
  const effectiveMinimum = {
   width: Math.max(minimum?.width ?? 0, ...limits.filter((limit) => limit.axis === 'width' && limit.kind === 'min').map((limit) => limit.pixels)),
   height: Math.max(minimum?.height ?? 0, ...limits.filter((limit) => limit.axis === 'height' && limit.kind === 'min').map((limit) => limit.pixels))
  };
  const needsConfirmation = belowElementMinimum(size, effectiveMinimum);
  if (needsConfirmation) {
   const rect = host.getBoundingClientRect(), root = previewElement.getBoundingClientRect(), scale = zoom || 1;
   const clamped = clampElementMinimum(size, effectiveMinimum);
   const changes = resizeLimitChanges(clamped, limits);
   if (changes.some((change) => change.source === 'element' && row && isFixed(row, change.key))) { alert(m.wpi_resize_fixed_limit()); return; }
   resizeNotice = { id, message: m.wpi_resize_content_minimum({ width: String(Math.ceil(effectiveMinimum.width)), height: String(Math.ceil(effectiveMinimum.height)) }), left: Math.max(0, Math.min((rect.left - root.left) / scale, Math.max(0, previewElement.clientWidth - 260))), top: Math.max(0, Math.min((rect.bottom - root.top) / scale + 8, previewElement.clientHeight - 110)), size: clamped, changes };
   return;
  }
  const changes = resizeLimitChanges(size, limits);
  if (changes.some((change) => change.source === 'element' && row && isFixed(row, change.key))) {
   alert(m.wpi_resize_fixed_limit()); return;
  }
  if (changes.length && !confirm(m.wpi_resize_override_limits({ limits: changes.map((change) => `${change.key}: ${change.value}`).join(', ') }))) return;
  resizeNotice = null;
  onElementResize(id, size, changes);
 }
 // Svelte delegates onclick to the document, after Svelte Flow's pane click handler.
 // Handle these clicks directly so they cannot clear the underlying selection first.
 function resizeDialogButton(node: HTMLButtonElement, callback: () => void) {
  const click = (event: MouseEvent) => { event.stopPropagation(); callback(); };
  const down = (event: Event) => event.stopPropagation();
  node.addEventListener('click', click); node.addEventListener('pointerdown', down); node.addEventListener('mousedown', down);
  return { destroy() { node.removeEventListener('click', click); node.removeEventListener('pointerdown', down); node.removeEventListener('mousedown', down); } };
 }
 function cancelResize() { const id = resizeNotice?.id; resizeNotice = null; if (id) onSelect(id); }
 function confirmResize() {
  const pending = resizeNotice;
  if (!pending || selectedId !== pending.id) return;
  if (pending.changes.length && !confirm(m.wpi_resize_override_limits({ limits: pending.changes.map((change) => `${change.key}: ${change.value}`).join(', ') }))) return;
  resizeNotice = null;
  onElementResize(pending.id, pending.size, pending.changes);
  onSelect(pending.id);
 }
 function elementResizeKey(event: KeyboardEvent, id: string, handle: ElementHandle) {
  const step = event.shiftKey ? 10 : 1;
  const dx = event.key === 'ArrowLeft' ? -step : event.key === 'ArrowRight' ? step : 0;
  const dy = event.key === 'ArrowUp' ? -step : event.key === 'ArrowDown' ? step : 0;
  if ((!dx && !dy) || !elementSizes[id]) return;
  const value = resizeElement(elementSizes[id], handle, dx, dy, 1);
  if ((value.width === undefined || value.width === elementSizes[id].width) && (value.height === undefined || value.height === elementSizes[id].height)) return;
  commitElementResize(id, value, (event.currentTarget as HTMLElement).parentElement!);
  event.preventDefault(); event.stopPropagation();
 }
 function endElementResize(event: PointerEvent, cancel = false) {
  const d = elementDrag;
  if (!d || d.pointer !== event.pointerId) return;
  elementDrag = null;
  elementOutline = null;
  if (!cancel) {
   if (event.clientX !== d.x || event.clientY !== d.y) commitElementResize(d.id, resizeElement(d.size, d.handle, event.clientX - d.x, event.clientY - d.y, d.zoom), d.host);
   onSelect(d.id);
  }
  event.stopPropagation();
 }
 // Dragging an element to another place in its pane. The drag starts after a few
 // pixels, so a click still selects; the drop lands among the pane's other elements.
 let reorder: { id: string; pane: string; pointer: number; x: number; y: number; from: number; dragging: boolean; index: number } | null = null;
 let reorderLine = $state<{ x: number; y: number; height: number } | null>(null);
 function startReorder(event: PointerEvent, id: string) {
  if (event.button !== 0) return;
  const pane = String(nodes.find((n) => n.id === id)?.data.pane ?? '');
  reorder = { id, pane, pointer: event.pointerId, x: event.clientX, y: event.clientY, from: paneNodes(elements, pane).findIndex((n) => n.id === id), dragging: false, index: 0 };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  event.preventDefault(); event.stopPropagation();
 }
 function moveReorder(event: PointerEvent) {
  const r = reorder;
  if (!r || r.pointer !== event.pointerId) return;
  if (!r.dragging) {
   if (Math.hypot(event.clientX - r.x, event.clientY - r.y) < 4) return;
   r.dragging = true;
  }
  const others = [...previewElement.querySelectorAll<HTMLElement>('[data-editor-element]')].filter((el) => el.dataset.editorElement !== r.id && el.closest('[data-editor-pane]')?.getAttribute('data-editor-pane') === r.pane);
  const boxes = others.map((el) => el.getBoundingClientRect());
  r.index = dropIndex(boxes, event.clientX, event.clientY);
  reorderLine = reorderMark(boxes, r.index);
  event.stopPropagation();
 }
 // The bar at the drop place, in the preview's coordinates like the element outline.
 function reorderMark(boxes: DOMRect[], index: number) {
  const box = boxes[index] ?? boxes[index - 1];
  if (!box) return null;
  const scale = zoom || 1;
  const preview = previewElement.getBoundingClientRect();
  const x = index < boxes.length ? box.left : box.right;
  return { x: (x - preview.left) / scale - previewElement.clientLeft, y: (box.top - preview.top) / scale - previewElement.clientTop, height: box.height / scale };
 }
 function endReorder(event: PointerEvent, cancel = false) {
  const r = reorder;
  if (!r || r.pointer !== event.pointerId) return;
  reorder = null; reorderLine = null;
  if (r.dragging && !cancel && r.index !== r.from) onMoveElement(r.id, r.index);
 }
 let drag: { id: number; x: number; y: number; width: number; height: number; zoom: number; pageWidth: number; pageHeight: number; original: JsonObject; movedX: boolean; movedY: boolean } | null = null;
 let resizeOutline = $state<{ width: number; height: number } | null>(null);
 function startResize(event: PointerEvent) {
  if (event.button !== 0) return;
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, ...size, zoom: zoom || 1, pageWidth: size.width, pageHeight: size.height, original: JSON.parse(JSON.stringify(descriptor)), movedX: false, movedY: false };
  resizeOutline = { ...size };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  onResizeStart();
  event.stopPropagation();
  event.preventDefault();
 }
 function resize(event: PointerEvent) {
  if (!drag || drag.id !== event.pointerId) return;
  const dx = event.clientX - drag.x, dy = event.clientY - drag.y;
  resizeOutline = { width: Math.max(240, Math.round(drag.width + dx / drag.zoom)), height: Math.max(160, Math.round(drag.height + dy / drag.zoom)) };
  drag.movedX ||= dx !== 0; drag.movedY ||= dy !== 0;
  event.stopPropagation();
 }
 function endResize(event: PointerEvent, cancel = false) {
  if (!drag || drag.id !== event.pointerId) return;
  if (!cancel && resizeOutline) {
   onResize(resizeOutline);
   if (drag.movedX) onPageResize('width', resizeOutline.width);
   if (drag.movedY) onPageResize('height', resizeOutline.height);
   measurePanes();
  }
  drag = null; resizeOutline = null; event.stopPropagation();
 }
 function resizeKey(event: KeyboardEvent) {
  let dx = 0, dy = 0; const step = event.shiftKey ? 50 : 10;
  if (event.key === 'ArrowRight') dx = step; else if (event.key === 'ArrowLeft') dx = -step;
  else if (event.key === 'ArrowDown') dy = step; else if (event.key === 'ArrowUp') dy = -step; else return;
  onResizeStart();
  onResize({ width: Math.max(240, size.width + dx), height: Math.max(160, size.height + dy) });
  if (dx) onPageResize('width', Math.max(240, size.width + dx));
  if (dy) onPageResize('height', Math.max(160, size.height + dy));
  measurePanes(); event.preventDefault(); event.stopPropagation();
 }
 let paneDrag: { pointer: number; start: number; first: number; second: number; a: string; b: string; axis: 'width' | 'height'; left: HTMLElement; right: HTMLElement; value: number; oldLeft: string; oldRight: string; min: number; max: number } | null = null;
 function startPaneResize(event: PointerEvent, a: string, b: string, axis: 'width' | 'height') {
  if (event.button !== 0) return;
  const left = (event.currentTarget as HTMLElement).parentElement as HTMLElement;
  const right = left.nextElementSibling as HTMLElement | null;
  if (!right) return;
  const first = left.getBoundingClientRect()[axis] / (zoom || 1);
  const second = right.getBoundingClientRect()[axis] / (zoom || 1);
  paneDrag = { pointer: event.pointerId, start: axis === 'width' ? event.clientX : event.clientY, first, second, a, b, axis, left, right, value: first, oldLeft: left.style.flex, oldRight: right.style.flex, min: MIN_PANE_SIZE, max: Math.max(MIN_PANE_SIZE, first + second - MIN_PANE_SIZE) };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  event.preventDefault(); event.stopPropagation();
 }
 function movePaneResize(event: PointerEvent) {
  const d = paneDrag;
  if (!d || d.pointer !== event.pointerId) return;
  const delta = ((d.axis === 'width' ? event.clientX : event.clientY) - d.start) / (zoom || 1);
  d.value = Math.round(Math.max(d.min, Math.min(d.max, d.first + delta)));
  d.left.style.flex = `0 0 ${d.value}px`;
  d.right.style.flex = `0 0 ${Math.round(d.first + d.second - d.value)}px`;
  event.stopPropagation();
 }
 function endPaneResize(event: PointerEvent, cancel = false) {
  const d = paneDrag;
  if (!d || d.pointer !== event.pointerId) return;
  d.left.style.flex = d.oldLeft; d.right.style.flex = d.oldRight;
  paneDrag = null;
  if (!cancel && Math.abs(d.value - d.first) >= 1) onPaneResize(d.a, d.b, d.axis, d.value, Math.round(d.first + d.second - d.value), d.left.parentElement!.getBoundingClientRect()[d.axis] / (zoom || 1));
  event.stopPropagation();
 }
 type Side = PageSide;
 let outerDrag: { pointer: number; side: Side; axis: 'width' | 'height'; start: number; initial: number; zoom: number; original: JsonObject } | null = null;
 function startOuter(event: PointerEvent, side: Side) {
  if (event.button !== 0) return;
  const axis = side === 'left' || side === 'right' ? 'width' : 'height';
  outerDrag = { pointer: event.pointerId, side, axis, start: axis === 'width' ? event.clientX : event.clientY, initial: size[axis], zoom: zoom || 1, original: JSON.parse(JSON.stringify(descriptor)) };
  resizeOutline = { ...size };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  onResizeStart(); event.stopPropagation(); event.preventDefault();
 }
 function moveOuter(event: PointerEvent) {
  const d = outerDrag;
  if (!d || d.pointer !== event.pointerId) return;
  const movement = (d.axis === 'width' ? event.clientX : event.clientY) - d.start;
  const value = Math.max(d.axis === 'width' ? 240 : 160, pageDragSize(d.initial, movement, d.zoom, d.side));
  resizeOutline = { ...size, [d.axis]: value };
  event.stopPropagation();
 }
 function endOuter(event: PointerEvent, cancel = false) {
  const d = outerDrag;
  if (!d || d.pointer !== event.pointerId) return;
  outerDrag = null;
  if (!cancel && resizeOutline && resizeOutline[d.axis] !== d.initial) { onResize(resizeOutline); onPageResize(d.axis, resizeOutline[d.axis]); measurePanes(); }
  resizeOutline = null;
  event.stopPropagation();
 }
 const page = $derived(descriptor as unknown as PageDescriptor);
 const setupPage = untrack(() => descriptor as unknown as PageDescriptor);
 // The runtime gives elements a page instance whose input doc they show; the
 // page drawn here is the one being edited, not this bundle's copy.
 const bundle: Bundle = { app: { descriptor_id: 'editor-preview', schema_version: '0.5', initial_page: setupPage.descriptor_id }, pages: { [setupPage.descriptor_id]: setupPage }, warnings: [] };
 const rt = new AppRuntime(bundle, true);
 setRuntime(rt);
 let classes = $state<Record<string, ClassRow>>({});
 let failures = $state<string[]>([]);
 let ready = $state(false);
 const elements = $derived(nodes.filter(isElementNode));
 const root = $derived(effectiveRoot(page));
 // The element nodes of a leaf pane with their instances resolved; a node
 // without a class copy uses the class row fetched for it.
 function entries(pane: string) {
  return paneNodes(elements, pane).map((node, i) => ({ node, entry: resolveEntry(page, pane, instanceJson(node) as ElementInstance, i, classes) }));
 }
 function panelConfig(pane: string) {
  const el = page.panels?.[pane]?.elem_layouter;
  return el?.config ?? el?.class?.config ?? {};
 }
 // Observe the rendered control, not its zoomed canvas coordinates.
 function measure(host: HTMLElement, id: string | null) {
  let target: HTMLElement | null = null;
  const observer = new ResizeObserver(() => {
   if (target && id) onMeasure(id, { width: target.offsetWidth, height: target.offsetHeight });
  });
  function refresh() {
   const next = id ? host.querySelector<HTMLElement>('[data-slot="button"]') : null;
   if (next === target) return;
   if (target) observer.unobserve(target);
   target = next;
   if (target) observer.observe(target);
  }
  const mutations = new MutationObserver(refresh);
  mutations.observe(host, { childList: true, subtree: true });
  refresh();
  return { update(next: string | null) { id = next; refresh(); }, destroy() { mutations.disconnect(); observer.disconnect(); } };
 }
 const requested = new Set<string>();
 function fetchClasses() {
  for (const node of elements) {
   const id = String(node.data.element ?? '');
   if (!id || node.data.class || classes[id] || requested.has(id)) continue;
   requested.add(id);
   void getElementClass(id).then((row) => {
    classes = { ...classes, [id]: row.class as unknown as ClassRow };
   }).catch(() => { failures = [...failures, id]; });
  }
 }
 $effect(() => {
  if (ready) fetchClasses();
 });
 onMount(() => {
  const observer = new ResizeObserver(() => { positionToolbar(); reportGeometry(); });
  observer.observe(previewElement);
  previewElement.addEventListener('scroll', positionToolbar, true);
  rt.start();
  fetchClasses();
  ready = true;
  return () => { observer.disconnect(); previewElement.removeEventListener('scroll', positionToolbar, true); };
 });
</script>

{#snippet paneTree(pane: PaneSpec, axis: 'width' | 'height' | undefined, next: string | undefined = undefined, inheritedColors: SurfaceColors = {}, inheritedBorder: PaneBorder = {})}
 {@const colors = inheritPaneColors(pane, inheritedColors)}
 {@const border = inheritPaneBorder(pane, inheritedBorder)}
 <section class="pane" class:pane-selected={selectedPane === pane.pane_id} data-editor-pane={isLeaf(pane) ? pane.pane_id : undefined} data-pane-id={pane.pane_id} data-pane-axis={axis}
  style={`${axis ? `${paneStyle(pane, axis)}; min-width:${MIN_PANE_SIZE}px; min-height:${MIN_PANE_SIZE}px; max-width:none; max-height:none; ${next ? '' : 'flex-grow:1'}` : rootPaneStyle(pane)}; ${surfaceColorStyle(colors)}; ${paneBorderStyle(border)}`}>
  {#if isLeaf(pane)}
   {@const items = entries(pane.pane_id)}
   {@const config = panelConfig(pane.pane_id)}
   {@const panel = page.panels?.[pane.pane_id]}
   {@const stretches = items.some((it) => !('error' in it.entry) && (it.entry.desc.width === 'elastic' || (typeof it.entry.desc.width === 'string' && it.entry.desc.width.endsWith('%'))))}
   <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
   <div style={placeStyle(pane)} onclick={(event) => { event.stopPropagation(); onSelectPane(pane.pane_id); }}>
    <div class="flow flex flex-wrap select-text" style="{flowStyle(config)}; {panelStyle(items.map((it) => ('error' in it.entry ? {} : it.entry.desc)))}; {surfaceColorStyle(panel ?? {}, !!colors.background_color)}; {flowMarginStyle(config)}; box-sizing: {stretches ? 'border-box' : 'content-box'}" data-flow-layout use:flowGeometry={{ config, count: items.length, sizeToRows: !items.some((it) => !('error' in it.entry) && (it.entry.desc.width === 'elastic' || (typeof it.entry.desc.width === 'string' && it.entry.desc.width.endsWith('%')))) }}>
     {#each items as item (item.node.id)}
      <div class="element min-w-0" class:flow-text={!('error' in item.entry) && item.entry.module === 'text'} class:stretch={!('error' in item.entry) && config.vertical_alignment === 'stretch' && (item.entry.desc.height === undefined || item.entry.desc.height === 'auto' || item.entry.desc.height === 'elastic')} use:measureElement={item.node.id} use:measure={'error' in item.entry || item.entry.module !== 'button' ? null : item.node.id} style={'error' in item.entry ? '' : flowItemStyle(item.entry.desc, config, elementMinimum(item.entry.module, item.entry.desc.editor))} data-flow-item data-editor-element={item.node.id}>
       {#if 'error' in item.entry}
        <ErrorBox title={m.wpi_error_title({ descriptor: page.descriptor_id })} message={item.entry.error} />
       {:else if rt.root}
        <ElementView instanceId={rt.root} element={item.entry} />
       {/if}
       <button class:selected={selectedId === item.node.id} class="overlay nodrag nopan" aria-label={m.wpi_preview_select({ name: String(item.node.data.label ?? item.node.data.element ?? '') })} aria-pressed={selectedId === item.node.id} onpointerdown={(event) => startReorder(event, item.node.id)} onpointermove={moveReorder} onpointerup={endReorder} onpointercancel={(event) => endReorder(event, true)} onclick={(event) => { event.stopPropagation(); onSelect(item.node.id); }}></button>
       {#if selectedId === item.node.id && elementSizes[item.node.id]}
        {#each resizeHandles(elementSizes[item.node.id].width, elementSizes[item.node.id].height, zoom || 1) as handle}
         <button type="button" class="element-grip nodrag nopan {handle}" aria-label={m.wpi_resize_element({ direction: handle })} title={m.wpi_resize_element({ direction: handle })} onpointerdown={(event) => startElementResize(event, item.node.id, handle)} onpointermove={moveElementResize} onclick={(event) => event.stopPropagation()} onkeydown={(event) => elementResizeKey(event, item.node.id, handle)} onpointerup={endElementResize} onpointercancel={(event) => endElementResize(event, true)}></button>
        {/each}
       {/if}
      </div>
     {/each}
    </div>
   </div>
   <button type="button" class="pane-tag nodrag nopan" aria-label={m.wpi_select_pane({ pane: pane.pane_id })} aria-pressed={selectedPane === pane.pane_id} onpointerdown={(event) => event.stopPropagation()} onclick={(event) => { event.stopPropagation(); onSelectPane(pane.pane_id); }}>{pane.pane_id}</button>
  {:else}
   <div style={splitStyle(pane)}>
    {#each pane.children ?? [] as child, i (child.pane_id)}{@render paneTree(child, pane.split === 'columns' ? 'width' : 'height', pane.children?.[i + 1]?.pane_id, colors, border)}{/each}
   </div>
  {/if}
  {#if axis && next}
   <div role="separator" aria-label={m.wpi_resize_pane()} aria-orientation={axis === 'width' ? 'vertical' : 'horizontal'} class="pane-divider nodrag nopan" class:vertical={axis === 'width'} onpointerdown={(event) => startPaneResize(event, pane.pane_id, next, axis)} onpointermove={movePaneResize} onpointerup={(event) => endPaneResize(event)} onpointercancel={(event) => endPaneResize(event, true)}></div>
  {/if}
 </section>
{/snippet}

<div class="page-status nodrag nopan select-text" style:width="{size.width}px" data-wpi-page-status>
 <span>{m.wpi_status_page_name({ name: pageName })}</span>
 <span>{dirty ? m.wpi_status_dirty() : m.wpi_status_clean()}</span>
 <span>{m.wpi_status_auto_saved({ time: autoSavedAt ? new Date(autoSavedAt).toLocaleString() : m.wpi_status_never() })}</span>
 <span>{m.wpi_status_opened({ time: openedAt ? new Date(openedAt).toLocaleString() : m.wpi_status_never() })}</span>
</div>
<svelte:window onkeydown={(event) => { if (event.key === 'Escape' && resizeNotice) { event.preventDefault(); resizeNotice = null; } }} />
<div bind:this={previewElement} class="preview select-text" style="width:{size.width}px; height:{size.height}px; color-scheme:{darkMode ? 'dark' : 'light'}" data-wpi-page-preview>
 {#if elementOutline}<div class="element-resize-outline" style:left="{elementOutline.x}px" style:top="{elementOutline.y}px" style:width="{elementOutline.width}px" style:height="{elementOutline.height}px" aria-hidden="true"></div>{/if}
 {#if reorderLine}<div class="reorder-line" style:left="{reorderLine.x}px" style:top="{reorderLine.y}px" style:height="{reorderLine.height}px" aria-hidden="true"></div>{/if}
 {#if failures.length}<ErrorBox title={m.wpi_editor_element_classes_failed()} message={failures.join(', ')} />{/if}
 {#if ready}
  <div style="{placeStyle({ hori_align: page.hori_align, vert_align: page.vert_align, hori_scrollbar: page.hori_scrollbar, vert_scrollbar: page.vert_scrollbar })}; padding: 0 40px 40px">
   <div style="{pageBoxStyle(page)}; position:relative; max-width:min(100%, {cssLength(page.max_width) ?? '100%'}); max-height:min(100%, {cssLength(page.max_height) ?? '100%'}); {(!page.height || page.height === 'auto') ? 'height:100%' : ''}" data-page={page.descriptor_id} data-page-box>
    {@render paneTree(root, undefined)}
   </div>
  </div>
 {/if}
    {#each ['bottom', 'right'] as side}
     <div role="separator" aria-label={m.wpi_resize_preview()} aria-orientation={side === 'left' || side === 'right' ? 'vertical' : 'horizontal'} class="outer-edge nodrag nopan {side}" onpointerdown={(event) => startOuter(event, side as Side)} onpointermove={moveOuter} onpointerup={(event) => endOuter(event)} onpointercancel={(event) => endOuter(event, true)}></div>
    {/each}
 <button type="button" class="resize-handle nodrag nopan select-none" aria-label={m.wpi_resize_preview()} title={m.wpi_resize_preview()} onpointerdown={startResize} onpointermove={resize} onpointerup={(event) => endResize(event)} onpointercancel={(event) => endResize(event, true)} onkeydown={resizeKey}></button>
</div>
 {#if actions && selectedId && toolbarPosition}
  <div class="element-toolbar nodrag nopan" role="group" aria-label={m.wpi_element_actions()} style:left="{toolbarPosition.left}px" style:top="{toolbarPosition.top}px" onpointerdown={(event) => event.stopPropagation()}>
   <DropdownMenu.Root>
    <DropdownMenu.Trigger>
     {#snippet child({ props })}
      <button {...props} title={m.wpi_preview_layouter_view_tooltip()} aria-label={m.wpi_preview_layouter_view_label()}><EyeIcon class="w-4 h-4" /></button>
     {/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content align="start" class="min-w-40">
     <DropdownMenu.Item disabled={!pageLayouterAvailable} onclick={() => onViewLayouter('page-layouter')}>{m.wpi_preview_page_layouter()}</DropdownMenu.Item>
     <DropdownMenu.Item disabled={!elementLayouterAvailable} onclick={() => onViewLayouter('element-layouter')}>{m.wpi_preview_element_layouter()}</DropdownMenu.Item>
    </DropdownMenu.Content>
   </DropdownMenu.Root>
   {#if actions.editClass}<button title={m.basenode_edit_class()} aria-label={m.basenode_edit_class()} onclick={(event) => { event.stopPropagation(); actions.editClass?.(selectedId!); }}><PencilIcon class="w-4 h-4" /></button>{/if}
   <button title={m.basenode_duplicate()} aria-label={m.basenode_duplicate()} onclick={(event) => { event.stopPropagation(); actions.duplicate(selectedId!); }}><CopyIcon class="w-4 h-4" /></button>
   <button title={m.basenode_delete()} aria-label={m.basenode_delete()} onclick={(event) => { event.stopPropagation(); actions.remove(selectedId!); }}><Trash2Icon class="w-4 h-4" /></button>
  </div>
 {/if}
{#if resizeNotice && selectedId === resizeNotice.id}
 <div class="resize-notice nodrag nopan" role="dialog" tabindex="-1" aria-label={m.wpi_resize_confirmation()} style:left="{resizeNotice.left}px" style:top="{resizeNotice.top}px" onpointerdown={(event) => event.stopPropagation()}>
  <span>{resizeNotice.message}</span><div class="resize-notice-actions"><button type="button" bind:this={resizeCancelButton} use:resizeDialogButton={cancelResize}>{m.wpi_create_element_cancel()}</button><button type="button" use:resizeDialogButton={confirmResize}>{m.wpi_create_element_saved_ok()}</button></div>
 </div>
{/if}
{#if resizeOutline}
 <div class="resize-outline" style:width="{resizeOutline.width}px" style:height="{resizeOutline.height}px" aria-hidden="true"></div>
{/if}

<style>
 .resize-notice { position:absolute; z-index:20; width:250px; display:flex; flex-direction:column; gap:8px; box-sizing:border-box; padding:10px 12px; border:1px solid var(--cv-border); border-radius:6px; background:var(--cv-panel); color:var(--cv-text); box-shadow:0 4px 16px #0004; font-size:11px; }
 .resize-notice-actions { display:flex; justify-content:flex-end; gap:6px; width:100%; }
 .resize-notice button { border:1px solid var(--cv-border); border-radius:4px; padding:3px 9px; background:var(--cv-surface); color:inherit; font-size:11px; cursor:pointer; }
 .page-status { position:absolute; left:0; top:-32px; height:32px; display:flex; align-items:center; gap:16px; padding:0 10px; box-sizing:border-box; overflow:auto; white-space:nowrap; background:var(--cv-surface); color:var(--cv-text); border:1px solid var(--cv-border); font-size:11px; pointer-events:auto; }
 .preview { position: absolute; left: 0; top: 0; background: var(--background); color: var(--foreground); border: 1px solid var(--cv-border); overflow: hidden; pointer-events: auto; }
 .element-resize-outline { position:absolute; box-sizing:border-box; border:2px dashed var(--cv-accent); pointer-events:none; z-index:9; }
 .resize-outline { position:absolute; left:0; top:0; box-sizing:border-box; border:2px dashed var(--cv-accent); pointer-events:none; z-index:6; }
 .element-toolbar { pointer-events:auto; position:absolute; z-index:10; transform:translate(-50%, -100%); display:flex; gap:4px; background:var(--cv-toolbar); border:1px solid var(--cv-border); border-radius:8px; padding:4px 6px; }
 .element-toolbar button { background:none; border:0; cursor:pointer; color:var(--cv-text-2); padding:4px; min-width:28px; min-height:28px; border-radius:4px; display:flex; align-items:center; justify-content:center; }
 .element-toolbar button:hover { background:var(--cv-toolbar-hover); color:#fff; }
 .element-toolbar button:focus-visible { outline:2px solid var(--cv-accent); outline-offset:1px; }
 .element-toolbar button:last-child { color:#ef4444; }
 .element-toolbar button:last-child:hover { color:#fff; }
 .resize-handle { position:absolute; right:0; bottom:0; width:18px; height:18px; z-index:5; border:0; background: linear-gradient(135deg, transparent 45%, var(--cv-accent) 46%, var(--cv-accent) 52%, transparent 53%, transparent 64%, var(--cv-accent) 65%, var(--cv-accent) 71%, transparent 72%); cursor:nwse-resize; touch-action:none; }
 .resize-handle:focus-visible { outline:2px solid var(--cv-accent); }
 .pane { position: relative; outline: 1px dashed rgba(99,102,241,.25); outline-offset: -1px; }
 .outer-edge { position:absolute; z-index:4; touch-action:none; }
 .outer-edge.top, .outer-edge.bottom { left:5px; right:5px; height:8px; cursor:row-resize; }
 .outer-edge.top { top:0; } .outer-edge.bottom { bottom:0; }
 .outer-edge.left, .outer-edge.right { top:5px; bottom:5px; width:8px; cursor:col-resize; }
 .outer-edge.left { left:0; } .outer-edge.right { right:0; }
 .outer-edge:hover { background:rgba(99,102,241,.25); }
 .pane-divider { position:absolute; z-index:4; bottom:-4px; left:0; right:0; height:8px; cursor:row-resize; touch-action:none; }
 .pane-divider.vertical { top:0; bottom:0; left:auto; right:-4px; width:8px; height:auto; cursor:col-resize; }
 .pane-divider:hover { background:rgba(99,102,241,.25); }
 .pane-selected { outline: 2px solid #6366f1 !important; outline-offset: -2px !important; }
 .pane-tag { position: absolute; top: 2px; right: 2px; z-index: 3; border: 0; border-radius: 3px; padding: 0 4px; font-size: 9px; line-height: 14px; background: rgba(99,102,241,.12); color: var(--cv-text-muted); cursor: pointer; }
 .pane-tag[aria-pressed='true'] { background: #6366f1; color: #fff; }
 .flow { position: relative; }
 .element { position: relative; box-sizing: border-box; overflow: auto; }
 .stretch { display: flex; flex-direction: column; }
 .stretch > :global(:not(.overlay):not(.element-grip)) { flex: 1 1 auto; }
 .overlay { position: absolute; inset: 0; z-index: 2; width: 100%; height: 100%; min-height: 20px; background: transparent; border: 1px solid transparent; cursor: pointer; }
 .element-grip { position:absolute; z-index:3; width:10px; height:10px; padding:0; border:1px solid #6366f1; border-radius:50%; background:white; touch-action:none; }
 .element-grip.nw, .element-grip.n, .element-grip.ne { top:0; }
 .element-grip.sw, .element-grip.s, .element-grip.se { bottom:0; }
 .element-grip.nw, .element-grip.w, .element-grip.sw { left:0; }
 .element-grip.ne, .element-grip.e, .element-grip.se { right:0; }
 .element-grip.n, .element-grip.s { left:50%; transform:translateX(-50%); cursor:ns-resize; }
 .element-grip.e, .element-grip.w { top:50%; transform:translateY(-50%); cursor:ew-resize; }
 .element-grip.nw, .element-grip.se { cursor:nwse-resize; }
 .element-grip.ne, .element-grip.sw { cursor:nesw-resize; }
 .overlay:hover { background: rgba(99,102,241,.08); border-color: rgba(99,102,241,.5); }
 .overlay.selected, .overlay:focus-visible { background: rgba(99,102,241,.08); border: 2px solid #6366f1; outline: none; }
.reorder-line { position: absolute; z-index: 4; width: 3px; margin-left: -1px; border-radius: 2px; background: #6366f1; pointer-events: none; }
</style>
