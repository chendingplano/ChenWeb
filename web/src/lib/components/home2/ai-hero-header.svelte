<script lang="ts">
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import BrainCircuitIcon from '@lucide/svelte/icons/brain-circuit';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import ServerIcon from '@lucide/svelte/icons/server';

	let { darkMode, onToggleDark }: { darkMode: boolean; onToggleDark: () => void } = $props();

	// Neural network node positions (x, y, radius, color, pulse-delay)
	// Laid out across the right 65% of the 1400px viewBox
	const nodes: [number, number, number, string, number][] = [
		[480,  80,  4.5, '#22C55E', 0.0],   // n0  — left entry hub
		[545,  28,  3.0, '#818cf8', 0.3],   // n1
		[552,  92,  3.0, '#06b6d4', 0.7],   // n2
		[558, 148,  2.5, '#22C55E', 1.1],   // n3
		[615,  55,  3.0, '#818cf8', 0.2],   // n4
		[624, 118,  3.0, '#06b6d4', 0.9],   // n5
		[685,  36,  2.5, '#22C55E', 0.5],   // n6
		[692,  90,  4.5, '#818cf8', 0.0],   // n7  — center hub
		[700, 150,  2.0, '#06b6d4', 1.3],   // n8
		[755,  62,  3.0, '#22C55E', 0.4],   // n9
		[762, 126,  3.0, '#818cf8', 0.8],   // n10
		[822,  44,  2.5, '#06b6d4', 0.1],   // n11
		[830, 100,  4.5, '#22C55E', 0.6],   // n12 — right-mid hub
		[838, 153,  2.0, '#818cf8', 1.2],   // n13
		[894,  72,  3.0, '#06b6d4', 0.3],   // n14
		[904, 132,  3.0, '#22C55E', 0.9],   // n15
		[965,  50,  2.5, '#818cf8', 0.5],   // n16
		[974, 112,  3.5, '#06b6d4', 0.0],   // n17
		[1040, 80,  5.0, '#22C55E', 0.7],   // n18 — right output hub
	];

	// Connections [from, to, opacity]
	const edges: [number, number, number][] = [
		[0,1,0.55],[0,2,0.55],[0,3,0.40],
		[1,2,0.35],[1,4,0.50],[2,4,0.45],[2,5,0.45],[3,5,0.40],
		[4,6,0.45],[4,7,0.55],[5,7,0.50],[5,8,0.35],
		[6,7,0.40],[6,9,0.50],[7,8,0.30],[7,9,0.55],[7,10,0.50],
		[8,10,0.35],[9,11,0.45],[9,12,0.55],[10,12,0.50],[10,13,0.35],
		[11,12,0.40],[11,14,0.50],[12,13,0.30],[12,14,0.55],[12,15,0.45],
		[13,15,0.35],[14,16,0.50],[14,17,0.55],[15,17,0.45],
		[16,17,0.40],[16,18,0.55],[17,18,0.60],
	];

	// Animated data-flow polyline paths (sequences of node indices)
	function pts(ids: number[]) {
		return ids.map(i => `${nodes[i][0]},${nodes[i][1]}`).join(' ');
	}
	const flow1 = pts([0,1,4,6,9,11,14,16,18]);  // top highway — green
	const flow2 = pts([0,2,4,7,9,12,14,17,18]);  // mid highway — purple
	const flow3 = pts([0,3,5,7,10,12,15,17,18]); // bot highway — cyan

	let edgeColor = $derived(darkMode ? 0.6 : 0.5);
</script>

<header
	class="relative flex-shrink-0 overflow-hidden"
	style="height:160px; border-bottom:1px solid {darkMode ? '#1E293B' : '#e2e8f0'};"
