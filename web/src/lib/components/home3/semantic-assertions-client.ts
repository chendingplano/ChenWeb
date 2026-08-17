export type Assertion = {
	id: number; logical_identity_key: string; revision: number; subject_ref_kind: string;
	subject_ref_id: string; subject_object_id?: string; predicate_term_id: string;
	object_ref_kind?: string; object_ref_id?: string; object_literal?: unknown;
	assertion_kind_term_id?: string; polarity: string; modality?: string; qualifiers?: unknown;
	confidence?: number; value_form?: string; numeric_value?: number; raw_text?: string;
	status: string; decision_reason?: string; dependency_fingerprint?: string;
	superseded_by?: number; create_time: string; modify_time: string;
};
export type AssertionSortKey = 'identity'|'subject'|'predicate'|'status'|'modified';
export type AssertionFilters = { status: string; logical_identity: string; subject_ref_kind: string; subject_ref_id: string; predicate_term_id: string; object_ref_id: string; subject_object_id: string; latest_only: boolean };
async function req<T>(path: string, init?: RequestInit): Promise<T> { const res = await fetch(path, { credentials:'same-origin', ...init }); const text = await res.text(); let body:any=null; try { body=text?JSON.parse(text):null } catch {} if(!res.ok) throw new Error(body?.error_msg||`HTTP ${res.status}`); return body as T; }
export async function listAssertions(filters: AssertionFilters, page=1, pageSize=50, sortBy?: AssertionSortKey, sortDir?: 'asc'|'desc') { const q=new URLSearchParams({page:String(page),page_size:String(pageSize),latest_only:String(filters.latest_only)}); for(const [k,v] of Object.entries(filters)) if(k!=='latest_only' && typeof v === 'string' && v.trim()) q.set(k,v.trim()); if(sortBy) q.set('sort_by',sortBy); if(sortDir) q.set('sort_dir',sortDir); return req<{results:Assertion[];total:number;page:number;page_size:number}>(`/api/v1/kb/semantic-assertions?${q}`); }
export async function getAssertion(id:number){return (await req<{record:Assertion}>(`/api/v1/kb/semantic-assertions/${id}`)).record;}
export async function saveAssertion(input:Partial<Assertion>){return (await req<{record:Assertion}>('/api/v1/kb/semantic-assertions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(input)})).record;}
async function action(id:number,name:string,body:unknown){return (await req<{record:Assertion}>(`/api/v1/kb/semantic-assertions/${id}/${name}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})).record;}
export const transitionAssertion=(id:number,body:{to:string;reason?:string})=>action(id,'transition',body);
export const deferAssertion=(id:number,body:{dependency_fingerprint:string;reason?:string})=>action(id,'defer',body);
export const retryAssertion=(id:number,body:{dependency_fingerprint:string})=>action(id,'retry',body);
