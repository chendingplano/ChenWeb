import { describe, expect, test } from 'bun:test';
import { convertStringMode, currentStringMode, emptyStringValue, warnsAboutTranslations, stringLanguages } from './string-value';

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
 test('switching to Single does not warn, while List still warns about discarded translations', () => {
  expect(warnsAboutTranslations({en: 'Hello', 'zh-cn': '你好'}, ['en', 'zh-cn'], 'en', 'single')).toBe(false);
  expect(warnsAboutTranslations({en: 'Hello', 'zh-cn': '你好'}, ['en', 'zh-cn'], 'en', 'list')).toBe(true);
 });
 test('warns when a conversion drops a translation, including unconfigured languages', () => {
  expect(warnsAboutTranslations({fr: 'Salut'}, ['en', 'zh-cn'], 'en', 'list')).toBe(true);
  expect(warnsAboutTranslations({en: 'Hello'}, ['en', 'zh-cn'], 'en', 'list')).toBe(false);
  expect(warnsAboutTranslations({en: 'Hello', 'zh-cn': '你好'}, ['en', 'zh-cn'], 'en', 'list')).toBe(true);
 });
});
