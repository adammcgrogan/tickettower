/**
 * A small, safe renderer for the subset of Discord markdown seen in tickets.
 * Input is HTML-escaped before any formatting is applied, and only tags
 * produced here reach the output.
 */

const ESCAPES: Record<string, string> = {
	'&': '&amp;',
	'<': '&lt;',
	'>': '&gt;',
	'"': '&quot;',
	"'": '&#39;'
};

export const escapeHTML = (s: string) => s.replace(/[&<>"']/g, (c) => ESCAPES[c]);

// Formatted fragments are stashed behind NUL-delimited placeholders so later
// passes can't alter them. NUL never appears in the (sanitised) input.
const PLACEHOLDER = /\u0000(\d+)\u0000/g;

const link = (url: string, text: string) =>
	`<a href="${url}" target="_blank" rel="noopener noreferrer nofollow" class="md-link">${text}</a>`;

export function renderMarkdown(src: string, users: Map<string, string> = new Map()): string {
	const stash: string[] = [];
	const keep = (html: string) => `\u0000${stash.push(html) - 1}\u0000`;

	let s = src
		.replace(/\u0000/g, '')
		.replace(/```(?:[\w+-]*\n)?([\s\S]*?)```/g, (_, code: string) =>
			keep(`<pre class="md-pre"><code>${escapeHTML(code.replace(/^\n+|\n+$/g, ''))}</code></pre>`)
		)
		.replace(/`([^`\n]+)`/g, (_, code: string) => keep(`<code class="md-code">${escapeHTML(code)}</code>`));

	s = escapeHTML(s)
		.replace(/&lt;@!?(\d+)&gt;/g, (_, id: string) =>
			keep(`<span class="md-mention">@${escapeHTML(users.get(id) ?? 'unknown-user')}</span>`)
		)
		.replace(/&lt;@&amp;\d+&gt;/g, () => keep('<span class="md-mention">@role</span>'))
		.replace(/&lt;#\d+&gt;/g, () => keep('<span class="md-mention">#channel</span>'))
		.replace(/&lt;a?:(\w+):\d+&gt;/g, ':$1:')
		.replace(/&lt;t:(\d+)(?::[a-zA-Z])?&gt;/g, (_, ts: string) =>
			keep(`<span class="md-code">${escapeHTML(new Date(Number(ts) * 1000).toLocaleString())}</span>`)
		)
		.replace(/\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)/g, (_, text: string, url: string) => keep(link(url, text)))
		.replace(/(^|[\s(])(https?:\/\/[^\s<]+)/g, (_, pre: string, url: string) => pre + keep(link(url, url)))
		.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
		.replace(/__(.+?)__/g, '<u>$1</u>')
		.replace(/(^|[^*\w])\*(?!\s)(.+?)\*(?!\*)/g, '$1<em>$2</em>')
		.replace(/(^|[^_\w])_(?!\s)(.+?)_(?![_\w])/g, '$1<em>$2</em>')
		.replace(/~~(.+?)~~/g, '<s>$1</s>')
		.replace(/\|\|(.+?)\|\|/g, '<span class="md-spoiler">$1</span>');

	s = s
		.split('\n')
		.map((line) => {
			const heading = line.match(/^(#{1,3}) (.+)$/);
			if (heading) return `<span class="md-h${heading[1].length}">${heading[2]}</span>`;
			const quote = line.match(/^&gt; ?(.*)$/);
			if (quote) return `<span class="md-quote">${quote[1]}</span>`;
			return line;
		})
		.join('\n');

	// Restore stashed fragments; loop because fragments can nest.
	for (let i = 0; i < 5 && s.includes('\u0000'); i++) {
		s = s.replace(PLACEHOLDER, (_, n: string) => stash[Number(n)] ?? '');
	}
	return s;
}
