<script lang="ts">
	import { goto } from '$app/navigation';
	import { locales, getLocale, setLocale } from '$lib/paraglide/runtime';
	import type { SiteConfig } from '$lib/services/siteConfigService';
	import BrainCircuitIcon from '@lucide/svelte/icons/brain-circuit';
	import BellIcon from '@lucide/svelte/icons/bell';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import LanguagesIcon from '@lucide/svelte/icons/languages';

	let {
		darkMode = true,
		height = 200,
		cfg,
		onToggleDark
	}: {
		darkMode: boolean;
		height: number;
		cfg: SiteConfig;
		onToggleDark: () => void;
	} = $props();

	function nextLocale(): (typeof locales)[number] {
		const idx = locales.indexOf(getLocale());
		return locales[(idx + 1) % locales.length];
	}

	// --- Typography (adjust here to change fonts) ---
	const fontUI   = "system-ui, -apple-system, 'Inter', sans-serif"; // all UI text
	const fontMono = "'Fira Code', 'Cascadia Code', monospace";        // data, code, version badges

	// --- Design tokens (adjust here to restyle the component) ---
	let pageBg        = $derived(darkMode ? '#171B26'  : '#F2F4F7');  // page background
	let cardBg        = $derived(darkMode ? '#1F2333'  : '#FFFFFF');  // card surface
	let borderColor   = $derived(darkMode ? '#2D3348'  : '#E4E6EB');  // border / divider lines
	let accent        = $derived(darkMode ? '#818CF8'  : '#6366F1');  // primary accent (indigo)
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)'); // subtle accent fill
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');   // headings and labels
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');   // body text

	// Aurora blob colours (adapt saturation for light vs dark)
	let auroraIndigo = $derived(darkMode ? 'rgba(99,102,241,0.22)'  : 'rgba(99,102,241,0.09)');
	let auroraViolet = $derived(darkMode ? 'rgba(139,92,246,0.20)'  : 'rgba(139,92,246,0.08)');
	let auroraCyan   = $derived(darkMode ? 'rgba(6,182,212,0.18)'   : 'rgba(6,182,212,0.07)');
</script>

<style>
	@keyframes aurora-drift-1 {
		0%   { transform: translate(0px, 0px) scale(1); }
		50%  { transform: translate(40px, -20px) scale(1.1); }
		100% { transform: translate(-20px, 30px) scale(0.95); }
	}
	@keyframes aurora-drift-2 {
		0%   { transform: translate(0px, 0px) scale(1); }
		50%  { transform: translate(-50px, 25px) scale(1.08); }
		100% { transform: translate(30px, -15px) scale(1.05); }
	}
	@keyframes aurora-drift-3 {
		0%   { transform: translate(0px, 0px) scale(1); }
		50%  { transform: translate(20px, 40px) scale(0.92); }
		100% { transform: translate(-35px, -20px) scale(1.12); }
	}
	.aurora-blob-1 { animation: aurora-drift-1 18s ease-in-out infinite alternate; }
	.aurora-blob-2 { animation: aurora-drift-2 22s ease-in-out infinite alternate; }
	.aurora-blob-3 { animation: aurora-drift-3 26s ease-in-out infinite alternate; }
</style>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<header
	class="flex-shrink-0 relative overflow-hidden"
	style="height:{height}px; border-bottom:1px solid {borderColor};"
