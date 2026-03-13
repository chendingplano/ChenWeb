// web/src/lib/services/flowService.ts
import type { Flow, NodeType } from '$lib/types/flow';

const BASE = '/api/v1';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { code: 'UNKNOWN', message: res.statusText } }));
    throw err;
  }
  return res.json();
}

export type FlowScope = 'mine' | 'shared' | 'templates';

export const flowService = {
  list: (scope: FlowScope): Promise<{ flows: Flow[] }> =>
    request('GET', `/flows?scope=${scope}`),

  create: (payload: Pick<Flow, 'flow_name' | 'flow_data' | 'is_shared'> & { thumbnail_svg?: string | null }): Promise<{ flow: Flow }> =>
    request('POST', '/flows', payload),

  getDefault: (): Promise<{ flow: Flow }> =>
    request('GET', '/flows/default'),

  get: (id: number): Promise<{ flow: Flow }> =>
    request('GET', `/flows/${id}`),

  update: (id: number, payload: Partial<Pick<Flow, 'flow_name' | 'flow_desc' | 'flow_data' | 'is_shared' | 'thumbnail_svg'>>): Promise<{ flow: Flow }> =>
    request('PUT', `/flows/${id}`, payload),

  delete: (id: number): Promise<void> =>
    request('DELETE', `/flows/${id}`),

  setDefault: (id: number): Promise<void> =>
    request('PUT', `/flows/${id}/default`),

  fork: (id: number): Promise<{ flow: Flow }> =>
    request('POST', `/flows/${id}/fork`),

  saveAsTemplate: (id: number): Promise<{ flow: Flow }> =>
    request('POST', `/flows/${id}/template`),

  getNodeTypes: (): Promise<{ nodeTypes: NodeType[] }> =>
    request('GET', '/flow-node-types'),
};
