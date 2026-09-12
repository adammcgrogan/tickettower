<script lang="ts">
	import { page } from '$app/state';
	import { APP_NAME } from '$lib/brand';
	import { guides } from '$lib/help';
	import Icon from '$lib/components/Icon.svelte';

	const index = $derived(guides.findIndex((g) => g.slug === page.params.slug));
	const guide = $derived(index >= 0 ? guides[index] : undefined);
	const next = $derived(index >= 0 ? guides[index + 1] : undefined);
</script>

<svelte:head><title>{guide?.title ?? 'Help'} · {APP_NAME}</title></svelte:head>

<a href="/help" class="mb-6 inline-flex items-center gap-1.5 text-sm text-muted hover:text-fg md:hidden">
	<Icon name="arrow-left" size={14} /> All guides
</a>

{#if !guide}
	<div class="rounded-xl border border-dashed border-border px-6 py-14 text-center">
		<p class="font-medium">Guide not found</p>
		<p class="mt-1 text-sm text-muted">It may have moved.</p>
		<a href="/help" class="btn btn-secondary mt-5">See all guides</a>
	</div>
{:else}
	<h1 class="font-display text-4xl leading-none font-bold">{guide.title}</h1>

	<article
		class="mt-6 text-[15px] leading-relaxed text-muted [&_a]:text-fg [&_a]:underline [&_a]:underline-offset-4 [&_code]:rounded [&_code]:bg-elevated [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em] [&_code]:text-fg [&_h2]:mt-9 [&_h2]:mb-2 [&_h2]:text-base [&_h2]:font-medium [&_h2]:text-fg [&_li]:ml-5 [&_li]:pl-1 [&_ol]:list-decimal [&_ol]:space-y-1.5 [&_p+p]:mt-3 [&_strong]:font-medium [&_strong]:text-fg [&_ul]:list-disc [&_ul]:space-y-1.5 [&>*+*]:mt-3"
	>
		{#if guide.slug === 'getting-started'}
			<p>
				Members open tickets by clicking a button in your server. This guide gets you from nothing
				to working ticket buttons in about a minute.
			</p>
			<ol>
				<li>
					<a href="/api/invite" data-sveltekit-reload>Add {APP_NAME} to your server</a>. Keep the
					permissions it asks for; it needs them to create tickets.
				</li>
				<li>Log in to the dashboard with Discord and pick your server.</li>
				<li>
					On Home, fill in the quick setup: what members need help with, whether tickets open as
					channels or private threads, which roles handle tickets, and the channel the buttons go in.
				</li>
				<li>Click <strong>Create and publish</strong>. The buttons appear in that channel straight away.</li>
				<li>Click a button yourself to check a ticket opens, then close it.</li>
			</ol>

			<h2>Changing your ticket buttons</h2>
			<p>
				<strong>Ticket buttons</strong> in the dashboard lets you change the message's title,
				description and colour, show the ticket types as buttons or a dropdown menu, and choose which
				ticket types appear. One message can offer up to 25 ticket types.
			</p>
			<p>
				Publishing again edits the message you already posted, so members never see a duplicate.
				Changes to a ticket type, like a new name or emoji, update published buttons automatically.
				If someone deletes the message in Discord, just publish it again.
			</p>

			<h2>What to set up next</h2>
			<ul>
				<li><a href="/help/forms">Questions</a> members answer before their ticket opens</li>
				<li><a href="/help/auto-close">Closing tickets</a> that have gone quiet</li>
				<li>A ticket log channel and dashboard roles, in Settings</li>
			</ul>
		{:else if guide.slug === 'ticket-types'}
			<p>
				A ticket type is one kind of help you offer, like "General support" or "Billing". Each has
				its own button, and decides where its tickets open and who handles them. A server can have
				up to 25.
			</p>

			<h2>Name, emoji and description</h2>
			<p>
				The name is the button's label. The description is shown under the option when your ticket
				buttons are a dropdown menu.
			</p>
			<p>
				For the emoji, use a single emoji, or a custom emoji from your server written as
				<code>&lt;:name:id&gt;</code>. To get that code, type a backslash before the emoji in Discord
				(<code>\:name:</code>) and send it. The bot can only use custom emoji from servers it's in.
			</p>

			<h2>Channel and thread names</h2>
			<p>
				New tickets are named from a pattern, <code>ticket-{'{number}'}</code> by default.
				<code>{'{number}'}</code> is the ticket number with leading zeros (<code>0042</code>) and
				<code>{'{username}'}</code> is the member's username.
			</p>

			<h2>Welcome message</h2>
			<p>
				Posted at the top of every new ticket, with Claim and Close buttons. Use
				<code>{'{user}'}</code> to mention the member. Answers to any questions are added below it.
			</p>

			<h2>Open tickets per member</h2>
			<p>
				How many tickets of this type one member can have open at once, from 1 to 10. Members at
				their limit are pointed to the ticket they already have.
			</p>
		{:else if guide.slug === 'channels-vs-threads'}
			<p>Each ticket type opens its tickets in one of two ways. Both are private to the member and your team.</p>

			<h2>Channels</h2>
			<ul>
				<li>Every ticket gets its own channel, in a category you choose (or at the top of the list).</li>
				<li>Only the member, your support roles and {APP_NAME} can see it.</li>
				<li>When the ticket closes, the channel is deleted after a few seconds. The transcript is kept.</li>
				<li>
					Discord allows 500 channels per server and 50 per category, so very busy servers can run
					out of room.
				</li>
			</ul>

			<h2>Private threads</h2>
			<ul>
				<li>Every ticket is a private thread inside a text channel you choose.</li>
				<li>
					Members must be able to see that channel, but each thread is only visible to the member
					and the people added to it.
				</li>
				<li>
					Your support roles are mentioned in the welcome message, which adds them to the thread.
					People who get a support role later can be added with <code>/ticket add</code>.
				</li>
				<li>When the ticket closes, the thread is archived and locked, so it stays readable.</li>
				<li>Threads don't count towards Discord's channel limits.</li>
			</ul>

			<h2>Which should I pick?</h2>
			<p>
				Channels are the most familiar, and suit servers with a moderate number of tickets and a
				category to keep them in. Threads keep your channel list tidy and suit busy servers. You can
				use different formats for different ticket types.
			</p>
		{:else if guide.slug === 'support-team'}
			<h2>Who counts as support staff</h2>
			<p>
				For each ticket type, anyone with one of its <strong>support roles</strong> is staff for those
				tickets. People with Manage Server or Administrator are staff for every ticket. With no
				support roles, only they can see a ticket type's tickets.
			</p>

			<h2>What staff can do</h2>
			<ul>
				<li><strong>Claim</strong> a ticket to show they're handling it (click again to unclaim).</li>
				<li><strong>Close</strong> it, with an optional reason. The member can close their own ticket too.</li>
				<li><strong>Add or remove</strong> people, and <strong>rename</strong> the ticket.</li>
			</ul>
			<p>The same actions are available as slash commands inside a ticket:</p>
			<ul>
				<li><code>/ticket claim</code>: claim the ticket, or unclaim it if it's yours</li>
				<li><code>/ticket close</code> or <code>/close</code>: close it, with an optional reason</li>
				<li><code>/ticket add</code> and <code>/ticket remove</code>: give someone access, or take it away</li>
				<li><code>/ticket rename</code>: rename it (Discord allows two renames every 10 minutes)</li>
			</ul>

			<h2>If your team can't see tickets</h2>
			<ul>
				<li>Check the role is chosen under <strong>Support roles</strong> on that ticket type.</li>
				<li>
					Channel tickets get their permissions when they open, so a role added later only applies to
					new tickets. Use <code>/ticket add</code> for existing ones.
				</li>
				<li>
					For private threads, staff also need to be able to see the channel the threads are in. See
					<a href="/help/channels-vs-threads">Channels or private threads</a>.
				</li>
			</ul>

			<h2>Dashboard access</h2>
			<p>
				People with Manage Server can always use the dashboard. To let others in, such as support
				leads, add their roles under <strong>Settings → Dashboard roles</strong>. They can do
				everything except change who has dashboard access.
			</p>
		{:else if guide.slug === 'permissions'}
			<p>
				{APP_NAME} asks for everything it needs when you add it. If tickets won't open, a permission
				has usually been turned off for its role, or for the category or channel tickets open in.
				Home and the ticket type pages check this for you and list anything missing, with a link to
				fix it.
			</p>

			<h2>For tickets that open as channels</h2>
			<p>In the server, or in the category you chose:</p>
			<ul>
				<li>View Channels, Manage Channels and Manage Roles, to create each ticket and set who can see it</li>
				<li>Send Messages, Embed Links, Attach Files and Read Message History</li>
			</ul>

			<h2>For tickets that open as private threads</h2>
			<p>In the channel the threads open in:</p>
			<ul>
				<li>View Channels, Create Private Threads and Manage Threads</li>
				<li>Send Messages in Threads, Embed Links and Read Message History</li>
			</ul>

			<h2>For the ticket log</h2>
			<p>View Channels, Send Messages and Embed Links in the log channel.</p>

			<h2>Turning a permission back on</h2>
			<p>
				In Discord, open <strong>Server Settings → Roles</strong>, pick {APP_NAME}'s role and turn the
				permission on. If it's on there but still missing, the category or channel is overriding it:
				open its <strong>Edit Channel → Permissions</strong> and allow it for {APP_NAME}'s role. You
				can also re-add the bot from the <a href="/api/invite" data-sveltekit-reload>invite link</a>
				to restore everything it asks for.
			</p>
		{:else if guide.slug === 'forms'}
			<p>
				A ticket type can ask members a few questions before their ticket opens, like an order
				number or what went wrong. Nobody has to start the conversation from scratch.
			</p>
			<ul>
				<li>Add up to 5 questions, each up to 45 characters, with an optional placeholder.</li>
				<li>
					Each answer is either a <strong>short answer</strong> (up to 200 characters) or a
					<strong>paragraph</strong> (up to 1,000), and can be required or optional.
				</li>
				<li>
					The answers are posted in the ticket's welcome message, so your team and the transcript
					have them.
				</li>
				<li>
					A ticket opened with answers starts as waiting on your team, since the member has already
					explained.
				</li>
				<li>
					Members who already have as many open tickets as they're allowed are told before they see
					the form.
				</li>
				<li>
					If you change the questions while someone is filling them in, they're asked to open the
					ticket again, so answers never end up under the wrong question.
				</li>
			</ul>
		{:else if guide.slug === 'auto-close'}
			<p>
				Tickets often go quiet once the problem is solved. Auto-close tidies them up, without ever
				closing a ticket your team still owes a reply on.
			</p>

			<h2>How it works</h2>
			<ol>
				<li>
					Choose a time for a ticket type: 12 hours, 1 day, 2 days, 3 days or 1 week without
					messages.
				</li>
				<li>
					Only tickets where the last message was from your team count. If the member wrote last,
					the ticket waits for you, however long it takes.
				</li>
				<li>
					Before closing, {APP_NAME} mentions the member with a "Still need help?" message and an
					<strong>I still need help</strong> button. This comes a quarter of the way before the end,
					and at most a day before.
				</li>
				<li>Any message, or that button, keeps the ticket open and restarts the clock.</li>
				<li>
					Otherwise the ticket closes as usual, with the reason "No activity", and the member is
					asked to rate it.
				</li>
			</ol>
			<p>For example, with 1 day chosen, the member is reminded after 18 quiet hours and the ticket closes 6 hours later.</p>
		{:else if guide.slug === 'transcripts'}
			<h2>What's saved</h2>
			<p>
				Every message sent in a ticket is saved, along with the welcome message and any answers to
				questions. Edits are kept, and deleted messages stay in the transcript, marked as deleted.
				Images and files are saved as links to Discord. Messages outside tickets are never read or
				stored.
			</p>

			<h2>Who can read a transcript</h2>
			<ul>
				<li>The member who opened the ticket. The message they get when it closes links to it.</li>
				<li>The ticket type's support roles.</li>
				<li>Anyone who can use your server's dashboard.</li>
			</ul>

			<h2>How long they're kept</h2>
			<p>
				By default, forever. Under <strong>Settings → Transcripts</strong> you can delete them
				automatically 7, 30 or 90 days, 6 months or a year after a ticket closes. Ticket details and
				analytics are kept either way.
			</p>

			<h2>Privacy</h2>
			<p>
				The <a href="/privacy">privacy policy</a> explains everything {APP_NAME} stores, why, and how
				to have it removed.
			</p>
		{/if}
	</article>

	{#if next}
		<a
			href="/help/{next.slug}"
			class="group mt-12 flex items-center justify-between gap-4 border-t border-border pt-6"
		>
			<span>
				<span class="block text-sm text-muted">Next</span>
				<span class="block font-medium">{next.title}</span>
			</span>
			<Icon name="arrow-right" size={15} class="text-subtle transition-colors group-hover:text-fg" />
		</a>
	{/if}
{/if}
