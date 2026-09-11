<script lang="ts">
	import { page } from '$app/state';
	import { api, errorMessage, type Ticket } from '$lib/api';
	import { discordURL, timeAgo } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Segmented from '$lib/components/Segmented.svelte';

	type Filter = 'open' | 'closed' | 'all';

	const guildId = page.params.id!;
	const filters: { value: Filter; label: string }[] = [
		{ value: 'open', label: 'Open' },
		{ value: 'closed', label: 'Closed' },
		{ value: 'all', label: 'All' }
	];

	let filter = $state<Filter>('open');
	let tickets = $state<Ticket[] | null>(null);
	let error = $state('');

	$effect(() => {
		const status = filter === 'all' ? '' : filter;
		tickets = null;
		error = '';
		api<Ticket[]>(`/guilds/${guildId}/tickets?status=${status}`)
			.then((t) => (tickets = t))
			.catch((e) => (error = errorMessage(e)));
	});
</script>

<PageHeader title="Tickets" description="Every ticket opened in your server, newest first.">
	{#snippet actions()}
		<Segmented options={filters} bind:value={filter} label="Filter tickets" />
	{/snippet}
</PageHeader>

<div class="mt-6">
	{#if error}
		<p class="card p-5 text-sm text-danger">{error}</p>
	{:else if tickets === null}
		<div class="card h-48 animate-pulse"></div>
	{:else if tickets.length === 0}
		<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
			<div class="mx-auto grid size-10 place-items-center rounded-xl bg-elevated text-muted">
				<Icon name="inbox" />
			</div>
			<p class="mt-4 font-medium">
				{filter === 'open' ? 'No open tickets' : filter === 'closed' ? 'No closed tickets yet' : 'No tickets yet'}
			</p>
			<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
				{filter === 'open' ? "You're all caught up." : 'Tickets will show up here once members start opening them.'}
			</p>
		</div>
	{:else}
		<div class="card overflow-x-auto">
			<table class="w-full min-w-[640px] text-left text-sm">
				<thead class="border-b border-border text-xs text-muted">
					<tr>
						<th class="px-4 py-2.5 font-medium">Ticket</th>
						<th class="px-4 py-2.5 font-medium">Opened by</th>
						<th class="px-4 py-2.5 font-medium">Claimed by</th>
						<th class="px-4 py-2.5 font-medium">Opened</th>
						<th class="px-4 py-2.5 font-medium">Status</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-border">
					{#each tickets as t (t.id)}
						<tr class="transition-colors hover:bg-elevated/50">
							<td class="px-4 py-3">
								<a href="/transcripts/{t.id}" class="font-medium hover:underline" title="View transcript">
									<span class="text-muted tabular-nums">#{t.number}</span>
									{t.type_name}
								</a>
							</td>
							<td class="px-4 py-3">{t.opener_name}</td>
							<td class="px-4 py-3 {t.claimed_by_name ? '' : 'text-subtle'}">
								{t.claimed_by_name ?? 'Unclaimed'}
							</td>
							<td class="px-4 py-3 whitespace-nowrap text-muted" title={new Date(t.opened_at).toLocaleString()}>
								{timeAgo(t.opened_at)}
							</td>
							<td class="px-4 py-3">
								{#if t.status === 'open'}
									<a
										href={discordURL(guildId, t.channel_id)}
										target="_blank"
										rel="noopener"
										class="inline-flex items-center gap-1.5 rounded-md bg-success/10 px-2 py-0.5 text-xs font-medium text-success hover:bg-success/20"
									>
										Open <Icon name="external" size={11} />
									</a>
								{:else}
									<span
										class="inline-flex max-w-[16rem] items-center gap-1.5 truncate rounded-md bg-elevated px-2 py-0.5 text-xs text-muted"
										title={t.close_reason}
									>
										Closed{t.close_reason ? ` · ${t.close_reason}` : ''}
									</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
