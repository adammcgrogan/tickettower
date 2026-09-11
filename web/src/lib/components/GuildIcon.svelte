<script lang="ts">
	let { name, url, size = 44 }: { name: string; url: string | null; size?: number } = $props();

	const initials = $derived(
		name
			.split(/\s+/)
			.filter(Boolean)
			.map((w) => w[0])
			.join('')
			.slice(0, 3)
	);
</script>

{#if url}
	<img
		src={url}
		alt=""
		class="shrink-0 rounded-xl object-cover"
		style="width:{size}px;height:{size}px"
	/>
{:else}
	<div
		class="grid shrink-0 place-items-center rounded-xl border border-border bg-elevated font-medium text-muted"
		style="width:{size}px;height:{size}px;font-size:{Math.round(size * 0.32)}px"
	>
		{initials}
	</div>
{/if}
