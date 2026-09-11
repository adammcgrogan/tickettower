<script lang="ts">
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, type Guild, type Ticket, type Transcript } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL, ticketState, timeAgo } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Segmented from '$lib/components/Segmented.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';
	import TicketSummary from '$lib/components/TicketSummary.svelte';
	import TranscriptView from '$lib/components/TranscriptView.svelte';

	type Filter = 'open' | 'closed' | 'all';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());

	const filters: { value: Filter; label: string }[] = [
		{ value: 'open', label: 'Open' },
		{ value: 'closed', label: 'Closed' },
		{ value: 'all', label: 'All' }
	];

	// The filter and selected ticket live in the URL, so links from Home and
	// the back button work.
	const filter = $derived<Filter>((page.url.searchParams.get('status') as Filter) || 'open');
	const selectedId = $derived(Number(page.url.searchParams.get('t')) || null);

	function hrefWith(changes: Record<string, string | null>) {
		const params = new URLSearchParams(page.url.searchParams);
		for (const [k, v] of Object.entries(changes)) {
			if (v === null) params.delete(k);
			else params.set(k, v);
		}
		const qs = params.toString();
		return page.url.pathname + (qs ? `?${qs}` : '');
	}

	let tickets = $state<Ticket[] | null>(null);
	let error = $state('');
	let query = $state('');
	let typeFilter = $state('');

	function load() {
		const status = filter === 'all' ? '' : filter;
		tickets = null;
		error = '';
		api<Ticket[]>(`/guilds/${guild.id}/tickets?status=${status}`)
			.then((t) => (tickets = t))
			.catch((e) => (error = errorMessage(e)));
	}
	$effect(load);

	const typeNames = $derived([...new Set((tickets ?? []).map((t) => t.type_name))].sort());

	const shown = $derived.by(() => {
		const q = query.trim().toLowerCase().replace(/^#0*/, '');
		const list = (tickets ?? []).filter(
			(t) =>
				(!typeFilter || t.type_name === typeFilter) &&
				(!q ||
					String(t.number).includes(q) ||
					t.opener_name.toLowerCase().includes(q) ||
					t.type_name.toLowerCase().includes(q) ||
					(t.claimed_by_name ?? '').toLowerCase().includes(q))
		);
		if (filter !== 'open') return list;
		// Tickets waiting on the team come first, longest wait at the top.
		const waiting = list.filter((t) => t.waiting_on_staff);
		waiting.sort((a, b) => a.last_activity_at.localeCompare(b.last_activity_at));
		return [...waiting, ...list.filter((t) => !t.waiting_on_staff)];
	});

	// --- The selected ticket ---

	let detail = $state<Transcript | null>(null);
	let detailError = $state('');
	// Closed tickets don't change, so their transcripts are kept.
	const closedCache = new Map<number, Transcript>();

	$effect(() => {
		const id = selectedId;
		detail = null;
		detailError = '';
		if (!id) return;
		const hit = closedCache.get(id);
		if (hit) {
			detail = hit;
			return;
		}
		let stale = false;
		api<Transcript>(`/transcripts/${id}`)
			.then((d) => {
				if (d.ticket.status === 'closed') closedCache.set(id, d);
				if (!stale) detail = d;
			})
			.catch((e) => {
				if (!stale) detailError = errorMessage(e);
			});
		return () => {
			stale = true;
		};
	});

	const emptyText = $derived(
		filter === 'open'
			? { title: 'No open tickets', body: "You're all caught up." }
			: { title: 'No tickets yet', body: 'Tickets show up here once members start opening them.' }
	);
</script>

<svelte:head><title>Tickets · {guild.name} · {APP_NAME}</title></svelte:head>

<PageHeader title="Tickets" description="Everything members have opened. Pick a ticket to read the conversation." />

<div class="mt-6 flex-wrap items-center gap-2 {selectedId ? 'hidden lg:flex' : 'flex'}">
	<Segmented
		options={filters}
		value={filter}
		label="Show tickets"
		onchange={(v) => goto(hrefWith({ status: v === 'open' ? null : v, t: null }), { replaceState: true, noScroll: true })}
	/>
	<label class="relative block w-full sm:w-72">
		<span class="sr-only">Search tickets</span>
		<Icon
			name="search"
			size={15}
			class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle"
		/>
		<input bind:value={query} placeholder="Search by number, member or type" class="input pl-9" />
	</label>
	{#if typeNames.length > 1}
		<select bind:value={typeFilter} class="input w-auto" aria-label="Ticket type">
			<option value="">All ticket types</option>
			{#each typeNames as name (name)}<option value={name}>{name}</option>{/each}
		</select>
	{/if}
</div>

<div class="mt-4 grid items-start gap-6 lg:grid-cols-[minmax(0,23rem)_minmax(0,1fr)]">
	<div class={selectedId ? 'hidden lg:block' : ''}>
		{#if error}
			<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
				<p class="text-danger">{error}</p>
				<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
					Try again
				</button>
			</div>
		{:else if tickets === null}
			<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
				{#each Array(6) as _, i (i)}<div class="h-[84px] animate-pulse bg-surface"></div>{/each}
			</div>
		{:else if tickets.length === 0}
			<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
				<p class="font-medium">{emptyText.title}</p>
				<p class="mx-auto mt-1 max-w-xs text-sm text-muted">{emptyText.body}</p>
			</div>
		{:else if shown.length === 0}
			<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
				<p class="font-medium">No tickets match</p>
				<p class="mt-1 text-sm text-muted">Try a different search or ticket type.</p>
			</div>
		{:else}
			<ul
				class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface lg:sticky lg:top-6 lg:max-h-[calc(100dvh-3rem)] lg:overflow-y-auto"
			>
				{#each shown as t (t.id)}
					{@const state = ticketState(t)}
					{@const active = t.id === selectedId}
					<li>
						<a
							href={hrefWith({ t: String(t.id) })}
							data-sveltekit-noscroll
							data-sveltekit-replacestate
							aria-current={active ? 'true' : undefined}
							class="relative flex gap-3 px-4 py-3 transition-colors {active
								? 'bg-elevated'
								: 'hover:bg-elevated/50'}"
						>
							{#if active}<span class="absolute inset-y-0 left-0 w-0.5 bg-accent"></span>{/if}
							<TicketStub number={t.number} tone={state.tone} size="sm" />
							<div class="min-w-0 flex-1">
								<div class="flex items-baseline justify-between gap-2">
									<span class="truncate text-sm font-medium">{t.opener_name}</span>
									<span class="shrink-0 text-xs text-subtle">
										{timeAgo(t.closed_at ?? t.last_activity_at)}
									</span>
								</div>
								<div class="truncate text-sm text-muted">{t.type_name}</div>
								<div class="mt-1 truncate text-xs {state.tone === 'waiting' ? 'text-accent' : 'text-subtle'}">
									{state.label}{t.claimed_by_name && t.status === 'open' ? `, claimed by ${t.claimed_by_name}` : ''}
								</div>
							</div>
						</a>
					</li>
				{/each}
			</ul>
			{#if tickets.length >= 200}
				<p class="mt-3 text-xs text-subtle">Showing the newest 200 tickets.</p>
			{/if}
		{/if}
	</div>

	<div class="min-w-0 {selectedId ? '' : 'hidden lg:block'}">
		{#if !selectedId}
			<div
				class="grid min-h-96 place-items-center rounded-xl border border-dashed border-border px-6 text-center"
			>
				<div>
					<p class="font-medium">Pick a ticket</p>
					<p class="mt-1 text-sm text-muted">Its details and the conversation show up here.</p>
				</div>
			</div>
		{:else}
			<a
				href={hrefWith({ t: null })}
				data-sveltekit-noscroll
				class="mb-4 inline-flex items-center gap-1.5 text-sm text-muted hover:text-fg lg:hidden"
			>
				<Icon name="arrow-left" size={14} /> All tickets
			</a>
			{#if detailError}
				<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm text-danger">{detailError}</div>
			{:else if !detail}
				<div class="h-40 animate-pulse rounded-xl bg-surface"></div>
				<div class="mt-4 h-80 animate-pulse rounded-xl bg-surface"></div>
			{:else}
				{@const t = detail.ticket}
				<TicketSummary ticket={t}>
					{#snippet actions()}
						{#if t.status === 'open'}
							<a
								href={discordURL(guild.id, t.channel_id)}
								target="_blank"
								rel="noopener"
								class="btn btn-secondary h-8 px-3"
							>
								Open in Discord <Icon name="external" size={13} />
							</a>
						{/if}
						<a href="/transcripts/{t.id}" target="_blank" rel="noopener" class="btn btn-ghost h-8 px-3">
							Transcript page <Icon name="external" size={13} />
						</a>
					{/snippet}
				</TicketSummary>
				<div class="mt-4">
					<TranscriptView messages={detail.messages} openerId={t.opener_id} openerName={t.opener_name} />
				</div>
			{/if}
		{/if}
	</div>
</div>
