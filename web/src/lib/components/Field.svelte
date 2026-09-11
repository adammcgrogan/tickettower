<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		label,
		for: htmlFor,
		hint,
		error,
		optional = false,
		children
	}: {
		label: string;
		for?: string;
		hint?: string;
		error?: string;
		optional?: boolean;
		children: Snippet;
	} = $props();
</script>

<div class="space-y-1.5">
	<label for={htmlFor} class="label flex items-baseline gap-2">
		{label}
		{#if optional}<span class="text-xs font-normal text-subtle">Optional</span>{/if}
	</label>
	{@render children()}
	{#if error}
		<p class="text-xs text-danger">{error}</p>
	{:else if hint}
		<p class="hint">{hint}</p>
	{/if}
</div>
