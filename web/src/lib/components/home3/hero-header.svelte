<script lang="ts">
	import BrainCircuitIcon from '@lucide/svelte/icons/brain-circuit';
	import BellIcon from '@lucide/svelte/icons/bell';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';

	let {
		darkMode = true,
		height = 200,
		onToggleDark
	}: {
		darkMode: boolean;
		height: number;
		onToggleDark: () => void;
	} = $props();

	// --- Hero image configuration ---
	const HERO_IMAGE_OPACITY = 0.85; // overall opacity of the centre illustration (0–1)
	const HERO_IMAGE_ANIMATE = true; // set false to freeze animation (e.g. for reduced-motion users)

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

	// SVG fill/stroke derived from accent
	let svgFillLow  = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.15)');
	let svgFillMid  = $derived(darkMode ? 'rgba(129,140,248,0.7)'  : 'rgba(99,102,241,0.7)');
	let svgStrokeLow = $derived(darkMode ? 'rgba(129,140,248,0.4)'  : 'rgba(99,102,241,0.4)');
	let svgStrokeVeryLow = $derived(darkMode ? 'rgba(129,140,248,0.25)' : 'rgba(99,102,241,0.25)');
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
	@keyframes ring-pulse {
		0%   { transform: scale(1);   opacity: 0.5; }
		100% { transform: scale(1.3); opacity: 0; }
	}
	@keyframes node-float-1 {
		0%, 100% { transform: translateY(0px); }
		50%       { transform: translateY(-6px); }
	}
	@keyframes node-float-2 {
		0%, 100% { transform: translateY(0px); }
		50%       { transform: translateY(6px); }
	}
	@keyframes node-float-3 {
		0%, 100% { transform: translateY(0px); }
		50%       { transform: translateY(-4px); }
	}

	.aurora-blob-1 { animation: aurora-drift-1 18s ease-in-out infinite alternate; }
	.aurora-blob-2 { animation: aurora-drift-2 22s ease-in-out infinite alternate; }
	.aurora-blob-3 { animation: aurora-drift-3 26s ease-in-out infinite alternate; }

	.ring-animated { animation: ring-pulse 2.5s ease-out infinite; }
	.ring-animated-delay { animation: ring-pulse 2.5s ease-out infinite 1.25s; }
	.node-float-1 { animation: node-float-1 3s ease-in-out infinite; }
	.node-float-2 { animation: node-float-2 4s ease-in-out infinite; }
	.node-float-3 { animation: node-float-3 5s ease-in-out infinite; }

	.no-animate .ring-animated,
	.no-animate .ring-animated-delay,
	.no-animate .node-float-1,
	.no-animate .node-float-2,
	.no-animate .node-float-3 {
		animation: none;
	}
</style>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<header
	class="flex-shrink-0 relative overflow-hidden"
	style="height:{height}px; border-bottom:1px solid {borderColor};"
