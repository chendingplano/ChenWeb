import test from 'node:test';
import assert from 'node:assert/strict';
import { attributeProblem, buildClassRow, CLASS_ID_PATTERN, draftAttribute, parseDefault } from './element-class-form';

test('a default is JSON when it parses, otherwise text', () => {
	assert.equal(parseDefault('100'), 100);
	assert.equal(parseDefault('true'), true);
	assert.deepEqual(parseDefault('{"en": "Name"}'), { en: 'Name' });
	assert.equal(parseDefault('"123"'), '123');
	assert.equal(parseDefault(' 200px '), '200px');
	assert.equal(parseDefault('read-write'), 'read-write');
});

test('a required attribute is stored as null', () => {
	assert.deepEqual(draftAttribute('label', true, 'ignored'), { name: 'label', value: null });
	assert.deepEqual(draftAttribute('max', false, '100'), { name: 'max', value: 100 });
});

test('attribute problems', () => {
	const added = [{ name: 'label', value: null }];
	assert.equal(attributeProblem('max-value', false, '1', added), 'name');
	assert.equal(attributeProblem('', false, '1', added), 'name');
	assert.equal(attributeProblem('element', true, '', added), 'reserved');
	assert.equal(attributeProblem('description', true, '', added), 'reserved');
	assert.equal(attributeProblem('label', true, '', added), 'duplicate');
	assert.equal(attributeProblem('max', false, '  ', added), 'no_default');
	assert.equal(attributeProblem('max', false, '100', added), null);
	assert.equal(attributeProblem('on_change', true, '', added), null);
});

test('class IDs', () => {
	assert.ok(CLASS_ID_PATTERN.test('slider'));
	assert.ok(CLASS_ID_PATTERN.test('date-range-2'));
	assert.ok(!CLASS_ID_PATTERN.test('Slider'));
	assert.ok(!CLASS_ID_PATTERN.test('date_range'));
});

test('the class row keeps the attributes in order and drops an empty Chinese description', () => {
	const attrs = [draftAttribute('label', true, ''), draftAttribute('max', false, '100')];
	assert.deepEqual(buildClassRow('slider', ' A slider ', '', attrs), {
		descriptor_id: 'slider',
		schema_version: '0.2',
		description: { en: 'A slider' },
		label: null,
		max: 100
	});
	assert.deepEqual(buildClassRow('slider', 'A slider', '滑块', []).description, { en: 'A slider', 'zh-cn': '滑块' });
	assert.deepEqual(Object.keys(buildClassRow('slider', 'x', '', attrs)), ['descriptor_id', 'schema_version', 'description', 'label', 'max']);
});
