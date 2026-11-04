import test from 'node:test';
import assert from 'node:assert/strict';
import {
	colorMode,
	moved,
	parseJsonText,
	parseLength,
	parseNumber,
	parseStringList,
	stringListText,
	withColorMode,
	withColorPart,
	withLanguages,
	withTranslation
} from './values';

const all = ['px', 'rem', 'em', '%'];

test('lengths: keywords, units, a bare number is px', () => {
	assert.deepEqual(parseLength('240', [], all), { ok: true, value: '240px' });
	assert.deepEqual(parseLength(' 1.5rem ', [], all), { ok: true, value: '1.5rem' });
	assert.deepEqual(parseLength('auto', ['auto', 'elastic'], all), { ok: true, value: 'auto' });
	assert.deepEqual(parseLength('', [], all), { ok: true, value: undefined });
});

test('lengths refused: unknown text, disallowed unit or keyword, percentage over 100', () => {
	assert.deepEqual(parseLength('12xy', [], all), { ok: false });
	assert.deepEqual(parseLength('50%', [], ['px', 'rem']), { ok: false });
	assert.deepEqual(parseLength('elastic', ['auto'], all), { ok: false });
	assert.deepEqual(parseLength('101%', [], all), { ok: false });
	assert.deepEqual(parseLength('-4px', [], all), { ok: false });
});

test('numbers within limits', () => {
	assert.deepEqual(parseNumber('3', { min: 1, max: 20, integer: true }), { ok: true, value: 3 });
	assert.deepEqual(parseNumber('2.5', { integer: true }), { ok: false });
	assert.deepEqual(parseNumber('21', { max: 20 }), { ok: false });
	assert.deepEqual(parseNumber('', {}), { ok: true, value: undefined });
	assert.deepEqual(parseNumber('x', {}), { ok: false });
});

test('JSON when it parses, otherwise text', () => {
	assert.deepEqual(parseJsonText('{"a":1}'), { ok: true, value: { a: 1 } });
	assert.deepEqual(parseJsonText('hello'), { ok: true, value: 'hello' });
	assert.deepEqual(parseJsonText('  '), { ok: true, value: undefined });
});

test('string lists from comma-separated text', () => {
	assert.deepEqual(parseStringList('a, b,,c '), { ok: true, value: ['a', 'b', 'c'] });
	assert.deepEqual(parseStringList(' , '), { ok: true, value: undefined });
	assert.equal(stringListText(['a', 'b']), 'a, b');
});

test('colour modes keep the chosen colour', () => {
	assert.equal(colorMode(undefined), 'none');
	assert.equal(colorMode('#fff'), 'single');
	assert.equal(colorMode({ light: '#fff', dark: '#000' }), 'theme');
	assert.deepEqual(withColorMode('auto', 'theme'), { light: '#ffffff', dark: '#000000' });
	assert.deepEqual(withColorMode('#123456', 'theme'), { light: '#123456', dark: '#123456' });
	assert.equal(withColorMode({ light: '#aaa', dark: '#000' }, 'single'), '#aaa');
	assert.equal(withColorMode('#aaa', 'none'), undefined);
	assert.deepEqual(withColorPart({ light: '#aaa', dark: '#000' }, 'dark', '#111'), { light: '#aaa', dark: '#111' });
	assert.equal(withColorPart('#aaa', 'single', ''), undefined);
});

test('localized text switches between one text and one per language', () => {
	assert.deepEqual(withLanguages('Name', true), { en: 'Name' });
	assert.equal(withLanguages({ en: 'Name', 'zh-cn': '名称' }, false), 'Name');
	assert.deepEqual(withTranslation({ en: 'Name' }, 'zh-cn', '名称'), { en: 'Name', 'zh-cn': '名称' });
	assert.deepEqual(withTranslation({ en: '', 'zh-cn': '名称' }, 'en', 'Name'), { en: 'Name', 'zh-cn': '名称' });
	assert.deepEqual(withTranslation({ en: 'Name', 'zh-cn': '名称' }, 'zh-cn', ''), { en: 'Name' });
});

test('moving a list item stays within the list', () => {
	assert.deepEqual(moved(['a', 'b', 'c'], 1, -1), ['b', 'a', 'c']);
	assert.deepEqual(moved(['a', 'b'], 1, 1), ['a', 'b']);
});
