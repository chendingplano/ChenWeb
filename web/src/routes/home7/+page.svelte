<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
</script>

<svelte:head>
	<title>{m.home7_signal_forge_ai_application_and()}</title>
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
			<div class="brand-mark">{m.home7_sf()}</div>
			<span>{m.home7_signal_forge()}</span>
		</div>
		<nav>
			<a href="#engines">{m.home7_engines()}</a>
			<a href="#workflow">{m.home7_workflow()}</a>
			<a href="#features">{m.home7_features()}</a>
		</nav>
		<a class="btn btn-ghost" href="#cta">{m.home7_start_building()}</a>
	</header>

	<main>
		<section class="hero">
			<p class="eyebrow">{m.home7_ai_native_creator_system()}</p>
			<h1>
				{m.home7_build_useful_ai_apps_with()}<br />
				{m.home7_grow_a_knowledge_base_that()}
			</h1>
			<p class="hero-copy">
				{m.home7_for_builders_who_want_more()}
			</p>
			<div class="hero-actions">
				<a class="btn btn-primary" href="#cta">{m.home7_launch_workspace()}</a>
				<a class="btn btn-outline" href="#workflow">{m.home7_see_how_it_works()}</a>
			</div>
			<div class="hero-metrics">
				<div><span>4.3x</span><p>{m.home7_faster_prototype_cycles()}</p></div>
				<div><span>96%</span><p>{m.home7_knowledge_retrieval_accuracy()}</p></div>
				<div><span>24/7</span><p>{m.home7_agent_assisted_ideation()}</p></div>
			</div>
		</section>

		<section id="engines" class="section">
			<div class="section-head">
				<p class="eyebrow">{m.home7_three_connected_engines()}</p>
				<h2>{m.home7_one_system_for_app_creation()}</h2>
			</div>
			<div class="engine-grid">
				<article class="card glow-cyan">
					<h3>{m.home7_ai_orchestration()}</h3>
					<p>
						{m.home7_route_tasks_across_models_tools()}
					</p>
				</article>
				<article class="card glow-lime">
					<h3>{m.home7_vibe_app_builder()}</h3>
					<p>
						{m.home7_translate_intent_into_full_interfaces()}
					</p>
				</article>
				<article class="card glow-amber">
					<h3>{m.home7_personal_knowledge_base()}</h3>
					<p>
						{m.home7_capture_notes_docs_and_decisions()}
					</p>
				</article>
			</div>
		</section>

		<section id="workflow" class="section workflow">
			<div class="section-head">
				<p class="eyebrow">{m.home7_workflow()}</p>
				<h2>{m.home7_from_idea_to_deployed_ai()}</h2>
			</div>
			<div class="steps">
				<div class="step">
					<span>01</span>
					<h4>{m.home7_capture_intent()}</h4>
					<p>{m.home7_define_outcomes_constraints_and_user()}</p>
				</div>
				<div class="step">
					<span>02</span>
					<h4>{m.home7_compose_agents()}</h4>
					<p>{m.home7_assemble_specialized_ai_workers_and()}</p>
				</div>
				<div class="step">
					<span>03</span>
					<h4>{m.home7_ship_and_learn()}</h4>
					<p>{m.home7_deploy_observe_usage_patterns_and()}</p>
				</div>
			</div>
		</section>

		<section id="features" class="section features">
			<div class="feature-left card">
				<p class="eyebrow">{m.home7_built_for_serious_builders()}</p>
				<h3>{m.home7_everything_needed_to_build_ai()}</h3>
				<ul>
					<li>{m.home7_model_routing_with_cost_and()}</li>
					<li>{m.home7_prompt_code_and_docs_unified()}</li>
					<li>{m.home7_knowledge_graph_with_semantic_search()}</li>
					<li>{m.home7_reusable_templates_for_rapid_app()}</li>
				</ul>
			</div>
			<div class="feature-right">
				<div class="mini card">
					<h4>{m.home7_context_memory()}</h4>
					<p>{m.home7_every_project_artifact_indexed_and()}</p>
				</div>
				<div class="mini card">
					<h4>{m.home7_vibe_loop()}</h4>
					<p>{m.home7_describe_generate_test_and_tune()}</p>
				</div>
				<div class="mini card">
					<h4>{m.home7_deployment_paths()}</h4>
					<p>{m.home7_export_to_api_web_app()}</p>
				</div>
			</div>
		</section>

		<section class="social-proof section">
			<p>{m.home7_trusted_by_indie_builders_product()}</p>
			<div class="proof-row">
				<span>{m.home7_iterative_ai_studio()}</span><span>{m.home7_contextops_labs()}</span><span>{m.home7_makers_guild()}</span
				><span>{m.home7_next_prototype_co()}</span>
			</div>
		</section>

		<section id="cta" class="section cta">
			<h2>{m.home7_turn_scattered_ideas_into_ai()}</h2>
			<p>{m.home7_start_with_one_workflow_end()}</p>
			<a class="btn btn-primary" href="/login">{m.home7_create_your_ai_workspace()}</a>
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
