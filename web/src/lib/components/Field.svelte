<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { GuideSlug } from '$lib/help';

	let {
		label,
		for: htmlFor,
		hint,
		help,
		error,
		optional = false,
		children
	}: {
		label: string;
		for?: string;
		hint?: string;
		/** A help guide about this field, linked after the hint. */
		help?: GuideSlug;
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
	{:else if hint || help}
		<p class="hint">
			{hint}
			{#if help}
				<!-- A new tab, so reading help doesn't lose unsaved changes. -->
				<a href="/help/{help}" target="_blank" class="whitespace-nowrap text-fg underline-offset-4 hover:underline">
					Learn more
				</a>
			{/if}
		</p>
	{/if}
</div>
