import { describe, expect, test } from 'bun:test';
import { convertStringMode, currentStringMode, emptyStringValue, losesTranslations, stringLanguages } from './string-value';

describe('string value conversions', () => {
 test('preserves or unsets empty strings as configured', () => {
  expect(emptyStringValue('', true)).toBeNull();
  expect(emptyStringValue('', false)).toBe('');
  expect(emptyStringValue('  ', true)).toBe('  ');
 });
 test('converts using the current language and preserves existing translations', () => {
  expect(convertStringMode('Hello', 'multilingual', ['en', 'zh-cn'], 'en')).toEqual({en: 'Hello'});
  expect(convertStringMode({'en': 'Hello', 'zh-cn': '你好'}, 'single', ['en', 'zh-cn'], 'zh-cn')).toBe('你好');
  expect(convertStringMode({'fr': 'Salut'}, 'multilingual', ['en', 'zh-cn'], 'en')).toEqual({fr: 'Salut'});
 });
 test('shows a language map as multilingual and other values in the chosen mode', () => {
  expect(currentStringMode({en: 'Hello'}, 'single')).toBe('multilingual');
  expect(currentStringMode('Hello', 'list')).toBe('list');
  expect(currentStringMode(null, 'multilingual')).toBe('single');
  expect(currentStringMode(undefined, undefined)).toBe('single');
  expect(stringLanguages([])).toEqual(['en', 'zh-cn']);
  expect(stringLanguages(['fr'])).toEqual(['fr']);
 });
 test('warns when a conversion drops a translation, including unconfigured languages', () => {
  expect(losesTranslations({fr: 'Salut'}, ['en', 'zh-cn'], 'en')).toBe(true);
  expect(losesTranslations({en: 'Hello'}, ['en', 'zh-cn'], 'en')).toBe(false);
  expect(losesTranslations({en: 'Hello', 'zh-cn': '你好'}, ['en', 'zh-cn'], 'en')).toBe(true);
 });
});
