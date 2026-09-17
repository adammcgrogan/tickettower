import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// Public pages (landing, help, privacy, terms) are prerendered so
			// search engines and link previews see them; everything else is a
			// client-rendered SPA. The Go API serves the prerendered files and
			// falls back to 200.html for client-side routes.
			adapter: adapter({ fallback: '200.html' }),
			prerender: {
				// Crawling the public pages finds links to the dashboard and the
				// API, which aren't prerendered; only a broken public link fails
				// the build.
				handleHttpError: ({ path, message }) => {
					if (path.startsWith('/api/')) return;
					throw new Error(message);
				}
			},

			// Written into index.html as a <meta> tag, with a hash for the
			// bootstrap script. Fonts are bundled; images come from Discord's
			// CDN and from embeds, which can link anywhere. The API adds the
			// headers a <meta> tag can't carry (frame-ancestors and friends).
			csp: {
				mode: 'hash',
				directives: {
					'default-src': ['self'],
					'script-src': ['self'],
					'style-src': ['self', 'unsafe-inline'],
					'img-src': ['self', 'data:', 'https:'],
					'font-src': ['self'],
					'connect-src': ['self'],
					'object-src': ['none'],
					'base-uri': ['self'],
					'form-action': ['self']
				}
			}
		})
	],
	server: {
		proxy: {
			// Matches the API's PORT; `make web` passes it through from .env.
			'/api': `http://localhost:${process.env.PORT || 8080}`
		}
	}
});
