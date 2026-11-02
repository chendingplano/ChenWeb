import { test, expect } from 'bun:test';
import {
	attributeTree,
	classTarget,
	renameAttribute,
	setAttributeDefault,
	setAttributeDefinition,
	definitionAt
} from './class-editor';
import type { JsonObject } from '$lib/wpi/types';

const element: JsonObject = {
	descriptor_id: 'sample',
	schema_version: '0.4',
	code_module_id: 'list',
	source: { help_tip: { en: 'Rows' }, default_value: { table: 'kb.inputs', fields: ['id'] } },
	required: { default_value: null },
	optional: {},
	fixed: { default_value: 3, instance_modifiable: false }
};

test('Class reads label scope and editor directly from the attribute object', () => {
	const row: JsonObject = {
		descriptor_id: 'button',
		schema_version: '0.4',
		code_module_id: 'button',
		label: {
			default_value: 'Button',
			help_tip: { en: 'Caption', 'zh-cn': '标题' },
			scope: 'instance',
			value_editor: { id: 'localized-text', options: {} }
		}
	};
	expect(definitionAt('element', row, '/label')).toEqual({
		description: { en: 'Caption', 'zh-cn': '标题' },
		scope: 'instance',
		value_editor: { id: 'localized-text', options: {} }
	});
	expect(row.attribute_definitions).toBeUndefined();
});

test('Class does not invent missing values from code or rewrite the loaded body', () => {
	const row: JsonObject = {
		descriptor_id: 'button',
		schema_version: '0.4',
		code_module_id: 'button',
		label: { default_value: 'Button' }
	};
	const before = JSON.stringify(row);
	expect(definitionAt('element', row, '/label').scope).toBeUndefined();
	expect(definitionAt('element', row, '/label').value_editor).toBeUndefined();
	expect(JSON.stringify(row)).toBe(before);
});

test('scope and editor changes are written inline beside help_tip and default_value', () => {
	const row: JsonObject = {
		descriptor_id: 'button',
		schema_version: '0.4',
		label: {
			default_value: 'Button',
			help_tip: 'Caption',
			scope: 'instance',
			value_editor: { id: 'localized-text', options: {} }
		}
	};
	let next = setAttributeDefinition('element', row, '/label', {
		scope: 'class',
		value_editor: { id: 'number', options: { min: 1 } }
	});
	expect(next.label).toEqual({
		default_value: 'Button',
		help_tip: 'Caption',
		scope: 'class',
		instance_modifiable: false,
		value_editor: { id: 'number', options: { min: 1 } }
	});
	expect(next.attribute_definitions).toBeUndefined();
	next = setAttributeDefinition('element', next, '/label', { scope: 'instance', description: '' });
	expect((next.label as JsonObject).scope).toBe('instance');
	expect((next.label as JsonObject).instance_modifiable).toBeUndefined();
	expect((next.label as JsonObject).help_tip).toBeUndefined();
});

test('explicit empty editor options are shown exactly as stored', () => {
	const row: JsonObject = {
		descriptor_id: 'input',
		schema_version: '0.4',
		code_module_id: 'input',
		width: {
			default_value: '200px',
			scope: 'instance',
			value_editor: { id: 'length', options: {} }
		}
	};
	expect(definitionAt('element', row, '/width').value_editor).toEqual({
		id: 'length',
		options: {}
	});
});

test('Class selection resolves elements, actions, panes and panel layouters, never a page or flow node', () => {
	const page: JsonObject = {
		page_layouter: { layouter: 'columns' },
		panels: { main: { elem_layouter: { layouter: 'flow-layouter' } } }
	};
	expect(
		classTarget({ type: 'wpi-element', data: { element: 'input' } }, page, null, null)
	).toEqual({ kind: 'element', id: 'input' });
	expect(classTarget({ type: 'wpi-action', data: { action: 'save' } }, page, null, null)).toEqual({
		kind: 'action',
		id: 'save'
	});
	expect(classTarget(null, page, 'panes', 'main')).toEqual({
		kind: 'page-layouter',
		id: 'columns'
	});
	expect(classTarget(null, page, 'panel', 'main')).toEqual({
		kind: 'element-layouter',
		id: 'flow-layouter'
	});
	expect(classTarget(null, page, 'page', null)).toBeNull();
	expect(classTarget({ type: 'text', data: { label: 'Text' } }, page, null, null)).toBeNull();
	expect(classTarget({ type: 'wpi-action', data: {} }, page, null, null)).toBeNull();
});

