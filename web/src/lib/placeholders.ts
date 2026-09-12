/**
 * Placeholders in ticket type names, welcome messages and staff replies. The
 * bot fills them in (`channelName` and `welcomeText` in ticketbot/tickets.go,
 * `replyText` in ticketbot/replies.go); these helpers mirror it for the
 * editor's previews.
 */

import type { Question } from './api';

export type Placeholder = { token: string; label: string };

/** Replaces known {placeholders} in one pass, leaving unknown ones as written. */
export const fillPlaceholders = (text: string, values: Record<string, string>) =>
	text.replace(/\{(\w+)\}/g, (m, key: string) => values[key] ?? m);

/** Lowercases text and joins its words with hyphens, like the bot does for names. */
export const slug = (s: string) =>
	s
		.toLowerCase()
		.split(/[^\p{L}\p{N}]+/u)
		.filter(Boolean)
		.join('-');

/** One placeholder per question, labelled with the question. */
export const answerPlaceholders = (questions: Question[]): Placeholder[] =>
	questions.map((q, i) => ({ token: `{answer${i + 1}}`, label: q.label.trim() || `Answer ${i + 1}` }));

export const namePlaceholders: Placeholder[] = [
	{ token: '{number}', label: 'Ticket number' },
	{ token: '{username}', label: "Member's username" },
	{ token: '{type}', label: 'Ticket type' }
];

export const welcomePlaceholders: Placeholder[] = [
	{ token: '{user}', label: 'Mention the member' },
	{ token: '{username}', label: "Member's name" },
	{ token: '{number}', label: 'Ticket number' },
	{ token: '{type}', label: 'Ticket type' },
	{ token: '{server}', label: 'Server name' },
	{ token: '{support}', label: 'Mention support roles' }
];

export const replyPlaceholders: Placeholder[] = [
	{ token: '{user}', label: 'Mention the member' },
	{ token: '{username}', label: "Member's name" },
	{ token: '{number}', label: 'Ticket number' },
	{ token: '{type}', label: 'Ticket type' },
	{ token: '{server}', label: 'Server name' },
	{ token: '{staff}', label: 'Your name' }
];

export const ratingPlaceholders: Placeholder[] = [
	{ token: '{staff}', label: 'Who handled it' },
	{ token: '{username}', label: "Member's name" },
	{ token: '{number}', label: 'Ticket number' },
	{ token: '{type}', label: 'Ticket type' },
	{ token: '{server}', label: 'Server name' }
];
