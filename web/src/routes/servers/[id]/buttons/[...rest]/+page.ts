import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// Keep old /buttons links working after the dashboard URL moved back to /panels.
export const load: PageLoad = ({ params }) => {
	redirect(308, `/servers/${params.id}/panels${params.rest ? `/${params.rest}` : ''}`);
};
