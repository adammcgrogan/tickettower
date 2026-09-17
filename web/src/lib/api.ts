export type User = {
	id: string;
	username: string;
	display_name: string;
	avatar_url: string;
	/** True only for the bot's owner (SUPERADMIN_USER_ID). */
	is_superadmin: boolean;
};

export type AdminOverview = {
	/** Ever installed, including servers that removed the bot. */
	total_guilds: number;
	active_guilds: number;
	joined_last_30: number;
	left_last_30: number;
	/** Among active guilds. */
	premium_guilds: number;
	free_guilds: number;
	tickets_opened_last_30: number;
	tickets_closed_last_30: number;
	/** Active guilds that opened at least one ticket in the last 30 days. */
	using_guilds_last_30: number;
	/** Invite link clicks per ?ref= source, last 30 days, most first. */
	invite_sources: { source: string; clicks: number }[];
};

export type AdminGuild = {
	id: string;
	name: string;
	icon_url: string | null;
	tier: 'free' | 'premium';
	joined_at: string;
	left_at: string | null;
	tickets_total: number;
	tickets_last_30: number;
	last_ticket_at: string | null;
	ticket_types: number;
	/** Whether any ticket panel has been published, i.e. setup is finished. */
	panel_published: boolean;
};

export type Guild = {
	id: string;
	name: string;
	icon_url: string | null;
	bot_present: boolean;
	/** False for members who only have a dashboard role. */
	can_manage: boolean;
	/** What the user may do here. Owners are server managers. */
	level: AccessLevel;
};

export type AccessLevel = 'viewer' | 'support' | 'admin' | 'owner';

const levelRank: Record<AccessLevel, number> = { viewer: 1, support: 2, admin: 3, owner: 4 };

/** Whether a user at `level` may do what `min` allows. */
export const atLeast = (level: AccessLevel | undefined, min: AccessLevel) =>
	!!level && levelRank[level] >= levelRank[min];

/** A role that grants dashboard access at a level. */
export type DashboardRole = { role_id: string; level: Exclude<AccessLevel, 'owner'> };

export type GuildSettings = {
	transcript_retention_days: number | null;
	dashboard_roles: DashboardRole[];
	log_channel_id: string | null;
};

export type Channel = {
	id: string;
	name: string;
	kind: 'text' | 'announcement' | 'category';
	parent_id: string | null;
	position: number;
};

export type Role = {
	id: string;
	name: string;
	color: number;
	position: number;
};

export type TicketMode = 'channel' | 'thread';

export type QuestionStyle = 'short' | 'paragraph';

/** A question asked when a member opens a ticket. */
export type Question = {
	label: string;
	placeholder: string;
	style: QuestionStyle;
	required: boolean;
};

export const MAX_QUESTIONS = 5;

/** A short, pre-written answer to a common question, shown before a member opens a ticket. */
export type Answer = {
	title: string;
	body: string;
};

export const MAX_ANSWERS = 5;
export const MAX_ANSWER_TITLE = 100;
export const MAX_ANSWER_BODY = 300;

export type TicketType = {
	id: number;
	guild_id: string;
	name: string;
	emoji: string;
	description: string;
	mode: TicketMode;
	parent_id: string | null;
	support_role_ids: string[];
	name_format: string;
	welcome_message: string;
	max_open_per_user: number;
	questions: Question[];
	/** Shown before the form (or welcome message) when a member clicks a ticket button. */
	answers: Answer[];
	/** Hours without activity before a ticket closes itself, or null for never. */
	auto_close_hours: number | null;
	/** Members need one of these roles to open this type; empty means anyone. */
	required_role_ids: string[];
	/** Members with one of these roles can't open this type. */
	blocked_role_ids: string[];
	/** Minutes a member waits after their last ticket of this type closes; 0 is no wait. */
	cooldown_minutes: number;
	/** Whether the opener is asked to rate the ticket when it closes. */
	ask_rating: boolean;
	/** The wording of that request; empty uses the default. */
	rating_prompt: string;
	/** The colour of this type's button on its ticket panel. */
	button_style: ButtonStyle;
	/** Replaces the type's name on the button when set. */
	button_label: string;
	/** What claiming does to the rest of the support team (channel tickets only). */
	claim_lock: ClaimLock;
	/** Support roles that keep full access despite the lock. */
	claim_lock_exempt_role_ids: string[];
	/** How quickly the team aims to reply, in minutes; null for no target. */
	reply_target_minutes: number | null;
	/** Minutes a ticket may wait on the team before staff are reminded; null for never. */
	reminder_minutes: number | null;
	reminder_repeat: boolean;
	reminder_where: 'ticket' | 'log' | 'both';
	reminder_ping: 'claimer' | 'roles' | 'none';
	/** Category closed channel tickets are kept in, read only; null deletes the channel. */
	closed_parent_id: string | null;
	/** Whether the opener can still read a kept channel. */
	closed_member_access: 'read' | 'hidden';
	/** Days a kept channel stays before it's deleted. */
	closed_keep_days: number;
	/** Who a new channel ticket pings; thread tickets always ping the support roles. */
	notify_on_open: 'roles' | 'custom' | 'none';
	/** The role pinged when notify_on_open is 'custom'. */
	notify_role_id: string | null;
	/** Shares out new tickets round robin among staff who've opted in with /ticket available. */
	auto_assign: boolean;
	created_at: string;
};

