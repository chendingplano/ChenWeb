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
