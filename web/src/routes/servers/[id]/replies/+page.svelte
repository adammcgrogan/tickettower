<script lang="ts">
	import { getContext } from 'svelte';
	import {
		api,
		ApiError,
		errorMessage,
		send,
		MAX_SAVED_REPLY_CONTENT,
		MAX_SAVED_REPLY_NAME,
		type Guild,
		type SavedReply,
		type SavedReplyInput
	} from '$lib/api';
	import { APP_NAME } from '$lib/brand';
	import { replyPlaceholders } from '$lib/placeholders';
	import { toast } from '$lib/toast.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PlaceholderChips from '$lib/components/PlaceholderChips.svelte';

	const getGuild = getContext<() => Guild>('guild');
	const guild = $derived(getGuild());

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

	// --- Adding and editing ---

	let editorOpen = $state(false);
	let editing = $state<SavedReply | null>(null);
	let form = $state<SavedReplyInput>({ name: '', content: '' });
	let errors = $state<{ name?: string; content?: string; other?: string }>({});
	let saving = $state(false);
	let textarea = $state<HTMLTextAreaElement>();

	function openEditor(reply?: SavedReply) {
		editing = reply ?? null;
		form = { name: reply?.name ?? '', content: reply?.content ?? '' };
		errors = {};
		editorOpen = true;
	}

	async function save(e?: SubmitEvent) {
		e?.preventDefault();
		if (!replies || saving) return;
		saving = true;
		errors = {};
		try {
			if (editing) {
				const updated = await api<SavedReply>(
					`/guilds/${guild.id}/saved-replies/${editing.id}`,
					send('PATCH', form)
				);
				replies = byName(replies.map((r) => (r.id === updated.id ? updated : r)));
				toast('Saved reply updated');
			} else {
				const created = await api<SavedReply>(`/guilds/${guild.id}/saved-replies`, send('POST', form));
				replies = byName([...replies, created]);
				toast('Saved reply added');
			}
			editorOpen = false;
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
	let deleting = $state<SavedReply | null>(null);
	let removing = $state(false);

	function confirmDelete(reply: SavedReply) {
		deleting = reply;
		deleteOpen = true;
	}

	async function remove() {
		const target = deleting;
		if (!target || !replies) return;
		removing = true;
		try {
			await api(`/guilds/${guild.id}/saved-replies/${target.id}`, send('DELETE'));
			replies = replies.filter((r) => r.id !== target.id);
			deleteOpen = false;
			toast('Saved reply deleted');
		} catch (err) {
			deleteOpen = false;
			toast(errorMessage(err), 'error');
		} finally {
			removing = false;
		}
	}
</script>

<svelte:head><title>Saved replies · {guild.name} · {APP_NAME}</title></svelte:head>

<PageHeader
	title="Saved replies"
	description="Answers your team sends often. Pick one in the reply box on the Tickets page, or send it with /reply in a ticket."
>
	{#snippet actions()}
		{#if replies?.length}
			<button class="btn btn-primary" onclick={() => openEditor()}>
				<Icon name="plus" size={15} /> New saved reply
			</button>
		{/if}
	{/snippet}
</PageHeader>

<div class="mt-6 max-w-3xl">
	{#if error}
		<div class="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm">
			<p class="text-danger">{error}</p>
			<button onclick={load} class="mt-3 text-muted underline-offset-4 hover:text-fg hover:underline">
				Try again
			</button>
		</div>
	{:else if replies === null}
		<div class="space-y-px overflow-hidden rounded-xl border border-border" aria-busy="true">
			{#each Array(3) as _, i (i)}<div class="h-[76px] animate-pulse bg-surface"></div>{/each}
		</div>
	{:else if replies.length === 0}
		<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
			<div class="mx-auto grid size-10 place-items-center rounded-xl bg-elevated text-muted">
				<Icon name="message" />
			</div>
			<p class="mt-4 font-medium">No saved replies yet</p>
			<p class="mx-auto mt-1 max-w-sm text-sm text-muted">
				Write the answers your team gives most, like a refund policy or how to appeal, and send them in a
				couple of clicks.
			</p>
			<button class="btn btn-primary mt-5" onclick={() => openEditor()}>
				<Icon name="plus" size={15} /> Add a saved reply
			</button>
		</div>
	{:else}
		<ul class="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
			{#each replies as r (r.id)}
				<li class="flex items-start gap-4 px-4 py-3.5">
					<div class="min-w-0 flex-1">
						<p class="truncate text-sm font-medium">{r.name}</p>
						<p class="mt-0.5 line-clamp-2 text-sm whitespace-pre-line text-muted">{r.content}</p>
					</div>
					<div class="flex shrink-0 items-center gap-1">
						<button class="btn btn-ghost h-8 px-3" onclick={() => openEditor(r)}>Edit</button>
						<button
							class="btn btn-ghost size-8 p-0"
							onclick={() => confirmDelete(r)}
							aria-label="Delete {r.name}"
							title="Delete"
						>
							<Icon name="trash" size={15} />
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<Dialog
	bind:open={editorOpen}
	title={editing ? 'Edit saved reply' : 'New saved reply'}
	description="It's posted in the ticket under the name of whoever sends it."
>
	<form id="saved-reply" onsubmit={save} class="space-y-4">
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
				bind:value={form.name}
				placeholder="e.g. Refund policy"
				aria-invalid={!!errors.name}
			/>
		</Field>
		<Field
			label="Message"
			for="reply-content"
			hint="Placeholders are filled in when it's sent."
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
		<PlaceholderChips items={replyPlaceholders} target={textarea} bind:value={form.content} />
	</form>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (editorOpen = false)}>Cancel</button>
		<button
			type="submit"
			form="saved-reply"
			class="btn btn-primary"
			disabled={saving || !form.name.trim() || !form.content.trim()}
		>
			{saving ? 'Saving…' : editing ? 'Save changes' : 'Add saved reply'}
		</button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={deleteOpen}
	title="Delete “{deleting?.name ?? ''}”?"
	description="Your team won't be able to send it any more. Tickets it was sent in keep their messages."
>
	{#snippet footer()}
		<button class="btn btn-ghost" onclick={() => (deleteOpen = false)}>Cancel</button>
		<button class="btn btn-danger" onclick={remove} disabled={removing}>
			{removing ? 'Deleting…' : 'Delete'}
		</button>
	{/snippet}
</Dialog>