/** The keep times offered for closed channels, matching the API. */
export const CLOSED_KEEP_DAYS = [1, 3, 7, 14, 30];

export type ClaimLock = 'off' | 'read_only' | 'hidden';

/** The waits offered for reply targets and reminders, matching the API. */
export const MINUTE_OPTIONS = [15, 30, 60, 120, 240, 480, 720, 1440, 2880];

/** Describes a number of minutes, e.g. "30 minutes", "2 hours", "1 day". */
export function minutesLabel(m: number): string {
	if (m % 1440 === 0) return `${m / 1440} day${m === 1440 ? '' : 's'}`;
	if (m % 60 === 0) return `${m / 60} hour${m === 60 ? '' : 's'}`;
	return `${m} minutes`;
}

/** When a waiting ticket passes its type's reply target, or null if it has none. */
export function overdueAt(t: Ticket, types: TicketType[]): number | null {
	if (t.status !== 'open' || !t.waiting_on_staff || t.on_hold || !t.waiting_since) return null;
	const target = types.find((x) => x.id === t.ticket_type_id)?.reply_target_minutes;
	if (!target) return null;
	return new Date(t.waiting_since).getTime() + target * 60_000;
}

export type ButtonStyle = 'primary' | 'secondary' | 'success' | 'danger';
export const MAX_BUTTON_LABEL = 80;

export const MAX_RATING_PROMPT = 300;

export type TicketTypeInput = Omit<TicketType, 'id' | 'guild_id' | 'created_at'>;

export type PanelStyle = 'buttons' | 'dropdown';

export type Panel = {
	id: number;
	guild_id: string;
	title: string;
	description: string;
	color: number;
	style: PanelStyle;
	/** Optional https images on the message. */
	image_url: string;
	thumbnail_url: string;
	/** The dropdown's prompt; empty uses the default. */
	placeholder: string;
	channel_id: string | null;
	message_id: string | null;
	ticket_type_ids: number[];
	created_at: string;
};

export type PanelInput = Pick<
	Panel,
	'title' | 'description' | 'color' | 'style' | 'image_url' | 'thumbnail_url' | 'placeholder' | 'ticket_type_ids'
>;

/** Matches DefaultPlaceholder in the bot. */
export const DEFAULT_DROPDOWN_PLACEHOLDER = 'Choose a topic…';

/** A member who can't open tickets in a server. */
export type Block = {
	guild_id: string;
	user_id: string;
	user_name: string;
	reason: string;
	blocked_by: string;
	blocked_by_name: string;
	created_at: string;
	expires_at: string | null;
};

export const MAX_BLOCK_REASON = 200;

/** Matches blockDurations in the API and bot. '' blocks until unblocked. */
export const blockDurationOptions: { value: string; label: string }[] = [
	{ value: '', label: 'Until unblocked' },
	{ value: '1h', label: '1 hour' },
	{ value: '1d', label: '1 day' },
	{ value: '7d', label: '7 days' },
	{ value: '30d', label: '30 days' }
];

/** An answer the team sends often, from the reply box or with /reply. */
export type SavedReply = {
	id: number;
	name: string;
	content: string;
	updated_at: string;
};

export type SavedReplyInput = Pick<SavedReply, 'name' | 'content'>;

export const MAX_SAVED_REPLY_NAME = 100;
export const MAX_SAVED_REPLY_CONTENT = 2000;

