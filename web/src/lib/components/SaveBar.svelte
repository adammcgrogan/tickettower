<script lang="ts">
	import type { Snippet } from 'svelte';

	// The sticky bar at the foot of an editor: where changes stand, and the buttons to save them.
	let { status, children }: { status?: string; children: Snippet } = $props();
</script>

<div
	class="sticky bottom-4 z-10 mt-6 flex items-center justify-between gap-2 rounded-xl border border-border bg-surface/90 p-3 shadow-2xl shadow-black/40 backdrop-blur"
>
	<span class="pl-1 text-xs text-muted" aria-live="polite">{status}</span>
	<div class="flex gap-2">{@render children()}</div>
</div>
