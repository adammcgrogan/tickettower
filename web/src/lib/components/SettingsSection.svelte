<script lang="ts">
	import type { Snippet } from 'svelte';

	// One row of a settings page: what it is on the left, the controls on the right.
	let {
		title,
		description,
		danger = false,
		children
	}: { title: string; description?: Snippet; danger?: boolean; children: Snippet } = $props();
</script>

<section class="grid gap-4 py-6 md:grid-cols-[13rem_minmax(0,1fr)] md:gap-8">
	<div>
		<h2 class="font-medium {danger ? 'text-danger' : ''}">{title}</h2>
		{#if description}<p class="hint mt-1">{@render description()}</p>{/if}
	</div>
	<div class="min-w-0">{@render children()}</div>
</section>
