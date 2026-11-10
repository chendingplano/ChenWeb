import { describe, expect, test } from 'bun:test';
import { parseRoles } from './doc-groups-client';

describe('parseRoles', () => {
	test('trims, lower-cases and de-duplicates', () => {
		expect(parseRoles(' Admin, dev ,ADMIN,,')).toEqual(['admin', 'dev']);
		expect(parseRoles('')).toEqual([]);
	});
});
