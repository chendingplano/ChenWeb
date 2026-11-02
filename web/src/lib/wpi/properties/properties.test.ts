import test from 'node:test';
import assert from 'node:assert/strict';
import { getLocale } from '$lib/paraglide/runtime';
import { textGroups } from '../text-attributes';
import type { ElementClassSummary } from '../api';
import type { JsonObject } from '../types';
import { elementEditor, page, pane, panel } from './index';
import { elementSource, pageSource, panelSource, paneSource } from './sources';
import type { EditorDef, PropertyContext, PropertyDef } from './types';

function prop(def: EditorDef, name: string): PropertyDef {
	const found = def.groups.flatMap((g) => g.properties).find((p) => p.name === name);
	assert.ok(found, name);
	return found;
}

function ctx(values: JsonObject = {}, extra: Partial<PropertyContext> = {}): PropertyContext {
	return { get: (n) => values[n], triggerActions: [], pageLayouters: [], ...extra };
}

function options(p: PropertyDef, c: PropertyContext) {
	return typeof p.options === 'function' ? p.options(c) : (p.options ?? {});
}

test('input: groups in order and its input types', () => {
	const input = elementEditor('input');
	assert.deepEqual(
		input.groups.map((g) => g.id),
		['placement', 'general', 'caption_layout', 'caption_appearance', 'input_help', 'validation', 'size']
	);
	assert.deepEqual(options(prop(input, 'input_type'), ctx()).choices, ['text', 'password', 'email', 'number', 'tel', 'url', 'search']);
});

test('input: caption gaps and alignment follow the placement', () => {
	const input = elementEditor('input');
	const shown = (name: string, placement: string) => prop(input, name).visible!(ctx({ label_placement: placement }));
	assert.equal(shown('label_horizontal_gap', 'left'), true);
	assert.equal(shown('label_horizontal_gap', 'top'), false);
	assert.equal(shown('label_vertical_gap', 'top'), true);
	assert.equal(shown('label_vertical_align', 'bottom'), false);
	assert.equal(prop(input, 'max_length').visible!(ctx({ input_type: 'number' })), false);
	assert.deepEqual(options(prop(input, 'validation'), ctx({ input_type: 'number' })), { numberRules: true });
});

test('text: the six groups of the text module, label and icon settings only when used', () => {
	const text = elementEditor('text');
	assert.deepEqual(
		text.groups.slice(1).map((g) => [g.id, g.properties.map((p) => p.name)]),
		textGroups
	);
	assert.equal(prop(text, 'label_placement').visible!(ctx({})), false);
	assert.equal(prop(text, 'label_placement').visible!(ctx({ label: 'Name' })), true);
	assert.equal(prop(text, 'icon_size').visible!(ctx({})), false);
	assert.equal(prop(text, 'font_weight').write!('500'), 500);
	assert.equal(prop(text, 'font_size').validate!('12xy', ctx()), prop(text, 'font_size').validate!('nope', ctx()));
	assert.equal(prop(text, 'font_size').validate!('12px', ctx()), null);
	assert.ok(prop(text, 'data_binding').warn!('$input_doc.name', ctx({ text: 'x' })));
});

test('button: on_click is chosen from the action classes and stored as an action instance', () => {
	const onClick = prop(elementEditor('button'), 'on_click');
	assert.deepEqual(options(onClick, ctx({}, { triggerActions: ['save', 'open-page'] })).choices, ['save', 'open-page']);
	assert.equal(onClick.read!({ action: 'open-page', page: 'x' }), 'open-page');
	assert.deepEqual(onClick.write!('save'), { action: 'save' });
	assert.equal(onClick.write!(undefined), undefined);
});

test('menu and chat: choices from the element\'s other values', () => {
	const items: JsonObject[] = [{ item_id: 'a' }, { item_id: 'b', children: [{ item_id: 'c' }] }];
	assert.deepEqual(options(prop(elementEditor('menu'), 'initial_item'), ctx({ items })).choices, ['a', 'c']);
	assert.deepEqual(options(prop(elementEditor('chat-composer'), 'default_agent'), ctx({ agents: ['Codex'] })).choices, ['Codex']);
	assert.deepEqual(options(prop(elementEditor('chat-panel'), 'initial_tab'), ctx({})).choices, ['chat', 'dialog']);
	assert.equal(prop(elementEditor('chat-composer'), 'slash_commands').read!(undefined), true);
});

test('an unknown module shows its placement only; its attributes go to Other', () => {
	assert.deepEqual(elementEditor('gauge').groups.map((g) => g.id), ['placement']);
});

