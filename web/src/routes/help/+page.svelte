<script lang="ts">
	import { APP_NAME } from '$lib/brand';
	import { guides } from '$lib/help';
	import Icon from '$lib/components/Icon.svelte';
</script>

<svelte:head><title>Help · {APP_NAME}</title></svelte:head>

<h1 class="font-display text-4xl leading-none font-bold">Help</h1>
<p class="mt-2.5 text-sm text-muted">
	Short guides to setting up {APP_NAME} and running tickets with your team.
</p>

<ul class="mt-8 divide-y divide-border border-y border-border">
	{#each guides as g (g.slug)}
		<li>
			<a href="/help/{g.slug}" class="group flex items-center gap-4 py-4">
				<span class="min-w-0 flex-1">
					<span class="block font-medium">{g.title}</span>
					<span class="mt-0.5 block text-sm text-muted">{g.summary}</span>
				</span>
				<Icon name="arrow-right" size={15} class="text-subtle transition-colors group-hover:text-fg" />
			</a>
		</li>
	{/each}
</ul>
