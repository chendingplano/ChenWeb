/**
 * @param {import('$lib/services/kbMetricSearch').KbMetricSearchResult | null | undefined} result
 * @returns {string}
 */
export function metricSearchResultSecondaryText(result) {
	return (
		result?.snippet?.trim() ||
		result?.metric_subject?.trim() ||
		result?.metric_name_en?.trim() ||
		''
	);
}

/**
 * @param {import('$lib/services/kbMetricSearch').KbMetricSearchResult | null | undefined} result
 * @returns {string[]}
 */
export function metricSearchResultChips(result) {
	const chips = [];
	if (result?.metric_unit) chips.push(result.metric_unit);
	if (result?.value_class) chips.push(result.value_class);
	if (result?.value_data_type) chips.push(result.value_data_type);
	if (result?.table_name_or_section) chips.push(result.table_name_or_section);
	return chips;
}