test('page: scroll bars, refresh and the page layouter', () => {
	const bar = prop(page, 'hori_scrollbar');
	assert.equal(bar.read!('hide'), false);
	assert.equal(bar.read!(undefined), true);
	assert.equal(bar.write!(false), 'hide');
	const refresh = prop(page, 'refresh');
	assert.equal(refresh.read!({ on_show: true }), true);
	assert.equal(refresh.write!(false), undefined);
	assert.equal(prop(page, 'page_layouter').read!({ layouter: 'single-pane' }), 'single-pane');
	assert.equal(prop(page, 'bkgd_color').validate!('auto', ctx()), null);
	assert.ok(prop(pane, 'background_color').validate!('auto', ctx()));
	assert.equal(prop(pane, 'background_color').validate!({ light: '#fff', dark: '#000' }, ctx()), null);
	assert.equal(pane.groups.find((g) => g.id === 'content')!.visible!(ctx({}, { leaf: false })), false);
	assert.equal(options(prop(panel, 'column_gap'), ctx()).units?.join(), 'px,rem');
});

const cls: ElementClassSummary = {
	descriptor_id: 'input',
	version: 1,
	status: 'active',
	description: null,
	attributes: {
		label: { default_value: null, help_tip: { en: 'Caption', 'zh-cn': '标题' } },
		width: { default_value: '200px' },
		binding_mode: { default_value: 'read-only', instance_modifiable: false }
	}
};

test('element source: own value, then class default, then module fallback', () => {
	let saved: JsonObject | null = null;
	const s = elementSource({ width: '10px' }, cls, { label_placement: 'left' }, (attrs) => (saved = attrs));
	assert.equal(s.get('width'), '10px');
	assert.equal(s.get('label_placement'), 'left');
	assert.equal(s.isSet('width'), true);
	assert.equal(s.required('label'), true);
	assert.equal(s.fixed('binding_mode'), true);
	assert.equal(s.helpTip('label'), getLocale() === 'en' ? 'Caption' : '标题');
	assert.deepEqual(s.names, ['label', 'width', 'binding_mode']);
	s.set('width', undefined);
	assert.deepEqual(saved, {});
	s.set('label', 'Name');
	assert.deepEqual(saved, { width: '10px', label: 'Name' });
	assert.equal(elementSource({}, cls, {}, () => {}).get('width'), '200px');
});

const tree: JsonObject = {
	descriptor_id: 'p',
	page_layouter: { layouter: 'two', root: { pane_id: 'main', split: 'columns', children: [{ pane_id: 'a' }, { pane_id: 'b' }] } },
	panels: { a: { elem_layouter: { layouter: 'flow-layouter', config: { row_gap: '4px' } } } }
};

test('page source: the page layouter is set by choosing a class; it has no Reset', () => {
	const chosen: string[] = [];
	let next: JsonObject | null = null;
	const s = pageSource(tree, (p) => (next = p), (id) => chosen.push(id));
	s.set('page_layouter', 'single-pane');
	assert.deepEqual(chosen, ['single-pane']);
	assert.equal(next, null);
	assert.equal(s.resettable('page_layouter'), false);
	s.set('hori_align', 'left');
	assert.equal(next!.hori_align, 'left');
	assert.deepEqual(s.names, ['descriptor_id']);
});

test('pane source: attributes of the pane; renaming goes through onRename', () => {
	let next: JsonObject | null = null;
	const renamed: string[] = [];
	const s = paneSource(tree, 'a', (p) => (next = p), (to) => renamed.push(to));
	s.set('width', '120px');
	const root = (next!.page_layouter as JsonObject).root as JsonObject;
	assert.equal(((root.children as JsonObject[])[0]).width, '120px');
	s.set('pane_id', 'left');
	assert.deepEqual(renamed, ['left']);
	assert.equal(s.resettable('pane_id'), false);
});

test('panel source: flow parameters with their defaults, colours beside the layouter', () => {
	let next: JsonObject | null = null;
	const s = panelSource(tree, 'a', (p) => (next = p));
	assert.equal(s.get('row_gap'), '4px');
	assert.equal(s.get('column_gap'), '0.75rem');
	assert.equal(s.isSet('column_gap'), false);
	assert.equal(s.get('layouter'), 'flow-layouter');
	s.set('row_gap', undefined);
	assert.equal(((next!.panels as JsonObject).a as JsonObject).elem_layouter && (((next!.panels as JsonObject).a as JsonObject).elem_layouter as JsonObject).config, undefined);
	s.set('background_color', '#fff');
	assert.equal(((next!.panels as JsonObject).a as JsonObject).background_color, '#fff');
});

test('text: choosing no icon stores the empty string, so it overrides a class icon', () => {
	const icon = prop(elementEditor('text'), 'icon');
	assert.equal(icon.write!('none'), '');
	assert.equal(icon.read!(''), 'none');
	assert.equal(icon.write!('info'), 'info');
	assert.equal(options(icon, ctx()).choices![0], 'none');
});
