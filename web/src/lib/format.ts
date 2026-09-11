import type { Ticket } from './api';

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

/**
 * Who a ticket is waiting on. Staff see whose turn it is; members (on their
 * own transcript) just see open or closed.
 */
export function ticketState(t: Ticket, staff = true): { label: string; tone: 'waiting' | 'default' | 'closed' } {
	if (t.status === 'closed') return { label: 'Closed', tone: 'closed' };
	if (!staff) return { label: 'Open', tone: 'default' };
	return t.waiting_on_staff
		? { label: 'Waiting on your team', tone: 'waiting' }
		: { label: 'Waiting on the member', tone: 'default' };
}

const units: [Intl.RelativeTimeFormatUnit, number][] = [
	['year', 31536000],
	['month', 2592000],
	['week', 604800],
	['day', 86400],
	['hour', 3600],
	['minute', 60]
];

/** Describes a number of hours, e.g. "12 hours", "2 days" or "1 week". */
export function hoursLabel(hours: number): string {
	const [n, unit] = hours % 168 === 0 ? [hours / 168, 'week'] : hours % 24 === 0 ? [hours / 24, 'day'] : [hours, 'hour'];
	return `${n} ${unit}${n === 1 ? '' : 's'}`;
}

/** Formats a timestamp relative to now, e.g. "3 hours ago". */
export function timeAgo(iso: string): string {
	const diff = (new Date(iso).getTime() - Date.now()) / 1000;
	for (const [unit, secs] of units) {
		if (Math.abs(diff) >= secs) return rtf.format(Math.round(diff / secs), unit);
	}
	return 'just now';
}

/** Formats a duration in seconds compactly, e.g. "4m", "2h 15m", "3d 4h". */
export function formatDuration(sec: number | null | undefined): string {
	if (sec == null) return '—';
	if (sec < 60) return `${Math.round(sec)}s`;
	const m = Math.round(sec / 60);
	if (m < 60) return `${m}m`;
	const h = Math.floor(m / 60);
	if (h < 24) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
	const d = Math.floor(h / 24);
	return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
}

/** Custom emoji can't be rendered without Discord's CDN, so show their name. */
export const emojiText = (e: string) => e.replace(/^<a?:(\w+):\d+>$/, ':$1:');

export function discordURL(guildId: string, channelId: string, messageId?: string | null) {
	return `https://discord.com/channels/${guildId}/${channelId}${messageId ? '/' + messageId : ''}`;
}
