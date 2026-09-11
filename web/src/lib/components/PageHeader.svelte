<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	let {
		title,
		description,
		back,
		actions
	}: { title: string; description?: string; back?: { href: string; label: string }; actions?: Snippet } =
		$props();
</script>

{#if back}
	<a
		href={back.href}
		class="mb-4 inline-flex items-center gap-1.5 text-sm text-muted transition-colors hover:text-fg"
	>
		<Icon name="arrow-left" size={14} />
		{back.label}
	</a>
{/if}
<div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
	<div class="min-w-0">
		<h1 class="font-display text-4xl leading-none font-bold">{title}</h1>
		{#if description}
			<p class="mt-2.5 max-w-xl text-sm text-muted">{description}</p>
		{/if}
	</div>
	{#if actions}
		<div class="flex shrink-0 flex-wrap gap-2">{@render actions()}</div>
	{/if}
</div>
