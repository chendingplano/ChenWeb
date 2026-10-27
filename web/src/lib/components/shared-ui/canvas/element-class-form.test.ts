import test from 'node:test';
import assert from 'node:assert/strict';
import {
	attributeProblem,
	buildClassRow,
	CLASS_ID_PATTERN,
	draftAttribute,
	editProblem,
	editText,
	initialAttributes,
	isClassAttribute,
	parseDefault,
	parseValue,
	renameProblem,
	rowProblem
} from './element-class-form';

test('the form starts with the class attributes', () => {
	assert.deepEqual(initialAttributes(), [
		{ name: 'descriptor_id', value: '' },
		{ name: 'schema_version', value: '0.2' },
		{ name: 'description', value: { en: '', 'zh-cn': '' } }
	]);
	assert.ok(isClassAttribute('description'));
	assert.ok(!isClassAttribute('label'));
});

test('a default is JSON when it parses, otherwise text', () => {
	assert.equal(parseDefault('100'), 100);
	assert.equal(parseDefault('true'), true);
	assert.deepEqual(parseDefault('{"en": "Name"}'), { en: 'Name' });
	assert.equal(parseDefault('"123"'), '123');
	assert.equal(parseDefault(' 200px '), '200px');
	assert.equal(parseDefault('read-write'), 'read-write');
});

test('an edit reads the ID and schema version as text, other values like defaults', () => {
	assert.equal(parseValue('descriptor_id', ' 123 '), '123');
	assert.equal(parseValue('schema_version', '0.2'), '0.2');
	assert.equal(parseValue('description', "The element for 'input'"), "The element for 'input'");
	assert.deepEqual(parseValue('description', '{"en": "A", "zh-cn": "甲"}'), { en: 'A', 'zh-cn': '甲' });
	assert.equal(parseValue('max', '100'), 100);
});

test('an edit starts from text that reads back as the same value', () => {
	for (const attr of [...initialAttributes(), { name: 'width', value: '240px' }, { name: 'n', value: '123' }, { name: 'label', value: null }]) {
		assert.deepEqual(parseValue(attr.name, editText(attr)), attr.value, attr.name);
	}
	assert.equal(editText({ name: 'descriptor_id', value: 'slider' }), 'slider');
	assert.equal(editText({ name: 'width', value: '240px' }), '"240px"');
});

test('an instance attribute cannot be edited to empty', () => {
	assert.equal(editProblem('max', '  '), 'empty_value');
	assert.equal(editProblem('max', 'null'), null);
	assert.equal(editProblem('descriptor_id', ''), null);
});

test('a required attribute is stored as null', () => {
	assert.deepEqual(draftAttribute('label', true, 'ignored'), { name: 'label', value: null });
	assert.deepEqual(draftAttribute('max', false, '100'), { name: 'max', value: 100 });
});

test('attribute problems', () => {
	const added = [...initialAttributes(), { name: 'label', value: null }];
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

test('the class row keeps the attributes in order and drops empty description texts', () => {
	const attrs = initialAttributes();
	attrs[0].value = 'slider';
	attrs[2].value = { en: 'A slider', 'zh-cn': '' };
	attrs.push(draftAttribute('label', true, ''), draftAttribute('max', false, '100'));
	const row = buildClassRow(attrs);
	assert.deepEqual(row, {
		descriptor_id: 'slider',
		schema_version: '0.2',
		description: { en: 'A slider' },
		label: null,
		max: 100
	});
	assert.deepEqual(Object.keys(row), ['descriptor_id', 'schema_version', 'description', 'label', 'max']);
	assert.equal(buildClassRow([{ name: 'description', value: 'Plain' }]).description, 'Plain');
});

test('row problems', () => {
	const row = (id: string, description: unknown) => buildClassRow([{ name: 'descriptor_id', value: id }, { name: 'description', value: description as never }]);
	assert.equal(rowProblem(row('slider', { en: 'A slider' })), null);
	assert.equal(rowProblem(row('slider', 'A slider')), null);
	assert.equal(rowProblem(row('', { en: 'A slider' })), 'class_id');
	assert.equal(rowProblem(row('Slider', { en: 'A slider' })), 'class_id');
	assert.equal(rowProblem(row('slider', { en: '', 'zh-cn': '滑块' })), 'description');
	assert.equal(rowProblem(row('slider', '')), 'description');
});

test('renaming an instance attribute', () => {
	const attrs = [...initialAttributes(), { name: 'label', value: null }, { name: 'max', value: 100 }];
	assert.equal(renameProblem('max', 'max', attrs), null);
	assert.equal(renameProblem('max', 'maximum', attrs), null);
	assert.equal(renameProblem('max', 'label', attrs), 'duplicate');
	assert.equal(renameProblem('max', 'description', attrs), 'reserved');
	assert.equal(renameProblem('max', 'max-value', attrs), 'name');
});
