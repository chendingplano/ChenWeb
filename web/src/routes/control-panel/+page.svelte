<script lang="ts">
	let currentPage = 'Intro';

	let expandedMenus = {
		Configuration: false,
		Run: false
	};

	function handleMenuItemClick(itemName) {
		currentPage = itemName;
	}

	function toggleMenu(menuName) {
		expandedMenus[menuName] = !expandedMenus[menuName];
	}
</script>

<div class="container">
	<div class="top-pane">
		<h1>Application Control Panel</h1>
	</div>

	<div class="main-content">
		<div class="left-pane">
			<div class="menu">
				<!-- Level 0 items -->
				<button
					class="menu-item"
					tabindex="0"
					onclick={() => handleMenuItemClick('Welcome to this Test!')}
					>Intro
				</button>

				<button
					class="menu-directory"
					onclick={() => toggleMenu('Configuration')}
					aria-expanded={expandedMenus['Configuration']}
				>
					Configuration {expandedMenus['Configuration'] ? '▼' : '▶'}
				</button>
				{#if expandedMenus['Configuration']}
					<button class="menu-item level-1" onclick={() => handleMenuItemClick('Config')}>
						<a href="/config" target="right-frame">Config</a>
					</button>
					<button class="menu-item level-1" onclick={() => handleMenuItemClick('Install')}
						>Install
					</button>
				{/if}

				<button
					class="menu-directory"
					onclick={() => toggleMenu('Run')}
					aria-expanded={expandedMenus['Configuration']}
				>
					Run {expandedMenus['Run'] ? '▼' : '▶'}
				</button>
				{#if expandedMenus['Run']}
					<button class="menu-item level-1" onclick={() => handleMenuItemClick('Start')}
						>Start
					</button>
					<button class="menu-item level-1" onclick={() => handleMenuItemClick('Stop')}
						>Stop
					</button>
				{/if}
			</div>
		</div>

		<div class="right-pane">
			{#if currentPage === 'Intro'}
				<div class="content">
					<h2>{currentPage}</h2>
					<p>You have selected the "{currentPage}" menu item.</p>
				</div>
			{:else if currentPage === 'Config'}
				<!-- Load the config form page -->
				<iframe
					src="/config"
					name="right-frame"
					style="width: 100%; height: 100%; border: none;"
					title="Config Form"
				></iframe>
			{:else}
				<div class="content">
					<h2>{currentPage}</h2>
					<p>You have selected the "{currentPage}" menu item.</p>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.container {
		display: flex;
		flex-direction: column;
		width: 1500px;
		height: 1300px;
		font-family: Arial, sans-serif;
		margin: 0 auto;
	}

	.top-pane {
		width: 1500px;
		height: 100px;
		background-color: #2c3e50;
		color: white;
		display: flex;
		align-items: center;
		padding: 0 20px;
		box-sizing: border-box;
	}

	.main-content {
		display: flex;
		flex-direction: row;
		flex: 1;
	}

	.left-pane {
		width: 200px;
		height: 1200px;
		background-color: #ecf0f1;
		padding: 10px;
		box-sizing: border-box;
		overflow-y: auto;
	}

	.right-pane {
		width: 1300px;
		height: 1200px;
		background-color: #ecf0f1;
		padding: 20px;
		box-sizing: border-box;
		border-left: 1px solid #bdc3c7;
	}

	.menu-directory {
		font-weight: bold;
		padding: 10px 0 5px 0;
		color: #2c3e50;
		cursor: default;
	}

	.menu-item {
		padding: 8px 0;
		cursor: pointer;
		color: #3498db;
		transition: all 0.2s;
	}

	.menu-item.level-1 {
		padding-left: 20px;
	}

	.menu-item:hover {
		color: #2980b9;
		background-color: #d6eaf8;
		padding-left: 5px;
	}

	.content {
		padding: 20px;
	}

	h1 {
		margin: 0;
	}

	h2 {
		color: #2c3e50;
		border-bottom: 2px solid #3498db;
		padding-bottom: 10px;
	}

	/* Add iframe styling */
	iframe {
		width: 100%;
		height: 100%;
		border: none;
	}
</style>
