import { expect, test } from 'bun:test';
import type { Json } from '$lib/wpi/types';
import { toggleField, renameField } from './field-mappings';

test('selecting columns defaults the attribute to the column name and preserves other mappings', () => {
 const fields = [{ column: 'id', attribute: 'identifier', required: true }];
 expect(toggleField(fields, 'name', true)).toEqual([...fields, { column: 'name', attribute: 'name' }]);
 expect(toggleField(fields, 'id', false)).toEqual([]);
 expect(renameField(fields, 'id', 'key')).toEqual([{ column: 'id', attribute: 'key', required: true }]);
});

test('renaming a column retains the mapping and does not alter other fields', async () => {
 const { renameColumn } = await import('./field-mappings');
 const fields: Json[] = [{ column: 'id', attribute: 'identifier', required: true }, { column: 'name', attribute: 'name' }];
 expect(renameColumn(fields, 'id', 'person_id')).toEqual([{ column: 'person_id', attribute: 'identifier', required: true }, fields[1]]);
 expect(renameColumn(fields, 'id', 'name')).toEqual(fields);
 expect(renameColumn(fields, 'id', '')).toEqual(fields);
});
