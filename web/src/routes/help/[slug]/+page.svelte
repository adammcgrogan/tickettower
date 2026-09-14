<script lang="ts">
	import { page } from '$app/state';
	import { APP_NAME, SUPPORT_URL } from '$lib/brand';
	import { guides } from '$lib/help';
	import Icon from '$lib/components/Icon.svelte';

	const index = $derived(guides.findIndex((g) => g.slug === page.params.slug));
	const guide = $derived(index >= 0 ? guides[index] : undefined);
	const prev = $derived(index > 0 ? guides[index - 1] : undefined);
	const next = $derived(index >= 0 ? guides[index + 1] : undefined);

	// "On this page" lists the guide's headings, which get ids to link to.
	let article = $state<HTMLElement>();
	let headings = $state<{ id: string; text: string }[]>([]);
	$effect(() => {
		void guide;
		if (!article) {
			headings = [];
			return;
		}
		headings = [...article.querySelectorAll('h2')].map((h) => {
			const text = h.textContent ?? '';
			h.id = text
				.toLowerCase()
				.replace(/[^a-z0-9]+/g, '-')
				.replace(/^-|-$/g, '');
			return { id: h.id, text };
		});
	});
</script>

<svelte:head><title>{guide?.title ?? 'Help'} · {APP_NAME}</title></svelte:head>

<a href="/help" class="mb-6 inline-flex items-center gap-1.5 text-sm text-muted hover:text-fg md:hidden">
	<Icon name="arrow-left" size={14} /> All guides
</a>

