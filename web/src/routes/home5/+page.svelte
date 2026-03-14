<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { appAuthStore } from '@chendingplano/shared';
	import type { Sidebar07MenuItem } from '$lib/types/menu';
	
	// ============================================================================
	// DESIGN VARIABLES - Easy to customize visual attributes
	// Developers can adjust these variables to change the page appearance
	// ============================================================================
	
	// --- Layout Dimensions ---
	let topPanelHeight = 200; // Top banner height (pixels)
	let minLeftPanelWidth = 220; // Minimum left panel width
	let minRightPanelWidth = 280; // Minimum right panel width
	let initialLeftPanelWidth = 280; // Initial left panel width
	let initialRightPanelWidth = 320; // Initial right panel width
	let footerHeight = 80; // Footer height
	let footerGap = 24; // Gap between content and footer
	
	// --- Color Palette (Light Mode) ---
	let pageBackgroundLight = '#f5f5f7'; // Main page background (light grey base)
	let cardSurfaceLight = '#ffffff'; // Card surface background
	let secondarySurfaceLight = '#fafafa'; // Secondary surface (panels, sidebars)
	let borderColorLight = '#e0e0e0'; // Border color
	let dividerColorLight = '#d0d0d0'; // Divider lines
	let textPrimaryLight = '#1a1a1a'; // Primary text color
	let textSecondaryLight = '#666666'; // Secondary text color
	let accentColorLight = '#0066cc'; // Accent color (calm blue-teal)
	let accentHoverLight = '#0052a3'; // Accent hover state
	
	// --- Color Palette (Dark Mode) ---
	let pageBackgroundDark = '#1a1a1c'; // Main page background (deep grey, not black)
	let cardSurfaceDark = '#252527'; // Card surface background
	let secondarySurfaceDark = '#202022'; // Secondary surface
	let borderColorDark = '#3a3a3c'; // Border color
	let dividerColorDark = '#404040'; // Divider lines
	let textPrimaryDark = '#f5f5f7'; // Primary text color
	let textSecondaryDark = '#a0a0a0'; // Secondary text color
	let accentColorDark = '#4da3ff'; // Accent color
	let accentHoverDark = '#66b3ff'; // Accent hover state
	
	// --- Typography ---
	let fontFamily = 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
	let fontSizeBase = '14px';
	let fontSizeSmall = '12px';
	let fontSizeLarge = '16px';
	let fontSizeXLarge = '20px';
	
	// --- Spacing ---
	let spacingXS = '4px';
	let spacingSM = '8px';
	let spacingMD = '16px';
	let spacingLG = '24px';
	let spacingXL = '32px';
	
	// --- Border Radius ---
	let borderRadiusSM = '4px';
	let borderRadiusMD = '8px';
	let borderRadiusLG = '12px';
	let borderRadiusXL = '16px';
	
	// --- Shadows ---
	let shadowSM = '0 1px 2px rgba(0, 0, 0, 0.05)';
	let shadowMD = '0 4px 6px rgba(0, 0, 0, 0.07)';
	let shadowLG = '0 10px 15px rgba(0, 0, 0, 0.1)';
	
	// --- Transitions ---
	let transitionFast = '150ms ease';
	let transitionNormal = '250ms ease';
	let transitionSlow = '350ms ease';
	
	// ============================================================================
	// STATE
	// ============================================================================
	let leftPanelWidth = $state(initialLeftPanelWidth);
	let rightPanelWidth = $state(initialRightPanelWidth);
	let isResizingLeft = $state(false);
	let isResizingRight = $state(false);
	let selectedMenuItem = $state<Sidebar07MenuItem | null>(null);
	let userInfo = $state<{ name: string; email: string; avatar: string } | null>(null);
	let isDarkMode = $state(false);
	let dashboardData = $state<any>(null);
	let agentsData = $state<any>(null);
	let skillsData = $state<any>(null);
	let applicationsData = $state<any>(null);
	let knowledgeBaseData = $state<any>(null);
	let settingsData = $state<any>(null);
	let loading = $state<{ [key: string]: boolean }>({});
	let showUserMenu = $state(false);
	let cleanupFn: (() => void) | null = null;
	let iconsLoaded = $state(false);
	
	// Icon components - declared with $state for proper reactivity tracking
	let LayoutDashboardIcon = $state<any>();
	let BotIcon = $state<any>();
	let ZapIcon = $state<any>();
	let GridIcon = $state<any>();
	let CodeIcon = $state<any>();
	let UserIcon = $state<any>();
	let DatabaseIcon = $state<any>();
	let SettingsIcon = $state<any>();
	let InfoIcon = $state<any>();
	let LogoutIcon = $state<any>();
	let BellIcon = $state<any>();
	let MoreVerticalIcon = $state<any>();
	let SunIcon = $state<any>();
	let MoonIcon = $state<any>();
	let GripVerticalIcon = $state<any>();
	
	// Menu definitions
	const mainMenuItems = [
		{ id: 'dashboard', title: 'Dashboard', icon: 'layout-dashboard', url: '/api/v1/ai-assistant/dashboard' },
		{ id: 'agents', title: 'Agents', icon: 'bot', url: '/api/v1/ai-assistant/agents' },
		{ id: 'skills', title: 'Skills', icon: 'zap', url: '/api/v1/ai-assistant/skills' },
		{ id: 'applications', title: 'Applications', icon: 'grid', url: '/api/v1/ai-assistant/applications' },
		{ id: 'coding-assistant', title: 'Coding Assistant', icon: 'code', url: '#' },
		{ id: 'personal-assistant', title: 'Personal Assistant', icon: 'user', url: '#' },
		{ id: 'knowledge-base', title: 'Knowledge Base', icon: 'database', url: '/api/v1/ai-assistant/knowledge-base' },
	];
	
	const bottomMenuItems = [
		{ id: 'settings', title: 'Settings', icon: 'settings', url: '/api/v1/ai-assistant/settings' },
		{ id: 'about', title: 'About', icon: 'info', url: '#' },
	];
	
	const userMenuItems = [
		{ id: 'user-info', title: 'User Info', icon: 'user' },
		{ id: 'account', title: 'Account', icon: 'settings' },
		{ id: 'logout', title: 'Log Out', icon: 'logout' },
	];
	
	function getIcon(iconName: string) {
		const iconMap: { [key: string]: any } = {
			'layout-dashboard': LayoutDashboardIcon, 'bot': BotIcon, 'zap': ZapIcon,
			'grid': GridIcon, 'code': CodeIcon, 'user': UserIcon, 'database': DatabaseIcon,
			'settings': SettingsIcon, 'info': InfoIcon, 'logout': LogoutIcon,
			'bell': BellIcon, 'more-vertical': MoreVerticalIcon, 'sun': SunIcon, 'moon': MoonIcon,
			'grip-vertical': GripVerticalIcon,
		};
		return iconMap[iconName] || null;
	}

	onMount(() => {
		isDarkMode = window.matchMedia('(prefers-color-scheme: dark)').matches;
		document.documentElement.setAttribute('data-theme', isDarkMode ? 'dark' : 'light');
		
		const fetchIcons = async () => {
			const isLoggedIn = appAuthStore.getIsLoggedIn();
			if (isLoggedIn) await fetchUserInfo();
			selectMenuItem(mainMenuItems[0]);

			const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
			const handler = (e: MediaQueryListEvent) => { 
				isDarkMode = e.matches; 
				document.documentElement.setAttribute('data-theme', isDarkMode ? 'dark' : 'light'); 
			};
			mediaQuery.addEventListener('change', handler);

			const icons = await Promise.all([
				import('@lucide/svelte/icons/layout-dashboard').then(m => m.default),
				import('@lucide/svelte/icons/bot').then(m => m.default),
				import('@lucide/svelte/icons/zap').then(m => m.default),
				import('@lucide/svelte/icons/grid').then(m => m.default),
				import('@lucide/svelte/icons/code').then(m => m.default),
				import('@lucide/svelte/icons/user').then(m => m.default),
				import('@lucide/svelte/icons/database').then(m => m.default),
				import('@lucide/svelte/icons/settings').then(m => m.default),
				import('@lucide/svelte/icons/info').then(m => m.default),
				import('@lucide/svelte/icons/log-out').then(m => m.default),
				import('@lucide/svelte/icons/bell').then(m => m.default),
				import('@lucide/svelte/icons/more-vertical').then(m => m.default),
				import('@lucide/svelte/icons/sun').then(m => m.default),
				import('@lucide/svelte/icons/moon').then(m => m.default),
				import('@lucide/svelte/icons/grip-vertical').then(m => m.default),
			]);
			[LayoutDashboardIcon, BotIcon, ZapIcon, GridIcon, CodeIcon, UserIcon, DatabaseIcon, SettingsIcon, InfoIcon, LogoutIcon, BellIcon, MoreVerticalIcon, SunIcon, MoonIcon, GripVerticalIcon] = icons as any;

			cleanupFn = () => { mediaQuery.removeEventListener('change', handler); };
			iconsLoaded = true;
		};
		
		fetchIcons();

		return () => { if (cleanupFn) cleanupFn(); };
	});
	
	function startResizeLeft() { isResizingLeft = true; document.body.style.cursor = 'col-resize'; document.body.style.userSelect = 'none'; }
	function startResizeRight() { isResizingRight = true; document.body.style.cursor = 'col-resize'; document.body.style.userSelect = 'none'; }
	function handleResize(e: MouseEvent) {
		if (isResizingLeft) { const w = e.clientX; if (w >= minLeftPanelWidth && w <= 500) leftPanelWidth = w; }
		if (isResizingRight) { const w = window.innerWidth - e.clientX; if (w >= minRightPanelWidth && w <= 500) rightPanelWidth = w; }
	}
	function stopResize() { isResizingLeft = false; isResizingRight = false; document.body.style.cursor = ''; document.body.style.userSelect = ''; }
	function selectMenuItem(item: any) { selectedMenuItem = { id: item.id, type: 'navigation', title: item.title, url: item.url }; if (item.url?.startsWith('/api/v1/ai-assistant/')) fetchDataForMenuItem(item.id, item.url); }
	function handleUserMenuAction(action: string) { showUserMenu = false; if (action === 'logout') { appAuthStore.logout(); window.location.href = '/login'; } }
	function toggleTheme() { isDarkMode = !isDarkMode; document.documentElement.setAttribute('data-theme', isDarkMode ? 'dark' : 'light'); }
	async function fetchUserInfo() { try { const r = await fetch('/api/v1/ai-assistant/user-info'); if (r.ok) userInfo = (await r.json()).user; } catch (e) { console.error('Error fetching user info:', e); } }
	async function fetchDataForMenuItem(menuId: string, url: string) {
		loading[menuId] = true;
		try {
			const r = await fetch(url);
			if (r.ok) {
				const data = await r.json();
				if (menuId === 'dashboard') dashboardData = data;
				else if (menuId === 'agents') agentsData = data;
				else if (menuId === 'skills') skillsData = data;
				else if (menuId === 'applications') applicationsData = data;
				else if (menuId === 'knowledge-base') knowledgeBaseData = data;
				else if (menuId === 'settings') settingsData = data;
			}
		} catch (e) { console.error(`Error fetching ${menuId}:`, e); }
		finally { loading[menuId] = false; }
	}
