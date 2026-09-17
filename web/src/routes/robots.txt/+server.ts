import { SITE_URL } from '$lib/brand';

export const prerender = true;

// Crawl the public pages; the dashboard and API need a login anyway.
export function GET() {
	const body = `User-agent: *\nDisallow: /api/\nDisallow: /servers/\nDisallow: /admin\nDisallow: /me/\nDisallow: /transcripts/\n\nSitemap: ${SITE_URL}/sitemap.xml\n`;
	return new Response(body, { headers: { 'Content-Type': 'text/plain' } });
}
