import { metricWikiCopyForLang } from './metric-wiki-i18n';

export type ArtifactRecordGroupRow = {
	label: string;
	value: string;
};

export type ArtifactRecordGroup = {
	title: string;
	rows: ArtifactRecordGroupRow[];
};

function stringifyValue(value: unknown): string {
	if (value == null) return '';
	if (typeof value === 'string') return value.trim();
	if (typeof value === 'number' || typeof value === 'boolean') return String(value);
	try {
		return JSON.stringify(value, null, 2);
	} catch {
		return String(value);
	}
}

function rowsFromRecord(record: Record<string, unknown>, keys: string[]): ArtifactRecordGroupRow[] {
	return keys
		.map((key) => ({ label: key, value: stringifyValue(record[key]) }))
		.filter((row) => row.value !== '');
}

export function buildArtifactRecordGroups(
	artifactType: string,
	record: Record<string, unknown>,
	lang?: string
): ArtifactRecordGroup[] {
	const copy = metricWikiCopyForLang(lang);
	if (artifactType !== 'metric') {
		return [
			{
				title: copy.record,
				rows: Object.keys(record)
					.sort()
					.map((key) => ({ label: key, value: stringifyValue(record[key]) }))
					.filter((row) => row.value !== '')
			}
		];
	}

	const jsonFieldKeys = ['source_line_spans', 'metric_keywords', 'metric_keywords_en', 'reasoning_tags'];
	const coreKeys = Object.keys(record)
		.filter((key) => !jsonFieldKeys.includes(key))
		.sort();

	return [
		{ title: copy.coreFields, rows: rowsFromRecord(record, coreKeys) },
		{ title: copy.jsonFields, rows: rowsFromRecord(record, jsonFieldKeys) }
	].filter((group) => group.rows.length > 0);
}
