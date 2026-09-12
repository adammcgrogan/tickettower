<script lang="ts">
	import { page } from '$app/state';
	import type { SetupProblem } from '$lib/api';
	import type { GuideSlug } from '$lib/help';
	import Icon from './Icon.svelte';

	/** Problems from the setup check, each linking to where it's fixed. */
	let {
		problems,
		guildId,
		onrecheck
	}: { problems: SetupProblem[]; guildId: string; onrecheck?: () => Promise<unknown> } = $props();

	const base = $derived(`/servers/${guildId}`);
	function href(p: SetupProblem) {
		switch (p.kind) {
			case 'ticket_type':
				return `${base}/ticket-types/${p.id}`;
			case 'panel':
				return `${base}/buttons/${p.id}`;
			case 'unlisted':
				return `${base}/buttons`;
			case 'log_channel':
				return `${base}/settings`;
		}
	}

	// The help guide that explains each kind of problem.
	const guides: Record<SetupProblem['kind'], GuideSlug> = {
		ticket_type: 'permissions',
		panel: 'getting-started',
		unlisted: 'getting-started',
		log_channel: 'permissions'
	};

	let checking = $state(false);
	async function recheck() {
		checking = true;
		await onrecheck?.().catch(() => {});
		checking = false;
	}
</script>

<section class="rounded-xl border border-danger/30 bg-surface">
	<div class="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
		<div>
			<h2 class="font-semibold">Needs attention</h2>
			<p class="mt-0.5 text-sm text-muted">
				{problems.length === 1 ? 'This stops' : 'These stop'} members from opening tickets until it's fixed.
			</p>
		</div>
		{#if onrecheck}
			<button class="btn btn-ghost h-8 shrink-0 px-3" onclick={recheck} disabled={checking}>
				{checking ? 'Checking…' : 'Check again'}
			</button>
		{/if}
	</div>
	<ul class="divide-y divide-border">
		{#each problems as p, i (i)}
			{@const to = href(p)}
			<li class="flex items-start gap-3.5 px-5 py-4">
				<Icon name="alert" class="mt-0.5 text-danger" />
				<div class="min-w-0 flex-1">
					<div class="text-sm font-medium">{p.title}</div>
					<div class="mt-0.5 text-sm text-muted">
						{p.detail}
						<a
							href="/help/{guides[p.kind]}"
							target="_blank"
							class="whitespace-nowrap text-fg underline-offset-4 hover:underline"
						>
							Learn more
						</a>
					</div>
				</div>
				{#if to !== page.url.pathname}
					<a href={to} class="btn btn-secondary h-8 shrink-0 px-3">Fix</a>
				{/if}
			</li>
		{/each}
	</ul>
</section>