{#if !guide}
	<div class="max-w-2xl rounded-xl border border-dashed border-border px-6 py-14 text-center">
		<p class="font-medium">Guide not found</p>
		<p class="mt-1 text-sm text-muted">It may have moved.</p>
		<a href="/help" class="btn btn-secondary mt-5">See all guides</a>
	</div>
{:else}
	<div class="xl:grid xl:grid-cols-[minmax(0,1fr)_11rem] xl:gap-12">
		<div class="max-w-2xl min-w-0">
			<p class="text-sm text-muted">{guide.section}</p>
			<h1 class="mt-2 font-display text-4xl leading-none font-bold sm:text-5xl">{guide.title}</h1>
			<p class="mt-4 text-lg text-pretty text-muted">{guide.summary}</p>

			<article
				bind:this={article}
				class="mt-8 text-[15px] leading-relaxed text-muted [&_a]:text-fg [&_a]:underline [&_a]:underline-offset-4 [&_code]:rounded [&_code]:bg-elevated [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em] [&_code]:text-fg [&_h2]:mt-10 [&_h2]:mb-2 [&_h2]:scroll-mt-10 [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:text-fg [&_li]:ml-5 [&_li]:pl-1 [&_ol]:list-decimal [&_ol]:space-y-1.5 [&_p+p]:mt-3 [&_strong]:font-medium [&_strong]:text-fg [&_ul]:list-disc [&_ul]:space-y-1.5 [&>*+*]:mt-3"
			>
				{#if guide.slug === 'getting-started'}
					<p>
						Members open tickets by clicking a button in your server, or with <code>/ticket open</code>.
						This guide gets you from nothing to working ticket buttons in about a minute.
					</p>
					<ol>
						<li>
							<a href="/api/invite" data-sveltekit-reload>Add {APP_NAME} to your server</a>. Keep the
							permissions it asks for; it needs them to create tickets.
						</li>
						<li>Log in to the dashboard with Discord and pick your server.</li>
						<li>
							On Home, fill in the quick setup: pick a template for your kind of server (a community, a
							store or a creator) or start with one ticket type, then choose whether tickets open as
							channels or private threads, which roles handle tickets, and the channel the buttons go in.
							Templates come with questions and welcome messages you can change later.
						</li>
						<li>Click <strong>Create and publish</strong>. The buttons appear in that channel straight away.</li>
						<li>Click a button yourself to check a ticket opens, then close it.</li>
					</ol>

					<h2>Changing your ticket buttons</h2>
					<p>
						<strong>Ticket buttons</strong> in the dashboard lets you change the message's title,
						description and colour, add a banner image or thumbnail, show the ticket types as buttons or a
						dropdown menu (with your own prompt), and choose which ticket types appear. One message can
						offer up to 25 ticket types. Each button's colour and label come from its ticket type.
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
					<p>
						Its settings are split into tabs: <strong>Basics</strong> (name, where tickets open and the
						support team) is all you need to start. The others, <strong>Button</strong>,
						<strong>Questions and welcome</strong>, <strong>Who can open</strong>, <strong>While open</strong>
						and <strong>Closing</strong>, have sensible defaults and are saved together.
					</p>

					<h2>Name, emoji and description</h2>
					<p>
						The name is the button's label, unless you set a <strong>button label</strong> of its own (so
						the button can say "Get help" while the type is called "General support"). Each button also
						has a <strong>colour</strong>: blurple, grey, green or red. The description is shown under the
						option when your ticket buttons are a dropdown menu.
					</p>
					<p>
						For the emoji, use a single emoji, or a custom emoji from your server written as
						<code>&lt;:name:id&gt;</code>. To get that code, type a backslash before the emoji in Discord
						(<code>\:name:</code>) and send it. The bot can only use custom emoji from servers it's in.
					</p>

					<h2>Channel and thread names</h2>
					<p>
						New tickets are named from a pattern, <code>ticket-{'{number}'}</code> by default. Click a
						placeholder under the field to add it:
					</p>
					<ul>
						<li><code>{'{number}'}</code>: the ticket number with leading zeros (<code>0042</code>)</li>
						<li><code>{'{username}'}</code>: the member's username</li>
						<li>
							<code>{'{type}'}</code>: the ticket type's name in lowercase with hyphens, so
							<code>{'{type}'}-{'{number}'}</code> gives <code>billing-0042</code>
						</li>
						<li>
							<code>{'{answer1}'}</code> to <code>{'{answer5}'}</code>: the member's answer to that
							question, in the same lowercase style
						</li>
					</ul>

					<h2>Welcome message</h2>
					<p>
						Posted at the top of every new ticket, with Claim and Close buttons. Answers to any
						questions are added below it. You can use these placeholders:
					</p>
					<ul>
						<li><code>{'{user}'}</code>: mentions the member</li>
						<li><code>{'{username}'}</code>: the member's name, without a mention</li>
						<li><code>{'{number}'}</code>: the ticket number (<code>0042</code>)</li>
						<li><code>{'{type}'}</code>: the ticket type's name</li>
						<li><code>{'{server}'}</code>: your server's name</li>
						<li><code>{'{support}'}</code>: mentions your support roles, or says "the team" if there are none</li>
						<li><code>{'{answer1}'}</code> to <code>{'{answer5}'}</code>: the member's answer to that question</li>
					</ul>
					<p>
						Mentions in the welcome message show who they mean but don't notify anyone. The member and
						your support roles are still pinged once when the ticket opens.
					</p>

					<h2>Claiming</h2>
					<p>
						By default, claiming a ticket just tells everyone who's handling it. For channel tickets you
						can make it do more: <strong>Others read only</strong> lets the rest of the team follow along
						without replying over the claimer, and <strong>Others lose access</strong> hides the ticket
						from them. Pick roles that keep full access, such as senior staff; administrators always keep
						it. Unclaiming, or moving the ticket, gives the team their access back. Private threads can't
						limit a role's access, so this only applies to channel tickets.
					</p>

					<h2>Reply target and reminders</h2>
					<p>
						A <strong>reply target</strong> is how long a member should wait for the team, counted from
						their first unanswered message. Analytics shows how often the first reply beat it, and Home
						marks tickets past it as overdue. <strong>Remind the team</strong> posts a reminder in the
						ticket or your log channel once a ticket has waited that long, mentioning the claimer (or the
						support roles if nobody has claimed it), and can repeat until someone replies.
					</p>
					<p>
						Tickets <strong>on hold</strong> are left out of all of this. Staff put a ticket on hold with
						<code>/ticket hold</code> (or Hold on the Tickets page) while waiting on something else, such
						as a payment provider. It leaves the team's queue and won't close for inactivity until the
						member writes again or staff resume it.
					</p>

					<h2>Who can open these tickets</h2>
					<p>
						By default, anyone who can see your ticket buttons can open any type, from the buttons or with
						<code>/ticket open</code>. Each type can narrow that down:
					</p>
					<ul>
						<li>
							<strong>Required roles</strong>: members need at least one of them, for example Verified for
							a Partnership type. Members without one are told which role they need.
						</li>
						<li><strong>Blocked roles</strong>: members with any of them can't open the type, such as a Muted role.</li>
						<li>
							<strong>Wait between tickets</strong>: how long a member waits after one of their tickets of
							this type closes before they can open another, so nobody can open, close and open again all
							day.
						</li>
					</ul>
					<p>
						To stop one person opening any ticket at all, staff can use <code>/ticket block</code> (with an
						optional reason, which the member sees) and <code>/ticket unblock</code>, or the blocked list in
						Settings. Blocking doesn't close tickets they already have.
					</p>

					<h2>Open tickets per member</h2>
					<p>
						How many tickets of this type one member can have open at once, from 1 to 10. Members at
						their limit are pointed to the ticket they already have.
					</p>

					<h2>When a ticket closes</h2>
					<p>
						The member gets a direct message saying their ticket was closed, with the reason and a link
						to the transcript. By default it also asks them to rate the ticket from one to five stars,
						with an optional comment. Ratings appear in Analytics and, if you have one, your log channel.
					</p>
					<p>
						Turn <strong>Ask for a rating</strong> off for types where it would feel wrong, such as
						reporting a member. You can also reword the request, with <code>{'{staff}'}</code> for whoever
						claimed the ticket (or "the team").
					</p>
				{:else if guide.slug === 'channels-vs-threads'}
					<p>Each ticket type opens its tickets in one of two ways. Both are private to the member and your team.</p>

					<h2>Channels</h2>
					<ul>
						<li>Every ticket gets its own channel, in a category you choose (or at the top of the list).</li>
						<li>Only the member, your support roles and {APP_NAME} can see it.</li>
						<li>
							When the ticket closes, the channel is deleted after a few seconds. The transcript is kept,
							but the ticket can't be reopened.
						</li>
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
						<li>
							<strong>Close</strong> it, with an optional reason. The member can close their own ticket
							too; if they type <code>/close</code> without a reason, they're asked to confirm first.
						</li>
						<li>
							<strong>Reopen</strong> a closed thread ticket with the button on its close message, from
							the member's closing DM, or from the Tickets page. Channel tickets can't be reopened, since
							the channel is deleted when they close.
						</li>
						<li><strong>Add or remove</strong> people, and <strong>rename</strong> the ticket.</li>
						<li>
							<strong>Open</strong> a ticket for a member, for example to follow up on a report. They're
							pinged into it, you're added to it, and the welcome message says you opened it. The type's
							rules on who can open it don't apply, though the member's limit of open tickets does.
						</li>
					</ul>
					<p>The same actions are available as slash commands inside a ticket:</p>
					<ul>
						<li><code>/ticket claim</code>: claim the ticket, or unclaim it if it's yours</li>
						<li><code>/ticket close</code> or <code>/close</code>: close it, with an optional reason</li>
						<li><code>/ticket add</code> and <code>/ticket remove</code>: give someone access, or take it away</li>
						<li><code>/ticket rename</code>: rename it (Discord allows two renames every 10 minutes)</li>
					<li>
						<code>/ticket move</code>: move it to another ticket type, which hands it to that type's
						support team (and category, for channels). You can also move tickets from the Tickets page.
					</li>
						<li><code>/reply</code>: send one of your <a href="/help/saved-replies">saved replies</a></li>
						<li>
							<code>/ticket hold</code> and <code>/ticket resume</code>: park a ticket that's waiting on
							something other than the member or your team, and pick it back up.
						</li>
						<li>
							<code>/ticket closerequest</code>: ask the member to confirm the ticket is resolved. They
							get Close ticket and Keep open buttons, and if they don't answer within the time you
							choose (a day by default) it closes on your behalf.
						</li>
						<li>
							<code>/ticket block</code> and <code>/ticket unblock</code>: stop someone opening tickets in
							the server, or let them again. These work anywhere, not just inside a ticket.
						</li>
						<li>
							<code>/ticket open</code> with <code>for</code>: open a ticket for a member, from anywhere in
							the server. Members can use <code>/ticket open</code> too, for the types on ticket buttons
							they can see.
						</li>
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
						People with Manage Server can always use the dashboard. To let others in, add their roles
						under <strong>Settings → Dashboard access</strong>, each at one of three levels:
					</p>
					<ul>
						<li><strong>Viewer</strong>: read tickets, transcripts and analytics.</li>
						<li>
							<strong>Support</strong>: also reply to, close, reopen, move and hold tickets from the
							dashboard, and block members.
						</li>
						<li>
							<strong>Admin</strong>: also change ticket types, ticket buttons, saved replies and settings.
						</li>
					</ul>
					<p>
						Only people with Manage Server can change who has access. A member with several roles gets
						the highest level among them.
					</p>
				{:else if guide.slug === 'saved-replies'}
					<p>
						Saved replies are answers your team sends often, like a refund policy or how to appeal a ban.
						Write them once, then send them into any ticket in a couple of clicks.
					</p>

					<h2>Adding a saved reply</h2>
					<p>
						In the dashboard, open <strong>Saved replies</strong> under Setup and choose
						<strong>New saved reply</strong>. Give it a name your team will recognise, like "Refund
						policy", and write the message. A server can have up to 50.
					</p>

					<h2>Sending one from the dashboard</h2>
					<p>
						On the Tickets page, pick a reply from <strong>Saved replies</strong> next to the reply box.
						It's added to the box, so you can change anything before you press Send.
					</p>

					<h2>Sending one from Discord</h2>
					<p>
						In a ticket, type <code>/reply</code>, start typing the reply's name and pick it from the
						list. {APP_NAME} posts it straight away, under your name. Only support staff can use it.
					</p>

					<h2>Placeholders</h2>
					<p>These are filled in when the reply is sent:</p>
					<ul>
						<li><code>{'{user}'}</code>: mentions the member, without pinging them</li>
						<li><code>{'{username}'}</code>: the member's name</li>
						<li><code>{'{number}'}</code>: the ticket number</li>
						<li><code>{'{type}'}</code>: the ticket type</li>
						<li><code>{'{server}'}</code>: your server's name</li>
						<li><code>{'{staff}'}</code>: the name of whoever sends it</li>
					</ul>
					<p>
						They work in replies you type yourself in the dashboard too. A saved reply counts as your
						team's response, just like one typed by hand.
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
					<p>
						Staff are brought into each private thread by the bot mentioning their support role. Discord
						only lets it mention a role that isn't set to "Allow anyone to @mention this role" if the bot
						has <strong>Mention @everyone, @here and All Roles</strong>, so keep that on, or make your
						support roles mentionable. The setup check on Home tells you if either is missing.
					</p>

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
							told (and asked to rate it, if the type asks for ratings).
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

			{#if prev || next}
				<nav aria-label="More guides" class="mt-14 grid gap-6 border-t border-border pt-6 sm:grid-cols-2">
					{#if prev}
						<a href="/help/{prev.slug}" class="group">
							<span class="flex items-center gap-1.5 text-sm text-muted">
								<Icon name="arrow-left" size={13} /> Previous
							</span>
							<span class="mt-1 block font-medium underline-offset-4 group-hover:underline">{prev.title}</span>
						</a>
					{:else}
						<span class="hidden sm:block"></span>
					{/if}
					{#if next}
						<a href="/help/{next.slug}" class="group sm:text-right">
							<span class="flex items-center gap-1.5 text-sm text-muted sm:justify-end">
								Next <Icon name="arrow-right" size={13} />
							</span>
							<span class="mt-1 block font-medium underline-offset-4 group-hover:underline">{next.title}</span>
						</a>
					{/if}
				</nav>
			{/if}

			<p class="mt-10 text-sm text-muted">
				Still stuck?
				<a href={SUPPORT_URL} target="_blank" rel="noopener" class="text-fg underline-offset-4 hover:underline">
					Get support
				</a>
			</p>
		</div>

		<aside class="hidden xl:block">
			{#if headings.length}
				<div class="sticky top-10">
					<p class="text-xs font-medium text-subtle">On this page</p>
					<ul class="mt-3 space-y-2 text-sm">
						{#each headings as h (h.id)}
							<li><a href="#{h.id}" class="text-muted transition-colors hover:text-fg">{h.text}</a></li>
						{/each}
					</ul>
				</div>
			{/if}
		</aside>
	</div>
{/if}
