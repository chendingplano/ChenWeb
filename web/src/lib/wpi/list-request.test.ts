import { expect, test } from 'bun:test';
import { hasListSource, requestWithTimeout, RequestTimeoutError } from './list-request';

test('unconfigured lists do not have a source to query', () => {
 expect(hasListSource(undefined)).toBe(false);
 expect(hasListSource({ action: 'load' })).toBe(false);
 expect(hasListSource({ action: 'load', table: '  ' })).toBe(false);
 expect(hasListSource({ action: 'load', table: 'kb.people' })).toBe(true);
 expect(hasListSource({ action: 'query' })).toBe(true);
});
test('slow requests time out and abort', async () => {
 let aborted = false;
 const request = requestWithTimeout((signal) => new Promise<never>(() => { signal.addEventListener('abort', () => { aborted = true; }); }), 10);
 await expect(request).rejects.toBeInstanceOf(RequestTimeoutError);
 expect(aborted).toBe(true);
});
test('completed requests return their value', async () => {
 expect(await requestWithTimeout(async () => 42, 100)).toBe(42);
});

test('a configured list on a page without a valid ID cannot run a stored action', async () => {
 const { listQueryAvailability } = await import('./list-request');
 expect(listQueryAvailability({ action: 'load', table: 'kb.people' }, '')).toBe('unsaved');
 expect(listQueryAvailability({ action: 'load', table: 'kb.people' }, 'Bad ID')).toBe('unsaved');
 expect(listQueryAvailability({ action: 'load', table: 'kb.people' }, 'customers')).toBe('ready');
 expect(listQueryAvailability(undefined, '')).toBe('unconfigured');
});

test('editor preview uses selected fields for columns and can load an unnamed page', async () => {
 const { listQueryAvailability, previewColumns } = await import('./list-request');
 const source = { action: 'load', table: 'shared.kb.people', result: 'list', fields: [{ column: 'user_name', attribute: 'user_name' }, { column: 'email', attribute: 'email' }] };
 expect(listQueryAvailability(source, '', true)).toBe('ready');
 expect(listQueryAvailability({ action: 'load', table: 'kb.people' }, '', true)).toBe('fields');
 expect(previewColumns([], source)).toEqual([{ field: 'user_name', label: 'User name' }, { field: 'email', label: 'Email' }]);
 expect(previewColumns([{field:'email',label:'Contact'}], source)).toEqual([{field:'email',label:'Contact'}]);
});

test('a table and fields are enough to preview a list when action and result are unset', async () => {
 const { listQueryAvailability, previewLoadSource } = await import('./list-request');
 const source = { table: 'miner.kb.people', fields: [{ column: 'id', attribute: 'id' }] };
 expect(listQueryAvailability(source, '', true)).toBe('ready');
 expect(previewLoadSource(source)).toEqual({ ...source, action: 'load', result: 'list' });
 expect(previewLoadSource({ ...source, action: 'query' })).toBeNull();
});
