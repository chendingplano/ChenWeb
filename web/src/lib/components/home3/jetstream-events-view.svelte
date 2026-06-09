<script lang="ts">
	import { onMount } from 'svelte';
	import SendIcon from '@lucide/svelte/icons/send';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckBigIcon from '@lucide/svelte/icons/circle-check-big';

	let { darkMode = true }: { darkMode: boolean } = $props();

	let subject = $state('');
	let subjectOptions = $state<string[]>([]);
	let subjectsLoading = $state(false);
	let subjectsError = $state('');
	let payload = $state('');
	let lastSuggestedPayload = $state('');
	let loading = $state(false);
	let error = $state('');
	let success = $state('');
	let lastResponse = $state<any>(null);

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');
	let ok = $derived(darkMode ? '#34D399' : '#059669');
	const requiredPayloadSubjects = new Set([
		'kb.pdf.parsed',
		'kb.pdf.staged',
		'kb.pdf.start-doc-processing',
		'kb.line-file-generated'
	]);
	const payloadExamples: Record<string, string> = {
		'kb.pdf.parsed': '{"record_id":376,"type":"pdf","status":"success","force":true}',
		'kb.pdf.staged': '{"record_id":376,"type":"pdf","status":"success","force":true}',
		'kb.pdf.start-doc-processing':
			'{"record_ids":[376],"doc-processors":["extract_metrics"],"failed-proc-only":true}',
		'kb.line-file-generated': '{"record_id":376}'
	};

	function payloadHelpForSubject(selectedSubject: string): string {
		if (selectedSubject === 'kb.pdf.start-doc-processing') {
			return 'Required: `all` or `record_ids` or `record_id`. Optional: `doc-processors`, `failed-proc-only`. `all` may be `parsed`, `failed-procs`, or `with-failed-procs`.';
		}
		if (selectedSubject === 'kb.pdf.parsed' || selectedSubject === 'kb.pdf.staged') {
			return 'Required: `record_id`, `type`, `status`. Optional: `force`.';
		}
		if (selectedSubject === 'kb.line-file-generated') {
			return 'Required: `record_id`.';
		}
		return '';
	}

	function exampleForSubject(selectedSubject: string): string {
		return payloadExamples[selectedSubject] ?? '';
	}

	function suggestPayloadForSubject(selectedSubject: string, force = false) {
		const example = exampleForSubject(selectedSubject);
		if (!example) {
			return;
		}
		if (force || payload.trim() === '' || payload === lastSuggestedPayload) {
			payload = example;
			lastSuggestedPayload = example;
		}
	}

	function validatePayloadForSubject(selectedSubject: string, rawPayload: string): string | null {
		if (!requiredPayloadSubjects.has(selectedSubject)) {
			return null;
		}
		const body = rawPayload.trim();
		if (!body) {
			return `Payload is required for subject ${selectedSubject}`;
		}
		let parsed: Record<string, unknown>;
		try {
			parsed = JSON.parse(body) as Record<string, unknown>;
		} catch {
			return `Payload must be valid JSON for subject ${selectedSubject}`;
		}
		if (selectedSubject === 'kb.pdf.start-doc-processing') {
			const hasAll = typeof parsed.all === 'string' && parsed.all.trim() !== '';
			const hasRecordIDs = Array.isArray(parsed.record_ids) && parsed.record_ids.length > 0;
			const hasRecordID = parsed.record_id != null;
			if (!hasAll && !hasRecordIDs && !hasRecordID) {
				return `Payload field 'all', 'record_ids', or 'record_id' is required for subject ${selectedSubject}`;
			}
			if (
				hasAll &&
				!['parsed', 'failed-procs', 'with-failed-procs'].includes(String(parsed.all).trim())
			) {
				return `Payload field 'all' must be parsed, failed-procs, or with-failed-procs`;
			}
			return null;
		}
		if (selectedSubject === 'kb.line-file-generated') {
			if (!('record_id' in parsed) || parsed.record_id == null) {
				return `Payload field 'record_id' is required for subject ${selectedSubject}`;
			}
			return null;
		}
		for (const key of ['record_id', 'type', 'status']) {
			if (!(key in parsed) || parsed[key] == null) {
				return `Payload field '${key}' is required for subject ${selectedSubject}`;
			}
		}
		if (typeof parsed.type !== 'string' || parsed.type.trim() === '') {
			return `Payload field 'type' must be a non-empty string for subject ${selectedSubject}`;
		}
		if (typeof parsed.status !== 'string' || parsed.status.trim() === '') {
			return `Payload field 'status' must be a non-empty string for subject ${selectedSubject}`;
		}
		if ('force' in parsed && parsed.force != null && typeof parsed.force !== 'boolean') {
			return `Payload field 'force' must be boolean for subject ${selectedSubject}`;
		}
		return null;
	}

	async function loadSubjects() {
		subjectsLoading = true;
		subjectsError = '';
		try {
			const res = await fetch('/api/v1/jetstream/nats-subjects', {
				credentials: 'same-origin'
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok || !data.ok) {
				throw new Error(data.message ?? `Failed to load subjects (${res.status})`);
			}
			const rawSubjects: unknown = data.subjects;
			const next = Array.isArray(rawSubjects)
				? rawSubjects
						.filter(
							(v: unknown): v is { subject?: string; is_active?: boolean } =>
								typeof v === 'object' && v !== null
						)
						.filter((v) => v.is_active === true)
						.map((v) => (typeof v.subject === 'string' ? v.subject.trim() : ''))
						.filter((v) => v.length > 0)
				: [];
			subjectOptions = next;
			if (next.length > 0 && !next.includes(subject)) {
				subject = next[0];
				suggestPayloadForSubject(subject);
			}
		} catch (err) {
			subjectOptions = [];
			subject = '';
			subjectsError = err instanceof Error ? err.message : String(err);
		} finally {
			subjectsLoading = false;
		}
	}

	async function publishEvent() {
		if (!subject.trim()) {
			error = 'Please select a subject';
			return;
		}
		const validationError = validatePayloadForSubject(subject.trim(), payload);
		if (validationError) {
			error = validationError;
			return;
		}

		loading = true;
		error = '';
		success = '';
		lastResponse = null;

		try {
			const res = await fetch('/api/v1/jetstream/events', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'same-origin',
				body: JSON.stringify({
					subject: subject.trim(),
					payload
				})
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok || !data.ok) {
				throw new Error(data.message ?? `Publish failed (${res.status})`);
			}
			success = data.message ?? 'Event published';
			lastResponse = data;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadSubjects();
	});
</script>

<div class="space-y-4 p-6">
	<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
		<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">JetStream Events</h2>
		<p style="font-size:13px; color:{textSecondary}; margin-top:4px;">
			Inject an event into JetStream by subject and payload.
		</p>

		<div class="mt-4 grid gap-3">
			<label class="grid gap-1.5">
				<div class="flex items-center justify-between gap-2">
					<span style="font-size:12px; color:{textSecondary};">Subject</span>
					<button
						type="button"
						onclick={loadSubjects}
						disabled={subjectsLoading}
						class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1"
						style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary}; font-size:12px;"
					>
						<RefreshCwIcon class="h-3.5 w-3.5" />
						Refresh
					</button>
				</div>
				<select
					bind:value={subject}
					onchange={(event) =>
						suggestPayloadForSubject((event.currentTarget as HTMLSelectElement).value)}
					style="height:36px; border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:0 10px;"
					disabled={subjectsLoading || subjectOptions.length === 0}
				>
					{#if subjectOptions.length === 0}
						<option value=""
							>{subjectsLoading ? 'Loading subjects...' : 'No subjects available'}</option
						>
					{:else}
						{#each subjectOptions as s}
							<option value={s}>{s}</option>
						{/each}
					{/if}
				</select>
				{#if subjectsError}
					<div style="font-size:12px; color:{danger};">{subjectsError}</div>
				{/if}
			</label>

			<label class="grid gap-1.5">
				<span style="font-size:12px; color:{textSecondary};">Payload (raw string)</span>
				{#if payloadHelpForSubject(subject)}
					<div style="font-size:12px; color:{textSecondary};">
						{payloadHelpForSubject(subject)}
						<button
							type="button"
							onclick={() => suggestPayloadForSubject(subject, true)}
							class="ml-2 inline-flex cursor-pointer items-center rounded-md px-2 py-0.5"
							style="background:{surface2}; border:1px solid {borderColor}; color:{accent}; font-size:12px;"
						>
							Use example
						</button>
					</div>
				{/if}
				<textarea
					bind:value={payload}
					rows="10"
					style="border:1px solid {borderColor}; background:{surface2}; color:{textPrimary}; border-radius:8px; padding:10px; resize:vertical;"
				></textarea>
			</label>

			<div>
				<button
					onclick={publishEvent}
					disabled={loading || subjectOptions.length === 0}
					class="inline-flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2"
					style="background:{accent}; color:white; border:none;"
				>
					<SendIcon class="h-4 w-4" />
					{loading ? 'Publishing...' : 'Publish Event'}
				</button>
			</div>
		</div>
	</div>

	{#if error}
		<div
			class="flex items-start gap-2 rounded-xl p-4"
			style="background:{danger}20; border:1px solid {danger}70; color:{danger};"
		>
			<CircleAlertIcon class="mt-0.5 h-4 w-4" />
			<div>
				<div style="font-weight:600;">Failed to publish event</div>
				<div style="font-size:13px;">{error}</div>
			</div>
		</div>
	{/if}

	{#if success}
		<div
			class="flex items-start gap-2 rounded-xl p-4"
			style="background:{ok}20; border:1px solid {ok}70; color:{ok};"
		>
			<CircleCheckBigIcon class="mt-0.5 h-4 w-4" />
			<div>
				<div style="font-weight:600;">{success}</div>
				{#if lastResponse}
					<pre
						class="mt-2 overflow-auto rounded-lg p-3"
						style="max-height:320px; background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:12px;">{JSON.stringify(
							lastResponse,
							null,
							2
						)}</pre>
				{/if}
			</div>
		</div>
	{/if}
</div>
