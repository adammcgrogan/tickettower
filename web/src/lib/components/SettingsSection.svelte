<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	// One row of a settings page: what it is on the left, the controls on the right.
	let {
		id,
		title,
		description,
		danger = false,
		saved = false,
		children
	}: {
		id?: string;
		title: string;
		description?: Snippet;
		danger?: boolean;
		/** Briefly true after a change in this section saves. */
		saved?: boolean;
		children: Snippet;
	} = $props();
</script>

<section {id} class="grid scroll-mt-8 gap-4 py-7 md:grid-cols-[13rem_minmax(0,1fr)] md:gap-8">
	<div>
		<h2 class="flex items-center gap-2 font-medium {danger ? 'text-danger' : ''}">
			{title}
			{#if saved}
				<span class="flex items-center gap-1 text-xs font-normal text-success" aria-live="polite">
					<Icon name="check" size={12} /> Saved
				</span>
			{/if}
		</h2>
		{#if description}<p class="hint mt-1">{@render description()}</p>{/if}
	</div>
	<div class="min-w-0">{@render children()}</div>
</section>
