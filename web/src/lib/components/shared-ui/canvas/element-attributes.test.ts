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
