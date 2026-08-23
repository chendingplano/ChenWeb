import { describe, expect, test } from 'bun:test';
import { validateRewriteDraft } from './keyword-rewrite-rules-client';

const base = { rule_id: 'r1', pattern: 'old', replacement: 'new', scope: '_', provenance: 'human:' };

describe('keyword rewrite rule draft validation', () => {
 test('requires nonblank editable fields without trimming literal values', () => {
  expect(validateRewriteDraft({ ...base, pattern: '   ' }, false)).toContain('required');
  expect(validateRewriteDraft({ ...base, pattern: ' old ' }, false)).toBeNull();
 });
 test('rejects regex-like pattern characters', () => {
  expect(validateRewriteDraft({ ...base, pattern: '(old)' }, false)).toContain('parentheses');
  expect(validateRewriteDraft({ ...base, pattern: 'old\\d' }, false)).toContain('backslashes');
 });
 test('requires identity for new rules', () => {
  expect(validateRewriteDraft({ ...base, rule_id: ' ' }, false)).toContain('required');
 });
});
