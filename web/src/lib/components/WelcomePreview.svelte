<script lang="ts">
	import type { Question, Role } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { renderMarkdown } from '$lib/markdown';

	/** A close approximation of the welcome message the bot posts in a ticket. */
	let {
		name,
		welcome,
		fallback,
		questions,
		supportRoles
	}: { name: string; welcome: string; fallback: string; questions: Question[]; supportRoles: Role[] } =
		$props();

	const users = new Map([['1', 'member']]);
	const text = $derived((welcome.trim() || fallback).replaceAll('{user}', '<@1>'));
</script>

<div class="rounded-xl bg-[#313338] p-4 font-[system-ui] text-[15px] leading-snug text-[#dbdee1]">
	<div class="flex gap-4">
		<div class="grid size-10 shrink-0 place-items-center rounded-full bg-accent text-on-accent">
			<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
				<path d="M4 12h10" /><path d="m10 6 6 6-6 6" /><path d="M20 5v14" />
			</svg>
		</div>
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-2">
				<span class="font-medium text-white">{APP_NAME}</span>
				<span class="rounded bg-[#5865f2] px-1.5 py-px text-[10px] font-semibold text-white">APP</span>
				<span class="text-xs text-[#949ba4]">Today at 09:41</span>
			</div>
			<div class="mt-0.5 flex flex-wrap gap-1">
				<span class="md-mention">@member</span>
				{#each supportRoles as r (r.id)}<span class="md-mention">@{r.name}</span>{/each}
			</div>

			<div class="mt-1.5 max-w-[432px] rounded border-l-4 border-[#f2b544] bg-[#2b2d31] py-3 pr-4 pl-3">
				<div class="font-semibold break-words text-white">{name || 'Ticket type'} · #42</div>
				<div class="mt-1.5 text-sm break-words whitespace-pre-wrap">
					<!-- Safe: renderMarkdown escapes all input before formatting. -->
					{@html renderMarkdown(text, users)}
				</div>
				{#each questions as q, i (i)}
					<div class="mt-2 text-sm">
						<div class="font-semibold break-words text-white">{q.label || `Question ${i + 1}`}</div>
						<div class="text-[#949ba4] italic">The member's answer</div>
					</div>
				{/each}
				<div class="mt-2 text-xs text-[#949ba4]">
					Staff can claim this ticket. Either side can close it when you're done.
				</div>
			</div>

			<div class="mt-2 flex gap-2 text-sm font-medium text-white">
				<span class="rounded bg-[#4e5058] px-4 py-1.5">🙋 Claim</span>
				<span class="rounded bg-[#da373c] px-4 py-1.5">🔒 Close</span>
			</div>
		</div>
	</div>
</div>
