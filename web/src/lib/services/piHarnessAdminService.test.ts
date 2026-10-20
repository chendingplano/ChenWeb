import { describe, expect, test } from 'bun:test';
import { checkPiGateway, getPiHarnessAdminState, updatePiProfile, type PiProfileCandidate } from './piHarnessAdminService';

describe('Pi harness admin service', () => {
	test('loads only the admin configuration endpoint', async () => {
		let requested = '';
		const fetchFn: typeof fetch = async (input, init) => {
			requested = String(input);
			expect(init?.credentials).toBe('same-origin');
			return Response.json({ gateway_url: 'http://localhost:4317', initialized: true, profiles: [], prompt_choices: {}, harnesses: ['pi'] });
		};
		const result = await getPiHarnessAdminState(fetchFn);
		expect(requested).toBe('/api/v1/system-admin/pi-harness');
		expect(result.initialized).toBe(true);
	});

	test('serializes profile updates to the slug path with expected version', async () => {
		let requested = '';
		let init: RequestInit | undefined;
		const candidate = { slug: 'problem-diagnostics', expected_active_version: 'v4', enabled: false } as PiProfileCandidate;
		const fetchFn: typeof fetch = async (input, options) => {
			requested = String(input);
			init = options;
			return Response.json({ profile: { slug: candidate.slug, version: 'v5' } });
		};
		const result = await updatePiProfile(candidate.slug, candidate, fetchFn);
		expect(requested).toBe('/api/v1/system-admin/pi-harness/profiles/problem-diagnostics');
		expect(init?.method).toBe('PUT');
		expect(JSON.parse(String(init?.body))).toEqual(candidate);
		expect(result.profile.version).toBe('v5');
	});

	test('returns safe gateway metadata and preserves conflict status', async () => {
		const healthy = await checkPiGateway(async () => Response.json({ status: 'healthy', elapsed_ms: 12, checked_at: '2026-10-18T00:00:00Z' }));
		expect(healthy.status).toBe('healthy');
		try {
			await updatePiProfile('knowledge-guide', {} as PiProfileCandidate, async () =>
				new Response(JSON.stringify({ error: 'active profile version changed' }), { status: 409 })
			);
			throw new Error('expected request rejection');
		} catch (error) {
			expect((error as Error & { status?: number }).status).toBe(409);
		}
	});
});
