import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createSchedule,
	deleteSchedule,
	formatInterval,
	listJobTypes,
	listScheduleRuns,
	listSchedules,
	progressToNextRun,
	updateSchedule,
	type Schedule
} from './schedules-client.js';

type FetchCall = { input: string | URL | Request; init?: RequestInit };

function installFetchMock(handler: (call: FetchCall) => Promise<Response>) {
	const originalFetch = globalThis.fetch;
	const calls: FetchCall[] = [];
	globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
		const call = { input, init };
		calls.push(call);
		return handler(call);
	}) as typeof fetch;
	return {
		calls,
		restore() {
			globalThis.fetch = originalFetch;
		}
	};
}

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

test('listJobTypes returns the job_types array', async () => {
	const mock = installFetchMock(async () =>
		jsonResponse({ status: true, job_types: [{ job_type: 'resolve_entity_objects', label: 'Resolve Entity Objects' }] })
	);
	try {
		const jobTypes = await listJobTypes();
		assert.equal(jobTypes.length, 1);
		assert.equal(jobTypes[0].job_type, 'resolve_entity_objects');
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/schedule-job-types');
	} finally {
		mock.restore();
	}
});

test('listSchedules returns the schedules array', async () => {
	const mock = installFetchMock(async () => jsonResponse({ status: true, schedules: [] }));
	try {
		const schedules = await listSchedules();
		assert.deepEqual(schedules, []);
	} finally {
		mock.restore();
	}
});

test('createSchedule POSTs the input and returns the created schedule', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(call.init?.method, 'POST');
		const body = JSON.parse(String(call.init?.body));
		assert.equal(body.name, 'Nightly Entity Resolve');
		return jsonResponse({ status: true, schedule: { id: 1, ...body, next_run_at: '2026-07-23T00:00:00Z' } });
	});
	try {
		const schedule = await createSchedule({
			name: 'Nightly Entity Resolve',
			job_type: 'resolve_entity_objects',
			interval_seconds: 3600,
			params: { limit: 200 },
			enabled: true
		});
		assert.equal(schedule.id, 1);
	} finally {
		mock.restore();
	}
});

test('updateSchedule PATCHes the given id', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(call.init?.method, 'PATCH');
		assert.ok(String(call.input).endsWith('/kb/schedules/5'));
		return jsonResponse({ status: true });
	});
	try {
		await updateSchedule(5, { name: 'x', interval_seconds: 60, params: {}, enabled: false });
	} finally {
		mock.restore();
	}
});

test('deleteSchedule DELETEs the given id', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(call.init?.method, 'DELETE');
		assert.ok(String(call.input).endsWith('/kb/schedules/5'));
		return jsonResponse({ status: true });
	});
	try {
		await deleteSchedule(5);
	} finally {
		mock.restore();
	}
});

test('listScheduleRuns returns the runs array with the given limit', async () => {
	const mock = installFetchMock(async (call) => {
		assert.ok(String(call.input).includes('limit=10'));
		return jsonResponse({ status: true, runs: [] });
	});
	try {
		await listScheduleRuns(1, 10);
	} finally {
		mock.restore();
	}
});

test('a non-ok response throws with the server error_msg', async () => {
	const mock = installFetchMock(async () => jsonResponse({ status: false, error_msg: 'unknown job_type' }, 400));
	try {
		await assert.rejects(() => listSchedules(), /unknown job_type/);
	} finally {
		mock.restore();
	}
});

test('formatInterval renders whole-unit intervals in the largest fitting unit', () => {
	assert.equal(formatInterval(60), 'every 1 minute');
	assert.equal(formatInterval(120), 'every 2 minutes');
	assert.equal(formatInterval(3600), 'every 1 hour');
	assert.equal(formatInterval(7200), 'every 2 hours');
	assert.equal(formatInterval(86400), 'every 1 day');
	assert.equal(formatInterval(45), 'every 45s');
});

test('progressToNextRun reads 1 (fully due) when a schedule has never run', () => {
	const sched = { interval_seconds: 60 } as Schedule;
	assert.equal(progressToNextRun(sched, new Date()), 1);
});

test('progressToNextRun is 0 right after a run and approaches 1 as the interval elapses', () => {
	const now = new Date('2026-07-23T12:00:00Z');
	const sched = { interval_seconds: 100, last_run_at: '2026-07-23T12:00:00Z' } as Schedule;
	assert.equal(progressToNextRun(sched, now), 0);

	const halfway = new Date('2026-07-23T12:00:50Z');
	assert.equal(progressToNextRun(sched, halfway), 0.5);

	const overdue = new Date('2026-07-23T12:05:00Z');
	assert.equal(progressToNextRun(sched, overdue), 1);
});
