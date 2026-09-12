<script lang="ts">
	import Icon, { type IconName } from '../Icon.svelte';
	import TicketStub from '../TicketStub.svelte';

	/** A static copy of the dashboard's Home page, filled with example tickets. */
	const nav: { label: string; icon: IconName; active?: boolean }[] = [
		{ label: 'Home', icon: 'home', active: true },
		{ label: 'Tickets', icon: 'inbox' },
		{ label: 'Analytics', icon: 'chart' }
	];

	const figures = [
		{ label: 'Waiting on your team', value: '3', loud: true },
		{ label: 'Open tickets', value: '11' },
		{ label: 'First response', value: '6m', hint: 'Median, last 7 days' },
		{ label: 'Satisfaction', value: '4.8', hint: 'Out of 5' }
	];

	const waiting = [
		{ number: 42, name: 'alex', type: 'Billing', claimed: 'Priya', wait: '14m' },
		{ number: 39, name: 'mira.k', type: 'General support', claimed: null, wait: '9m' },
		{ number: 41, name: 'jonah', type: 'Report a member', claimed: 'Sam', wait: '2m' }
	];
</script>

<div class="overflow-hidden rounded-2xl border border-border bg-bg shadow-2xl shadow-black/40" aria-hidden="true">
	<div class="flex">
		<aside class="hidden w-44 shrink-0 border-r border-border p-3 md:block">
			<div class="flex items-center gap-2 rounded-lg px-2 py-1.5">
				<span class="grid size-6 place-items-center rounded-md bg-elevated text-xs font-semibold">N</span>
				<span class="truncate text-sm font-medium">Northwind</span>
			</div>
			<ul class="mt-4 space-y-0.5">
				{#each nav as n (n.label)}
					<li
						class="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm {n.active
							? 'bg-elevated text-fg'
							: 'text-muted'}"
					>
						<Icon name={n.icon} size={15} />
						{n.label}
					</li>
				{/each}
			</ul>
			<p class="mt-5 px-2 text-xs text-subtle">Setup</p>
			<ul class="mt-1.5 space-y-0.5 text-sm text-muted">
				<li class="flex items-center gap-2.5 px-2 py-1.5"><Icon name="tag" size={15} />Ticket types</li>
				<li class="flex items-center gap-2.5 px-2 py-1.5"><Icon name="panel" size={15} />Ticket buttons</li>
				<li class="flex items-center gap-2.5 px-2 py-1.5"><Icon name="settings" size={15} />Settings</li>
			</ul>
		</aside>

		<div class="min-w-0 flex-1 p-5 sm:p-6">
			<div class="font-display text-3xl font-bold">Home</div>

			<dl class="mt-5 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border">
				{#each figures as f (f.label)}
					<div class="bg-surface p-4">
						<dt class="truncate text-xs text-muted">{f.label}</dt>
						<dd class="mt-2 font-display text-3xl leading-none font-bold tabular-nums {f.loud ? 'text-accent' : ''}">
							{f.value}
						</dd>
						{#if f.hint}<dd class="mt-1.5 truncate text-[11px] text-subtle">{f.hint}</dd>{/if}
					</div>
				{/each}
			</dl>

			<div class="mt-6 text-sm font-semibold">Waiting on your team</div>
			<ul class="mt-3 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
				{#each waiting as t (t.number)}
					<li class="flex items-center gap-3 px-3.5 py-3 sm:gap-4">
						<TicketStub number={t.number} tone="waiting" />
						<div class="min-w-0 flex-1">
							<div class="truncate text-sm font-medium">{t.name}</div>
							<div class="truncate text-xs text-muted">{t.type}</div>
						</div>
						<div class="hidden text-sm sm:block {t.claimed ? 'text-muted' : 'text-subtle'}">
							{t.claimed ? `Claimed by ${t.claimed}` : 'Unclaimed'}
						</div>
						<div class="flex w-16 items-center justify-end gap-1.5 text-sm text-accent tabular-nums">
							<Icon name="clock" size={14} />
							{t.wait}
						</div>
					</li>
				{/each}
			</ul>
		</div>
	</div>
</div>
