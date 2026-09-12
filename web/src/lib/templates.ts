/**
 * Starter setups for quick setup: a few ticket types with questions and
 * welcome messages, plus the ticket buttons that offer them. Where tickets open
 * and who handles them are asked separately, since they depend on the server.
 * Everything here must pass the API's ticket type and panel validation.
 */

import type { PanelStyle, Question, TicketTypeInput } from './api';

export type TypeTemplate = Pick<
	TicketTypeInput,
	'name' | 'emoji' | 'description' | 'name_format' | 'welcome_message' | 'max_open_per_user' | 'questions' | 'auto_close_hours'
>;

export type Template = {
	id: string;
	name: string;
	summary: string;
	panel: { title: string; description: string; style: PanelStyle };
	/** Empty for the simple setup, which makes one type named by the user. */
	types: TypeTemplate[];
};

const ask = (label: string, placeholder = '', style: Question['style'] = 'short', required = true): Question => ({
	label,
	placeholder,
	style,
	required
});

const generalSupport: TypeTemplate = {
	name: 'General support',
	emoji: '💬',
	description: 'Questions about anything else',
	name_format: 'support-{number}',
	welcome_message: '',
	max_open_per_user: 1,
	questions: [],
	auto_close_hours: 48
};

const defaultPanel = {
	title: 'Need a hand?',
	description: "Pick a topic below and we'll open a private ticket for you. Our team will be with you shortly.",
	style: 'buttons' as const
};

export const templates: Template[] = [
	{
		id: 'simple',
		name: 'Start simple',
		summary: 'One ticket type, named by you.',
		panel: defaultPanel,
		types: []
	},
	{
		id: 'community',
		name: 'Community or gaming',
		summary: 'Support, reports and appeals.',
		panel: defaultPanel,
		types: [
			generalSupport,
			{
				name: 'Report a member',
				emoji: '🚩',
				description: 'Tell the moderators about someone, privately',
				name_format: 'report-{number}',
				welcome_message:
					'Thanks for letting us know, {user}. Only you and the moderators can see this ticket. Add any screenshots or message links that help us look into it.',
				max_open_per_user: 3,
				questions: [
					ask('Who are you reporting?', 'Their username'),
					ask('What happened?', 'What they did, and where', 'paragraph')
				],
				auto_close_hours: 72
			},
			{
				name: 'Appeal a punishment',
				emoji: '📝',
				description: 'Ask us to look again at a warning, mute or timeout',
				name_format: 'appeal-{number}',
				welcome_message:
					'Thanks, {user}. A moderator will review your appeal and reply here, so please be patient.',
				max_open_per_user: 1,
				questions: [
					ask('What were you punished for?', 'For example, a timeout in #general'),
					ask('Why should we reconsider?', '', 'paragraph')
				],
				auto_close_hours: 72
			}
		]
	},
	{
		id: 'store',
		name: 'Store or business',
		summary: 'Orders, billing and refunds.',
		panel: {
			title: 'Help with an order',
			description: "Choose a topic and we'll open a private ticket with our team. Have your order number ready.",
			style: 'buttons'
		},
		types: [
			{
				name: 'Order help',
				emoji: '📦',
				description: "Problems with an order you've placed",
				name_format: 'order-{number}',
				welcome_message: "Thanks, {user}. We've got your order details, and {support} will be with you shortly.",
				max_open_per_user: 2,
				questions: [
					ask('Order number', 'For example, 10492'),
					ask("What's wrong with it?", 'Missing, damaged, late…', 'paragraph')
				],
				auto_close_hours: 48
			},
			{
				name: 'Billing',
				emoji: '💳',
				description: 'Payments, invoices and subscriptions',
				name_format: 'billing-{number}',
				welcome_message: '',
				max_open_per_user: 1,
				questions: [ask("What's it about?", "For example, a charge you don't recognise", 'paragraph')],
				auto_close_hours: 48
			},
			{
				name: 'Refunds',
				emoji: '💸',
				description: 'Ask for your money back',
				name_format: 'refund-{number}',
				welcome_message: 'Thanks, {user}. Someone from {support} will look at your refund request and reply here.',
				max_open_per_user: 1,
				questions: [ask('Order number', 'For example, 10492'), ask('Why would you like a refund?', '', 'paragraph')],
				auto_close_hours: 72
			}
		]
	},
	{
		id: 'creator',
		name: 'Creator',
		summary: 'Partnerships, commissions and support.',
		panel: {
			title: 'Get in touch',
			description: "Pick what it's about and we'll open a private ticket for you.",
			style: 'buttons'
		},
		types: [
			{
				name: 'Partnerships',
				emoji: '🤝',
				description: 'Sponsorships, brand deals and collaborations',
				name_format: 'partner-{number}',
				welcome_message: "Thanks for reaching out, {user}! We'll read your proposal and get back to you here.",
				max_open_per_user: 1,
				questions: [
					ask('Who are you?', 'Your name, brand or channel'),
					ask('What do you have in mind?', '', 'paragraph')
				],
				auto_close_hours: 168
			},
			{
				name: 'Commissions',
				emoji: '🎨',
				description: 'Request custom work',
				name_format: 'commission-{number}',
				welcome_message: "Thanks, {user}! We'll look at your request and reply here with any questions.",
				max_open_per_user: 1,
				questions: [
					ask('What would you like made?', 'Describe it, and link any references', 'paragraph'),
					ask('Budget', 'For example, $50', 'short', false),
					ask('Deadline', 'For example, by 20 October', 'short', false)
				],
				auto_close_hours: 168
			},
			generalSupport
		]
	}
];

/** The ticket types a template creates. The simple setup makes one, named by the user. */
export function typesFor(t: Template, name: string): TypeTemplate[] {
	if (t.types.length) return t.types;
	return [
		{
			name,
			emoji: '',
			description: '',
			name_format: 'ticket-{number}',
			welcome_message: '',
			max_open_per_user: 1,
			questions: [],
			auto_close_hours: null
		}
	];
}
