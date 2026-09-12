<script lang="ts">
	import { getContext, onMount } from 'svelte';
	import {
		api,
		atLeast,
		errorMessage,
		overdueAt,
		type Analytics,
		type Channel,
		type Guild,
		type Panel,
		type Role,
		type SetupProblem,
		type Ticket,
		type TicketType
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { formatDuration } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import QuickSetup from '$lib/components/QuickSetup.svelte';
	import SetupProblems from '$lib/components/SetupProblems.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());
	const base = $derived(`/servers/${guild.id}`);

	let open = $state<Ticket[] | null>(null);
	let types = $state<TicketType[] | null>(null);
	let panels = $state<Panel[] | null>(null);
	let week = $state<Analytics | null>(null);
	let channels = $state<Channel[]>([]);
	let roles = $state<Role[]>([]);
	let problems = $state<SetupProblem[]>([]);
	let error = $state('');
	let now = $state(Date.now());

	async function load() {
		error = '';
		const g = `/guilds/${guild.id}`;
		checkSetup();
		try {
			[open, types, panels, week] = await Promise.all([
				api<Ticket[]>(`${g}/tickets?status=open`),
				api<TicketType[]>(`${g}/ticket-types`),
				api<Panel[]>(`${g}/panels`),
				api<Analytics>(`${g}/analytics?days=7`)
			]);
			if (types.length === 0 && panels.length === 0) {
				[channels, roles] = await Promise.all([
					api<Channel[]>(`${g}/channels`).catch(() => []),
					api<Role[]>(`${g}/roles`).catch(() => [])
				]);
			}
		} catch (e) {
			error = errorMessage(e);
		}
	}

	// A bonus: Home still works if Discord can't be reached for the check.
	function checkSetup() {
		return api<{ problems: SetupProblem[] }>(`/guilds/${guild.id}/setup-check`)
			.then((r) => (problems = r.problems))
			.catch(() => {});
	}

	onMount(() => {
		load();
		const timer = setInterval(() => (now = Date.now()), 60_000);
		return () => clearInterval(timer);
	});

	// Longest wait first. Tickets on hold aren't waiting on the team.
	const waiting = $derived(
		(open ?? [])
			.filter((t) => t.waiting_on_staff && !t.on_hold)
			.sort((a, b) => (a.waiting_since ?? a.last_activity_at).localeCompare(b.waiting_since ?? b.last_activity_at))
	);
	const waitedFor = (t: Ticket) => formatDuration((now - new Date(t.waiting_since ?? t.last_activity_at).getTime()) / 1000);
	const isOverdue = (t: Ticket) => {
		const at = overdueAt(t, types ?? []);
		return at !== null && at <= now;
	};
	const overdue = $derived(waiting.filter(isOverdue).length);
	const onHold = $derived((open ?? []).filter((t) => t.on_hold).length);
	const hasTargets = $derived(!!types?.some((t) => t.reply_target_minutes));

	const loaded = $derived(open !== null && types !== null && panels !== null);
	const fresh = $derived(types?.length === 0 && panels?.length === 0);
	const live = $derived(!!panels?.some((p) => p.message_id));

	const figures = $derived([
		{
			label: 'Waiting on your team',
			value: open && String(waiting.length),
			loud: waiting.length > 0,
			hint: hasTargets ? (overdue ? `${overdue} past the reply target` : 'None past the reply target') : undefined
		},
		{
			label: 'Open tickets',
			value: open && (open.length >= 200 ? '200+' : String(open.length)),
			hint: onHold ? `${onHold} on hold` : undefined
		},
		{
			label: 'First response',
			value: week && formatDuration(week.summary.first_response_median_seconds),
			hint: 'Median, last 7 days'
		},
		{
			label: 'Satisfaction',
			value: week && (week.summary.rating_avg != null ? week.summary.rating_avg.toFixed(1) : '—'),
			hint: week?.summary.rating_count
				? `Out of 5, from ${week.summary.rating_count} rating${week.summary.rating_count === 1 ? '' : 's'}`
				: 'No ratings in the last 7 days'
		}
	]);

	// Setup that's been started but not finished: an ordered checklist.
	const steps = $derived([
		{
			title: 'Create a ticket type',
			body: 'Choose who handles it, and whether it opens a channel or a private thread.',
			done: !!types?.length,
			href: `${base}/ticket-types/new`
		},
		{
			title: 'Design your ticket buttons',
			body: 'The message members click to open a ticket.',
			done: !!panels?.length,
			href: `${base}/buttons/new`
		},
		{
			title: 'Publish them',
			body: 'Post the buttons in a channel and start taking tickets.',
			done: live,
			href: panels?.length ? `${base}/buttons/${panels[0].id}` : `${base}/buttons`
		}
	]);
	const nextStep = $derived(steps.findIndex((s) => !s.done));

</script>

<svelte:head><title>{guild.name} · {APP_NAME}</title></svelte:head>

<h1 class="font-display text-4xl leading-none font-bold">{guild.name}</h1>
<p class="mt-2.5 text-sm text-muted">
	{fresh ? `Let's get ${APP_NAME} taking tickets.` : 'Who needs a reply, and how support is going.'}
</p>

{#if error}
	<div class="mt-8 rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
		<p class="text-danger">{error}</p>
		<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
			Try again
		</button>
	</div>
{:else if loaded && fresh && atLeast(guild.level, 'admin')}
	<div class="mt-8">
		<QuickSetup guildId={guild.id} {channels} {roles} ondone={load} />
	</div>
{:else}
	{#if problems.length}
		<div class="mt-8">
			<SetupProblems {problems} guildId={guild.id} onrecheck={checkSetup} />
		</div>
	{/if}

	{#if loaded && !live && atLeast(guild.level, 'admin')}
		<section class="mt-8 rounded-xl border border-border bg-surface">
			<div class="border-b border-border px-5 py-4">
				<h2 class="font-semibold">Finish setting up</h2>
				<p class="mt-0.5 text-sm text-muted">Members can open tickets once your ticket buttons are published.</p>
			</div>
			<ol class="divide-y divide-border">
				{#each steps as step, i (step.title)}
					<li class="flex items-start gap-4 px-5 py-4">
						{#if step.done}
							<span class="mt-0.5 grid size-6 shrink-0 place-items-center rounded-full bg-success/15 text-success">
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
							<div class="text-sm font-medium {step.done ? 'text-muted' : ''}">{step.title}</div>
							<div class="mt-0.5 text-sm text-muted">{step.body}</div>
						</div>
						{#if i === nextStep}
							<a href={step.href} class="btn btn-primary h-8 shrink-0 px-3">Continue</a>
						{/if}
					</li>
				{/each}
			</ol>
		</section>
	{/if}

	<dl class="mt-8 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border lg:grid-cols-4">
		{#each figures as f (f.label)}
			<div class="bg-surface p-5">
				<dt class="text-sm text-muted">{f.label}</dt>
				<dd
					class="mt-3 h-9 font-display text-4xl leading-none font-bold tabular-nums {f.loud
						? 'text-accent'
						: ''}"
				>
					{#if f.value == null}
						<div class="h-8 w-14 animate-pulse rounded bg-elevated"></div>
					{:else}
						{f.value}
					{/if}
				</dd>
				{#if f.hint}<dd class="mt-2 text-xs text-subtle">{f.hint}</dd>{/if}
			</div>
		{/each}
	</dl>

	<section class="mt-12">
		<div class="flex items-end justify-between gap-4">
			<div>
				<h2 class="text-lg font-semibold">Waiting on your team</h2>
				<p class="mt-1 text-sm text-muted">Open tickets where the member spoke last, longest wait first.</p>
			</div>
			<a href="{base}/tickets" class="shrink-0 text-sm text-muted transition-colors hover:text-fg">
				All tickets
			</a>
		</div>

		{#if !open}
			<div class="mt-4 space-y-px overflow-hidden rounded-xl border border-border">
				{#each Array(3) as _, i (i)}<div class="h-16 animate-pulse bg-surface"></div>{/each}
			</div>
		{:else if waiting.length === 0}
			<div class="mt-4 rounded-xl border border-dashed border-border px-6 py-12 text-center">
				<p class="font-medium">Nobody's waiting on you</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
					{#if open.length}
						{open.length} open ticket{open.length === 1 ? ' is' : 's are'} waiting on the member instead.
					{:else}
						There are no open tickets right now.
					{/if}
				</p>
			</div>
		{:else}
			<ul class="mt-4 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
				{#each waiting.slice(0, 8) as t (t.id)}
					<li>
						<a
							href="{base}/tickets?t={t.id}"
							class="flex items-center gap-4 px-4 py-3.5 transition-colors hover:bg-elevated/60"
						>
							<TicketStub number={t.number} tone="waiting" />
							<div class="min-w-0 flex-1">
								<div class="truncate text-sm font-medium">{t.opener_name}</div>
								<div class="truncate text-sm text-muted">{t.type_name}</div>
							</div>
							<div class="hidden text-sm sm:block {t.claimed_by_name ? 'text-muted' : 'text-subtle'}">
								{t.claimed_by_name ? `Claimed by ${t.claimed_by_name}` : 'Unclaimed'}
							</div>
							<div
								class="flex w-24 items-center justify-end gap-1.5 text-sm tabular-nums {isOverdue(t) ? 'text-danger' : 'text-accent'}"
								title="{isOverdue(t) ? 'Past the reply target. ' : ''}Waiting since {new Date(t.waiting_since ?? t.last_activity_at).toLocaleString()}"
							>
								<Icon name="clock" size={14} />
								{waitedFor(t)}
							</div>
						</a>
					</li>
				{/each}
			</ul>
			{#if waiting.length > 8}
				<a href="{base}/tickets" class="mt-3 inline-block text-sm text-muted hover:text-fg">
					See all {waiting.length} waiting tickets
				</a>
			{/if}
		{/if}
	</section>
{/if}
