import test from 'node:test';
import assert from 'node:assert/strict';
import { flowValue, validFlowValue, setFlowValue } from './flow-config';

test('flow editor uses the selected whole config and module defaults', () => {
	assert.equal(flowValue('max_num_elems', undefined, {}), 50);
	assert.equal(flowValue('column_gap', undefined, { column_gap: '2rem' }), '2rem');
	assert.equal(flowValue('column_gap', {}, { column_gap: '2rem' }), '0.75rem');
	assert.equal(flowValue('max_num_elems', { max_num_elems: 0 }, {}), 0);
});

test('flow editor validates lengths and integer count and retains other settings', () => {
	assert.equal(validFlowValue('max_num_elems', '0'), true);
	assert.equal(validFlowValue('max_num_elems', '-1'), false);
	assert.equal(validFlowValue('max_num_elems', '1.5'), false);
	assert.equal(validFlowValue('row_gap', '1.5rem'), true);
	assert.equal(validFlowValue('row_gap', '-1px'), false);
	assert.deepEqual(setFlowValue({ column_gap: '1rem', extra: 'preserved' }, 'max_num_elems', '0'), {
		column_gap: '1rem',
		extra: 'preserved',
		max_num_elems: 0
	});
	assert.deepEqual(setFlowValue({ max_num_elems: 3 }, 'max_num_elems', ''), {});
});
