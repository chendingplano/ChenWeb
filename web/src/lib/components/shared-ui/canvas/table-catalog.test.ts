import { expect, test } from 'bun:test';
import { catalogSchemas, catalogTables, schemaForTable } from './table-catalog';

const tables = [{ schema: 'kb', table: 'people' }, { schema: 'public', table: 'people' }, { schema: 'kb', table: 'docs' }];
test('catalog groups tables by schema and retains qualified references', () => {
 expect(catalogSchemas(tables)).toEqual(['kb', 'public']);
 expect(catalogTables(tables, 'kb')).toEqual(['docs', 'people']);
 expect(schemaForTable('public.people', tables)).toBe('public');
 expect(schemaForTable('missing.old', tables)).toBe('missing');
 expect(schemaForTable('people', tables)).toBe('public');
});
