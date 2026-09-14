<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, getMe, type Transcript, type User } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL } from '$lib/format';
	import GuildIcon from '$lib/components/GuildIcon.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import TicketSummary from '$lib/components/TicketSummary.svelte';
	import TranscriptView from '$lib/components/TranscriptView.svelte';

	let data = $state<Transcript | null>(null);
	let error = $state('');
	let user = $state<User | null>(null);

	onMount(async () => {
		user = await getMe();
		try {
			data = await api<Transcript>(`/transcripts/${page.params.ticketId}`);
		} catch (e) {
			error = errorMessage(e);
		}
	});
</script>

<svelte:head>
	<title>{data ? `Ticket #${data.ticket.number} · ${data.guild.name}` : 'Transcript'} · {APP_NAME}</title>
</svelte:head>

<main class="mx-auto max-w-4xl px-5 py-10">
	{#if user}
		<div class="mb-4 flex justify-end">
			<a href="/me/tickets" class="inline-flex items-center gap-1.5 text-sm text-muted hover:text-fg">
				<Icon name="inbox" size={14} /> My tickets
			</a>
		</div>
	{/if}
	{#if error}
		<div class="mx-auto max-w-md py-16 text-center">
			<h1 class="font-display text-3xl font-bold">Transcript unavailable</h1>
			<p class="mt-2 text-sm text-muted">
				It may not exist, or you may not have access. Transcripts are visible to the member who opened
				the ticket and the server's support team.
			</p>
		</div>
	{:else if !data}
		<div class="h-40 animate-pulse rounded-xl bg-surface"></div>
		<div class="mt-4 h-96 animate-pulse rounded-xl bg-surface"></div>
	{:else}
		{@const t = data.ticket}
		<div class="mb-4 flex items-center gap-2.5 text-sm text-muted">
			<GuildIcon name={data.guild.name || 'Server'} url={data.guild.icon_url} size={24} />
			{data.guild.name}
		</div>
		<TicketSummary ticket={t} staff={data.guild.can_manage}>
			{#snippet actions()}
				{#if t.status === 'open'}
					<a
						href={discordURL(data!.guild.id, t.channel_id)}
						target="_blank"
						rel="noopener"
						class="btn btn-secondary h-8 px-3"
					>
						Open in Discord <Icon name="external" size={13} />
					</a>
				{/if}
				<a href="/api/transcripts/{t.id}/download" class="btn btn-secondary h-8 px-3">
					Download <Icon name="download" size={13} />
				</a>
				{#if data!.guild.can_manage}
					<a href="/servers/{data!.guild.id}/tickets?t={t.id}" class="btn btn-ghost h-8 px-3">
						View in dashboard
					</a>
				{/if}
			{/snippet}
		</TicketSummary>
		<div class="mt-4">
			<TranscriptView
				messages={data.messages}
				notes={data.notes ?? []}
				openerId={t.opener_id}
				openerName={t.opener_name}
				roles={data.roles}
				channels={data.channels}
			/>
		</div>
		<p class="mt-3 text-center text-xs text-subtle">
			Attachment links are hosted by Discord and may stop working after a while.
		</p>
	{/if}
</main>