>
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

		<!-- Three zones row -->
		<div class="flex items-center flex-1 px-6 gap-4">

			<!-- Left zone (~280px) -->
			<div class="flex-shrink-0" style="width:280px; font-family:{fontUI};">
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
				<p style="font-size:13px; color:{textSecondary}; margin-left:52px; font-family:{fontUI};">Your intelligent AI workspace</p>
			</div>

			<!-- Centre zone (flex-1): SVG neural constellation -->
			<div class="flex-1 flex items-center justify-center">
				<div style="opacity:{HERO_IMAGE_OPACITY};">
					<svg
						width="300"
						height="160"
						viewBox="0 0 300 160"
						fill="none"
						xmlns="http://www.w3.org/2000/svg"
						class={HERO_IMAGE_ANIMATE ? '' : 'no-animate'}
					>
						<!-- Large glowing background circles -->
						<circle cx="150" cy="80" r="55" fill={svgFillLow} stroke={svgStrokeLow} stroke-width="1.5" />
						<circle cx="60"  cy="50" r="28" fill={svgFillLow} stroke={svgStrokeLow} stroke-width="1.5" />
						<circle cx="245" cy="110" r="32" fill={svgFillLow} stroke={svgStrokeLow} stroke-width="1.5" />

						<!-- Connection lines between nodes -->
						<line x1="150" y1="80"  x2="60"  y2="50"  stroke={svgStrokeVeryLow} stroke-width="1.5" />
						<line x1="150" y1="80"  x2="245" y2="110" stroke={svgStrokeVeryLow} stroke-width="1.5" />
						<line x1="150" y1="80"  x2="100" y2="130" stroke={svgStrokeVeryLow} stroke-width="1.5" />
						<line x1="150" y1="80"  x2="220" y2="35"  stroke={svgStrokeVeryLow} stroke-width="1.5" />
						<line x1="60"  y1="50"  x2="30"  y2="110" stroke={svgStrokeVeryLow} stroke-width="1" />
						<line x1="245" y1="110" x2="270" y2="55"  stroke={svgStrokeVeryLow} stroke-width="1" />
						<line x1="100" y1="130" x2="245" y2="110" stroke={svgStrokeVeryLow} stroke-width="1" />
						<line x1="220" y1="35"  x2="60"  y2="50"  stroke={svgStrokeVeryLow} stroke-width="1" />

						<!-- Animated rings around central node -->
						<circle class="ring-animated" cx="150" cy="80" r="28" fill="none" stroke={accent} stroke-width="1.5" />
						<circle class="ring-animated-delay" cx="150" cy="80" r="28" fill="none" stroke={accent} stroke-width="1" />

						<!-- Central node -->
						<circle cx="150" cy="80" r="8" fill={accent} />
						<circle cx="150" cy="80" r="4" fill="white" opacity="0.8" />

						<!-- Satellite nodes (some animated) -->
						<g class="node-float-1">
							<circle cx="60" cy="50" r="6" fill={svgFillMid} stroke={accent} stroke-width="1.5" />
						</g>
						<g class="node-float-2">
							<circle cx="245" cy="110" r="6" fill={svgFillMid} stroke={accent} stroke-width="1.5" />
						</g>
						<g class="node-float-3">
							<circle cx="100" cy="130" r="5" fill={svgFillMid} stroke={accent} stroke-width="1.5" />
						</g>
						<circle cx="220" cy="35" r="5" fill={svgFillMid} stroke={accent} stroke-width="1.5" />
						<circle cx="30"  cy="110" r="4" fill={svgFillMid} stroke={accent} stroke-width="1.5" />
						<circle cx="270" cy="55"  r="4" fill={svgFillMid} stroke={accent} stroke-width="1.5" />

						<!-- Small floating accent dots -->
						<g class="node-float-1">
							<circle cx="185" cy="55" r="3" fill={accent} opacity="0.6" />
						</g>
						<g class="node-float-2">
							<circle cx="120" cy="115" r="3" fill={accent} opacity="0.7" />
						</g>
						<g class="node-float-3">
							<circle cx="200" cy="140" r="2.5" fill={accent} opacity="0.5" />
						</g>
					</svg>
				</div>
			</div>

			<!-- Right zone (~220px, text-right) -->
			<div class="flex-shrink-0 flex flex-col items-end gap-2" style="width:220px; font-family:{fontUI};">
				<!-- AI model pills -->
				<div class="flex gap-2 flex-wrap justify-end">
					<span
						class="px-2.5 py-1 rounded-full text-xs"
						style="background:{accentTint}; color:{accent}; font-family:{fontMono}; border:1px solid {accent}30;"
					>Claude Sonnet 4.6</span>
					<span
						class="px-2.5 py-1 rounded-full text-xs"
						style="background:rgba(16,185,129,0.10); color:#10B981; font-family:{fontMono}; border:1px solid rgba(16,185,129,0.25);"
					>GPT-4o</span>
				</div>
				<!-- Bell + toggle row -->
				<div class="flex items-center gap-3">
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
