<script lang="ts">
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import BrainCircuitIcon from '@lucide/svelte/icons/brain-circuit';
	import BellIcon from '@lucide/svelte/icons/bell';
	import ServerIcon from '@lucide/svelte/icons/server';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import ZapIcon from '@lucide/svelte/icons/zap';

	let {
		darkMode,
		height,
		onToggleDark
	}: {
		darkMode: boolean;
		height: number;
		onToggleDark: () => void;
	} = $props();

	// --- Configurable illustration constants ---
	const HERO_IMAGE_OPACITY = 0.85; // overall opacity of the centre illustration
	const HERO_IMAGE_ANIMATE = true; // set false to freeze animation

	// --- Colour tokens (all $derived from darkMode) ---
	let pageBg      = $derived(darkMode ? '#171B26' : '#F2F4F7');       // page background
	let surface2    = $derived(darkMode ? '#1E2535' : '#E8EBF2');       // secondary surface
	let border      = $derived(darkMode ? '#2D3348' : '#E4E6EB');       // border / divider lines
	let textMain    = $derived(darkMode ? '#E2E8F0' : '#111827');       // primary text
	let textMuted   = $derived(darkMode ? '#64748b' : '#94a3b8');       // muted text
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');       // primary accent (indigo)
	let ctrlBg      = $derived(darkMode ? 'rgba(30,37,53,0.85)' : 'rgba(232,235,242,0.9)'); // control background
	let ctrlBorder  = $derived(darkMode ? '#2D3348' : '#D0D4DF');       // control border

	// Suppress unused var warning
	void HERO_IMAGE_ANIMATE;
</script>

<header
	class="relative flex-shrink-0 overflow-hidden"
	style="height:{height}px; border-bottom:1px solid {border};"
