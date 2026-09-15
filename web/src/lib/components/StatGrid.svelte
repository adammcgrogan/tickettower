<script lang="ts">
	// A row of headline figures divided by hairlines. A null value shows a skeleton.
	type Stat = {
		label: string;
		value: string | null | undefined;
		hint?: string;
		/** The figure needs action, so it's amber. */
		loud?: boolean;
		/** The hint needs action, so it's amber. */
		hintLoud?: boolean;
	};
	let { items }: { items: Stat[] } = $props();

	// Five figures fit one row; otherwise four, so rows stay even.
	const cols = $derived(items.length === 5 ? 'lg:grid-cols-5' : items.length === 3 ? 'lg:grid-cols-3' : 'lg:grid-cols-4');
</script>

<dl class="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border {cols}">
	{#each items as s (s.label)}
		<div class="bg-surface p-5">
			<dt class="text-sm text-muted">{s.label}</dt>
			<dd class="mt-3 h-9 font-display text-4xl leading-none font-bold tabular-nums {s.loud ? 'text-accent' : ''}">
				{#if s.value == null}
					<div class="h-8 w-14 animate-pulse rounded bg-elevated"></div>
				{:else}
					{s.value}
				{/if}
			</dd>
			{#if s.hint}<dd class="mt-2 text-xs {s.hintLoud ? 'text-accent' : 'text-subtle'}">{s.hint}</dd>{/if}
		</div>
	{/each}
</dl>
