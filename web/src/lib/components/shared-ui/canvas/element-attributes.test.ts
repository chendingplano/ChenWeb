import test from 'node:test';
import assert from 'node:assert/strict';
import { valueText, withAttribute } from './element-attributes';

test('valueText shows strings bare and other values as JSON', () => {
	assert.equal(valueText(undefined), '');
	assert.equal(valueText('elastic'), 'elastic');
	assert.equal(valueText({ en: 'Save', 'zh-cn': '保存' }), '{"en":"Save","zh-cn":"保存"}');
	assert.equal(valueText(200), '200');
	assert.equal(valueText(null), 'null');
});

test('valueText quotes strings that would read back as something else', () => {
	assert.equal(valueText('200'), '"200"');
	assert.equal(valueText('true'), '"true"');
	assert.equal(valueText(' padded '), '" padded "');
});

test('withAttribute reads JSON when it parses, otherwise text', () => {
	assert.deepEqual(withAttribute({}, 'label', 'Save'), { label: 'Save' });
	assert.deepEqual(withAttribute({}, 'on_click', '{"action": "save-page"}'), { on_click: { action: 'save-page' } });
	assert.deepEqual(withAttribute({}, 'label', '{"en": "Sa'), { label: '{"en": "Sa' });
});

test('withAttribute unsets an attribute on empty text and keeps the others', () => {
	const attrs = { label: 'Save', width: '240px' };
	assert.deepEqual(withAttribute(attrs, 'label', '  '), { width: '240px' });
	assert.deepEqual(attrs, { label: 'Save', width: '240px' });
});

test('every value reads back from the text it is shown as', () => {
	for (const v of ['elastic', '200', 'true', ' padded ', 200, false, null, ['a'], { en: 'Name' }]) {
		assert.deepEqual(withAttribute({}, 'x', valueText(v)).x, v);
	}
});

test('source field edits preserve the source and other attributes, including defaults', async () => {
	const { withSourceFields } = await import('./element-attributes');
	const defaults = { action: 'load', table: 'customers', fields: [{ column: 'id', attribute: 'id' }] };
	assert.deepEqual(withSourceFields({ width: '100%' }, defaults, [{ column: 'name', attribute: 'title' }]), {
		width: '100%', source: { action: 'load', table: 'customers', fields: [{ column: 'name', attribute: 'title' }] }
	});
	assert.deepEqual(withSourceFields({ source: { action: 'query', limit: 20 } }, defaults, []), {
		source: { action: 'query', limit: 20, fields: [] }
	});
});

test('source property edits retain other settings and can initialize a source', async () => {
	const { withSourceProperty } = await import('./element-attributes');
	assert.deepEqual(withSourceProperty({ source: { action: 'load', where: { id: 1 } } }, undefined, 'limit', 25), { source: { action: 'load', where: { id: 1 }, limit: 25 } });
	assert.deepEqual(withSourceProperty({}, null, 'action', 'load'), { source: { action: 'load' } });
	assert.deepEqual(withSourceProperty({ source: { limit: 25, action: 'load' } }, undefined, 'limit', undefined), { source: { action: 'load' } });
});

test('list source defaults are persisted without replacing explicit choices', async () => {
 const { withListSourceDefaults } = await import('./element-attributes');
 assert.deepEqual(withListSourceDefaults({ source: { table: 'kb.people' } }, null), { source: { table: 'kb.people', action: 'load', result: 'list' } });
 assert.deepEqual(withListSourceDefaults({ source: { table: 'kb.people', action: 'query', result: 'list' } }, null), { source: { table: 'kb.people', action: 'query', result: 'list' } });
});

test('selecting a table clears old field mappings and keeps other source settings', async () => {
 const { withSelectedTable } = await import('./element-attributes');
 const attrs = { width: '100%', source: { table: 'old.items', fields: [{ column: 'id', attribute: 'id' }], limit: 25, action: 'load' } };
 assert.deepEqual(withSelectedTable(attrs, undefined, 'new.items'), { width: '100%', source: { table: 'new.items', fields: [], limit: 25, action: 'load', result: 'list' } });
 assert.deepEqual(withSelectedTable(attrs, undefined, 'old.items').source, { table: 'old.items', fields: [], limit: 25, action: 'load', result: 'list' });
 assert.deepEqual(attrs.source.fields, [{ column: 'id', attribute: 'id' }]);
});

test('unit-bearing element attributes accept bare numeric input as pixels', () => {
 assert.deepEqual(withAttribute({}, 'width', '150'), { width: '150px' });
 assert.deepEqual(withAttribute({}, 'label_font_size', '1.5'), { label_font_size: '1.5px' });
 assert.deepEqual(withAttribute({}, 'width', '2rem'), { width: '2rem' });
 assert.deepEqual(withAttribute({}, 'max_length', '150'), { max_length: 150 });
});
