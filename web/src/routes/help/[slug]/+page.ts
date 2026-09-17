import { guides } from '$lib/help';
import type { EntryGenerator } from './$types';

// Every guide is prerendered, even one nothing links to yet.
export const entries: EntryGenerator = () => guides.map((g) => ({ slug: g.slug }));
