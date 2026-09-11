<script lang="ts">
	/**
	 * A weekday × hour grid of counts (UTC) on one amber ramp, quiet to busy.
	 * Each cell has a hover/focus tooltip, and the table view lists every busy
	 * cell so nothing depends on reading colour.
	 */
	let { data, label, unit = 'tickets' }: { data: number[][]; label: string; unit?: string } = $props();

	// The data is Sunday first; the grid reads Monday to Sunday.
	const days = [
		{ i: 1, short: 'Mon', long: 'Monday' },
		{ i: 2, short: 'Tue', long: 'Tuesday' },
		{ i: 3, short: 'Wed', long: 'Wednesday' },
		{ i: 4, short: 'Thu', long: 'Thursday' },
		{ i: 5, short: 'Fri', long: 'Friday' },
		{ i: 6, short: 'Sat', long: 'Saturday' },
		{ i: 0, short: 'Sun', long: 'Sunday' }
	];
	const hours = Array.from({ length: 24 }, (_, h) => h);
	const levels = ['bg-elevated', 'bg-heat-1', 'bg-heat-2', 'bg-heat-3', 'bg-heat-4'];

	const max = $derived(Math.max(1, ...data.flat()));
	const level = (v: number) => (v === 0 ? 0 : Math.min(4, Math.ceil((v / max) * 4)));
	const hh = (h: number) => `${String(h).padStart(2, '0')}:00`;
	const count = (n: number) => `${n.toLocaleString()} ${n === 1 ? unit.replace(/s$/, '') : unit}`;

	const busy = $derived(
		days.flatMap((d) => hours.filter((h) => data[d.i][h] > 0).map((h) => ({ day: d, h, n: data[d.i][h] })))
	);

	// Fixed positioning keeps the tooltip visible above the scrolling grid.
	let tip = $state<{ x: number; y: number; value: string; when: string } | null>(null);
	function show(e: Event, day: (typeof days)[number], h: number) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		tip = { x: r.left + r.width / 2, y: r.top, value: count(data[day.i][h]), when: `${day.long}, ${hh(h)} to ${hh((h + 1) % 24)}` };
	}
</script>

<div class="overflow-x-auto" onscroll={() => (tip = null)}>
	<div
		role="img"
		aria-label={label}
		class="grid min-w-[36rem] gap-[2px]"
		style="grid-template-columns: 2.5rem repeat(24, minmax(0, 1fr))"
	>
		<span></span>
		{#each hours as h (h)}
			<span class="text-[11px] text-subtle">{h % 3 === 0 ? String(h).padStart(2, '0') : ''}</span>
		{/each}

		{#each days as day (day.i)}
			<span class="self-center text-[11px] text-subtle">{day.short}</span>
			{#each hours as h (h)}
				<div
					role="button"
					tabindex="0"
					aria-label="{day.long} {hh(h)}: {count(data[day.i][h])}"
					class="h-5 rounded-[3px] outline-none focus-visible:ring-2 focus-visible:ring-fg {levels[level(data[day.i][h])]}"
					onpointerenter={(e) => show(e, day, h)}
					onpointerleave={() => (tip = null)}
					onfocus={(e) => show(e, day, h)}
					onblur={() => (tip = null)}
				></div>
			{/each}
		{/each}
	</div>
</div>

<div class="mt-3 flex items-center justify-end gap-1.5 text-[11px] text-subtle" aria-hidden="true">
	<span class="mr-1">Less</span>
	{#each levels as cls (cls)}
		<span class="h-3 w-3 rounded-[3px] {cls}"></span>
	{/each}
	<span class="ml-1">More</span>
</div>

{#if tip}
	<div
		class="pointer-events-none fixed z-50 -translate-x-1/2 -translate-y-full rounded-lg border border-border bg-elevated px-2.5 py-1.5 text-xs whitespace-nowrap shadow-xl shadow-black/40"
		style="left:{tip.x}px;top:{tip.y - 8}px"
	>
		<div class="font-semibold text-fg tabular-nums">{tip.value}</div>
		<div class="text-muted">{tip.when}</div>
	</div>
{/if}

<details class="mt-1 text-xs text-muted">
	<summary class="cursor-pointer select-none hover:text-fg">Show as table</summary>
	<div class="mt-2 max-h-64 overflow-auto rounded-lg border border-border">
		{#if busy.length === 0}
			<p class="px-3 py-2">No {unit} in this period.</p>
		{:else}
			<table class="w-full text-left">
				<thead class="sticky top-0 bg-surface text-subtle">
					<tr>
						<th class="px-3 py-1.5 font-medium">Day</th>
						<th class="px-3 py-1.5 font-medium">Hour (UTC)</th>
						<th class="px-3 py-1.5 text-right font-medium">{unit}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-border">
					{#each busy as c (`${c.day.i}-${c.h}`)}
						<tr>
							<td class="px-3 py-1.5">{c.day.long}</td>
							<td class="px-3 py-1.5 tabular-nums">{hh(c.h)}</td>
							<td class="px-3 py-1.5 text-right text-fg tabular-nums">{c.n.toLocaleString()}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</details>
