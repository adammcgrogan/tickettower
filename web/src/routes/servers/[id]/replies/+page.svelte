<script lang="ts">
	import { getContext, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		api,
		ApiError,
		errorMessage,
		send,
		MAX_SAVED_REPLY_CONTENT,
		MAX_SAVED_REPLY_NAME,
		type Guild,
		type SavedReply,
		type SavedReplyInput,
		type User
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { timeAgo } from '$lib/format';
	import { renderMarkdown } from '$lib/markdown';
	import { fillPlaceholders, replyPlaceholders } from '$lib/placeholders';
	import { toast } from '$lib/toast.svelte';
	import { guardUnsaved } from '$lib/unsaved';
	import Dialog from '$lib/components/Dialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Field from '$lib/components/Field.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import LoadError from '$lib/components/LoadError.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PlaceholderChips from '$lib/components/PlaceholderChips.svelte';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());
	const getUser = getContext<() => User | null>('user');
	const me = $derived(getUser());

	let replies = $state<SavedReply[] | null>(null);
	let error = $state('');

	function load() {
		const id = guild.id;
		replies = null;
		error = '';
		api<SavedReply[]>(`/guilds/${id}/saved-replies`)
			.then((r) => {
				if (guild.id === id) replies = r;
			})
			.catch((e) => {
				if (guild.id === id) error = errorMessage(e);
			});
	}
	$effect(load);

	// The same order as the server, so a new or renamed reply lands in place.
	const byName = (list: SavedReply[]) =>
		[...list].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));

	let query = $state('');
	const shown = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const list = replies ?? [];
		return q ? list.filter((r) => r.name.toLowerCase().includes(q) || r.content.toLowerCase().includes(q)) : list;
	});

	// --- The open reply, in the URL so links and the back button work ---
	// ?r=<id> edits one; ?r=new writes a new one.

	const selected = $derived(page.url.searchParams.get('r'));
	const current = $derived(
		selected && selected !== 'new' ? (replies?.find((r) => String(r.id) === selected) ?? null) : null
	);
	const open = $derived(selected === 'new' || !!current);

	let form = $state<SavedReplyInput>({ name: '', content: '' });
	let errors = $state<{ name?: string; content?: string; other?: string }>({});
	let saving = $state(false);
	let textarea = $state<HTMLTextAreaElement>();
	let nameInput = $state<HTMLInputElement>();

	// Loads the chosen reply into the form whenever the choice changes.
	let loadedFor = '';
	$effect(() => {
		const key = selected === 'new' ? 'new' : current ? `${current.id}` : '';
		if (key === loadedFor) return;
		loadedFor = key;
		form = { name: current?.name ?? '', content: current?.content ?? '' };
		errors = {};
		if (key === 'new') tick().then(() => nameInput?.focus());
	});

	const dirty = $derived(
		open && (form.name !== (current?.name ?? '') || form.content !== (current?.content ?? ''))
	);
	guardUnsaved(() => dirty && !saving);

	function choose(r: SavedReply | 'new' | null, force = false) {
		if (!force && dirty && !confirm('You have unsaved changes. Leave without saving them?')) return;
		const params = new URLSearchParams(page.url.searchParams);
		if (r === null) params.delete('r');
		else params.set('r', r === 'new' ? 'new' : String(r.id));
		const qs = params.toString();
		// goto passes the unsaved guard, which is why choose() asks itself.
		goto(page.url.pathname + (qs ? `?${qs}` : ''), { replaceState: true, noScroll: true, keepFocus: true });
	}

	async function save(e?: SubmitEvent) {
		e?.preventDefault();
		if (!replies || saving) return;
		saving = true;
		errors = {};
		try {
			if (current) {
				const updated = await api<SavedReply>(
					`/guilds/${guild.id}/saved-replies/${current.id}`,
					send('PATCH', form)
				);
				replies = byName(replies.map((r) => (r.id === updated.id ? updated : r)));
				form = { name: updated.name, content: updated.content };
				toast('Saved reply updated');
			} else {
				const created = await api<SavedReply>(`/guilds/${guild.id}/saved-replies`, send('POST', form));
				replies = byName([...replies, created]);
				loadedFor = String(created.id);
				form = { name: created.name, content: created.content };
				toast('Saved reply added');
				choose(created, true);
			}
		} catch (err) {
			if (err instanceof ApiError && (err.field === 'name' || err.field === 'content')) {
				errors = { [err.field]: err.message };
			} else errors = { other: errorMessage(err) };
		} finally {
			saving = false;
		}
	}

	// --- Deleting ---

	let deleteOpen = $state(false);
	let removing = $state(false);

	async function remove() {
		const target = current;
		if (!target || !replies) return;
		removing = true;
		try {
			await api(`/guilds/${guild.id}/saved-replies/${target.id}`, send('DELETE'));
			replies = replies.filter((r) => r.id !== target.id);
			deleteOpen = false;
			form = { name: '', content: '' };
			toast('Saved reply deleted');
			choose(null, true);
		} catch (err) {
			deleteOpen = false;
			toast(errorMessage(err), 'error');
		} finally {
			removing = false;
		}
	}

	// --- The preview: the message as a member sees it, placeholders filled in ---

	const example = $derived({
		user: '<@1>',
		username: 'Maya',
		number: '42',
		type: 'Billing',
		server: guild.name,
		staff: me?.display_name ?? 'You'
	});
	const previewHTML = $derived(
		renderMarkdown(fillPlaceholders(form.content, example), new Map([['1', 'Maya']]))
	);
