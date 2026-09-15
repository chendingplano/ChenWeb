<script lang="ts">
	import { onMount } from 'svelte';
	import {
		createAgentConversation, decideAgentPermission, deleteAgentConversation, getAgentConversation,
		listAgentConversations, listAgentProfiles, rateAgentMessage, stopAgentRun, streamAgentRun,
		type AgentConversation, type AgentProfile, type AgentResumeState
	} from '$lib/services/agentServiceClient';
	import { applyAgentEvent, emptyAgentLiveState, type AgentLiveState, type AgentSource } from '$lib/services/agentServiceStream';

	let profiles = $state<AgentProfile[]>([]);
	let conversations = $state<AgentConversation[]>([]);
	let selectedSlug = $state('');
	let activeId = $state('');
	let resume = $state<AgentResumeState | null>(null);
	let live = $state<AgentLiveState | null>(null);
	let composer = $state('');
	let lastPrompt = $state('');
	let runId = $state('');
	let running = $state(false);
	let loading = $state(true);
	let busy = $state(false);
	let error = $state('');
	let permissionMode = $state<'ask' | 'auto'>('auto');
	let currentAbort: AbortController | null = null;
	let selectedProfile = $derived(profiles.find((profile) => profile.slug === selectedSlug));
	let visibleConversations = $derived(conversations.filter((conversation) => conversation.service_slug === selectedSlug));
	let savedSources = $derived(resume?.sources_by_message ?? {});
	let latestSources = $derived((resume?.messages ?? []).filter((message) => message.role === 'assistant' && message.status === 'complete').flatMap((message) => savedSources[message.id] ?? []).slice(-8));

	onMount(() => {
		void loadDesk();
		return () => currentAbort?.abort();
	});

	async function loadDesk() {
		loading = true; error = '';
		try {
			[profiles, conversations] = await Promise.all([listAgentProfiles(), listAgentConversations()]);
			selectedSlug = profiles[0]?.slug ?? '';
			permissionMode = profiles[0]?.permission_default ?? 'auto';
			const fromURL = new URLSearchParams(window.location.search).get('conversation');
			const initial = conversations.find((conversation) => conversation.id === fromURL) ?? conversations.find((conversation) => conversation.service_slug === selectedSlug);
			if (initial) { selectedSlug = initial.service_slug; await openConversation(initial.id); }
		} catch (cause) { error = readableError(cause); }
		finally { loading = false; }
	}

	function readableError(cause: unknown) { return cause instanceof Error ? cause.message : 'Something went wrong. Try again.'; }

	async function chooseService(slug: string) {
		if (running) return;
		selectedSlug = slug; resume = null; activeId = ''; live = null; runId = '';
		history.replaceState(null, '', window.location.pathname);
		permissionMode = profiles.find((profile) => profile.slug === slug)?.permission_default ?? 'auto';
		const first = conversations.find((conversation) => conversation.service_slug === slug);
		if (first) await openConversation(first.id);
	}

	async function openConversation(id: string) {
		if (running) return;
		busy = true; error = '';
		try {
			resume = await getAgentConversation(id);
			activeId = id; live = null; runId = '';
			selectedSlug = resume.conversation.service_slug;
			history.replaceState(null, '', `${window.location.pathname}?conversation=${encodeURIComponent(id)}`);
		} catch (cause) { error = readableError(cause); }
		finally { busy = false; }
	}

	async function newConversation() {
		if (!selectedSlug || running || busy) return;
		busy = true; error = '';
		try {
			const created = await createAgentConversation(selectedSlug, 'New conversation');
			conversations = await listAgentConversations();
			resume = await getAgentConversation(created.id); activeId = created.id; live = null;
			history.replaceState(null, '', `${window.location.pathname}?conversation=${encodeURIComponent(created.id)}`);
		} catch (cause) { error = readableError(cause); }
		finally { busy = false; }
	}

	async function removeConversation(id: string) {
		if (running || busy || !confirm('Delete this conversation and its saved answers?')) return;
		busy = true; error = '';
		try {
			await deleteAgentConversation(id);
			conversations = await listAgentConversations();
			if (activeId === id) { activeId = ''; resume = null; live = null; history.replaceState(null, '', window.location.pathname); }
		} catch (cause) { error = readableError(cause); }
		finally { busy = false; }
	}

	async function sendMessage(text = composer) {
		const question = text.trim();
		if (!question || running || busy || !selectedSlug) return;
		if (question.length > 16000) { error = 'Please shorten your question.'; return; }
		let conversationId = activeId;
		if (!conversationId) {
			await newConversation();
			conversationId = activeId;
			if (!conversationId) return;
		}
		composer = ''; lastPrompt = question; error = ''; runId = ''; running = true;
		live = emptyAgentLiveState();
		currentAbort = new AbortController();
		try {
			await streamAgentRun(conversationId, question, permissionMode,
				(event) => { if (live) live = applyAgentEvent(live, event); },
				(id) => { runId = id; }, currentAbort.signal);
			if (live?.status === 'running') live = { ...live, status: 'interrupted', error: 'The connection ended before the answer was finished.' };
		} catch (cause) {
			error = readableError(cause);
			if (live) live = { ...live, status: 'interrupted', permission: null, sources: [] };
		} finally {
			running = false; currentAbort = null;
			try {
				resume = await getAgentConversation(conversationId); conversations = await listAgentConversations();
				if (live && runId && resume.messages.some((message) => message.attempt_id === runId && message.role === 'assistant')) live = { ...live, answer: '' };
			}
			catch (cause) { error = readableError(cause); }
		}
	}

	async function stopRun() {
		if (!activeId || !runId) return;
		try { await stopAgentRun(activeId, runId); }
		catch (cause) { error = readableError(cause); }
	}

	async function answerPermission(allowed: boolean) {
		if (!activeId || !runId || !live?.permission) return;
		try {
			await decideAgentPermission(activeId, runId, live.permission.requestId, allowed);
			live = { ...live, permission: null };
		} catch (cause) { error = readableError(cause); }
	}

	async function rate(messageId: string, rating: 'helpful' | 'unhelpful') {
		if (!activeId) return;
		try { await rateAgentMessage(activeId, messageId, rating); }
		catch (cause) { error = readableError(cause); }
	}

	function sourceLabel(source: AgentSource & { document_title?: string }) { return source.document_title || source.source_title || `Document ${source.document_id}`; }
	function sourceLocation(source: AgentSource & { page_start?: number; page_end?: number }) {
		const page = source.page_start || source.page;
		const lines = source.line_start && source.line_end ? ` · lines ${source.line_start}–${source.line_end}` : '';
		return `${page ? `page ${page}` : 'source'}${lines}`;
	}
	function runStatus(status: string) {
		return ({ running: 'Working', completed: 'Completed', stopped: 'Stopped', interrupted: 'Interrupted', limit: 'Limit reached', failed: 'Could not finish' } as Record<string, string>)[status] ?? status;
	}
