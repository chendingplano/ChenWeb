<script lang="ts">
	import AudioWaveformIcon from "@lucide/svelte/icons/audio-waveform";
	import BookOpenIcon from "@lucide/svelte/icons/book-open";
	import BotIcon from "@lucide/svelte/icons/bot";
	import ChartPieIcon from "@lucide/svelte/icons/chart-pie";
	import CommandIcon from "@lucide/svelte/icons/command";
	import FrameIcon from "@lucide/svelte/icons/frame";
	import GalleryVerticalEndIcon from "@lucide/svelte/icons/gallery-vertical-end";
	import MapIcon from "@lucide/svelte/icons/map";
	import Settings2Icon from "@lucide/svelte/icons/settings-2";
	import SquareTerminalIcon from "@lucide/svelte/icons/square-terminal";
	import type { Icon } from "@tabler/icons-svelte";
	import NavMain from "./nav-main-07.svelte";
	import NavProjects from "./nav-projects-07.svelte";
	import NavUser from "./nav-user-07.svelte";
	import TeamSwitcher from "./team-switcher-07.svelte";
	import * as Sidebar from "$lib/components/ui/sidebar/index.js";
	import type { ComponentProps } from "svelte";
	import type { Sidebar07MenuItem} from "$lib/types/menu";

	// This is sample data.
	const data = {
		user: {
			name: "DeepDoc",
			email: "chending@deepdoc.me",
			avatar: "/avatars/shadcn.jpg",
		},
		teams: [
			{
				name: "Acme Inc",
                url: "",
				logo: GalleryVerticalEndIcon,
				plan: "Enterprise",
			},
			{
				name: "Acme Corp.",
                url:"",
				logo: AudioWaveformIcon,
				plan: "Startup",
			},
			{
				name: "Evil Corp.",
                url:"",
				logo: CommandIcon,
				plan: "Free",
			},
		],
		navMain: [
			{
				title: "Playground",
				url: "#",
				icon: SquareTerminalIcon,
				isActive: true,
				items: [
					{
						title: "History",
						url: "#",
					},
					{
						title: "Starred",
						url: "#",
					},
					{
						title: "Settings",
						url: "#",
					},
				],
			},
			{
				title: "Models",
				url: "#",
				icon: BotIcon,
				items: [
					{
						title: "Genesis",
						url: "#",
					},
					{
						title: "Explorer",
						url: "#",
					},
					{
						title: "Quantum",
						url: "#",
					},
				],
			},
			{
				title: "Documentation",
				url: "#",
				icon: BookOpenIcon,
				items: [
					{
						title: "Introduction",
						url: "#",
					},
					{
						title: "Get Started",
						url: "#",
					},
					{
						title: "Tutorials",
						url: "#",
					},
					{
						title: "Changelog",
						url: "#",
					},
				],
			},
			{
				title: "Settings",
				url: "#",
				icon: Settings2Icon,
				items: [
					{
						title: "General",
						url: "#",
					},
					{
						title: "Team",
						url: "#",
					},
					{
						title: "Billing",
						url: "#",
					},
					{
						title: "Limits",
						url: "#",
					},
				],
			},
		],
		projects: [
			{
				name: "Design Engineering",
				url: "#",
				icon: FrameIcon,
			},
			{
				name: "Sales & Marketing",
				url: "#",
				icon: ChartPieIcon,
			},
			{
				name: "Travel",
				url: "#",
				icon: MapIcon,
			},
		],
	};

    let { onMenuSelect,
		  collapsible = "icon",
		  ...restProps
	    }: ComponentProps<typeof Sidebar.Root> & {
		onMenuSelect: (item: Sidebar07MenuItem) => void 
	} = $props();

	function handleSelection(item: Sidebar07MenuItem) {
        onMenuSelect(item);
    }

</script>

<Sidebar.Root {collapsible} {...restProps}>
	<Sidebar.Header>
		<TeamSwitcher teams={data.teams} onMenuSelect={handleSelection}/>
	</Sidebar.Header>
	<Sidebar.Content>
		<NavMain items={data.navMain} onMenuSelect={handleSelection}/>
		<NavProjects projects={data.projects} onMenuSelect={handleSelection}/>
	</Sidebar.Content>
	<Sidebar.Footer>
		<NavUser user={data.user} onMenuSelect={handleSelection}/>
	</Sidebar.Footer>
	<Sidebar.Rail />
</Sidebar.Root>
