import { describe, test, expect } from 'bun:test';
import { kbInputRecordKey, metricBenchmarkSelection } from './metric-benchmark-record';

const opus = { id: 753, gold_metric_runs: [{ skill_version: '6.0.0', model_name: 'claude-opus-5-5', run_id: 'run-one' }] };
const tuna = { id: 753, gold_metric_runs: [{ skill_version: '6.0.0', model_name: 'gpt-6-tuna', run_id: 'run-two' }] };

describe('benchmark record selection', () => {
	test('distinguishes repeated runs of the same input, version and model', () => {
		const repeated = { ...opus, gold_metric_runs: [{ ...opus.gold_metric_runs[0], run_id: 'run-later' }] };
		expect(kbInputRecordKey(opus)).not.toBe(kbInputRecordKey(repeated));
		expect(metricBenchmarkSelection(repeated, '6.0.0', '').runId).toBe('run-later');
	});
	test('distinguishes two model benchmarks of the same input', () => {
		expect(kbInputRecordKey(opus)).not.toBe(kbInputRecordKey(tuna));
		expect(kbInputRecordKey({ id: 753 })).toBe('753');
	});
	test('loads the clicked benchmark even while the filter shows all models or a different model', () => {
		expect(metricBenchmarkSelection(opus, '6.0.0', '')).toEqual({ skillVersion: '6.0.0', modelName: 'claude-opus-5-5', runId: 'run-one' });
		expect(metricBenchmarkSelection(tuna, '5.0.0', 'claude-opus-5-5')).toEqual({ skillVersion: '6.0.0', modelName: 'gpt-6-tuna', runId: 'run-two' });
	});
	test('uses the filters when there is no selected benchmark entry', () => {
		expect(metricBenchmarkSelection(null, '6.0.0', 'gpt-6-tuna')).toEqual({ skillVersion: '6.0.0', modelName: 'gpt-6-tuna', runId: undefined });
	});
});
