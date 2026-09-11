<script lang="ts">
	import { intToHex, type Attachment, type TranscriptMessage } from '$lib/api';
	import { timeAgo } from '$lib/format';
	import { renderMarkdown } from '$lib/markdown';
	import Icon from './Icon.svelte';

	/** A ticket's saved messages, laid out the way Discord shows them. */
	let {
		messages,
		openerId,
		openerName
	}: { messages: TranscriptMessage[]; openerId: string; openerName: string } = $props();

	const users = $derived.by(() => {
		const m = new Map<string, string>([[openerId, openerName]]);
		for (const msg of messages) m.set(msg.author_id, msg.author_name);
		return m;
	});

	// Group consecutive messages from the same author, as Discord does.
	const groups = $derived.by(() => {
		const out: TranscriptMessage[][] = [];
		for (const msg of messages) {
			const last = out.at(-1);
			const prev = last?.at(-1);
			const sameAuthor = prev && prev.author_id === msg.author_id;
			const close = prev && new Date(msg.created_at).getTime() - new Date(prev.created_at).getTime() < 7 * 60_000;
			if (last && sameAuthor && close) last.push(msg);
			else out.push([msg]);
		}
		return out;
	});

	const time = (iso: string) =>
		new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	const clock = (iso: string) => new Date(iso).toLocaleTimeString(undefined, { timeStyle: 'short' });
	const isImage = (a: Attachment) => a.content_type?.startsWith('image/');
	const size = (n: number) =>
		n > 1_048_576 ? `${(n / 1_048_576).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`;
</script>

<section
	class="overflow-hidden rounded-xl border border-border bg-[#313338] py-4 text-[15px] leading-[1.375] text-[#dbdee1]"
	aria-label="Conversation"
>
	{#if messages.length === 0}
		<p class="px-5 py-10 text-center text-sm text-[#949ba4]">
			No messages were saved for this ticket. It may predate transcripts, or its history was removed by
			the server's retention setting.
		</p>
	{/if}
	{#each groups as group (group[0].id)}
		{@const first = group[0]}
		<article class="group/msg mt-4 flex gap-4 px-4 py-0.5 first:mt-0 hover:bg-[#2e3035]">
			{#if first.author_avatar}
				<img src={first.author_avatar} alt="" class="mt-0.5 size-10 shrink-0 rounded-full" loading="lazy" />
			{:else}
				<div class="mt-0.5 size-10 shrink-0 rounded-full bg-[#5865f2]"></div>
			{/if}
			<div class="min-w-0 flex-1">
				<div class="flex items-baseline gap-2">
					<span class="font-medium text-white">{first.author_name}</span>
					{#if first.author_bot}
						<span class="rounded bg-[#5865f2] px-1.5 py-px text-[10px] font-semibold text-white">APP</span>
					{/if}
					<time class="text-xs text-[#949ba4]" datetime={first.created_at}>{time(first.created_at)}</time>
				</div>
				{#each group as msg, i (msg.id)}
					<div class="relative {i > 0 ? 'mt-1' : ''} {msg.deleted_at ? 'opacity-60' : ''}">
						{#if i > 0}
							<time
								class="absolute top-0.5 -left-14 hidden w-10 text-right text-[10px] text-[#949ba4] group-hover/msg:block"
								datetime={msg.created_at}>{clock(msg.created_at)}</time
							>
						{/if}
						{#if msg.content}
							<div class="break-words whitespace-pre-wrap {msg.deleted_at ? 'line-through' : ''}">
								<!-- Safe: renderMarkdown escapes all input before formatting. -->
								{@html renderMarkdown(msg.content, users)}
								{#if msg.edited_at}<span class="text-[10px] text-[#949ba4]"> (edited)</span>{/if}
							</div>
						{/if}
						{#if msg.deleted_at}
							<div class="text-[11px] text-[#f0616d]">Deleted {timeAgo(msg.deleted_at)}</div>
						{/if}

						{#each msg.embeds as embed, j (j)}
							<div
								class="mt-1 max-w-[520px] rounded border-l-4 bg-[#2b2d31] py-2.5 pr-4 pl-3"
								style="border-color:{embed.color ? intToHex(embed.color) : '#1e1f22'}"
							>
								{#if embed.title}<div class="font-semibold text-white">{embed.title}</div>{/if}
								{#if embed.description}
									<div class="mt-1 text-sm break-words whitespace-pre-wrap">
										{@html renderMarkdown(embed.description, users)}
									</div>
								{/if}
								{#each embed.fields ?? [] as field, k (k)}
									<div class="mt-2 text-sm">
										<div class="font-semibold text-white">{field.name}</div>
										<div class="break-words whitespace-pre-wrap">
											{@html renderMarkdown(field.value, users)}
										</div>
									</div>
								{/each}
								{#if embed.footer}<div class="mt-2 text-xs text-[#949ba4]">{embed.footer}</div>{/if}
							</div>
						{/each}

						{#each msg.attachments as a (a.url)}
							{#if isImage(a)}
								<a href={a.url} target="_blank" rel="noopener noreferrer" class="mt-1 block w-fit">
									<img src={a.url} alt={a.name} class="max-h-72 max-w-full rounded-md" loading="lazy" />
								</a>
							{:else}
								<a
									href={a.url}
									target="_blank"
									rel="noopener noreferrer"
									class="mt-1 flex w-fit max-w-full items-center gap-3 rounded-md border border-[#1e1f22] bg-[#2b2d31] px-3 py-2"
								>
									<Icon name="transcript" size={22} class="text-[#949ba4]" />
									<span class="min-w-0">
										<span class="block truncate text-sm text-[#00a8fc]">{a.name}</span>
										<span class="block text-xs text-[#949ba4]">{size(a.size)}</span>
									</span>
								</a>
							{/if}
						{/each}
					</div>
				{/each}
			</div>
		</article>
	{/each}
</section>
