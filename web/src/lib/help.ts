/** The help guides, in reading order. Their content is in routes/help/[slug]. */
export const guides = [
	{
		slug: 'getting-started',
		title: 'Set up your first ticket buttons',
		summary: 'Add the bot, create a ticket type and post buttons members can click.'
	},
	{
		slug: 'ticket-types',
		title: 'Ticket types',
		summary: 'Names, emoji, channel names, welcome messages and limits.'
	},
	{
		slug: 'channels-vs-threads',
		title: 'Channels or private threads',
		summary: 'Where tickets open, and how to choose.'
	},
	{
		slug: 'support-team',
		title: 'Your support team',
		summary: 'Who can see and manage tickets, commands, and dashboard access.'
	},
	{
		slug: 'permissions',
		title: 'Permissions the bot needs',
		summary: 'What to turn on so tickets can open, and how to check.'
	},
	{
		slug: 'forms',
		title: 'Questions before a ticket opens',
		summary: 'Ask members for details up front.'
	},
	{
		slug: 'auto-close',
		title: 'Closing inactive tickets',
		summary: 'How reminders and automatic closing work.'
	},
	{
		slug: 'transcripts',
		title: 'Transcripts and privacy',
		summary: "What's saved, who can read it and how long it's kept."
	}
] as const;

export type GuideSlug = (typeof guides)[number]['slug'];