>
	<!-- SVG scene — pure CSS animations, no JS loop -->
	<svg
		class="absolute inset-0 w-full h-full"
		viewBox="0 0 1400 160"
		preserveAspectRatio="xMidYMid slice"
		aria-hidden="true"
	>
		<defs>
			<linearGradient id="hdr-bg" x1="0" y1="0" x2="1" y2="1" gradientUnits="objectBoundingBox">
				<stop offset="0%" stop-color={darkMode ? '#020617' : '#f1f5f9'} />
				<stop offset="100%" stop-color={darkMode ? '#0d1628' : '#e8edf5'} />
			</linearGradient>

			<!-- Fine dot grid -->
			<pattern id="hdr-dots" x="0" y="0" width="32" height="32" patternUnits="userSpaceOnUse">
				<circle cx="16" cy="16" r="0.8" fill={darkMode ? 'rgba(34,197,94,0.18)' : 'rgba(34,197,94,0.28)'} />
			</pattern>

			<!-- Soft glow filter for nodes -->
			<filter id="node-glow" x="-80%" y="-80%" width="260%" height="260%">
				<feGaussianBlur in="SourceGraphic" stdDeviation="2.5" result="blur" />
				<feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
			</filter>

			<!-- Stronger glow for hub nodes -->
			<filter id="hub-glow" x="-100%" y="-100%" width="300%" height="300%">
				<feGaussianBlur in="SourceGraphic" stdDeviation="5" result="blur" />
				<feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
			</filter>
		</defs>

		<!-- Background fill -->
		<rect width="1400" height="160" fill="url(#hdr-bg)" />
		<!-- Dot grid overlay -->
		<rect width="1400" height="160" fill="url(#hdr-dots)" />

		<!-- Decorative backdrop rings -->
		<circle cx="692" cy="90" r="220" fill="none"
			stroke={darkMode ? 'rgba(129,140,248,0.05)' : 'rgba(129,140,248,0.08)'} stroke-width="1" />
		<circle cx="1040" cy="80" r="180" fill="none"
			stroke={darkMode ? 'rgba(34,197,94,0.05)' : 'rgba(34,197,94,0.08)'} stroke-width="1" />
		<circle cx="480" cy="80" r="130" fill="none"
			stroke={darkMode ? 'rgba(6,182,212,0.04)' : 'rgba(6,182,212,0.07)'} stroke-width="0.8" />

		<!-- Static connection edges -->
		{#each edges as [from, to, alpha]}
			<line
				x1={nodes[from][0]} y1={nodes[from][1]}
				x2={nodes[to][0]}   y2={nodes[to][1]}
				stroke={darkMode
					? `rgba(34,197,94,${alpha * edgeColor})`
					: `rgba(20,120,60,${alpha * edgeColor})`}
				stroke-width="0.9"
			/>
		{/each}

		<!-- Animated data-flow paths -->
		<polyline points={flow1} fill="none" stroke="#22C55E"  stroke-width="1.2"
			stroke-dasharray="14 600" class="flow flow1" />
		<polyline points={flow2} fill="none" stroke="#818cf8"  stroke-width="1.0"
			stroke-dasharray="11 560" class="flow flow2" />
		<polyline points={flow3} fill="none" stroke="#06b6d4"  stroke-width="1.0"
			stroke-dasharray="9  520" class="flow flow3" />

		<!-- Nodes (regular) -->
		{#each nodes as [x, y, r, color, delay], i}
			{#if r < 4}
				<!-- Regular node: outer halo + core -->
				<circle {x} {y} r={r * 3} fill={color} fill-opacity="0.08"
					class="pulse" style="animation-delay:{delay}s" />
				<circle {x} {y} {r} fill={color} filter="url(#node-glow)"
					class="pulse" style="animation-delay:{delay}s" />
			{:else}
				<!-- Hub node: larger glow -->
				<circle {x} {y} r={r * 4} fill={color} fill-opacity="0.07"
					class="hub-pulse" style="animation-delay:{delay}s" />
				<circle {x} {y} r={r * 2} fill={color} fill-opacity="0.12"
					class="hub-pulse" style="animation-delay:{delay}s" />
				<circle {x} {y} {r} fill={color} filter="url(#hub-glow)"
					class="hub-pulse" style="animation-delay:{delay}s" />
			{/if}
		{/each}
	</svg>

	<!-- Gradient mask so the left logo area reads cleanly -->
	<div
		class="absolute inset-0 pointer-events-none"
		style="background: linear-gradient(to right,
			{darkMode ? '#020617' : '#f8fafc'} 0%,
			{darkMode ? 'rgba(2,6,23,0.92)' : 'rgba(248,250,252,0.92)'} 22%,
			{darkMode ? 'rgba(2,6,23,0.55)' : 'rgba(248,250,252,0.55)'} 40%,
			transparent 62%);"
	></div>

	<!-- Foreground content -->
	<div class="relative z-10 flex h-full w-full items-center justify-between px-6">
		<!-- Logo + Branding -->
		<div class="flex items-center gap-4">
			<div
				class="flex items-center justify-center w-12 h-12 rounded-xl flex-shrink-0"
				style="background:rgba(34,197,94,0.15); border:1px solid rgba(34,197,94,0.35);"
			>
				<BrainCircuitIcon class="w-6 h-6" style="color:#22C55E;" />
			</div>
			<div>
				<div class="flex items-center gap-2 mb-0.5">
					<h1
						class="text-2xl font-bold tracking-tight"
						style="font-family:'Fira Code',monospace; color:{darkMode ? '#F8FAFC' : '#0f172a'};"
					>
						My<span style="color:#22C55E;">AI</span>Assistant
					</h1>
					<span
						class="px-2 py-0.5 text-xs font-medium rounded-full"
						style="background:rgba(34,197,94,0.15); color:#22C55E; border:1px solid rgba(34,197,94,0.3); font-family:'Fira Code',monospace;"
					>v2.0</span>
				</div>
				<p class="text-sm" style="color:{darkMode ? '#64748b' : '#64748b'};">
					Your intelligent workspace — powered by AI
				</p>
				<div class="flex items-center gap-4 mt-2">
					<div class="flex items-center gap-1.5">
						<div class="w-1.5 h-1.5 rounded-full" style="background:#22C55E; animation:ledpulse 2s infinite;"></div>
						<span class="text-xs" style="color:{darkMode ? '#94a3b8' : '#64748b'};">3 agents active</span>
					</div>
					<div class="flex items-center gap-1.5">
						<ZapIcon class="w-3 h-3" style="color:#eab308;" />
						<span class="text-xs" style="color:{darkMode ? '#94a3b8' : '#64748b'};">12 tasks running</span>
					</div>
					<div class="flex items-center gap-1.5">
						<ActivityIcon class="w-3 h-3" style="color:#22C55E;" />
						<span class="text-xs" style="color:{darkMode ? '#94a3b8' : '#64748b'};">All systems nominal</span>
					</div>
				</div>
			</div>
		</div>

		<!-- Right controls -->
		<div class="flex items-center gap-3">
			<div class="hidden lg:flex items-center gap-2">
				<div
					class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs"
					style="background:{darkMode ? 'rgba(30,41,59,0.85)' : 'rgba(226,232,240,0.9)'}; border:1px solid {darkMode ? '#334155' : '#cbd5e1'}; color:{darkMode ? '#94a3b8' : '#475569'}; font-family:'Fira Code',monospace;"
				>
					<ServerIcon class="w-3 h-3" style="color:#22C55E;" />
					Claude Sonnet 4.6
				</div>
				<div
					class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs"
					style="background:{darkMode ? 'rgba(30,41,59,0.85)' : 'rgba(226,232,240,0.9)'}; border:1px solid {darkMode ? '#334155' : '#cbd5e1'}; color:{darkMode ? '#94a3b8' : '#475569'}; font-family:'Fira Code',monospace;"
				>
					<ServerIcon class="w-3 h-3" style="color:#818cf8;" />
					GPT-4o
				</div>
			</div>

			<!-- Activity indicator -->
			<div
				class="relative flex items-center justify-center w-9 h-9 rounded-lg"
				style="background:{darkMode ? 'rgba(30,41,59,0.85)' : 'rgba(226,232,240,0.9)'}; border:1px solid {darkMode ? '#334155' : '#cbd5e1'};"
			>
				<ActivityIcon class="w-4 h-4" style="color:{darkMode ? '#94a3b8' : '#64748b'};" />
				<div class="absolute top-1.5 right-1.5 w-1.5 h-1.5 rounded-full" style="background:#22C55E; animation:ledpulse 2s infinite;"></div>
			</div>

			<!-- Dark / light toggle -->
			<button
				onclick={onToggleDark}
				class="flex items-center justify-center w-9 h-9 rounded-lg cursor-pointer transition-colors duration-200"
				style="background:{darkMode ? 'rgba(30,41,59,0.85)' : 'rgba(226,232,240,0.9)'}; border:1px solid {darkMode ? '#334155' : '#cbd5e1'};"
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
</header>

<style>
	/* Data packet flowing along path — one small dot per path */
	@keyframes dashflow {
		from { stroke-dashoffset: 614; }
		to   { stroke-dashoffset: 0; }
	}
	.flow { animation: dashflow linear infinite; }
	.flow1 { animation-duration: 3.2s; }
	.flow2 { animation-duration: 4.5s; animation-delay: 0.9s; }
	.flow3 { animation-duration: 5.8s; animation-delay: 2.1s; }

	/* Node pulse — fade in/out */
	@keyframes nodepulse {
		0%, 100% { opacity: 0.9; }
		50%       { opacity: 0.4; }
	}
	.pulse { animation: nodepulse 3s ease-in-out infinite; }

	/* Hub nodes breathe a little more dramatically */
	@keyframes hubpulse {
		0%, 100% { opacity: 1; }
		50%       { opacity: 0.55; }
	}
	.hub-pulse { animation: hubpulse 2.2s ease-in-out infinite; }

	/* LED status dot */
	@keyframes ledpulse {
		0%, 100% { opacity: 1; }
		50%       { opacity: 0.25; }
	}
</style>
