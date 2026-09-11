<script lang="ts">
	import { page } from '$app/state';
	import { api, errorMessage, type Analytics, type TicketType } from '$lib/api';
	import { emojiText, formatDuration } from '$lib/format';
	import BarList from '$lib/components/BarList.svelte';
	import ColumnChart from '$lib/components/ColumnChart.svelte';
	import Heatmap from '$lib/components/Heatmap.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Segmented from '$lib/components/Segmented.svelte';

	type Range = '7' | '30' | '90' | '365' | 'all';
	type View = 'opened' | 'closed' | 'backlog';
	type TypeRow = Analytics['by_type'][number];

	const guildId = page.params.id!;
	const ranges: { value: Range; label: string }[] = [
		{ value: '7', label: '7 days' },
		{ value: '30', label: '30 days' },
		{ value: '90', label: '90 days' },
		{ value: '365', label: '1 year' },
		{ value: 'all', label: 'All time' }
	];
	const views: { value: View; label: string }[] = [
		{ value: 'opened', label: 'Opened' },
		{ value: 'closed', label: 'Closed' },
		{ value: 'backlog', label: 'Still open' }
	];
	// Round durations for the response-time axis.
	const durationSteps = [60, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200, 86400, 172800, 345600, 604800];

	let range = $state<Range>('30');
	let typeId = $state('');
	let view = $state<View>('opened');
	let types = $state<TicketType[]>([]);
	let data = $state<Analytics | null>(null);
	let loading = $state(true);
	let error = $state('');

	// The filter is optional, so a failure here just hides it.
	api<TicketType[]>(`/guilds/${guildId}/ticket-types`)
		.then((t) => (types = t))
		.catch(() => {});

	// Refetching keeps the previous render (dimmed) rather than flashing a
	// skeleton. Only the latest request may update the page.
	let latest = 0;
	$effect(() => {
		const query = `days=${range}${typeId ? `&type=${typeId}` : ''}`;
		const req = ++latest;
		loading = true;
		api<Analytics>(`/guilds/${guildId}/analytics?${query}`)
			.then((a) => {
				if (req !== latest) return;
				data = a;
				error = '';
			})
			.catch((e) => req === latest && (error = errorMessage(e)))
			.finally(() => req === latest && (loading = false));
	});

	const period = $derived(
		!data ? '' : data.days === 0 ? 'all time' : data.days === 365 ? 'the last year' : `the last ${data.days} days`
	);
	const before = $derived(data?.days === 365 ? 'the year before' : `the ${data?.days} days before`);

	const pct = (n: number, of: number) => (of ? `${Math.round((n / of) * 100)}%` : '—');
	const plural = (n: number, word: string) => `${n.toLocaleString()} ${word}${n === 1 ? '' : 's'}`;
	const typeName = (t: TypeRow) => (t.emoji ? `${emojiText(t.emoji)} ${t.name}` : t.name);

	/** Describes the change from the previous window, e.g. "12% faster than the 30 days before". */
	function change(cur: number | null | undefined, prev: number | null | undefined, [up, down]: [string, string]) {
		if (cur == null || prev == null || prev === 0) return undefined;
		const p = Math.round(((cur - prev) / prev) * 100);
		return p === 0 ? `Same as ${before}` : `${Math.abs(p)}% ${p > 0 ? up : down} than ${before}`;
	}

	const fmtDate = (iso: string, opts: Intl.DateTimeFormatOptions) =>
		new Date(iso + 'T00:00:00Z').toLocaleDateString(undefined, { ...opts, timeZone: 'UTC' });
	const bucketLabel = (iso: string) =>
		data?.bucket === 'month'
			? fmtDate(iso, { month: 'short', year: 'numeric' })
			: data?.bucket === 'week'
				? `Week of ${fmtDate(iso, { month: 'short', day: 'numeric' })}`
				: fmtDate(iso, { month: 'short', day: 'numeric' });

	const per = $derived(data ? `per ${data.bucket}` : '');
	const chart = $derived.by(() => {
		if (!data) return null;
		const titles: Record<View, [string, string]> = {
			opened: [`Tickets opened ${per}`, `Over ${period}`],
			closed: [`Tickets closed ${per}`, `Over ${period}`],
			backlog: ['Tickets still open', `Counted at the end of each ${data.bucket}`]
		};
		const [title, hint] = titles[view];
		return {
			title,
			hint,
			points: data.series.map((p) => ({ key: p.date, label: bucketLabel(p.date), value: p[view] }))
		};
	});

	const responseByHour = $derived(
		(data?.response_by_hour ?? []).map((v, h) => ({ key: String(h), label: `${String(h).padStart(2, '0')}:00`, value: v }))
	);
	const hasResponses = $derived(responseByHour.some((d) => d.value != null));

	const tiles = $derived.by(() => {
		if (!data) return [];
		const s = data.summary;
		const p = data.previous;
		const perTicket = s.transcripts ? (s.team_messages + s.member_messages) / s.transcripts : null;
		return [
			{
				label: 'Tickets opened',
				value: s.opened.toLocaleString(),
				hint: change(s.opened, p?.opened, ['more', 'fewer']) ?? `${s.closed.toLocaleString()} closed`
			},
			{
				label: 'Open now',
				value: data.open_now.toLocaleString(),
				hint: data.waiting_now ? `${data.waiting_now.toLocaleString()} waiting on your team` : 'None waiting on your team',
				waiting: data.waiting_now > 0
			},
			{
				label: 'First response',
				value: formatDuration(s.first_response_median_seconds),
				hint:
					change(s.first_response_median_seconds, p?.first_response_median_seconds, ['slower', 'faster']) ??
					'Median time to the first reply'
			},
			{
				label: 'Resolution time',
				value: formatDuration(s.resolution_median_seconds),
				hint:
					change(s.resolution_median_seconds, p?.resolution_median_seconds, ['slower', 'faster']) ??
					'Median time from open to close'
			},
			{
				label: 'Satisfaction',
				value: s.rating_avg != null ? `${s.rating_avg.toFixed(1)} / 5` : '—',
				hint: s.rating_count
					? `${plural(s.rating_count, 'rating')}, ${pct(s.rating_count, s.closed)} of closed tickets`
					: 'No ratings yet'
			},
			{
				label: 'Solved in one reply',
				value: pct(s.one_touch, s.transcripts),
				hint: s.transcripts ? `Of ${plural(s.transcripts, 'closed ticket')}` : 'No closed tickets yet'
			},
			{
				label: 'Messages per ticket',
				value: perTicket != null ? perTicket.toFixed(1) : '—',
				hint: s.transcripts
					? `Team ${(s.team_messages / s.transcripts).toFixed(1)}, member ${(s.member_messages / s.transcripts).toFixed(1)}`
					: 'No closed tickets yet'
			},
			{
				label: 'Closed without a reply',
				value: pct(s.closed_unanswered, s.closed),
				hint: s.closed ? plural(s.closed_unanswered, 'ticket') : 'No closed tickets yet'
			}
		];
	});

	const closures = $derived.by(() => {
		if (!data) return [];
		const c = data.closures;
		const total = c.team + c.member + c.auto + c.deleted;
		return [
			{ key: 'team', label: 'By your team', value: c.team },
			{ key: 'member', label: 'By the member', value: c.member },
			{ key: 'auto', label: 'Auto-closed for inactivity', value: c.auto },
			{ key: 'deleted', label: 'Channel deleted', value: c.deleted }
		]
			.filter((i) => i.value > 0)
			.map((i) => ({ ...i, detail: pct(i.value, total) }));
	});

	const ratings = $derived(
		[5, 4, 3, 2, 1].map((n) => ({ key: String(n), label: plural(n, 'star'), value: data?.ratings[n - 1] ?? 0 }))
	);

	const kinds = $derived.by(() => {
		if (!data) return [];
		const total = data.channels + data.threads;
		return [
			{ key: 'channel', label: 'Channels', value: data.channels, detail: pct(data.channels, total) },
			{ key: 'thread', label: 'Private threads', value: data.threads, detail: pct(data.threads, total) }
		];
	});

	// Point out a type that waits much longer than the others for a reply.
	const typeInsight = $derived.by(() => {
		const timed = (data?.by_type ?? []).filter((t) => t.first_response_median_seconds != null && t.opened >= 3);
		if (timed.length < 2) return '';
		timed.sort((a, b) => b.first_response_median_seconds! - a.first_response_median_seconds!);
		const slow = timed[0];
		const fast = timed[timed.length - 1];
		const ratio = slow.first_response_median_seconds! / Math.max(1, fast.first_response_median_seconds!);
		if (ratio < 1.5) return '';
		return `${typeName(slow)} waits longest for a first reply (${formatDuration(slow.first_response_median_seconds)}), ${ratio.toFixed(1)} times as long as ${typeName(fast)}.`;
	});

	const nothing = $derived(data != null && data.summary.opened === 0 && data.summary.closed === 0 && data.open_now === 0);
