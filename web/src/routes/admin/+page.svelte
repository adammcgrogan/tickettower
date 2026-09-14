<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, errorMessage, send, type AdminGuild, type AdminOverview } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { timeAgo } from '$lib/format';
	import { toast } from '$lib/toast.svelte';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import GuildIcon from '$lib/components/GuildIcon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Segmented from '$lib/components/Segmented.svelte';

	let overview = $state<AdminOverview | null>(null);
	let guilds = $state<AdminGuild[] | null>(null);
	let error = $state('');
	let notFound = $state(false);

	const tierOptions: { value: 'free' | 'premium'; label: string }[] = [
		{ value: 'free', label: 'Free' },
		{ value: 'premium', label: 'Premium' }
	];

	async function load() {
		error = '';
		notFound = false;
		try {
			const [o, g] = await Promise.all([
				api<AdminOverview>('/admin/overview'),
				api<AdminGuild[]>('/admin/guilds')
			]);
			overview = o;
			guilds = g;
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) {
				notFound = true;
			} else {
				error = errorMessage(e);
			}
		}
	}

	onMount(load);

	async function setTier(g: AdminGuild, tier: 'free' | 'premium') {
		if (g.tier === tier) return;
		const prev = g.tier;
		g.tier = tier;
		try {
			await api(`/admin/guilds/${g.id}/tier`, send('POST', { tier }));
			toast(`${g.name} is now on ${tier}.`);
		} catch (e) {
			g.tier = prev;
			toast(errorMessage(e), 'error');
		}
	}
</script>

<svelte:head><title>Admin · {APP_NAME}</title></svelte:head>

<AppHeader />

<main class="mx-auto max-w-6xl px-5 py-12">
	<PageHeader title="Admin" description="Install and usage stats across every server. Only you can see this." />

	{#if notFound}
		<div class="mt-8 rounded-xl border border-border bg-surface px-6 py-16 text-center">
			<p class="font-medium">Not found</p>
		</div>
	{:else if error && !overview}
		<div class="mt-8 rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
			<p class="text-danger">{error}</p>
			<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
				Try again
			</button>
		</div>
	{:else if !overview || !guilds}
		<div class="mt-8 h-[164px] animate-pulse rounded-xl bg-surface"></div>
		<div class="mt-4 h-80 animate-pulse rounded-xl bg-surface"></div>
	{:else}
		<dl class="mt-8 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border lg:grid-cols-4">
			<div class="bg-surface p-5">
				<dt class="text-sm text-muted">Active servers</dt>
				<dd class="mt-3 font-display text-4xl leading-none font-bold tabular-nums">
					{overview.active_guilds.toLocaleString()}
				</dd>
				<dd class="mt-2 text-xs text-subtle">{overview.total_guilds.toLocaleString()} ever installed</dd>
			</div>
			<div class="bg-surface p-5">
				<dt class="text-sm text-muted">Joined, last 30 days</dt>
				<dd class="mt-3 font-display text-4xl leading-none font-bold tabular-nums">
					{overview.joined_last_30.toLocaleString()}
				</dd>
				<dd class="mt-2 text-xs text-subtle">{overview.left_last_30.toLocaleString()} left</dd>
			</div>
			<div class="bg-surface p-5">
				<dt class="text-sm text-muted">Premium</dt>
				<dd class="mt-3 font-display text-4xl leading-none font-bold tabular-nums">
					{overview.premium_guilds.toLocaleString()}
				</dd>
				<dd class="mt-2 text-xs text-subtle">{overview.free_guilds.toLocaleString()} free</dd>
			</div>
			<div class="bg-surface p-5">
				<dt class="text-sm text-muted">Using the bot, last 30 days</dt>
				<dd class="mt-3 font-display text-4xl leading-none font-bold tabular-nums">
					{overview.using_guilds_last_30.toLocaleString()}
				</dd>
				<dd class="mt-2 text-xs text-subtle">
					{overview.tickets_opened_last_30.toLocaleString()} tickets opened, of {overview.active_guilds.toLocaleString()}
					active
				</dd>
			</div>
		</dl>

		<section class="card mt-4 overflow-hidden">
			<div class="p-5 pb-3">
				<h2 class="font-semibold">Servers</h2>
				<p class="hint mt-0.5">Every server the bot has ever joined, most recently joined first</p>
			</div>
			{#if guilds.length === 0}
				<p class="px-5 pb-5 text-sm text-subtle">Nobody has added the bot yet.</p>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full text-left text-sm">
						<thead class="border-y border-border text-xs text-muted">
							<tr>
								<th class="px-5 py-2 font-medium">Server</th>
								<th class="px-3 py-2 font-medium">Plan</th>
								<th class="px-3 py-2 text-right font-medium">Joined</th>
								<th class="px-3 py-2 text-right font-medium">Tickets</th>
								<th class="px-5 py-2 text-right font-medium">Last activity</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-border">
							{#each guilds as g (g.id)}
								<tr class={g.left_at ? 'opacity-50' : ''}>
									<td class="px-5 py-2.5">
										<div class="flex items-center gap-2.5">
											<GuildIcon name={g.name} url={g.icon_url} size={24} />
											<span class="max-w-[14rem] truncate">{g.name}</span>
											{#if g.left_at}
												<span class="rounded-full border border-border px-2 py-0.5 text-xs text-subtle">Left</span>
											{/if}
										</div>
									</td>
									<td class="px-3 py-2.5">
										<Segmented
											options={tierOptions}
											value={g.tier}
											label="Plan for {g.name}"
											onchange={(v) => setTier(g, v)}
										/>
									</td>
									<td class="px-3 py-2.5 text-right tabular-nums text-muted" title={g.joined_at}>
										{timeAgo(g.joined_at)}
									</td>
									<td class="px-3 py-2.5 text-right tabular-nums">
										{g.tickets_total.toLocaleString()}
										<span class="text-subtle">({g.tickets_last_30} in 30d)</span>
									</td>
									<td class="px-5 py-2.5 text-right tabular-nums {g.last_ticket_at ? '' : 'text-subtle'}">
										{g.last_ticket_at ? timeAgo(g.last_ticket_at) : 'Never'}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>
	{/if}
</main>
