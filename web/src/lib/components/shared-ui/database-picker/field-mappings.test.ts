import { expect, test } from 'bun:test';
import { toggleField, renameField } from './field-mappings';

test('selecting columns defaults the attribute to the column name and preserves other mappings', () => {
 const fields = [{ column: 'id', attribute: 'identifier', required: true }];
 expect(toggleField(fields, 'name', true)).toEqual([...fields, { column: 'name', attribute: 'name' }]);
 expect(toggleField(fields, 'id', false)).toEqual([]);
 expect(renameField(fields, 'id', 'key')).toEqual([{ column: 'id', attribute: 'key', required: true }]);
});
