/** The help guides, in reading order. Their content is in routes/help/[slug]. */
export const guides = [
	{
		slug: 'getting-started',
		title: 'Set up your first ticket buttons',
		summary: 'Add the bot, create a ticket type and post buttons members can click.',
		section: 'Getting started'
	},
	{
		slug: 'ticket-types',
		title: 'Ticket types',
		summary: 'Names, emoji, channel names, welcome messages and limits.',
		section: 'Getting started'
	},
	{
		slug: 'channels-vs-threads',
		title: 'Channels or private threads',
		summary: 'Where tickets open, and how to choose.',
		section: 'Getting started'
	},
	{
		slug: 'support-team',
		title: 'Your support team',
		summary: 'Who can see and manage tickets, commands, and dashboard access.',
		section: 'Running tickets'
	},
	{
		slug: 'forms',
		title: 'Questions before a ticket opens',
		summary: 'Ask members for details up front.',
		section: 'Running tickets'
	},
	{
		slug: 'auto-close',
		title: 'Closing inactive tickets',
		summary: 'How reminders and automatic closing work.',
		section: 'Running tickets'
	},
	{
		slug: 'permissions',
		title: 'Permissions the bot needs',
		summary: 'What to turn on so tickets can open, and how to check.',
		section: 'Reference'
	},
	{
		slug: 'transcripts',
		title: 'Transcripts and privacy',
		summary: "What's saved, who can read it and how long it's kept.",
		section: 'Reference'
	}
] as const;

export type Guide = (typeof guides)[number];
export type GuideSlug = Guide['slug'];

/** The guides grouped for the help index and sidebar. */
export const sections = (['Getting started', 'Running tickets', 'Reference'] as const).map((title) => ({
	title,
	guides: guides.filter((g) => g.section === title)
}));
