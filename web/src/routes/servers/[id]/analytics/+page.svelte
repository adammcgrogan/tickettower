<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, type Analytics, type TicketType } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { emojiText, formatDuration } from '$lib/format';
	import BarList from '$lib/components/BarList.svelte';
	import ColumnChart from '$lib/components/ColumnChart.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Heatmap from '$lib/components/Heatmap.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import Segmented from '$lib/components/Segmented.svelte';

	type Range = '7' | '30' | '90' | '365' | 'all';
	type View = 'opened' | 'closed' | 'backlog';
	type TypeRow = Analytics['by_type'][number];
	type StaffRow = Analytics['staff'][number];

	const guildId = page.params.id!;
	// The browser's IANA zone, so the heatmap and response-by-hour line up
	// with the team's local time instead of UTC.
	const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
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

	// The filters live in the URL, so a view can be shared and the back button works.
	const range = $derived<Range>(
		ranges.some((r) => r.value === page.url.searchParams.get('range'))
			? (page.url.searchParams.get('range') as Range)
			: '30'
	);
	const typeId = $derived(page.url.searchParams.get('type') ?? '');
	function setFilter(key: 'range' | 'type', value: string, fallback: string) {
		const params = new URLSearchParams(page.url.searchParams);
		if (value === fallback) params.delete(key);
		else params.set(key, value);
		const qs = params.toString();
		goto(page.url.pathname + (qs ? `?${qs}` : ''), { replaceState: true, noScroll: true, keepFocus: true });
	}

	let view = $state<View>('opened');
	let types = $state<TicketType[]>([]);
	let data = $state<Analytics | null>(null);
	let loading = $state(true);
	let error = $state('');
	// Bumped by "Try again" to refetch with the same filters.
	let attempt = $state(0);

	// The filter is optional, so a failure here just hides it.
	api<TicketType[]>(`/guilds/${guildId}/ticket-types`)
		.then((t) => (types = t))
		.catch(() => {});

	// Refetching keeps the previous render (dimmed) rather than flashing a
	// skeleton. Only the latest request may update the page.
	let latest = 0;
	$effect(() => {
		void attempt;
		const query = `days=${range}${typeId ? `&type=${typeId}` : ''}&tz=${encodeURIComponent(timezone)}`;
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

	type Change = { text: string; tone: 'good' | 'bad' | 'neutral' };
	/**
	 * The change from the previous window, e.g. "59% faster than the 30 days
	 * before". `better` says which direction is good, when one is.
	 */
	function change(
		cur: number | null | undefined,
		prev: number | null | undefined,
		[up, down]: [string, string],
		better?: 'up' | 'down'
	): Change | undefined {
		if (cur == null || prev == null || prev === 0) return undefined;
		const p = Math.round(((cur - prev) / prev) * 100);
		if (p === 0) return { text: `Same as ${before}`, tone: 'neutral' };
		const tone = !better ? 'neutral' : (p > 0) === (better === 'up') ? 'good' : 'bad';
		return { text: `${Math.abs(p)}% ${p > 0 ? up : down} than ${before}`, tone };
	}
	const toneClass = (c?: Change) =>
		c?.tone === 'good' ? 'text-success' : c?.tone === 'bad' ? 'text-danger' : 'text-subtle';

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

	// --- The overview: four headline figures, then everything else in a table ---

	const headline = $derived.by(() => {
		if (!data) return [];
		const s = data.summary;
		const p = data.previous;
		return [
			{
				label: 'Tickets opened',
				value: s.opened.toLocaleString(),
				change: change(s.opened, p?.opened, ['more', 'fewer']),
				note: `${s.closed.toLocaleString()} closed`
			},
			{
				label: 'First response',
				value: formatDuration(s.first_response_median_seconds),
				change: change(s.first_response_median_seconds, p?.first_response_median_seconds, ['slower', 'faster'], 'down'),
				note: 'Median time to the first reply'
			},
			{
				label: 'Replied within target',
				value: s.target_measured ? pct(s.target_met, s.target_measured) : '—',
				change: s.target_measured
					? change(
							s.target_met / s.target_measured,
							p?.target_measured ? p.target_met / p.target_measured : null,
							['better', 'worse'],
							'up'
						)
					: undefined,
				note: s.target_measured ? `Of ${plural(s.target_measured, 'ticket')} with a target` : 'Set a reply target on a ticket type'
			},
			{
				label: 'Satisfaction',
				value: s.rating_avg != null ? s.rating_avg.toFixed(1) : '—',
				change: change(s.rating_avg, p?.rating_avg, ['higher', 'lower'], 'up'),
				note: s.rating_count ? `Out of 5, from ${plural(s.rating_count, 'rating')}` : 'No ratings yet'
			}
		];
	});

	const details = $derived.by(() => {
		if (!data) return [];
		const s = data.summary;
		const p = data.previous;
		const perTicket = s.transcripts ? (s.team_messages + s.member_messages) / s.transcripts : null;
		return [
			{
				group: 'Volume',
				rows: [
					{
						label: 'Open now',
						value: data.open_now.toLocaleString(),
						note: [
							data.waiting_now ? `${data.waiting_now} waiting on your team` : 'none waiting on your team',
							data.overdue_now ? `${data.overdue_now} past target` : '',
							data.on_hold_now ? `${data.on_hold_now} on hold` : ''
						]
							.filter(Boolean)
							.join(', ')
					},
					{ label: 'Backlog age', value: formatDuration(data.backlog_age_median_seconds), note: 'Median age of open tickets' },
					{
						label: 'Answered without a ticket',
						value: s.answers_deflected.toLocaleString(),
						note: change(s.answers_deflected, p?.answers_deflected, ['more', 'fewer'])?.text ?? 'Solved by a suggested answer'
					}
				]
			},
			{
				group: 'Speed',
				rows: [
					{
						label: 'Time to claim',
						value: formatDuration(s.claim_median_seconds),
						note: change(s.claim_median_seconds, p?.claim_median_seconds, ['slower', 'faster'])?.text ?? 'Median, open to claim'
					},
					{
						label: 'Resolution time',
						value: formatDuration(s.resolution_median_seconds),
						note:
							change(s.resolution_median_seconds, p?.resolution_median_seconds, ['slower', 'faster'])?.text ??
							'Median, open to close'
					}
				]
			},
			{
				group: 'Outcomes',
				rows: [
					{
						label: 'Solved in one reply',
						value: pct(s.one_touch, s.transcripts),
						note: s.transcripts ? `Of ${plural(s.transcripts, 'closed ticket')}` : 'No closed tickets yet'
					},
					{
						label: 'Closed without a reply',
						value: pct(s.closed_unanswered, s.closed),
						note: s.closed ? plural(s.closed_unanswered, 'ticket') : 'No closed tickets yet'
					},
					{ label: 'Reopened', value: pct(s.reopened, s.closed), note: s.closed ? plural(s.reopened, 'ticket') : '' },
					{
						label: 'Messages per ticket',
						value: perTicket != null ? perTicket.toFixed(1) : '—',
						note: s.transcripts
							? `Team ${(s.team_messages / s.transcripts).toFixed(1)}, member ${(s.member_messages / s.transcripts).toFixed(1)}`
							: ''
					}
				]
			}
		];
	});

	// The period in words, so the page answers "how are we doing?" before any chart.
	const story = $derived.by(() => {
		if (!data) return '';
		const s = data.summary;
		const parts: string[] = [];
		const opened = change(s.opened, data.previous?.opened, ['more', 'fewer']);
		parts.push(
			`${plural(s.opened, 'ticket')} opened in ${period}${opened && opened.text.startsWith('Same') ? ', the same as before' : opened ? `, ${opened.text.replace(/ than .*/, '')} than before` : ''}.`
		);
		if (s.first_response_median_seconds != null) {
			const fr = change(s.first_response_median_seconds, data.previous?.first_response_median_seconds, ['slower', 'faster']);
			parts.push(
				`Your team usually replied within ${formatDuration(s.first_response_median_seconds)}${fr && !fr.text.startsWith('Same') ? `, ${fr.text.replace(/ than .*/, '')} than before` : ''}.`
			);
		}
		if (s.rating_avg != null) parts.push(`Members rated their help ${s.rating_avg.toFixed(1)} out of 5.`);
		return parts.join(' ');
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

	// --- Sortable tables ---

	type Sort = { key: string; desc: boolean };
	const typeCols: { key: keyof TypeRow | 'in_target'; label: string }[] = [
		{ key: 'name', label: 'Type' },
		{ key: 'opened', label: 'Opened' },
		{ key: 'first_response_median_seconds', label: 'First response' },
		{ key: 'resolution_median_seconds', label: 'Resolution' },
		{ key: 'in_target', label: 'In target' },
		{ key: 'rating_avg', label: 'Rating' }
	];
	const staffCols: { key: keyof StaffRow; label: string }[] = [
		{ key: 'name', label: 'Member' },
		{ key: 'replies', label: 'Replies' },
		{ key: 'tickets', label: 'Tickets helped' },
		{ key: 'claimed', label: 'Claimed' },
		{ key: 'closed', label: 'Closed' },
		{ key: 'avg_rating', label: 'Rating' }
	];
	let typeSort = $state<Sort>({ key: 'opened', desc: true });
	let staffSort = $state<Sort>({ key: 'replies', desc: true });

	function sortBy<T>(rows: T[], get: (r: T) => unknown, desc: boolean) {
		// Missing values always go last.
		return [...rows].sort((a, b) => {
			const x = get(a);
			const y = get(b);
			if (x == null) return 1;
			if (y == null) return -1;
			const c = typeof x === 'string' ? x.localeCompare(String(y)) : Number(x) - Number(y);
			return desc ? -c : c;
		});
	}
	const inTarget = (t: TypeRow) => (t.target_measured ? t.target_met / t.target_measured : null);
	const typeRows = $derived(
		sortBy(data?.by_type ?? [], (t) => (typeSort.key === 'in_target' ? inTarget(t) : t[typeSort.key as keyof TypeRow]), typeSort.desc)
	);
	const staffRows = $derived(sortBy(data?.staff ?? [], (s) => s[staffSort.key as keyof StaffRow], staffSort.desc));
	function toggle(sort: Sort, key: string): Sort {
		// Names sort A to Z first; numbers biggest first.
		return sort.key === key ? { key, desc: !sort.desc } : { key, desc: key !== 'name' };
	}

	const nothing = $derived(data != null && data.summary.opened === 0 && data.summary.closed === 0 && data.open_now === 0);

	const sections = [
		{ id: 'overview', label: 'Overview' },
		{ id: 'volume', label: 'Volume' },
		{ id: 'speed', label: 'Speed' },
		{ id: 'outcomes', label: 'Outcomes' },
		{ id: 'types', label: 'Ticket types' },
		{ id: 'team', label: 'Team' }
	];
</script>

<svelte:head><title>Analytics · {APP_NAME}</title></svelte:head>

{#snippet sortHead(cols: { key: string; label: string }[], sort: Sort, set: (s: Sort) => void)}
	<thead class="border-y border-border text-xs text-muted">
		<tr>
			{#each cols as c, i (c.key)}
				<th
					class="py-2 font-medium {i === 0 ? 'px-5 text-left' : i === cols.length - 1 ? 'px-5 text-right' : 'px-3 text-right'}"
					aria-sort={sort.key === c.key ? (sort.desc ? 'descending' : 'ascending') : 'none'}
				>
					<button
						class="inline-flex items-center gap-1 transition-colors hover:text-fg {sort.key === c.key ? 'text-fg' : ''}"
						onclick={() => set(toggle(sort, c.key))}
					>
						{c.label}
						{#if sort.key === c.key}
							<span aria-hidden="true">{sort.desc ? '↓' : '↑'}</span>
						{/if}
					</button>
				</th>
			{/each}
		</tr>
	</thead>
{/snippet}

<div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
	<div class="min-w-0">
		<h1 class="font-display text-4xl leading-none font-bold">Analytics</h1>
		<p class="mt-2.5 max-w-xl text-sm text-muted">How your support team is doing. Times and days are in your local time.</p>
	</div>
</div>

<!-- The filters and section links stay in reach on a long page. -->
<div class="sticky top-14 z-10 -mx-5 mt-6 border-b border-border bg-bg/90 px-5 py-3 backdrop-blur md:top-0 md:-mx-10 md:px-10">
	<div class="flex flex-wrap items-center gap-2">
		<Segmented options={ranges} value={range} label="Date range" onchange={(v) => setFilter('range', v, '30')} />
		{#if types.length > 1}
			<select
				class="input w-auto"
				value={typeId}
				onchange={(e) => setFilter('type', e.currentTarget.value, '')}
				aria-label="Ticket type"
			>
				<option value="">All ticket types</option>
				{#each types as t (t.id)}
					<option value={String(t.id)}>{t.emoji ? `${emojiText(t.emoji)} ` : ''}{t.name}</option>
				{/each}
			</select>
		{/if}
		{#if data && !nothing}
			<nav aria-label="Sections" class="ml-auto hidden gap-4 text-sm xl:flex">
				{#each sections as s (s.id)}
					<a href="#{s.id}" class="text-muted transition-colors hover:text-fg">{s.label}</a>
				{/each}
			</nav>
		{/if}
	</div>
</div>

{#if error && !data}
	<div class="mt-8"><LoadError message={error} onretry={() => attempt++} /></div>
{:else if !data}
	<div class="mt-8 h-24 animate-pulse rounded-xl bg-surface"></div>
	<div class="mt-4 h-80 animate-pulse rounded-xl bg-surface"></div>
{:else if nothing}
	<div class="mt-8">
		<EmptyState icon="chart" title="No tickets in {period}">
			{range !== 'all'
				? 'Try a longer date range to see older tickets.'
				: "Once members start opening tickets, you'll see how quickly your team replies and how members rate their help."}
			{#snippet action()}
				{#if range !== 'all'}
					<button type="button" class="btn btn-secondary" onclick={() => setFilter('range', 'all', '30')}>
						Show all time
					</button>
				{:else}
					<a href="/servers/{guildId}/panels" class="btn btn-primary">Set up your ticket panel</a>
				{/if}
			{/snippet}
		</EmptyState>
	</div>
{:else}
	<div class="transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
		<section id="overview" class="scroll-mt-20 pt-8">
			<p class="max-w-3xl text-lg text-pretty">{story}</p>
			{#if data.waiting_now}
				<p class="mt-2 text-sm text-muted">
					<a href="/servers/{guildId}/tickets" class="text-accent-ink underline-offset-4 hover:underline">
						{plural(data.waiting_now, 'ticket')} waiting on your team right now
					</a>{data.overdue_now ? `, ${data.overdue_now} past the reply target` : ''}.
				</p>
			{/if}

			<dl class="mt-8 grid grid-cols-2 gap-x-8 gap-y-8 lg:grid-cols-4">
				{#each headline as f (f.label)}
					<div class="border-t border-border pt-4">
						<dt class="text-sm text-muted">{f.label}</dt>
						<dd class="mt-2 font-display text-5xl leading-none font-bold tabular-nums">{f.value}</dd>
						<dd class="mt-2 text-xs {f.change ? toneClass(f.change) : 'text-subtle'}">{f.change?.text ?? f.note}</dd>
					</div>
				{/each}
			</dl>

			<div class="mt-10 grid gap-x-10 gap-y-6 md:grid-cols-3">
				{#each details as g (g.group)}
					<div>
						<h3 class="text-xs text-subtle">{g.group}</h3>
						<dl class="mt-2 divide-y divide-border border-y border-border">
							{#each g.rows as r (r.label)}
								<div class="flex items-baseline justify-between gap-3 py-2.5">
									<dt class="min-w-0">
										<span class="block text-sm">{r.label}</span>
										{#if r.note}<span class="block truncate text-xs text-subtle">{r.note}</span>{/if}
									</dt>
									<dd class="shrink-0 font-medium tabular-nums">{r.value}</dd>
								</div>
							{/each}
						</dl>
					</div>
				{/each}
			</div>
			<p class="hint mt-4">
				Showing {period}{data.previous ? `, compared with ${before}` : ''}. Open now counts every open ticket.
			</p>
		</section>

		<section id="volume" class="mt-14 scroll-mt-20 border-t border-border pt-8">
			<h2 class="font-display text-2xl font-bold">Volume</h2>
			{#if chart}
				<div class="mt-6">
					<div class="flex flex-wrap items-start justify-between gap-3">
						<div>
							<h3 class="font-semibold">{chart.title}</h3>
							<p class="hint mt-0.5">{chart.hint}</p>
						</div>
						<Segmented options={views} bind:value={view} label="Chart" />
					</div>
					<div class="mt-4">
						<ColumnChart data={chart.points} label="{chart.title}, {chart.hint.toLowerCase()}" />
					</div>
				</div>
			{/if}
			<div class="mt-10 grid gap-10 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
				<div class="min-w-0">
					<h3 class="font-semibold">Busiest times</h3>
					<p class="hint mt-0.5">Tickets opened by day and hour ({data.timezone})</p>
					<div class="mt-4">
						<Heatmap data={data.heatmap} label="Tickets opened by day of the week and hour, {period}" />
					</div>
				</div>
				<div>
					<h3 class="font-semibold">Where tickets were opened</h3>
					<p class="hint mt-0.5">Tickets opened in this period</p>
					<div class="mt-4">
						{#if data.channels + data.threads === 0}
							<p class="text-sm text-subtle">No tickets were opened in this period.</p>
						{:else}
							<BarList items={kinds} />
						{/if}
					</div>
				</div>
			</div>
		</section>

		<section id="speed" class="mt-14 scroll-mt-20 border-t border-border pt-8">
			<h2 class="font-display text-2xl font-bold">Speed</h2>
			{#if hasResponses}
				<div class="mt-6">
					<h3 class="font-semibold">First response by hour</h3>
					<p class="hint mt-0.5">
						Median time to the first reply, by the hour the ticket was opened ({data.timezone}). Tall columns are the
						hours your team is stretched.
					</p>
					<div class="mt-4">
						<ColumnChart
							data={responseByHour}
							label="Median first response time by hour opened, {period}"
							format={formatDuration}
							steps={durationSteps}
							keyLabel="Hour opened ({data.timezone})"
							valueLabel="Median first response"
							emptyText="No replies"
							height={180}
						/>
					</div>
				</div>
			{:else}
				<p class="mt-4 text-sm text-subtle">No replies in this period yet.</p>
			{/if}
		</section>

		<section id="outcomes" class="mt-14 scroll-mt-20 border-t border-border pt-8">
			<h2 class="font-display text-2xl font-bold">Outcomes</h2>
			<div class="mt-6 grid gap-10 lg:grid-cols-3">
				<div>
					<h3 class="font-semibold">How tickets were closed</h3>
					<p class="hint mt-0.5">Tickets closed in this period</p>
					<div class="mt-4">
						{#if closures.length === 0}
							<p class="text-sm text-subtle">No tickets were closed in this period.</p>
						{:else}
							<BarList items={closures} />
						{/if}
					</div>
				</div>
				<div>
					<h3 class="font-semibold">Top close reasons</h3>
					<p class="hint mt-0.5">Reasons your team and members gave</p>
					<div class="mt-4">
						{#if data.close_reasons.length === 0}
							<p class="text-sm text-subtle">No close reasons were given in this period.</p>
						{:else}
							<BarList items={data.close_reasons.map((r) => ({ key: r.reason, label: r.reason, value: r.count }))} />
						{/if}
					</div>
				</div>
				<div>
					<h3 class="font-semibold">Ratings</h3>
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
				</div>
			</div>
		</section>

		<section id="types" class="mt-14 scroll-mt-20 border-t border-border pt-8">
			<h2 class="font-display text-2xl font-bold">Ticket types</h2>
			<p class="hint mt-1">Tickets opened in this period and how they went. Click a column to sort.</p>
			{#if typeInsight}
				<p class="mt-3 text-sm">{typeInsight}</p>
			{/if}
			{#if data.by_type.length === 0}
				<p class="mt-4 text-sm text-subtle">No tickets were opened in this period.</p>
			{:else}
				<div class="mt-4 overflow-x-auto rounded-xl border border-border bg-surface">
					<table class="w-full text-sm">
						{@render sortHead(typeCols, typeSort, (s) => (typeSort = s))}
						<tbody class="divide-y divide-border">
							{#each typeRows as t (t.type_id ?? `deleted-${t.name}`)}
								<tr>
									<td class="max-w-[16rem] truncate px-5 py-2.5 text-left">
										{#if t.type_id}
											<a href="?type={t.type_id}{range !== '30' ? `&range=${range}` : ''}" class="hover:underline" title="Show only {t.name}">
												{typeName(t)}
											</a>
										{:else}
											{typeName(t)}
										{/if}
									</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{t.opened.toLocaleString()}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{formatDuration(t.first_response_median_seconds)}</td>
									<td class="px-3 py-2.5 text-right tabular-nums">{formatDuration(t.resolution_median_seconds)}</td>
									<td class="px-3 py-2.5 text-right tabular-nums {t.target_measured ? '' : 'text-subtle'}">
										{t.target_measured ? pct(t.target_met, t.target_measured) : '—'}
									</td>
									<td class="px-5 py-2.5 text-right tabular-nums {t.rating_avg == null ? 'text-subtle' : ''}">
										{t.rating_avg != null ? t.rating_avg.toFixed(1) : t.asks_rating ? '—' : 'Not asked'}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<section id="team" class="mt-14 scroll-mt-20 border-t border-border pt-8">
			<h2 class="font-display text-2xl font-bold">Team</h2>
			<p class="hint mt-1">
				Replies count messages in tickets opened in this period, while their transcripts are kept. Rating is the average
				for tickets they claimed.
			</p>
			{#if data.staff.length === 0}
				<p class="mt-4 text-sm text-subtle">Nobody from your team has replied to a ticket in this period.</p>
			{:else}
				<div class="mt-4 overflow-x-auto rounded-xl border border-border bg-surface">
					<table class="w-full text-sm">
						{@render sortHead(staffCols, staffSort, (s) => (staffSort = s))}
						<tbody class="divide-y divide-border">
							{#each staffRows as s (s.user_id)}
								<tr>
									<td class="max-w-[12rem] truncate px-5 py-2.5 text-left">{s.name}</td>
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
