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
		type TicketType,
		type User
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { formatDuration, ticketState } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import QuickSetup from '$lib/components/QuickSetup.svelte';
	import SetupProblems from '$lib/components/SetupProblems.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());
	const getUser = getContext<() => User | null>('user');
	const me = $derived(getUser());
	const base = $derived(`/servers/${guild.id}`);
	const admin = $derived(atLeast(guild.level, 'admin'));

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
			[open, types, panels] = await Promise.all([
				api<Ticket[]>(`${g}/tickets?status=open`),
				api<TicketType[]>(`${g}/ticket-types`),
				api<Panel[]>(`${g}/panels`)
			]);
			// Figures and channel names are extras: Home works without them.
			api<Analytics>(`${g}/analytics?days=7`)
				.then((a) => (week = a))
				.catch(() => {});
			if (admin || (types.length === 0 && panels.length === 0)) {
				[channels, roles] = await Promise.all([
					api<Channel[]>(`${g}/channels`).catch(() => []),
					types.length === 0 && panels.length === 0 ? api<Role[]>(`${g}/roles`).catch(() => []) : []
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
		const timer = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(timer);
	});

	// Longest wait first. Tickets on hold aren't waiting on the team.
	const waiting = $derived(
		(open ?? [])
			.filter((t) => t.waiting_on_staff && !t.on_hold)
			.sort((a, b) => (a.waiting_since ?? a.last_activity_at).localeCompare(b.waiting_since ?? b.last_activity_at))
	);
	const mine = $derived((open ?? []).filter((t) => me && t.claimed_by === me.id));
	const waitedFor = (t: Ticket) => formatDuration((now - new Date(t.waiting_since ?? t.last_activity_at).getTime()) / 1000);
	const isOverdue = (t: Ticket) => {
		const at = overdueAt(t, types ?? []);
		return at !== null && at <= now;
	};
	const overdue = $derived(waiting.filter(isOverdue).length);
	const onMember = $derived((open ?? []).filter((t) => !t.waiting_on_staff && !t.on_hold).length);

	const loaded = $derived(open !== null && types !== null && panels !== null);
	const fresh = $derived(types?.length === 0 && panels?.length === 0);
	const live = $derived(!!panels?.some((p) => p.message_id));
	const plural = (n: number, one: string, many: string) => (n === 1 ? `1 ${one}` : `${n} ${many}`);

	const greeting = $derived.by(() => {
		const h = new Date(now).getHours();
		const part = h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening';
		return me ? `${part}, ${me.display_name}` : part;
	});

	// The one sentence that says how things stand.
	const headline = $derived.by(() => {
		if (!open) return null;
		if (waiting.length) return `${plural(waiting.length, 'ticket is', 'tickets are')} waiting on your team`;
		if (open.length) return "Nobody's waiting on your team";
		return 'All quiet';
	});
	const summary = $derived.by(() => {
		if (!open) return '';
		if (waiting.length) {
			const first = waiting[0];
			let s = `${first.opener_name} has waited longest, for ${waitedFor(first)}.`;
			if (overdue) s += ` ${plural(overdue, 'is', 'are')} past the reply target.`;
			return s;
		}
		if (open.length) return `${plural(onMember, 'open ticket is', 'open tickets are')} waiting on the member${open.length - onMember ? `, and ${open.length - onMember} on hold` : ''}.`;
		return 'There are no open tickets right now.';
	});

	// The last seven days next to the seven before.
	function change(now: number | null | undefined, before: number | null | undefined, lowerIsBetter = false) {
		if (now == null || before == null || before === 0) return null;
		const pct = Math.round(((now - before) / before) * 100);
		if (pct === 0) return { text: 'Same as the week before', good: null };
		const more = pct > 0;
		return {
			text: `${Math.abs(pct)}% ${lowerIsBetter ? (more ? 'slower' : 'faster') : more ? 'more' : 'fewer'} than the week before`,
			good: lowerIsBetter ? !more : null
		};
	}
	const figures = $derived.by(() => {
		if (!week) return null;
		const s = week.summary;
		const p = week.previous;
		return [
			{ label: 'Tickets opened', value: String(s.opened), note: change(s.opened, p?.opened) },
			{
				label: 'First response',
				value: formatDuration(s.first_response_median_seconds),
				note: change(s.first_response_median_seconds, p?.first_response_median_seconds, true)
			},
			...(s.target_measured
				? [
						{
							label: 'Replied within target',
							value: `${Math.round((s.target_met / s.target_measured) * 100)}%`,
							note: null
						}
					]
				: []),
			{
				label: 'Satisfaction',
				value: s.rating_avg != null ? s.rating_avg.toFixed(1) : '—',
				note: {
					text: s.rating_count ? `Out of 5, from ${plural(s.rating_count, 'rating', 'ratings')}` : 'No ratings yet',
					good: null
				}
			}
		];
	});

	const channelName = (id: string | null) => channels.find((c) => c.id === id)?.name;

	// Setup that's been started but not finished: an ordered checklist.
	const steps = $derived([
		{
			title: 'Create a ticket type',
			body: 'Choose who handles it, and whether it opens a channel or a private thread.',
			done: !!types?.length,
			href: `${base}/ticket-types/new`
		},
		{
			title: 'Design your ticket panel',
			body: 'The message members click to open a ticket.',
			done: !!panels?.length,
			href: `${base}/panels/new`
		},
		{
			title: 'Publish it',
			body: 'Post the buttons in a channel and start taking tickets.',
			done: live,
			href: panels?.length ? `${base}/panels/${panels[0].id}` : `${base}/panels`
		}
	]);
	const nextStep = $derived(steps.findIndex((s) => !s.done));
</script>

<svelte:head><title>{guild.name} · {APP_NAME}</title></svelte:head>

{#snippet row(t: Ticket, showWait: boolean)}
	{@const state = ticketState(t)}
	<li>
		<a
			href="{base}/tickets?t={t.id}"
			class="-mx-3 flex items-center gap-4 rounded-lg px-3 py-3 transition-colors hover:bg-surface"
		>
			<TicketStub number={t.number} tone={state.tone} />
			<div class="min-w-0 flex-1">
				<div class="truncate text-sm font-medium">{t.opener_name}</div>
				<div class="truncate text-sm text-muted">
					{t.type_name}{showWait ? '' : `, ${state.label.charAt(0).toLowerCase() + state.label.slice(1)}`}
				</div>
			</div>
			{#if showWait}
				<div class="hidden text-sm sm:block {t.claimed_by_name ? 'text-muted' : 'text-subtle'}">
					{t.claimed_by_name ? `Claimed by ${t.claimed_by_name}` : 'Unclaimed'}
				</div>
				<div
					class="w-20 text-right text-sm tabular-nums {isOverdue(t) ? 'text-danger' : 'text-accent-ink'}"
					title="{isOverdue(t) ? 'Past the reply target. ' : ''}Waiting since {new Date(t.waiting_since ?? t.last_activity_at).toLocaleString()}"
				>
					{waitedFor(t)}
				</div>
			{/if}
			<Icon name="chevron-right" size={15} class="text-subtle" />
		</a>
	</li>
{/snippet}

{#if error}
	<LoadError message={error} onretry={load} />
{:else if loaded && fresh && admin}
	<header>
		<p class="text-sm text-muted">{greeting}</p>
		<h1 class="mt-2 font-display text-4xl leading-[1.05] font-bold sm:text-5xl">
			Let's get {guild.name} taking tickets
		</h1>
		<p class="mt-3 max-w-xl text-sm text-muted">
			Answer a few questions and your ticket panel goes live in one step. Everything can be changed afterwards.
		</p>
	</header>
	<div class="mt-8">
		<QuickSetup guildId={guild.id} {channels} {roles} ondone={load} />
	</div>
{:else}
	<header class="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
		<div class="min-w-0">
			<p class="text-sm text-muted">{greeting}</p>
			{#if headline}
				<h1 class="mt-2 font-display text-4xl leading-[1.05] font-bold sm:text-5xl">{headline}</h1>
				<p class="mt-3 text-sm {overdue ? 'text-danger' : 'text-muted'}">{summary}</p>
			{:else}
				<div class="mt-3 h-11 w-96 max-w-full animate-pulse rounded-lg bg-elevated"></div>
				<div class="mt-4 h-4 w-64 animate-pulse rounded bg-elevated"></div>
			{/if}
		</div>
		{#if open}
			<div class="flex shrink-0 gap-2">
				{#if waiting.length}
					<a href="{base}/tickets?t={waiting[0].id}" class="btn btn-primary">
						Start with #{String(waiting[0].number).padStart(4, '0')}
					</a>
				{/if}
				<a href="{base}/tickets" class="btn btn-secondary">Open the inbox</a>
			</div>
		{/if}
	</header>

	{#if problems.length}
		<div class="mt-10">
			<SetupProblems {problems} guildId={guild.id} onrecheck={checkSetup} />
		</div>
	{/if}

	{#if loaded && !live && admin}
		<section class="mt-10 rounded-xl border border-border bg-surface">
			<div class="border-b border-border px-5 py-4">
				<h2 class="font-semibold">Finish setting up</h2>
				<p class="mt-0.5 text-sm text-muted">Members can open tickets once your ticket panel is published.</p>
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
									? 'border-accent text-accent-ink'
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

	<div class="mt-12 grid gap-12 lg:grid-cols-[minmax(0,1fr)_17rem] lg:gap-16">
		<div class="min-w-0 space-y-12">
			<section>
				<div class="flex items-baseline justify-between gap-4 border-b border-border pb-3">
					<h2 class="font-semibold">Waiting on your team</h2>
					{#if waiting.length}<span class="text-sm text-subtle tabular-nums">{waiting.length}</span>{/if}
				</div>
				{#if !open}
					<div class="mt-2 space-y-1" aria-busy="true">
						{#each Array(3) as _, i (i)}<div class="h-14 animate-pulse rounded-lg bg-surface"></div>{/each}
					</div>
				{:else if waiting.length === 0}
					<p class="mt-4 rounded-lg border border-dashed border-border px-4 py-6 text-center text-sm text-muted">
						Every member has a reply. New messages show up here, longest wait first.
					</p>
				{:else}
					<ul class="mt-1">
						{#each waiting.slice(0, 6) as t (t.id)}{@render row(t, true)}{/each}
					</ul>
					{#if waiting.length > 6}
						<a href="{base}/tickets" class="mt-2 inline-block text-sm text-muted hover:text-fg">
							See all {waiting.length} in the inbox
						</a>
					{/if}
				{/if}
			</section>

			{#if open && me}
				<section>
					<div class="flex items-baseline justify-between gap-4 border-b border-border pb-3">
						<h2 class="font-semibold">Claimed by you</h2>
						{#if mine.length}<span class="text-sm text-subtle tabular-nums">{mine.length}</span>{/if}
					</div>
					{#if mine.length === 0}
						<p class="mt-4 text-sm text-muted">
							You haven't claimed any open tickets. Claim one and it stays here until it's closed.
						</p>
					{:else}
						<ul class="mt-1">
							{#each mine as t (t.id)}{@render row(t, false)}{/each}
						</ul>
					{/if}
				</section>
			{/if}
		</div>

		<aside class="space-y-10">
			<section>
				<div class="flex items-baseline justify-between gap-4 border-b border-border pb-3">
					<h2 class="font-semibold">Last 7 days</h2>
					<a href="{base}/analytics" class="text-sm text-muted hover:text-fg">Analytics</a>
				</div>
				{#if figures}
					<dl class="divide-y divide-border">
						{#each figures as f (f.label)}
							<div class="py-3.5">
								<div class="flex items-baseline justify-between gap-3">
									<dt class="text-sm text-muted">{f.label}</dt>
									<dd class="font-display text-2xl leading-none font-bold tabular-nums">{f.value}</dd>
								</div>
								{#if f.note}
									<dd class="mt-1 text-xs {f.note.good === false ? 'text-danger' : 'text-subtle'}">{f.note.text}</dd>
								{/if}
							</div>
						{/each}
					</dl>
				{:else}
					<div class="mt-3 space-y-3" aria-busy="true">
						{#each Array(4) as _, i (i)}<div class="h-8 animate-pulse rounded bg-surface"></div>{/each}
					</div>
				{/if}
			</section>

			{#if admin && panels && panels.length}
				<section>
					<div class="flex items-baseline justify-between gap-4 border-b border-border pb-3">
						<h2 class="font-semibold">Ticket panels</h2>
						<a href="{base}/panels" class="text-sm text-muted hover:text-fg">Manage</a>
					</div>
					<ul class="divide-y divide-border">
						{#each panels.slice(0, 5) as p (p.id)}
							<li>
								<a href="{base}/panels/{p.id}" class="group flex items-center gap-3 py-3">
									<span
										class="size-2 shrink-0 rounded-full {p.message_id ? 'bg-success' : 'border border-border-strong'}"
									></span>
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm group-hover:text-fg">{p.title || 'Untitled ticket panel'}</span>
										<span class="block truncate text-xs text-subtle">
											{#if p.message_id}
												Live{channelName(p.channel_id) ? ` in #${channelName(p.channel_id)}` : ''}
											{:else}
												Not published yet
											{/if}
										</span>
									</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		</aside>
	</div>
{/if}
