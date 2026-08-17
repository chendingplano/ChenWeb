import assert from 'node:assert/strict';
import test from 'node:test';
import { detailRows } from './assertion-evidence-detail';

test('Details exposes the evidence provenance fields', () => {
	const rows = detailRows({
		artifact_type: 'metric',
		artifact_object_id: 'obj-42',
		evidence_quote: 'quoted source text',
		source_line_spans: ['12:14'],
		evidence_role: 'supports'
	});

	assert.deepEqual(
		rows.filter((row) => row.value !== null).map((row) => [row.key, row.value]),
		[
			['artifact_type', 'metric'],
			['artifact_object_id', 'obj-42'],
			['evidence_quote', 'quoted source text'],
			['[0]', '12:14'],
			['evidence_role', 'supports']
		]
	);
	assert.ok(rows.some((row) => row.key === 'source_line_spans' && row.value === null));
});
