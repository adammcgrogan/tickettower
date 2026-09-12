<script lang="ts">
	import { page } from '$app/state';
	import { SUPPORT_URL } from '$lib/brand';
	import { sections } from '$lib/help';
	import Logo from '$lib/components/Logo.svelte';

	let { children } = $props();

	// Guides get a sidebar; the index uses the full width.
	const current = $derived(page.params.slug ?? '');
</script>

<div class="min-h-dvh">
	<header class="border-b border-border">
		<div class="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-5">
			<div class="flex items-center gap-3">
				<a href="/"><Logo /></a>
				<span class="h-5 w-px bg-border" aria-hidden="true"></span>
				<a href="/help" class="text-sm font-medium text-muted transition-colors hover:text-fg">Help</a>
			</div>
			<nav class="flex items-center gap-1 text-sm">
				<a href={SUPPORT_URL} target="_blank" rel="noopener" class="btn btn-ghost hidden sm:inline-flex">
					Get support
				</a>
				<a href="/servers" class="btn btn-secondary">Dashboard</a>
			</nav>
		</div>
	</header>

	<div class="mx-auto max-w-6xl px-5 pb-24">
		{#if current}
			<div class="grid gap-12 pt-10 md:grid-cols-[13rem_minmax(0,1fr)] lg:gap-16 lg:pt-14">
				<aside class="hidden md:block">
					<nav aria-label="Guides" class="sticky top-10 space-y-8">
						{#each sections as s (s.title)}
							<div>
								<p class="pl-4 text-xs font-medium text-subtle">{s.title}</p>
								<ul class="mt-2 border-l border-border">
									{#each s.guides as g (g.slug)}
										{@const active = g.slug === current}
										<li>
											<a
												href="/help/{g.slug}"
												aria-current={active ? 'page' : undefined}
												class="-ml-px block border-l py-1.5 pl-4 text-sm transition-colors {active
													? 'border-fg font-medium text-fg'
													: 'border-transparent text-muted hover:border-border-strong hover:text-fg'}"
											>
												{g.title}
											</a>
										</li>
									{/each}
								</ul>
							</div>
						{/each}
					</nav>
				</aside>
				<div class="min-w-0">{@render children()}</div>
			</div>
		{:else}
			{@render children()}
		{/if}
	</div>
</div>
