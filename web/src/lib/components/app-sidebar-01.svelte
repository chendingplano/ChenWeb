<script lang="ts">
	import CameraIcon from "@tabler/icons-svelte/icons/camera";
	import ChartBarIcon from "@tabler/icons-svelte/icons/chart-bar";
	import DashboardIcon from "@tabler/icons-svelte/icons/dashboard";
	import DatabaseIcon from "@tabler/icons-svelte/icons/database";
	import FileAiIcon from "@tabler/icons-svelte/icons/file-ai";
	import FileDescriptionIcon from "@tabler/icons-svelte/icons/file-description";
	import FileWordIcon from "@tabler/icons-svelte/icons/file-word";
	import FolderIcon from "@tabler/icons-svelte/icons/folder";
	import HelpIcon from "@tabler/icons-svelte/icons/help";
	import InnerShadowTopIcon from "@tabler/icons-svelte/icons/inner-shadow-top";
	import ListDetailsIcon from "@tabler/icons-svelte/icons/list-details";
	import ReportIcon from "@tabler/icons-svelte/icons/report";
	import SearchIcon from "@tabler/icons-svelte/icons/search";
	import SettingsIcon from "@tabler/icons-svelte/icons/settings";
	import UsersIcon from "@tabler/icons-svelte/icons/users";
	import NavDocuments from "./nav-documents-01.svelte";
	import type {Sidebar01MenuItem} from "$lib/types/menu";
	import NavMain from "./nav-main-01.svelte";
	import NavSecondary from "./nav-secondary-01.svelte";
	import NavUser from "./nav-user-01.svelte";
	import * as Sidebar from "$lib/components/ui/sidebar/index.js";
	import type { ComponentProps } from "svelte";
	import type { Icon } from "@tabler/icons-svelte";

	const data = {
		user: {
			name: "Chen Ding",
			email: "chending@example.com",
			avatar: "/avatars/shadcn.jpg",
		},
		navMain: [
			{
				title: "Dashboard",
				url: "#",
				icon: DashboardIcon,
			},
			{
				title: "Documents",
				url: "#",
				icon: ListDetailsIcon,
			},
			{
				title: "Login",
				url: "#",
				icon: ChartBarIcon,
			},
			{
				title: "Projects",
				url: "#",
				icon: FolderIcon,
			},
			{
				title: "Team",
				url: "#",
				icon: UsersIcon,
			},
		],
		navClouds: [
			{
				title: "Capture",
				icon: CameraIcon,
				isActive: true,
				url: "#",
				items: [
					{
						title: "Active Proposals",
						url: "#",
					},
					{
						title: "Archived",
						url: "#",
					},
				],
			},
			{
				title: "Proposal",
				icon: FileDescriptionIcon,
				url: "#",
				items: [
					{
						title: "Active Proposals",
						url: "#",
					},
					{
						title: "Archived",
						url: "#",
					},
				],
			},
			{
				title: "Prompts",
				icon: FileAiIcon,
				url: "#",
				items: [
					{
						title: "Active Proposals",
						url: "#",
					},
					{
						title: "Archived",
						url: "#",
					},
				],
			},
		],
		navSecondary: [
			{
				title: "Settings",
				url: "#",
				icon: SettingsIcon,
			},
			{
				title: "Get Help",
				url: "#",
				icon: HelpIcon,
			},
			{
				title: "Search",
				url: "#",
				icon: SearchIcon,
			},
		],
		documents: [
			{
				title: "Data Library",
				url: "#",
				icon: DatabaseIcon,
			},
			{
				title: "Reports",
				url: "#",
				icon: ReportIcon,
			},
			{
				title: "Word Assistant",
				url: "#",
				icon: FileWordIcon,
			},
		],
	};

	let { onMenuSelect,
		  ...restProps
	    }: ComponentProps<typeof Sidebar.Root> & {
		onMenuSelect: (item: Sidebar01MenuItem) => void 
	} = $props();

	// function handleSelection(item: { title: string; url: string; icon?: Icon }) {
	function handleSelection(item: Sidebar01MenuItem) {
        onMenuSelect(item);
    }
</script>

<Sidebar.Root collapsible="offcanvas" {...restProps}>
	<Sidebar.Header>
		<Sidebar.Menu>
			<Sidebar.MenuItem>
				<Sidebar.MenuButton class="data-[slot=sidebar-menu-button]:!p-1.5">
					{#snippet child({ props })}
						<a href="##" {...props}>
							<InnerShadowTopIcon class="!size-5" />
							<span class="text-base font-semibold">DeepDoc</span>
						</a>
					{/snippet}
				</Sidebar.MenuButton>
			</Sidebar.MenuItem>
		</Sidebar.Menu>
	</Sidebar.Header>
	<Sidebar.Content>
		<NavMain items={data.navMain} onMenuSelect={handleSelection}/>
		<NavDocuments items={data.documents} onMenuSelect={handleSelection}/>
		<NavSecondary items={data.navSecondary} onMenuSelect={handleSelection} class="mt-auto" />
	</Sidebar.Content>
	<Sidebar.Footer>
		<NavUser user={data.user} onMenuSelect={handleSelection}/>
	</Sidebar.Footer>
</Sidebar.Root>
