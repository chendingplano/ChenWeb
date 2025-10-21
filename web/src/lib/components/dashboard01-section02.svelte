<script lang="ts">
	import TrendingDownIcon from "@tabler/icons-svelte/icons/trending-down";
	import TrendingUpIcon from "@tabler/icons-svelte/icons/trending-up";
	import { Badge } from "$lib/components/ui/badge/index.js";
	import * as Card from "$lib/components/ui/card/index.js";
	import { Content } from "./ui/dialog";
    import Echart02 from "$lib/components/echart-02.svelte";
</script>

<!-- ✅ Flex container that wraps on small screens -->
<div class="flex flex-wrap gap-4 w-full">
	<Card.Root class="min-w-[240px] flex-1 basis-0">
		<Card.Header>
			<Card.Description>Total Revenue</Card.Description>
			<Card.Title class="text-2xl font-semibold tabular-nums">
				$1,250.00
			</Card.Title>
			<Card.Action>
				<Badge variant="outline">
					<TrendingUpIcon />
					+12.5%
				</Badge>
			</Card.Action>
		</Card.Header>
		<Card.Footer class="flex-col items-start gap-1.5 text-sm">
			<div class="line-clamp-1 flex gap-2 font-medium">
				Trending up this month <TrendingUpIcon class="size-4" />
			</div>
			<div class="text-muted-foreground">Visitors for the last year</div>
		</Card.Footer>
	</Card.Root>
	
	<Card.Root class="min-w-[240px] flex-1 basis-0">
		<Card.Header>
			<Card.Description>New Customers</Card.Description>
			<Card.Title class="text-2xl font-semibold tabular-nums">
				1,234
			</Card.Title>
			<Card.Action>
				<Badge variant="outline">
					<TrendingDownIcon />
					-20%
				</Badge>
			</Card.Action>
		</Card.Header>
		<Card.Footer class="flex-col items-start gap-1.5 text-sm">
			<div class="line-clamp-1 flex gap-2 font-medium">
				Down 20% this period <TrendingDownIcon class="size-4" />
			</div>
			<div class="text-muted-foreground">Acquisition needs attention</div>
		</Card.Footer>
	</Card.Root>
	
	<Card.Root class="min-w-[240px] flex-1 basis-0">
		<Card.Header>
			<Card.Description>Active Accounts</Card.Description>
			<Card.Title class="text-2xl font-semibold tabular-nums">
				45,678
			</Card.Title>
			<Card.Action>
				<Badge variant="outline">
					<TrendingUpIcon />
					+12.5%
				</Badge>
			</Card.Action>
		</Card.Header>
		<Card.Footer class="flex-col items-start gap-1.5 text-sm">
			<div class="line-clamp-1 flex gap-2 font-medium">
				Strong user retention <TrendingUpIcon class="size-4" />
			</div>
			<div class="text-muted-foreground">Engagement exceed targets</div>
		</Card.Footer>
	</Card.Root>
	
	<Card.Root class="min-w-[240px] flex-1 basis-0">
        <Card.Content class="pt-0 h-full">
            <div>
                <Echart02 />
            </div>
        </Card.Content>
	</Card.Root>
</div>