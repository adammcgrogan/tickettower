<script lang="ts">
	import AppHeader from '$lib/components/AppHeader.svelte';

	let { children } = $props();
</script>

<div class="min-h-dvh">
	<AppHeader />
	{@render children()}
</div>
