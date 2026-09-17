<script lang="ts">
	import { APP_NAME } from '$lib/brand';
	import Logo from '$lib/components/Logo.svelte';
	import Seo from '$lib/components/Seo.svelte';

	const GITHUB_URL = 'https://github.com/adammcgrogan/tickettower';
</script>

<Seo
	title="Terms · {APP_NAME}"
	description="The terms for using {APP_NAME}, the free ticket bot for Discord, and its dashboard."
	path="/terms"
/>

<div class="min-h-dvh">
	<header class="mx-auto flex h-16 max-w-3xl items-center px-5">
		<a href="/"><Logo /></a>
	</header>

	<main class="mx-auto max-w-3xl px-5 pt-8 pb-24">
		<h1 class="text-3xl font-semibold tracking-tight">Terms of service</h1>
		<p class="mt-2 text-sm text-muted">
			These terms cover using the {APP_NAME} bot and its dashboard. By adding the bot to a server or logging in
			to the dashboard, you agree to them.
		</p>

		<div class="mt-10 space-y-8 text-[15px] leading-relaxed text-muted [&_h2]:mb-2 [&_h2]:text-base [&_h2]:font-medium [&_h2]:text-fg [&_li]:ml-5 [&_li]:list-disc">
			<section>
				<h2>Using {APP_NAME}</h2>
				<p>
					{APP_NAME} runs support tickets in Discord servers. You can use it on any server you manage, as long as
					you and your server follow
					<a href="https://discord.com/terms" class="text-accent hover:underline">Discord's Terms of Service</a>
					and
					<a href="https://discord.com/guidelines" class="text-accent hover:underline">Community Guidelines</a>.
				</p>
			</section>

			<section>
				<h2>What you can't do</h2>
				<ul class="space-y-1">
					<li>Use the bot to harass people, spam, or share illegal content.</li>
					<li>Try to access servers, tickets or transcripts you haven't been given access to.</li>
					<li>Overload, attack or deliberately disrupt the bot, the dashboard or its API.</li>
					<li>Resell access to the hosted bot as your own service.</li>
				</ul>
			</section>

			<section>
				<h2>Your server's content</h2>
				<p>
					Tickets, transcripts, ticket panels and settings belong to your server. {APP_NAME} stores them only to run
					the bot, as described in the <a href="/privacy" class="text-accent hover:underline">privacy policy</a>.
					Server managers are responsible for what their team and members post in tickets, and for telling
					members that ticket conversations are saved.
				</p>
			</section>

			<section>
				<h2>Plans and payment</h2>
				<p>
					The free plan costs nothing. The premium plan is a monthly subscription paid through Stripe; it raises
					limits for one server and can be cancelled at any time from the dashboard, staying active until the end
					of the period already paid for. Prices are shown before you pay.
				</p>
			</section>

			<section>
				<h2>Availability</h2>
				<p>
					{APP_NAME} is provided as it is, without guarantees. It may sometimes be unavailable, and features and
					limits can change. It isn't built for emergencies or anything where losing a message would cause harm.
				</p>
			</section>

			<section>
				<h2>Liability</h2>
				<p>
					To the extent the law allows, {APP_NAME} and its operator aren't liable for lost data, lost revenue or
					other indirect damages from using or being unable to use the bot. For a paid plan, liability is limited
					to what you paid in the previous three months.
				</p>
			</section>

			<section>
				<h2>Ending access</h2>
				<p>
					You can stop using {APP_NAME} at any time by removing the bot from your server. Access may be suspended
					for servers or users that break these terms. The bot's own source code is open under the MIT licence,
					which these terms don't change.
				</p>
			</section>

			<section>
				<h2>Changes and contact</h2>
				<p>
					These terms may be updated; the current version is always on this page. Questions go to
					<a href={GITHUB_URL} class="text-accent hover:underline">GitHub</a>.
				</p>
			</section>
		</div>
	</main>
</div>
