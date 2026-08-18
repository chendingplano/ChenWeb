export type StateSeverity = 'good' | 'warn' | 'bad' | 'neutral';

// Labels/severity for the governed term IDs that can appear in
// Assertion.class_identity_state_term_id, .mapping_resolution_state_term_id,
// .value_state_term_id, and .conformance_state_term_id (server/api/ontology/semantic/terms.go).
const STATE_LABELS: Record<string, string> = {
	'semantic:resolved_existing': 'Resolved (existing class)',
	'semantic:provisional_new': 'Provisional (new class)',
	'semantic:ambiguous_candidates': 'Ambiguous candidates',
	'semantic:candidate_evidence_conflict': 'Candidate/evidence conflict',
	'semantic:mapping_resolved': 'Resolved',
	'semantic:mapping_state_unresolved': 'Unresolved',
	'semantic:mapping_state_ambiguous': 'Ambiguous',
	'semantic:mapping_not_required': 'Not required',
	'semantic:value_present': 'Present',
	'semantic:value_state_missing': 'Missing',
	'semantic:value_state_unparsed': 'Unparsed',
	'semantic:value_state_datatype_mismatch': 'Datatype mismatch',
	'semantic:value_state_unknown': 'Unknown',
	'semantic:value_state_not_applicable': 'Not applicable',
	'semantic:conforms': 'Conforms',
	'semantic:conformance_contract_violation': 'Contract violation',
	'semantic:not_evaluated': 'Not evaluated'
};

const STATE_SEVERITY: Record<string, StateSeverity> = {
	'semantic:resolved_existing': 'good',
	'semantic:provisional_new': 'warn',
	'semantic:ambiguous_candidates': 'bad',
	'semantic:candidate_evidence_conflict': 'bad',
	'semantic:mapping_resolved': 'good',
	'semantic:mapping_state_unresolved': 'bad',
	'semantic:mapping_state_ambiguous': 'warn',
	'semantic:mapping_not_required': 'neutral',
	'semantic:value_present': 'good',
	'semantic:value_state_missing': 'warn',
	'semantic:value_state_unparsed': 'warn',
	'semantic:value_state_datatype_mismatch': 'bad',
	'semantic:value_state_unknown': 'warn',
	'semantic:value_state_not_applicable': 'neutral',
	'semantic:conforms': 'good',
	'semantic:conformance_contract_violation': 'bad',
	'semantic:not_evaluated': 'neutral'
};

// Assertion lifecycle statuses (server/api/ontology/assertions/state_machine.go),
// including represented/unsupported from ADR 2026081801 DR7.
const STATUS_SEVERITY: Record<string, StateSeverity> = {
	accepted: 'good',
	represented: 'neutral',
	candidate: 'neutral',
	in_review: 'neutral',
	deferred: 'warn',
	unsupported: 'bad',
	rejected: 'bad',
	superseded: 'neutral'
};

function prettify(value: string): string {
	return value
		.split('_')
		.filter(Boolean)
		.map((word) => word[0].toUpperCase() + word.slice(1))
		.join(' ');
}

// Human label for a governed semantic:* state term ID. Falls back to a
// prettified form of any unrecognized term rather than showing the raw
// underscore identifier (which DR9 reserves for machine consumers only).
export function stateLabel(termId: string | undefined | null): string {
	if (!termId) return '—';
	if (STATE_LABELS[termId]) return STATE_LABELS[termId];
	const stripped = termId.startsWith('semantic:') ? termId.slice('semantic:'.length) : termId;
	return prettify(stripped);
}

export function stateSeverity(termId: string | undefined | null): StateSeverity {
	if (!termId) return 'neutral';
	return STATE_SEVERITY[termId] ?? 'neutral';
}

export function statusLabel(status: string | undefined | null): string {
	if (!status) return '—';
	return prettify(status);
}

export function statusSeverity(status: string | undefined | null): StateSeverity {
	if (!status) return 'neutral';
	return STATUS_SEVERITY[status] ?? 'neutral';
}

export const SEVERITY_COLORS: Record<StateSeverity, { bg: string; fg: string }> = {
	good: { bg: 'rgba(34,197,94,0.15)', fg: '#22c55e' },
	warn: { bg: 'rgba(245,158,11,0.15)', fg: '#f59e0b' },
	bad: { bg: 'rgba(239,68,68,0.15)', fg: '#ef4444' },
	neutral: { bg: 'rgba(148,163,184,0.15)', fg: '#94a3b8' }
};
