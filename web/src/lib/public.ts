// Shared by the public, prerendered pages.

/** Invite link sources the API counts (inviteSources in internal/api/server.go). */
const REF = /^[a-z]{1,20}$/;

/**
 * The invite link for a page, tagged with where the visitor came from: a
 * ?ref= on the page's own URL (a bot list, a video) wins over the page's
 * default, so the source survives a visit to the landing page.
 */
export function inviteHref(fallbackRef: string, search = ''): string {
	const ref = new URLSearchParams(search).get('ref')?.toLowerCase() ?? '';
	return `/api/invite?ref=${REF.test(ref) ? ref : fallbackRef}`;
}
