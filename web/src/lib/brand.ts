/** Product name, set with VITE_APP_NAME at build time. */
export const APP_NAME: string = import.meta.env.VITE_APP_NAME || 'Ticket Tower';

/**
 * Where people can ask for help, such as a support server invite. Set with
 * VITE_SUPPORT_URL at build time; defaults to the GitHub issue tracker.
 */
export const SUPPORT_URL: string =
	import.meta.env.VITE_SUPPORT_URL || 'https://github.com/adammcgrogan/tickettower/issues';

/**
 * The public site's address, for canonical links, the sitemap and link
 * previews. Set with VITE_SITE_URL at build time (no trailing slash).
 */
export const SITE_URL: string = (import.meta.env.VITE_SITE_URL || 'https://tickettower.net').replace(/\/$/, '');
