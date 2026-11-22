const ACTIVITY_LINE = /^Activity \S+ \[[^\],]+, lines \d+-\d+\]: /;
const JUDGED_HEADING = /^Judged \d+ candidate artifact\(s\) from the knowledge base:$/;

// Shortens a compliance assessment's description for the Finding panel. It keeps the verdict
// counts, the activity lines and the "Judged N candidate artifact(s)" heading, and drops the
// per-candidate verdict lines and the candidate list under the heading: the panel's Candidates
// row shows each candidate with its verdict and summary. The stored description keeps them all,
// because compliance-score reads the candidate list from it. Any other text is returned as is.
export function compliancePanelDescription(description: string): string {
	if (!description.startsWith('Candidate verdicts:')) return description;
	const lines = description.split('\n');
	const first = lines.findIndex((l) => ACTIVITY_LINE.test(l));
	if (first < 0) return description;
	const out = [lines[0], ''];
	for (const line of lines.slice(first)) {
		out.push(line);
		if (JUDGED_HEADING.test(line)) break;
	}
	return out.join('\n').trimEnd();
}
