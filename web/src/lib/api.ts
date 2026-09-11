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
	opened_at: string;
	first_response_at: string | null;
	closed_at: string | null;
};

export type Stats = {
	tickets: { open: number; opened_week: number };
	limits: { max_panels: number; max_ticket_types: number };
};

export type Analytics = {
	days: number;
	daily: { date: string; opened: number; closed: number }[];
	opened: number;
	closed: number;
	first_response_median_seconds: number | null;
	resolution_median_seconds: number | null;
	rating_avg: number | null;
	rating_count: number;
	by_type: { name: string; count: number }[];
	staff: { user_id: string; name: string; claimed: number; closed: number; avg_rating: number | null }[];
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
		window.location.href = loginURL;
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
