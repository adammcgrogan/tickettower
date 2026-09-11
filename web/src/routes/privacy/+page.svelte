<script lang="ts">
	import { APP_NAME } from '$lib/brand';
	import Logo from '$lib/components/Logo.svelte';

	const GITHUB_URL = 'https://github.com/adammcgrogan/ticketsbot';
</script>

<svelte:head><title>Privacy · {APP_NAME}</title></svelte:head>

<div class="min-h-dvh">
	<header class="mx-auto flex h-16 max-w-3xl items-center px-5">
		<a href="/"><Logo /></a>
	</header>

	<main class="mx-auto max-w-3xl px-5 pt-8 pb-24">
		<h1 class="text-3xl font-semibold tracking-tight">Privacy policy</h1>
		<p class="mt-2 text-sm text-muted">
			This explains what {APP_NAME} stores and why. {APP_NAME} is open source, so you can check
			exactly how it works on <a href={GITHUB_URL} class="text-accent hover:underline">GitHub</a>.
		</p>

		<div class="mt-10 space-y-8 text-[15px] leading-relaxed text-muted [&_h2]:mb-2 [&_h2]:text-base [&_h2]:font-medium [&_h2]:text-fg [&_li]:ml-5 [&_li]:list-disc">
			<section>
				<h2>What we store</h2>
				<ul class="space-y-1">
					<li><strong class="text-fg">Servers:</strong> the ID, name and icon of servers the bot is in, plus the settings you configure in the dashboard.</li>
					<li><strong class="text-fg">Tickets:</strong> who opened, claimed and closed each ticket, when, its type, and any close reason.</li>
					<li><strong class="text-fg">Transcripts:</strong> messages sent inside ticket channels and threads, including author names, avatars and attachment links. Messages outside tickets are never stored.</li>
					<li><strong class="text-fg">Feedback:</strong> ratings and comments members choose to leave after a ticket closes.</li>
					<li><strong class="text-fg">Dashboard logins:</strong> your Discord user ID, name and avatar, and a session token, for as long as you stay logged in.</li>
				</ul>
			</section>

			<section>
				<h2>Why</h2>
				<p>
					Only to run the bot: opening and managing tickets, showing transcripts to the member who
					opened a ticket and the server's support team, and producing analytics for server
					managers. Data is never sold, shared with third parties, or used for advertising.
				</p>
			</section>

			<section>
				<h2>Message content</h2>
				<p>
					{APP_NAME} uses Discord's Message Content intent solely to save transcripts of ticket
					conversations. Server managers can choose how long transcripts are kept in the dashboard
					settings; older transcripts are deleted automatically.
				</p>
			</section>

			<section>
				<h2>Who can see it</h2>
				<p>
					Server data is only visible in the dashboard to people who can manage that server, and to
					members with a dashboard role the server's managers have chosen. A transcript is visible to
					the member who opened the ticket, those same dashboard users, and the ticket type's support
					roles.
				</p>
			</section>

			<section>
				<h2>Removing your data</h2>
				<p>
					Removing the bot from a server stops all collection. To have stored data deleted, open an
					issue on <a href={GITHUB_URL} class="text-accent hover:underline">GitHub</a> or contact the
					bot's operator, and include the server or user ID.
				</p>
			</section>
		</div>
	</main>
</div>
