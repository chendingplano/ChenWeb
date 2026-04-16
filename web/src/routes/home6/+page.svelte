<script lang="ts">
  import { onMount } from 'svelte';

  let mouseX = $state(0);
  let mouseY = $state(0);
  let scrolled = $state(0);

  onMount(() => {
    const onMove = (e: MouseEvent) => {
      mouseX = (e.clientX / window.innerWidth - 0.5) * 2;
      mouseY = (e.clientY / window.innerHeight - 0.5) * 2;
    };
    const onScroll = () => { scrolled = window.scrollY; };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('scroll', onScroll);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('scroll', onScroll);
    };
  });

  const catalog = [
    { no: '001.A', title: 'Vibe Compiler', tag: 'Runtime',
      body: 'Sketch an application in plain prose. The atelier translates intent into typed modules, data contracts, and deployable binaries — without losing the hand of the author.',
      meta: 'est. 2024 · rev. 14' },
    { no: '002.B', title: 'Card Catalog', tag: 'Memory',
      body: 'Every note, paper, transcript, and half-formed idea is cross-referenced by concept, not folder. Your knowledge retrieves itself when the question arrives.',
      meta: 'index: 48,201' },
    { no: '003.C', title: 'Resident Agents', tag: 'Studio',
      body: 'A small troupe of specialized agents live inside the workshop — a librarian, an engineer, a critic, an archivist. They confer before they act.',
      meta: 'ensemble: 7' },
    { no: '004.D', title: 'Press & Publish', tag: 'Output',
      body: 'Ship an internal tool by lunch. Print a monograph by dinner. The same workshop that thinks for you also sets the type and binds the spine.',
      meta: 'throughput: ∞' }
  ];

  const marquee = 'AN ATELIER FOR THINKING MACHINES  ·  VIBE × KNOWLEDGE × CRAFT  ·  EST. MMXXIV  ·  ';

  const principles = [
    { n: '§I',   t: 'Intention over configuration', b: 'You describe the thing. The thing describes itself. No YAML priesthood.' },
    { n: '§II',  t: 'Memory is a material',          b: 'Your notes are not storage — they are stock, grain, and pigment the agents compose with.' },
    { n: '§III', t: 'Small tools, tightly joined',   b: 'We reject the monolith. Each module is a chisel; the workshop is the hand.' },
    { n: '§IV',  t: 'The author is present',         b: 'Nothing ships without your stamp. Automation amplifies taste; it never replaces it.' }
  ];
</script>

<svelte:head>
  <title>Foundry — An Atelier for Thinking Machines</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
  <link
    href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght,SOFT,WONK@9..144,300..900,0..100,0..1&family=Instrument+Sans:ital,wght@0,400..700;1,400..700&family=JetBrains+Mono:wght@400;600&display=swap"
    rel="stylesheet"
  />
</svelte:head>