export type Ticket = {
	id: number;
	number: number;
	ticket_type_id: number | null;
	type_name: string;
	mode: TicketMode;
	channel_id: string;
	opener_id: string;
	opener_name: string;
	claimed_by: string | null;
	claimed_by_name: string | null;
	status: 'open' | 'closed';
	close_reason: string;
	closed_by_name: string | null;
	opened_at: string;
	first_response_at: string | null;
	closed_at: string | null;
	last_activity_at: string;
	/** True when the member sent the last message, so the team owes a reply. */
	waiting_on_staff: boolean;
	/** When the team started owing a reply; null while it's the member's turn. */
	waiting_since: string | null;
	/** Waiting on something else: out of the queue, reminders and auto-close. */
	on_hold: boolean;
	hold_reason: string;
	/** The message that matched a search, when the search matched what was said. */
	match?: { author_name: string; snippet: string };
	/** The opener's rating and comment, once the ticket has been rated. */
	feedback: { rating: number; comment: string; created_at: string } | null;
	/** For a closed channel ticket whose channel is kept: when it's deleted. Reopen works until then. */
	reopen_until: string | null;
};

/** Whether a closed ticket can be reopened: threads always, kept channels until they're deleted. */
export function canReopen(t: Ticket): boolean {
	if (t.status !== 'closed') return false;
	if (t.mode === 'thread') return true;
	return !!t.reopen_until && new Date(t.reopen_until) > new Date();
}

/** A server member from a search: someone to assign a ticket to, or add to it. */
export type Member = { id: string; name: string; avatar_url: string };

/** Someone with their own access to a ticket. Whoever opened it can't be removed. */
export type TicketMember = Member & { opener: boolean };

/** The most tickets one bulk close request takes. */
export const MAX_BULK_CLOSE = 50;

/** What POST /guilds/{id}/tickets/close did. Pending tickets weren't reached in time; send them again. */
export type BulkCloseResult = {
	closed: Ticket[];
	failed: { id: number; error: string }[];
	pending: number[];
};

/** Wrap the matching words in a search snippet; see `snippetParts`. */
export const MATCH_START = '\ue000';
export const MATCH_END = '\ue001';

/** Splits a search snippet into runs, marking the ones that matched. */
export function snippetParts(snippet: string): { text: string; hit: boolean }[] {
	const out: { text: string; hit: boolean }[] = [];
	for (const [i, chunk] of snippet.split(MATCH_START).entries()) {
		if (i === 0) {
			if (chunk) out.push({ text: chunk, hit: false });
			continue;
		}
		const end = chunk.indexOf(MATCH_END);
		if (end < 0) {
			out.push({ text: chunk, hit: false });
			continue;
		}
		out.push({ text: chunk.slice(0, end), hit: true });
		if (chunk.length > end + 1) out.push({ text: chunk.slice(end + 1), hit: false });
	}
	return out;
}

export type Embed = {
	/** Who wrote it, e.g. the staff member behind a dashboard reply. */
	author?: { name: string; icon_url?: string };
	title?: string;
	description?: string;
	color?: number;
	footer?: string;
	fields?: { name: string; value: string }[];
};

export type Attachment = { name: string; url: string; size: number; content_type?: string };

export type TranscriptMessage = {
	id: string;
	author_id: string;
	author_name: string;
	author_avatar: string;
	author_bot: boolean;
	/** The staff member a bot-posted reply was sent for (dashboard, /reply), else null. */
	sent_by: string | null;
	/** Whether the message is the team's (a support role, a server manager, or sent for staff). */
	author_staff: boolean;
	content: string;
	embeds: Embed[];
	attachments: Attachment[];
	created_at: string;
	edited_at: string | null;
	deleted_at: string | null;
};

/** A private note staff left on a ticket. Only the team sees notes. */
export type TicketNote = {
	id: number;
	author_id: string;
	author_name: string;
	content: string;
	created_at: string;
};

/** Matches store.MaxNoteLength. */
export const MAX_NOTE = 1000;

export type Transcript = {
	ticket: Ticket;
	messages: TranscriptMessage[];
	/** Private staff notes. Only sent to the team, never to the member who opened the ticket. */
	notes?: TicketNote[];
	/** can_manage means the viewer has dashboard access to the server. */
	guild: { id: string; name: string; icon_url: string | null; can_manage: boolean };
	/** Current role and channel names by ID, for showing mentions. */
	roles: Record<string, string>;
	channels: Record<string, string>;
};

/** A ticket the current user opened, wherever it is, for the "my tickets" page. */
export type MyTicket = {
	ticket: Ticket;
	guild: { id: string; name: string; icon_url: string | null };
};

/**
 * Something in a server's setup that stops tickets working. `kind` says where
 * it's fixed: a ticket type or ticket panel (by `id`), a ticket type that
 * isn't on any published ticket panel ("unlisted"), or the log channel.
 */
export type SetupProblem = {
	kind: 'ticket_type' | 'panel' | 'unlisted' | 'log_channel';
	id?: number;
	title: string;
	detail: string;
};

