export type User = {
	id: string;
	username: string;
	display_name: string;
	avatar_url: string;
};

export type Guild = {
	id: string;
	name: string;
	icon_url: string | null;
	bot_present: boolean;
	/** False for members who only have a dashboard role. */
	can_manage: boolean;
};

export type GuildSettings = {
	transcript_retention_days: number | null;
	dashboard_role_ids: string[];
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
	/** Hours without activity before a ticket closes itself, or null for never. */
	auto_close_hours: number | null;
	/** Members need one of these roles to open this type; empty means anyone. */
	required_role_ids: string[];
	/** Members with one of these roles can't open this type. */
	blocked_role_ids: string[];
	/** Minutes a member waits after their last ticket of this type closes; 0 is no wait. */
	cooldown_minutes: number;
	created_at: string;
};

export type TicketTypeInput = Omit<TicketType, 'id' | 'guild_id' | 'created_at'>;

export type PanelStyle = 'buttons' | 'dropdown';

export type Panel = {
	id: number;
	guild_id: string;
	title: string;
	description: string;
	color: number;
	style: PanelStyle;
	channel_id: string | null;
	message_id: string | null;
	ticket_type_ids: number[];
	created_at: string;
};

export type PanelInput = Pick<Panel, 'title' | 'description' | 'color' | 'style' | 'ticket_type_ids'>;

/** A member who can't open tickets in a server. */
export type Block = {
	guild_id: string;
	user_id: string;
	user_name: string;
	reason: string;
	blocked_by: string;
	blocked_by_name: string;
	created_at: string;
};

export const MAX_BLOCK_REASON = 200;

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
	/** The message that matched a search, when the search matched what was said. */
	match?: { author_name: string; snippet: string };
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
	content: string;
	embeds: Embed[];
	attachments: Attachment[];
	created_at: string;
	edited_at: string | null;
	deleted_at: string | null;
};

export type Transcript = {
	ticket: Ticket;
	messages: TranscriptMessage[];
	/** can_manage means the viewer has dashboard access to the server. */
	guild: { id: string; name: string; icon_url: string | null; can_manage: boolean };
	/** Current role and channel names by ID, for showing mentions. */
	roles: Record<string, string>;
	channels: Record<string, string>;
};

/**
 * Something in a server's setup that stops tickets working. `kind` says where
 * it's fixed: a ticket type or ticket buttons (by `id`), a ticket type that
 * isn't on any published ticket buttons ("unlisted"), or the log channel.
 */
export type SetupProblem = {
	kind: 'ticket_type' | 'panel' | 'unlisted' | 'log_channel';
	id?: number;
	title: string;
	detail: string;
};

export type Stats = {
	tickets: { open: number; opened_week: number };
	limits: { max_panels: number; max_ticket_types: number; max_saved_replies: number };
};

export type AnalyticsSummary = {
	opened: number;
	closed: number;
	first_response_median_seconds: number | null;
	resolution_median_seconds: number | null;
	rating_avg: number | null;
	rating_count: number;
	closed_unanswered: number;
	transcripts: number;
	team_messages: number;
	member_messages: number;
	one_touch: number;
};

export type Analytics = {
	/** 0 means all time. */
	days: number;
	from: string;
	bucket: 'day' | 'week' | 'month';
	summary: AnalyticsSummary;
	previous: AnalyticsSummary | null;
	open_now: number;
	waiting_now: number;
	series: { date: string; opened: number; closed: number; backlog: number }[];
	/** [weekday, Sunday first][hour], UTC. */
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
