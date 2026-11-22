import test from 'node:test';
import assert from 'node:assert/strict';

import { compliancePanelDescription } from './compliance-panel-description.js';

// The layout written by the server's compliance reviewer (unitFinding / complianceVerdictRows).
const stored = [
	'Candidate verdicts: 1 not_compliant, 2 unqualified',
	'not_compliant [provision 9_prv_3] record 9: the noise limit is stricter',
	'  Note: checked against table 2',
	'unqualified [provision 9_prv_7] record 9: installation requirement',
	'unqualified [chunk 9_chk_15] record 9: water quality',
	'',
	'Activity a1 [text, lines 49-52]: document: heat pump water heaters',
	'Activity a2 [metric, lines 60-61]: noise level',
	'',
	'Judged 3 candidate artifact(s) from the knowledge base:',
	'- [provision 9_prv_3] (cosine 0.720) record 9 Some standard: noise shall comply',
	'- [chunk 9_chk_15] (cosine 0.684) record 9 Some standard: [240 h2] 6.3 water',
	'[241] passage text that spans',
	'',
	'several lines',
	'- [provision 9_prv_7] (cosine 0.651) record 9 Some standard: installation',
	''
].join('\n');

test('drops the verdict lines and the judged candidate list', () => {
	assert.equal(
		compliancePanelDescription(stored),
		[
			'Candidate verdicts: 1 not_compliant, 2 unqualified',
			'',
			'Activity a1 [text, lines 49-52]: document: heat pump water heaters',
			'Activity a2 [metric, lines 60-61]: noise level',
			'',
			'Judged 3 candidate artifact(s) from the knowledge base:'
		].join('\n')
	);
});

test('leaves other descriptions unchanged', () => {
	const noGrounding =
		'No candidate was left after the knowledge-base search and the relevance check.\n\nActivity a1 [text, lines 1-2]: x\n';
	assert.equal(compliancePanelDescription(noGrounding), noGrounding);
	assert.equal(compliancePanelDescription('A plain finding.'), 'A plain finding.');
	assert.equal(compliancePanelDescription(''), '');
});

test('leaves a verdict description without activity lines unchanged', () => {
	const odd = 'Candidate verdicts: 1 unqualified\nunqualified [provision 9_prv_7] record 9: x\n';
	assert.equal(compliancePanelDescription(odd), odd);
});
