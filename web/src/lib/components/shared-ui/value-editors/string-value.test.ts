import { describe, expect, test } from 'bun:test';
import { convertStringMode, emptyStringValue, losesTranslations } from './string-value';

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
 test('warns when a conversion drops a translation, including unconfigured languages', () => {
  expect(losesTranslations({fr: 'Salut'}, ['en', 'zh-cn'], 'en')).toBe(true);
  expect(losesTranslations({en: 'Hello'}, ['en', 'zh-cn'], 'en')).toBe(false);
  expect(losesTranslations({en: 'Hello', 'zh-cn': '你好'}, ['en', 'zh-cn'], 'en')).toBe(true);
 });
});
