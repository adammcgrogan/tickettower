<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, type Channel, type Panel, type TicketType } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL } from '$lib/format';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PanelPreview from '$lib/components/PanelPreview.svelte';

	const guildId = page.params.id!;
	const base = `/servers/${guildId}`;

	let panels = $state<Panel[] | null>(null);
	let types = $state<TicketType[]>([]);
	let channels = $state<Channel[]>([]);
	let error = $state('');

	async function load() {
		error = '';
		try {
			[panels, types] = await Promise.all([
				api<Panel[]>(`/guilds/${guildId}/panels`),
				api<TicketType[]>(`/guilds/${guildId}/ticket-types`)
			]);
			channels = await api<Channel[]>(`/guilds/${guildId}/channels`).catch(() => []);
		} catch (e) {
			error = errorMessage(e);
		}
	}
	onMount(load);

	const channelName = (id: string | null) => channels.find((c) => c.id === id)?.name;
	const typesOf = (p: Panel) =>
		p.ticket_type_ids.map((id) => types.find((t) => t.id === id)).filter((t): t is TicketType => !!t);
	// Types members can't pick anywhere, because no ticket panel shows them.
	const offPanel = $derived(types.filter((t) => !(panels ?? []).some((p) => p.ticket_type_ids.includes(t.id))));
</script>

<svelte:head><title>Ticket panels · {APP_NAME}</title></svelte:head>

<PageHeader
	title="Ticket panels"
	description="The messages members click to open a ticket. Post them in any channel, as buttons or a dropdown menu."
>
	{#snippet actions()}
		{#if panels?.length && types.length > 0}
			<a href="{base}/panels/new" class="btn btn-primary">
				<Icon name="plus" size={15} /> New ticket panel
			</a>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-8">
	{#if error}
		<LoadError message={error} onretry={load} />
	{:else if panels === null}
		<div class="grid gap-5 md:grid-cols-2" aria-busy="true">
			{#each Array(2) as _, i (i)}<div class="h-72 animate-pulse rounded-xl bg-surface"></div>{/each}
		</div>
	{:else if panels.length === 0}
		{#if types.length === 0}
			<EmptyState icon="panel" title="Create a ticket type first">
				Each button opens one of your ticket types, so you'll need at least one.
				{#snippet action()}
					<a href="{base}/ticket-types/new" class="btn btn-primary">
						<Icon name="plus" size={15} /> New ticket type
					</a>
				{/snippet}
			</EmptyState>
		{:else}
			<EmptyState icon="panel" title="No ticket panels yet">
				Design a message with buttons and post it in a channel so members can open tickets.
				{#snippet action()}
					<a href="{base}/panels/new" class="btn btn-primary">
						<Icon name="plus" size={15} /> New ticket panel
					</a>
				{/snippet}
			</EmptyState>
		{/if}
	{:else}
		{#if offPanel.length}
			<div class="mb-6 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-border px-4 py-3 text-sm">
				<Icon name="alert" size={15} class="text-accent-ink" />
				<span class="min-w-0 flex-1">
					{offPanel.length === 1
						? `${offPanel[0].name} isn't on any ticket panel, so members can't open it.`
						: `${offPanel.length} ticket types aren't on any ticket panel, so members can't open them.`}
				</span>
				<a href="{base}/panels/{panels[0].id}" class="text-muted hover:text-fg">Add to {panels[0].title}</a>
			</div>
		{/if}

		<ul class="grid gap-5 md:grid-cols-2">
			{#each panels as p (p.id)}
				{@const name = channelName(p.channel_id)}
				{@const count = `${p.ticket_type_ids.length} ${p.style === 'dropdown' ? (p.ticket_type_ids.length === 1 ? 'option' : 'options') : p.ticket_type_ids.length === 1 ? 'button' : 'buttons'}`}
				<li class="group relative flex flex-col overflow-hidden rounded-xl border border-border bg-surface transition-colors hover:border-border-strong">
					<!-- The message itself, as members see it. Scaled down and clipped so tall panels stay tidy. -->
					<div class="relative h-56 overflow-hidden bg-[#313338]" aria-hidden="true">
						<div class="pointer-events-none origin-top-left scale-[0.8] p-1" style="width:125%">
							<PanelPreview
								title={p.title}
								description={p.description}
								color={p.color}
								style={p.style}
								imageUrl={p.image_url}
								thumbnailUrl={p.thumbnail_url}
								placeholder={p.placeholder}
								types={typesOf(p)}
							/>
						</div>
						<div class="absolute inset-x-0 bottom-0 h-10 bg-gradient-to-t from-[#313338]"></div>
					</div>
					<div class="flex flex-1 items-center gap-3 border-t border-border px-4 py-3.5">
						<div class="min-w-0 flex-1">
							<a
								href="{base}/panels/{p.id}"
								class="block truncate font-medium after:absolute after:inset-0 after:content-['']"
							>
								{p.title || 'Untitled ticket panel'}
							</a>
							<p class="mt-0.5 flex items-center gap-1.5 text-sm text-muted">
								<span
									class="size-2 shrink-0 rounded-full {p.message_id ? 'bg-success' : 'border border-border-strong'}"
								></span>
								<span class="truncate">
									{p.message_id ? `Live in #${name ?? 'a deleted channel'}` : 'Not published yet'}, {count}
								</span>
							</p>
						</div>
						{#if p.message_id && p.channel_id}
							<a
								href={discordURL(guildId, p.channel_id, p.message_id)}
								target="_blank"
								rel="noopener"
								class="btn btn-ghost relative z-10 h-8 px-2.5 text-xs"
							>
								View in Discord <Icon name="external" size={12} />
							</a>
						{:else}
							<a href="{base}/panels/{p.id}?publish=1" class="btn btn-secondary relative z-10 h-8 px-3 text-xs">
								Publish
							</a>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>