test('tree unwraps element attributes, includes nested arrays, hides editor metadata', () => {
	const tree = attributeTree('element', {
		...element,
		attribute_definitions: { '/source': { scope: 'instance' } }
	});
	const source = tree.find((a) => a.name === 'source')!;
	expect(source.children.map((a) => a.path)).toEqual(['/source/table', '/source/fields']);
	expect(source.children[1].children[0].path).toBe('/source/fields/0');
	expect(tree.find((a) => a.name === 'required')!.value).toBeNull();
	expect(tree.find((a) => a.name === 'optional')!.value).toBeUndefined();
	expect(tree.some((a) => a.name === 'attribute_definitions')).toBe(false);
});

test('nested edits preserve help, fixed flags, siblings and the original class', () => {
	const edited = setAttributeDefault('element', element, '/source/table', 'kb.metrics');
	expect(edited.source).toEqual({
		help_tip: { en: 'Rows' },
		default_value: { table: 'kb.metrics', fields: ['id'] }
	});
	expect((element.source as JsonObject).default_value).toEqual({
		table: 'kb.inputs',
		fields: ['id']
	});
	expect(setAttributeDefault('element', edited, '/optional', undefined).optional).toEqual({});
	expect(setAttributeDefault('element', edited, '/required', null).required).toEqual({
		default_value: null
	});
});

test('renaming moves descendant metadata without deleting other definitions', () => {
	const row = {
		...element,
		attribute_definitions: {
			'/source/table': { description: 'Table', value_editor: { id: 'table-picker', options: {} } },
			'/fixed': { scope: 'class' }
		}
	} as JsonObject;
	const next = renameAttribute('element', row, '/source', 'rows');
	expect(next.source).toBeUndefined();
	expect(next.rows).toEqual(element.source);
	expect((next.attribute_definitions as JsonObject)['/rows/table']).toEqual(
		(row.attribute_definitions as JsonObject)['/source/table']
	);
	expect((next.attribute_definitions as JsonObject)['/fixed']).toEqual({ scope: 'class' });
	expect(() => renameAttribute('element', row, '/source', 'fixed')).toThrow();
	expect(() => renameAttribute('element', row, '/descriptor_id', 'renamed')).toThrow();
});

test('scope uses fixed defaults and editor metadata survives further edits', () => {
	let row = setAttributeDefinition('element', element, '/fixed', {
		scope: 'instance',
		value_editor: { id: 'number', options: { min: 1, integer: true } }
	});
	expect((row.fixed as JsonObject).instance_modifiable).toBeUndefined();
	row = setAttributeDefinition('element', row, '/fixed', { scope: 'class' });
	expect((row.fixed as JsonObject).instance_modifiable).toBe(false);
	expect(definitionAt('element', row, '/fixed').value_editor).toEqual({
		id: 'number',
		options: { min: 1, integer: true }
	});
	expect(definitionAt('element', row, '/source').description).toEqual({ en: 'Rows' });
	expect(() => setAttributeDefinition('element', row, '/required', { scope: 'class' })).toThrow();
});

test('layouter trees edit the stored template, preserving descriptor identity and config', () => {
	const row: JsonObject = {
		descriptor_id: 'flow-layouter',
		schema_version: '0.5',
		config: { column_gap: '10px' }
	};
	const next = setAttributeDefault('element-layouter', row, '/config/column_gap', '20px');
	expect(next).toEqual({ ...row, config: { column_gap: '20px' } });
	expect(row.config).toEqual({ column_gap: '10px' });
});

test('editing accepts reactive proxies without attempting to clone the proxy itself', () => {
	const reactive = new Proxy(element, {});
	expect(setAttributeDefault('element', reactive, '/fixed', 4).fixed).toEqual({
		default_value: 4,
		instance_modifiable: false
	});
	expect(
		(
			setAttributeDefinition('element', reactive, '/fixed', { description: 'Fixed' })
				.fixed as JsonObject
		).help_tip
	).toBe('Fixed');
	expect(renameAttribute('element', reactive, '/fixed', 'renamed').renamed).toEqual(element.fixed);
});