</script>

<svelte:head>
	<title>Knowledge Desk · ChenWeb</title>
	<meta name="description" content="Ask ChenWeb's evidence-guided agents about your documents and problems." />
</svelte:head>

<div class="desk">
	<header class="masthead">
		<a class="back" href="/home3/knowledge">← Workspace</a>
		<div class="brand"><span class="mark">CW</span><span>CHENWEB <small>KNOWLEDGE DESK</small></span></div>
		<div class="mast-note">AI-assisted research · evidence first</div>
	</header>

	<section class="intro">
		<div><p class="eyebrow">FIELD NOTES / 01</p><h1>Ask the knowledge<br /><em>behind the answer.</em></h1></div>
		<p class="intro-copy">Choose a guide, ask your question, and see the documents it used. The guide can be useful, but it can be wrong—check important answers against the original source.</p>
	</section>

	{#if error}<div class="notice error" role="alert">{error}<button onclick={() => error = ''} aria-label="Dismiss error">×</button></div>{/if}
	{#if loading}<div class="loading">Opening the knowledge desk…</div>{/if}
	{#if !loading && profiles.length === 0}<div class="notice">No guides are enabled for your account yet. Ask your ChenWeb administrator about the pilot.</div>{/if}

	{#if profiles.length > 0}
		<div class="service-strip" role="group" aria-label="Choose a guide">
			{#each profiles as profile (profile.slug)}
				<button class:chosen={selectedSlug === profile.slug} disabled={running} onclick={() => chooseService(profile.slug)}>
					<span class="service-number">{profile.slug === 'knowledge-guide' ? '01' : '02'}</span>
					<span><strong>{profile.friendly_name}</strong><small>{profile.description}</small></span>
					<span class="arrow">↗</span>
				</button>
			{/each}
		</div>

		<div class="workbench">
			<aside class="conversations" aria-label="Conversations">
				<div class="panel-heading"><span>CONVERSATIONS</span><button class="new" disabled={running || busy} onclick={newConversation}>+ New</button></div>
				{#if visibleConversations.length === 0}<p class="empty-list">No conversations yet. Start with a question.</p>{/if}
				{#each visibleConversations as conversation (conversation.id)}
					<div class:current={activeId === conversation.id} class="conversation-row">
						<button class="conversation-open" disabled={running || busy} onclick={() => openConversation(conversation.id)}><span>{conversation.title || 'Untitled conversation'}</span><small>{new Date(conversation.updated_at).toLocaleDateString()}</small></button>
						<button class="delete" disabled={running || busy} onclick={() => removeConversation(conversation.id)} title="Delete conversation" aria-label={`Delete ${conversation.title}`}>×</button>
					</div>
				{/each}
			</aside>

			<main class="thread">
				<div class="thread-head"><div><span class="eyebrow">{selectedProfile?.friendly_name ?? 'GUIDE'}</span><h2>{resume?.conversation.title || 'A new conversation'}</h2></div><span class="provider">{selectedProfile?.provider_disclosure || selectedProfile?.provider || 'AI provider'}</span></div>
				<div class="messages" aria-live="polite">
					{#if resume?.omission_notice}<div class="notice access">{resume.omission_notice}</div>{/if}
					{#if !resume?.messages.length && !live}<div class="empty-thread"><span class="large-mark">?</span><h3>What would you like to understand?</h3><p>Ask about a document, product, metric, or problem. Your guide can search only knowledge you are allowed to see.</p></div>{/if}
					{#each resume?.messages ?? [] as message (message.id)}
						<article class:user={message.role === 'user'} class="message">
							<div class="message-label">{message.role === 'user' ? 'YOU' : selectedProfile?.friendly_name?.toUpperCase() || 'GUIDE'} <span>{message.status !== 'complete' ? `· ${message.status}` : ''}</span></div>
							<p>{message.content || (message.status === 'streaming' ? 'Answer in progress…' : '')}</p>
							{#if message.role === 'assistant' && savedSources[message.id]?.length}
								<div class="inline-sources">{#each savedSources[message.id] as source}<span>↗ {sourceLabel(source)} · {sourceLocation(source)}</span>{/each}</div>
							{/if}
							{#if message.role === 'assistant' && message.status === 'complete'}<div class="rating"><span>Was this useful?</span><button onclick={() => rate(message.id, 'helpful')}>Yes</button><button onclick={() => rate(message.id, 'unhelpful')}>No</button></div>{/if}
						</article>
					{/each}
					{#if running && lastPrompt}<article class="message user pending"><div class="message-label">YOU · SENDING</div><p>{lastPrompt}</p></article>{/if}
					{#if live && (running || live.status !== 'completed')}
						<article class="message live"><div class="message-label">{selectedProfile?.friendly_name?.toUpperCase() || 'GUIDE'} · {runStatus(live.status)}</div><p>{live.answer || (running ? 'Searching the knowledge base…' : 'Any partial answer is saved above.')}</p>{#if live.error}<small class="live-error">{live.error}</small>{/if}</article>
					{/if}
				</div>
				<form class="composer" onsubmit={(event) => { event.preventDefault(); void sendMessage(); }}>
					<label for="question">YOUR QUESTION</label>
					<textarea id="question" bind:value={composer} disabled={running || busy} maxlength="16000" placeholder="Describe the question or problem you are working through…" rows="3"></textarea>
					<div class="composer-foot"><div class="mode"><span>Knowledge tools:</span><label><input type="radio" name="permission" value="auto" bind:group={permissionMode} disabled={running} />Use allowed tools automatically</label><label><input type="radio" name="permission" value="ask" bind:group={permissionMode} disabled={running} />Ask me first</label></div><div class="composer-actions">{#if running}<button type="button" class="stop" disabled={!runId} onclick={stopRun}>Stop</button>{:else if lastPrompt && live?.status !== 'completed'}<button type="button" class="retry" onclick={() => { composer = lastPrompt; }}>Try again</button>{/if}<button type="submit" class="send" disabled={!composer.trim() || running || busy}>Ask guide ↗</button></div></div>
				</form>
			</main>

			<aside class="evidence" aria-label="Evidence and activity">
				<div class="evidence-top"><span class="eyebrow">THE RESEARCH TRAIL</span><h2>Evidence &amp;<br /><em>activity</em></h2><p>These are documents and actions the guide used—not its private reasoning.</p></div>
				{#if live?.permission}<div class="approval" role="alert"><strong>Approval needed</strong><p>May the guide use <code>{live.permission.tool.replaceAll('_', ' ')}</code> to look up knowledge?</p><div><button onclick={() => answerPermission(false)}>Deny</button><button class="approve" onclick={() => answerPermission(true)}>Allow this time</button></div></div>{/if}
				<div class="evidence-section"><div class="section-title"><span>01 / SOURCES</span><span>{(live?.sources.length || latestSources.length) || '—'}</span></div>
					{#if (live?.sources.length || latestSources.length) === 0}<p class="quiet">Document references will appear here after the answer is saved and checked.</p>{/if}
					{#each (live?.sources.length ? live.sources : latestSources) as source, index (`${source.document_id}-${index}`)}
						<div class="source-card"><span class="source-index">{String(index + 1).padStart(2, '0')}</span><strong>{sourceLabel(source)}</strong><small>{sourceLocation(source)}</small>{#if source.artifact_type}<small>{source.artifact_type}{source.artifact_id ? ` · ${source.artifact_id}` : ''}</small>{/if}<a href="/home3/inputs" target="_blank" rel="noopener noreferrer">Open knowledge documents ↗</a><small>Record ID {source.document_id}</small></div>
					{/each}
				</div>
				<div class="evidence-section"><div class="section-title"><span>02 / ACTIVITY</span><span>{live?.activity.length || '—'}</span></div>{#if !live?.activity.length}<p class="quiet">Search and reading actions will appear during a run.</p>{/if}{#each live?.activity ?? [] as action, index (index)}<div class="activity-row"><span class="activity-dot" class:bad={action.error}></span><span>{action.tool ? action.tool.replaceAll('_', ' ') : 'Retrying'} · {action.status}{action.error ? ' (failed)' : ''}</span></div>{/each}</div>
				<div class="scope-note"><strong>ABOUT THIS GUIDE</strong><p>{selectedProfile?.description}</p><p>Model: {selectedProfile?.model}. Allowed store names (subject to your access): {selectedProfile?.allowed_knowledge_stores?.join(', ') || 'none'}.</p><p>AI can misread evidence or miss context. Verify consequential decisions with the source document and a qualified person.</p></div>
			</aside>
		</div>
	{/if}
</div>

<style>
	:global(body){margin:0;background:#f5f1e8;color:#26312b}
	.desk{--ink:#26312b;--muted:#6c756e;--paper:#f5f1e8;--line:#d5d1c5;--red:#a34d32;min-height:100vh;font-family:'Avenir Next','Gill Sans','Trebuchet MS',sans-serif;background:radial-gradient(circle at 20% 0%,#fffdf7 0,transparent 34%),var(--paper);color:var(--ink)}
	button,textarea{font:inherit}button{cursor:pointer}.masthead{height:72px;display:flex;align-items:center;justify-content:space-between;padding:0 clamp(20px,4vw,68px);background:#26312b;color:#f5f1e8}.back{color:#e3dac8;text-decoration:none;font-size:13px;letter-spacing:.02em}.back:hover{text-decoration:underline}.brand{display:flex;gap:12px;align-items:center;font-size:15px;font-weight:800;letter-spacing:.18em}.brand small{display:block;font-size:9px;letter-spacing:.28em;color:#bbb9ab;margin-top:2px}.mark{border:1px solid #e6d8bd;border-radius:50%;height:36px;width:36px;display:grid;place-items:center;font-family:'Iowan Old Style',Georgia,serif;font-style:italic;font-size:13px;letter-spacing:0}.mast-note{font-size:11px;letter-spacing:.08em;color:#d1cabc}
	.intro{display:flex;align-items:end;justify-content:space-between;gap:48px;padding:52px clamp(20px,4vw,68px) 34px;border-bottom:1px solid var(--line)}.eyebrow{font-size:10px;letter-spacing:.18em;font-weight:800;color:var(--red)}.intro h1,.evidence h2{font-family:'Iowan Old Style','Baskerville',Georgia,serif;font-weight:400;letter-spacing:-.045em}.intro h1{font-size:clamp(42px,5vw,76px);line-height:.95;margin:10px 0 0}.intro em,.evidence em{color:var(--red);font-weight:400}.intro-copy{max-width:360px;font-size:14px;line-height:1.7;color:#5d665d;margin:0 0 5px}.notice{margin:16px clamp(20px,4vw,68px);padding:13px 16px;background:#ede5d6;border-left:3px solid #9d7951;font-size:13px}.notice.error{display:flex;justify-content:space-between;background:#f4e1d9;border-color:var(--red)}.notice button{border:0;background:none;font-size:20px;color:var(--red)}.loading{padding:30px clamp(20px,4vw,68px);font-family:Georgia,serif;font-style:italic}
	.service-strip,.service-strip button{box-sizing:border-box}.service-strip{display:flex;gap:12px;padding:22px clamp(20px,4vw,68px)}.service-strip button{display:flex;align-items:center;gap:18px;text-align:left;flex:1;min-height:82px;padding:15px 19px;border:1px solid var(--line);background:#fbf9f2;color:var(--ink);transition:transform .18s,border-color .18s,background .18s}.service-strip button:hover{transform:translateY(-2px);border-color:#9f9e90}.service-strip button.chosen{border-color:var(--red);background:#fffaf0;box-shadow:inset 0 0 0 1px var(--red)}.service-strip button:disabled{opacity:.65;cursor:default}.service-number{font-family:Georgia,serif;font-size:22px;color:var(--red)}.service-strip strong{display:block;font-size:16px}.service-strip small{display:block;color:var(--muted);font-size:11px;line-height:1.4;margin-top:3px}.arrow{margin-left:auto;font-size:20px;color:var(--red)}
	.workbench{display:grid;grid-template-columns:minmax(180px,240px) minmax(430px,1fr) minmax(250px,320px);margin:0 clamp(20px,4vw,68px) 50px;min-height:650px;border:1px solid var(--line);background:#fffdf7;box-shadow:0 24px 50px #3b4a3420}.conversations{border-right:1px solid var(--line);background:#f7f4eb;padding:24px 12px}.panel-heading,.section-title{display:flex;align-items:center;justify-content:space-between;font-size:10px;font-weight:800;letter-spacing:.14em}.panel-heading{padding:0 10px 18px}.new{border:0;background:none;color:var(--red);font-size:13px;font-weight:700;letter-spacing:0}.new:hover{text-decoration:underline}.empty-list,.quiet{color:#878d82;font-size:12px;line-height:1.6}.empty-list{padding:14px 10px}.conversation-row{display:flex;border-bottom:1px solid #e2ded3}.conversation-row.current{background:#eee9dc;border-left:2px solid var(--red)}.conversation-open{flex:1;text-align:left;border:0;background:none;padding:13px 10px;color:var(--ink);min-width:0}.conversation-open span{display:block;white-space:nowrap;text-overflow:ellipsis;overflow:hidden;font-size:13px;font-weight:700}.conversation-open small{font-size:10px;color:#8a8b7d}.delete{border:0;background:none;color:#a69b8d;padding:0 9px;font-size:20px}.delete:hover{color:var(--red)}
	.thread{display:flex;flex-direction:column;min-height:650px;min-width:0}.thread-head{min-height:112px;padding:25px 30px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;gap:15px;align-items:center}.thread-head h2{font-family:'Iowan Old Style',Georgia,serif;font-size:27px;font-weight:400;margin:6px 0 0}.provider{font-size:10px;color:#8c7865;border:1px solid #e1d7c5;padding:7px 10px;max-width:180px;line-height:1.4}.messages{flex:1;padding:25px 30px;max-height:640px;overflow:auto}.messages .notice{margin:0 0 20px}.empty-thread{text-align:center;max-width:400px;margin:65px auto;color:var(--muted)}.empty-thread h3{font-family:Georgia,serif;font-weight:400;font-size:26px;color:var(--ink)}.empty-thread p{font-size:13px;line-height:1.6}.large-mark{display:grid;place-items:center;width:70px;height:70px;margin:auto;border:1px solid #bdbaa8;border-radius:50%;font-family:Georgia,serif;font-style:italic;font-size:36px;color:var(--red)}.message{margin-bottom:24px;max-width:730px}.message.user{margin-left:10%;padding:15px 19px;background:#f0eee5;border-left:2px solid #b7b2a4}.message.live{border-left:2px solid var(--red);padding-left:19px}.message-label{font-size:10px;font-weight:800;letter-spacing:.15em;color:var(--red)}.message.user .message-label{color:#687c70}.message-label span{color:#a1a59a}.message p{white-space:pre-wrap;overflow-wrap:anywhere;font-size:15px;line-height:1.7;margin:8px 0}.inline-sources{display:flex;flex-wrap:wrap;gap:6px;margin-top:12px}.inline-sources span{font-size:10px;color:#6b7266;background:#eeeadf;padding:5px 8px}.rating{display:flex;gap:9px;align-items:center;margin-top:12px;font-size:10px;color:#8b8d83}.rating button{font-size:10px;border:1px solid #d8d4c9;background:transparent;color:#526c58;padding:4px 8px}.rating button:hover{border-color:#526c58}.live-error{color:var(--red)}
	.composer{border-top:1px solid var(--line);padding:20px 30px;background:#f9f6ef}.composer>label{display:block;font-size:10px;letter-spacing:.14em;font-weight:800;margin-bottom:9px}.composer textarea{resize:vertical;width:100%;box-sizing:border-box;min-height:82px;padding:12px;border:1px solid #cac8b9;background:#fffdf8;color:var(--ink);outline:none}.composer textarea:focus{border-color:var(--red);box-shadow:0 0 0 2px #a34d3220}.composer-foot{display:flex;justify-content:space-between;align-items:end;gap:15px;margin-top:13px}.mode{display:flex;flex-wrap:wrap;gap:8px 12px;align-items:center;font-size:10px;color:var(--muted)}.mode label{display:flex;gap:4px;align-items:center;cursor:pointer}.mode input{accent-color:var(--red)}.composer-actions{display:flex;gap:7px;flex-shrink:0}.composer-actions button{font-size:12px;padding:9px 12px;border:1px solid #aca99b;background:transparent;color:var(--ink)}.composer-actions .send{background:var(--ink);color:#fff;border-color:var(--ink);font-weight:700}.composer-actions .send:hover{background:#435246}.composer-actions .stop{color:var(--red);border-color:var(--red)}.composer-actions button:disabled{opacity:.45;cursor:default}
	.evidence{border-left:1px solid var(--line);background:#f1efe6;padding:26px 22px}.evidence-top{border-bottom:1px solid #d1cdbe;padding-bottom:22px}.evidence h2{font-size:38px;line-height:.95;margin:12px 0}.evidence-top p,.scope-note p{font-size:11px;line-height:1.65;color:#687168}.evidence-section{border-bottom:1px solid #d1cdbe;padding:20px 0}.section-title{padding-bottom:13px}.source-card{position:relative;padding:12px 14px 13px 38px;background:#fffdf7;border:1px solid #d5d0c3;margin:8px 0;box-shadow:2px 2px 0 #ded8cb}.source-index{position:absolute;left:12px;color:var(--red);font-family:Georgia,serif;font-size:13px}.source-card strong{display:block;font-size:12px}.source-card small{display:block;font-size:10px;color:#8c8d80;margin-top:4px}.source-card a{display:block;margin-top:9px;color:#647e68;font-size:10px;text-decoration:none;font-weight:700}.source-card a:hover{text-decoration:underline}.activity-row{display:flex;gap:9px;align-items:center;padding:7px 0;font-size:11px;color:#586c5c;text-transform:capitalize}.activity-dot{width:6px;height:6px;border-radius:50%;background:#688f6d}.activity-dot.bad{background:var(--red)}.scope-note{padding-top:25px}.scope-note strong{font-size:10px;letter-spacing:.14em}.approval{padding:15px;background:#e7ddca;border-left:3px solid var(--red);margin:16px 0}.approval strong{font-size:12px}.approval p{font-size:12px;line-height:1.5}.approval code{font:inherit;font-weight:700}.approval div{display:flex;gap:8px}.approval button{padding:7px 11px;border:1px solid #9a988b;background:transparent}.approval .approve{background:var(--red);border-color:var(--red);color:#fff}
	@media(max-width:1170px){.workbench{grid-template-columns:200px minmax(350px,1fr)}.evidence{grid-column:1/-1;border-left:0;border-top:1px solid var(--line);display:grid;grid-template-columns:1fr 1fr;gap:0 30px}.evidence-top,.scope-note{grid-column:1/-1}.evidence h2 br{display:none}.approval{grid-column:1/-1}}@media(max-width:780px){.mast-note{display:none}.intro{display:block;padding-top:30px}.intro-copy{margin-top:20px}.service-strip{display:block}.service-strip button{width:100%;margin-bottom:8px}.workbench{display:block}.conversations{border-right:0;border-bottom:1px solid var(--line);max-height:160px;overflow:auto}.thread-head,.messages,.composer{padding-left:18px;padding-right:18px}.composer-foot{display:block}.composer-actions{justify-content:flex-end;margin-top:12px}.evidence{display:block}.intro h1{font-size:47px}}
</style>