</script>

<svelte:window onmousemove={handleResize} onmouseup={stopResize} />

<div class="page-container">
	<header class="top-panel" style="height: {topPanelHeight}px;">
		<div class="top-panel-content">
			<div class="top-panel-text">
				<h1 class="top-panel-title">My AI Assistant</h1>
				<p class="top-panel-subtitle">Your intelligent workspace for daily productivity</p>
			</div>
			<div class="top-panel-actions">
				<button class="icon-button theme-toggle" onclick={toggleTheme}>{#if iconsLoaded}{#if isDarkMode}<SunIcon size={20} />{:else}<MoonIcon size={20} />{/if}{/if}</button>
				<button class="icon-button notification-button">{#if iconsLoaded}<BellIcon size={20} />{/if}<span class="notification-badge">3</span></button>
			</div>
		</div>
		<div class="top-panel-bg">
			<svg class="ai-pattern" viewBox="0 0 800 200" preserveAspectRatio="none">
				<defs><linearGradient id="gradient1" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0%" style="stop-color:{isDarkMode ? '#4da3ff' : '#0066cc'};stop-opacity:0.3" /><stop offset="100%" style="stop-color:{isDarkMode ? '#0066cc' : '#4da3ff'};stop-opacity:0.1" /></linearGradient></defs>
				<circle cx="100" cy="100" r="80" fill="url(#gradient1)" opacity="0.5" />
				<circle cx="300" cy="80" r="60" fill="url(#gradient1)" opacity="0.4" />
				<circle cx="500" cy="120" r="100" fill="url(#gradient1)" opacity="0.3" />
				<circle cx="700" cy="60" r="70" fill="url(#gradient1)" opacity="0.5" />
				<line x1="100" y1="100" x2="300" y2="80" stroke="{isDarkMode ? '#4da3ff' : '#0066cc'}" stroke-width="1" opacity="0.3" />
				<line x1="300" y1="80" x2="500" y2="120" stroke="{isDarkMode ? '#4da3ff' : '#0066cc'}" stroke-width="1" opacity="0.3" />
				<line x1="500" y1="120" x2="700" y2="60" stroke="{isDarkMode ? '#4da3ff' : '#0066cc'}" stroke-width="1" opacity="0.3" />
			</svg>
		</div>
	</header>

	<div class="main-content">
		<aside class="left-panel" style="width: {leftPanelWidth}px;">
			<nav class="left-nav">
				<div class="menu-section">
					{#each mainMenuItems as item (item.id)}
						{@const Icon = getIcon(item.icon)}
						<button class="menu-item {selectedMenuItem?.id === item.id ? 'active' : ''}" onclick={() => selectMenuItem(item)}>
							{#if iconsLoaded && Icon}<Icon size={18} class="menu-icon" />{/if}
							<span class="menu-title">{item.title}</span>
						</button>
					{/each}
				</div>
				<div class="menu-section menu-section-bottom">
					{#each bottomMenuItems as item (item.id)}
						{@const Icon = getIcon(item.icon)}
						<button class="menu-item {selectedMenuItem?.id === item.id ? 'active' : ''}" onclick={() => selectMenuItem(item)}>
							{#if iconsLoaded && Icon}<Icon size={18} class="menu-icon" />{/if}
							<span class="menu-title">{item.title}</span>
						</button>
					{/each}
				</div>
			</nav>
			
			<div class="left-panel-footer">
				{#if userInfo}
					<div class="user-info">
						<div class="user-avatar">
							{#if userInfo.avatar}
								<img src={userInfo.avatar} alt={userInfo.name} />
							{:else}
								<span class="avatar-fallback">{userInfo.name.charAt(0)}</span>
							{/if}
						</div>
						<div class="user-details"><div class="user-name">{userInfo.name}</div><div class="user-email">{userInfo.email}</div></div>
						<button class="user-menu-button" onclick={() => showUserMenu = !showUserMenu}>{#if iconsLoaded}<MoreVerticalIcon size={18} />{/if}</button>
						{#if showUserMenu}
							<div class="user-menu-popup">
								{#each userMenuItems as item (item.id)}
									{@const Icon = getIcon(item.icon)}
									<button class="user-menu-item" onclick={() => handleUserMenuAction(item.id)}>{#if iconsLoaded && Icon}<Icon size={16} class="user-menu-icon" />{/if}<span>{item.title}</span></button>
								{/each}
							</div>
						{/if}
					</div>
				{:else}
					<div class="user-info"><div class="user-avatar"><span class="avatar-fallback">U</span></div><div class="user-details"><div class="user-name">Guest User</div><div class="user-email">Not logged in</div></div></div>
				{/if}
			</div>
			<div class="resize-divider left-resize-divider" class:hovered={isResizingLeft} onmousedown={startResizeLeft}><div class="grab-dots"><span class="dot"></span><span class="dot"></span><span class="dot"></span><span class="dot"></span></div></div>
		</aside>

		<main class="middle-panel" style="min-height: calc(100vh - {topPanelHeight + footerHeight + footerGap * 2}px);">
			<div class="middle-panel-content">
				{#if selectedMenuItem}
					<div class="content-header"><h2 class="content-title">{selectedMenuItem.title}</h2></div>
					<div class="content-body">
						{#if selectedMenuItem.id === 'dashboard' && dashboardData}
							<div class="dashboard-grid">
								{#if dashboardData.stats}<div class="stats-grid">{#each Object.entries(dashboardData.stats) as [key, value]}<div class="stat-card"><div class="stat-label">{key}</div><div class="stat-value">{value}</div></div>{/each}</div>{/if}
								{#if dashboardData.recentActivity}<div class="activity-section"><h3 class="section-title">Recent Activity</h3><div class="activity-list">{#each dashboardData.recentActivity as activity (activity.id)}<div class="activity-item status-{activity.status}"><div class="activity-icon"></div><div class="activity-content"><div class="activity-msg">{activity.msg}</div><div class="activity-time">{activity.time}</div></div></div>{/each}</div></div>{/if}
								{#if dashboardData.systemStatus}<div class="status-section"><h3 class="section-title">System Status</h3><div class="status-grid">{#each Object.entries(dashboardData.systemStatus) as [key, value]}<div class="status-item status-{value}"><span class="status-dot"></span><span class="status-label">{key}</span><span class="status-value">{value}</span></div>{/each}</div></div>{/if}
							</div>
						{/if}
						{#if selectedMenuItem.id === 'agents' && agentsData}
							<div class="agents-list">{#if agentsData.agents}{#each agentsData.agents as agent (agent.id)}<div class="agent-card"><div class="agent-header"><div class="agent-name">{agent.name}</div><span class="agent-status status-{agent.status}">{agent.status}</span></div><div class="agent-model">Model: {agent.model}</div><div class="agent-desc">{agent.desc}</div></div>{/each}{/if}</div>
						{/if}
						{#if selectedMenuItem.id === 'skills' && skillsData}
							<div class="skills-grid">{#if skillsData.skills}{#each skillsData.skills as skill (skill.id)}<div class="skill-card"><div class="skill-name">{skill.name}</div><div class="skill-category">{skill.category}</div><div class="skill-usage">Used {skill.usageCount} times</div></div>{/each}{/if}</div>
						{/if}
						{#if selectedMenuItem.id === 'applications' && applicationsData}
							<div class="applications-list">{#if applicationsData.applications}{#each applicationsData.applications as app (app.id)}<div class="application-card"><div class="app-header"><div class="app-name">{app.name}</div><span class="app-status status-{app.status}">{app.status}</span></div><div class="app-category">{app.category}</div></div>{/each}{/if}</div>
						{/if}
						{#if selectedMenuItem.id === 'knowledge-base' && knowledgeBaseData}
							<div class="knowledge-base-content">
								{#if knowledgeBaseData.stats}<div class="kb-stats">{#each Object.entries(knowledgeBaseData.stats) as [key, value]}<div class="kb-stat"><div class="kb-stat-label">{key}</div><div class="kb-stat-value">{value}</div></div>{/each}</div>{/if}
								{#if knowledgeBaseData.recentDocuments}<div class="recent-docs"><h3 class="section-title">Recent Documents</h3>{#each knowledgeBaseData.recentDocuments as doc (doc.id)}<div class="doc-item"><div class="doc-icon">📄</div><div class="doc-info"><div class="doc-name">{doc.name}</div><div class="doc-meta"><span>{(doc.size / 1024 / 1024).toFixed(2)} MB</span><span>•</span><span>Updated {new Date(doc.updatedAt).toLocaleDateString()}</span></div></div></div>{/each}</div>{/if}
							</div>
						{/if}
						{#if selectedMenuItem.id === 'settings' && settingsData}
							<div class="settings-content">
								{#if settingsData.settings?.general}<div class="settings-section"><h3 class="section-title">General Settings</h3>{#each Object.entries(settingsData.settings.general) as [key, value]}<div class="setting-item"><span class="setting-label">{key}</span><span class="setting-value">{typeof value === 'boolean' ? (value ? '✓' : '✗') : value}</span></div>{/each}</div>{/if}
								{#if settingsData.settings?.aiModels}<div class="settings-section"><h3 class="section-title">AI Models</h3>{#each Object.entries(settingsData.settings.aiModels) as [key, value]}<div class="setting-item"><span class="setting-label">{key}</span><span class="setting-value">{value}</span></div>{/each}</div>{/if}
							</div>
						{/if}
						{#if loading[selectedMenuItem.id]}<div class="loading-state"><div class="loading-spinner"></div><span>Loading...</span></div>{/if}
					</div>
				{/if}
			</div>
			
			<footer class="bottom-panel" style="height: {footerHeight}px; margin-top: {footerGap}px;">
				<div class="footer-content">
					<div class="footer-left"><div class="footer-logo"><span class="logo-text">AI Assistant</span></div><div class="footer-copyright">© 2026 ChenWeb. All rights reserved.</div></div>
					<div class="footer-center"><div class="footer-links"><button type="button" class="footer-link">Documentation</button><button type="button" class="footer-link">API Reference</button><button type="button" class="footer-link">Support</button><button type="button" class="footer-link">Privacy</button></div></div>
					<div class="footer-right"><div class="footer-status"><span class="status-indicator online"></span><span>All systems operational</span></div><div class="footer-version">Version 1.0.0</div></div>
				</div>
			</footer>
		</main>

		<aside class="right-panel" style="width: {rightPanelWidth}px;">
			<div class="right-panel-content">
				{#if selectedMenuItem}
					{#if selectedMenuItem.id === 'dashboard'}<div class="info-section"><h3 class="info-title">Quick Actions</h3><div class="quick-actions"><button class="action-button">{#if iconsLoaded}<BotIcon size={16} />{/if}<span>New Agent</span></button><button class="action-button">{#if iconsLoaded}<ZapIcon size={16} />{/if}<span>Add Skill</span></button><button class="action-button">{#if iconsLoaded}<CodeIcon size={16} />{/if}<span>Start Task</span></button></div></div>{/if}
					{#if selectedMenuItem.id === 'agents' && agentsData}<div class="info-section"><h3 class="info-title">Agent Stats</h3><div class="info-stat"><span class="stat-label">Total Agents</span><span class="stat-value">{agentsData.total}</span></div><div class="info-stat"><span class="stat-label">Active</span><span class="stat-value">{agentsData.active}</span></div></div>{/if}
					{#if selectedMenuItem.id === 'knowledge-base' && knowledgeBaseData?.stats}<div class="info-section"><h3 class="info-title">Storage Info</h3><div class="info-stat"><span class="stat-label">Indexed Chunks</span><span class="stat-value">{knowledgeBaseData.stats.indexedChunks}</span></div><div class="info-stat"><span class="stat-label">Searches Today</span><span class="stat-value">{knowledgeBaseData.stats.searchesToday}</span></div></div>{/if}
					<div class="info-section"><h3 class="info-title">About</h3><p class="info-text">{selectedMenuItem.title} provides powerful tools and features to enhance your workflow.</p></div>
					<div class="info-section"><h3 class="info-title">Help & Resources</h3><ul class="info-links"><li><button type="button" class="info-link">Documentation</button></li><li><button type="button" class="info-link">Tutorials</button></li><li><button type="button" class="info-link">Community</button></li><li><button type="button" class="info-link">Report Issue</button></li></ul></div>
				{:else}
					<div class="info-section"><h3 class="info-title">Welcome</h3><p class="info-text">Select a menu item from the left panel to view detailed information.</p></div>
				{/if}
			</div>
			<div class="resize-divider right-resize-divider" class:hovered={isResizingRight} onmousedown={startResizeRight}><div class="grab-dots"><span class="dot"></span><span class="dot"></span><span class="dot"></span><span class="dot"></span></div></div>
		</aside>
	</div>
</div>

<style>
	:global(body) { margin: 0; padding: 0; font-family: var(--font-family); background-color: var(--page-bg); color: var(--text-primary); transition: background-color var(--transition-normal), color var(--transition-normal); }
	
	/* CSS Custom Properties - Theme Variables (Light Mode Default) */
	:root {
		--page-bg: #f5f5f7;
		--card-surface: #ffffff;
		--secondary-surface: #fafafa;
		--border-color: #e0e0e0;
		--divider-color: #d0d0d0;
		--text-primary: #1a1a1a;
		--text-secondary: #666666;
		--accent-color: #0066cc;
		--accent-hover: #0052a3;
		--hover-bg: rgba(0, 0, 0, 0.03);
		--active-bg: rgba(0, 102, 204, 0.1);
		--font-family: Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
		--font-size-base: 14px;
		--font-size-small: 12px;
		--font-size-large: 16px;
		--font-size-xlarge: 20px;
		--spacing-xs: 4px;
		--spacing-sm: 8px;
		--spacing-md: 16px;
		--spacing-lg: 24px;
		--spacing-xl: 32px;
		--border-radius-sm: 4px;
		--border-radius-md: 8px;
		--border-radius-lg: 12px;
		--border-radius-xl: 16px;
		--shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05);
		--shadow-md: 0 4px 6px rgba(0, 0, 0, 0.07);
		--shadow-lg: 0 10px 15px rgba(0, 0, 0, 0.1);
		--transition-fast: 150ms ease;
		--transition-normal: 250ms ease;
		--transition-slow: 350ms ease;
	}
	
	:global([data-theme="dark"]) {
		--page-bg: #1a1a1c;
		--card-surface: #252527;
		--secondary-surface: #202022;
		--border-color: #3a3a3c;
		--divider-color: #404040;
		--text-primary: #f5f5f7;
		--text-secondary: #a0a0a0;
		--accent-color: #4da3ff;
		--accent-hover: #66b3ff;
		--hover-bg: rgba(255, 255, 255, 0.05);
		--active-bg: rgba(77, 163, 255, 0.15);
	}
	.page-container { display: flex; flex-direction: column; height: 100vh; overflow: hidden; }
	.top-panel { position: relative; background: linear-gradient(135deg, var(--secondary-surface) 0%, var(--page-bg) 100%); border-bottom: 1px solid var(--border-color); overflow: hidden; }
	.top-panel-content { position: relative; z-index: 2; display: flex; justify-content: space-between; align-items: center; height: 100%; padding: 0 var(--spacing-lg); }
	.top-panel-text { display: flex; flex-direction: column; gap: var(--spacing-xs); }
	.top-panel-title { font-size: var(--font-size-xlarge); font-weight: 600; margin: 0; color: var(--text-primary); }
	.top-panel-subtitle { font-size: var(--font-size-base); color: var(--text-secondary); margin: 0; }
	.top-panel-actions { display: flex; gap: var(--spacing-md); }
	.icon-button { display: flex; align-items: center; justify-content: center; width: 40px; height: 40px; border: none; border-radius: var(--border-radius-md); background-color: var(--card-surface); color: var(--text-primary); cursor: pointer; transition: all var(--transition-fast); position: relative; }
	.icon-button:hover { background-color: var(--secondary-surface); transform: translateY(-1px); }
	.notification-badge { position: absolute; top: 6px; right: 6px; background-color: #ef4444; color: white; font-size: 10px; font-weight: 600; min-width: 16px; height: 16px; border-radius: 8px; display: flex; align-items: center; justify-content: center; }
	.top-panel-bg { position: absolute; top: 0; left: 0; width: 100%; height: 100%; opacity: 0.5; pointer-events: none; }
	.ai-pattern { width: 100%; height: 100%; }
	.main-content { display: flex; flex: 1; overflow: hidden; position: relative; }
	.left-panel { position: relative; background-color: var(--secondary-surface); border-right: 1px solid var(--border-color); display: flex; flex-direction: column; flex-shrink: 0; transition: width var(--transition-slow); }
	.left-nav { flex: 1; overflow-y: auto; padding: var(--spacing-md); }
	.menu-section { display: flex; flex-direction: column; gap: var(--spacing-xs); }
	.menu-section-bottom { margin-top: auto; padding-top: var(--spacing-md); border-top: 1px solid var(--divider-color); }
	.menu-item { display: flex; align-items: center; gap: var(--spacing-sm); width: 100%; padding: var(--spacing-sm) var(--spacing-md); border: none; border-radius: var(--border-radius-md); background-color: transparent; color: var(--text-primary); font-size: var(--font-size-base); text-align: left; cursor: pointer; transition: all var(--transition-fast); }
	.menu-item:hover { background-color: var(--hover-bg); }
	.menu-item.active { background-color: var(--active-bg); color: var(--accent-color); font-weight: 500; }
	.menu-icon { flex-shrink: 0; opacity: 0.8; }
	.menu-title { flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.left-panel-footer { padding: var(--spacing-md); border-top: 1px solid var(--border-color); background-color: var(--secondary-surface); }
	.user-info { position: relative; display: flex; align-items: center; gap: var(--spacing-sm); }
	.user-avatar { width: 36px; height: 36px; border-radius: var(--border-radius-md); overflow: hidden; flex-shrink: 0; }
	.user-avatar img { width: 100%; height: 100%; object-fit: cover; }
	.avatar-fallback { display: flex; align-items: center; justify-content: center; width: 100%; height: 100%; background-color: var(--accent-color); color: white; font-weight: 600; font-size: var(--font-size-base); }
	.user-details { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.user-name { font-size: var(--font-size-base); font-weight: 500; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.user-email { font-size: var(--font-size-small); color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.user-menu-button { display: flex; align-items: center; justify-content: center; width: 32px; height: 32px; border: none; border-radius: var(--border-radius-sm); background-color: transparent; color: var(--text-secondary); cursor: pointer; transition: all var(--transition-fast); }
	.user-menu-button:hover { background-color: var(--hover-bg); color: var(--text-primary); }
	.user-menu-popup { position: absolute; bottom: 100%; right: 0; margin-bottom: var(--spacing-sm); background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); box-shadow: var(--shadow-lg); padding: var(--spacing-xs); min-width: 160px; z-index: 100; }
	.user-menu-item { display: flex; align-items: center; gap: var(--spacing-sm); width: 100%; padding: var(--spacing-sm) var(--spacing-md); border: none; border-radius: var(--border-radius-sm); background-color: transparent; color: var(--text-primary); font-size: var(--font-size-base); text-align: left; cursor: pointer; transition: all var(--transition-fast); }
	.user-menu-item:hover { background-color: var(--hover-bg); }
	.user-menu-icon { opacity: 0.7; }
	.resize-divider { position: absolute; top: 0; width: 12px; height: 100%; cursor: col-resize; z-index: 10; display: flex; align-items: center; justify-content: center; opacity: 0; transition: opacity var(--transition-fast); }
	.left-resize-divider { right: -6px; }
	.right-resize-divider { left: -6px; }
	.resize-divider:hover, .resize-divider:hovered { opacity: 1; }
	.resize-divider:hover .grab-dots, .resize-divider:hovered .grab-dots { opacity: 1; }
	.grab-dots { display: flex; flex-direction: column; gap: 3px; opacity: 0.3; transition: opacity var(--transition-fast); }
	.dot { width: 4px; height: 4px; border-radius: 50%; background-color: var(--text-primary); }
	.middle-panel { flex: 1; overflow-y: auto; background-color: var(--page-bg); display: flex; flex-direction: column; }
	.middle-panel-content { flex: 1; padding: var(--spacing-lg); }
	.content-header { margin-bottom: var(--spacing-lg); }
	.content-title { font-size: var(--font-size-xlarge); font-weight: 600; margin: 0; color: var(--text-primary); }
	.content-body { display: flex; flex-direction: column; gap: var(--spacing-lg); }
	.dashboard-grid { display: flex; flex-direction: column; gap: var(--spacing-lg); }
	.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--spacing-md); }
	.stat-card { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-lg); box-shadow: var(--shadow-sm); }
	.stat-label { font-size: var(--font-size-small); color: var(--text-secondary); margin-bottom: var(--spacing-xs); text-transform: capitalize; }
	.stat-value { font-size: 28px; font-weight: 600; color: var(--accent-color); }
	.activity-section, .status-section { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-lg); box-shadow: var(--shadow-sm); }
	.section-title { font-size: var(--font-size-large); font-weight: 600; margin: 0 0 var(--spacing-md) 0; color: var(--text-primary); }
	.activity-list { display: flex; flex-direction: column; gap: var(--spacing-md); }
	.activity-item { display: flex; align-items: flex-start; gap: var(--spacing-md); padding: var(--spacing-md); border-radius: var(--border-radius-sm); border-left: 3px solid var(--border-color); }
	.activity-item.status-success { border-left-color: #10b981; }
	.activity-item.status-warning { border-left-color: #f59e0b; }
	.activity-item.status-error { border-left-color: #ef4444; }
	.activity-icon { width: 8px; height: 8px; border-radius: 50%; background-color: var(--text-secondary); margin-top: 4px; }
	.activity-content { flex: 1; }
	.activity-msg { font-size: var(--font-size-base); color: var(--text-primary); margin-bottom: var(--spacing-xs); }
	.activity-time { font-size: var(--font-size-small); color: var(--text-secondary); }
	.status-grid { display: flex; flex-direction: column; gap: var(--spacing-sm); }
	.status-item { display: flex; align-items: center; gap: var(--spacing-md); padding: var(--spacing-sm) 0; }
	.status-dot { width: 8px; height: 8px; border-radius: 50%; }
	.status-item.status-operational .status-dot { background-color: #10b981; }
	.status-label { flex: 1; font-size: var(--font-size-base); color: var(--text-primary); text-transform: capitalize; }
	.status-value { font-size: var(--font-size-small); color: var(--text-secondary); }
	.agents-list, .applications-list { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: var(--spacing-md); }
	.agent-card, .application-card { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-lg); box-shadow: var(--shadow-sm); }
	.agent-header, .app-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--spacing-sm); }
	.agent-name, .app-name { font-weight: 600; font-size: var(--font-size-base); color: var(--text-primary); }
	.agent-status, .app-status { font-size: var(--font-size-small); padding: var(--spacing-xs) var(--spacing-sm); border-radius: var(--border-radius-sm); text-transform: capitalize; }
	.agent-status.status-active, .app-status.status-connected { background-color: rgba(16, 185, 129, 0.1); color: #10b981; }
	.agent-status.status-idle, .app-status.status-disconnected { background-color: rgba(107, 114, 128, 0.1); color: #6b7280; }
	.agent-model, .app-category { font-size: var(--font-size-small); color: var(--text-secondary); margin-bottom: var(--spacing-sm); }
	.agent-desc { font-size: var(--font-size-small); color: var(--text-secondary); line-height: 1.5; }
	.skills-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: var(--spacing-md); }
	.skill-card { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-lg); box-shadow: var(--shadow-sm); }
	.skill-name { font-weight: 600; font-size: var(--font-size-base); color: var(--text-primary); margin-bottom: var(--spacing-xs); }
	.skill-category { font-size: var(--font-size-small); color: var(--text-secondary); margin-bottom: var(--spacing-xs); }
	.skill-usage { font-size: var(--font-size-small); color: var(--accent-color); }
	.knowledge-base-content { display: flex; flex-direction: column; gap: var(--spacing-lg); }
	.kb-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: var(--spacing-md); }
	.kb-stat { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-md); text-align: center; }
	.kb-stat-label { font-size: var(--font-size-small); color: var(--text-secondary); margin-bottom: var(--spacing-xs); text-transform: capitalize; }
	.kb-stat-value { font-size: var(--font-size-xlarge); font-weight: 600; color: var(--accent-color); }
	.recent-docs { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-lg); }
	.doc-item { display: flex; align-items: center; gap: var(--spacing-md); padding: var(--spacing-md) 0; border-bottom: 1px solid var(--divider-color); }
	.doc-item:last-child { border-bottom: none; }
	.doc-icon { font-size: 24px; }
	.doc-info { flex: 1; }
	.doc-name { font-size: var(--font-size-base); color: var(--text-primary); margin-bottom: var(--spacing-xs); }
	.doc-meta { font-size: var(--font-size-small); color: var(--text-secondary); display: flex; gap: var(--spacing-sm); }
	.settings-content { display: flex; flex-direction: column; gap: var(--spacing-lg); }
	.settings-section { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-lg); }
	.setting-item { display: flex; justify-content: space-between; align-items: center; padding: var(--spacing-sm) 0; border-bottom: 1px solid var(--divider-color); }
	.setting-item:last-child { border-bottom: none; }
	.setting-label { font-size: var(--font-size-base); color: var(--text-primary); text-transform: capitalize; }
	.setting-value { font-size: var(--font-size-base); color: var(--text-secondary); }
	.loading-state { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: var(--spacing-xl); gap: var(--spacing-md); }
	.loading-spinner { width: 40px; height: 40px; border: 3px solid var(--border-color); border-top-color: var(--accent-color); border-radius: 50%; animation: spin 1s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	.bottom-panel { background-color: var(--card-surface); border-top: 1px solid var(--border-color); display: flex; align-items: center; justify-content: center; padding: 0 var(--spacing-lg); }
	.footer-content { display: flex; justify-content: space-between; align-items: center; width: 100%; max-width: 1400px; }
	.footer-left { display: flex; flex-direction: column; gap: var(--spacing-xs); }
	.footer-logo { display: flex; align-items: center; gap: var(--spacing-sm); }
	.logo-text { font-weight: 600; font-size: var(--font-size-base); color: var(--text-primary); }
	.footer-copyright { font-size: var(--font-size-small); color: var(--text-secondary); }
	.footer-center { display: flex; gap: var(--spacing-lg); }
	.footer-links { display: flex; gap: var(--spacing-lg); }
	.footer-link { font-size: var(--font-size-small); color: var(--text-secondary); background: none; border: none; padding: 0; cursor: pointer; transition: color var(--transition-fast); }
	.footer-link:hover { color: var(--accent-color); }
	.footer-right { display: flex; flex-direction: column; align-items: flex-end; gap: var(--spacing-xs); }
	.footer-status { display: flex; align-items: center; gap: var(--spacing-sm); font-size: var(--font-size-small); color: var(--text-secondary); }
	.status-indicator { width: 8px; height: 8px; border-radius: 50%; }
	.status-indicator.online { background-color: #10b981; }
	.footer-version { font-size: var(--font-size-small); color: var(--text-secondary); }
	.right-panel { position: relative; background-color: var(--secondary-surface); border-left: 1px solid var(--border-color); flex-shrink: 0; transition: width var(--transition-slow); overflow-y: auto; }
	.right-panel-content { padding: var(--spacing-lg); display: flex; flex-direction: column; gap: var(--spacing-lg); }
	.info-section { background-color: var(--card-surface); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); padding: var(--spacing-lg); box-shadow: var(--shadow-sm); }
	.info-title { font-size: var(--font-size-large); font-weight: 600; margin: 0 0 var(--spacing-md) 0; color: var(--text-primary); }
	.info-text { font-size: var(--font-size-base); color: var(--text-secondary); line-height: 1.6; margin: 0; }
	.info-stat { display: flex; justify-content: space-between; align-items: center; padding: var(--spacing-sm) 0; border-bottom: 1px solid var(--divider-color); }
	.info-stat:last-child { border-bottom: none; }
	.stat-label { font-size: var(--font-size-base); color: var(--text-secondary); }
	.stat-value { font-size: var(--font-size-large); font-weight: 600; color: var(--text-primary); }
	.info-links { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: var(--spacing-sm); }
	.info-link { font-size: var(--font-size-base); color: var(--accent-color); background: none; border: none; padding: 0; cursor: pointer; text-align: left; transition: color var(--transition-fast); }
	.info-link:hover { color: var(--accent-hover); }
	.quick-actions { display: flex; flex-direction: column; gap: var(--spacing-sm); }
	.action-button { display: flex; align-items: center; gap: var(--spacing-sm); width: 100%; padding: var(--spacing-sm) var(--spacing-md); border: 1px solid var(--border-color); border-radius: var(--border-radius-md); background-color: var(--secondary-surface); color: var(--text-primary); font-size: var(--font-size-base); cursor: pointer; transition: all var(--transition-fast); }
	.action-button:hover { background-color: var(--card-surface); border-color: var(--accent-color); }
	::-webkit-scrollbar { width: 8px; height: 8px; }
	::-webkit-scrollbar-track { background: var(--secondary-surface); }
	::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 4px; }
	::-webkit-scrollbar-thumb:hover { background: var(--text-secondary); }
	@media (max-width: 1024px) { .right-panel { display: none; } .footer-content { flex-direction: column; gap: var(--spacing-md); text-align: center; } .footer-left, .footer-center, .footer-right { align-items: center; } }
	@media (max-width: 768px) { .left-panel { display: none; } .top-panel-content { flex-direction: column; align-items: flex-start; gap: var(--spacing-md); } }
</style>
