import test from 'node:test';
import assert from 'node:assert/strict';
import { commonSenseEditor } from './common-sense';

test('a property fixed by its class is read-only', () => {
	assert.equal(commonSenseEditor('width', '10px', true), 'read-only');
});

test('booleans and numbers by their default', () => {
	assert.equal(commonSenseEditor('resizable', false), 'switch');
	assert.equal(commonSenseEditor('rows', 3), 'number');
});

test('lengths by name', () => {
	for (const name of ['width', 'height', 'min_width', 'max_height', 'label_max_width', 'icon_gap', 'left_margin', 'font_size', 'border_thickness'])
		assert.equal(commonSenseEditor(name, undefined), 'length', name);
});

test('colours by name', () => {
	assert.equal(commonSenseEditor('border_color', undefined), 'color');
	assert.equal(commonSenseEditor('label_color', '#000'), 'color');
});

test('text people read is localized', () => {
	assert.equal(commonSenseEditor('placeholder', undefined), 'localized-text');
	assert.equal(commonSenseEditor('caption', { en: 'Hi', 'zh-cn': '你好' }), 'localized-text');
});

test('other objects and arrays are JSON, everything else text', () => {
	assert.equal(commonSenseEditor('options', [{ value: 'a' }]), 'json');
	assert.equal(commonSenseEditor('source', { action: 'load' }), 'json');
	assert.equal(commonSenseEditor('data_binding', undefined), 'text');
	assert.equal(commonSenseEditor('required_field', null), 'text');
});