>
	<!-- Aurora background — pure CSS @keyframes, no JS -->
	<div class="aurora-bg absolute inset-0" style="background:{pageBg};"></div>

	<!-- Three-zone layout -->
	<div class="relative z-10 flex h-full w-full items-stretch">

		<!-- LEFT ZONE: logo + wordmark + badge + tagline -->
		<div class="flex flex-col justify-center px-8 flex-shrink-0" style="width:280px;">
			<!-- Gradient mask so left zone reads cleanly -->
			<div
				class="absolute inset-y-0 left-0 pointer-events-none"
				style="width:320px; background: linear-gradient(to right, {pageBg} 0%, {pageBg}E0 55%, transparent 100%);"
			></div>
			<div class="relative z-10">
				<!-- Logo + wordmark row -->
				<div class="flex items-center gap-3 mb-1">
					<div
						class="flex items-center justify-center w-11 h-11 rounded-xl flex-shrink-0"
						style="background:rgba(99,102,241,0.18); border:1px solid rgba(99,102,241,0.40);"
					>
						<BrainCircuitIcon class="w-6 h-6" style="color:{accent};" />
					</div>
					<div>
						<div class="flex items-center gap-2">
							<h1 class="text-2xl font-bold tracking-tight" style="font-family:'Fira Code',monospace; color:{textMain};">
								MyAI<span style="color:{accent};">Assistant</span>
							</h1>
							<span
								class="px-2 py-0.5 text-xs font-semibold rounded-full"
								style="background:rgba(99,102,241,0.15); color:{accent}; border:1px solid rgba(99,102,241,0.35); font-family:'Fira Code',monospace;"
							>v4.0</span>
						</div>
					</div>
				</div>

				<!-- Tagline -->
				<p class="text-sm mt-1 ml-14" style="color:{textMuted};">
					Your intelligent AI workspace
				</p>
			</div>
		</div>

		<!-- CENTRE ZONE: hero SVG neural constellation -->
		<div class="flex-1 flex items-center justify-center relative overflow-hidden">
			<svg
				viewBox="0 0 340 160"
				preserveAspectRatio="xMidYMid meet"
				aria-hidden="true"
				style="width:100%; height:100%; max-width:680px; opacity:{HERO_IMAGE_OPACITY};"
			>
				<defs>
					<!-- Radial gradients for glow blobs -->
					<radialGradient id="blob1" cx="50%" cy="50%" r="50%">
						<stop offset="0%" stop-color="#6366F1" stop-opacity="0.35" />
						<stop offset="100%" stop-color="#6366F1" stop-opacity="0" />
					</radialGradient>
					<radialGradient id="blob2" cx="50%" cy="50%" r="50%">
						<stop offset="0%" stop-color="#818CF8" stop-opacity="0.28" />
						<stop offset="100%" stop-color="#818CF8" stop-opacity="0" />
					</radialGradient>
					<radialGradient id="blob3" cx="50%" cy="50%" r="50%">
						<stop offset="0%" stop-color="#7C3AED" stop-opacity="0.22" />
						<stop offset="100%" stop-color="#7C3AED" stop-opacity="0" />
					</radialGradient>
					<radialGradient id="blob4" cx="50%" cy="50%" r="50%">
						<stop offset="0%" stop-color="#06b6d4" stop-opacity="0.18" />
						<stop offset="100%" stop-color="#06b6d4" stop-opacity="0" />
					</radialGradient>

					<!-- Node glow filter -->
					<filter id="node-glow4" x="-80%" y="-80%" width="260%" height="260%">
						<feGaussianBlur in="SourceGraphic" stdDeviation="2.5" result="blur" />
						<feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
					</filter>
					<filter id="hub-glow4" x="-100%" y="-100%" width="300%" height="300%">
						<feGaussianBlur in="SourceGraphic" stdDeviation="5" result="blur" />
						<feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
					</filter>
				</defs>

				<!-- Glow blobs -->
				<ellipse cx="170" cy="80" rx="90" ry="70" fill="url(#blob1)" />
				<ellipse cx="100" cy="60" rx="60" ry="50" fill="url(#blob2)" />
				<ellipse cx="240" cy="100" rx="65" ry="55" fill="url(#blob3)" />
				<ellipse cx="170" cy="130" rx="80" ry="45" fill="url(#blob4)" />

				<!-- Connecting lines (stroke accent, opacity 0.25) -->
				<line x1="80" y1="50" x2="130" y2="80" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="80" y1="50" x2="110" y2="35" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="130" y1="80" x2="170" y2="80" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="130" y1="80" x2="155" y2="55" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="170" y1="80" x2="210" y2="80" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="170" y1="80" x2="185" y2="55" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="170" y1="80" x2="155" y2="110" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="210" y1="80" x2="260" y2="55" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="210" y1="80" x2="235" y2="100" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />
				<line x1="110" y1="35" x2="155" y2="55" stroke="#6366F1" stroke-width="0.8" stroke-opacity="0.20" />
				<line x1="155" y1="55" x2="185" y2="55" stroke="#6366F1" stroke-width="0.8" stroke-opacity="0.20" />
				<line x1="185" y1="55" x2="225" y2="40" stroke="#6366F1" stroke-width="0.8" stroke-opacity="0.20" />
				<line x1="155" y1="110" x2="185" y2="125" stroke="#06b6d4" stroke-width="0.8" stroke-opacity="0.20" />
				<line x1="185" y1="125" x2="235" y2="100" stroke="#06b6d4" stroke-width="0.8" stroke-opacity="0.20" />
				<line x1="260" y1="55" x2="260" y2="85" stroke="#818CF8" stroke-width="0.8" stroke-opacity="0.25" />

				<!-- Nodes with float animation -->
				<!-- n0: left entry -->
				<circle cx="80" cy="50" r="4" fill="#818CF8" filter="url(#hub-glow4)" class="node-float" style="animation-delay:0s;" />
				<!-- n1: top left -->
				<circle cx="110" cy="35" r="2.5" fill="#6366F1" filter="url(#node-glow4)" class="node-float" style="animation-delay:0.4s;" />
				<!-- n2: mid-left hub -->
				<circle cx="130" cy="80" r="3.5" fill="#818CF8" filter="url(#node-glow4)" class="node-float" style="animation-delay:0.8s;" />
				<!-- n3: top-centre left -->
				<circle cx="155" cy="55" r="2.5" fill="#6366F1" filter="url(#node-glow4)" class="node-float" style="animation-delay:1.2s;" />
				<!-- n4: CENTRE HUB — has pulse rings -->
				<circle cx="170" cy="80" r="6" fill="#818CF8" filter="url(#hub-glow4)" class="node-float" style="animation-delay:0s;" />
				<circle cx="170" cy="80" r="6" fill="none" stroke="#818CF8" stroke-width="1.5" stroke-opacity="0.5" class="ring-pulse" style="animation-delay:0s;" />
				<circle cx="170" cy="80" r="6" fill="none" stroke="#6366F1" stroke-width="1" stroke-opacity="0.4" class="ring-pulse" style="animation-delay:1.25s;" />
				<!-- n5: top-centre right -->
				<circle cx="185" cy="55" r="2.5" fill="#6366F1" filter="url(#node-glow4)" class="node-float" style="animation-delay:0.5s;" />
				<!-- n6: below centre -->
				<circle cx="155" cy="110" r="2.5" fill="#06b6d4" filter="url(#node-glow4)" class="node-float" style="animation-delay:1.6s;" />
				<!-- n7: right hub -->
				<circle cx="210" cy="80" r="4" fill="#818CF8" filter="url(#hub-glow4)" class="node-float" style="animation-delay:0.7s;" />
				<!-- n8: top right far -->
				<circle cx="225" cy="40" r="2.5" fill="#6366F1" filter="url(#node-glow4)" class="node-float" style="animation-delay:1.0s;" />
				<!-- n9: far right top -->
				<circle cx="260" cy="55" r="3" fill="#818CF8" filter="url(#node-glow4)" class="node-float" style="animation-delay:0.3s;" />
				<!-- n10: far right bottom -->
				<circle cx="260" cy="85" r="2.5" fill="#6366F1" filter="url(#node-glow4)" class="node-float" style="animation-delay:1.4s;" />
				<!-- n11: bottom right -->
				<circle cx="235" cy="100" r="3" fill="#06b6d4" filter="url(#node-glow4)" class="node-float" style="animation-delay:0.9s;" />
				<!-- n12: bottom centre -->
				<circle cx="185" cy="125" r="2.5" fill="#06b6d4" filter="url(#node-glow4)" class="node-float" style="animation-delay:2.0s;" />
			</svg>
		</div>

		<!-- RIGHT ZONE: model pills + bell + dark toggle -->
		<div class="flex flex-col justify-center items-end px-8 flex-shrink-0 gap-3" style="width:280px;">
			<!-- Gradient mask so right zone reads cleanly -->
			<div
				class="absolute inset-y-0 right-0 pointer-events-none"
				style="width:320px; background: linear-gradient(to left, {pageBg} 0%, {pageBg}E0 55%, transparent 100%);"
			></div>

			<div class="relative z-10 flex flex-col items-end gap-3">
				<!-- Model pills -->
				<div class="flex flex-col gap-2">
					<div
						class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs"
						style="background:{ctrlBg}; border:1px solid {ctrlBorder}; color:{textMuted}; font-family:'Fira Code',monospace;"
					>
						<ServerIcon class="w-3 h-3" style="color:{accent};" />
						Claude Sonnet 4.6
					</div>
					<div
						class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs"
						style="background:{ctrlBg}; border:1px solid {ctrlBorder}; color:{textMuted}; font-family:'Fira Code',monospace;"
					>
						<ServerIcon class="w-3 h-3" style="color:#818cf8;" />
						GPT-4o
					</div>
				</div>

				<!-- Bell + toggle row -->
				<div class="flex items-center gap-2">
					<!-- Activity bell with notification dot -->
					<div
						class="relative flex items-center justify-center w-9 h-9 rounded-lg"
						style="background:{ctrlBg}; border:1px solid {ctrlBorder};"
					>
						<BellIcon class="w-4 h-4" style="color:{textMuted};" />
						<div class="absolute top-1.5 right-1.5 w-1.5 h-1.5 rounded-full" style="background:{accent}; animation:ledpulse 2s infinite;"></div>
					</div>

					<!-- Dark/light toggle -->
					<button
						onclick={onToggleDark}
						class="flex items-center justify-center w-9 h-9 rounded-lg cursor-pointer transition-colors duration-200"
						style="background:{ctrlBg}; border:1px solid {ctrlBorder};"
						aria-label="Toggle dark mode"
					>
						{#if darkMode}
							<SunIcon class="w-4 h-4" style="color:#fbbf24;" />
						{:else}
							<MoonIcon class="w-4 h-4" style="color:#475569;" />
						{/if}
					</button>
				</div>
			</div>
		</div>
	</div>

	<!-- STATUS STRIP (28px, full width, below three zones) -->
	<div
		class="absolute bottom-0 left-0 right-0 flex items-center gap-6 px-8"
		style="height:28px; background:{surface2}80; border-top:1px solid {border};"
	>
		<div class="flex items-center gap-1.5">
			<div class="w-1.5 h-1.5 rounded-full" style="background:#22C55E; animation:ledpulse 2s infinite;"></div>
			<span class="text-xs" style="color:{textMuted};">3 agents active</span>
		</div>
		<div class="flex items-center gap-1.5">
			<ZapIcon class="w-3 h-3" style="color:#eab308;" />
			<span class="text-xs" style="color:{textMuted};">12 tasks running</span>
		</div>
		<div class="flex items-center gap-1.5">
			<ActivityIcon class="w-3 h-3" style="color:#22C55E;" />
			<span class="text-xs" style="color:{textMuted};">All systems nominal</span>
		</div>
	</div>
</header>

<style>
	/* Aurora background — CSS only, no JS */
	.aurora-bg {
		position: absolute;
		inset: 0;
	}
	.aurora-bg::before,
	.aurora-bg::after {
		content: '';
		position: absolute;
		border-radius: 50%;
		filter: blur(60px);
		opacity: 0.35;
	}
	.aurora-bg::before {
		width: 500px;
		height: 300px;
		top: -80px;
		left: 20%;
		background: radial-gradient(ellipse, #6366F1 0%, #7C3AED 50%, transparent 80%);
		animation: aurora-drift1 18s ease-in-out infinite alternate;
	}
	.aurora-bg::after {
		width: 400px;
		height: 250px;
		bottom: -60px;
		right: 15%;
		background: radial-gradient(ellipse, #818CF8 0%, #06b6d4 60%, transparent 80%);
		animation: aurora-drift2 22s ease-in-out infinite alternate;
	}

	@keyframes aurora-drift1 {
		0%   { transform: translate(0, 0) scale(1); }
		50%  { transform: translate(80px, 30px) scale(1.15); }
		100% { transform: translate(-40px, 20px) scale(0.95); }
	}
	@keyframes aurora-drift2 {
		0%   { transform: translate(0, 0) scale(1); }
		50%  { transform: translate(-60px, -20px) scale(1.10); }
		100% { transform: translate(50px, 15px) scale(0.90); }
	}

	/* Node float — Y translate ±6px */
	@keyframes node-float {
		0%, 100% { transform: translateY(0); }
		50%       { transform: translateY(-6px); }
	}
	.node-float {
		animation: node-float 3.5s ease-in-out infinite;
	}

	/* Pulse rings around centre node */
	@keyframes ring-pulse {
		0%   { transform: scale(1);   opacity: 0.5; }
		100% { transform: scale(1.4); opacity: 0; }
	}
	.ring-pulse {
		animation: ring-pulse 2.5s ease-out infinite;
		transform-origin: 170px 80px;
	}

	/* LED status dot */
	@keyframes ledpulse {
		0%, 100% { opacity: 1; }
		50%       { opacity: 0.25; }
	}
</style>
