import test from 'node:test';
import assert from 'node:assert/strict';
import type { Node } from '@xyflow/svelte';
import { isClassKey, nameProblem, renamed, withValue, without } from './node-descriptor';

const button: Node = { id: 'b', type: 'wpi-element', position: { x: 0, y: 0 }, data: { label: 'Save', element: 'button' } };
const page: Node = { id: 'p', type: 'wpi-page', position: { x: 0, y: 0 }, data: { label: 'Page' } };

test('an element node names its class with element; other nodes have no such key', () => {
	assert.equal(isClassKey(button, 'element'), true);
	assert.equal(isClassKey(button, 'label'), false);
	assert.equal(isClassKey(page, 'element'), false);
});

test('a name must be an attribute name the descriptor does not have yet', () => {
	const d = { element: 'button', label: 'Save' };
	assert.equal(nameProblem(d, 'on_click'), null);
	assert.equal(nameProblem(d, ''), 'name');
	assert.equal(nameProblem(d, '1st'), 'name');
	assert.equal(nameProblem(d, 'on-click'), 'name');
	assert.equal(nameProblem(d, 'label'), 'duplicate');
	assert.equal(nameProblem(d, 'element'), 'duplicate');
	assert.equal(nameProblem(d, 'constructor'), null, 'only the descriptor\'s own keys count');
});

test('a typed value is read as JSON when it parses, otherwise as text; a new one goes last', () => {
	const d = { element: 'button', label: 'Save' };
	assert.deepEqual(withValue(button, d, 'label', 'Send'), { element: 'button', label: 'Send' });
	assert.deepEqual(withValue(button, d, 'on_click', '{"action": "save-page"}'), { element: 'button', label: 'Save', on_click: { action: 'save-page' } });
	assert.deepEqual(Object.keys(withValue(button, d, 'width', '200')), ['element', 'label', 'width']);
	assert.deepEqual(withValue(button, d, 'width', '200').width, 200);
	assert.deepEqual(withValue(button, d, 'label', '').label, '');
});

test('the class key of an element node is always text', () => {
	assert.deepEqual(withValue(button, { element: 'button' }, 'element', ' 123 '), { element: '123' });
	assert.deepEqual(withValue(page, { label: 'Page' }, 'element', '123'), { label: 'Page', element: 123 });
});

test('a renamed attribute keeps its place and value', () => {
	const d = { element: 'button', label: 'Save', width: 200 };
	assert.deepEqual(Object.entries(renamed(d, 'label', 'text')), [['element', 'button'], ['text', 'Save'], ['width', 200]]);
});

test('a removed attribute is gone and the rest are left alone', () => {
	const d = { element: 'button', label: 'Save', width: 200 };
	assert.deepEqual(without(d, 'label'), { element: 'button', width: 200 });
	assert.deepEqual(d, { element: 'button', label: 'Save', width: 200 });
});
