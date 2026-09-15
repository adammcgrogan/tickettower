<script lang="ts">
	// Shown in place of a page's content when it couldn't load.
	let { message, onretry }: { message: string; onretry?: () => void } = $props();
</script>

<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm" role="alert">
	<p class="text-danger">{message}</p>
	{#if onretry}
		<button onclick={onretry} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
			Try again
		</button>
	{/if}
</div>