</script>

<PageHeader title="Analytics" description="How your support team is doing. Times and days are in UTC.">
	{#snippet actions()}
		<div class="flex flex-wrap items-center gap-2">
			{#if types.length > 1}
				<select class="input w-auto" bind:value={typeId} aria-label="Ticket type">
					<option value="">All ticket types</option>
					{#each types as t (t.id)}
						<option value={String(t.id)}>{t.emoji ? `${emojiText(t.emoji)} ` : ''}{t.name}</option>
					{/each}
				</select>
			{/if}
			<Segmented options={ranges} bind:value={range} label="Date range" />
		</div>
	{/snippet}
</PageHeader>

{#if error && !data}
	<p class="card mt-6 p-5 text-sm text-danger">{error}</p>
{:else if !data}
	<div class="mt-8 h-[248px] animate-pulse rounded-xl bg-surface"></div>
	<div class="mt-4 h-80 animate-pulse rounded-xl bg-surface"></div>
	<div class="mt-4 h-64 animate-pulse rounded-xl bg-surface"></div>
{:else if nothing}
	<div class="mt-8 rounded-xl border border-dashed border-border px-6 py-14 text-center">
		<h2 class="font-semibold">No tickets in {period}</h2>
		{#if range !== 'all'}
			<p class="hint mx-auto mt-1 max-w-sm">Try a longer date range to see older tickets.</p>
			<button type="button" class="btn btn-secondary mt-5" onclick={() => (range = 'all')}>Show all time</button>
		{:else}
			<p class="hint mx-auto mt-1 max-w-sm">
				Once members start opening tickets, you'll see how quickly your team replies and how members rate their help.
			</p>
			<a href="/servers/{guildId}/buttons" class="btn btn-primary mt-5">Set up ticket buttons</a>
		{/if}
	</div>
{:else}
	<div class="transition-opacity {loading ? 'opacity-60' : ''}">
		<dl
			class="mt-8 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border lg:grid-cols-4"
		>
			{#each tiles as tile (tile.label)}
				<div class="bg-surface p-5">
					<dt class="text-sm text-muted">{tile.label}</dt>
					<dd class="mt-3 font-display text-4xl leading-none font-bold tabular-nums">{tile.value}</dd>
					<dd class="mt-2 text-xs {tile.waiting ? 'text-accent' : 'text-subtle'}">{tile.hint}</dd>
				</div>
			{/each}
		</dl>
		<p class="hint mt-2">
			Showing {period}{data.previous ? `, compared with ${before}` : ''}. Open now counts every open ticket.
		</p>

		{#if chart}
			<section class="card mt-6 p-5">
				<div class="flex flex-wrap items-start justify-between gap-3">
					<div>
						<h2 class="font-semibold">{chart.title}</h2>
						<p class="hint mt-0.5">{chart.hint}</p>
					</div>
					<Segmented options={views} bind:value={view} label="Chart" />
				</div>
				<div class="mt-4">
					<ColumnChart data={chart.points} label="{chart.title}, {chart.hint.toLowerCase()}" />
				</div>
			</section>
		{/if}

		<section class="card mt-4 p-5">
			<h2 class="font-semibold">Busiest times</h2>
			<p class="hint mt-0.5">Tickets opened by day and hour (UTC)</p>
			<div class="mt-4">
				<Heatmap data={data.heatmap} label="Tickets opened by day of the week and hour, {period}" />
			</div>
		</section>

		{#if hasResponses}
			<section class="card mt-4 p-5">
				<h2 class="font-semibold">First response by hour</h2>
				<p class="hint mt-0.5">Median time to the first reply, by the hour the ticket was opened (UTC)</p>
				<div class="mt-4">
					<ColumnChart
						data={responseByHour}
						label="Median first response time by hour opened, {period}"
						format={formatDuration}
						steps={durationSteps}
						keyLabel="Hour opened (UTC)"
						valueLabel="Median first response"
						emptyText="No replies"
						height={180}
					/>
				</div>
			</section>
		{/if}

		<div class="mt-4 grid gap-4 lg:grid-cols-2">
			<section class="card p-5">
				<h2 class="font-semibold">How tickets were closed</h2>
				<p class="hint mt-0.5">Tickets closed in this period</p>
				<div class="mt-4">
					{#if closures.length === 0}
						<p class="text-sm text-subtle">No tickets were closed in this period.</p>
					{:else}
						<BarList items={closures} />
					{/if}
				</div>
			</section>

			<section class="card p-5">
				<h2 class="font-semibold">Top close reasons</h2>
				<p class="hint mt-0.5">Reasons your team and members gave</p>
				<div class="mt-4">
					{#if data.close_reasons.length === 0}
						<p class="text-sm text-subtle">No close reasons were given in this period.</p>
					{:else}
						<BarList items={data.close_reasons.map((r) => ({ key: r.reason, label: r.reason, value: r.count }))} />
					{/if}
				</div>
			</section>

			<section class="card p-5">
				<h2 class="font-semibold">Ratings</h2>
				<p class="hint mt-0.5">
					{data.summary.rating_count
						? `${plural(data.summary.rating_count, 'rating')}, averaging ${data.summary.rating_avg?.toFixed(1)} out of 5`
						: 'Members are asked to rate their help when a ticket closes'}
				</p>
				<div class="mt-4">
					{#if data.summary.rating_count === 0}
						<p class="text-sm text-subtle">No ratings in this period.</p>
					{:else}
						<BarList items={ratings} />
					{/if}
				</div>
			</section>

			<section class="card p-5">
				<h2 class="font-semibold">Where tickets were opened</h2>
				<p class="hint mt-0.5">Tickets opened in this period</p>
				<div class="mt-4">
					{#if data.channels + data.threads === 0}
						<p class="text-sm text-subtle">No tickets were opened in this period.</p>
					{:else}
						<BarList items={kinds} />
					{/if}
				</div>
			</section>
		</div>

		<section class="card mt-4 overflow-hidden">
			<div class="p-5 pb-3">
				<h2 class="font-semibold">Ticket types</h2>
				<p class="hint mt-0.5">Tickets opened in this period and how they went</p>
				{#if typeInsight}
					<p class="mt-3 text-sm">{typeInsight}</p>
				{/if}
			</div>
			{#if data.by_type.length === 0}
				<p class="px-5 pb-5 text-sm text-subtle">No tickets were opened in this period.</p>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full text-left text-sm">
						<thead class="border-y border-border text-xs text-muted">
							<tr>
								<th class="px-5 py-2 font-medium">Type</th>
								<th class="px-3 py-2 text-right font-medium">Opened</th>
								<th class="px-3 py-2 text-right font-medium">First response</th>
								<th class="px-3 py-2 text-right font-medium">Resolution</th>
								<th class="px-5 py-2 text-right font-medium">Rating</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-border">
							{#each data.by_type as t (t.type_id ?? `deleted-${t.name}`)}
								<tr>
									<td class="max-w-[16rem] truncate px-5 py-2.5">{typeName(t)}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{t.opened.toLocaleString()}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{formatDuration(t.first_response_median_seconds)}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{formatDuration(t.resolution_median_seconds)}</td>
									<td class="px-5 py-2.5 text-right tabular-nums {t.rating_avg == null ? 'text-subtle' : ''}">
										{t.rating_avg != null ? t.rating_avg.toFixed(1) : '—'}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<section class="card mt-4 overflow-hidden">
			<div class="p-5 pb-3">
				<h2 class="font-semibold">Support team</h2>
				<p class="hint mt-0.5">
					Replies count messages in tickets opened in this period, while their transcripts are kept. Rating is the
					average for tickets they claimed.
				</p>
			</div>
			{#if data.staff.length === 0}
				<p class="px-5 pb-5 text-sm text-subtle">Nobody from your team has replied to a ticket in this period.</p>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full text-left text-sm">
						<thead class="border-y border-border text-xs text-muted">
							<tr>
								<th class="px-5 py-2 font-medium">Member</th>
								<th class="px-3 py-2 text-right font-medium">Replies</th>
								<th class="px-3 py-2 text-right font-medium">Tickets helped</th>
								<th class="px-3 py-2 text-right font-medium">Claimed</th>
								<th class="px-3 py-2 text-right font-medium">Closed</th>
								<th class="px-5 py-2 text-right font-medium">Rating</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-border">
							{#each data.staff as s (s.user_id)}
								<tr>
									<td class="max-w-[12rem] truncate px-5 py-2.5">{s.name}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{s.replies.toLocaleString()}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{s.tickets.toLocaleString()}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{s.claimed.toLocaleString()}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{s.closed.toLocaleString()}</td>
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
{/if}
