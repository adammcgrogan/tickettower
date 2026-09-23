<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		api,
		errorMessage,
		minutesLabel,
		type Channel,
		type Panel,
		type Role,
		type SetupProblem,
		type Ticket,
		type TicketType
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { emojiText, hoursLabel } from '$lib/format';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	const guildId = page.params.id!;
	const base = `/servers/${guildId}`;

	let types = $state<TicketType[] | null>(null);
	let channels = $state<Channel[]>([]);
	let roles = $state<Role[]>([]);
	let panels = $state<Panel[]>([]);
	let open = $state<Ticket[] | null>(null);
	let problems = $state<SetupProblem[]>([]);
	let error = $state('');

	async function load() {
		error = '';
		api<{ problems: SetupProblem[] }>(`/guilds/${guildId}/setup-check`)
			.then((r) => (problems = r.problems))
			.catch(() => {});
		// Extras: the list works without them.
		api<Ticket[]>(`/guilds/${guildId}/tickets?status=open&limit=200`)
			.then((t) => (open = t))
			.catch(() => {});
		try {
			[types, panels] = await Promise.all([
				api<TicketType[]>(`/guilds/${guildId}/ticket-types`),
				api<Panel[]>(`/guilds/${guildId}/panels`).catch(() => [])
			]);
			[channels, roles] = await Promise.all([
				api<Channel[]>(`/guilds/${guildId}/channels`).catch(() => []),
				api<Role[]>(`/guilds/${guildId}/roles`).catch(() => [])
			]);
		} catch (e) {
			error = errorMessage(e);
		}
	}
	onMount(load);

	// Problems that stop members opening a type, with the setup check's words.
	const problemFor = (t: TicketType) =>
		problems.find((p) => (p.kind === 'ticket_type' || p.kind === 'unlisted') && p.id === t.id);

	function where(t: TicketType) {
		const parent = channels.find((c) => c.id === t.parent_id);
		if (t.mode === 'thread') return parent ? `Threads in #${parent.name}` : 'Private threads';
		return parent ? `Channels in ${parent.name}` : 'Private channels';
	}

	const role = (id: string) => roles.find((r) => r.id === id);
	const hex = (n: number) => (n ? `#${n.toString(16).padStart(6, '0')}` : 'var(--color-subtle)');

	// The ticket panels a type is on, since that's where members open it.
	const panelsFor = (t: TicketType) => panels.filter((p) => p.ticket_type_ids.includes(t.id));
	const openCount = (t: TicketType) => open?.filter((x) => x.ticket_type_id === t.id).length ?? null;
	const waitingCount = (t: TicketType) =>
		open?.filter((x) => x.ticket_type_id === t.id && x.waiting_on_staff && !x.on_hold).length ?? 0;

	// The settings worth knowing at a glance; the rest are one click away.
	function extras(t: TicketType) {
		const out: string[] = [];
		if (t.questions.length) out.push(`${t.questions.length} question${t.questions.length === 1 ? '' : 's'}`);
		if (t.auto_close_hours) out.push(`closes after ${hoursLabel(t.auto_close_hours)} quiet`);
		if (t.required_role_ids.length) out.push('some roles only');
		if (t.auto_assign) out.push('auto-assigns');
		return out;
	}
</script>

<svelte:head><title>Ticket types · {APP_NAME}</title></svelte:head>

<PageHeader
	title="Ticket types"
	description="The kinds of help members can ask for. Each one decides where its tickets open, who handles them and what members are asked first."