</script>

<svelte:head><title>Saved replies · {guild.name} · {APP_NAME}</title></svelte:head>

<PageHeader
	title="Saved replies"
	description="Answers your team sends often. Pick one from the reply box in Tickets, or send it with /reply in a ticket."
>
	{#snippet actions()}
		{#if replies?.length}
			<button class="btn btn-primary" onclick={() => choose('new')}>
				<Icon name="plus" size={15} /> New saved reply
			</button>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-8">
	{#if error}
		<LoadError message={error} onretry={load} />
	{:else if replies === null}
		<div class="space-y-px overflow-hidden rounded-xl border border-border lg:max-w-sm" aria-busy="true">
			{#each Array(3) as _, i (i)}<div class="h-[68px] animate-pulse bg-surface"></div>{/each}
		</div>
	{:else if replies.length === 0 && !open}
		<EmptyState icon="message" title="No saved replies yet">
			Write the answers your team gives most, like a refund policy or how to appeal, and send them in a couple of
			clicks.
			{#snippet action()}
				<button class="btn btn-primary" onclick={() => choose('new')}>
					<Icon name="plus" size={15} /> New saved reply
				</button>
			{/snippet}
		</EmptyState>
	{:else}
		<div class="grid items-start gap-6 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
			<!-- The list -->
			<div class={open ? 'hidden lg:block' : ''}>
				{#if replies.length > 5}
					<label class="relative mb-3 block">
						<span class="sr-only">Search saved replies</span>
						<Icon
							name="search"
							size={15}
							class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle"
						/>
						<input bind:value={query} placeholder="Search saved replies" class="input pl-9" />
					</label>
				{/if}
				<ul class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
					{#each shown as r (r.id)}
						{@const active = current?.id === r.id}
						<li>
							<button
								class="relative block w-full px-4 py-3 text-left transition-colors {active
									? 'bg-elevated'
									: 'hover:bg-elevated/50'}"
								onclick={() => choose(r)}
								aria-current={active ? 'true' : undefined}
							>
								{#if active}<span class="absolute inset-y-0 left-0 w-0.5 bg-accent"></span>{/if}
								<span class="block truncate text-sm font-medium">{r.name}</span>
								<span class="mt-0.5 block truncate text-sm text-muted">{r.content}</span>
							</button>
						</li>
					{:else}
						<li class="px-4 py-6 text-center text-sm text-muted">No saved reply matches that.</li>
					{/each}
				</ul>
				<p class="mt-3 text-xs text-subtle">
					{replies.length} saved {replies.length === 1 ? 'reply' : 'replies'}
				</p>
			</div>

			<!-- The editor -->
			<div class="min-w-0 {open ? '' : 'hidden lg:block'}">
				{#if !open}
					<div class="grid min-h-80 place-items-center rounded-xl border border-dashed border-border px-6 text-center">
						<div>
							<p class="font-medium">Pick a saved reply to edit it</p>
							<p class="mt-1 text-sm text-muted">Or write a new one for an answer your team keeps typing.</p>
						</div>
					</div>
				{:else}
					<button
						class="mb-4 inline-flex items-center gap-1.5 text-sm text-muted hover:text-fg lg:hidden"
						onclick={() => choose(null)}
					>
						<Icon name="arrow-left" size={14} /> All saved replies
					</button>
					<form onsubmit={save} class="card">
						<div class="space-y-5 p-5">
							{#if errors.other}<p class="text-sm text-danger">{errors.other}</p>{/if}
							<Field
								label="Name"
								for="reply-name"
								hint="Your team picks replies by name, here and in /reply."
								error={errors.name}
							>
								<input
									id="reply-name"
									class="input"
									maxlength={MAX_SAVED_REPLY_NAME}
									bind:this={nameInput}
									bind:value={form.name}
									placeholder="e.g. Refund policy"
									aria-invalid={!!errors.name}
								/>
							</Field>
							<Field
								label="Message"
								for="reply-content"
								hint="Discord formatting works, like **bold** and links."
								help="saved-replies"
								error={errors.content}
							>
								<textarea
									id="reply-content"
									class="input"
									rows="7"
									maxlength={MAX_SAVED_REPLY_CONTENT}
									bind:this={textarea}
									bind:value={form.content}
									placeholder="Hi {'{user}'}, thanks for waiting…"
									aria-invalid={!!errors.content}
								></textarea>
							</Field>
							<div class="space-y-2">
								<p class="text-xs text-muted">Click to insert. They're filled in when it's sent.</p>
								<PlaceholderChips items={replyPlaceholders} target={textarea} bind:value={form.content} />
							</div>

							<div>
								<p class="mb-2 text-xs text-muted">How it looks in a ticket</p>
								<div class="rounded-xl bg-[#313338] p-4 text-[15px] leading-[1.375] text-[#dbdee1]">
									<div class="flex gap-4">
										{#if me}
											<img src={me.avatar_url} alt="" class="size-10 shrink-0 rounded-full" />
										{:else}
											<div class="size-10 shrink-0 rounded-full bg-[#5865f2]"></div>
										{/if}
										<div class="min-w-0 flex-1">
											<div class="flex items-baseline gap-2">
												<span class="font-medium text-white">{me?.display_name ?? 'You'}</span>
												<span class="text-xs text-[#949ba4]">Today at 09:41</span>
											</div>
											{#if form.content.trim()}
												<div class="break-words whitespace-pre-wrap">
													<!-- Safe: renderMarkdown escapes all input before formatting. -->
													{@html previewHTML}
												</div>
											{:else}
												<p class="text-[#949ba4] italic">Your message shows here.</p>
											{/if}
										</div>
									</div>
								</div>
							</div>
						</div>
						<div class="flex items-center gap-2 border-t border-border px-5 py-3">
							{#if current}
								<button
									type="button"
									class="btn btn-ghost h-8 px-2.5 text-danger hover:text-danger"
									onclick={() => (deleteOpen = true)}
								>
									<Icon name="trash" size={14} /> Delete
								</button>
								<span class="text-xs text-subtle">Updated {timeAgo(current.updated_at)}</span>
							{/if}
							<div class="ml-auto flex gap-2">
								{#if !current}
									<button type="button" class="btn btn-ghost" onclick={() => choose(null)}>Cancel</button>
								{/if}
								<button
									type="submit"
									class="btn btn-primary"
									disabled={saving || !dirty || !form.name.trim() || !form.content.trim()}
								>
									{saving ? 'Saving…' : current ? 'Save changes' : 'Add saved reply'}
								</button>
							</div>
						</div>
					</form>
				{/if}
			</div>
		</div>
	{/if}
</div>

<Dialog
	bind:open={deleteOpen}
	title="Delete “{current?.name ?? ''}”?"
	description="Your team won't be able to send it any more. Tickets it was sent in keep their messages."
>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (deleteOpen = false)}>Cancel</button>
		<button class="btn btn-danger" onclick={remove} disabled={removing}>
			{removing ? 'Deleting…' : 'Delete'}
		</button>
	{/snippet}
</Dialog>
