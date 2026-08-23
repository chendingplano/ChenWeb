export type RewriteRule = {
	rule_id: string;
	pattern: string;
	replacement: string;
	scope: string;
	enabled: boolean;
	provenance: string;
	create_time: string;
	modify_time: string;
};

type Envelope<T> = { status: boolean; results?: T; record?: T; total?: number; error_msg?: string };
async function request<T>(url: string, init?: RequestInit): Promise<T> {
	const response = await fetch(url, { credentials: 'same-origin', ...init });
	const body = await response.text();
	let parsed: Envelope<T> | null = null;
	try { parsed = body ? (JSON.parse(body) as Envelope<T>) : null; } catch { /* normalized below */ }
	if (!response.ok) throw new Error(parsed?.error_msg || `Request failed (${response.status})`);
	return parsed as T;
}
export async function listRewriteRules(scope = ''): Promise<RewriteRule[]> {
	const result = await request<Envelope<RewriteRule[]>>(`/api/v1/kb/keyword-rewrite-rules/admin${scope ? `?scope=${encodeURIComponent(scope)}` : ''}`);
	return result.results ?? [];
}
export async function listRewriteScopes(): Promise<string[]> {
	const result = await request<Envelope<string[]>>('/api/v1/kb/keyword-rewrite-rules/admin/scopes');
	return result.results ?? ['_'];
}
export type RewriteRuleInput = Omit<RewriteRule, 'create_time' | 'modify_time'>;
export function createRewriteRule(input: Partial<RewriteRuleInput>) {
	return request<Envelope<RewriteRule>>('/api/v1/kb/keyword-rewrite-rules', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) });
}
export function updateRewriteRule(ruleID: string, input: Omit<RewriteRuleInput, 'rule_id'>) {
	return request<Envelope<RewriteRule>>(`/api/v1/kb/keyword-rewrite-rules/${encodeURIComponent(ruleID)}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) });
}
export function toggleRewriteRule(ruleID: string, enabled: boolean) {
	return request<Envelope<RewriteRule>>(`/api/v1/kb/keyword-rewrite-rules/${encodeURIComponent(ruleID)}/enabled`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled }) });
}

export function validateRewriteDraft(draft: { rule_id: string; pattern: string; replacement: string; scope: string; provenance: string }, editing: boolean): string | null {
	if (!draft.rule_id.trim() || !draft.pattern.trim() || !draft.replacement.trim() || !draft.scope.trim() || !draft.provenance.trim()) return 'Rule ID, pattern, replacement, scope, and provenance are required.';
	if (draft.pattern.includes('(') || draft.pattern.includes(')') || draft.pattern.includes('\\')) return 'Pattern cannot contain parentheses or backslashes.';
	if (!editing && !draft.rule_id.trim()) return 'Rule ID is required.';
	return null;
}