export type Stats = {
	tickets: { open: number; opened_week: number; closed_total: number };
	tier: 'free' | 'premium';
	limits: {
		max_panels: number;
		max_ticket_types: number;
		max_saved_replies: number;
		/** 0 means no cap. */
		max_transcript_retention_days: number;
		branding: boolean;
	};
	/** Whether this server has ever started a Stripe subscription. */
	has_billing_customer: boolean;
	/** Whether the dashboard is running with Stripe billing configured at all. */
	billing_enabled: boolean;
	/** Where to ask for a review (the bot's top.gg page), or '' when it isn't listed. */
	review_url: string;
};

export type AnalyticsSummary = {
	opened: number;
	closed: number;
	first_response_median_seconds: number | null;
	resolution_median_seconds: number | null;
	rating_avg: number | null;
	rating_count: number;
	closed_unanswered: number;
	/** Tickets whose type has a reply target, and how many met it. */
	target_measured: number;
	target_met: number;
	transcripts: number;
	team_messages: number;
	member_messages: number;
	one_touch: number;
	/** Median time from open to claim, for tickets opened in the window that were claimed. */
	claim_median_seconds: number | null;
	/** Tickets closed in the window that had been reopened before their final close. */
	reopened: number;
	/** Members who said a ticket type's suggested answers solved it, without opening one. */
	answers_deflected: number;
};

export type Analytics = {
	/** 0 means all time. */
	days: number;
	from: string;
	bucket: 'day' | 'week' | 'month';
	/** The IANA zone the heatmap, response-by-hour and series are bucketed in. */
	timezone: string;
	summary: AnalyticsSummary;
	previous: AnalyticsSummary | null;
	open_now: number;
	waiting_now: number;
	on_hold_now: number;
	/** Waiting longer than the type's reply target. */
	overdue_now: number;
	/** Median age of tickets open right now. */
	backlog_age_median_seconds: number | null;
	series: { date: string; opened: number; closed: number; backlog: number }[];
	/** [weekday, Sunday first][hour], in timezone. */
	heatmap: number[][];
	response_by_hour: (number | null)[];
	/** Counts of 1 to 5 star ratings. */
	ratings: number[];
	closures: { team: number; member: number; auto: number; deleted: number };
	close_reasons: { reason: string; count: number }[];
	threads: number;
	channels: number;
	by_type: {
		type_id: number | null;
		name: string;
		emoji: string;
		opened: number;
		first_response_median_seconds: number | null;
		resolution_median_seconds: number | null;
		rating_avg: number | null;
		rating_count: number;
		/** False for types that don't ask for ratings. */
		asks_rating: boolean;
		target_measured: number;
		target_met: number;
	}[];
	staff: {
		user_id: string;
		name: string;
		claimed: number;
		closed: number;
		replies: number;
		tickets: number;
		avg_rating: number | null;
	}[];
};

export class ApiError extends Error {
	constructor(
		public status: number,
		message: string,
		/** The form field the error relates to, for validation errors. */
		public field?: string
	) {
		super(message);
	}
}

export const loginURL = '/api/auth/login';

/**
 * Calls the API. A 401 sends the user to the Discord login page unless
 * `redirectOnUnauthorized` is false.
 */
export async function api<T>(
	path: string,
	init: RequestInit & { redirectOnUnauthorized?: boolean } = {}
): Promise<T> {
	const { redirectOnUnauthorized = true, ...rest } = init;
	const res = await fetch(`/api${path}`, {
		credentials: 'same-origin',
		...rest,
		headers: { 'Content-Type': 'application/json', ...rest.headers }
	});

	if (res.status === 401 && redirectOnUnauthorized) {
		// Come back here after logging in, e.g. to a transcript linked from a DM.
		const next = window.location.pathname + window.location.search;
		window.location.href = `${loginURL}?next=${encodeURIComponent(next)}`;
	}
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		throw new ApiError(res.status, body.error ?? res.statusText, body.field || undefined);
	}
	if (res.status === 204) return undefined as T;
	return res.json();
}

/** Builds a JSON request for `api`. */
export function send(method: 'POST' | 'PATCH' | 'DELETE', body?: unknown): RequestInit {
	return { method, body: body === undefined ? undefined : JSON.stringify(body) };
}

/** Returns the logged-in user, or null if there is no session. */
export async function getMe(): Promise<User | null> {
	try {
		return await api<User>('/me', { redirectOnUnauthorized: false });
	} catch (e) {
		if (e instanceof ApiError && e.status === 401) return null;
		throw e;
	}
}

export function errorMessage(e: unknown): string {
	return e instanceof Error ? e.message : 'Something went wrong.';
}

export const intToHex = (n: number) => '#' + n.toString(16).padStart(6, '0');
export const hexToInt = (hex: string) => parseInt(hex.replace('#', ''), 16) || 0;
