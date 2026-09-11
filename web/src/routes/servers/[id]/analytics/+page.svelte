<script lang="ts">
	import { page } from '$app/state';
	import { api, errorMessage, type Analytics } from '$lib/api';
	import { formatDuration } from '$lib/format';
	import ColumnChart from '$lib/components/ColumnChart.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Segmented from '$lib/components/Segmented.svelte';

	type Range = '7' | '30' | '90';

	const guildId = page.params.id!;
	const ranges: { value: Range; label: string }[] = [
		{ value: '7', label: '7 days' },
		{ value: '30', label: '30 days' },
		{ value: '90', label: '90 days' }
	];

	let range = $state<Range>('30');
	let data = $state<Analytics | null>(null);
	let loading = $state(true);
	let error = $state('');

	// Refetching keeps the previous render (dimmed) rather than flashing a skeleton.
	$effect(() => {
		const days = range;
		loading = true;
		api<Analytics>(`/guilds/${guildId}/analytics?days=${days}`)
			.then((a) => {
				data = a;
				error = '';
			})
			.catch((e) => (error = errorMessage(e)))
			.finally(() => (loading = false));
	});

	const dayLabel = (iso: string) =>
		new Date(iso + 'T00:00:00Z').toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });

	const daily = $derived((data?.daily ?? []).map((d) => ({ key: d.date, label: dayLabel(d.date), value: d.opened })));
	const typeMax = $derived(Math.max(1, ...(data?.by_type ?? []).map((t) => t.count)));

	const tiles = $derived(
		data
			? [
					{ label: 'Opened', value: data.opened.toLocaleString() },
					{ label: 'Closed', value: data.closed.toLocaleString() },
					{ label: 'First response', value: formatDuration(data.first_response_median_seconds), hint: 'Median' },
					{ label: 'Resolution time', value: formatDuration(data.resolution_median_seconds), hint: 'Median' },
					{
						label: 'Satisfaction',
						value: data.rating_avg != null ? `${data.rating_avg.toFixed(1)} / 5` : '—',
						hint: `${data.rating_count} rating${data.rating_count === 1 ? '' : 's'}`
					}
				]
			: []
	);
</script>

<PageHeader title="Analytics" description="How your support team is doing. Days are counted in UTC.">
	{#snippet actions()}
		<Segmented options={ranges} bind:value={range} label="Date range" />
	{/snippet}
</PageHeader>

{#if error && !data}
	<p class="card mt-6 p-5 text-sm text-danger">{error}</p>
{:else if !data}
	<div class="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
		{#each Array(5) as _, i (i)}<div class="card h-[92px] animate-pulse"></div>{/each}
	</div>
	<div class="card mt-4 h-72 animate-pulse"></div>
{:else}
	<div class="transition-opacity {loading ? 'opacity-60' : ''}">
		<div class="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
			{#each tiles as tile (tile.label)}
				<div class="card p-4">
					<div class="text-xs text-muted">{tile.label}</div>
					<div class="mt-2 text-2xl font-semibold tracking-tight">{tile.value}</div>
					<div class="mt-1 h-4 text-xs text-subtle">{tile.hint ?? ''}</div>
				</div>
			{/each}
		</div>

		<section class="card mt-4 p-5">
			<h2 class="font-medium">Tickets opened per day</h2>
			<p class="hint mt-0.5">Last {data.days} days</p>
			<div class="mt-4">
				<ColumnChart data={daily} label="Tickets opened per day over the last {data.days} days" />
			</div>
		</section>

		<div class="mt-4 grid gap-4 lg:grid-cols-2">
			<section class="card p-5">
				<h2 class="font-medium">By ticket type</h2>
				<p class="hint mt-0.5">Tickets opened in this period</p>
				{#if data.by_type.length === 0}
					<p class="mt-6 text-sm text-subtle">No tickets in this period.</p>
				{:else}
					<ul class="mt-4 space-y-3">
						{#each data.by_type as t (t.name)}
							<li>
								<div class="flex items-baseline justify-between gap-3 text-sm">
									<span class="truncate">{t.name}</span>
									<span class="text-muted tabular-nums">{t.count}</span>
								</div>
								<div class="mt-1.5 h-2 rounded-full bg-elevated">
									<div class="h-full rounded-full bg-accent" style="width:{(t.count / typeMax) * 100}%"></div>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</section>

			<section class="card overflow-hidden">
				<div class="p-5 pb-3">
					<h2 class="font-medium">Support team</h2>
					<p class="hint mt-0.5">Tickets claimed in this period</p>
				</div>
				{#if data.staff.length === 0}
					<p class="px-5 pb-5 text-sm text-subtle">Nobody has claimed a ticket in this period.</p>
				{:else}
					<div class="overflow-x-auto">
						<table class="w-full text-left text-sm">
							<thead class="border-y border-border text-xs text-muted">
								<tr>
									<th class="px-5 py-2 font-medium">Member</th>
									<th class="px-3 py-2 text-right font-medium">Claimed</th>
									<th class="px-3 py-2 text-right font-medium">Closed</th>
									<th class="px-5 py-2 text-right font-medium">Rating</th>
								</tr>
							</thead>
							<tbody class="divide-y divide-border">
								{#each data.staff as s (s.user_id)}
									<tr>
										<td class="max-w-[12rem] truncate px-5 py-2.5">{s.name}</td>
										<td class="px-3 py-2.5 text-right tabular-nums">{s.claimed}</td>
										<td class="px-3 py-2.5 text-right tabular-nums">{s.closed}</td>
										<td class="px-5 py-2.5 text-right tabular-nums {s.avg_rating == null ? 'text-subtle' : ''}">
											{s.avg_rating != null ? s.avg_rating.toFixed(1) : '—'}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</section>
		</div>
	</div>
{/if}
