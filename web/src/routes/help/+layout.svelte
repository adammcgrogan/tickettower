<script lang="ts">
	import { page } from '$app/state';
	import { SUPPORT_URL } from '$lib/brand';
	import { guides } from '$lib/help';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';

	let { children } = $props();

	const current = $derived(page.params.slug ?? '');
</script>

<div class="min-h-dvh">
	<header class="mx-auto flex h-16 max-w-5xl items-center justify-between px-5">
		<a href="/"><Logo /></a>
		<a href="/servers" class="btn btn-secondary">Dashboard</a>
	</header>

	<div class="mx-auto grid max-w-5xl gap-10 px-5 pt-8 pb-24 md:grid-cols-[13rem_minmax(0,1fr)]">
		<!-- On small screens the list is only shown on /help itself. -->
		<nav aria-label="Guides" class="{current ? 'hidden md:block' : 'hidden'} md:sticky md:top-8 md:self-start">
			<a
				href="/help"
				aria-current={current ? undefined : 'page'}
				class="block rounded-lg px-3 py-2 text-sm transition-colors {current
					? 'text-muted hover:text-fg'
					: 'bg-elevated font-medium text-fg'}"
			>
				All guides
			</a>
			<ul class="mt-1 space-y-0.5">
				{#each guides as g (g.slug)}
					<li>
						<a
							href="/help/{g.slug}"
							aria-current={g.slug === current ? 'page' : undefined}
							class="block rounded-lg px-3 py-2 text-sm transition-colors {g.slug === current
								? 'bg-elevated font-medium text-fg'
								: 'text-muted hover:bg-elevated/60 hover:text-fg'}"
						>
							{g.title}
						</a>
					</li>
				{/each}
			</ul>
		</nav>

		<main class="max-w-2xl min-w-0">
			{@render children()}

			<section class="mt-16 border-t border-border pt-6">
				<h2 class="font-medium">Still stuck?</h2>
				<p class="mt-1 text-sm text-muted">
					Ask us and we'll help you get set up.
					<a
						href={SUPPORT_URL}
						target="_blank"
						rel="noopener"
						class="inline-flex items-center gap-1 text-fg underline-offset-4 hover:underline"
					>
						Get support <Icon name="external" size={12} />
					</a>
				</p>
			</section>
		</main>
	</div>
</div>
