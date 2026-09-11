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
		class="mb-3 inline-flex items-center gap-1.5 text-xs text-muted transition-colors hover:text-fg"
	>
		<Icon name="arrow-left" size={13} />
		{back.label}
	</a>
{/if}
<div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
	<div class="min-w-0">
		<h1 class="text-xl font-semibold tracking-tight">{title}</h1>
		{#if description}
			<p class="mt-1 max-w-xl text-sm text-muted">{description}</p>
		{/if}
	</div>
	{#if actions}
		<div class="flex shrink-0 gap-2">{@render actions()}</div>
	{/if}
</div>
