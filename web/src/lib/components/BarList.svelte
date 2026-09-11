<script lang="ts">
	/**
	 * A short ranked list with a bar per row, one series in the chart colour.
	 * Every value is printed, so it needs no tooltip or table view.
	 */
	type Item = { key: string; label: string; value: number; detail?: string };

	let { items }: { items: Item[] } = $props();

	const max = $derived(Math.max(1, ...items.map((i) => i.value)));
</script>

<ul class="space-y-3">
	{#each items as item (item.key)}
		<li>
			<div class="flex items-baseline justify-between gap-3 text-sm">
				<span class="truncate" title={item.label}>{item.label}</span>
				<span class="shrink-0 text-muted tabular-nums">
					{item.value.toLocaleString()}
					{#if item.detail}<span class="ml-1 text-subtle">{item.detail}</span>{/if}
				</span>
			</div>
			<div class="mt-1.5 h-2 rounded-full bg-elevated">
				<div class="h-full rounded-full bg-chart" style="width:{(item.value / max) * 100}%"></div>
			</div>
		</li>
	{/each}
</ul>
