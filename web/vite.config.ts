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

			// Build a static SPA; the Go API serves it and falls back to
			// index.html for client-side routes.
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	server: {
		proxy: {
			// Matches the API's PORT; `make web` passes it through from .env.
			'/api': `http://localhost:${process.env.PORT || 8080}`
		}
	}
});
