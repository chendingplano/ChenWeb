<script lang="ts">
	import AlertTriangleIcon from '@lucide/svelte/icons/alert-triangle';
	import CheckCircleIcon from '@lucide/svelte/icons/check-circle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint = $derived(darkMode ? 'rgba(248,113,113,0.10)' : 'rgba(220,38,38,0.08)');
	let ok = $derived(darkMode ? '#34D399' : '#059669');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let inputBg = $derived(darkMode ? '#141824' : '#FFFFFF');

	type CleanResult = {
		status: boolean;
		record_id: number;
		record_found: boolean;
		artifact_web_cleanup?: {
			files_scanned: number;
			files_changed: number;
			lines_removed: number;
		};
	};

	let recordID = $state<string | number>('');
	let cleaning = $state(false);
	let error = $state('');
	let result = $state<CleanResult | null>(null);
	let pendingConfirmID = $state<number | null>(null);

	function parsedRecordID(): number {
		const n = Number.parseInt(String(recordID).trim(), 10);
		return Number.isFinite(n) ? n : 0;
	}

	function requestCleanConfirmation() {
		const id = parsedRecordID();
		error = '';
		result = null;
		if (id <= 0) {
			error = 'Enter a positive record ID.';
			return;
		}
		pendingConfirmID = id;
	}

	async function cleanArtifacts(id: number) {
		error = '';
		result = null;
		pendingConfirmID = null;
		cleaning = true;
		try {
			const res = await fetch('/api/v1/admin/db/kb-input-artifacts/clean', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ record_id: id })
			});
			const body = await res.json().catch(() => ({}));
			if (!res.ok) {
				throw new Error(body.error_msg ?? body.error ?? `HTTP ${res.status}`);
			}
			result = body as CleanResult;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			cleaning = false;
		}
	}

	function cancelCleanConfirmation() {
		pendingConfirmID = null;
	}
</script>

<div class="p-6 max-w-3xl">
	<div class="mb-6">
		<h1 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">
			Database Maintenance — Clean Artifact Data
		</h1>
		<p style="font-size:13px; color:{textSecondary}; line-height:1.6;">
			Clean all data generated for a <code>kb.inputs</code> record ID. The record may already be
			missing; the cleaner still removes orphaned ArtifactWeb index entries such as
			<code>metrics.txt</code>, <code>topics.txt</code>, and <code>semantic_projections.txt</code>.
		</p>
	</div>

	<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
		<div class="flex items-start gap-3 mb-5 rounded-lg p-3" style="background:{dangerTint}; border:1px solid {danger}40;">
			<AlertTriangleIcon style="width:18px; height:18px; color:{danger}; flex-shrink:0; margin-top:1px;" />
			<p style="font-size:13px; color:{textSecondary}; line-height:1.55;">
				This is a destructive clean operation. Use it for testing and orphan cleanup after verifying
				the record ID. You will be asked to confirm before anything runs.
			</p>
		</div>

		<div class="flex flex-wrap items-end gap-3">
			<div class="flex flex-col gap-1">
				<label for="record-id" style="font-size:12px; font-weight:500; color:{textMuted};">Record ID</label>
				<input
					id="record-id"
					type="number"
					min="1"
					bind:value={recordID}
					placeholder="430"
					style="
						width:220px; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary};
						border-radius:7px; padding:8px 10px; font-size:14px;
					"
				/>
			</div>

			<button
				onclick={requestCleanConfirmation}
				disabled={cleaning}
				style="
					display:flex; align-items:center; gap:7px;
					padding:9px 16px; border-radius:8px; border:none; cursor:pointer;
					font-size:13px; font-weight:600;
					background:{danger}; color:white;
					opacity:{cleaning ? 0.65 : 1};
				"
			>
				{#if cleaning}
					<RefreshCwIcon style="width:14px; height:14px; animation:spin 1s linear infinite;" />
					Cleaning...
				{:else}
					<Trash2Icon style="width:14px; height:14px;" />
					Clean Artifact Data
				{/if}
			</button>
		</div>

		{#if pendingConfirmID !== null}
			<div
				class="mt-4 rounded-lg p-4"
				style="background:{dangerTint}; border:1px solid {danger}66;"
				role="alertdialog"
				aria-labelledby="clean-confirm-title"
				aria-describedby="clean-confirm-desc"
			>
				<div class="flex items-start gap-3">
					<AlertTriangleIcon style="width:18px; height:18px; color:{danger}; flex-shrink:0; margin-top:2px;" />
					<div class="flex-1">
						<div id="clean-confirm-title" style="font-size:14px; font-weight:700; color:{textPrimary}; margin-bottom:4px;">
							Confirm clean for record #{pendingConfirmID}
						</div>
						<p id="clean-confirm-desc" style="font-size:13px; color:{textSecondary}; line-height:1.55; margin-bottom:12px;">
							This will run the same cleanup used by delete record. It may delete related DB rows,
							input row data if present, artifact directories, and ArtifactWeb index entries.
							This action cannot be undone.
						</p>
						<div class="flex flex-wrap gap-2">
							<button
								onclick={() => cleanArtifacts(pendingConfirmID ?? 0)}
								disabled={cleaning}
								style="
									padding:7px 14px; border-radius:7px; border:none; cursor:pointer;
									font-size:13px; font-weight:700; background:{danger}; color:white;
									opacity:{cleaning ? 0.65 : 1};
								"
							>
								Yes, clean record #{pendingConfirmID}
							</button>
							<button
								onclick={cancelCleanConfirmation}
								disabled={cleaning}
								style="
									padding:7px 14px; border-radius:7px; border:1px solid {borderColor}; cursor:pointer;
									font-size:13px; font-weight:600; background:{surface2}; color:{textPrimary};
									opacity:{cleaning ? 0.65 : 1};
								"
							>
								Cancel
							</button>
						</div>
					</div>
				</div>
			</div>
		{/if}

		{#if error}
			<div class="mt-4 rounded-lg p-3" style="background:{dangerTint}; color:{danger}; font-size:13px;">
				{error}
			</div>
		{/if}

		{#if result}
			<div class="mt-4 rounded-lg p-4" style="background:{surface2}; border:1px solid {borderColor};">
				<div class="flex items-center gap-2 mb-3">
					<CheckCircleIcon style="width:16px; height:16px; color:{ok};" />
					<span style="font-size:14px; font-weight:600; color:{textPrimary};">
						Clean completed for record #{result.record_id}
					</span>
				</div>
				<div class="grid gap-2" style="font-size:13px; color:{textSecondary};">
					<div>Input row existed: <strong style="color:{textPrimary};">{result.record_found ? 'yes' : 'no'}</strong></div>
					<div>
						ArtifactWeb files scanned:
						<strong style="color:{textPrimary};">{result.artifact_web_cleanup?.files_scanned ?? 0}</strong>
					</div>
					<div>
						ArtifactWeb files changed:
						<strong style="color:{textPrimary};">{result.artifact_web_cleanup?.files_changed ?? 0}</strong>
					</div>
					<div>
						ArtifactWeb lines removed:
						<strong style="color:{textPrimary};">{result.artifact_web_cleanup?.lines_removed ?? 0}</strong>
					</div>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	@keyframes spin {
		from { transform: rotate(0deg); }
		to { transform: rotate(360deg); }
	}
</style>
