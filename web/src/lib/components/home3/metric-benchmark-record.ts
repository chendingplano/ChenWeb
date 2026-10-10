import type { KbInputRecord } from '$lib/services/kbService';

type BenchmarkRecord = Pick<KbInputRecord, 'id' | 'gold_metric_runs'>;

export function kbInputRecordKey(record: BenchmarkRecord): string {
	const run = record.gold_metric_runs?.length === 1 ? record.gold_metric_runs[0] : null;
	return run ? JSON.stringify([record.id, run.run_id, run.skill_version, run.model_name]) : String(record.id);
}

export function metricBenchmarkSelection(record: BenchmarkRecord | null, skillVersion: string, modelName: string) {
	const run = record?.gold_metric_runs?.length === 1 ? record.gold_metric_runs[0] : null;
	return { skillVersion: run?.skill_version ?? skillVersion, modelName: run?.model_name ?? modelName, runId: run?.run_id };
}