>
	{#snippet actions()}
		{#if types?.length}
			<a href="{base}/ticket-types/new" class="btn btn-primary">
				<Icon name="plus" size={15} /> New ticket type
			</a>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-8">
	{#if error}
		<LoadError message={error} onretry={load} />
	{:else if types === null}
		<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
			{#each Array(3) as _, i (i)}<div class="h-[76px] animate-pulse bg-surface"></div>{/each}
		</div>
	{:else if types.length === 0}
		<EmptyState icon="tag" title="No ticket types yet">
			A ticket type decides where its tickets open, who handles them, and what members are asked first.
			{#snippet action()}
				<a href="{base}/ticket-types/new" class="btn btn-primary">
					<Icon name="plus" size={15} /> New ticket type
				</a>
			{/snippet}
		</EmptyState>
	{:else}
		<div class="overflow-hidden rounded-xl border border-border bg-surface">
			<div
				class="hidden grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)_5rem_2.5rem] gap-6 border-b border-border px-5 py-2.5 text-xs text-subtle lg:grid"
			>
				<span>Ticket type</span>
				<span>Handled by</span>
				<span>Members open it from</span>
				<span class="text-right">Open now</span>
				<span></span>
			</div>
			<ul class="divide-y divide-border">
				{#each types as t (t.id)}
					{@const problem = problemFor(t)}
					{@const on = panelsFor(t)}
					{@const count = openCount(t)}
					{@const waiting = waitingCount(t)}
					{@const more = extras(t)}
					<li class="group relative">
						<div
							class="grid gap-x-6 gap-y-3 px-5 py-4 transition-colors group-hover:bg-elevated/40 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)_5rem_2.5rem] lg:items-center"
						>
							<div class="flex min-w-0 items-center gap-3.5">
								<span class="grid size-10 shrink-0 place-items-center rounded-lg bg-elevated text-lg" aria-hidden="true">
									{#if t.emoji}{emojiText(t.emoji)}{:else}<Icon name="tag" class="text-muted" />{/if}
								</span>
								<div class="min-w-0">
									<a
										href="{base}/ticket-types/{t.id}"
										class="block truncate font-medium after:absolute after:inset-0 after:content-['']"
									>
										{t.name}
									</a>
									<p class="mt-0.5 truncate text-sm text-muted">
										{where(t)}{t.reply_target_minutes ? `, reply within ${minutesLabel(t.reply_target_minutes)}` : ''}
									</p>
									{#if more.length}
										<p class="mt-0.5 truncate text-xs text-subtle">
											{more.join(', ').replace(/^./, (c) => c.toUpperCase())}
										</p>
									{/if}
								</div>
							</div>

							<div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 pl-[3.375rem] text-sm lg:pl-0">
								{#if t.support_role_ids.length === 0}
									<span class="text-muted">Server managers only</span>
								{:else}
									{#each t.support_role_ids.slice(0, 2) as id (id)}
										<span class="flex min-w-0 items-center gap-1.5">
											<span class="size-2 shrink-0 rounded-full" style="background:{hex(role(id)?.color ?? 0)}"></span>
											<span class="truncate">{role(id)?.name ?? 'Deleted role'}</span>
										</span>
									{/each}
									{#if t.support_role_ids.length > 2}
										<span class="text-muted">and {t.support_role_ids.length - 2} more</span>
									{/if}
								{/if}
							</div>

							<div class="min-w-0 pl-[3.375rem] text-sm lg:pl-0">
								{#if problem}
									<span class="flex items-start gap-1.5 text-danger" title={problem.detail}>
										<Icon name="alert" size={14} class="mt-0.5" />
										<span class="min-w-0">{problem.title}</span>
									</span>
								{:else if on.length === 0}
									<span class="flex items-center gap-1.5 text-muted">
										<span class="size-2 rounded-full border border-border-strong"></span> Not on a ticket panel
									</span>
								{:else}
									<span class="flex min-w-0 items-center gap-1.5">
										<span class="size-2 shrink-0 rounded-full {on.some((p) => p.message_id) ? 'bg-success' : 'border border-border-strong'}"></span>
										<span class="truncate">{on.length === 1 ? on[0].title : `${on.length} ticket panels`}</span>
									</span>
								{/if}
							</div>

							<div class="hidden text-right text-sm tabular-nums lg:block">
								{#if count === null}
									<span class="text-subtle">—</span>
								{:else if count === 0}
									<span class="text-subtle">0</span>
								{:else}
									<a
										href="{base}/tickets?type={t.id}"
										class="relative z-10 {waiting ? 'font-medium text-accent-ink' : ''} hover:underline"
										title={waiting ? `${waiting} waiting on your team` : undefined}
									>
										{count}
									</a>
								{/if}
							</div>

							<div class="hidden justify-end lg:flex">
								<a
									href="{base}/ticket-types/new?from={t.id}"
									class="btn btn-ghost relative z-10 size-8 p-0"
									aria-label="Duplicate {t.name}"
									title="Duplicate"
								>
									<Icon name="copy" size={15} />
								</a>
							</div>
						</div>
					</li>
				{/each}
			</ul>
		</div>
		{#if panels.length === 0}
			<p class="mt-4 text-sm text-muted">
				Members open tickets from a ticket panel.
				<a href="{base}/panels/new" class="text-fg underline-offset-4 hover:underline">Create one</a> to put these
				types in front of them.
			</p>
		{/if}
	{/if}
</div>
