<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, intToHex, type Channel, type Panel, type TicketType } from '$lib/api';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	const guildId = page.params.id!;

	let panels = $state<Panel[] | null>(null);
	let typeCount = $state(0);
	let channels = $state<Channel[]>([]);
	let error = $state('');

	onMount(async () => {
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
	});

	const channelName = (id: string | null) => channels.find((c) => c.id === id)?.name ?? 'unknown';
</script>

<PageHeader
	title="Ticket panels"
	description="The messages members click to open a ticket. Post them in any channel, as buttons or a dropdown menu."
>
	{#snippet actions()}
		{#if typeCount > 0}
			<a href="/servers/{guildId}/panels/new" class="btn btn-primary">
				<Icon name="plus" size={15} /> New ticket panel
			</a>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-6">
	{#if error}
		<p class="card p-5 text-sm text-danger">{error}</p>
	{:else if panels === null}
		<div class="grid gap-3 sm:grid-cols-2">
			{#each Array(2) as _, i (i)}
				<div class="card h-28 animate-pulse"></div>
			{/each}
		</div>
	{:else if panels.length === 0}
		<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
			<div class="mx-auto grid size-10 place-items-center rounded-xl bg-elevated text-muted">
				<Icon name="panel" />
			</div>
			{#if typeCount === 0}
				<p class="mt-4 font-medium">Create a ticket type first</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
					Each button opens one of your ticket types, so you'll need at least one.
				</p>
				<a href="/servers/{guildId}/ticket-types/new" class="btn btn-primary mt-5">
					<Icon name="plus" size={15} /> Create ticket type
				</a>
			{:else}
				<p class="mt-4 font-medium">No ticket panels yet</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
					Design a message with buttons and post it in a channel so members can open tickets.
				</p>
				<a href="/servers/{guildId}/panels/new" class="btn btn-primary mt-5">
					<Icon name="plus" size={15} /> Create ticket panel
				</a>
			{/if}
		</div>
	{:else}
		<ul class="grid gap-3 sm:grid-cols-2">
			{#each panels as p (p.id)}
				<li>
					<a
						href="/servers/{guildId}/panels/{p.id}"
						class="group card block overflow-hidden transition-colors hover:border-border-strong"
					>
						<div class="h-1" style="background:{intToHex(p.color)}"></div>
						<div class="p-4">
							<div class="flex items-start justify-between gap-3">
								<div class="truncate font-medium">{p.title}</div>
								<Icon
									name="arrow-right"
									class="mt-0.5 text-subtle transition-all group-hover:translate-x-0.5 group-hover:text-fg"
								/>
							</div>
							<div class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
								{#if p.message_id}
									<span class="inline-flex items-center gap-1.5">
										<span class="size-1.5 rounded-full bg-success"></span>
										#{channelName(p.channel_id)}
									</span>
								{:else}
									<span class="inline-flex items-center gap-1.5">
										<span class="size-1.5 rounded-full bg-subtle"></span>
										Draft
									</span>
								{/if}
								<span>{p.ticket_type_ids.length} ticket type{p.ticket_type_ids.length === 1 ? '' : 's'}</span>
								<span>{p.style === 'dropdown' ? 'Dropdown menu' : 'Buttons'}</span>
							</div>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
