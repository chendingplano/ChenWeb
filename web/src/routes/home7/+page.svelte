<svelte:head>
	<title>Signal Forge | AI Application and Knowledge System</title>
	<meta
		name="description"
		content="Build AI-enabled applications with a vibe-first workflow and manage your personal knowledge base in one connected system."
	/>
</svelte:head>

<div class="page">
	<div class="bg-layer bg-grid"></div>
	<div class="bg-layer bg-radial"></div>
	<div class="noise"></div>

	<header class="topbar">
		<div class="brand">
			<div class="brand-mark">SF</div>
			<span>Signal Forge</span>
		</div>
		<nav>
			<a href="#engines">Engines</a>
			<a href="#workflow">Workflow</a>
			<a href="#features">Features</a>
		</nav>
		<a class="btn btn-ghost" href="#cta">Start Building</a>
	</header>

	<main>
		<section class="hero">
			<p class="eyebrow">AI-native creator system</p>
			<h1>
				Build useful AI apps with momentum.<br />
				Grow a knowledge base that compounds.
			</h1>
			<p class="hero-copy">
				For builders who want more than prompts: orchestrate AI, ship vibe-coded applications,
				and keep every insight searchable, linked, and reusable.
			</p>
			<div class="hero-actions">
				<a class="btn btn-primary" href="#cta">Launch Workspace</a>
				<a class="btn btn-outline" href="#workflow">See How It Works</a>
			</div>
			<div class="hero-metrics">
				<div><span>4.3x</span><p>faster prototype cycles</p></div>
				<div><span>96%</span><p>knowledge retrieval accuracy</p></div>
				<div><span>24/7</span><p>agent-assisted ideation</p></div>
			</div>
		</section>

		<section id="engines" class="section">
			<div class="section-head">
				<p class="eyebrow">Three connected engines</p>
				<h2>One system for app creation and knowledge mastery</h2>
			</div>
			<div class="engine-grid">
				<article class="card glow-cyan">
					<h3>AI Orchestration</h3>
					<p>
						Route tasks across models, tools, and data sources with predictable controls and
						auditable outcomes.
					</p>
				</article>
				<article class="card glow-lime">
					<h3>Vibe App Builder</h3>
					<p>
						Translate intent into full interfaces and backend logic, then refine with real-time
						feedback loops.
					</p>
				</article>
				<article class="card glow-amber">
					<h3>Personal Knowledge Base</h3>
					<p>
						Capture notes, docs, and decisions into linked memory so every new project starts with
						context.
					</p>
				</article>
			</div>
		</section>

		<section id="workflow" class="section workflow">
			<div class="section-head">
				<p class="eyebrow">Workflow</p>
				<h2>From idea to deployed AI experience</h2>
			</div>
			<div class="steps">
				<div class="step">
					<span>01</span>
					<h4>Capture intent</h4>
					<p>Define outcomes, constraints, and user signals in a structured brief.</p>
				</div>
				<div class="step">
					<span>02</span>
					<h4>Compose agents</h4>
					<p>Assemble specialized AI workers and tool chains for coding, testing, and docs.</p>
				</div>
				<div class="step">
					<span>03</span>
					<h4>Ship and learn</h4>
					<p>Deploy, observe usage patterns, and feed insights back into your knowledge graph.</p>
				</div>
			</div>
		</section>

		<section id="features" class="section features">
			<div class="feature-left card">
				<p class="eyebrow">Built for serious builders</p>
				<h3>Everything needed to build AI-enabled products without losing context</h3>
				<ul>
					<li>Model routing with cost and quality policies</li>
					<li>Prompt, code, and docs unified in one workspace</li>
					<li>Knowledge graph with semantic search and source tracing</li>
					<li>Reusable templates for rapid app iteration</li>
				</ul>
			</div>
			<div class="feature-right">
				<div class="mini card">
					<h4>Context Memory</h4>
					<p>Every project artifact indexed and linked for retrieval at build time.</p>
				</div>
				<div class="mini card">
					<h4>Vibe Loop</h4>
					<p>Describe, generate, test, and tune with tight creative feedback cycles.</p>
				</div>
				<div class="mini card">
					<h4>Deployment Paths</h4>
					<p>Export to API, web app, or internal tool with one consistent pipeline.</p>
				</div>
			</div>
		</section>

		<section class="social-proof section">
			<p>Trusted by indie builders, product teams, and research engineers.</p>
			<div class="proof-row">
				<span>Iterative AI Studio</span><span>ContextOps Labs</span><span>Makers Guild</span
				><span>Next Prototype Co.</span>
			</div>
		</section>

		<section id="cta" class="section cta">
			<h2>Turn scattered ideas into AI products and lasting knowledge</h2>
			<p>Start with one workflow. End with a system that scales with your thinking.</p>
			<a class="btn btn-primary" href="/registration-page-app">Create Your AI Workspace</a>
		</section>
	</main>
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=Syne:wght@500;700;800&family=Manrope:wght@400;500;600;700&display=swap');

	:global(body) {
		margin: 0;
		font-family: 'Manrope', sans-serif;
		background: #0d1117;
	}

	.page {
		--bg: #0d1117;
		--surface: #131a24;
		--surface-2: #182230;
		--text: #edf3ff;
		--muted: #8fa3bd;
		--cyan: #3ce5ff;
		--lime: #b7ff66;
		--amber: #ffce73;
		position: relative;
		isolation: isolate;
		min-height: 100vh;
		overflow: hidden;
		color: var(--text);
		background: var(--bg);
	}

	.bg-layer {
		position: absolute;
		inset: 0;
		pointer-events: none;
		z-index: -3;
	}

	.bg-grid {
		background-image:
			linear-gradient(rgba(111, 127, 150, 0.1) 1px, transparent 1px),
			linear-gradient(90deg, rgba(111, 127, 150, 0.1) 1px, transparent 1px);
		background-size: 40px 40px;
		opacity: 0.25;
	}

	.bg-radial {
		background:
			radial-gradient(circle at 20% 10%, rgba(60, 229, 255, 0.2), transparent 35%),
			radial-gradient(circle at 80% 15%, rgba(183, 255, 102, 0.18), transparent 35%),
			radial-gradient(circle at 70% 80%, rgba(255, 206, 115, 0.14), transparent 40%);
	}

	.noise {
		position: absolute;
		inset: 0;
		pointer-events: none;
		background-image: radial-gradient(rgba(255, 255, 255, 0.06) 0.5px, transparent 0.5px);
		background-size: 2px 2px;
		opacity: 0.08;
		z-index: -2;
	}

	.topbar {
		position: sticky;
		top: 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 1rem clamp(1rem, 3vw, 3rem);
		border-bottom: 1px solid rgba(146, 165, 191, 0.2);
		background: rgba(13, 17, 23, 0.72);
		backdrop-filter: blur(10px);
		z-index: 10;
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		font-family: 'Syne', sans-serif;
		font-weight: 700;
		letter-spacing: 0.03em;
	}

	.brand-mark {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		border-radius: 0.45rem;
		font-size: 0.75rem;
		color: #091018;
		background: linear-gradient(135deg, var(--cyan), var(--lime));
	}

	nav {
		display: flex;
		gap: 1.2rem;
	}

	nav a {
		color: var(--muted);
		text-decoration: none;
		font-weight: 600;
		font-size: 0.92rem;
	}

	nav a:hover {
		color: var(--text);
	}

	main {
		width: min(1100px, 92%);
		margin: 0 auto;
		padding: 2rem 0 5rem;
	}

	.hero {
		padding-top: clamp(2.4rem, 6vw, 6rem);
		animation: reveal 0.7s ease-out;
	}

	.eyebrow {
		display: inline-block;
		padding: 0.3rem 0.65rem;
		border-radius: 999px;
		border: 1px solid rgba(143, 163, 189, 0.35);
		color: var(--muted);
		font-size: 0.8rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	h1,
	h2,
	h3,
	h4 {
		font-family: 'Syne', sans-serif;
		margin: 0;
	}

	h1 {
		margin-top: 1rem;
		font-size: clamp(2rem, 6vw, 4.3rem);
		line-height: 1.04;
		max-width: 15ch;
		text-wrap: balance;
	}

	.hero-copy {
		margin-top: 1.2rem;
		color: var(--muted);
		font-size: clamp(1rem, 1.8vw, 1.2rem);
		max-width: 64ch;
	}

	.hero-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.9rem;
		margin-top: 1.7rem;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0.75rem 1.1rem;
		border-radius: 0.7rem;
		font-weight: 700;
		text-decoration: none;
		transition: all 0.2s ease;
	}

	.btn-primary {
		background: linear-gradient(135deg, var(--cyan), var(--lime));
		color: #08101a;
	}

	.btn-primary:hover {
		transform: translateY(-1px);
		filter: brightness(1.06);
	}

	.btn-outline {
		border: 1px solid rgba(143, 163, 189, 0.45);
		color: var(--text);
	}

	.btn-outline:hover {
		border-color: rgba(237, 243, 255, 0.65);
	}

	.btn-ghost {
		border: 1px solid rgba(143, 163, 189, 0.35);
		color: var(--text);
		padding: 0.55rem 0.8rem;
	}

	.btn-ghost:hover {
		border-color: rgba(237, 243, 255, 0.68);
	}

	.hero-metrics {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.85rem;
		margin-top: 2rem;
	}

	.hero-metrics > div {
		border: 1px solid rgba(146, 165, 191, 0.28);
		background: rgba(19, 26, 36, 0.72);
		padding: 0.9rem;
		border-radius: 0.8rem;
	}

	.hero-metrics span {
		display: block;
		font-family: 'Syne', sans-serif;
		font-size: clamp(1.2rem, 3.5vw, 2rem);
	}

	.hero-metrics p {
		margin: 0.25rem 0 0;
		font-size: 0.9rem;
		color: var(--muted);
	}

	.section {
		margin-top: clamp(3rem, 7vw, 6.5rem);
	}

	.section-head {
		margin-bottom: 1.4rem;
	}

	.section-head h2 {
		margin-top: 0.7rem;
		font-size: clamp(1.35rem, 3.5vw, 2.5rem);
		max-width: 22ch;
	}

	.engine-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1rem;
	}

	.card {
		position: relative;
		border: 1px solid rgba(146, 165, 191, 0.24);
		background: linear-gradient(160deg, rgba(24, 34, 48, 0.88), rgba(19, 26, 36, 0.86));
		border-radius: 1rem;
		padding: 1.1rem;
		overflow: hidden;
	}

	.card h3 {
		font-size: 1.2rem;
	}

	.card p {
		margin: 0.65rem 0 0;
		color: var(--muted);
		line-height: 1.55;
	}

	.glow-cyan::after,
	.glow-lime::after,
	.glow-amber::after {
		content: '';
		position: absolute;
		inset: -25% auto auto -20%;
		width: 10rem;
		height: 10rem;
		filter: blur(32px);
		opacity: 0.28;
	}

	.glow-cyan::after {
		background: var(--cyan);
	}

	.glow-lime::after {
		background: var(--lime);
	}

	.glow-amber::after {
		background: var(--amber);
	}

	.workflow .steps {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.9rem;
	}

	.step {
		padding: 1rem;
		border: 1px dashed rgba(146, 165, 191, 0.34);
		border-radius: 0.9rem;
		background: rgba(19, 26, 36, 0.45);
	}

	.step span {
		font-family: 'Syne', sans-serif;
		color: var(--cyan);
		font-size: 0.9rem;
	}

	.step h4 {
		margin-top: 0.45rem;
		font-size: 1.05rem;
	}

	.step p {
		margin: 0.45rem 0 0;
		font-size: 0.95rem;
		color: var(--muted);
	}

	.features {
		display: grid;
		grid-template-columns: 1.3fr 1fr;
		gap: 1rem;
	}

	.feature-left h3 {
		font-size: clamp(1.2rem, 2.6vw, 1.8rem);
		margin-top: 0.7rem;
	}

	.feature-left ul {
		list-style: none;
		padding: 0;
		margin: 1rem 0 0;
		display: grid;
		gap: 0.55rem;
	}

	.feature-left li {
		padding: 0.56rem 0.7rem;
		border-radius: 0.58rem;
		color: #dce8ff;
		background: rgba(143, 163, 189, 0.12);
	}

	.feature-right {
		display: grid;
		gap: 0.75rem;
	}

	.mini h4 {
		font-size: 1rem;
	}

	.mini p {
		font-size: 0.92rem;
	}

	.social-proof {
		padding: 1rem;
		border: 1px solid rgba(146, 165, 191, 0.2);
		border-radius: 0.95rem;
		background: rgba(19, 26, 36, 0.65);
	}

	.social-proof p {
		margin: 0;
		color: var(--muted);
	}

	.proof-row {
		margin-top: 0.7rem;
		display: flex;
		flex-wrap: wrap;
		gap: 0.7rem;
	}

	.proof-row span {
		padding: 0.35rem 0.6rem;
		font-size: 0.85rem;
		border-radius: 999px;
		border: 1px solid rgba(143, 163, 189, 0.35);
	}

	.cta {
		text-align: center;
		padding: clamp(1.2rem, 4vw, 2.1rem);
		border-radius: 1rem;
		border: 1px solid rgba(146, 165, 191, 0.28);
		background:
			linear-gradient(130deg, rgba(60, 229, 255, 0.12), rgba(183, 255, 102, 0.09)),
			rgba(19, 26, 36, 0.72);
	}

	.cta h2 {
		font-size: clamp(1.3rem, 3.5vw, 2.25rem);
		max-width: 25ch;
		margin: 0 auto;
	}

	.cta p {
		margin: 0.75rem auto 1rem;
		color: var(--muted);
		max-width: 50ch;
	}

	@keyframes reveal {
		from {
			opacity: 0;
			transform: translateY(15px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	@media (max-width: 960px) {
		nav {
			display: none;
		}

		.engine-grid,
		.workflow .steps,
		.hero-metrics {
			grid-template-columns: 1fr;
		}

		.features {
			grid-template-columns: 1fr;
		}
	}

	@media (max-width: 640px) {
		main {
			width: min(1100px, 94%);
		}

		h1 {
			max-width: 100%;
		}

		.hero-copy {
			font-size: 0.98rem;
		}

		.btn {
			width: 100%;
		}

		.hero-actions {
			width: 100%;
		}
	}
</style>
