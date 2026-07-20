<script lang="ts">
	let { darkMode = true }: { darkMode?: boolean } = $props();

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const accent = $derived(darkMode ? '#F59E0B' : '#D97706');
</script>

<section class="wrap" style="background:{pageBg}; color:{heading};">
	<div class="card" style="background:{card}; border:1px solid {border};">
		<div class="eyebrow" style="background:{accent}20; color:{accent};">Deferred</div>
		<h2>Access Controls</h2>
		<p style="color:{sub};">Page under construction.</p>
	</div>
</section>

<style>
	.wrap {
		min-height: 100%;
		padding: 24px;
		display: flex;
		align-items: flex-start;
	}
	.card {
		max-width: 720px;
		border-radius: 20px;
		padding: 24px;
	}
	.eyebrow {
		display: inline-flex;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.14em;
		padding: 6px 10px;
		text-transform: uppercase;
	}
	h2 {
		margin: 14px 0 8px;
		font-size: 24px;
	}
</style>