<div class="page" style="--mx:{mouseX}; --my:{mouseY}; --sy:{scrolled}px">
  <div class="grain"></div>
  <div class="rules"></div>

  <!-- MASTHEAD -->
  <header class="mast">
    <div class="mast-row">
      <div class="seal">
        <svg viewBox="0 0 80 80" aria-hidden="true">
          <circle cx="40" cy="40" r="38" fill="none" stroke="currentColor" stroke-width="1.2"/>
          <circle cx="40" cy="40" r="30" fill="none" stroke="currentColor" stroke-width="0.6"/>
          <path id="arc" d="M 12 40 A 28 28 0 1 1 68 40" fill="none"/>
          <text font-family="JetBrains Mono" font-size="7" letter-spacing="2">
            <textPath href="#arc">FOUNDRY · MMXXIV · ATELIER</textPath>
          </text>
          <text x="40" y="46" text-anchor="middle" font-family="Fraunces" font-size="22" font-style="italic" font-weight="400">ƒ</text>
        </svg>
      </div>
      <div class="mast-title">
        <span class="overline">Vol. I · No. 6</span>
        <h1 class="wordmark">Foundry</h1>
        <span class="tagline">An atelier for thinking machines</span>
      </div>
      <nav class="mast-nav">
        <a href="#catalog">Catalog</a>
        <a href="#principles">Principles</a>
        <a href="#press">Press Room</a>
        <a href="#visit" class="cta">Visit the workshop →</a>
      </nav>
    </div>
    <div class="mast-rule"></div>
    <div class="mast-meta">
      <span>Printed in the key of cobalt &amp; coral</span>
      <span class="dot">●</span>
      <span>Issue reset daily at dawn</span>
      <span class="dot">●</span>
      <span>For authors who refuse to outsource taste</span>
    </div>
  </header>

  <!-- HERO -->
  <section class="hero">
    <div class="hero-grid">
      <div class="hero-left">
        <div class="folio">Folio 01 / The Thesis</div>
        <h2 class="headline">
          Build the app<br/>
          <em>and</em> the library<br/>
          <span class="accent">in the same breath.</span>
        </h2>
        <p class="lede">
          Foundry is a private workshop where prose becomes software and notes
          become instinct. A single surface for vibe-coding AI applications and
          tending the knowledge that feeds them — so the thing you make and the
          thing you know stop living in different rooms.
        </p>
        <div class="hero-actions">
          <a class="btn primary" href="#visit">
            <span>Open the workshop</span>
            <svg viewBox="0 0 24 12" aria-hidden="true"><path d="M0 6h22M16 0l6 6-6 6" fill="none" stroke="currentColor" stroke-width="1.6"/></svg>
          </a>
          <a class="btn ghost" href="#catalog">Read the catalog</a>
        </div>
        <div class="hero-stats">
          <div><b>12ms</b><span>prose → prototype</span></div>
          <div><b>48k+</b><span>notes indexed live</span></div>
          <div><b>7</b><span>resident agents</span></div>
        </div>
      </div>

      <aside class="hero-right">
        <div class="card-specimen" style="transform: translate3d(calc(var(--mx)*6px), calc(var(--my)*6px), 0)">
          <div class="card-head">
            <span class="card-no">№ 006 · A</span>
            <span class="card-stamp">APPROVED</span>
          </div>
          <div class="card-art">
            <svg viewBox="0 0 300 200" aria-hidden="true">
              <defs>
                <pattern id="dots" width="8" height="8" patternUnits="userSpaceOnUse">
                  <circle cx="1" cy="1" r="1" fill="currentColor" opacity="0.35"/>
                </pattern>
              </defs>
              <rect width="300" height="200" fill="url(#dots)"/>
              <circle cx="100" cy="100" r="62" fill="none" stroke="var(--coral)" stroke-width="1.5"/>
              <circle cx="170" cy="100" r="62" fill="none" stroke="var(--cobalt)" stroke-width="1.5"/>
              <circle cx="135" cy="100" r="4" fill="var(--ink)"/>
              <text x="100" y="40" font-family="JetBrains Mono" font-size="9" fill="var(--coral)">KNOWLEDGE</text>
              <text x="170" y="172" font-family="JetBrains Mono" font-size="9" fill="var(--cobalt)" text-anchor="end">APPLICATION</text>
              <text x="135" y="115" font-family="Fraunces" font-style="italic" font-size="13" text-anchor="middle" fill="var(--ink)">foundry</text>
            </svg>
          </div>
          <div class="card-rows">
            <div><span>Subject</span><span>Vibe development · PKM</span></div>
            <div><span>Class</span><span>Workshop software</span></div>
            <div><span>Shelf</span><span>Studio / Atelier / 006</span></div>
            <div><span>Author</span><span>You, principally</span></div>
          </div>
          <div class="card-barcode">
            {#each Array(38) as _, i}
              <span style="--h:{40 + ((i * 13) % 30)}px; --w:{1 + ((i * 7) % 3)}px"></span>
            {/each}
          </div>
        </div>

        <div class="sticky-note">
          <svg class="pin" viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="5" fill="var(--coral)"/><circle cx="10" cy="10" r="2" fill="#5a1e0e"/></svg>
          <p>“Finally — one room where the <u>making</u> and the <u>remembering</u> share a desk.”</p>
          <span>— resident architect, week 3</span>
        </div>
      </aside>
    </div>

    <div class="marquee" aria-hidden="true">
      <div class="marquee-track">
        {#each Array(6) as _}<span>{marquee}</span>{/each}
      </div>
    </div>
  </section>

  <!-- CATALOG -->
  <section class="catalog" id="catalog">
    <div class="section-head">
      <span class="section-no">§ 02</span>
      <h3>The card catalog</h3>
      <p>Four instruments currently on the bench. Pull a drawer.</p>
    </div>

    <div class="catalog-grid">
      {#each catalog as c, i}
        <article class="cat-card" style="--i:{i}">
          <header>
            <span class="cat-no">{c.no}</span>
            <span class="cat-tag">{c.tag}</span>
          </header>
          <h4>{c.title}</h4>
          <p>{c.body}</p>
          <footer>
            <span class="meta">{c.meta}</span>
            <span class="arrow">→</span>
          </footer>
        </article>
      {/each}
    </div>
  </section>

  <!-- PRINCIPLES -->
  <section class="principles" id="principles">
    <div class="section-head alt">
      <span class="section-no">§ 03</span>
      <h3>The manifesto,<br/><em>four sentences long.</em></h3>
    </div>
    <ol class="principles-list">
      {#each principles as p}
        <li>
          <span class="pn">{p.n}</span>
          <div>
            <h5>{p.t}</h5>
            <p>{p.b}</p>
          </div>
        </li>
      {/each}
    </ol>
  </section>

  <!-- PRESS ROOM / CTA -->
  <section class="press" id="press">
    <div class="press-inner">
      <div class="press-left">
        <span class="section-no light">§ 04</span>
        <h3>Press room</h3>
        <p>
          Foundry is in limited residency. We seat authors in small cohorts
          so the workshop stays quiet enough to hear yourself think. If the
          above reads like a room you’d like to work in, knock.
        </p>
      </div>
      <form class="press-form" id="visit" on:submit|preventDefault={() => {}}>
        <label>
          <span>Name of author</span>
          <input type="text" placeholder="Jane Q. Maker" />
        </label>
        <label>
          <span>Address for correspondence</span>
          <input type="email" placeholder="you@studio.co" />
        </label>
        <label>
          <span>What are you trying to make?</span>
          <textarea rows="3" placeholder="An app that reads my field notes and drafts tomorrow's experiments..."></textarea>
        </label>
        <button type="submit">
          <span>Request the key</span>
          <svg viewBox="0 0 24 12" aria-hidden="true"><path d="M0 6h22M16 0l6 6-6 6" fill="none" stroke="currentColor" stroke-width="1.8"/></svg>
        </button>
      </form>
    </div>
  </section>

  <!-- COLOPHON -->
  <footer class="colophon">
    <div class="col-row">
      <span class="wordmark sm">Foundry</span>
      <span>An atelier for thinking machines</span>
      <span>© MMXXIV</span>
    </div>
    <div class="col-row fine">
      <span>Set in Fraunces, Instrument Sans &amp; JetBrains Mono.</span>
      <span>Printed on recycled bits.</span>
      <span>No slop was used in the making of this page.</span>
    </div>
  </footer>
</div>

<style>
  :global(html, body) {
    margin: 0;
    padding: 0;
    background: #f3ece0;
  }

  .page {
    --paper: #f3ece0;
    --paper-2: #ebe1cf;
    --ink: #141210;
    --ink-2: #2a2620;
    --coral: #e5431f;
    --cobalt: #1a3aa8;
    --rule: rgba(20, 18, 16, 0.18);

    position: relative;
    min-height: 100vh;
    color: var(--ink);
    background: var(--paper);
    font-family: 'Instrument Sans', -apple-system, sans-serif;
    font-size: 16px;
    line-height: 1.55;
    overflow-x: hidden;
  }

  .grain {
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 100;
    mix-blend-mode: multiply;
    opacity: 0.32;
    background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0.6 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  }
  .rules {
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 1;
    background-image: linear-gradient(var(--rule) 1px, transparent 1px);
    background-size: 100% 32px;
    background-position: 0 80px;
    opacity: 0.22;
    mask-image: linear-gradient(180deg, #000 0%, #000 60%, transparent 100%);
  }

  .page :global(::selection) { background: var(--coral); color: var(--paper); }

  .overline, .section-no, .card-no, .cat-no, .cat-tag, .card-stamp, .tagline, .col-row.fine, .mast-meta {
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    letter-spacing: 0.14em;
    text-transform: uppercase;
  }

  h1, h2, h3, h4, h5 {
    font-family: 'Fraunces', serif;
    font-weight: 400;
    font-variation-settings: 'opsz' 144, 'SOFT' 30, 'WONK' 1;
    letter-spacing: -0.02em;
    margin: 0;
  }

  em { font-style: italic; font-variation-settings: 'opsz' 144, 'SOFT' 80, 'WONK' 1; }
  .accent { color: var(--coral); font-style: italic; }

  /* MASTHEAD */
  .mast { position: relative; z-index: 2; padding: 28px clamp(24px, 5vw, 72px) 0; }
  .mast-row { display: grid; grid-template-columns: 80px 1fr auto; align-items: center; gap: 24px; }
  .seal { width: 80px; height: 80px; color: var(--ink); }
  .seal svg { width: 100%; height: 100%; animation: spin 60s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .mast-title { display: flex; flex-direction: column; align-items: center; gap: 2px; }
  .overline { color: var(--ink-2); }
  .wordmark { font-size: clamp(44px, 6vw, 72px); line-height: 0.9; font-variation-settings: 'opsz' 144, 'SOFT' 20, 'WONK' 1; }
  .tagline { color: var(--ink-2); font-size: 10px; }

  .mast-nav { display: flex; gap: 26px; align-items: center; }
  .mast-nav a { color: var(--ink); text-decoration: none; font-size: 14px; position: relative; padding-bottom: 2px; }
  .mast-nav a:not(.cta)::after {
    content: ''; position: absolute; left: 0; right: 100%; bottom: 0;
    height: 2px; background: var(--coral);
    transition: right 0.35s cubic-bezier(.2,.8,.2,1);
  }
  .mast-nav a:not(.cta):hover::after { right: 0; }
  .mast-nav .cta { background: var(--ink); color: var(--paper); padding: 10px 16px; border-radius: 999px; font-size: 13px; }
  .mast-nav .cta:hover { background: var(--coral); }

  .mast-rule {
    margin-top: 20px;
    height: 4px;
    background:
      linear-gradient(var(--ink), var(--ink)) 0 0/100% 1px no-repeat,
      linear-gradient(var(--ink), var(--ink)) 0 100%/100% 1px no-repeat;
  }
  .mast-meta { display: flex; gap: 14px; justify-content: center; align-items: center; padding: 10px 0 0; color: var(--ink-2); }
  .mast-meta .dot { color: var(--coral); font-size: 8px; }

  /* HERO */
  .hero { position: relative; z-index: 2; padding: clamp(48px, 8vw, 120px) clamp(24px, 5vw, 72px) 0; }
  .hero-grid { display: grid; grid-template-columns: 1.2fr 1fr; gap: clamp(32px, 5vw, 80px); align-items: start; }
  .folio {
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px; letter-spacing: 0.14em; text-transform: uppercase;
    color: var(--cobalt); margin-bottom: 24px;
  }
  .folio::before { content: '— '; }

  .headline {
    font-size: clamp(56px, 9vw, 132px);
    line-height: 0.92;
    letter-spacing: -0.035em;
    font-variation-settings: 'opsz' 144, 'SOFT' 20, 'WONK' 1;
    margin-bottom: 28px;
    animation: rise 1s cubic-bezier(.2,.8,.2,1) both;
  }
  .headline em { display: inline-block; transform: translateX(-6px); }
  @keyframes rise {
    from { opacity: 0; transform: translateY(24px); filter: blur(6px); }
    to   { opacity: 1; transform: none; filter: none; }
  }

  .lede {
    font-size: clamp(16px, 1.3vw, 19px);
    max-width: 52ch; color: var(--ink-2);
    margin: 0 0 36px;
    animation: rise 1s 0.15s cubic-bezier(.2,.8,.2,1) both;
  }

  .hero-actions { display: flex; gap: 14px; align-items: center; margin-bottom: 48px; }
  .btn {
    display: inline-flex; gap: 12px; align-items: center;
    padding: 14px 22px; border-radius: 999px; font-size: 14px;
    text-decoration: none; font-weight: 500;
    transition: transform 0.3s, background 0.3s, color 0.3s;
  }
  .btn svg { width: 22px; height: 11px; }
  .btn.primary { background: var(--ink); color: var(--paper); }
  .btn.primary:hover { background: var(--coral); transform: translateX(4px); }
  .btn.ghost { color: var(--ink); border-bottom: 1px solid var(--ink); border-radius: 0; padding: 14px 4px; }
  .btn.ghost:hover { color: var(--coral); border-color: var(--coral); }

  .hero-stats {
    display: flex; gap: 48px;
    padding-top: 24px; border-top: 1px solid var(--rule);
    max-width: 620px;
  }
  .hero-stats div { display: flex; flex-direction: column; gap: 4px; }
  .hero-stats b {
    font-family: 'Fraunces', serif; font-weight: 400;
    font-size: 38px;
    font-variation-settings: 'opsz' 144, 'SOFT' 30, 'WONK' 1;
    line-height: 1;
  }
  .hero-stats span {
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
    color: var(--ink-2);
  }

  .hero-right { position: relative; padding-top: 20px; }
  .card-specimen {
    background: var(--paper-2);
    border: 1px solid var(--ink);
    box-shadow: 12px 14px 0 var(--ink);
    padding: 22px 24px;
    transform-origin: center;
    transition: transform 0.15s ease-out;
  }
  .card-head {
    display: flex; justify-content: space-between; align-items: center;
    padding-bottom: 14px; border-bottom: 1px dashed var(--ink);
  }
  .card-no { color: var(--ink); }
  .card-stamp {
    color: var(--coral); border: 2px solid var(--coral);
    padding: 4px 10px;
    letter-spacing: 0.2em; font-weight: 600;
  }
  .card-art { padding: 18px 0; color: var(--ink); }
  .card-art svg { width: 100%; height: auto; }
  .card-rows { font-size: 12px; border-top: 1px solid var(--rule); padding-top: 12px; }
  .card-rows div {
    display: flex; justify-content: space-between;
    padding: 6px 0; border-bottom: 1px dotted var(--rule);
    font-family: 'JetBrains Mono', monospace; font-size: 11px;
  }
  .card-rows div span:first-child { color: var(--ink-2); text-transform: uppercase; letter-spacing: 0.1em; }
  .card-rows div span:last-child { color: var(--ink); }
  .card-barcode { display: flex; align-items: flex-end; gap: 2px; margin-top: 14px; height: 42px; }
  .card-barcode span { display: block; width: var(--w); height: var(--h); background: var(--ink); }

  .sticky-note {
    position: absolute; right: -20px; bottom: -80px; width: 240px;
    background: #f4d27a; padding: 22px 22px 18px;
    box-shadow: 6px 8px 18px rgba(20,18,16,0.15);
    font-family: 'Fraunces', serif; font-style: italic; font-size: 15px;
    color: var(--ink);
  }
  .sticky-note p { margin: 0 0 10px; line-height: 1.4; }
  .sticky-note u { text-decoration-color: var(--coral); text-decoration-thickness: 2px; }
  .sticky-note span {
    font-family: 'JetBrains Mono', monospace; font-style: normal;
    font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
    color: var(--ink-2);
  }
  .pin { position: absolute; top: -10px; left: 50%; width: 20px; height: 20px; transform: translateX(-50%); }

  .marquee {
    margin-top: clamp(80px, 10vw, 140px);
    margin-left: -5vw; margin-right: -5vw;
    overflow: hidden;
    border-top: 1px solid var(--ink); border-bottom: 1px solid var(--ink);
    padding: 22px 0;
    background: var(--ink); color: var(--paper);
  }
  .marquee-track {
    display: flex; gap: 48px; white-space: nowrap;
    font-family: 'Fraunces', serif; font-style: italic;
    font-size: clamp(32px, 5vw, 62px);
    font-variation-settings: 'opsz' 144, 'SOFT' 60, 'WONK' 1;
    animation: slide 40s linear infinite;
  }
  .marquee-track span::after { content: '✦'; color: var(--coral); margin-left: 48px; }
  @keyframes slide { from { transform: translateX(0); } to { transform: translateX(-50%); } }

  /* CATALOG */
  .catalog { position: relative; z-index: 2; padding: clamp(80px, 10vw, 140px) clamp(24px, 5vw, 72px); }
  .section-head {
    display: grid; grid-template-columns: auto 1fr;
    column-gap: 28px; row-gap: 8px; align-items: baseline;
    margin-bottom: 56px; padding-bottom: 20px;
    border-bottom: 1px solid var(--ink);
  }
  .section-head.alt { border-bottom-color: var(--coral); }
  .section-no { color: var(--coral); align-self: start; padding-top: 12px; }
  .section-head h3 { font-size: clamp(38px, 5vw, 64px); line-height: 1; }
  .section-head p {
    grid-column: 2; color: var(--ink-2);
    font-family: 'Fraunces', serif; font-style: italic; font-size: 18px; margin: 0;
  }

  .catalog-grid {
    display: grid; grid-template-columns: repeat(4, 1fr); gap: 0;
    border-left: 1px solid var(--ink); border-top: 1px solid var(--ink);
  }
  .cat-card {
    border-right: 1px solid var(--ink); border-bottom: 1px solid var(--ink);
    padding: 28px 26px 30px;
    background: var(--paper);
    position: relative;
    transition: background 0.3s, color 0.3s;
    cursor: pointer;
  }
  .cat-card:nth-child(even) { background: var(--paper-2); }
  .cat-card:hover { background: var(--ink); color: var(--paper); }
  .cat-card:hover .cat-no, .cat-card:hover .cat-tag { color: var(--coral); }
  .cat-card:hover .meta { color: #c7b28a; }
  .cat-card header { display: flex; justify-content: space-between; margin-bottom: 28px; }
  .cat-card .cat-no { color: var(--ink-2); }
  .cat-card .cat-tag { color: var(--coral); border: 1px solid currentColor; padding: 2px 8px; }
  .cat-card h4 {
    font-size: 30px; line-height: 1.05; margin-bottom: 14px;
    font-variation-settings: 'opsz' 144, 'SOFT' 40, 'WONK' 1;
  }
  .cat-card p { font-size: 14px; color: inherit; opacity: 0.85; min-height: 100px; }
  .cat-card footer {
    display: flex; justify-content: space-between; align-items: center;
    margin-top: 20px; padding-top: 14px; border-top: 1px dashed currentColor;
  }
  .cat-card .meta {
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
    color: var(--ink-2);
  }
  .cat-card .arrow { font-size: 22px; transition: transform 0.3s; }
  .cat-card:hover .arrow { transform: translateX(8px); }

  /* PRINCIPLES */
  .principles {
    position: relative; z-index: 2;
    padding: clamp(80px, 10vw, 140px) clamp(24px, 5vw, 72px);
    background: var(--paper-2);
    border-top: 1px solid var(--ink); border-bottom: 1px solid var(--ink);
  }
  .principles-list {
    list-style: none; padding: 0; margin: 0;
    display: grid; grid-template-columns: repeat(2, 1fr);
    gap: 48px 72px; max-width: 1200px;
  }
  .principles-list li {
    display: grid; grid-template-columns: 80px 1fr; gap: 20px;
    align-items: start; padding-top: 20px;
    border-top: 2px solid var(--ink);
  }
  .pn {
    font-family: 'Fraunces', serif; font-style: italic;
    font-size: 46px; line-height: 0.9; color: var(--coral);
    font-variation-settings: 'opsz' 144, 'SOFT' 80, 'WONK' 1;
  }
  .principles-list h5 {
    font-size: 24px; margin-bottom: 8px;
    font-variation-settings: 'opsz' 144, 'SOFT' 30, 'WONK' 1;
  }
  .principles-list p { margin: 0; color: var(--ink-2); font-size: 15px; }

  /* PRESS */
  .press {
    position: relative; z-index: 2;
    padding: clamp(80px, 10vw, 140px) clamp(24px, 5vw, 72px);
    background: var(--ink); color: var(--paper);
  }
  .press-inner {
    display: grid; grid-template-columns: 1fr 1.2fr;
    gap: clamp(40px, 6vw, 100px); align-items: start;
  }
  .press h3 { font-size: clamp(40px, 5vw, 68px); line-height: 0.95; margin-bottom: 24px; }
  .press p { color: #c7b28a; max-width: 42ch; font-size: 16px; }
  .section-no.light { color: var(--coral); display: block; margin-bottom: 16px; }

  .press-form { display: grid; gap: 22px; }
  .press-form label { display: grid; gap: 8px; }
  .press-form span {
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px; letter-spacing: 0.16em; text-transform: uppercase;
    color: #c7b28a;
  }
  .press-form input, .press-form textarea {
    background: transparent; border: none;
    border-bottom: 1px solid #5b4f3a;
    color: var(--paper);
    padding: 10px 0;
    font-family: 'Fraunces', serif;
    font-size: 22px;
    font-variation-settings: 'opsz' 144, 'SOFT' 20;
    outline: none; transition: border-color 0.3s; resize: vertical;
  }
  .press-form input::placeholder, .press-form textarea::placeholder { color: #6b5d44; font-style: italic; }
  .press-form input:focus, .press-form textarea:focus { border-color: var(--coral); }
  .press-form button {
    justify-self: start;
    display: inline-flex; gap: 14px; align-items: center;
    background: var(--coral); color: var(--paper);
    border: none; padding: 16px 26px; border-radius: 999px;
    cursor: pointer; font-size: 14px; font-family: inherit;
    transition: transform 0.3s, background 0.3s, color 0.3s;
  }
  .press-form button:hover { background: var(--paper); color: var(--ink); transform: translateX(6px); }
  .press-form button svg { width: 24px; height: 12px; }

  /* COLOPHON */
  .colophon {
    position: relative; z-index: 2;
    padding: 40px clamp(24px, 5vw, 72px);
    background: var(--paper);
    border-top: 1px solid var(--ink);
  }
  .col-row { display: flex; justify-content: space-between; gap: 20px; padding: 8px 0; }
  .col-row.fine {
    color: var(--ink-2);
    border-top: 1px dashed var(--rule);
    margin-top: 8px; padding-top: 16px;
  }
  .wordmark.sm {
    font-size: 28px;
    font-family: 'Fraunces', serif;
    font-variation-settings: 'opsz' 144, 'SOFT' 20, 'WONK' 1;
  }

  /* Responsive */
  @media (max-width: 1000px) {
    .mast-row { grid-template-columns: 60px 1fr; }
    .mast-nav { display: none; }
    .hero-grid { grid-template-columns: 1fr; }
    .hero-right { margin-top: 40px; }
    .sticky-note { right: 10px; bottom: -60px; }
    .catalog-grid { grid-template-columns: repeat(2, 1fr); }
    .principles-list { grid-template-columns: 1fr; gap: 32px; }
    .press-inner { grid-template-columns: 1fr; }
    .col-row { flex-direction: column; gap: 6px; }
  }
  @media (max-width: 600px) {
    .catalog-grid { grid-template-columns: 1fr; }
    .hero-stats { flex-direction: column; gap: 20px; }
    .mast-title { align-items: flex-start; }
  }
</style>
