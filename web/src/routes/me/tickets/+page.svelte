<script lang="ts">
	import { onMount } from 'svelte';
	import { api, errorMessage, type MyTicket } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL, ticketState, timeAgo } from '$lib/format';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import GuildIcon from '$lib/components/GuildIcon.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import TicketStub from '$lib/components/TicketStub.svelte';

	let tickets = $state<MyTicket[] | null>(null);
	let error = $state('');

	async function load() {
		try {
			const res = await api<{ tickets: MyTicket[] }>('/me/tickets');
			tickets = res.tickets;
			error = '';
		} catch (e) {
			error = errorMessage(e);
		}
	}

	onMount(load);

	// Group by server, keeping the newest-first order the API already returns.
	const groups = $derived.by(() => {
		const out: { guild: MyTicket['guild']; tickets: MyTicket['ticket'][] }[] = [];
		for (const { ticket, guild } of tickets ?? []) {
			let g = out.find((g) => g.guild.id === guild.id);
			if (!g) {
				g = { guild, tickets: [] };
				out.push(g);
			}
			g.tickets.push(ticket);
		}
		return out;
	});
</script>

<svelte:head><title>My tickets · {APP_NAME}</title></svelte:head>

<AppHeader />

<main class="mx-auto max-w-3xl px-5 py-12">
	<h1 class="font-display text-4xl leading-none font-bold">My tickets</h1>
	<p class="mt-2.5 text-sm text-muted">Every ticket you've opened, across every server, with its transcript.</p>

	<div class="mt-8">
		{#if error}
			<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
				<p class="text-danger">{error}</p>
				<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
					Try again
				</button>
			</div>
		{:else if tickets === null}
			<div class="space-y-3">
				{#each Array(3) as _, i (i)}
					<div class="card h-32 animate-pulse"></div>
				{/each}
			</div>
		{:else if groups.length === 0}
			<div class="rounded-xl border border-dashed border-border px-6 py-16 text-center">
				<p class="font-medium">No tickets yet</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
					When you open a ticket in a server that uses {APP_NAME}, it'll show up here.
				</p>
			</div>
		{:else}
			<ul class="space-y-6">
				{#each groups as g (g.guild.id)}
					<li>
						<div class="mb-3 flex items-center gap-2.5">
							<GuildIcon name={g.guild.name} url={g.guild.icon_url} size={24} />
							<span class="font-medium">{g.guild.name}</span>
						</div>
						<ul class="card divide-y divide-border overflow-hidden">
							{#each g.tickets as t (t.id)}
								{@const state = ticketState(t, false)}
								<li class="flex flex-wrap items-center justify-between gap-3 p-4">
									<a href="/transcripts/{t.id}" class="flex min-w-0 items-center gap-3">
										<TicketStub number={t.number} tone={state.tone} />
										<div class="min-w-0">
											<div class="truncate font-medium">{t.type_name}</div>
											<div class="text-xs text-muted">
												{state.label} · {timeAgo(t.status === 'closed' && t.closed_at ? t.closed_at : t.opened_at)}
											</div>
										</div>
									</a>
									<div class="flex shrink-0 items-center gap-2">
										{#if t.status === 'open'}
											<a
												href={discordURL(g.guild.id, t.channel_id)}
												target="_blank"
												rel="noopener"
												class="btn btn-secondary h-8 px-3"
											>
												Open in Discord <Icon name="external" size={13} />
											</a>
										{/if}
										<a href="/transcripts/{t.id}" class="btn btn-ghost h-8 px-3">Transcript</a>
									</div>
								</li>
							{/each}
						</ul>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</main>
