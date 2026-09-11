<script lang="ts">
	import { getContext, onMount } from 'svelte';
	import { api, type Analytics, type Guild, type Panel, type Stats, type TicketType } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { formatDuration } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());

	let stats = $state<Stats | null>(null);
	let types = $state<TicketType[] | null>(null);
	let panels = $state<Panel[] | null>(null);
	let week = $state<Analytics | null>(null);

	onMount(async () => {
		const base = `/guilds/${guild.id}`;
		[stats, types, panels, week] = await Promise.all([
			api<Stats>(`${base}/stats`),
			api<TicketType[]>(`${base}/ticket-types`),
			api<Panel[]>(`${base}/panels`),
			api<Analytics>(`${base}/analytics?days=7`)
		]);
	});

	const cards = $derived([
		{ label: 'Open tickets', value: stats?.tickets.open, hint: 'Right now' },
		{ label: 'Opened', value: stats?.tickets.opened_week, hint: 'Last 7 days' },
		{
			label: 'First response',
			value: week ? formatDuration(week.first_response_median_seconds) : undefined,
			hint: 'Median, last 7 days'
		},
		{
			label: 'Satisfaction',
			value: week ? (week.rating_avg != null ? `${week.rating_avg.toFixed(1)} / 5` : '—') : undefined,
			hint: week?.rating_count ? `${week.rating_count} rating${week.rating_count === 1 ? '' : 's'}` : 'No ratings yet'
		}
	]);

	const steps = $derived.by(() => {
		const base = `/servers/${guild.id}`;
		return [
			{
				title: `Add ${APP_NAME} to your server`,
				body: 'The bot is in your server and ready to go.',
				done: true,
				href: ''
			},
			{
				title: 'Create a ticket type',
				body: 'Choose who handles it, and whether it opens a channel or a private thread.',
				done: !!types?.length,
				href: `${base}/ticket-types/new`
			},
			{
				title: 'Design a panel',
				body: 'The message members click to open a ticket.',
				done: !!panels?.length,
				href: `${base}/panels/new`
			},
			{
				title: 'Publish the panel',
				body: 'Post it in a channel and start taking tickets.',
				done: !!panels?.some((p) => p.message_id),
				href: panels?.length ? `${base}/panels/${panels[0].id}` : `${base}/panels`
			}
		];
	});

	const completed = $derived(steps.filter((s) => s.done).length);
	const nextStep = $derived(steps.findIndex((s) => !s.done));
	const loaded = $derived(types !== null && panels !== null);
</script>

<svelte:head><title>{guild.name} · {APP_NAME}</title></svelte:head>

<h1 class="text-xl font-semibold tracking-tight">Overview</h1>
<p class="mt-1 text-sm text-muted">How support is running in {guild.name}.</p>

<div class="mt-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
	{#each cards as card (card.label)}
		<div class="card p-4">
			<div class="text-xs text-muted">{card.label}</div>
			<div class="mt-2 h-8 text-2xl font-semibold tracking-tight">
				{#if card.value === undefined}
					<div class="h-7 w-10 animate-pulse rounded bg-elevated"></div>
				{:else}
					{card.value}
				{/if}
			</div>
			<div class="mt-1 text-xs text-subtle">{card.hint}</div>
		</div>
	{/each}
</div>

{#if loaded && completed < steps.length}
	<section class="card mt-8">
		<div class="flex items-center justify-between gap-4 border-b border-border px-5 py-4">
			<div>
				<h2 class="font-medium">Get started</h2>
				<p class="mt-0.5 text-xs text-muted">{completed} of {steps.length} complete</p>
			</div>
			<div class="h-1.5 w-32 overflow-hidden rounded-full bg-elevated">
				<div
					class="h-full rounded-full bg-accent transition-all duration-500"
					style="width:{(completed / steps.length) * 100}%"
				></div>
			</div>
		</div>
		<ol class="divide-y divide-border">
			{#each steps as step, i (step.title)}
				<li class="flex items-start gap-4 px-5 py-4">
					{#if step.done}
						<span class="mt-0.5 grid size-6 shrink-0 place-items-center rounded-full bg-accent text-white">
							<Icon name="check" size={13} />
						</span>
					{:else}
						<span
							class="mt-0.5 grid size-6 shrink-0 place-items-center rounded-full border text-xs tabular-nums {i ===
							nextStep
								? 'border-accent text-accent'
								: 'border-border-strong text-muted'}"
						>
							{i + 1}
						</span>
					{/if}
					<div class="min-w-0 flex-1">
						<div class="text-sm font-medium {step.done ? 'text-muted line-through decoration-subtle' : ''}">
							{step.title}
						</div>
						<div class="mt-0.5 text-sm text-muted">{step.body}</div>
					</div>
					{#if !step.done && i === nextStep}
						<a href={step.href} class="btn btn-primary h-8 shrink-0 px-3">Start</a>
					{/if}
				</li>
			{/each}
		</ol>
	</section>
{:else if loaded}
	<section class="card mt-8 flex items-center gap-4 px-5 py-4">
		<span class="grid size-8 shrink-0 place-items-center rounded-full bg-success/15 text-success">
			<Icon name="check" size={15} />
		</span>
		<div class="flex-1">
			<div class="text-sm font-medium">You're all set up</div>
			<div class="text-sm text-muted">Members can open tickets from your published panels.</div>
		</div>
		<a href="/servers/{guild.id}/tickets" class="btn btn-secondary h-8 px-3">View tickets</a>
	</section>
{/if}
