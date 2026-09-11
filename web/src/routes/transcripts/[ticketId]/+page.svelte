<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, errorMessage, intToHex, type Ticket } from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { discordURL, timeAgo } from '$lib/format';
	import { renderMarkdown } from '$lib/markdown';
	import GuildIcon from '$lib/components/GuildIcon.svelte';
	import Icon from '$lib/components/Icon.svelte';

	type Embed = { title?: string; description?: string; color?: number; footer?: string };
	type Attachment = { name: string; url: string; size: number; content_type?: string };
	type Message = {
		id: string;
		author_id: string;
		author_name: string;
		author_avatar: string;
		author_bot: boolean;
		content: string;
		embeds: Embed[];
		attachments: Attachment[];
		created_at: string;
		edited_at: string | null;
		deleted_at: string | null;
	};
	type Transcript = {
		ticket: Ticket & { closed_by_name: string | null };
		messages: Message[];
		guild: { id: string; name: string; icon_url: string | null; can_manage: boolean };
	};

	let data = $state<Transcript | null>(null);
	let error = $state('');

	onMount(async () => {
		try {
			data = await api<Transcript>(`/transcripts/${page.params.ticketId}`);
		} catch (e) {
			error = errorMessage(e);
		}
	});

	const users = $derived.by(() => {
		const m = new Map<string, string>();
		if (!data) return m;
		m.set(data.ticket.opener_id, data.ticket.opener_name);
		for (const msg of data.messages) m.set(msg.author_id, msg.author_name);
		return m;
	});

	// Group consecutive messages from the same author, as Discord does.
	const groups = $derived.by(() => {
		const out: Message[][] = [];
		for (const msg of data?.messages ?? []) {
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

<svelte:head>
	<title>{data ? `Ticket #${data.ticket.number} · ${data.guild.name}` : 'Transcript'} · {APP_NAME}</title>
</svelte:head>

<main class="mx-auto max-w-4xl px-5 py-10">
	{#if error}
		<div class="mx-auto max-w-md py-16 text-center">
			<h1 class="text-lg font-semibold">Transcript unavailable</h1>
			<p class="mt-1 text-sm text-muted">
				It may not exist, or you may not have access. Transcripts are visible to the member who
				opened the ticket and the server's support team.
			</p>
		</div>
	{:else if !data}
		<div class="card h-40 animate-pulse"></div>
		<div class="card mt-4 h-96 animate-pulse"></div>
	{:else}
		{@const t = data.ticket}
		<header class="card p-5">
			<div class="flex flex-wrap items-start justify-between gap-4">
				<div class="flex items-center gap-3">
					<GuildIcon name={data.guild.name || 'Server'} url={data.guild.icon_url} size={40} />
					<div>
						<div class="text-xs text-muted">{data.guild.name}</div>
						<h1 class="text-lg font-semibold tracking-tight">
							<span class="text-muted tabular-nums">#{t.number}</span>
							{t.type_name}
						</h1>
					</div>
				</div>
				<div class="flex items-center gap-2">
					{#if t.status === 'open'}
						<a href={discordURL(data.guild.id, t.channel_id)} target="_blank" rel="noopener" class="btn btn-secondary h-8 px-3">
							Open in Discord <Icon name="external" size={13} />
						</a>
					{/if}
					{#if data.guild.can_manage}
						<a href="/servers/{data.guild.id}/tickets" class="btn btn-ghost h-8 px-3">All tickets</a>
					{/if}
				</div>
			</div>

			<dl class="mt-5 grid grid-cols-2 gap-x-6 gap-y-3 border-t border-border pt-4 text-sm sm:grid-cols-4">
				<div>
					<dt class="text-xs text-muted">Opened by</dt>
					<dd class="mt-0.5 truncate">{t.opener_name}</dd>
				</div>
				<div>
					<dt class="text-xs text-muted">Opened</dt>
					<dd class="mt-0.5" title={time(t.opened_at)}>{timeAgo(t.opened_at)}</dd>
				</div>
				<div>
					<dt class="text-xs text-muted">Claimed by</dt>
					<dd class="mt-0.5 truncate {t.claimed_by_name ? '' : 'text-subtle'}">{t.claimed_by_name ?? 'Nobody'}</dd>
				</div>
				<div>
					<dt class="text-xs text-muted">Status</dt>
					<dd class="mt-0.5">
						{#if t.status === 'open'}
							<span class="inline-flex items-center gap-1.5"><span class="size-1.5 rounded-full bg-success"></span>Open</span>
						{:else}
							<span title={t.closed_at ? time(t.closed_at) : ''}>
								Closed{t.closed_by_name ? ` by ${t.closed_by_name}` : ''}
							</span>
						{/if}
					</dd>
				</div>
				{#if t.close_reason}
					<div class="col-span-full">
						<dt class="text-xs text-muted">Close reason</dt>
						<dd class="mt-0.5">{t.close_reason}</dd>
					</div>
				{/if}
			</dl>
		</header>

		<section class="mt-4 overflow-hidden rounded-xl border border-border bg-[#313338] py-4 text-[15px] leading-[1.375] text-[#dbdee1]">
			{#if data.messages.length === 0}
				<p class="px-5 py-10 text-center text-sm text-[#949ba4]">
					No messages were saved for this ticket. It may predate transcripts, or its history was
					removed by the server's retention setting.
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
		<p class="mt-3 text-center text-xs text-subtle">
			Attachment links are hosted by Discord and may stop working after a while.
		</p>
	{/if}
</main>
