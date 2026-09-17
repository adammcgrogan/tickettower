<script lang="ts">
	import { APP_NAME, SITE_URL } from '$lib/brand';

	// Title, description and link preview tags for a public, prerendered page.
	let { title, description, path }: { title: string; description: string; path: string } = $props();

	const url = $derived(SITE_URL + (path === '/' ? '' : path));
	const image = `${SITE_URL}/og.png`;
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={url || SITE_URL} />
	<meta property="og:type" content="website" />
	<meta property="og:site_name" content={APP_NAME} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={url || SITE_URL} />
	<meta property="og:image" content={image} />
	<meta property="og:image:width" content="1200" />
	<meta property="og:image:height" content="630" />
	<meta property="og:image:alt" content="{APP_NAME}, a free ticket bot for Discord" />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	<meta name="twitter:image" content={image} />
</svelte:head>
