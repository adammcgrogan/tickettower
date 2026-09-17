<script lang="ts">
	import { APP_NAME, SUPPORT_URL } from '$lib/brand';
	import { guides, sections } from '$lib/help';
	import Icon from '$lib/components/Icon.svelte';
	import Seo from '$lib/components/Seo.svelte';

	const start = guides[0];
</script>

<Seo
	title="Help · {APP_NAME}"
	description="Short guides to setting up {APP_NAME}, the free Discord ticket bot, and running tickets with your team."
	path="/help"
/>

<section class="pt-14 pb-10 lg:pt-20">
	<h1 class="font-display text-5xl leading-none font-bold sm:text-6xl">Help</h1>
	<p class="mt-4 max-w-xl text-lg text-pretty text-muted">
		Short guides to setting up {APP_NAME} and running tickets with your team.
	</p>
</section>

<a
	href="/help/{start.slug}"
	class="group flex flex-col gap-5 rounded-xl border border-border bg-surface p-6 transition-colors hover:border-border-strong sm:flex-row sm:items-center sm:p-8"
>
	<div class="min-w-0 flex-1">
		<p class="text-sm text-muted">New here? Start with</p>
		<p class="mt-1 text-xl font-semibold">{start.title}</p>
		<p class="mt-1 text-sm text-muted">{start.summary}</p>
	</div>
	<span class="btn btn-secondary shrink-0 self-start sm:self-auto">
		Read the guide <Icon name="arrow-right" size={14} />
	</span>
</a>

<div class="mt-14 grid gap-x-12 gap-y-12 sm:grid-cols-2 lg:grid-cols-3">
	{#each sections as s (s.title)}
		<section>
			<h2 class="text-sm font-medium text-subtle">{s.title}</h2>
			<ul class="mt-3 divide-y divide-border border-t border-border">
				{#each s.guides as g (g.slug)}
					<li>
						<a href="/help/{g.slug}" class="group flex items-start gap-3 py-4">
							<span class="min-w-0 flex-1">
								<span class="block font-medium">{g.title}</span>
								<span class="mt-1 block text-sm text-muted">{g.summary}</span>
							</span>
							<Icon
								name="arrow-right"
								size={15}
								class="mt-1 text-subtle transition-all group-hover:translate-x-0.5 group-hover:text-fg"
							/>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
</div>

<section
	class="mt-20 flex flex-col gap-4 border-t border-border pt-8 sm:flex-row sm:items-center sm:justify-between"
>
	<div>
		<h2 class="font-medium">Still stuck?</h2>
		<p class="mt-1 text-sm text-muted">Ask us and we'll help you get set up.</p>
	</div>
	<a href={SUPPORT_URL} target="_blank" rel="noopener" class="btn btn-secondary self-start sm:self-auto">
		Get support <Icon name="external" size={13} />
	</a>
</section>
