<script lang="ts">
	import type { PanelStyle, TicketType } from '$lib/api';
	import { intToHex } from '$lib/api';
	import { APP_NAME } from '$lib/brand';

	let {
		title,
		description,
		color,
		style,
		types
	}: {
		title: string;
		description: string;
		color: number;
		style: PanelStyle;
		types: TicketType[];
	} = $props();

	// Custom emoji can't be rendered without Discord's CDN, so show their name.
	const emojiText = (e: string) => e.replace(/^<a?:(\w+):\d+>$/, ':$1:');

	const rows = $derived.by(() => {
		const out: TicketType[][] = [];
		for (let i = 0; i < types.length; i += 5) out.push(types.slice(i, i + 5));
		return out;
	});
</script>

<!-- A close approximation of how Discord renders the panel message. -->
<div class="rounded-xl bg-[#313338] p-4 font-[system-ui] text-[15px] leading-snug">
	<div class="flex gap-4">
		<div class="grid size-10 shrink-0 place-items-center rounded-full bg-accent text-on-accent">
			<svg viewBox="0 0 24 24" width="28" height="28" fill="currentColor" aria-hidden="true">
				<path fill-rule="evenodd" d="M4.5 2.5h3.4v1.7h2.4V2.5h3.4v1.7h2.4V2.5h3.4v5.1a1.5 1.5 0 0 1-1.5 1.5h-.6v12.4H6.6V9.1H6a1.5 1.5 0 0 1-1.5-1.5zm5.4 14.3h4.2v-3a2.1 2.1 0 0 0-4.2 0z" />
			</svg>
		</div>
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-2">
				<span class="font-medium text-white">{APP_NAME}</span>
				<span class="rounded bg-[#5865f2] px-1.5 py-px text-[10px] font-semibold text-white">APP</span>
				<span class="text-xs text-[#949ba4]">Today at 09:41</span>
			</div>

			<div
				class="mt-1.5 max-w-[432px] rounded border-l-4 bg-[#2b2d31] py-3 pr-4 pl-3"
				style="border-color:{intToHex(color)}"
			>
				<div class="font-semibold break-words text-white">{title || 'Panel title'}</div>
				{#if description}
					<p class="mt-1.5 text-sm break-words whitespace-pre-wrap text-[#dbdee1]">{description}</p>
				{/if}
				<div class="mt-2 text-xs text-[#949ba4]">Powered by {APP_NAME}</div>
			</div>

			{#if types.length === 0}
				<p class="mt-2 text-xs text-[#949ba4] italic">Add ticket types to show buttons here.</p>
			{:else if style === 'dropdown'}
				<div
					class="mt-2 flex max-w-[400px] items-center justify-between rounded border border-[#1e1f22] bg-[#1e1f22] px-3 py-2.5 text-sm text-[#949ba4]"
				>
					Choose a topic…
					<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
				</div>
			{:else}
				<div class="mt-2 space-y-2">
					{#each rows as row, i (i)}
						<div class="flex flex-wrap gap-2">
							{#each row as t (t.id)}
								<span class="inline-flex items-center gap-1.5 rounded bg-[#5865f2] px-4 py-1.5 text-sm font-medium text-white">
									{#if t.emoji}<span>{emojiText(t.emoji)}</span>{/if}
									{t.name}
								</span>
							{/each}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
