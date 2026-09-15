<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';

	let {
		icon,
		title,
		children,
		action
	}: { icon?: IconName; title: string; children?: Snippet; action?: Snippet } = $props();
</script>

<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
	{#if icon}
		<div class="mx-auto mb-4 grid size-10 place-items-center rounded-xl bg-elevated text-muted">
			<Icon name={icon} />
		</div>
	{/if}
	<p class="font-medium">{title}</p>
	{#if children}
		<p class="mx-auto mt-1 max-w-sm text-sm text-muted">{@render children()}</p>
	{/if}
	{#if action}
		<div class="mt-5">{@render action()}</div>
	{/if}
</div>
