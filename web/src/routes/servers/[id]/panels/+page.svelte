<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, intToHex, type Channel, type Panel, type TicketType } from '$lib/api';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	const guildId = page.params.id!;

	let panels = $state<Panel[] | null>(null);
	let typeCount = $state(0);
	let channels = $state<Channel[]>([]);
	let error = $state('');

	async function load() {
		error = '';
		try {
			const [p, t] = await Promise.all([
				api<Panel[]>(`/guilds/${guildId}/panels`),
				api<TicketType[]>(`/guilds/${guildId}/ticket-types`)
			]);
			panels = p;
			typeCount = t.length;
			channels = await api<Channel[]>(`/guilds/${guildId}/channels`).catch(() => []);
		} catch (e) {
			error = errorMessage(e);
		}
	}
	onMount(load);

	const channelName = (id: string | null) => channels.find((c) => c.id === id)?.name ?? 'unknown';
</script>

<PageHeader
	title="Ticket panels"
	description="The messages members click to open a ticket. Post them in any channel, as buttons or a dropdown menu."
>
	{#snippet actions()}
		{#if panels?.length && typeCount > 0}
			<a href="/servers/{guildId}/panels/new" class="btn btn-primary">
				<Icon name="plus" size={15} /> New ticket panel
			</a>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-8 max-w-4xl">
	{#if error}
		<LoadError message={error} onretry={load} />
	{:else if panels === null}
		<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
			{#each Array(2) as _, i (i)}<div class="h-[76px] animate-pulse bg-surface"></div>{/each}
		</div>
	{:else if panels.length === 0}
		{#if typeCount === 0}
			<EmptyState icon="panel" title="Create a ticket type first">
				Each button opens one of your ticket types, so you'll need at least one.
				{#snippet action()}
					<a href="/servers/{guildId}/ticket-types/new" class="btn btn-primary">
						<Icon name="plus" size={15} /> New ticket type
					</a>
				{/snippet}
			</EmptyState>
		{:else}
			<EmptyState icon="panel" title="No ticket panels yet">
				Design a message with buttons and post it in a channel so members can open tickets.
				{#snippet action()}
					<a href="/servers/{guildId}/panels/new" class="btn btn-primary">
						<Icon name="plus" size={15} /> New ticket panel
					</a>
				{/snippet}
			</EmptyState>
		{/if}
	{:else}
		<ul class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
			{#each panels as p (p.id)}
				<li>
					<a
						href="/servers/{guildId}/panels/{p.id}"
						class="group flex items-center gap-4 px-4 py-4 transition-colors hover:bg-elevated/60"
					>
						<!-- The panel's embed colour, as it shows in Discord. -->
						<span class="grid size-10 shrink-0 place-items-center rounded-lg bg-elevated">
							<span class="h-5 w-1 rounded-full" style="background:{intToHex(p.color)}"></span>
						</span>
						<div class="min-w-0 flex-1">
							<div class="truncate font-medium">{p.title}</div>
							<div class="mt-0.5 flex items-center gap-1.5 text-sm text-muted">
								{#if p.message_id}
									<span class="size-1.5 shrink-0 rounded-full bg-success"></span>
									<span class="truncate">Live in #{channelName(p.channel_id)}</span>
								{:else}
									<span class="size-1.5 shrink-0 rounded-full bg-subtle"></span>
									<span>Not published yet</span>
								{/if}
							</div>
						</div>
						<ul class="hidden gap-1.5 sm:flex">
							<li class="rounded-md border border-border px-2 py-0.5 text-xs text-muted">
								{p.ticket_type_ids.length} ticket type{p.ticket_type_ids.length === 1 ? '' : 's'}
							</li>
							<li class="rounded-md border border-border px-2 py-0.5 text-xs text-muted">
								{p.style === 'dropdown' ? 'Dropdown menu' : 'Buttons'}
							</li>
						</ul>
						<Icon name="arrow-right" class="text-subtle transition-colors group-hover:text-fg" />
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
