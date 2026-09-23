<script lang="ts">
	import Icon from '../Icon.svelte';
	import TicketStub from '../TicketStub.svelte';

	/** A static copy of the dashboard's ticket inbox, filled with example tickets. */
	const waiting = [
		{ number: 42, name: 'alex', type: 'Billing', claimed: 'Unclaimed', wait: '14m', active: true },
		{ number: 40, name: 'mira.k', type: 'General support', claimed: 'Priya', wait: '9m' },
		{ number: 41, name: 'jonah', type: 'Report a member', claimed: 'Sam', wait: '2m' }
	];
	const theirs = [{ number: 38, name: 'sol', type: 'Billing', claimed: 'Priya', wait: '1h' }];
</script>

<div class="overflow-hidden rounded-2xl border border-border bg-bg shadow-2xl shadow-black/30" aria-hidden="true">
	<div class="flex h-[25rem]">
		<div class="w-full shrink-0 border-r border-border sm:w-60">
			<div class="px-4 pt-4">
				<div class="font-display text-2xl leading-none font-bold">Tickets</div>
				<div class="mt-3 flex gap-4 border-b border-border text-xs">
					<span class="-mb-px border-b-2 border-accent pb-2 font-medium">Open 4</span>
					<span class="pb-2 text-muted">Closed</span>
					<span class="pb-2 text-muted">All</span>
				</div>
			</div>
			<div class="mt-2 flex justify-between border-b border-border px-4 py-1.5 text-[11px]">
				<span class="font-medium text-accent-ink">Waiting on your team</span><span class="text-subtle">3</span>
			</div>
			{#each waiting as t (t.number)}
				<div class="relative flex gap-2.5 px-4 py-2.5 {t.active ? 'bg-elevated' : ''}">
					{#if t.active}<span class="absolute inset-y-0 left-0 w-0.5 bg-accent"></span>{/if}
					<TicketStub number={t.number} tone="waiting" size="sm" />
					<div class="min-w-0 flex-1 text-xs">
						<div class="flex justify-between gap-2">
							<span class="truncate text-sm font-medium">{t.name}</span>
							<span class="font-medium text-accent-ink">{t.wait}</span>
						</div>
						<div class="mt-0.5 flex justify-between gap-2 text-muted">
							<span class="truncate">{t.type}</span><span class="text-subtle">{t.claimed}</span>
						</div>
					</div>
				</div>
			{/each}
			<div class="flex justify-between border-y border-border px-4 py-1.5 text-[11px]">
				<span class="font-medium text-muted">Waiting on the member</span><span class="text-subtle">1</span>
			</div>
			{#each theirs as t (t.number)}
				<div class="flex gap-2.5 px-4 py-2.5">
					<TicketStub number={t.number} size="sm" />
					<div class="min-w-0 flex-1 text-xs">
						<div class="flex justify-between gap-2">
							<span class="truncate text-sm font-medium">{t.name}</span><span class="text-subtle">{t.wait}</span>
						</div>
						<div class="mt-0.5 truncate text-muted">{t.type}</div>
					</div>
				</div>
			{/each}
		</div>

		<div class="hidden min-w-0 flex-1 flex-col sm:flex">
			<div class="flex items-center gap-3 border-b border-border px-4 py-2.5">
				<TicketStub number={42} tone="waiting" size="sm" />
				<div class="min-w-0 flex-1">
					<div class="truncate text-sm font-semibold">alex</div>
					<div class="truncate text-xs text-accent-ink">Billing, waiting on your team</div>
				</div>
				<span class="rounded-md border border-border px-2 py-1 text-xs">Claim</span>
				<span class="flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs">
					<Icon name="lock" size={11} /> Close
				</span>
			</div>
			<div class="flex flex-1 flex-col justify-end p-3">
				<div class="rounded-lg bg-[#313338] p-3 text-[13px] leading-snug text-[#dbdee1]">
					<div class="flex gap-2.5">
						<div class="grid size-7 shrink-0 place-items-center rounded-full bg-[#3ba55d] text-xs font-semibold text-white">A</div>
						<div class="min-w-0">
							<div class="font-medium text-white">alex <span class="text-[10px] font-normal text-[#949ba4]">Today at 09:52</span></div>
							<p>I was charged twice for my annual plan. Can one of them be refunded?</p>
						</div>
					</div>
				</div>
				<div class="mt-2.5 rounded-lg border border-border bg-surface p-2.5">
					<div class="flex gap-3 text-xs"><span class="font-medium">Reply</span><span class="text-subtle">Private note</span></div>
					<div class="mt-2 text-xs text-subtle">Reply to alex…</div>
					<div class="mt-2 flex justify-end">
						<span class="flex items-center gap-1 rounded-md bg-accent px-2 py-1 text-xs font-medium text-on-accent">
							<Icon name="send" size={11} /> Send
						</span>
					</div>
				</div>
			</div>
		</div>
	</div>
</div>
