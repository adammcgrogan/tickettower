<script lang="ts">
	/**
	 * A single-series column chart. One series needs no legend: the card's
	 * title names what's plotted. Each column has a hover/focus tooltip, and a
	 * table view below keeps every value reachable without hovering. A null
	 * value (no data for that column) draws nothing and says so in its tooltip.
	 */
	type Datum = { key: string; label: string; value: number | null };

	let {
		data,
		label,
		unit = 'tickets',
		height = 220,
		format,
		steps,
		keyLabel = 'Date',
		valueLabel = unit,
		emptyText = 'No data'
	}: {
		data: Datum[];
		label: string;
		unit?: string;
		height?: number;
		/** Formats values for ticks, tooltips and the table. Defaults to "N {unit}". */
		format?: (v: number) => string;
		/** Allowed axis steps (e.g. round durations). Defaults to 1, 2, 5, 10… */
		steps?: number[];
		keyLabel?: string;
		valueLabel?: string;
		emptyText?: string;
	} = $props();

	let width = $state(0);
	let active = $state<number | null>(null);

	const pad = { top: 12, right: 4, bottom: 26, left: 40 };
	const plotW = $derived(Math.max(0, width - pad.left - pad.right));
	const plotH = $derived(height - pad.top - pad.bottom);

	const show = (v: number | null) => (v == null ? emptyText : format ? format(v) : `${v.toLocaleString()} ${unit}`);
	const tick = (v: number) => (v === 0 ? '0' : format ? format(v) : v.toLocaleString());

	// Round the axis to at most four clean steps.
	const scale = $derived.by(() => {
		const max = Math.max(1, ...data.map((d) => d.value ?? 0));
		let step: number;
		if (steps?.length) {
			step = steps.find((s) => max / s <= 4) ?? steps[steps.length - 1];
		} else {
			const raw = max / 3;
			const pow = 10 ** Math.floor(Math.log10(raw));
			const n = raw / pow;
			step = Math.max(1, (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * pow);
		}
		const top = Math.ceil(max / step) * step;
		const ticks: number[] = [];
		for (let v = 0; v <= top; v += step) ticks.push(v);
		return { top, ticks };
	});

	const y = (v: number) => pad.top + plotH * (1 - v / scale.top);
	const band = $derived(data.length ? plotW / data.length : 0);
	// Columns are capped at 24px and keep a 2px surface gap between them.
	const barW = $derived(Math.max(1, Math.min(24, band * 0.7, band - 2)));
	const cx = (i: number) => pad.left + band * i + band / 2;

	// Rounded 4px data-end, square at the baseline.
	function column(i: number, v: number) {
		const x = cx(i) - barW / 2;
		const top = y(v);
		const base = y(0);
		const r = Math.min(4, barW / 2, base - top);
		return `M${x},${base}V${top + r}Q${x},${top} ${x + r},${top}H${x + barW - r}Q${x + barW},${top} ${x + barW},${top + r}V${base}Z`;
	}

	// Label the first, middle and last columns so labels never collide.
	const xLabels = $derived(
		data.length <= 1 ? [0] : [...new Set([0, Math.floor((data.length - 1) / 2), data.length - 1])]
	);

	const tip = $derived(active === null ? null : { d: data[active], x: cx(active), y: y(data[active].value ?? 0) });
</script>

<div class="relative" bind:clientWidth={width}>
	{#if width > 0}
		<svg {width} {height} role="img" aria-label={label} class="block">
			{#each scale.ticks as t (t)}
				<line x1={pad.left} x2={width - pad.right} y1={y(t)} y2={y(t)} stroke="var(--color-border)" stroke-width="1" />
				<text x={pad.left - 8} y={y(t)} dy="0.32em" text-anchor="end" class="fill-subtle text-[11px] tabular-nums"
					>{tick(t)}</text
				>
			{/each}

			{#each data as d, i (d.key)}
				{#if d.value}
					<!-- Hover dims the other columns so the data colour stays the validated accent. -->
					<path
						d={column(i, d.value)}
						fill="var(--color-chart)"
						opacity={active === null || active === i ? 1 : 0.45}
						class="transition-opacity duration-100"
					/>
				{/if}
			{/each}

			{#each xLabels as i (i)}
				<text
					x={cx(i)}
					y={height - 6}
					text-anchor={i === 0 ? 'start' : i === data.length - 1 ? 'end' : 'middle'}
					dx={i === 0 ? -barW / 2 : i === data.length - 1 ? barW / 2 : 0}
					class="fill-subtle text-[11px]">{data[i]?.label}</text
				>
			{/each}

			<!-- Hit targets span the whole band so columns are easy to hover. -->
			{#each data as d, i (d.key)}
				<rect
					x={pad.left + band * i}
					y={pad.top}
					width={band}
					height={plotH}
					fill="transparent"
					tabindex="0"
					role="button"
					aria-label="{d.label}: {show(d.value)}"
					class="cursor-default outline-none"
					onpointerenter={() => (active = i)}
					onpointerleave={() => (active = null)}
					onfocus={() => (active = i)}
					onblur={() => (active = null)}
				/>
			{/each}
		</svg>

		{#if tip}
			<div
				class="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-lg border border-border bg-elevated px-2.5 py-1.5 text-xs whitespace-nowrap shadow-xl shadow-black/40"
				style="left:{Math.min(Math.max(tip.x, 56), width - 56)}px;top:{tip.y - 8}px"
			>
				<div class="font-semibold text-fg tabular-nums">{show(tip.d.value)}</div>
				<div class="text-muted">{tip.d.label}</div>
			</div>
		{/if}
	{:else}
		<div style="height:{height}px"></div>
	{/if}
</div>

<details class="mt-3 text-xs text-muted">
	<summary class="cursor-pointer select-none hover:text-fg">Show as table</summary>
	<div class="mt-2 max-h-64 overflow-auto rounded-lg border border-border">
		<table class="w-full text-left">
			<thead class="sticky top-0 bg-surface text-subtle">
				<tr
					><th class="px-3 py-1.5 font-medium">{keyLabel}</th><th class="px-3 py-1.5 text-right font-medium"
						>{valueLabel}</th
					></tr
				>
			</thead>
			<tbody class="divide-y divide-border">
				{#each data as d (d.key)}
					<tr
						><td class="px-3 py-1.5">{d.label}</td><td class="px-3 py-1.5 text-right text-fg tabular-nums"
							>{d.value == null ? emptyText : format ? format(d.value) : d.value.toLocaleString()}</td
						></tr
					>
				{/each}
			</tbody>
		</table>
	</div>
</details>
