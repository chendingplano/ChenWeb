<script lang="ts">
	// Date picker: a text-like trigger that opens a calendar popover.
	// `value` is an ISO date string (YYYY-MM-DD) or '' when empty; min/max use the same format.
	import { Calendar, Popover } from 'bits-ui';
	import { parseDate, type DateValue } from '@internationalized/date';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import XIcon from '@lucide/svelte/icons/x';
	import { getLocale } from '$lib/paraglide/runtime.js';

	let {
		value = $bindable(''),
		min = undefined,
		max = undefined,
		darkMode = true,
		ariaLabel = '',
		clearLabel = 'Clear',
		placeholder = 'yyyy-mm-dd',
		width = '140px'
	}: {
		value?: string;
		min?: string;
		max?: string;
		darkMode?: boolean;
		ariaLabel?: string;
		clearLabel?: string;
		placeholder?: string;
		width?: string;
	} = $props();

	function toDate(v: string | undefined): DateValue | undefined {
		if (!v) return undefined;
		try {
			return parseDate(v);
		} catch {
			return undefined;
		}
	}

	let open = $state(false);
	let calendarValue = $derived(toDate(value));
	let theme = $derived(
		darkMode
			? '--dp-bg:#1F2333;--dp-surface:#252A3A;--dp-border:#2D3348;--dp-accent:#818CF8;--dp-text:#E2E8F0;--dp-muted:#64748B'
			: '--dp-bg:#FFFFFF;--dp-surface:#ECEEF2;--dp-border:#E4E6EB;--dp-accent:#6366F1;--dp-text:#111827;--dp-muted:#9CA3AF'
	);
</script>

<Popover.Root bind:open>
	<span class="dp-wrap" style="{theme};width:{width}">
		<Popover.Trigger class="dp-trigger" aria-label={ariaLabel}>
			<span class:dp-empty={!value}>{value || placeholder}</span>
			<CalendarIcon size={14} />
		</Popover.Trigger>
		{#if value}
			<button type="button" class="dp-clear" aria-label={clearLabel} onclick={() => (value = '')}>
				<XIcon size={12} />
			</button>
		{/if}
	</span>
	<Popover.Portal>
		<Popover.Content class="dp-content" style={theme} sideOffset={4} align="start">
			<Calendar.Root
				type="single"
				locale={getLocale()}
				weekdayFormat="short"
				fixedWeeks
				value={calendarValue}
				onValueChange={(v) => {
					value = v ? v.toString() : '';
					open = false;
				}}
				minValue={toDate(min)}
				maxValue={toDate(max)}
			>
				{#snippet children({ months, weekdays })}
					<Calendar.Header class="dp-header">
						<Calendar.PrevButton class="dp-nav"><ChevronLeftIcon size={14} /></Calendar.PrevButton>
						<Calendar.Heading class="dp-heading" />
						<Calendar.NextButton class="dp-nav"><ChevronRightIcon size={14} /></Calendar.NextButton>
					</Calendar.Header>
					{#each months as month (month.value.toString())}
						<Calendar.Grid class="dp-grid">
							<Calendar.GridHead>
								<Calendar.GridRow class="dp-row">
									{#each weekdays as day (day)}
										<Calendar.HeadCell class="dp-head-cell">{day}</Calendar.HeadCell>
									{/each}
								</Calendar.GridRow>
							</Calendar.GridHead>
							<Calendar.GridBody>
								{#each month.weeks as week, i (i)}
									<Calendar.GridRow class="dp-row">
										{#each week as date (date.toString())}
											<Calendar.Cell {date} month={month.value} class="dp-cell">
												<Calendar.Day class="dp-day" />
											</Calendar.Cell>
										{/each}
									</Calendar.GridRow>
								{/each}
							</Calendar.GridBody>
						</Calendar.Grid>
					{/each}
				{/snippet}
			</Calendar.Root>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>

<style>
	.dp-wrap {
		position: relative;
		display: inline-flex;
	}
	:global(.dp-trigger) {
		display: flex;
		width: 100%;
		align-items: center;
		justify-content: space-between;
		gap: 6px;
		padding: 5px 24px 5px 6px;
		border-radius: 4px;
		border: 1px solid var(--dp-border);
		background: var(--dp-surface);
		color: var(--dp-text);
		text-align: left;
		user-select: none;
	}
	.dp-empty {
		color: var(--dp-muted);
	}
	.dp-clear {
		position: absolute;
		right: 24px;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		color: var(--dp-muted);
	}
	:global(.dp-trigger svg) {
		position: absolute;
		right: 6px;
	}
	:global(.dp-content) {
		z-index: 1000;
		padding: 10px;
		border-radius: 8px;
		border: 1px solid var(--dp-border);
		background: var(--dp-bg);
		color: var(--dp-text);
		font-size: 12px;
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.35);
	}
	:global(.dp-header) {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 6px;
	}
	:global(.dp-heading) {
		font-weight: 600;
	}
	:global(.dp-nav) {
		display: flex;
		padding: 4px;
		border-radius: 4px;
	}
	:global(.dp-nav:hover) {
		background: var(--dp-surface);
	}
	:global(.dp-grid) {
		border-collapse: collapse;
	}
	:global(.dp-row) {
		display: flex;
	}
	:global(.dp-head-cell),
	:global(.dp-cell) {
		width: 32px;
		text-align: center;
	}
	:global(.dp-head-cell) {
		color: var(--dp-muted);
		padding-bottom: 4px;
	}
	:global(.dp-day) {
		display: flex;
		width: 30px;
		height: 30px;
		margin: 1px;
		align-items: center;
		justify-content: center;
		border-radius: 6px;
	}
	:global(.dp-day:hover) {
		background: var(--dp-surface);
	}
	:global(.dp-day[data-today]) {
		border: 1px solid var(--dp-accent);
	}
	:global(.dp-day[data-outside-month]) {
		color: var(--dp-muted);
	}
	:global(.dp-day[data-selected]) {
		background: var(--dp-accent);
		color: white;
	}
	:global(.dp-day[data-disabled]),
	:global(.dp-day[data-unavailable]) {
		opacity: 0.35;
		pointer-events: none;
	}
</style>