>
	<!-- Hero background image, lightened with a paper veil (same treatment as the Main page hero) -->
	<img src={cfg.hero.image} alt="" class="absolute inset-0 h-full w-full object-cover" style="z-index:0;" />
	<div class="absolute inset-0 bg-[#faf9f7]/70 dark:bg-[#101216]/70" style="z-index:0;"></div>

	<!-- Aurora background (position absolute, z-index 0) -->
	<div class="absolute inset-0 overflow-hidden" style="z-index:0;">
		<div
			class="aurora-blob-1 absolute rounded-full"
			style="width:420px; height:280px; top:-80px; left:-60px; background:radial-gradient(ellipse, {auroraIndigo} 0%, transparent 70%);"
		></div>
		<div
			class="aurora-blob-2 absolute rounded-full"
			style="width:380px; height:260px; top:-40px; left:35%; background:radial-gradient(ellipse, {auroraViolet} 0%, transparent 70%);"
		></div>
		<div
			class="aurora-blob-3 absolute rounded-full"
			style="width:340px; height:220px; top:-20px; right:60px; background:radial-gradient(ellipse, {auroraCyan} 0%, transparent 70%);"
		></div>
	</div>

	<!-- Header content (position relative, z-index 1) -->
	<div class="relative flex flex-col h-full" style="z-index:1;">

		<!-- Three zones row: left spacer / centred branding / right controls -->
		<div class="grid items-center flex-1 px-6 gap-4" style="grid-template-columns: 1fr auto 1fr;">

			<!-- Left spacer (keeps centre column truly centred against the right zone) -->
			<div></div>

			<!-- Centre zone: company logo + wordmark, centred in the panel -->
			<div class="flex flex-col items-center" style="font-family:{fontUI};">
				<!-- Company logo (click to return to the Main page) -->
				<button
					type="button"
					onclick={() => goto('/semos')}
					class="block cursor-pointer border-0 bg-transparent p-0 mb-2"
					aria-label="Go to Main page"
				>
					<img src={cfg.branding.logo_image} alt={cfg.branding.logo_text} class="h-7 w-auto" />
				</button>
				<!-- Logo + wordmark row -->
				<div class="flex items-center gap-3 mb-1">
					<div
						class="flex items-center justify-center rounded-xl"
						style="width:40px; height:40px; background:{accentTint}; border:1px solid {accent}40;"
					>
						<BrainCircuitIcon class="w-6 h-6" style="color:{accent};" />
					</div>
					<div>
						<div class="flex items-center gap-2">
							<span style="font-size:20px; font-weight:700; color:{accent}; font-family:{fontMono};">MyAI</span>
							<span style="font-size:20px; font-weight:700; color:{textPrimary};">Assistant</span>
							<span
								class="px-2 py-0.5 rounded-full"
								style="font-size:11px; font-family:{fontMono}; background:{accentTint}; color:{accent}; border:1px solid {accent}30;"
							>v3.0</span>
						</div>
					</div>
				</div>
				<!-- Tagline -->
				<p style="font-size:13px; color:{textSecondary}; font-family:{fontUI};">Your intelligent AI workspace</p>
			</div>

			<!-- Right zone (text-right) -->
			<div class="flex flex-col items-end gap-2 justify-self-end" style="font-family:{fontUI};">
				<!-- Main / Workspace nav buttons -->
				<div class="flex gap-2 flex-wrap justify-end">
					<button
						type="button"
						onclick={() => goto('/semos')}
						class="px-2.5 py-1 rounded-full text-xs cursor-pointer"
						style="background:{accentTint}; color:{accent}; font-family:{fontMono}; border:1px solid {accent}30;"
					>Main</button>
					<button
						type="button"
						onclick={() => goto('/semos/workspace')}
						class="px-2.5 py-1 rounded-full text-xs cursor-pointer"
						style="background:{accentTint}; color:{accent}; font-family:{fontMono}; border:1px solid {accent}30;"
					>Workspace</button>
				</div>
				<!-- Language + bell + toggle row -->
				<div class="flex items-center gap-3">
					<!-- Language control -->
					<button
						type="button"
						onclick={() => setLocale(nextLocale())}
						class="flex items-center gap-1.5 h-8 px-2.5 rounded-lg cursor-pointer"
						style="background:{accentTint}; color:{textSecondary};"
						aria-label="Switch language"
					>
						<LanguagesIcon class="w-4 h-4" />
						<span style="font-size:12px;">{getLocale() === 'zh-cn' ? '中文' : 'English'}</span>
					</button>
					<!-- Bell with notification dot -->
					<div class="relative">
						<button
							class="flex items-center justify-center w-8 h-8 rounded-lg cursor-pointer"
							style="background:{accentTint};"
							aria-label="Notifications"
						>
							<BellIcon class="w-4 h-4" style="color:{textSecondary};" />
						</button>
						<div
							class="absolute top-1 right-1 w-2 h-2 rounded-full"
							style="background:#F59E0B; border:1.5px solid {pageBg};"
						></div>
					</div>
					<!-- Dark/light toggle -->
					<button
						onclick={onToggleDark}
						class="flex items-center justify-center w-8 h-8 rounded-lg cursor-pointer transition-colors duration-150"
						style="background:{accentTint}; color:{accent};"
						aria-label="Toggle dark mode"
					>
						{#if darkMode}
							<SunIcon class="w-4 h-4" />
						{:else}
							<MoonIcon class="w-4 h-4" />
						{/if}
					</button>
				</div>
			</div>
		</div>

		<!-- Status strip (full width, 28px, subtle top border) -->
		<div
			class="flex items-center gap-4 px-6 flex-shrink-0"
			style="height:28px; border-top:1px solid {borderColor}; background:{cardBg};"
		>
			<!-- 3 agents active -->
			<div class="flex items-center gap-1.5" style="font-family:{fontUI};">
				<div class="w-1.5 h-1.5 rounded-full" style="background:#10B981;"></div>
				<span style="font-size:12px; color:{textSecondary};">3 agents active</span>
			</div>
			<div style="color:{textSecondary}; font-size:12px;">•</div>
			<!-- 12 tasks running -->
			<div class="flex items-center gap-1.5" style="font-family:{fontUI};">
				<div class="w-1.5 h-1.5 rounded-full" style="background:#F59E0B;"></div>
				<span style="font-size:12px; color:{textSecondary};">12 tasks running</span>
			</div>
			<div style="color:{textSecondary}; font-size:12px;">•</div>
			<!-- All systems nominal -->
			<div class="flex items-center gap-1.5" style="font-family:{fontUI};">
				<div class="w-1.5 h-1.5 rounded-full" style="background:#10B981;"></div>
				<span style="font-size:12px; color:{textSecondary};">All systems nominal</span>
			</div>
		</div>
	</div>
</header>
