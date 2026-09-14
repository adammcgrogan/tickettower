<script lang="ts">
	/** A ticket's rating as five stars. `compact` shows the number and one star. */
	let { rating, compact = false }: { rating: number; compact?: boolean } = $props();
	const label = $derived(`Rated ${rating} out of 5`);
</script>

{#if compact}
	<span class="inline-flex items-center gap-0.5 text-accent" title={label} aria-label={label}>
		<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="12" height="12" fill="currentColor" aria-hidden="true">
			<path d="M12 2.5l2.9 6.2 6.8.8-5 4.6 1.3 6.7L12 17.4l-6 3.4 1.3-6.7-5-4.6 6.8-.8z" />
		</svg>
		<span class="text-xs font-medium">{rating}</span>
	</span>
{:else}
	<span class="inline-flex items-center gap-0.5" role="img" aria-label={label} title={label}>
		{#each [1, 2, 3, 4, 5] as n (n)}
			<svg
				xmlns="http://www.w3.org/2000/svg"
				viewBox="0 0 24 24"
				width="14"
				height="14"
				fill="currentColor"
				class={n <= rating ? 'text-accent' : 'text-border'}
				aria-hidden="true"
			>
				<path d="M12 2.5l2.9 6.2 6.8.8-5 4.6 1.3 6.7L12 17.4l-6 3.4 1.3-6.7-5-4.6 6.8-.8z" />
			</svg>
		{/each}
	</span>
{/if}
